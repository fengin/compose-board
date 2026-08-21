package compose

import (
	"reflect"
	"testing"
)

func TestBuildUpArgs_LocalPullPolicyForComposeV2(t *testing.T) {
	executor := &Executor{detected: "docker compose"}
	got := executor.buildUpArgs([]string{"rule-engine"}, UpOptions{
		NoDeps:     true,
		PullPolicy: "never",
	})
	want := []string{"up", "-d", "--pull", "never", "--no-deps", "rule-engine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildUpArgs() = %v, want %v", got, want)
	}
}

func TestBuildUpArgs_LocalPullPolicyKeepsComposeV1Compatible(t *testing.T) {
	executor := &Executor{detected: "docker-compose"}
	got := executor.buildUpArgs([]string{"rule-engine"}, UpOptions{
		NoDeps:     true,
		PullPolicy: "never",
	})
	want := []string{"up", "-d", "--no-deps", "rule-engine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildUpArgs() = %v, want %v", got, want)
	}
}

func TestBuildUpArgs_OnlineUpgradeForceRecreatesForComposeV2(t *testing.T) {
	executor := &Executor{detected: "docker compose"}
	got := executor.buildUpArgs([]string{"rule-engine"}, UpOptions{
		ForceRecreate: true,
		NoDeps:        true,
	})
	want := []string{"up", "-d", "--force-recreate", "--no-deps", "rule-engine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildUpArgs() = %v, want %v", got, want)
	}
}

func TestBuildUpArgs_OnlineUpgradeForceRecreatesForComposeV1(t *testing.T) {
	executor := &Executor{detected: "docker-compose"}
	got := executor.buildUpArgs([]string{"rule-engine"}, UpOptions{
		ForceRecreate: true,
		NoDeps:        true,
	})
	want := []string{"up", "-d", "--force-recreate", "--no-deps", "rule-engine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildUpArgs() = %v, want %v", got, want)
	}
}
