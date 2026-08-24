package filelog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/config"
	"github.com/fengin/composeboard/internal/service"
)

func TestServiceDiscoveryOnlyReturnsSelectedServiceCandidates(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	writeLogFile(t, filepath.Join(dataRoot, "emqx", "log", "emqx.log"), "emqx ready\n")
	writeLogFile(t, filepath.Join(dataRoot, "inxvision", "logs", "inxaiot-starter-platform", "platform.log"), "platform ready\n")
	writeLogFile(t, filepath.Join(dataRoot, "inxvision", "logs", "inxaiot-starter-platform", "2026-08", "platform.log.gz"), "archive")
	writeLogFile(t, filepath.Join(dataRoot, "inxvision", "logs", "inxaiot-starter-app", "app.log"), "app ready\n")
	if err := os.MkdirAll(filepath.Join(dataRoot, "redis", "data"), 0755); err != nil {
		t.Fatal(err)
	}
	writeLogFile(t, filepath.Join(dataRoot, "redis", "data", "dump.rdb"), "not a log")

	writeProject(t, projectDir, dataRoot, `
services:
  emqx:
    image: emqx/emqx:latest
    volumes:
      - ${EMQX_LOG_DIR}:/opt/emqx/log
  redis:
    image: redis:7
    volumes:
      - ${REDIS_DATA_DIR}:/data
  inxvision-starter-platform:
    image: registry.example.com/inxaiot-starter-platform:1.0.0
    volumes:
      - ${APP_LOGS_DIR}:/data/logs
`, `DATA_ROOT=`+filepath.ToSlash(dataRoot)+`
EMQX_LOG_DIR=${DATA_ROOT}/emqx/log
REDIS_DATA_DIR=${DATA_ROOT}/redis/data
APP_LOGS_DIR=${DATA_ROOT}/inxvision/logs
`)

	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	emqx, err := manager.GetServiceSource(context.Background(), "emqx", false)
	if err != nil {
		t.Fatal(err)
	}
	if emqx.Mode != "automatic" || emqx.Selected == nil || emqx.Selected.Path != "emqx/log" {
		t.Fatalf("unexpected emqx source: %#v", emqx)
	}

	redis, err := manager.GetServiceSource(context.Background(), "redis", false)
	if err != nil {
		t.Fatal(err)
	}
	if redis.Mode != "unmatched" || redis.Selected != nil || len(redis.Directories) != 0 {
		t.Fatalf("redis must not fall back to another service directory: %#v", redis)
	}

	platform, err := manager.GetServiceSource(context.Background(), "inxvision-starter-platform", false)
	if err != nil {
		t.Fatal(err)
	}
	if platform.Mode != "automatic" || platform.Selected == nil || platform.Selected.Path != "inxvision/logs/inxaiot-starter-platform" {
		t.Fatalf("unexpected platform source: %#v", platform)
	}
	archiveFound := false
	for _, directory := range platform.Directories {
		if directory.Path == "inxvision/logs/inxaiot-starter-app" {
			t.Fatal("selected service discovery leaked another application's log directory")
		}
		if directory.Path == "inxvision/logs/inxaiot-starter-platform/2026-08" {
			archiveFound = true
		}
	}
	if !archiveFound {
		t.Fatal("service source did not expose its bounded archive subdirectory")
	}
}

func TestDiscoveryStopsAtConfiguredEntryLimit(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	bulkRoot := filepath.Join(dataRoot, "bulk")
	for index := 0; index < 50; index++ {
		writeLogFile(t, filepath.Join(bulkRoot, "directory-"+twoDigits(index), "worker.log"), "line\n")
	}
	writeProject(t, projectDir, dataRoot, `
services:
  bulk:
    image: example/bulk:latest
    volumes:
      - ${BULK_DIR}:/data
`, `DATA_ROOT=`+filepath.ToSlash(dataRoot)+`
BULK_DIR=${DATA_ROOT}/bulk
`)

	discovery := defaultDiscoveryConfig()
	discovery.MaxEntries = 10
	manager := newProjectManager(projectDir, dataRoot, discovery)
	result, err := manager.GetServiceSource(context.Background(), "bulk", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DiscoveryTruncated {
		t.Fatalf("expected bounded discovery to truncate: %#v", result)
	}
	if result.Selected != nil {
		t.Fatal("truncated ambiguous discovery must not select a directory")
	}
}

func TestManualMappingPersistsAsRelativePathAndOverridesAutomaticDiscovery(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	writeLogFile(t, filepath.Join(dataRoot, "redis", "logs", "redis.log"), "ready\n")
	writeProject(t, projectDir, dataRoot, `
services:
  redis:
    image: redis:7
`, "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\n")

	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	mapping, err := manager.SaveMapping("redis", []MappingDirectory{{
		ID:           "default",
		Name:         "Redis logs",
		BaseID:       "project-data",
		RelativePath: "redis/logs",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Directories[0].RelativePath != "redis/logs" {
		t.Fatalf("mapping stored an unexpected path: %#v", mapping)
	}
	if _, err := os.Stat(filepath.Join(projectDir, mappingFileName)); err != nil {
		t.Fatalf("mapping file was not persisted: %v", err)
	}

	reloaded := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	result, err := reloaded.GetServiceSource(context.Background(), "redis", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "manual" || result.Selected == nil || result.Selected.Path != "redis/logs" {
		t.Fatalf("persisted mapping did not override discovery: %#v", result)
	}
	if err := reloaded.DeleteMapping("redis"); err != nil {
		t.Fatal(err)
	}
	result, err = reloaded.GetServiceSource(context.Background(), "redis", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "unmatched" || result.Selected != nil {
		t.Fatalf("deleting mapping did not restore automatic discovery: %#v", result)
	}
}

func TestMappingAndBrowseRejectTraversalAndSymlink(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	writeLogFile(t, filepath.Join(dataRoot, "safe", "info.log"), "ok\n")
	writeProject(t, projectDir, dataRoot, "services:\n  app:\n    image: example/app\n", "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\n")
	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())

	validation := manager.ValidateMapping("project-data", "../secret")
	if validation.Valid || validation.Error == "" {
		t.Fatalf("traversal validation unexpectedly succeeded: %#v", validation)
	}
	if _, err := manager.BrowseDirectories("project-data", "../secret"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("browse traversal error = %v", err)
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dataRoot, "linked")); err == nil {
		validation = manager.ValidateMapping("project-data", "linked")
		if validation.Valid || !stringsContains(validation.Error, "日志路径") {
			t.Fatalf("symlink validation unexpectedly succeeded: %#v", validation)
		}
	}
}

func TestBrowseIsSingleLevelAndLimited(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	for index := 0; index < 110; index++ {
		if err := os.MkdirAll(filepath.Join(dataRoot, "dir-"+threeDigits(index), "nested"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeProject(t, projectDir, dataRoot, "services:\n  app:\n    image: example/app\n", "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\n")
	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())

	result, err := manager.BrowseDirectories("project-data", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != browsePageSize || !result.Truncated {
		t.Fatalf("unexpected browse limit: %#v", result)
	}
	for _, entry := range result.Entries {
		if filepath.Base(filepath.FromSlash(entry.Path)) == "nested" {
			t.Fatal("browse recursively returned a nested directory")
		}
	}
}

func newTestManager(basePath string, serviceManager *service.ServiceManager) *Manager {
	projectDir := filepath.Dir(basePath)
	if serviceManager != nil {
		projectDir = serviceManager.GetProject().FilePath
		projectDir = filepath.Dir(projectDir)
	}
	return NewManager(fileLogsConfig(basePath, defaultDiscoveryConfig()), projectDir, serviceManager, nil)
}

func newProjectManager(projectDir string, basePath string, discovery config.FileLogDiscoveryConfig) *Manager {
	executor := compose.NewExecutor(projectDir, "auto")
	serviceManager := service.NewServiceManager(projectDir, nil, executor)
	return NewManager(fileLogsConfig(basePath, discovery), projectDir, serviceManager, nil)
}

func fileLogsConfig(basePath string, discovery config.FileLogDiscoveryConfig) config.FileLogsConfig {
	return config.FileLogsConfig{
		Enabled: true,
		AllowedBases: []config.FileLogBaseConfig{{
			ID:   "project-data",
			Name: "Project Data",
			Path: basePath,
		}},
		FollowExtensions:   []string{".log"},
		DownloadExtensions: []string{".log", ".gz"},
		Discovery:          discovery,
	}
}

func defaultDiscoveryConfig() config.FileLogDiscoveryConfig {
	return config.FileLogDiscoveryConfig{
		MaxDepth:        2,
		MaxEntries:      2000,
		TimeoutMS:       300,
		CacheTTLSeconds: 60,
	}
}

func writeProject(t *testing.T, projectDir string, dataRoot string, composeContent string, envContent string) {
	t.Helper()
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		t.Fatal(err)
	}
	writeLogFile(t, filepath.Join(projectDir, "compose.yaml"), composeContent)
	writeLogFile(t, filepath.Join(projectDir, ".env"), envContent)
}

func writeLogFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func twoDigits(value int) string                          { return fmt.Sprintf("%02d", value) }
func threeDigits(value int) string                        { return fmt.Sprintf("%03d", value) }
func stringsContains(value string, substring string) bool { return strings.Contains(value, substring) }
