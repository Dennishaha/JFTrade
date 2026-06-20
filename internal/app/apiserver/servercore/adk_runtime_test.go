package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestValidateADKStrategyDraftScriptRejectsUnsupportedPineRuntimeSemantics(t *testing.T) {
	script := `//@version=6
strategy("TME_Bollinger_RSI_V1")
htfClose = request.security("NASDAQ:AAPL", "1D", close)`

	err := validateADKStrategyDraftScript(script)
	if err == nil {
		t.Fatal("validateADKStrategyDraftScript() error = nil, want unsupported Pine rejection")
	}
	if !strings.Contains(err.Error(), "Pine Script v6") || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("validateADKStrategyDraftScript() error = %q, want Pine unsupported-feature hint", err)
	}
}

func TestValidateADKStrategyDraftScriptAcceptsJFTradePine(t *testing.T) {
	script := `//@version=6
strategy("Mean Revert", overlay=true)
htfClose = request.security(syminfo.tickerid, "1D", close)
log.info("ready")`

	if err := validateADKStrategyDraftScript(script); err != nil {
		t.Fatalf("validateADKStrategyDraftScript() error = %v", err)
	}
}

func TestValidateADKStrategyDraftScriptReturnsSharedHintForInvalidPine(t *testing.T) {
	err := validateADKStrategyDraftScript(`strategy("Broken")
fast =`)
	if err == nil {
		t.Fatal("validateADKStrategyDraftScript() error = nil, want invalid Pine error")
	}
	if !strings.Contains(err.Error(), "可以先查询 Pine v6 规范和示例，确认脚本格式正确。也可以从下面这个 JFTrade Pine v6 骨架开始") {
		t.Fatalf("validateADKStrategyDraftScript() error = %q, want shared skeleton hint", err)
	}
}

func TestADKStrategyPineSpecToolReturnsStructuredPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	tool, ok := server.adkRuntime.Tools().Get(strategypinespec.ToolName)
	if !ok {
		t.Fatalf("%s tool not registered", strategypinespec.ToolName)
	}
	if tool.Descriptor.Category != "strategy" || tool.Descriptor.Permission != "read_internal" {
		t.Fatalf("descriptor = %+v, want strategy/read_internal", tool.Descriptor)
	}
	if tool.Descriptor.InputSchema == nil {
		t.Fatalf("descriptor input schema = nil")
	}

	output, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	if got := payload["sourceFormat"]; got != strategypinespec.SourceFormat {
		t.Fatalf("sourceFormat = %#v, want %q", got, strategypinespec.SourceFormat)
	}
	if got := payload["runtime"]; got != strategypinespec.Runtime {
		t.Fatalf("runtime = %#v, want %q", got, strategypinespec.Runtime)
	}
	examples, ok := payload["examples"].([]map[string]any)
	if !ok {
		t.Fatalf("examples payload = %T, want []map[string]any", payload["examples"])
	}
	if len(examples) != 0 {
		t.Fatalf("default examples len = %d, want 0", len(examples))
	}
	supportMatrix, ok := payload["supportMatrix"].([]map[string]any)
	if !ok || len(supportMatrix) == 0 {
		t.Fatalf("supportMatrix payload = %#v, want non-empty support matrix", payload["supportMatrix"])
	}
	if _, ok := payload["compatibilityLayers"]; ok {
		t.Fatalf("compatibilityLayers should not be present in v1.0 payload: %#v", payload["compatibilityLayers"])
	}
	unsupportedPatterns, ok := payload["unsupportedPatterns"].([]string)
	if !ok || len(unsupportedPatterns) == 0 {
		t.Fatalf("unsupportedPatterns payload = %#v, want non-empty unsupported pattern list", payload["unsupportedPatterns"])
	}
	goldenScripts, ok := payload["goldenScripts"].([]map[string]any)
	if !ok || len(goldenScripts) == 0 {
		t.Fatalf("goldenScripts payload = %#v, want non-empty golden script table", payload["goldenScripts"])
	}

	output, err = tool.Handler(context.Background(), map[string]any{
		"section":         "orders",
		"includeExamples": true,
	})
	if err != nil {
		t.Fatalf("tool.Handler orders: %v", err)
	}
	payload, ok = output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	if got := payload["selectedSection"]; got != "orders" {
		t.Fatalf("selectedSection = %#v, want orders", got)
	}
	examples, ok = payload["examples"].([]map[string]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("examples payload = %#v, want populated examples", payload["examples"])
	}
}

func TestADKStrategyValidateDSLToolReturnsValidationPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	tool, ok := server.adkRuntime.Tools().Get("strategy.validate_pine")
	if !ok {
		t.Fatal("strategy.validate_pine tool not registered")
	}
	if tool.Descriptor.Category != "strategy" || tool.Descriptor.Permission != "read_internal" {
		t.Fatalf("descriptor = %+v, want strategy/read_internal", tool.Descriptor)
	}
	if tool.Descriptor.InputSchema == nil {
		t.Fatal("strategy.validate_pine input schema = nil")
	}

	output, err := tool.Handler(context.Background(), map[string]any{
		"script": strategypinespec.Skeleton(),
	})
	if err != nil {
		t.Fatalf("tool.Handler(valid): %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	if got := payload["ok"]; got != true {
		t.Fatalf("ok = %#v, want true", got)
	}
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok || metadata["name"] != "Minimal Draft" {
		t.Fatalf("metadata = %#v, want parsed Pine strategy name", payload["metadata"])
	}
	hooks, ok := payload["hooks"].([]string)
	if !ok || len(hooks) == 0 {
		t.Fatalf("hooks = %#v, want non-empty []string", payload["hooks"])
	}
	requirements, ok := payload["requirements"].(map[string]any)
	if !ok {
		t.Fatalf("requirements = %T, want map[string]any", payload["requirements"])
	}
	if _, ok := requirements["indicators"]; !ok {
		t.Fatalf("requirements = %#v, want indicators field", requirements)
	}

	output, err = tool.Handler(context.Background(), map[string]any{
		"script": `//@version=6
strategy("pine")
plot(close)`,
		"includeRequirements": false,
	})
	if err != nil {
		t.Fatalf("tool.Handler(plot warning): %v", err)
	}
	payload, ok = output.(map[string]any)
	if !ok {
		t.Fatalf("pine payload type = %T, want map", output)
	}
	if got := payload["ok"]; got != true {
		t.Fatalf("ok = %#v, want true", got)
	}
	warnings, ok := payload["warnings"].([]string)
	if !ok || len(warnings) == 0 || !strings.Contains(warnings[0], "visual-only call") {
		t.Fatalf("warnings = %#v, want visual-only warning", payload["warnings"])
	}
	if payload["saveHint"] != nil {
		t.Fatalf("saveHint = %#v, want nil on valid warning-only Pine", payload["saveHint"])
	}
	if payload["requirements"] != nil {
		t.Fatalf("requirements = %#v, want nil when includeRequirements=false", payload["requirements"])
	}

	output, err = tool.Handler(context.Background(), map[string]any{
		"script": `//@version=6
strategy("unsupported")
dailyClose = request.security("NASDAQ:AAPL", "1D", close)`,
		"includeRequirements": false,
	})
	if err != nil {
		t.Fatalf("tool.Handler(unsupported): %v", err)
	}
	payload, ok = output.(map[string]any)
	if !ok {
		t.Fatalf("unsupported payload type = %T, want map", output)
	}
	if got := payload["ok"]; got != false {
		t.Fatalf("ok = %#v, want false", got)
	}
	errors, ok := payload["errors"].([]string)
	if !ok || len(errors) == 0 || !strings.Contains(errors[0], "request.security") {
		t.Fatalf("errors = %#v, want request.security rejection", payload["errors"])
	}
}

func TestADKStrategySaveDefinitionToolCreateAndUpdate(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	tool, ok := server.adkRuntime.Tools().Get("strategy.save_definition")
	if !ok {
		t.Fatal("strategy.save_definition tool not registered")
	}
	if tool.Descriptor.Permission != "write_strategy" {
		t.Fatalf("descriptor permission = %q, want write_strategy", tool.Descriptor.Permission)
	}

	createOutput, err := tool.Handler(context.Background(), map[string]any{
		"name":        "ADK Saved Strategy",
		"description": "created via ADK tool",
		"symbol":      "US.TME",
		"interval":    "5m",
		"script": `//@version=6
strategy("ADK Saved Strategy", overlay=true)
log.info("created")`,
		"visualModel": map[string]any{
			"engine":  "logic-flow",
			"version": 1,
			"nodes":   []map[string]any{{"id": "n1", "type": "note"}},
		},
	})
	if err != nil {
		t.Fatalf("tool.Handler(create): %v", err)
	}
	createPayload, ok := createOutput.(map[string]any)
	if !ok {
		t.Fatalf("create output type = %T, want map", createOutput)
	}
	if got := createPayload["operation"]; got != "created" {
		t.Fatalf("operation = %#v, want created", got)
	}
	created, ok := createPayload["definition"].(strategyDesignDefinition)
	if !ok {
		t.Fatalf("definition payload = %T, want strategyDesignDefinition", createPayload["definition"])
	}
	if created.ID == "" {
		t.Fatalf("created definition id = empty")
	}
	if created.Symbol != "US.TME" || created.Interval != "5m" {
		t.Fatalf("created definition symbol/interval = %q/%q, want US.TME/5m", created.Symbol, created.Interval)
	}
	if created.VisualModel == nil || len(created.VisualModel.Nodes) != 1 {
		t.Fatalf("created visualModel = %#v, want populated model", created.VisualModel)
	}

	updateOutput, err := tool.Handler(context.Background(), map[string]any{
		"definitionId": created.ID,
		"name":         "ADK Saved Strategy Updated",
		"description":  "updated via ADK tool",
		"interval":     "15m",
		"script": `//@version=6
strategy("ADK Saved Strategy Updated", overlay=true)
alert("updated")`,
	})
	if err != nil {
		t.Fatalf("tool.Handler(update): %v", err)
	}
	updatePayload, ok := updateOutput.(map[string]any)
	if !ok {
		t.Fatalf("update output type = %T, want map", updateOutput)
	}
	if got := updatePayload["operation"]; got != "updated" {
		t.Fatalf("operation = %#v, want updated", got)
	}
	updated, ok := updatePayload["definition"].(strategyDesignDefinition)
	if !ok {
		t.Fatalf("updated definition payload = %T, want strategyDesignDefinition", updatePayload["definition"])
	}
	if updated.ID != created.ID {
		t.Fatalf("updated definition id = %q, want %q", updated.ID, created.ID)
	}
	if updated.Name != "ADK Saved Strategy Updated" || updated.Interval != "15m" {
		t.Fatalf("updated definition = %+v, want updated name/interval", updated)
	}

	if _, err := tool.Handler(context.Background(), map[string]any{
		"definitionId": "missing-definition",
		"name":         "Missing",
		"script":       strategypinespec.Skeleton(),
	}); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("tool.Handler(update missing) error = %v, want not found", err)
	}
}

func TestADKStrategyUpdateInstanceModeToolUpdatesStoppedInstance(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	definition, err := server.designStore.saveDefinition(strategyDesignDefinition{
		Name:         "Mode Test",
		Description:  "mode update test",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: "pine-v6",
		Symbol:       "US.TME",
		Interval:     "5m",
		Script: `//@version=6
strategy("Mode Test", overlay=true)
log.info("ready")`,
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeLive,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}

	tool, ok := server.adkRuntime.Tools().Get("strategy.update_instance_mode")
	if !ok {
		t.Fatal("strategy.update_instance_mode tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{
		"instanceId":    instance.ID,
		"executionMode": strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("tool.Handler(update mode): %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map", output)
	}
	updated, ok := payload["instance"].(strategyListItem)
	if !ok {
		t.Fatalf("instance payload = %T, want strategyListItem", payload["instance"])
	}
	if updated.Binding.ExecutionMode != strategyExecutionModeNotifyOnly {
		t.Fatalf("executionMode = %q, want %q", updated.Binding.ExecutionMode, strategyExecutionModeNotifyOnly)
	}
	updatedFields, ok := payload["updatedFields"].([]string)
	if !ok || len(updatedFields) != 1 || updatedFields[0] != "executionMode" {
		t.Fatalf("updatedFields = %#v, want [executionMode]", payload["updatedFields"])
	}

	stored, exists := server.strategyStore.strategy(instance.ID)
	if !exists {
		t.Fatalf("strategyStore.strategy(%q) not found after update", instance.ID)
	}
	stored.Status = strategyStatusRunning
	if err := server.strategyStore.saveStrategy(stored); err != nil {
		t.Fatalf("saveStrategy(running): %v", err)
	}
	if _, err := tool.Handler(context.Background(), map[string]any{
		"instanceId":    instance.ID,
		"executionMode": strategyExecutionModeLive,
	}); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("tool.Handler(non-stopped) error = %v, want %v", err, stratsrv.ErrBusy)
	}
}

func TestADKStrategyDefinitionsToolReturnsCompactSummaries(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	definition, err := server.designStore.saveDefinition(strategyDesignDefinition{
		Name:         "TME Demo",
		Version:      "0.1.1",
		Description:  "ADK summary test definition",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: "pine-v6",
		Symbol:       "US.TME",
		Interval:     "1d",
		Script: `//@version=6
strategy("TME Demo", overlay=true)
log.info("demo")`,
		VisualModel: &strategyVisualModel{
			Nodes: []strategyVisualNode{{ID: "n1"}, {ID: "n2"}},
			Edges: []strategyVisualEdge{{ID: "e1", SourceNodeID: "n1", TargetNodeID: "n2"}},
		},
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	_, err = server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "1d",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	tool, ok := server.adkRuntime.Tools().Get("strategy.definitions")
	if !ok {
		t.Fatal("strategy.definitions tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	definitions, ok := payload["definitions"].([]map[string]any)
	if !ok || len(definitions) == 0 {
		t.Fatalf("definitions payload = %#v, want non-empty []map[string]any", payload["definitions"])
	}
	definitionSummary := definitions[0]
	if _, exists := definitionSummary["script"]; exists {
		t.Fatalf("definition summary should not include raw script: %#v", definitionSummary)
	}
	if definitionSummary["scriptPreview"] == "" {
		t.Fatalf("definition summary missing scriptPreview: %#v", definitionSummary)
	}
	if got := definitionSummary["linkedInstanceCount"]; got != 1 {
		t.Fatalf("linkedInstanceCount = %#v, want 1", got)
	}
	instances, ok := payload["instances"].([]map[string]any)
	if !ok || len(instances) == 0 {
		t.Fatalf("instances payload = %#v, want non-empty []map[string]any", payload["instances"])
	}
	instanceSummary := instances[0]
	if _, exists := instanceSummary["logs"]; exists {
		t.Fatalf("instance summary should not include raw logs: %#v", instanceSummary)
	}
	if got := instanceSummary["definitionId"]; got != definition.ID {
		t.Fatalf("definitionId = %#v, want %q", got, definition.ID)
	}
}

func TestADKBacktestRunsToolReturnsSeriesCountsInsteadOfFullArrays(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	run := &backtestRunState{
		ID:     "bt-demo",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-demo",
			Market:         "US",
			Code:           "TME",
			Symbol:         "US.TME",
			Interval:       "1d",
			StartDate:      "2025-01-01",
			EndDate:        "2025-12-31",
			StartTime:      "2025-01-01T00:00:00Z",
			EndTime:        "2025-12-31T00:00:00Z",
			MarketTimezone: "America/New_York",
			InitialBalance: 100000,
			RehabType:      "forward",
		},
		Result: &backtest.RunResult{
			QuoteCurrency:   "USD",
			FinalBalance:    101500,
			PnL:             1500,
			MaxDrawdown:     0.12,
			CurrentDrawdown: 0.02,
			TotalTrades:     3,
			WinRate:         0.66,
			Trades:          []backtest.TradeEvent{{Time: "2025-03-01T00:00:00Z", Side: "BUY", Price: "10", Qty: "1"}},
			OrderBook:       []backtest.OrderBookEntry{{OrderID: "o1", Symbol: "US.TME", Side: "BUY", Quantity: "1", Status: "FILLED"}},
			PnLCurve:        []backtest.PnLPoint{{Time: "2025-03-01T00:00:00Z", Equity: 100500}},
			DrawdownCurve:   []backtest.DrawdownPoint{{Time: "2025-03-01T00:00:00Z", Drawdown: 0.03}},
			Candles:         []backtest.Candle{{Time: "2025-03-01T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100"}},
			Logs:            []string{"line 1", "line 2"},
			RuntimeErrors:   []string{"warn"},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("backtestRuns.add: %v", err)
	}

	tool, ok := server.adkRuntime.Tools().Get("backtest.runs")
	if !ok {
		t.Fatal("backtest.runs tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	runs, ok := payload["runs"].([]map[string]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs payload = %#v, want single summarized run", payload["runs"])
	}
	summary := runs[0]
	if _, exists := summary["result"]; exists {
		t.Fatalf("backtest summary should not include raw result: %#v", summary)
	}
	if got := summary["candlesCount"]; got != 1 {
		t.Fatalf("candlesCount = %#v, want 1", got)
	}
	if got := summary["tradeCount"]; got != 3 {
		t.Fatalf("tradeCount = %#v, want 3", got)
	}
	if got := summary["latestLog"]; got != "line 2" {
		t.Fatalf("latestLog = %#v, want line 2", got)
	}
	if summary["startDate"] != "2025-01-01" || summary["endDate"] != "2025-12-31" || summary["marketTimezone"] != "America/New_York" {
		t.Fatalf("market date metadata was not summarized: %#v", summary)
	}
}

func TestADKStrategyResearchBacktestToolStartsTransientRun(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	tool, ok := server.adkRuntime.Tools().Get("strategy.research_backtest")
	if !ok {
		t.Fatal("strategy.research_backtest tool not registered")
	}
	if tool.Descriptor.Permission != "optimize_strategy" {
		t.Fatalf("descriptor permission = %q, want optimize_strategy", tool.Descriptor.Permission)
	}
	before := len(server.designStore.listDefinitions())
	output, err := tool.Handler(context.Background(), map[string]any{
		"script":     strategypinespec.Skeleton(),
		"market":     "US",
		"symbol":     "US.TME",
		"interval":   "1m",
		"startDate":  "2025-01-01",
		"endDate":    "2025-01-02",
		"resultView": map[string]any{"view": "summary"},
	})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	if got := payload["ok"]; got != true {
		t.Fatalf("ok = %#v, want true", got)
	}
	runID := jftradeCheckedTypeAssertion[string](payload["runId"])
	if !strings.HasPrefix(runID, "bt-") {
		t.Fatalf("runId = %#v, want bt- prefix", payload["runId"])
	}
	if after := len(server.designStore.listDefinitions()); after != before {
		t.Fatalf("strategy definitions count = %d, want unchanged %d", after, before)
	}
	status, ok := server.backtestRuns.get(runID)
	if !ok {
		t.Fatalf("transient run %q not found", runID)
	}
	if !strings.HasPrefix(status.Request.DefinitionID, "adk-research-") {
		t.Fatalf("definitionId = %q, want transient research id", status.Request.DefinitionID)
	}
	if strings.Contains(status.Request.DefinitionID, "Minimal Draft") {
		t.Fatalf("definitionId leaked strategy name: %q", status.Request.DefinitionID)
	}
	if status.Request.StartDate != "2025-01-01" || status.Request.EndDate != "2025-01-02" || status.Request.MarketTimezone != "America/New_York" {
		t.Fatalf("research backtest market date metadata = %+v", status.Request)
	}
}

func TestADKBacktestResultViewToolReturnsBoundedChartWindow(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	run := &backtestRunState{
		ID:     "bt-view",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-demo",
			Market:         "US",
			Code:           "TME",
			Symbol:         "US.TME",
			Interval:       "1m",
			StartTime:      "2025-01-01T00:00:00Z",
			EndTime:        "2025-01-01T00:10:00Z",
			InitialBalance: 100000,
			RehabType:      "forward",
		},
		Result: &backtest.RunResult{
			QuoteCurrency: "USD",
			FinalBalance:  100200,
			PnL:           200,
			Candles: []backtest.Candle{
				{Time: "2025-01-01T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100"},
				{Time: "2025-01-01T00:01:00Z", Open: "10.5", High: "12", Low: "10", Close: "11.5", Volume: "200"},
				{Time: "2025-01-01T00:02:00Z", Open: "11.5", High: "13", Low: "11", Close: "12.5", Volume: "300"},
			},
			Trades: []backtest.TradeEvent{{Time: "2025-01-01T00:01:00Z", Side: "BUY", Price: "11", Qty: "1"}},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:10:00Z",
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("backtestRuns.add: %v", err)
	}

	tool, ok := server.adkRuntime.Tools().Get("backtest.result_view")
	if !ok {
		t.Fatal("backtest.result_view tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{
		"runId":      "bt-view",
		"view":       "chart",
		"resolution": "2m",
		"include":    []any{"candles", "trades"},
		"limit":      float64(1),
	})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	window := jftradeCheckedTypeAssertion[map[string]any](payload["window"])
	if window["resolution"] != "2m" || window["truncated"] != true || window["nextCursor"] != "1" {
		t.Fatalf("window = %#v, want 2m truncated next cursor", window)
	}
	series := jftradeCheckedTypeAssertion[map[string]any](payload["series"])
	candles := jftradeCheckedTypeAssertion[[]backtest.Candle](series["candles"])
	if len(candles) != 1 || candles[0].Open != "10" || candles[0].High != "12" || candles[0].Low != "9" || candles[0].Close != "11.5" {
		t.Fatalf("candles = %#v, want first 2m aggregate", candles)
	}
	trades := jftradeCheckedTypeAssertion[[]backtest.TradeEvent](series["trades"])
	if len(trades) != 1 || trades[0].Side != "BUY" {
		t.Fatalf("trades = %#v, want bounded BUY trade", trades)
	}
}
