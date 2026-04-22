package api

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fengin/composeboard/internal/docker"
)

func TestReadLogLines_SupportsLongLogLine(t *testing.T) {
	longLine := "2026-04-22T08:30:15.123456789Z " + strings.Repeat("x", 96*1024)

	lines, err := readLogLines(strings.NewReader(longLine + "\n"))
	if err != nil {
		t.Fatalf("readLogLines returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("len(lines) = %d, want 1", len(lines))
	}
	if lines[0] != longLine {
		t.Fatalf("unexpected log line length = %d, want %d", len(lines[0]), len(longLine))
	}
}

func TestExtractLogTimestamp(t *testing.T) {
	line := "2026-04-22T08:30:15.123456789Z application started"

	got := extractLogTimestamp(line)
	want := "2026-04-22T08:30:15.123456789Z"
	if got != want {
		t.Fatalf("extractLogTimestamp() = %q, want %q", got, want)
	}
}

func TestExtractLogSinceValue(t *testing.T) {
	line := "2026-04-22T08:30:15.123456789Z application started"

	got := extractLogSinceValue(line)
	want := strconv.FormatInt(time.Date(2026, 4, 22, 8, 30, 15, 123456789, time.UTC).Unix(), 10)
	if got != want {
		t.Fatalf("extractLogSinceValue() = %q, want %q", got, want)
	}
}

func TestIsAttachableLogStatus(t *testing.T) {
	cases := map[string]bool{
		"running":    true,
		"restarting": true,
		"exited":     false,
		"created":    false,
	}
	for status, want := range cases {
		if got := isAttachableLogStatus(status); got != want {
			t.Fatalf("isAttachableLogStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestServiceLogStreamerNextCursor_ResetsWhenContainerChanges(t *testing.T) {
	streamer := &serviceLogStreamer{
		initialTail: "100",
	}

	tail, since := streamer.nextCursor("old123")
	if tail != "100" || since != "" {
		t.Fatalf("first cursor = (%q,%q), want (100,\"\")", tail, since)
	}

	streamer.lastTimestamp = "1713862457"
	streamer.lastLine = "old line"

	tail, since = streamer.nextCursor("old123")
	if tail != "0" || since != "1713862457" {
		t.Fatalf("same container cursor = (%q,%q), want (0,1713862457)", tail, since)
	}

	tail, since = streamer.nextCursor("new456")
	if tail != "100" || since != "" {
		t.Fatalf("new container cursor = (%q,%q), want (100,\"\")", tail, since)
	}
	if streamer.lastTimestamp != "" || streamer.lastLine != "" {
		t.Fatalf("expected cursor state reset on container change, got timestamp=%q line=%q", streamer.lastTimestamp, streamer.lastLine)
	}
}

func TestShouldRotateLogSourceState(t *testing.T) {
	statusRunning := &docker.ContainerStatus{Status: "running"}
	statusExited := &docker.ContainerStatus{Status: "exited"}

	tests := []struct {
		name       string
		status     *docker.ContainerStatus
		currentID  string
		attachedID string
		err        error
		wantRotate bool
		wantErr    bool
	}{
		{
			name:       "same running container keeps stream",
			status:     statusRunning,
			currentID:  "abc123",
			attachedID: "abc123",
			wantRotate: false,
		},
		{
			name:       "container replaced rotates stream",
			status:     statusRunning,
			currentID:  "def456",
			attachedID: "abc123",
			wantRotate: true,
		},
		{
			name:       "service stopped rotates stream",
			status:     statusExited,
			currentID:  "abc123",
			attachedID: "abc123",
			wantRotate: true,
		},
		{
			name:       "container missing rotates stream",
			currentID:  "",
			attachedID: "abc123",
			err:        docker.ErrNotFound,
			wantRotate: true,
		},
		{
			name:       "transient docker error keeps current stream",
			currentID:  "",
			attachedID: "abc123",
			err:        errors.New("temporary failure"),
			wantRotate: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRotate, err := shouldRotateLogSourceState(tt.status, tt.currentID, tt.attachedID, tt.err)
			if gotRotate != tt.wantRotate {
				t.Fatalf("rotate = %v, want %v", gotRotate, tt.wantRotate)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
