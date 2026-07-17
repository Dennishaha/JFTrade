package servercore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverage98StrategyDesignPersistencePathAndSerializationContracts(t *testing.T) {
	if got := deriveStrategyDesignPath("settings.json"); got != defaultStrategyDesignFilename {
		t.Fatalf("deriveStrategyDesignPath relative = %q, want %q", got, defaultStrategyDesignFilename)
	}
	settingsPath := filepath.Join(t.TempDir(), "config", "settings.json")
	if got, want := deriveStrategyDesignPath(settingsPath), filepath.Join(filepath.Dir(settingsPath), defaultStrategyDesignFilename); got != want {
		t.Fatalf("deriveStrategyDesignPath nested = %q, want %q", got, want)
	}

	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "/tmp/coverage98-runtime.db")
	if got := deriveStrategyDesignDBPath(settingsPath); got != "/tmp/coverage98-runtime.db" {
		t.Fatalf("deriveStrategyDesignDBPath env override = %q", got)
	}
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")
	if got := deriveStrategyDesignDBPath("legacy.json"); got != defaultStrategyRuntimeDBFilename {
		t.Fatalf("deriveStrategyDesignDBPath relative = %q, want %q", got, defaultStrategyRuntimeDBFilename)
	}

	if err := (&strategyDesignStore{}).openDB(); err == nil || !strings.Contains(err.Error(), "db path is required") {
		t.Fatalf("openDB without path = %v, want path requirement", err)
	}
	store := &strategyDesignStore{dbPath: filepath.Join(t.TempDir(), "nested", "strategy-runtime.db")}
	if err := store.openDB(); err != nil {
		t.Fatalf("openDB nested path: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close strategy design store: %v", err)
		}
	})

	if _, err := strategyDesignDefinitionFromRow(strategyDesignDefinitionRow{VisualModelJSON: "{"}); err == nil {
		t.Fatal("strategyDesignDefinitionFromRow accepted malformed visual model JSON")
	}
	_, err := strategyDesignDefinitionRowFromDefinition(strategyDesignDefinition{
		VisualModel: &strategyVisualModel{Nodes: []strategyVisualNode{{
			ID: "node", Type: "indicator", Properties: map[string]any{"unsupported": func() {}},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("strategyDesignDefinitionRowFromDefinition marshal error = %v", err)
	}
}
