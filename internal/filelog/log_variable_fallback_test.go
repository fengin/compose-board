package filelog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReferencedGenericLogVariableProvidesSafeFallbackCandidate(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	if err := os.MkdirAll(filepath.Join(dataRoot, "middleware", "log"), 0755); err != nil {
		t.Fatal(err)
	}
	writeProject(t, projectDir, dataRoot, `
services:
  middleware:
    image: example/middleware
    environment:
      - LOG_HOME=${GENERIC_LOG_DIR}
`, "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\nGENERIC_LOG_DIR=${DATA_ROOT}/middleware/log\n")

	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	result, err := manager.GetServiceSource(context.Background(), "middleware", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "automatic" || result.Selected == nil || result.Selected.Path != "middleware/log" {
		t.Fatalf("referenced generic log variable was not used: %#v", result)
	}
}
