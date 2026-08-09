package persistence

import (
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestNormalizeSessionComposerStateClearsInvalidModes(t *testing.T) {
	normalized := NormalizeSessionComposerState("session", jfadkmodel.SessionComposerState{
		WorkModeOverride:       "bad-mode",
		PermissionModeOverride: "bad-permission",
	})
	if normalized.WorkModeOverride != "" || normalized.PermissionModeOverride != "" {
		t.Fatalf("NormalizeSessionComposerState invalid modes = %+v, want cleared", normalized)
	}
}
