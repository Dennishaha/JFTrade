package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
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
	t.Cleanup(func() { jftradeErr1 := store.Close(); jftradeCheckTestError(t, jftradeErr1) })
	if got := store.strategies(); len(got) != 0 {
		t.Fatalf("expected legacy json catalog to be ignored, got %+v", got)
	}
	if persisted, err := os.ReadFile(path); err != nil {
		t.Fatalf("read legacy catalog: %v", err)
	} else if string(persisted) != legacy {
		t.Fatalf("expected legacy catalog file to remain untouched, got %s", string(persisted))
	}
}

func TestNormalizeStrategyKeepsExplicitLegacyRuntimeInstanceUnsupported(t *testing.T) {
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

	if normalized.PluginID != "removed-script-runtime" {
		t.Fatalf("expected explicit legacy plugin to be preserved, got %q", normalized.PluginID)
	}
	if got := strategyRuntimeFromParams(normalized.Params); got != "removed-script-runtime" {
		t.Fatalf("expected explicit legacy runtime to be preserved, got %q", got)
	}
	if got := strategySourceFormatFromParams(normalized.Params); got != "removed-script-source" {
		t.Fatalf("expected explicit legacy source format to be preserved, got %q", got)
	}
	script := jftradeCheckedTypeAssertion[string](normalized.Params["script"])
	if !strings.Contains(script, "function onInit") {
		t.Fatalf("expected explicit legacy script to be preserved, got %q", script)
	}
	if strategyInstanceStartable(normalized) {
		t.Fatalf("legacy runtime/source instance should not be startable: %+v", normalized.Params)
	}
}

func TestRefreshStrategyDefinitionUpdatesSnapshotForStoppedInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	store, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeErr2 := store.Close(); jftradeCheckTestError(t, jftradeErr2) })
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
	if script := jftradeCheckedTypeAssertion[string](item.Params["script"]); !strings.Contains(script, "fast = ta.sma(close, 10)") {
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
	}); !errors.Is(err, errStrategyInstanceBusy) {
		t.Fatalf("refresh busy instance error = %v, want errStrategyInstanceBusy", err)
	}
}

func TestApplyDefinitionToLinkedStrategiesRefreshesStoppedInstancesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-catalog.json")
	store, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeErr3 := store.Close(); jftradeCheckTestError(t, jftradeErr3) })

	oldScript := "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nlog.info(\"old\")"
	newScript := "//@version=6\nstrategy(\"Mean Revert\", overlay=true)\nfast = ta.sma(close, 10)\nstrategy.entry(\"Long\", strategy.long, qty=1)"
	linkedBinding := strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	}
	for _, input := range []managedStrategyInstance{
		{
			ID:         "linked-stale-stopped",
			Definition: strategyDefinitionSummary{StrategyID: "dsl-mean-revert", Name: "Mean Revert", Version: "0.1.0"},
			Binding:    linkedBinding,
			Params: map[string]any{
				"definitionId": "dsl-mean-revert",
				"runtime":      strategyRuntimePinePlan,
				"sourceFormat": strategydefinition.SourceFormatPineV6,
				"script":       oldScript,
			},
			Status:    strategyStatusStopped,
			CreatedAt: "2026-05-29T00:00:00Z",
		},
		{
			ID:         "linked-already-latest",
			Definition: strategyDefinitionSummary{StrategyID: "dsl-mean-revert", Name: "Mean Revert v2", Version: "0.2.0"},
			Binding:    linkedBinding,
			Params: map[string]any{
				"definitionId": "dsl-mean-revert",
				"runtime":      strategyRuntimePinePlan,
				"sourceFormat": strategydefinition.SourceFormatPineV6,
				"script":       newScript,
			},
			Status:    strategyStatusStopped,
			CreatedAt: "2026-05-29T00:01:00Z",
		},
		{
			ID:         "linked-running",
			Definition: strategyDefinitionSummary{StrategyID: "dsl-mean-revert", Name: "Mean Revert", Version: "0.1.0"},
			Binding:    linkedBinding,
			Params: map[string]any{
				"definitionId": "dsl-mean-revert",
				"runtime":      strategyRuntimePinePlan,
				"sourceFormat": strategydefinition.SourceFormatPineV6,
				"script":       oldScript,
			},
			Status:    strategyStatusRunning,
			CreatedAt: "2026-05-29T00:02:00Z",
		},
		{
			ID:         "unrelated-stale-stopped",
			Definition: strategyDefinitionSummary{StrategyID: "other-definition", Name: "Other", Version: "0.1.0"},
			Binding:    linkedBinding,
			Params: map[string]any{
				"definitionId": "other-definition",
				"runtime":      strategyRuntimePinePlan,
				"sourceFormat": strategydefinition.SourceFormatPineV6,
				"script":       oldScript,
			},
			Status:    strategyStatusStopped,
			CreatedAt: "2026-05-29T00:03:00Z",
		},
	} {
		if err := store.saveStrategy(input); err != nil {
			t.Fatalf("saveStrategy(%s): %v", input.ID, err)
		}
	}

	result, err := store.applyDefinitionToLinkedStrategies(strategyDesignDefinition{
		ID:           "dsl-mean-revert",
		Name:         "Mean Revert v2",
		Version:      "0.2.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       newScript,
	})
	if err != nil {
		t.Fatalf("applyDefinitionToLinkedStrategies: %v", err)
	}
	if result.DefinitionID != "dsl-mean-revert" || result.LatestVersion != "0.2.0" || result.TotalLinked != 3 {
		t.Fatalf("unexpected apply result summary: %+v", result)
	}
	if !stringSliceEqual(result.Applied, []string{"linked-stale-stopped"}) {
		t.Fatalf("applied = %+v", result.Applied)
	}
	if !stringSliceEqual(result.AlreadyLatest, []string{"linked-already-latest"}) {
		t.Fatalf("alreadyLatest = %+v", result.AlreadyLatest)
	}
	if !stringSliceEqual(result.SkippedBusy, []string{"linked-running"}) {
		t.Fatalf("skippedBusy = %+v", result.SkippedBusy)
	}

	refreshed, ok := store.strategy("linked-stale-stopped")
	if !ok {
		t.Fatal("refreshed strategy not found")
	}
	if refreshed.Definition.Name != "Mean Revert v2" || refreshed.Definition.Version != "0.2.0" {
		t.Fatalf("refreshed definition = %+v", refreshed.Definition)
	}
	if script := jftradeCheckedTypeAssertion[string](refreshed.Params["script"]); !strings.Contains(script, "fast = ta.sma(close, 10)") {
		t.Fatalf("expected refreshed script snapshot, got %q", script)
	}
	if symbols, ok := refreshed.Params["symbols"].([]string); !ok || len(symbols) != 1 || symbols[0] != "US.AAPL" {
		t.Fatalf("expected refreshed params to preserve binding symbols, got %#v", refreshed.Params["symbols"])
	}
	if refreshed.Binding.Interval != "1m" || refreshed.Binding.ExecutionMode != strategyExecutionModeNotifyOnly {
		t.Fatalf("expected refreshed binding to preserve runtime placement, got %+v", refreshed.Binding)
	}
	if audit, ok := store.strategyAudit(refreshed.ID); !ok || len(audit.Entries) == 0 || audit.Entries[0].Kind != "definition.refreshed" {
		t.Fatalf("expected definition.refreshed audit entry, got %+v", audit)
	}

	running, ok := store.strategy("linked-running")
	if !ok {
		t.Fatal("running strategy not found")
	}
	if running.Status != strategyStatusRunning || running.Definition.Version != "0.1.0" {
		t.Fatalf("running strategy should remain unchanged, got %+v", running)
	}
	unrelated, ok := store.strategy("unrelated-stale-stopped")
	if !ok {
		t.Fatal("unrelated strategy not found")
	}
	if unrelated.Definition.Version != "0.1.0" {
		t.Fatalf("unrelated strategy should not be refreshed, got %+v", unrelated.Definition)
	}

	reloaded, err := NewStrategyCatalogStore(path, "plugins")
	if err != nil {
		t.Fatalf("reload NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeErr4 := reloaded.Close(); jftradeCheckTestError(t, jftradeErr4) })
	persisted, ok := reloaded.strategy("linked-stale-stopped")
	if !ok {
		t.Fatal("reloaded refreshed strategy not found")
	}
	if persisted.Definition.Version != "0.2.0" {
		t.Fatalf("persisted refreshed version = %q", persisted.Definition.Version)
	}
	if logs, ok := reloaded.strategyLogs("linked-stale-stopped"); !ok || len(logs.Logs) == 0 || !strings.Contains(logs.Logs[0], "refreshed strategy definition dsl-mean-revert to v0.2.0") {
		t.Fatalf("expected persisted refresh log, got %+v", logs)
	}
}

func TestStrategyAdaptersClassifyDefinitionCatalogAndRuntimeContracts(t *testing.T) {
	designStore, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, designStore.Close()) })
	catalogStore, err := NewStrategyCatalogStore(filepath.Join(t.TempDir(), "strategy-catalog.json"), "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, catalogStore.Close()) })

	designAdapter := &strategyDesignStoreAdapter{store: designStore}
	validDefinition, err := designAdapter.SaveDefinition(stratsrv.Definition{
		ID:           "adapter-def",
		Name:         "Adapter Definition",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Adapter\", overlay=true)\nstrategy.entry(\"Long\", strategy.long, qty=1)",
	})
	if err != nil {
		t.Fatalf("SaveDefinition valid: %v", err)
	}
	if validDefinition.Version != defaultStrategyVersion || validDefinition.CreatedAt == "" {
		t.Fatalf("valid definition = %#v", validDefinition)
	}
	if _, err := designAdapter.SaveDefinition(stratsrv.Definition{
		ID:           "adapter-invalid",
		Name:         "Invalid Definition",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Invalid\", overlay=true)\nrequest.security(\"NASDAQ:AAPL\", \"D\", close)",
	}); !errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("SaveDefinition invalid err = %v, want bad request", err)
	}
	if _, err := designAdapter.DeleteDefinition("missing-definition"); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("DeleteDefinition missing err = %v, want not found", err)
	}

	catalogAdapter := &strategyCatalogStoreAdapter{store: catalogStore, designStore: designStore}
	instance, err := catalogAdapter.CreateInstance(validDefinition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if instance.Definition.StrategyID != validDefinition.ID || instance.Status != strategyStatusStopped || !instance.Startable {
		t.Fatalf("created instance = %#v", instance)
	}
	if err := catalogAdapter.ValidateStartable(stratsrv.ManagedInstance{Params: map[string]any{"runtime": "legacy-runtime"}}); !errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("ValidateStartable legacy err = %v, want bad request", err)
	}
	if _, err := catalogAdapter.UpdateInstanceRuntimeRisk(instance.ID, stratsrv.RuntimeRiskSettings{CloseOnly: true, PauseOnReject: true}); err != nil {
		t.Fatalf("UpdateInstanceRuntimeRisk: %v", err)
	}
	running, err := catalogAdapter.TransitionInstance(instance.ID, strategyStatusRunning)
	if err != nil {
		t.Fatalf("TransitionInstance running: %v", err)
	}
	if running.Status != strategyStatusRunning || len(running.Logs) == 0 {
		t.Fatalf("running transition = %#v", running)
	}
	if _, err := catalogAdapter.UpdateInstance(instance.ID, stratsrv.InstanceBinding{Symbols: []string{"US.MSFT"}, Interval: "1m"}); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("UpdateInstance running err = %v, want busy", err)
	}
	if _, err := catalogAdapter.DeleteInstance("missing-instance"); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("DeleteInstance missing err = %v, want not found", err)
	}
	if _, err := catalogAdapter.RefreshInstanceDefinition("missing-instance"); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("RefreshInstanceDefinition missing err = %v, want not found", err)
	}
	if _, err := catalogAdapter.ReconcileOnStartup(); err != nil {
		t.Fatalf("ReconcileOnStartup: %v", err)
	}
	items := catalogAdapter.ListInstances()
	if len(items) != 1 || items[0].Status != strategyStatusStopped || items[0].DefinitionSync == nil || !items[0].DefinitionSync.IsLatest {
		t.Fatalf("instances after reconcile = %#v", items)
	}

	runtimeAdapter := &strategyRuntimeManagerAdapter{mgr: &strategyRuntimeManager{
		server:   &Server{},
		runtimes: map[string]*managedStrategyRuntime{},
		starting: map[string]struct{}{},
	}}
	if err := runtimeAdapter.Start(context.Background(), stratsrv.ManagedInstance{
		ID: "bad-runtime",
		Binding: stratsrv.InstanceBinding{
			Symbols:  []string{"US.AAPL"},
			Interval: "1m",
		},
		Params: map[string]any{},
	}); !errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("runtime missing script err = %v, want bad request", err)
	}
	runtimeAdapter.mgr.exchangeProvider = func() strategyRuntimeExchange { return nil }
	if err := runtimeAdapter.Start(context.Background(), stratsrv.ManagedInstance{
		ID: "missing-exchange",
		Binding: stratsrv.InstanceBinding{
			Symbols:  []string{"US.AAPL"},
			Interval: "1m",
		},
		Params: map[string]any{"script": validDefinition.Script},
	}); !errors.Is(err, stratsrv.ErrUpstream) {
		t.Fatalf("runtime missing exchange err = %v, want upstream", err)
	}
	if summary := runtimeAdapter.RuntimeSummary(); summary.ActiveStrategies != 0 {
		t.Fatalf("empty runtime summary = %#v", summary)
	}
	if ids := runtimeAdapter.ActiveInstrumentIDs(); len(ids) != 0 {
		t.Fatalf("active instruments = %#v, want none", ids)
	}
	runtimeAdapter.Stop("missing-exchange")
}

func stringSliceEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
