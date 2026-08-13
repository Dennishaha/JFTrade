package persistence

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"
)

func TestNormalizeSessionComposerStateClearsInvalidModes(t *testing.T) {
	normalized := NormalizeSessionComposerState("session", assistantmodel.SessionComposerState{
		WorkModeOverride:       "bad-mode",
		PermissionModeOverride: "bad-permission",
	})
	if normalized.WorkModeOverride != "" || normalized.PermissionModeOverride != "" {
		t.Fatalf("NormalizeSessionComposerState invalid modes = %+v, want cleared", normalized)
	}
}
