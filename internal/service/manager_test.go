package service

import (
	"testing"
	"time"

	"github.com/fengin/composeboard/internal/docker"
)

func TestIsStartupWarning_RunningUnhealthy(t *testing.T) {
	ctr := &docker.ContainerInfo{
		Status: "running",
		Health: "unhealthy",
	}
	if !isStartupWarning(ctr) {
		t.Fatal("expected unhealthy running service to be marked as startup warning")
	}
}

func TestIsStartupWarning_CreatedTooLong(t *testing.T) {
	ctr := &docker.ContainerInfo{
		Status:  "created",
		State:   "Created",
		Created: time.Now().Add(-time.Minute).Unix(),
	}
	if !isStartupWarning(ctr) {
		t.Fatal("expected long-lived created service to be marked as startup warning")
	}
}

func TestIsStartupWarning_RestartingPastThreshold(t *testing.T) {
	ctr := &docker.ContainerInfo{
		Status: "restarting",
		State:  "Restarting (1) 40 seconds ago",
	}
	if !isStartupWarning(ctr) {
		t.Fatal("expected prolonged restarting service to be marked as startup warning")
	}
}

func TestIsStartupWarning_RestartingShortWindowNoWarning(t *testing.T) {
	ctr := &docker.ContainerInfo{
		Status: "restarting",
		State:  "Restarting (1) 10 seconds ago",
	}
	if isStartupWarning(ctr) {
		t.Fatal("expected short restarting window to remain non-warning")
	}
}
