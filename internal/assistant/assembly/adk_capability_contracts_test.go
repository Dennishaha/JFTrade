package assembly

import (
	"context"
	"strings"
	"testing"

	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestADKNewCapabilityToolsForwardInputsAndApprovalBoundaries(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	var selectedScope, selectedProvider string
	var instantiatedID string
	var startedID string
	var stoppedAction string
	var refreshedID string
	var riskInstance string
	var activityKind string
	var cancelledID string
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketProviders: func(context.Context) (any, error) { return map[string]any{"liveProvider": "futu"}, nil },
		SelectMarketProvider: func(_ context.Context, scope, provider string) (any, error) {
			selectedScope, selectedProvider = scope, provider
			return map[string]any{"scope": scope, "providerId": provider}, nil
		},
		RuntimeDependencies:   func(context.Context) any { return map[string]any{"status": "ready"} },
		ResearchScreenCatalog: func(market string) (any, error) { return map[string]any{"market": market}, nil },
		MarketCandlesAdvanced: func(_ context.Context, input map[string]any) (any, error) { return input, nil },
		InstantiateStrategy: func(id string, _ stratsrv.InstanceBinding) (any, error) {
			instantiatedID = id
			return map[string]any{"id": id}, nil
		},
		StartStrategyInstance: func(_ context.Context, id string) (any, error) {
			startedID = id
			return map[string]any{"id": id}, nil
		},
		StopStrategyInstance: func(id, action string) (any, error) {
			stoppedAction = id + ":" + action
			return map[string]any{"id": id, "action": action}, nil
		},
		RefreshStrategyInstance: func(id string) (any, error) {
			refreshedID = id
			return map[string]any{"id": id}, nil
		},
		UpdateStrategyInstanceRisk: func(id string, _ stratsrv.RuntimeRiskSettings) (any, error) {
			riskInstance = id
			return map[string]any{"id": id}, nil
		},
		StrategyInstanceActivity: func(id, kind string, _, _ int) (any, error) {
			activityKind = id + ":" + kind
			return map[string]any{"id": id, "kind": kind}, nil
		},
		CancelBacktestResult: func(id string) bool { cancelledID = id; return true },
	})

	call := func(name string, input map[string]any) any {
		t.Helper()
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("tool %s is not registered", name)
		}
		output, err := tool.Handler(t.Context(), input)
		if err != nil {
			t.Fatalf("tool %s: %v", name, err)
		}
		return output
	}
	call("market.providers", nil)
	call("market.provider.select", map[string]any{"scope": "backtest", "providerId": "yfinance"})
	call("system.runtime_dependencies", nil)
	call("research.screen_catalog", map[string]any{"market": "us"})
	call("market.candles", map[string]any{"market": "US", "symbol": "AAPL", "period": "1d"})
	call("strategy.instantiate", map[string]any{"definitionId": "def-1", "binding": map[string]any{"symbols": []string{"US.AAPL"}}})
	call("strategy.instance_start", map[string]any{"instanceId": "instance-1"})
	call("strategy.instance_stop", map[string]any{"instanceId": "instance-1", "action": "pause"})
	call("strategy.instance_refresh_definition", map[string]any{"instanceId": "instance-1"})
	call("strategy.instance_risk.update", map[string]any{"instanceId": "instance-1", "risk": map[string]any{"mode": "enforce"}})
	call("strategy.instance_activity", map[string]any{"instanceId": "instance-1", "kind": "audit", "limit": 10})
	call("backtest.cancel", map[string]any{"runId": "run-1"})

	if selectedScope != "backtest" || selectedProvider != "yfinance" || instantiatedID != "def-1" || startedID != "instance-1" || stoppedAction != "instance-1:pause" || refreshedID != "instance-1" || riskInstance != "instance-1" || activityKind != "instance-1:audit" || cancelledID != "run-1" {
		t.Fatalf("tool forwarding scope=%q provider=%q instantiate=%q start=%q stop=%q refresh=%q risk=%q activity=%q cancel=%q", selectedScope, selectedProvider, instantiatedID, startedID, stoppedAction, refreshedID, riskInstance, activityKind, cancelledID)
	}
	for _, name := range []string{"market.provider.select", "strategy.instance_start", "strategy.instance_risk.update"} {
		tool, _ := registry.Get(name)
		if len(tool.Descriptor.RequiresApprovalIn) == 0 {
			t.Fatalf("%s approval modes = %#v", name, tool.Descriptor.RequiresApprovalIn)
		}
	}
}

func TestADKNewCapabilityHandlersRejectUnavailableAndMalformedInputs(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	runtimeTool, _ := registry.Get("system.runtime_dependencies")
	if output, err := runtimeTool.Handler(t.Context(), map[string]any{}); err != nil || output.(map[string]any)["status"] != "unavailable" {
		t.Fatalf("unavailable runtime dependencies = %#v, %v", output, err)
	}
	for _, name := range []string{
		"market.providers", "market.provider.select",
		"research.screen_catalog", "strategy.instantiate", "strategy.instance_start",
		"strategy.instance_stop", "strategy.instance_refresh_definition", "strategy.instance_risk.update",
		"strategy.instance_activity", "backtest.cancel",
	} {
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("tool %s is not registered", name)
		}
		if _, err := tool.Handler(t.Context(), map[string]any{}); err == nil {
			t.Fatalf("unavailable %s returned nil error", name)
		}
	}

	registry = assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		SelectMarketProvider: func(context.Context, string, string) (any, error) { return nil, nil },
		InstantiateStrategy:  func(string, stratsrv.InstanceBinding) (any, error) { return nil, nil },
		UpdateStrategyInstanceRisk: func(string, stratsrv.RuntimeRiskSettings) (any, error) {
			return nil, nil
		},
		CancelBacktest: func(string) {},
		StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
			return BacktestRunSummary{}, nil
		},
	})
	selectTool, _ := registry.Get("market.provider.select")
	for _, input := range []map[string]any{
		{"scope": "other", "providerId": "futu"},
		{"scope": "live", "providerId": " "},
	} {
		if _, err := selectTool.Handler(t.Context(), input); err == nil {
			t.Fatalf("invalid provider input %#v returned nil error", input)
		}
	}
	instantiateTool, _ := registry.Get("strategy.instantiate")
	if _, err := instantiateTool.Handler(t.Context(), map[string]any{
		"definitionId": "def-1", "binding": make(chan int),
	}); err == nil {
		t.Fatal("instantiate accepted malformed binding")
	}
	riskTool, _ := registry.Get("strategy.instance_risk.update")
	if _, err := riskTool.Handler(t.Context(), map[string]any{
		"instanceId": "instance-1", "risk": make(chan int),
	}); err == nil {
		t.Fatal("risk update accepted malformed risk")
	}
	if _, err := riskTool.Handler(t.Context(), map[string]any{
		"instanceId": "instance-1", "bad": make(chan int),
	}); err == nil {
		t.Fatal("risk update accepted malformed inline risk")
	}
	for _, input := range []map[string]any{
		{"instanceId": "instance-1", "risk": map[string]any{"mode": "close_only"}},
		{"instanceId": "instance-1", "risk": map[string]any{"mode": "enforce", "maxOrderQuantity": 0}},
		{"instanceId": "instance-1", "risk": map[string]any{"mode": "monitor", "maxOrderNotional": -1}},
		{"instanceId": "instance-1", "risk": map[string]any{"mode": "enforce", "dailyMaxOrders": 0}},
	} {
		if _, err := riskTool.Handler(t.Context(), input); err == nil {
			t.Fatalf("risk update accepted invalid risk %#v", input)
		}
	}
	cancelTool, _ := registry.Get("backtest.cancel")
	if output, err := cancelTool.Handler(t.Context(), map[string]any{"runId": "run-1"}); err != nil || output.(map[string]any)["cancelRequested"] != true {
		t.Fatalf("fallback backtest cancel = %#v, %v", output, err)
	}
	if _, err := cancelTool.Handler(t.Context(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "runId") {
		t.Fatalf("missing cancel run id error = %v", err)
	}

	researchTool, _ := registry.Get("strategy.research_backtest")
	if _, err := researchTool.Handler(t.Context(), map[string]any{
		"script": "not pine",
	}); err == nil {
		t.Fatal("invalid research script returned nil error")
	}
	if _, err := researchTool.Handler(t.Context(), map[string]any{
		"script": validPineScriptForADKToolFailure, "tradingCosts": make(chan int),
	}); err == nil {
		t.Fatal("malformed research trading costs returned nil error")
	}
	optimizeRegistry := assistanttestkit.NewToolRegistry()
	registerADKStrategyOptimizationTools(nil, optimizeRegistry, ToolDeps{})
	optimizeTool, _ := optimizeRegistry.Get("strategy.optimize")
	if _, err := optimizeTool.Handler(t.Context(), map[string]any{
		"definitionId": "def-1", "tradingCosts": make(chan int),
	}); err == nil {
		t.Fatal("malformed optimization trading costs returned nil error")
	}
}
