package filelog

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadTailLinesReadsLastLinesWithoutWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeFile(t, path, "one\ntwo\nthree\nfour\n")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	lines, offset, err := ReadTailLines(file, 2)
	if err != nil {
		t.Fatalf("ReadTailLines() error = %v", err)
	}
	if !reflect.DeepEqual(lines, []string{"three", "four"}) {
		t.Fatalf("lines = %v", lines)
	}
	if offset != int64(len("one\ntwo\nthree\nfour\n")) {
		t.Fatalf("offset = %d", offset)
	}
}

func TestFollowReadsAppendAndContinuesAfterRotation(t *testing.T) {
	rootPath := t.TempDir()
	logPath := filepath.Join(rootPath, "info.log")
	writeFile(t, logPath, "initial\n")
	manager := newTestManager(rootPath, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	events := make(chan StreamEvent, 64)
	errorsCh := make(chan error, 1)
	go func() {
		errorsCh <- manager.Follow(ctx, "project-data", "info.log", 10, func(event StreamEvent) error {
			events <- event
			return nil
		})
	}()

	waitForLine(t, events, "initial")
	appendFile(t, logPath, "appended\n")
	waitForLine(t, events, "appended")

	rotatedPath := filepath.Join(rootPath, "info.log.1")
	if err := os.Rename(logPath, rotatedPath); err != nil {
		t.Fatal(err)
	}
	writeFile(t, logPath, "after-rotation\n")
	waitForLine(t, events, "after-rotation")

	cancel()
	select {
	case <-errorsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Follow did not stop after context cancellation")
	}
}

func appendFile(t *testing.T, path string, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func waitForLine(t *testing.T, events <-chan StreamEvent, line string) {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == "line" && event.Line == line {
				return
			}
		case <-timer.C:
			t.Fatalf("did not receive log line %q", line)
		}
	}
}
