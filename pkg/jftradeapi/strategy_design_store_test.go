package jftradeapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	      "runtime": "quickjs-js",
	      "symbol": "00700",
	      "interval": "1m",
	      "script": "function onKLineClosed(ctx) { const fast = ctx.indicators[\"ma:EMA:5\"]; const slow = ctx.indicators[\"ma:20\"]; return fast && slow; }",
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
	if !strings.Contains(definition.Script, `ma:EMA:5:day`) {
		t.Fatalf("expected fast MA script to migrate to day semantics, got %q", definition.Script)
	}
	if !strings.Contains(definition.Script, `ma:MA:20:day`) {
		t.Fatalf("expected slow MA script to migrate to day semantics, got %q", definition.Script)
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
	if !strings.Contains(persistedText, `"periodUnit": "day"`) {
		t.Fatalf("expected migrated file to persist visualModel day semantics, got %s", persistedText)
	}
	if !strings.Contains(persistedText, `ma:EMA:5:day`) || !strings.Contains(persistedText, `ma:MA:20:day`) {
		t.Fatalf("expected migrated file to persist day semantic MA keys, got %s", persistedText)
	}
}
