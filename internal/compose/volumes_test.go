package compose

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadServiceVolumesSupportsShortAndLongSyntax(t *testing.T) {
	dir := t.TempDir()
	content := `
services:
  app:
    image: example/app:latest
    volumes:
      - ${APP_LOGS_DIR}:/data/logs
      - C:/host/logs:/windows/logs:ro
      - type: bind
        source: ./local-logs
        target: /var/log/app
        read_only: true
`
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadServiceVolumes(dir, "app")
	if err != nil {
		t.Fatalf("ReadServiceVolumes() error = %v", err)
	}
	want := []VolumeMount{
		{Type: "bind", Source: "${APP_LOGS_DIR}", Target: "/data/logs"},
		{Type: "bind", Source: "C:/host/logs", Target: "/windows/logs", ReadOnly: true},
		{Type: "bind", Source: "./local-logs", Target: "/var/log/app", ReadOnly: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("volumes = %#v, want %#v", got, want)
	}
}

func TestParseShortVolumeMountSupportsNamedAndAnonymousVolumes(t *testing.T) {
	named, ok := parseShortVolumeMount("app-logs:/var/log/app")
	if !ok || named.Type != "volume" || named.Source != "app-logs" || named.Target != "/var/log/app" {
		t.Fatalf("unexpected named volume: %#v", named)
	}
	anonymous, ok := parseShortVolumeMount("/var/lib/app")
	if !ok || anonymous.Source != "" || anonymous.Target != "/var/lib/app" {
		t.Fatalf("unexpected anonymous volume: %#v", anonymous)
	}
}
