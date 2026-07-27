package instanceview

import (
	"testing"

	strategy "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestInstanceViewHandlesUntypedRuntimeAndNilParams(t *testing.T) {
	if got := RuntimeFromParams(map[string]any{"runtime": 42}); got != DefaultPluginID {
		t.Fatalf("RuntimeFromParams(untyped) = %q, want %q", got, DefaultPluginID)
	}
	normalized := NormalizeManagedInstance(strategy.ManagedInstance{})
	if normalized.Params == nil {
		t.Fatal("NormalizeManagedInstance left params nil")
	}
}
