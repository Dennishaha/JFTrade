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
		PluginID: IDPinePlanPlugin(),
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"definitionId": "mean-revert",
			"instruments": []map[string]any{{
				"market": "US",
				"code":   "AAPL",
			}},
			"symbols": []string{"US.AAPL"},
			"brokerAccount": map[string]any{
				"brokerId":           "futu",
				"accountId":          "123456",
				"tradingEnvironment": "SIMULATE",
				"market":             "US",
			},
		},
	}

	normalized := store.normalizeStrategy(input)

	if _, ok := input.Params["runtime"]; ok {
		t.Fatal("normalizeStrategy mutated input params")
	}
	if got, ok := normalized.Params["runtime"].(string); !ok || got == "" {
		t.Fatalf("normalized runtime = %#v, want non-empty string", normalized.Params["runtime"])
	}

	normalized.Params["definitionId"] = "changed"
	normalizedSymbols, ok := normalized.Params["symbols"].([]string)
	if !ok || len(normalizedSymbols) != 1 {
		t.Fatalf("normalized symbols = %#v", normalized.Params["symbols"])
	}
	normalizedSymbols[0] = "HK.00700"
	normalizedInstruments, ok := normalized.Params["instruments"].([]map[string]any)
	if !ok || len(normalizedInstruments) != 1 {
		t.Fatalf("normalized instruments = %#v", normalized.Params["instruments"])
	}
	normalizedInstruments[0]["code"] = "00700"
	normalizedBrokerAccount, ok := normalized.Params["brokerAccount"].(map[string]any)
	if !ok {
		t.Fatalf("normalized brokerAccount type = %T", normalized.Params["brokerAccount"])
	}
	normalizedBrokerAccount["brokerId"] = "ib"
	normalized.Binding.Symbols[0] = "US.MSFT"
	normalized.Binding.Instruments[0].Code = "MSFT"
	if normalized.Binding.BrokerAccount == nil {
		t.Fatal("expected normalized binding broker account")
	}
	normalized.Binding.BrokerAccount.BrokerID = "test"

	if got := input.Params["definitionId"]; got != "mean-revert" {
		t.Fatalf("input params shared with normalized copy: %#v", got)
	}
	inputSymbols, ok := input.Params["symbols"].([]string)
	if !ok || len(inputSymbols) != 1 || inputSymbols[0] != "US.AAPL" {
		t.Fatalf("input symbols shared with normalized copy: %#v", input.Params["symbols"])
	}
	inputInstruments, ok := input.Params["instruments"].([]map[string]any)
	if !ok || len(inputInstruments) != 1 || inputInstruments[0]["code"] != "AAPL" {
		t.Fatalf("input instruments shared with normalized copy: %#v", input.Params["instruments"])
	}
	inputBrokerAccount, ok := input.Params["brokerAccount"].(map[string]any)
	if !ok || inputBrokerAccount["brokerId"] != "futu" {
		t.Fatalf("input brokerAccount shared with normalized copy: %#v", input.Params["brokerAccount"])
	}
}

func TestNormalizeStrategyInstanceBindingPrefersExplicitInstruments(t *testing.T) {
	got := normalizeStrategyInstanceBinding(strategyInstanceBinding{
		Instruments: []strategyBindingInstrument{
			{Market: "us", Code: "aapl"},
			{Market: "hk", Code: "00700"},
		},
		Symbols: []string{"US.MSFT"},
	}, nil)

	if len(got.Symbols) != 2 || got.Symbols[0] != "US.AAPL" || got.Symbols[1] != "HK.00700" {
		t.Fatalf("normalized symbols = %+v", got.Symbols)
	}
	if len(got.Instruments) != 2 || got.Instruments[0].Market != "US" || got.Instruments[0].Code != "AAPL" || got.Instruments[1].Market != "HK" || got.Instruments[1].Code != "00700" {
		t.Fatalf("normalized instruments = %+v", got.Instruments)
	}
}

func TestStrategyCatalogStoreIgnoresLegacyJSONFile(t *testing.T) {
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
	t.Cleanup(func() { _ = store.Close() })
	if got := store.strategies(); len(got) != 0 {
		t.Fatalf("expected legacy json catalog to be ignored, got %+v", got)
	}
	if persisted, err := os.ReadFile(path); err != nil {
		t.Fatalf("read legacy catalog: %v", err)
	} else if string(persisted) != legacy {
		t.Fatalf("expected legacy catalog file to remain untouched, got %s", string(persisted))
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

	if normalized.PluginID != IDPinePlanPlugin() {
		t.Fatalf("expected Pine plugin, got %q", normalized.PluginID)
	}
	if got := strategyRuntimeFromParams(normalized.Params); got != strategyRuntimePinePlan {
		t.Fatalf("expected Pine runtime, got %q", got)
	}
	if got := strategySourceFormatFromParams(normalized.Params); got != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("expected Pine source format, got %q", got)
	}
	script, _ := normalized.Params["script"].(string)
	if strings.Contains(script, "function onInit") {
		t.Fatalf("expected removed runtime script to be replaced, got %q", script)
	}
	if !strings.Contains(script, "strategy(\"Removed Runtime Strategy\"") {
		t.Fatalf("expected Pine skeleton, got %q", script)
	}
}

func TestRefreshStrategyDefinitionUpdatesSnapshotForStoppedInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	store, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.saveStrategy(managedStrategyInstance{
		ID: "instance-1",
		Definition: strategyDefinitionSummary{
			StrategyID: "dsl-mean-revert",
			Name:       "Mean Revert",
			Version:    "0.1.0",
		},
		Binding: strategyInstanceBinding{
			Symbols:       []string{"US.AAPL"},
			Interval:      "1m",
			ExecutionMode: strategyExecutionModeNotifyOnly,
		},
		Params: map[string]any{
			"definitionId": "dsl-mean-revert",
			"runtime":      strategyRuntimePinePlan,
			"sourceFormat": strategydefinition.SourceFormatPineV6,
			"interval":     "1m",
			"symbols":      []string{"US.AAPL"},
			"symbol":       "US.AAPL",
			"script":       "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nlog.info(\"old\")",
		},
		Status:    strategyStatusStopped,
		CreatedAt: "2026-05-29T00:00:00Z",
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}

	item, err := store.refreshStrategyDefinition("instance-1", strategyDesignDefinition{
		ID:           "dsl-mean-revert",
		Name:         "Mean Revert v2",
		Version:      "0.1.1",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nfast = ta.sma(close, 10)\nlog.info(\"new\")",
	})
	if err != nil {
		t.Fatalf("refreshStrategyDefinition: %v", err)
	}
	if item.Definition.Version != "0.1.1" {
		t.Fatalf("refreshed definition version = %q, want 0.1.1", item.Definition.Version)
	}
	if item.Definition.Name != "Mean Revert v2" {
		t.Fatalf("refreshed definition name = %q", item.Definition.Name)
	}
	if got := strategyDefinitionIDFromParams(item.Params); got != "dsl-mean-revert" {
		t.Fatalf("definitionId = %q, want dsl-mean-revert", got)
	}
	if script, _ := item.Params["script"].(string); !strings.Contains(script, "fast = ta.sma(close, 10)") {
		t.Fatalf("expected refreshed script snapshot, got %q", script)
	}
	if symbols, ok := item.Params["symbols"].([]string); !ok || len(symbols) != 1 || symbols[0] != "US.AAPL" {
		t.Fatalf("expected binding symbols to be preserved, got %#v", item.Params["symbols"])
	}
	if audit, ok := store.strategyAudit(item.ID); !ok || len(audit.Entries) == 0 || audit.Entries[0].Kind != "definition.refreshed" {
		t.Fatalf("expected definition.refreshed audit entry, got %+v", audit)
	}
	if logs, ok := store.strategyLogs(item.ID); !ok || len(logs.Logs) == 0 || !strings.Contains(logs.Logs[0], "refreshed strategy definition dsl-mean-revert to v0.1.1") {
		t.Fatalf("expected refresh log entry, got %+v", logs)
	}

	if _, err := store.refreshStrategyDefinition("instance-1", strategyDesignDefinition{
		ID:           "dsl-mean-revert",
		Name:         "Mean Revert v3",
		Version:      "0.1.2",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nlog.info(\"busy\")",
	}); err != nil {
		t.Fatalf("second refreshStrategyDefinition: %v", err)
	}

	if err := store.saveStrategy(managedStrategyInstance{
		ID:         "instance-busy",
		Definition: strategyDefinitionSummary{StrategyID: "dsl-mean-revert", Name: "Mean Revert", Version: "0.1.0"},
		Params: map[string]any{
			"definitionId": "dsl-mean-revert",
			"runtime":      strategyRuntimePinePlan,
			"sourceFormat": strategydefinition.SourceFormatPineV6,
			"script":       "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nlog.info(\"busy\")",
		},
		Status:    strategyStatusRunning,
		CreatedAt: "2026-05-29T00:01:00Z",
	}); err != nil {
		t.Fatalf("saveStrategy busy: %v", err)
	}
	if _, err := store.refreshStrategyDefinition("instance-busy", strategyDesignDefinition{
		ID:           "dsl-mean-revert",
		Name:         "Mean Revert v4",
		Version:      "0.1.3",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nlog.info(\"busy\")",
	}); err != errStrategyInstanceBusy {
		t.Fatalf("refresh busy instance error = %v, want errStrategyInstanceBusy", err)
	}
}
