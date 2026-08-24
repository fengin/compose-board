package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadWithoutFileLogsKeepsFeatureDisabled(t *testing.T) {
	path := writeTestConfig(t, `
server:
  port: 9090
project:
  dir: C:/compose/project
auth:
  username: admin
  password: test-password
  jwt_secret: test-secret
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FileLogs.Enabled {
		t.Fatal("file logs should remain disabled when configuration is omitted")
	}
	if len(cfg.FileLogs.AllowedBases) != 0 {
		t.Fatalf("file log bases = %v, want empty", cfg.FileLogs.AllowedBases)
	}
}

func TestLoadFileLogsAppliesDefaults(t *testing.T) {
	path := writeTestConfig(t, `
project:
  dir: C:/compose/project
auth:
  username: admin
  password: test-password
  jwt_secret: test-secret
file_logs:
  enabled: true
  allowed_bases:
    - id: project-data
      name: Project Data
      path: C:/data
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.FileLogs.Enabled || len(cfg.FileLogs.AllowedBases) != 1 {
		t.Fatalf("unexpected file logs config: %+v", cfg.FileLogs)
	}
	if !reflect.DeepEqual(cfg.FileLogs.FollowExtensions, []string{".log"}) {
		t.Fatalf("follow extensions = %v", cfg.FileLogs.FollowExtensions)
	}
	if !reflect.DeepEqual(cfg.FileLogs.DownloadExtensions, []string{".log", ".gz"}) {
		t.Fatalf("download extensions = %v", cfg.FileLogs.DownloadExtensions)
	}
	if cfg.FileLogs.Discovery.MaxDepth != 2 || cfg.FileLogs.Discovery.MaxEntries != 2000 ||
		cfg.FileLogs.Discovery.TimeoutMS != 300 || cfg.FileLogs.Discovery.CacheTTLSeconds != 60 {
		t.Fatalf("unexpected discovery defaults: %+v", cfg.FileLogs.Discovery)
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
