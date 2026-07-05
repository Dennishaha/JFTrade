package servercore

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy(t *testing.T) {
	for _, input := range []string{"", pineworker.RuntimeID, pineworker.LegacyRuntimeID, " Pine-Go-Plan "} {
		got, err := normalizeStrategyRuntime(input)
		if err != nil {
			t.Fatalf("normalizeStrategyRuntime(%q) error = %v", input, err)
		}
		if got != pineworker.RuntimeID {
			t.Fatalf("normalizeStrategyRuntime(%q) = %q, want %q", input, got, pineworker.RuntimeID)
		}
	}
	if _, err := normalizeStrategyRuntime("legacy-runtime"); err == nil {
		t.Fatal("normalizeStrategyRuntime(legacy-runtime) error = nil, want error")
	}
}

func TestStrategyRuntimeFromParamsMigratesLegacyRuntime(t *testing.T) {
	for _, input := range []string{"", pineworker.RuntimeID, pineworker.LegacyRuntimeID} {
		params := map[string]any{"runtime": input}
		if input == "" {
			params = map[string]any{}
		}
		if got := strategyRuntimeFromParams(params); got != pineworker.RuntimeID {
			t.Fatalf("strategyRuntimeFromParams(%q) = %q, want %q", input, got, pineworker.RuntimeID)
		}
	}
}

func TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime(t *testing.T) {
	store := &strategyCatalogStore{}
	normalized := store.normalizeStrategy(managedStrategyInstance{
		Params: map[string]any{"runtime": pineworker.LegacyRuntimeID},
	})
	if got := normalized.Params["runtime"]; got != pineworker.RuntimeID {
		t.Fatalf("normalized runtime = %#v, want %q", got, pineworker.RuntimeID)
	}
	if !strategyInstanceStartable(normalized) {
		t.Fatal("legacy runtime migrated to PineTS should be startable")
	}
}
