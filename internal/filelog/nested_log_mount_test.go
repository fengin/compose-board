package filelog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNestedLogMountDoesNotCompeteWithServiceRoot(t *testing.T) {
	projectDir := t.TempDir()
	dataRoot := filepath.Join(projectDir, "data")
	serviceRoot := filepath.Join(dataRoot, "inxvision", "logs", "inxaiot-starter-platform")
	nacosRoot := filepath.Join(serviceRoot, "nacos")
	if err := os.MkdirAll(nacosRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceRoot, "platform.log"), []byte("platform log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nacosRoot, "config.log"), []byte("nacos log\n"), 0644); err != nil {
		t.Fatal(err)
	}

	writeProject(t, projectDir, dataRoot, `
services:
  inxvision-starter-platform:
    image: registry.example/inxaiot-starter-platform:latest
    volumes:
      - ${APP_LOGS_DIR}:/data/logs
      - ${APP_LOGS_DIR}/inxaiot-starter-platform/nacos:/root/logs/nacos
`, "DATA_ROOT="+filepath.ToSlash(dataRoot)+"\nAPP_LOGS_DIR=${DATA_ROOT}/inxvision/logs\n")

	manager := newProjectManager(projectDir, dataRoot, defaultDiscoveryConfig())
	result, err := manager.GetServiceSource(context.Background(), "inxvision-starter-platform", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "automatic" || result.Selected == nil {
		t.Fatalf("nested log mount should remain an automatic source: %#v", result)
	}
	if result.Selected.Path != "inxvision/logs/inxaiot-starter-platform" {
		t.Fatalf("selected path = %q", result.Selected.Path)
	}
	foundNacos := false
	for _, directory := range result.Directories {
		if directory.Path == "inxvision/logs/inxaiot-starter-platform/nacos" {
			foundNacos = true
			break
		}
	}
	if !foundNacos {
		t.Fatalf("nested Nacos directory was not retained: %#v", result.Directories)
	}
}
