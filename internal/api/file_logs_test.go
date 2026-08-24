package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/config"
	"github.com/fengin/composeboard/internal/filelog"
	"github.com/fengin/composeboard/internal/service"
	"github.com/gin-gonic/gin"
)

func TestGetFileLogBasesKeepsFeatureDisabledByDefault(t *testing.T) {
	handler := &Handler{FileLogs: filelog.NewManager(config.FileLogsConfig{}, t.TempDir(), nil, nil)}
	response := performFileLogRequest(t, http.MethodGet, handler.GetFileLogBases, "/bases", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDownloadFileLogStreamsGzipAndSupportsRange(t *testing.T) {
	rootPath := t.TempDir()
	content := "compressed-archive-placeholder"
	archivePath := filepath.Join(rootPath, "archive.log.gz")
	if err := os.WriteFile(archivePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{FileLogs: newAPIFileLogManager(t, rootPath)}

	query := url.Values{"base": {"project-data"}, "path": {"archive.log.gz"}}.Encode()
	response := performFileLogRequestWithHeaders(t, http.MethodGet, handler.DownloadFileLog, "/download?"+query, "", map[string]string{
		"Range": "bytes=0-9",
	})
	if response.Code != http.StatusPartialContent || response.Body.String() != content[:10] {
		t.Fatalf("unexpected download: status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
}

func TestDownloadFileLogRejectsTraversal(t *testing.T) {
	rootPath := t.TempDir()
	handler := &Handler{FileLogs: newAPIFileLogManager(t, rootPath)}
	query := url.Values{"base": {"project-data"}, "path": {"../secret.log"}}.Encode()
	response := performFileLogRequest(t, http.MethodGet, handler.DownloadFileLog, "/download?"+query, "")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "INVALID_LOG_PATH") {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMappingAPIValidatesPersistsAndDeletesRelativePath(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootPath, "redis", "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "redis", "logs", "redis.log"), []byte("ready\n"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{FileLogs: newAPIFileLogManager(t, rootPath)}

	validationBody := `{"base_id":"project-data","relative_path":"redis/logs"}`
	validation := performFileLogRequest(t, http.MethodPost, handler.ValidateFileLogMapping, "/validate", validationBody)
	if validation.Code != http.StatusOK || !strings.Contains(validation.Body.String(), `"valid":true`) {
		t.Fatalf("validation failed: status=%d body=%s", validation.Code, validation.Body.String())
	}

	saveBody := `{"directories":[{"id":"default","name":"Redis logs","base_id":"project-data","relative_path":"redis/logs"}]}`
	saved := performFileLogRequestWithParam(t, http.MethodPut, handler.SaveServiceFileLogMapping, "/mapping", saveBody, "name", "redis")
	if saved.Code != http.StatusOK {
		t.Fatalf("save failed: status=%d body=%s", saved.Code, saved.Body.String())
	}

	source := performFileLogRequestWithParam(t, http.MethodGet, handler.GetServiceFileLogSource, "/source", "", "name", "redis")
	if source.Code != http.StatusOK || !strings.Contains(source.Body.String(), `"mode":"manual"`) {
		t.Fatalf("source failed: status=%d body=%s", source.Code, source.Body.String())
	}

	deleted := performFileLogRequestWithParam(t, http.MethodDelete, handler.DeleteServiceFileLogMapping, "/mapping", "", "name", "redis")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete failed: status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestStreamFileLogValidatesTailBeforeSSEHeaders(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootPath, "info.log"), []byte("ready\n"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{FileLogs: newAPIFileLogManager(t, rootPath)}
	query := url.Values{"base": {"project-data"}, "path": {"info.log"}, "tail": {"1"}}.Encode()
	response := performFileLogRequest(t, http.MethodGet, handler.StreamFileLog, "/stream?"+query, "")
	if response.Code != http.StatusBadRequest || strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("invalid tail was not rejected before SSE: status=%d content-type=%s", response.Code, response.Header().Get("Content-Type"))
	}
}

func newAPIFileLogManager(t *testing.T, basePath string) *filelog.Manager {
	t.Helper()
	projectDir := filepath.Dir(basePath)
	composeContent := "services:\n  redis:\n    image: redis:7\n"
	if err := os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("DATA_ROOT="+filepath.ToSlash(basePath)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	executor := compose.NewExecutor(projectDir, "auto")
	serviceManager := service.NewServiceManager(projectDir, nil, executor)
	return filelog.NewManager(config.FileLogsConfig{
		Enabled: true,
		AllowedBases: []config.FileLogBaseConfig{{
			ID: "project-data", Name: "Project Data", Path: basePath,
		}},
		FollowExtensions:   []string{".log"},
		DownloadExtensions: []string{".log", ".gz"},
		Discovery: config.FileLogDiscoveryConfig{
			MaxDepth: 2, MaxEntries: 2000, TimeoutMS: 300, CacheTTLSeconds: 60,
		},
	}, projectDir, serviceManager, nil)
}

func performFileLogRequest(t *testing.T, method string, handler gin.HandlerFunc, target string, body string) *httptest.ResponseRecorder {
	return performFileLogRequestWithHeaders(t, method, handler, target, body, nil)
}

func performFileLogRequestWithParam(t *testing.T, method string, handler gin.HandlerFunc, target string, body string, key string, value string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		context.Request.Header.Set("Content-Type", "application/json")
	}
	context.Params = gin.Params{{Key: key, Value: value}}
	handler(context)
	return response
}

func performFileLogRequestWithHeaders(t *testing.T, method string, handler gin.HandlerFunc, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler(context)
	return response
}
