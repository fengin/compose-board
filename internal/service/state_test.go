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

func TestStateManager_BackfillMissingComposeHashes_DetectsNextComposeChange(t *testing.T) {
	dir := t.TempDir()

	initialComposeYAML := `services:
  kafka:
    image: "demo/kafka:${KAFKA_VERSION}"
    environment:
      - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
    ports:
      - "${KAFKA_PORT}:9092"
`
	changedComposeYAML := `services:
  kafka:
    image: "demo/kafka:${KAFKA_VERSION}"
    environment:
      - KAFKA_CFG_LISTENERS=INTERNAL://:9092,EXTERNAL://:19092,CONTROLLER://:9093
      - KAFKA_CFG_INTER_BROKER_LISTENER_NAME=INTERNAL
    ports:
      - "${KAFKA_PORT}:9092"
      - "19092:19092"
`
	envContent := "KAFKA_VERSION=4.0\nKAFKA_PORT=9092\n"

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), initialComposeYAML)
	writeTestFile(t, filepath.Join(dir, ".env"), envContent)

	manager := NewServiceManager(dir, nil, compose.NewExecutor(dir, "docker-compose"))
	stateM := NewStateManager(dir, manager)
	stateM.EnsureState()

	stateM.mu.Lock()
	state, err := stateM.loadStateLocked()
	if err != nil {
		stateM.mu.Unlock()
		t.Fatalf("load state: %v", err)
	}
	entry := state.Services["kafka"]
	entry.ComposeHash = ""
	state.Services["kafka"] = entry
	if err := stateM.writeStateLocked(state); err != nil {
		stateM.mu.Unlock()
		t.Fatalf("write legacy state: %v", err)
	}
	stateM.mu.Unlock()

	if err := stateM.BackfillMissingComposeHashes(); err != nil {
		t.Fatalf("backfill compose hashes: %v", err)
	}
	if pending := stateM.GetPendingConfigChanges(); len(pending) != 0 {
		t.Fatalf("backfill should not mark unchanged compose as pending, got %v", pending)
	}

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), changedComposeYAML)
	manager.ReloadCompose()

	pending := stateM.GetPendingConfigChanges()
	if !pending["kafka"] {
		t.Fatalf("expected kafka config change after compose save, got %v", pending)
	}
}

func TestStateManager_BackfillMissingComposeHashes_FillsMissingVolumeEnvBaseline(t *testing.T) {
	dir := t.TempDir()

	composeYAML := `services:
  emqx:
    image: "demo/emqx:5.10"
    ports:
      - "${EMQX_MQTT_PORT}:1883"
    volumes:
      - "${EMQX_DATA_DIR}:/opt/emqx/data"
      - "${EMQX_LOG_DIR}:/opt/emqx/log"
`
	envContent := "EMQX_MQTT_PORT=1883\nEMQX_DATA_DIR=/data/emqx/data\nEMQX_LOG_DIR=/data/emqx/log\n"

	writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), composeYAML)
	writeTestFile(t, filepath.Join(dir, ".env"), envContent)

	manager := NewServiceManager(dir, nil, compose.NewExecutor(dir, "docker-compose"))
	stateM := NewStateManager(dir, manager)
	stateM.EnsureState()

	stateM.mu.Lock()
	state, err := stateM.loadStateLocked()
	if err != nil {
		stateM.mu.Unlock()
		t.Fatalf("load state: %v", err)
	}
	entry := state.Services["emqx"]
	entry.Env = map[string]string{"EMQX_MQTT_PORT": "1883"}
	entry.ComposeHash = ""
	state.Services["emqx"] = entry
	if err := stateM.writeStateLocked(state); err != nil {
		stateM.mu.Unlock()
		t.Fatalf("write legacy state: %v", err)
	}
	stateM.mu.Unlock()

	if err := stateM.BackfillMissingComposeHashes(); err != nil {
		t.Fatalf("backfill compose hashes: %v", err)
	}

	if pending := stateM.GetPendingEnvChanges(); len(pending) != 0 {
		t.Fatalf("backfilled volume env vars should not produce pending env, got %v", pending)
	}

	stateM.mu.Lock()
	state, err = stateM.loadStateLocked()
	stateM.mu.Unlock()
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	got := state.Services["emqx"].Env
	if got["EMQX_DATA_DIR"] != "/data/emqx/data" || got["EMQX_LOG_DIR"] != "/data/emqx/log" {
		t.Fatalf("expected missing volume env vars to be backfilled, got %v", got)
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
