package api

import (
	"strings"
	"testing"
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
