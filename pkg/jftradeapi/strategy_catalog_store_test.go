package jftradeapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestNormalizeStrategyDoesNotShareReferenceFields(t *testing.T) {
	store := &strategyCatalogStore{}
	input := managedStrategyInstance{
		ID:       "instance-1",
		PluginID: IDDSLPlanPlugin(),
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"definitionId": "mean-revert",
		},
		Logs: []string{"started"},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "instance-1",
			Kind:       "started",
			Detail:     "mean-revert",
			At:         "2026-05-23T17:54:38Z",
		}},
	}

	normalized := store.normalizeStrategy(input)

	if _, ok := input.Params["runtime"]; ok {
		t.Fatal("normalizeStrategy mutated input params")
	}
	if got, ok := normalized.Params["runtime"].(string); !ok || got == "" {
		t.Fatalf("normalized runtime = %#v, want non-empty string", normalized.Params["runtime"])
	}

	normalized.Params["definitionId"] = "changed"
	normalized.Logs[0] = "mutated"
	normalized.AuditEntries[0].Kind = "mutated"

	if got := input.Params["definitionId"]; got != "mean-revert" {
		t.Fatalf("input params shared with normalized copy: %#v", got)
	}
	if got := input.Logs[0]; got != "started" {
		t.Fatalf("input logs shared with normalized copy: %q", got)
	}
	if got := input.AuditEntries[0].Kind; got != "started" {
		t.Fatalf("input audit entries shared with normalized copy: %q", got)
	}
}

func TestStrategyCatalogStoreLoadPersistsRemovedRuntimeMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	legacy := `{
	  "targetDir": "plugins",
	  "strategies": [
	    {
	      "id": "legacy-instance-1",
	      "pluginId": "removed-script-runtime",
	      "definition": {
	        "strategyId": "removed-runtime-strategy",
	        "name": "Removed Runtime Strategy",
	        "version": "0.1.0"
	      },
	      "params": {
	        "runtime": "removed-script-runtime",
	        "sourceFormat": "removed-script-source",
	        "script": "function onInit(ctx) { console.log(ctx.symbol); }"
	      },
	      "status": "STOPPED",
	      "createdAt": "2026-05-26T00:00:00Z"
	    }
	  ]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	store, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	items := store.strategies()
	if len(items) != 1 {
		t.Fatalf("expected one strategy, got %d", len(items))
	}
	if items[0].PluginID != IDDSLPlanPlugin() || items[0].Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("expected DSL strategy item, got %+v", items[0])
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated catalog: %v", err)
	}
	persistedText := string(persisted)
	if strings.Contains(persistedText, "function onInit") {
		t.Fatalf("expected removed runtime script to be replaced, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `"pluginId": "dsl-go-plan"`) {
		t.Fatalf("expected persisted DSL plugin id, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `"sourceFormat": "dsl-v1"`) {
		t.Fatalf("expected persisted DSL source format, got %s", persistedText)
	}
}

func TestNormalizeStrategyMigratesRemovedRuntimeInstanceToDSL(t *testing.T) {
	store := &strategyCatalogStore{}
	input := managedStrategyInstance{
		ID:       "legacy-instance-1",
		PluginID: "removed-script-runtime",
		Definition: strategyDefinitionSummary{
			StrategyID: "removed-runtime-strategy",
			Name:       "Removed Runtime Strategy",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"runtime":      "removed-script-runtime",
			"sourceFormat": "removed-script-source",
			"script":       "function onInit(ctx) { console.log(ctx.symbol); }",
		},
	}

	normalized := store.normalizeStrategy(input)

	if normalized.PluginID != IDDSLPlanPlugin() {
		t.Fatalf("expected DSL plugin, got %q", normalized.PluginID)
	}
	if got := strategyRuntimeFromParams(normalized.Params); got != strategyRuntimeDSLPlan {
		t.Fatalf("expected DSL runtime, got %q", got)
	}
	if got := strategySourceFormatFromParams(normalized.Params); got != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("expected DSL source format, got %q", got)
	}
	script, _ := normalized.Params["script"].(string)
	if strings.Contains(script, "function onInit") {
		t.Fatalf("expected removed runtime script to be replaced, got %q", script)
	}
	if !strings.Contains(script, "strategy Removed Runtime Strategy") {
		t.Fatalf("expected DSL skeleton, got %q", script)
	}
}
