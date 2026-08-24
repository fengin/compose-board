package filelog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMappingFileCanBeCopiedToProjectWithDifferentAbsoluteBase(t *testing.T) {
	projectA := t.TempDir()
	baseA := filepath.Join(projectA, "customer-a")
	writeLogFile(t, filepath.Join(baseA, "worker", "logs", "worker.log"), "a\n")
	writeProject(t, projectA, baseA, "services:\n  worker:\n    image: example/worker\n", "DATA_ROOT="+filepath.ToSlash(baseA)+"\n")
	managerA := newProjectManager(projectA, baseA, defaultDiscoveryConfig())
	if _, err := managerA.SaveMapping("worker", []MappingDirectory{{
		ID: "default", BaseID: "project-data", RelativePath: "worker/logs",
	}}); err != nil {
		t.Fatal(err)
	}

	projectB := t.TempDir()
	baseB := filepath.Join(projectB, "another-root")
	writeLogFile(t, filepath.Join(baseB, "worker", "logs", "worker.log"), "b\n")
	writeProject(t, projectB, baseB, "services:\n  worker:\n    image: example/worker\n", "DATA_ROOT="+filepath.ToSlash(baseB)+"\n")
	mappingData, err := os.ReadFile(filepath.Join(projectA, mappingFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectB, mappingFileName), mappingData, 0600); err != nil {
		t.Fatal(err)
	}

	managerB := newProjectManager(projectB, baseB, defaultDiscoveryConfig())
	result, err := managerB.GetServiceSource(context.Background(), "worker", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "manual" || result.Selected == nil || result.Selected.Path != "worker/logs" {
		t.Fatalf("copied relative mapping was not reusable: %#v", result)
	}
	if result.Selected.BaseID != "project-data" {
		t.Fatalf("unexpected copied base id: %#v", result.Selected)
	}
}
