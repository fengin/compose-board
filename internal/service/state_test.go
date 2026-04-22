// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fengin/composeboard/internal/compose"
)

func TestStateManager_GetPendingEnvChanges_IncludesNonImageServiceVars(t *testing.T) {
	dir := t.TempDir()

	composeYAML := `services:
  app:
    image: "demo/app:${APP_VERSION}"
    ports:
      - "${HOST_IP}:${APP_PORT}:8080"
    environment:
      APP_PORT: "${APP_PORT}"
      HOST_IP: "${HOST_IP}"
`

	initialEnv := "APP_VERSION=1.0.0\nHOST_IP=127.0.0.1\nAPP_PORT=8081\n"
	changedEnv := "APP_VERSION=1.0.0\nHOST_IP=192.168.3.44\nAPP_PORT=8082\n"

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), composeYAML)
	writeTestFile(t, filepath.Join(dir, ".env"), initialEnv)

	manager := NewServiceManager(dir, nil, compose.NewExecutor(dir, "docker-compose"))
	stateM := NewStateManager(dir, manager)
	stateM.EnsureState()

	writeTestFile(t, filepath.Join(dir, ".env"), changedEnv)
	manager.ReloadCompose()

	pending := stateM.GetPendingEnvChanges()
	if pending == nil {
		t.Fatalf("expected pending env changes, got nil")
	}

	got := sliceToSet(pending["app"])
	if len(got) != 2 {
		t.Fatalf("expected 2 pending vars, got %v", pending["app"])
	}
	if _, ok := got["HOST_IP"]; !ok {
		t.Fatalf("expected HOST_IP to be pending, got %v", pending["app"])
	}
	if _, ok := got["APP_PORT"]; !ok {
		t.Fatalf("expected APP_PORT to be pending, got %v", pending["app"])
	}
	if _, ok := got["APP_VERSION"]; ok {
		t.Fatalf("image-only APP_VERSION should not appear in pending env, got %v", pending["app"])
	}
}

func TestStateManager_GetPendingEnvChanges_IgnoresImageOnlyVars(t *testing.T) {
	dir := t.TempDir()

	composeYAML := `services:
  app:
    image: "demo/app:${APP_VERSION}"
`

	initialEnv := "APP_VERSION=1.0.0\n"
	changedEnv := "APP_VERSION=1.0.1\n"

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), composeYAML)
	writeTestFile(t, filepath.Join(dir, ".env"), initialEnv)

	manager := NewServiceManager(dir, nil, compose.NewExecutor(dir, "docker-compose"))
	stateM := NewStateManager(dir, manager)
	stateM.EnsureState()

	writeTestFile(t, filepath.Join(dir, ".env"), changedEnv)
	manager.ReloadCompose()

	pending := stateM.GetPendingEnvChanges()
	if len(pending) != 0 {
		t.Fatalf("image-only var change should not produce pending env, got %v", pending)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func sliceToSet(items []string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
}
