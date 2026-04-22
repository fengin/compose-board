package docker

import (
	"strings"
	"testing"
)

func TestBuildContainerLogsPath_IncludesSince(t *testing.T) {
	path := buildContainerLogsPath(
		"abc123",
		"200",
		true,
		"2026-04-22T08:30:15.123456789Z",
	)

	for _, want := range []string{
		"/containers/abc123/logs?",
		"follow=true",
		"tail=200",
		"timestamps=true",
		"since=2026-04-22T08%3A30%3A15.123456789Z",
	} {
		if !strings.Contains(path, want) {
			t.Fatalf("path = %q, want contains %q", path, want)
		}
	}
}

func TestSelectBestContainer_PrefersRunningThenNewest(t *testing.T) {
	best := selectBestContainer([]dockerContainer{
		{ID: "exited-old", State: "exited", Created: 100},
		{ID: "running-old", State: "running", Created: 90},
		{ID: "running-new", State: "running", Created: 110},
	})

	if best.ID != "running-new" {
		t.Fatalf("best container = %q, want %q", best.ID, "running-new")
	}
}
