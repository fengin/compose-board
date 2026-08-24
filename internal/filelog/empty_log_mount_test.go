package filelog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLogSemanticMountCanBeSelectedBeforeFileCreated(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	if err := os.MkdirAll(filepath.Join(dataRoot, "emqx", "log"), 0755); err != nil {
		t.Fatal(err)
	}
	dataLogDir := filepath.Join(dataRoot, "emqx", "data", "mnesia", "node")
	if err := os.MkdirAll(dataLogDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataLogDir, "node.log"), []byte("low confidence candidate\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeProject(t, projectDir, dataRoot, `
services:
  emqx:
    image: emqx/emqx:latest
    volumes:
      - ${EMQX_LOG_DIR}:/opt/emqx/log
      - ${EMQX_DATA_DIR}:/opt/emqx/data
`, "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\nEMQX_LOG_DIR=${DATA_ROOT}/emqx/log\nEMQX_DATA_DIR=${DATA_ROOT}/emqx/data\n")

	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	result, err := manager.GetServiceSource(context.Background(), "emqx", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "automatic" || result.Selected == nil || result.Selected.Path != "emqx/log" {
		t.Fatalf("empty semantic log mount was not selected: %#v", result)
	}
}
