package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

func TestClientImageExists_UsesReferenceFilter(t *testing.T) {
	const imageRef = "192.168.3.48:5000/inxvision/rule-engine:0.0.1.20260806.xian"
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		filters := req.URL.Query().Get("filters")
		if !strings.Contains(filters, imageRef) {
			t.Fatalf("filters = %q, want image ref %q", filters, imageRef)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[{"Id":"sha256:test"}]`)),
			Header:     make(http.Header),
		}, nil
	})}}

	exists, err := client.ImageExists(context.Background(), imageRef)
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Fatal("ImageExists() = false, want true")
	}
}

func TestClientImageExists_ReturnsFalseForEmptyResult(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[]`)),
			Header:     make(http.Header),
		}, nil
	})}}

	exists, err := client.ImageExists(context.Background(), "demo/app:1.0.0")
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if exists {
		t.Fatal("ImageExists() = true, want false")
	}
}
