package futuapp

import (
	"context"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestCoordinatorDisabledProbeAndSettingsBoundaries(t *testing.T) {
	coordinator := New(Options{Settings: coordinatorTestSettings{}})
	if probe := coordinator.Probe(context.Background()); probe.Connectivity != "" || probe.LastError != nil {
		t.Fatalf("disabled probe = %#v, want empty probe", probe)
	}
	if got := coordinator.OnboardingStateFromSettings(context.Background(), jfsettings.OnboardingSettings{}); got == nil {
		t.Fatal("disabled onboarding state = nil")
	}
}
