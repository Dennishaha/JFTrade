package servercore

import (
	"context"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type adkTestLenOnly struct{}

func (adkTestLenOnly) Len() int { return 4 }

func TestADKRuntimeHelperInputNormalization(t *testing.T) {
	trueValue := true
	falseValue := false
	input := map[string]any{
		"boolTrue":           true,
		"boolFalse":          false,
		"blankString":        "   ",
		"yes":                "yes",
		"zeroString":         "0",
		"oneFloat":           float64(1),
		"zeroInt":            0,
		"unsupportedSlice":   []string{"true"},
		"title":              "rebalance positions",
		"description":        nil,
		"order":              "9",
		"dependsOn":          []any{"task-a", " task-b "},
		"plannerWarnings":    []any{"slow broker", " market closed "},
		"market":             "",
		"tradingEnvironment": "SIMULATE",
		"accountId":          "acct-1",
		"scope":              "",
		"symbol":             "AAPL",
		"startTime":          "2025-01-01T09:30:00Z",
		"endTime":            "2025-01-01T16:00:00Z",
		"status":             []any{"SUBMITTED"},
		"statuses":           []any{"FILLED", "CANCELLED"},
	}

	if got := optionalBoolInput(input, "missing"); got != nil {
		t.Fatalf("optionalBoolInput(missing) = %v, want nil", *got)
	}
	if got := optionalBoolInput(input, "boolTrue"); got == nil || *got != trueValue {
		t.Fatalf("optionalBoolInput(boolTrue) = %#v, want true", got)
	}
	if got := optionalBoolInput(input, "boolFalse"); got == nil || *got != falseValue {
		t.Fatalf("optionalBoolInput(boolFalse) = %#v, want false", got)
	}
	if got := optionalBoolInput(input, "yes"); got == nil || *got != trueValue {
		t.Fatalf("optionalBoolInput(yes) = %#v, want true", got)
	}
	if got := optionalBoolInput(input, "zeroString"); got == nil || *got != falseValue {
		t.Fatalf("optionalBoolInput(zeroString) = %#v, want false", got)
	}
	if got := optionalBoolInput(input, "oneFloat"); got == nil || *got != trueValue {
		t.Fatalf("optionalBoolInput(oneFloat) = %#v, want true", got)
	}
	if got := optionalBoolInput(input, "zeroInt"); got == nil || *got != falseValue {
		t.Fatalf("optionalBoolInput(zeroInt) = %#v, want false", got)
	}
	if got := optionalBoolInput(input, "blankString"); got != nil {
		t.Fatalf("optionalBoolInput(blankString) = %#v, want nil", got)
	}
	if got := optionalBoolInput(input, "unsupportedSlice"); got != nil {
		t.Fatalf("optionalBoolInput(unsupportedSlice) = %#v, want nil", got)
	}

	type nestedInput struct {
		RunID      string `json:"runId"`
		View       string `json:"view"`
		Resolution string `json:"resolution"`
		StartTime  string `json:"startTime"`
		EndTime    string `json:"endTime"`
		Include    string `json:"include"`
		Limit      int    `json:"limit"`
		Cursor     string `json:"cursor"`
	}
	viewInput := backtestResultViewInputFromNested(nestedInput{
		RunID:      "bt-1",
		View:       "chart",
		Resolution: "5m",
		StartTime:  "2025-01-01T00:00:00Z",
		EndTime:    "2025-01-01T01:00:00Z",
		Include:    "candles,trades",
		Limit:      20,
		Cursor:     "next",
	})
	if viewInput.RunID != "bt-1" || viewInput.View != "chart" || viewInput.Resolution != "5m" {
		t.Fatalf("backtestResultViewInputFromNested() = %#v, want mapped run/view/resolution", viewInput)
	}
	if len(viewInput.Include) != 2 || viewInput.Include[0] != "candles" || viewInput.Include[1] != "trades" {
		t.Fatalf("Include = %#v, want CSV split values", viewInput.Include)
	}
	mapInput := backtestResultViewInputFromNested(map[string]any{
		"runId":      "bt-map",
		"view":       "orders",
		"resolution": "1m",
		"include":    []string{"fills"},
		"limit":      5,
	})
	if mapInput.RunID != "bt-map" || mapInput.View != "orders" || mapInput.Limit != 5 || len(mapInput.Include) != 1 || mapInput.Include[0] != "fills" {
		t.Fatalf("backtestResultViewInputFromNested(map) = %#v, want direct map conversion", mapInput)
	}
	if got := backtestResultViewInputFromNested(nil); got.RunID != "" || got.View != "" || got.Limit != 0 || len(got.Include) != 0 {
		t.Fatalf("backtestResultViewInputFromNested(nil) = %#v, want zero value", got)
	}
	if got := backtestResultViewInputFromNested(func() {}); got.RunID != "" || got.View != "" || got.Limit != 0 || len(got.Include) != 0 {
		t.Fatalf("backtestResultViewInputFromNested(func) = %#v, want zero value on marshal failure", got)
	}

	patch := taskPatchFromInput(input)
	if patch.Title == nil || *patch.Title != "rebalance positions" {
		t.Fatalf("Title = %#v, want rebalance positions", patch.Title)
	}
	if patch.Description == nil || *patch.Description != "" {
		t.Fatalf("Description = %#v, want explicit empty string pointer for nil payload", patch.Description)
	}
	if patch.Order == nil || *patch.Order != 9 {
		t.Fatalf("Order = %#v, want 9", patch.Order)
	}
	if len(patch.DependsOn) != 2 || patch.DependsOn[1] != "task-b" {
		t.Fatalf("DependsOn = %#v, want trimmed dependency list", patch.DependsOn)
	}
	if len(patch.PlannerWarnings) != 2 || patch.PlannerWarnings[1] != "market closed" {
		t.Fatalf("PlannerWarnings = %#v, want trimmed warnings", patch.PlannerWarnings)
	}
	if got := intPtrFromInput(map[string]any{}, "order"); got != nil {
		t.Fatalf("intPtrFromInput(absent) = %#v, want nil", got)
	}
	if got := stringPtrFromInput(map[string]any{"status": 404}, "status"); got == nil || *got != "404" {
		t.Fatalf("stringPtrFromInput(non-string) = %#v, want \"404\"", got)
	}
	if got := stringSliceFromPresentInput(map[string]any{"symbols": []string{"US.AAPL"}}, "symbols"); len(got) != 1 || got[0] != "US.AAPL" {
		t.Fatalf("stringSliceFromPresentInput() = %#v, want single symbol", got)
	}

	read := brokerReadInput(input, ToolDeps{
		DefaultTradeMarket: func() string { return "US" },
	}, "CURRENT")
	if read.Market != "US" || read.Scope != "CURRENT" {
		t.Fatalf("brokerReadInput() = %#v, want default market and scope", read)
	}
	if read.AccountID != "acct-1" || read.TradingEnvironment != "SIMULATE" || read.Symbol != "AAPL" {
		t.Fatalf("brokerReadInput() = %#v, want account/env/symbol copied", read)
	}
	if len(read.Status) != 1 || read.Status[0] != "SUBMITTED" || len(read.Statuses) != 2 || read.Statuses[1] != "CANCELLED" {
		t.Fatalf("brokerReadInput status fields = %#v / %#v, want both lists preserved", read.Status, read.Statuses)
	}
}

func TestADKRuntimePollingAndPayloadHelpers(t *testing.T) {
	if got := waitForADKBacktestStatus(context.Background(), ToolDeps{}, "", 100, "queued"); got != "queued" {
		t.Fatalf("waitForADKBacktestStatus(no deps) = %q, want queued", got)
	}

	backtestCalls := 0
	status := waitForADKBacktestStatus(context.Background(), ToolDeps{
		BacktestResultView: func(BacktestResultViewInput) (any, error) {
			backtestCalls++
			return map[string]any{"run": map[string]any{"status": "completed"}}, nil
		},
	}, "bt-done", 300, "running")
	if status != "completed" || backtestCalls != 1 {
		t.Fatalf("waitForADKBacktestStatus() = %q calls=%d, want completed after first terminal read", status, backtestCalls)
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledCalls := 0
	cancelledStatus := waitForADKBacktestStatus(cancelledCtx, ToolDeps{
		BacktestResultView: func(BacktestResultViewInput) (any, error) {
			cancelledCalls++
			return map[string]any{"run": map[string]any{"status": "running"}}, nil
		},
	}, "bt-cancelled", 350, "pending")
	if cancelledStatus != "running" || cancelledCalls != 1 {
		t.Fatalf("waitForADKBacktestStatus(cancelled) = %q calls=%d, want first observed status before context exit", cancelledStatus, cancelledCalls)
	}

	progressCalls := 0
	progress, ok := waitForADKKLineSyncProgress(context.Background(), ToolDeps{
		BacktestKLineSyncProgress: func(taskID string) (*backtest.SyncProgress, bool) {
			progressCalls++
			if progressCalls == 1 {
				return &backtest.SyncProgress{TaskID: taskID, Status: "running", Symbol: "US.AAPL"}, true
			}
			return &backtest.SyncProgress{TaskID: taskID, Status: "completed", Symbol: "US.AAPL"}, true
		},
	}, "sync-1", 350)
	if !ok || progress == nil || progress.Status != "completed" || progressCalls < 2 {
		t.Fatalf("waitForADKKLineSyncProgress() progress=%#v ok=%v calls=%d, want completed after polling", progress, ok, progressCalls)
	}

	lostProgressCalls := 0
	lostProgress, found := waitForADKKLineSyncProgress(context.Background(), ToolDeps{
		BacktestKLineSyncProgress: func(taskID string) (*backtest.SyncProgress, bool) {
			lostProgressCalls++
			if lostProgressCalls == 1 {
				return &backtest.SyncProgress{TaskID: taskID, Status: "running", Symbol: "US.AAPL"}, true
			}
			return nil, false
		},
	}, "sync-missing", 350)
	if found || lostProgress != nil || lostProgressCalls < 2 {
		t.Fatalf("waitForADKKLineSyncProgress(missing) progress=%#v found=%v calls=%d, want nil/false after lookup disappears", lostProgress, found, lostProgressCalls)
	}

	if isTerminalKLineSyncStatus("running") {
		t.Fatal("isTerminalKLineSyncStatus(running) = true, want false")
	}
	if !isTerminalKLineSyncStatus("completed") {
		t.Fatal("isTerminalKLineSyncStatus(completed) = false, want true")
	}
	if got := statusFromBacktestResultView(map[string]any{"run": map[string]any{"status": " failed "}}); got != "failed" {
		t.Fatalf("statusFromBacktestResultView() = %q, want failed", got)
	}
	if got := statusFromBacktestResultView(map[string]any{"run": "not-a-map"}); got != "" {
		t.Fatalf("statusFromBacktestResultView(non-map run) = %q, want empty", got)
	}
	if isTerminalBacktestStatus("running") {
		t.Fatal("isTerminalBacktestStatus(running) = true, want false")
	}
	if !isTerminalBacktestStatus("cancelled") {
		t.Fatal("isTerminalBacktestStatus(cancelled) = false, want true")
	}

	nilPayload := klineSyncProgressPayload(nil)
	if nilPayload["status"] != "unknown" || nilPayload["readyToRetry"] != false {
		t.Fatalf("klineSyncProgressPayload(nil) = %#v, want unknown/not ready", nilPayload)
	}

	syncingPayload := backtestDataReadinessPayload(BacktestDataReadiness{
		Status: "syncing_data",
		DataSync: &BacktestDataSync{
			TaskID: "sync-1", Symbol: "US.AAPL", Intervals: []string{"1m", "5m"},
			Since: "2025-01-01", Until: "2025-01-02", SessionScope: "regular", Status: "running",
		},
		Progress: &backtest.SyncProgress{TaskID: "sync-1", Status: "completed", Symbol: "US.AAPL"},
	})
	nextTool, ok := syncingPayload["nextTool"].(map[string]any)
	if syncingPayload["ok"] != true || syncingPayload["status"] != "syncing_data" || !ok {
		t.Fatalf("backtestDataReadinessPayload(syncing) = %#v, want retry guidance", syncingPayload)
	}
	if nextTool["name"] != "backtest.kline_sync_status" {
		t.Fatalf("nextTool = %#v, want kline status tool", nextTool)
	}

	failedPayload := backtestDataReadinessPayload(BacktestDataReadiness{
		Status: "failed",
		Error:  "coverage still insufficient",
	})
	if failedPayload["ok"] != false || failedPayload["error"] != "coverage still insufficient" {
		t.Fatalf("backtestDataReadinessPayload(failed) = %#v, want failed response with error", failedPayload)
	}
}

func TestADKReadToolsNormalizeInputsAndExposeBusinessHandlers(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	var ordersInput BrokerReadInput
	var fillsInput BrokerReadInput
	var cashFlowsInput BrokerReadInput
	var feesInput BrokerReadInput
	var marginInput BrokerReadInput
	var depthMarket, depthSymbol string
	var depthNum int

	registerJFTradeADKReadTools(registry, ToolDeps{
		DefaultTradeMarket: func() string { return "US" },
		BrokerOrders: func(_ context.Context, input BrokerReadInput) (any, error) {
			ordersInput = input
			return map[string]any{"orders": []string{"ord-1"}}, nil
		},
		BrokerFills: func(_ context.Context, input BrokerReadInput) (any, error) {
			fillsInput = input
			return map[string]any{"fills": []string{"fill-1"}}, nil
		},
		BrokerCashFlows: func(_ context.Context, input BrokerReadInput) (any, error) {
			cashFlowsInput = input
			return map[string]any{"cashFlows": []string{"flow-1"}}, nil
		},
		BrokerFees: func(_ context.Context, input BrokerReadInput) (any, error) {
			feesInput = input
			return map[string]any{"fees": []string{"fee-1"}}, nil
		},
		BrokerMarginRatios: func(_ context.Context, input BrokerReadInput) (any, error) {
			marginInput = input
			return map[string]any{"ratios": []string{"margin-1"}}, nil
		},
		MarketDepth: func(_ context.Context, market, symbol string, num int) (any, error) {
			depthMarket, depthSymbol, depthNum = market, symbol, num
			return map[string]any{"symbol": market + "." + symbol}, nil
		},
		RiskState:  func() any { return map[string]any{"killSwitch": false} },
		RiskEvents: func() any { return []string{"none"} },
		ExecutionOrders: func() any {
			return []map[string]any{{"id": "exec-1"}}
		},
		ExecutionOrderEvents: func(internalOrderID string) any {
			return map[string]any{"internalOrderId": internalOrderID, "events": []string{"accepted"}}
		},
	})

	ordersTool, _ := registry.Get("broker.orders")
	if _, err := ordersTool.Handler(context.Background(), map[string]any{
		"tradingEnvironment": "REAL",
		"accountId":          "acct-live",
		"symbol":             "AAPL",
		"status":             []any{"SUBMITTED"},
	}); err != nil {
		t.Fatalf("broker.orders Handler: %v", err)
	}
	if ordersInput.Scope != "CURRENT" || ordersInput.Market != "US" || ordersInput.AccountID != "acct-live" || ordersInput.Symbol != "AAPL" {
		t.Fatalf("ordersInput = %#v, want normalized CURRENT/US/account/symbol", ordersInput)
	}

	fillsTool, _ := registry.Get("broker.fills")
	if _, err := fillsTool.Handler(context.Background(), map[string]any{
		"scope":     "history",
		"symbol":    "MSFT",
		"startTime": "2025-01-01T09:30:00Z",
		"endTime":   "2025-01-01T16:00:00Z",
	}); err != nil {
		t.Fatalf("broker.fills Handler: %v", err)
	}
	if fillsInput.Scope != "history" || fillsInput.Symbol != "MSFT" || fillsInput.StartTime == "" || fillsInput.EndTime == "" {
		t.Fatalf("fillsInput = %#v, want history fill query", fillsInput)
	}

	cashFlowsTool, _ := registry.Get("broker.cash_flows")
	if _, err := cashFlowsTool.Handler(context.Background(), map[string]any{
		"market":       "HK",
		"clearingDate": "2025-01-03",
		"direction":    "IN",
	}); err != nil {
		t.Fatalf("broker.cash_flows Handler: %v", err)
	}
	if cashFlowsInput.Market != "HK" || cashFlowsInput.ClearingDate != "2025-01-03" || cashFlowsInput.Direction != "IN" {
		t.Fatalf("cashFlowsInput = %#v, want market/clearingDate/direction", cashFlowsInput)
	}

	feesTool, _ := registry.Get("broker.fees")
	if _, err := feesTool.Handler(context.Background(), map[string]any{
		"orderIdEx":     []any{"oid-1"},
		"orderIdExList": []any{"oid-2", "oid-3"},
	}); err != nil {
		t.Fatalf("broker.fees Handler: %v", err)
	}
	if len(feesInput.OrderIDEx) != 1 || feesInput.OrderIDEx[0] != "oid-1" || len(feesInput.OrderIDExList) != 2 {
		t.Fatalf("feesInput = %#v, want fee order identifiers", feesInput)
	}

	marginTool, _ := registry.Get("broker.margin_ratios")
	if _, err := marginTool.Handler(context.Background(), map[string]any{
		"symbols": []any{"700", "3690"},
		"symbol":  "AAPL",
	}); err != nil {
		t.Fatalf("broker.margin_ratios Handler: %v", err)
	}
	if len(marginInput.Symbols) != 3 || marginInput.Symbols[2] != "AAPL" {
		t.Fatalf("marginInput = %#v, want appended explicit symbol", marginInput)
	}

	depthTool, _ := registry.Get("market.depth")
	if _, err := depthTool.Handler(context.Background(), map[string]any{}); err == nil {
		t.Fatal("market.depth missing instrument error = nil, want validation error")
	}
	if _, err := depthTool.Handler(context.Background(), map[string]any{
		"query": "please inspect US.AAPL depth now",
		"num":   "15",
	}); err != nil {
		t.Fatalf("market.depth Handler: %v", err)
	}
	if depthMarket != "US" || depthSymbol != "AAPL" || depthNum != 15 {
		t.Fatalf("market.depth args = %q %q %d, want US AAPL 15", depthMarket, depthSymbol, depthNum)
	}

	riskStateTool, _ := registry.Get("risk.state")
	riskStateOutput, err := riskStateTool.Handler(context.Background(), map[string]any{})
	if err != nil || riskStateOutput.(map[string]any)["killSwitch"] != false {
		t.Fatalf("risk.state output=%#v err=%v, want killSwitch=false", riskStateOutput, err)
	}

	riskEventsTool, _ := registry.Get("risk.events")
	riskEventsOutput, err := riskEventsTool.Handler(context.Background(), map[string]any{})
	if err != nil || riskEventsOutput.([]string)[0] != "none" {
		t.Fatalf("risk.events output=%#v err=%v, want risk event payload", riskEventsOutput, err)
	}

	orderEventsTool, _ := registry.Get("execution.order_events")
	orderListOutput, err := orderEventsTool.Handler(context.Background(), map[string]any{})
	if err != nil || len(orderListOutput.([]map[string]any)) != 1 {
		t.Fatalf("execution.order_events list output=%#v err=%v, want execution order list", orderListOutput, err)
	}
	orderEventOutput, err := orderEventsTool.Handler(context.Background(), map[string]any{"internalOrderId": "exec-1"})
	if err != nil || orderEventOutput.(map[string]any)["internalOrderId"] != "exec-1" {
		t.Fatalf("execution.order_events detail output=%#v err=%v, want event timeline", orderEventOutput, err)
	}
}

func TestADKRuntimeMiscHelpersAndMetadata(t *testing.T) {
	metadata := StrategyMetadataPayload(nil)
	if metadata["defaultQtyMode"] != "fixed" || metadata["defaultQtyValue"] != "1" || metadata["pyramiding"] != 1 {
		t.Fatalf("StrategyMetadataPayload(nil) = %#v, want defaults", metadata)
	}

	program := &strategyir.Program{
		Metadata: strategyir.StrategyMetadata{
			Name:                    "Momentum",
			Version:                 "v2",
			Symbol:                  "US.TME",
			Interval:                "5m",
			DefaultQtyMode:          "percent_of_equity",
			DefaultQtyValue:         "25",
			Pyramiding:              3,
			MaxDrawdownValue:        12,
			MaxDrawdownType:         "percent_of_equity",
			MaxIntradayFilledOrders: 4,
			MaxPositionSize:         7,
		},
		Hooks: []strategyir.HookBlock{{Kind: strategyir.HookInit}, {Kind: strategyir.HookKLineClose}},
	}
	metadata = strategyMetadataPayload(program)
	if metadata["name"] != "Momentum" || metadata["symbol"] != "US.TME" || metadata["pyramiding"] != 3 {
		t.Fatalf("strategyMetadataPayload(program) = %#v, want populated metadata", metadata)
	}
	risk, ok := metadata["risk"].(map[string]any)
	if !ok || risk["maxPositionSize"] != 7.0 {
		t.Fatalf("strategyMetadataPayload(program) risk = %#v, want risk metadata", metadata["risk"])
	}
	hooks := BuildCompiledHookKinds(program)
	if len(hooks) != 2 || hooks[1] != "on_kline_close" {
		t.Fatalf("BuildCompiledHookKinds() = %#v, want ordered hook names", hooks)
	}
	defaultedProgram := &strategyir.Program{Metadata: strategyir.StrategyMetadata{}}
	metadata = strategyMetadataPayload(defaultedProgram)
	if metadata["defaultQtyMode"] != "fixed" || metadata["defaultQtyValue"] != "1" || metadata["pyramiding"] != 1 {
		t.Fatalf("strategyMetadataPayload(defaultedProgram) = %#v, want normalized qty and pyramiding defaults", metadata)
	}
	if hooks := BuildCompiledHookKinds(nil); len(hooks) != 0 {
		t.Fatalf("BuildCompiledHookKinds(nil) = %#v, want empty slice", hooks)
	}

	requirements := BuildCompiledRequirementsPayload(strategyir.Requirements{
		Indicators:       []strategyir.IndicatorRequirement{{Alias: "fast", Kind: "ema", Key: "ema:10"}},
		RequiresPosition: true,
	})
	indicators, ok := requirements["indicators"].([]map[string]any)
	if !ok || len(indicators) != 1 || indicators[0]["alias"] != "fast" {
		t.Fatalf("BuildCompiledRequirementsPayload() = %#v, want indicators payload", requirements)
	}

	if got := pageEnvelope(20, 40, 55, 15); got["hasMore"] != false || got["returned"] != 15 {
		t.Fatalf("pageEnvelope() = %#v, want final page summary", got)
	}
	market, symbol := inferMarketSymbol(map[string]any{"query": "关注 hk.00700。"})
	if market != "HK" || symbol != "00700" {
		t.Fatalf("inferMarketSymbol(query) = %q %q, want HK 00700", market, symbol)
	}
	market, symbol = inferMarketSymbol(map[string]any{"market": "US", "symbol": "tsla"})
	if market != "US" || symbol != "TSLA" {
		t.Fatalf("inferMarketSymbol(explicit) = %q %q, want US TSLA", market, symbol)
	}
	market, symbol = inferMarketSymbol(map[string]any{"query": "rotate into sg.TSLA after lunch"})
	if market != "SG" || symbol != "TSLA" {
		t.Fatalf("inferMarketSymbol(field split) = %q %q, want SG TSLA", market, symbol)
	}
	market, symbol = inferMarketSymbol(map[string]any{"query": "no clear ticker here"})
	if market != "" || symbol != "" {
		t.Fatalf("inferMarketSymbol(no match) = %q %q, want empty values", market, symbol)
	}

	if got := floatValue(map[string]any{"balance": float64(1.5)}, "balance", 0); got != 1.5 {
		t.Fatalf("floatValue(float64) = %v, want 1.5", got)
	}
	if got := floatValue(map[string]any{"balance": "2.75"}, "balance", 0); got != 2.75 {
		t.Fatalf("floatValue(string) = %v, want 2.75", got)
	}
	if got := floatValue(map[string]any{}, "balance", 9.5); got != 9.5 {
		t.Fatalf("floatValue(default) = %v, want 9.5", got)
	}
	if got := boolInputValue(map[string]any{"enabled": "true"}, "enabled"); got != true {
		t.Fatalf("boolInputValue(string true) = %v, want true", got)
	}
	if got := boolInputValueDefault(map[string]any{"enabled": 1}, "enabled", true); got != true {
		t.Fatalf("boolInputValueDefault(non-bool fallback) = %v, want default true", got)
	}

	if got := summarizeADKText("  market    open   soon  ", 80); got != "market open soon" {
		t.Fatalf("summarizeADKText(trimmed) = %q, want collapsed whitespace", got)
	}
	if got := summarizeADKText("abcdef", 4); got != "abcd..." {
		t.Fatalf("summarizeADKText(truncated) = %q, want abcd...", got)
	}
	if got := lastString([]string{"a", "b"}); got != "b" {
		t.Fatalf("lastString() = %q, want b", got)
	}
	if got := lastString(nil); got != "" {
		t.Fatalf("lastString(nil) = %q, want empty string", got)
	}
	if got := lastBacktestTrade(nil); got != nil {
		t.Fatalf("lastBacktestTrade(nil) = %#v, want nil", got)
	}
	if got := lastBacktestTrade([]backtest.TradeEvent{{Side: "BUY"}, {Side: "SELL"}}); got == nil || got.Side != "SELL" {
		t.Fatalf("lastBacktestTrade() = %#v, want SELL trade", got)
	}
	if got := lastBacktestCandle(nil); got != nil {
		t.Fatalf("lastBacktestCandle(nil) = %#v, want nil", got)
	}
	if got := lastBacktestCandle([]backtest.Candle{{Close: "10"}, {Close: "11"}}); got == nil || got.Close != "11" {
		t.Fatalf("lastBacktestCandle() = %#v, want latest candle", got)
	}

	if collectionLen([]any{"a", "b"}) != 2 {
		t.Fatal("collectionLen([]any) != 2")
	}
	if collectionLen([]map[string]any{{"id": 1}}) != 1 {
		t.Fatal("collectionLen([]map[string]any) != 1")
	}
	if collectionLen(adkTestLenOnly{}) != 4 {
		t.Fatal("collectionLen(Len()) != 4")
	}
	if collectionLen("plain") != 0 {
		t.Fatal("collectionLen(plain) != 0")
	}

	if got := callMap(nil); len(got) != 0 {
		t.Fatalf("callMap(nil) = %#v, want empty map", got)
	}
	if got := callMap(func() map[string]any { return map[string]any{"ok": true} }); got["ok"] != true {
		t.Fatalf("callMap(fn) = %#v, want ok=true", got)
	}
	if callBool(nil) {
		t.Fatal("callBool(nil) = true, want false")
	}
	if !callBool(func() bool { return true }) {
		t.Fatal("callBool(true fn) = false, want true")
	}

	timestamp := nowStringRFC3339Nano()
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("nowStringRFC3339Nano() = %q, parse error: %v", timestamp, err)
	}
	if got := SourceFormatPineV6(); got != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("SourceFormatPineV6() = %q, want %q", got, strategydefinition.SourceFormatPineV6)
	}
}

func TestADKCoreToolHandlersNormalizeMarketAndPortfolioFlows(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	var snapshotMarket, snapshotSymbol string
	var candlesMarket, candlesSymbol, candlesPeriod string
	var candlesLimit int
	var fundsQuery, positionsQuery any

	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		FutuOpenDHealth: func(context.Context) (any, error) {
			return map[string]any{"connected": true}, nil
		},
		PluginCatalog: func() any {
			return map[string]any{"plugins": []string{"alpha"}}
		},
		MarketSubscriptions: func(context.Context) (any, any, error) {
			return []string{"US.AAPL"}, []string{"US.AAPL"}, nil
		},
		MarketSnapshot: func(_ context.Context, market, symbol string) (any, error) {
			snapshotMarket, snapshotSymbol = market, symbol
			return map[string]any{"symbol": market + "." + symbol}, nil
		},
		MarketCandles: func(_ context.Context, market, symbol, period string, limit int) (any, error) {
			candlesMarket, candlesSymbol, candlesPeriod, candlesLimit = market, symbol, period, limit
			return map[string]any{"count": limit}, nil
		},
		ManagedAccounts: func() any { return []string{"acct-1"} },
		BrokerEnabled:   func() bool { return true },
		DefaultTradeMarket: func() string {
			return "US"
		},
		ExecutionOrders: func() any {
			return []map[string]any{{"id": "ord-1"}, {"id": "ord-2"}}
		},
		BrokerFunds: func(_ context.Context, query broker.ReadQuery, _ time.Duration) any {
			fundsQuery = query
			return map[string]any{"cash": 1000}
		},
		BrokerPositions: func(_ context.Context, query broker.ReadQuery, _ time.Duration) any {
			positionsQuery = query
			return []map[string]any{{"symbol": "AAPL"}}
		},
	})

	futuTool, _ := registry.Get("system.futu_opend")
	futuOutput, err := futuTool.Handler(context.Background(), map[string]any{})
	if err != nil || futuOutput.(map[string]any)["connected"] != true {
		t.Fatalf("system.futu_opend output=%#v err=%v, want connected payload", futuOutput, err)
	}

	catalogTool, _ := registry.Get("plugins.catalog")
	catalogOutput, err := catalogTool.Handler(context.Background(), map[string]any{})
	if err != nil || catalogOutput.(map[string]any)["plugins"].([]string)[0] != "alpha" {
		t.Fatalf("plugins.catalog output=%#v err=%v, want plugin catalog", catalogOutput, err)
	}

	subscriptionsTool, _ := registry.Get("market.subscriptions")
	subscriptionsOutput, err := subscriptionsTool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("market.subscriptions Handler: %v", err)
	}
	subscriptionsPayload := subscriptionsOutput.(map[string]any)
	if _, parseErr := time.Parse(time.RFC3339Nano, subscriptionsPayload["checkedAt"].(string)); parseErr != nil {
		t.Fatalf("market.subscriptions checkedAt parse error: %v", parseErr)
	}

	snapshotTool, _ := registry.Get("market.snapshot")
	if _, err := snapshotTool.Handler(context.Background(), map[string]any{}); err == nil {
		t.Fatal("market.snapshot missing instrument error = nil, want validation error")
	}
	if _, err := snapshotTool.Handler(context.Background(), map[string]any{"query": "watch us.msft into the open"}); err != nil {
		t.Fatalf("market.snapshot Handler: %v", err)
	}
	if snapshotMarket != "US" || snapshotSymbol != "MSFT" {
		t.Fatalf("market.snapshot args = %q %q, want US MSFT", snapshotMarket, snapshotSymbol)
	}

	candlesTool, _ := registry.Get("market.candles")
	if _, err := candlesTool.Handler(context.Background(), map[string]any{"query": "US.MSFT", "limit": 0}); err == nil {
		t.Fatal("market.candles limit validation error = nil, want range error")
	}
	if _, err := candlesTool.Handler(context.Background(), map[string]any{
		"query":  "US.MSFT",
		"period": "60min",
		"limit":  20,
	}); err != nil {
		t.Fatalf("market.candles Handler: %v", err)
	}
	if candlesMarket != "US" || candlesSymbol != "MSFT" || candlesPeriod != "1h" || candlesLimit != 20 {
		t.Fatalf("market.candles args = %q %q %q %d, want US MSFT 1h 20", candlesMarket, candlesSymbol, candlesPeriod, candlesLimit)
	}

	portfolioTool, _ := registry.Get("portfolio.summary")
	portfolioOutput, err := portfolioTool.Handler(context.Background(), map[string]any{
		"accountId":          "acct-1",
		"tradingEnvironment": "real",
	})
	if err != nil {
		t.Fatalf("portfolio.summary Handler: %v", err)
	}
	portfolioPayload := portfolioOutput.(map[string]any)
	if portfolioPayload["orderCount"] != 2 || portfolioPayload["brokerEnabled"] != true {
		t.Fatalf("portfolio.summary payload = %#v, want broker summary", portfolioPayload)
	}
	if fundsQuery != (broker.ReadQuery{BrokerID: "futu", AccountID: "acct-1", TradingEnvironment: "REAL", Market: "US"}) {
		t.Fatalf("fundsQuery = %#v, want normalized broker read query", fundsQuery)
	}
	if positionsQuery != fundsQuery {
		t.Fatalf("positionsQuery = %#v, want same query as funds", positionsQuery)
	}

	ordersTool, _ := registry.Get("account.orders")
	ordersOutput, err := ordersTool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("account.orders Handler: %v", err)
	}
	ordersPayload := ordersOutput.(map[string]any)
	if ordersPayload["count"] != 2 {
		t.Fatalf("account.orders payload = %#v, want count=2", ordersPayload)
	}
}
