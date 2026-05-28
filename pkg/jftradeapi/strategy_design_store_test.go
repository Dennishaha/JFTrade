package jftradeapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyDesignStoreLoadMigratesLegacyMovingAverageDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-definitions.json")
	legacy := `{
	  "definitions": [
	    {
	      "id": "legacy-ma-strategy",
	      "name": "Legacy MA",
	      "version": "0.1.0",
	      "description": "legacy builder payload",
	      "runtime": "legacy-runtime",
	      "sourceFormat": "legacy-v0",
	      "symbol": "00700",
	      "interval": "1m",
	      "script": "",
	      "visualModel": {
	        "engine": "logic-flow",
	        "version": 1,
	        "nodes": [
	          {
	            "id": "indicator-fast",
	            "type": "rect",
	            "x": 120,
	            "y": 160,
	            "text": "获取 均线 EMA 5",
	            "properties": {
	              "blockKind": "getTechnicalIndicator",
	              "indicatorType": "movingAverage",
	              "movingAverageType": "EMA",
	              "windowSize": 5
	            }
	          },
	          {
	            "id": "indicator-slow",
	            "type": "rect",
	            "x": 280,
	            "y": 160,
	            "text": "获取 均线 MA 20",
	            "properties": {
	              "blockKind": "getTechnicalIndicator",
	              "indicatorType": "movingAverage",
	              "movingAverageType": "MA",
	              "windowSize": 20
	            }
	          }
	        ],
	        "edges": []
	      },
	      "createdAt": "2026-05-26T00:00:00Z",
	      "updatedAt": "2026-05-26T00:00:00Z"
	    }
	  ]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy definitions: %v", err)
	}

	store, err := NewStrategyDesignStore(path)
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}

	definition, ok := store.definition("legacy-ma-strategy")
	if !ok {
		t.Fatal("expected migrated definition to exist")
	}
	if !strings.Contains(definition.Script, `strategy Legacy MA`) {
		t.Fatalf("expected empty legacy script to fall back to the default DSL skeleton, got %q", definition.Script)
	}
	if definition.SourceFormat != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("expected strategy source format to normalize to DSL, got %q", definition.SourceFormat)
	}
	if definition.Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("expected strategy runtime to normalize to DSL plan, got %q", definition.Runtime)
	}
	if definition.VisualModel == nil || len(definition.VisualModel.Nodes) != 2 {
		t.Fatalf("expected migrated visual model nodes, got %+v", definition.VisualModel)
	}
	for _, node := range definition.VisualModel.Nodes {
		if got := node.Properties["periodUnit"]; got != "day" {
			t.Fatalf("expected node %s periodUnit to migrate to day, got %#v", node.ID, got)
		}
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated definitions: %v", err)
	}
	persistedText := string(persisted)
	if !strings.Contains(persistedText, `"sourceFormat": "dsl-v1"`) {
		t.Fatalf("expected migrated file to persist sourceFormat, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `"runtime": "dsl-go-plan"`) {
		t.Fatalf("expected migrated file to persist DSL runtime, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `"periodUnit": "day"`) {
		t.Fatalf("expected migrated file to persist visualModel day semantics, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `strategy Legacy MA`) {
		t.Fatalf("expected migrated file to persist the DSL fallback script, got %s", persistedText)
	}
}

func TestStrategyDesignStoreLoadReplacesRemovedRuntimeScriptWithDSL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-definitions.json")
	legacy := `{
	  "definitions": [
	    {
	      "id": "removed-runtime-strategy",
	      "name": "Removed Runtime Strategy",
	      "version": "0.1.0",
	      "description": "legacy script payload",
	      "runtime": "removed-script-runtime",
	      "sourceFormat": "removed-script-source",
	      "symbol": "00700",
	      "interval": "1m",
	      "script": "function onInit(ctx) { console.log(ctx.symbol); }",
	      "createdAt": "2026-05-26T00:00:00Z",
	      "updatedAt": "2026-05-26T00:00:00Z"
	    }
	  ]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy definitions: %v", err)
	}

	store, err := NewStrategyDesignStore(path)
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}

	definition, ok := store.definition("removed-runtime-strategy")
	if !ok {
		t.Fatal("expected migrated definition to exist")
	}
	if strings.Contains(definition.Script, "function onInit") {
		t.Fatalf("expected removed runtime script to be replaced, got %q", definition.Script)
	}
	if !strings.Contains(definition.Script, "strategy Removed Runtime Strategy") {
		t.Fatalf("expected migrated DSL skeleton, got %q", definition.Script)
	}
	if definition.SourceFormat != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("expected DSL source format, got %q", definition.SourceFormat)
	}
	if definition.Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("expected DSL runtime, got %q", definition.Runtime)
	}
}
