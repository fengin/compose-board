package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/fengin/composeboard/internal/compose"
)

func TestOnlineUpgradeOptions_ForceRecreatesWithoutDependencies(t *testing.T) {
	got := onlineUpgradeOptions()
	want := compose.UpOptions{ForceRecreate: true, NoDeps: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("onlineUpgradeOptions() = %+v, want %+v", got, want)
	}
}

func TestLocalUpgradeOptions_RemainsPullNeverWithoutForceRecreate(t *testing.T) {
	got := localUpgradeOptions()
	want := compose.UpOptions{NoDeps: true, PullPolicy: "never"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("localUpgradeOptions() = %+v, want %+v", got, want)
	}
}

type missingImageChecker struct {
	checkedRef string
}

func (c *missingImageChecker) ImageExists(_ context.Context, imageRef string) (bool, error) {
	c.checkedRef = imageRef
	return false, nil
}

func TestApplyLocalUpgrade_RejectsMissingTargetImage(t *testing.T) {
	checker := &missingImageChecker{}
	manager := &ServiceManager{
		project: &compose.ComposeProject{Services: map[string]*compose.DeclaredService{
			"rule-engine": {
				Name:        "rule-engine",
				Image:       "${REGISTRY}/inxvision/rule-engine:${RULE_ENGINE_VERSION}",
				ImageSource: "registry",
			},
		}},
		envVars: map[string]string{
			"REGISTRY":            "192.168.3.48:5000",
			"RULE_ENGINE_VERSION": "0.0.1.20260806.xian",
		},
	}
	upgrade := &UpgradeManager{manager: manager, imageChecker: checker}

	err := upgrade.ApplyLocalUpgrade("rule-engine")
	if err == nil {
		t.Fatal("ApplyLocalUpgrade() error = nil, want missing image error")
	}
	wantRef := "192.168.3.48:5000/inxvision/rule-engine:0.0.1.20260806.xian"
	if checker.checkedRef != wantRef {
		t.Fatalf("checked image = %q, want %q", checker.checkedRef, wantRef)
	}
	if !strings.Contains(err.Error(), "本地未找到目标镜像") {
		t.Fatalf("error = %q, want missing image message", err)
	}
}
