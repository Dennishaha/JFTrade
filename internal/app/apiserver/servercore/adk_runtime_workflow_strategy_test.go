package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestADKCoreToolHandlersSurfaceSubscriptionErrors(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		MarketSubscriptions: func(context.Context) (any, any, error) {
			return nil, nil, errors.New("feed unavailable")
		},
	})
	tool, _ := registry.Get("market.subscriptions")
	if _, err := tool.Handler(context.Background(), map[string]any{}); err == nil || err.Error() != "feed unavailable" {
		t.Fatalf("market.subscriptions error = %v, want propagated feed unavailable", err)
	}
}

func TestADKWorkflowAuditAndAdapterHelpers(t *testing.T) {
	var recorded struct {
		kind     string
		subject  string
		detail   string
		metadata map[string]any
	}
	recordADKWorkflowAudit(context.Background(), ToolDeps{
		RecordAudit: func(_ context.Context, kind, subjectID, detail string, metadata map[string]any) {
			recorded.kind = kind
			recorded.subject = subjectID
			recorded.detail = detail
			recorded.metadata = metadata
		},
	}, "task.saved", "task-1", "saved", map[string]any{"status": "todo"})
	if recorded.kind != "task.saved" || recorded.subject != "task-1" || recorded.detail != "saved" || recorded.metadata["status"] != "todo" {
		t.Fatalf("recordADKWorkflowAudit() recorded = %#v, want saved audit payload", recorded)
	}
	recordADKWorkflowAudit(context.Background(), ToolDeps{}, "noop", "noop", "noop", nil)

	query := brokerReadQueryFromADK(&trdsrv.Service{}, BrokerReadInput{
		TradingEnvironment: "SIMULATE",
		AccountID:          "acct-1",
		Market:             "",
	})
	if query.BrokerID != "futu" || query.TradingEnvironment != "SIMULATE" || query.AccountID != "acct-1" || query.Market != "HK" {
		t.Fatalf("brokerReadQueryFromADK() = %#v, want futu/HK query", query)
	}

	if scope, err := normalizeTradingBrokerScope(" history "); err != nil || scope != "HISTORY" {
		t.Fatalf("normalizeTradingBrokerScope(history) = %q %v, want HISTORY nil", scope, err)
	}
	if scope, err := normalizeTradingBrokerScope(""); err != nil || scope != "CURRENT" {
		t.Fatalf("normalizeTradingBrokerScope(empty) = %q %v, want CURRENT nil", scope, err)
	}
	if _, err := normalizeTradingBrokerScope("archive"); err == nil {
		t.Fatal("normalizeTradingBrokerScope(invalid) error = nil, want validation error")
	}

	merged := mergeADKBrokerValues([]string{" submitted , filled", "FILLED"}, []string{"cancelled", "Submitted"})
	if len(merged) != 3 || merged[0] != "submitted" || merged[1] != "filled" || merged[2] != "cancelled" {
		t.Fatalf("mergeADKBrokerValues() = %#v, want deduplicated broker values", merged)
	}
}

func TestADKSystemAndWorkflowToolHandlersReflectBusinessState(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	store, err := jfadk.NewStore(
		filepath.Join(t.TempDir(), "adk.db"),
		filepath.Join(t.TempDir(), "secrets"),
		filepath.Join(t.TempDir(), "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	var auditKinds []string
	RegisterJFTradeADKTools(store, registry, ToolDeps{
		SystemStatus: func() map[string]any { return map[string]any{"api": "ok"} },
		ADKEnabled:   func() bool { return true },
		RecordAudit: func(_ context.Context, kind, subjectID, detail string, metadata map[string]any) {
			auditKinds = append(auditKinds, kind)
		},
	})

	systemTool, _ := registry.Get("system.status")
	systemOutput, err := systemTool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("system.status Handler: %v", err)
	}
	systemPayload := systemOutput.(map[string]any)
	adkPayload, ok := systemPayload["adk"].(map[string]any)
	if !ok || adkPayload["enabled"] != true {
		t.Fatalf("system.status payload = %#v, want ADK enabled block", systemPayload)
	}

	createTool, _ := registry.Get("tasks.create")
	createOutput, err := createTool.Handler(context.Background(), map[string]any{
		"title":       "Review market-open checklist",
		"description": "Before US premarket starts, confirm data feed health.",
		"status":      "todo",
	})
	if err != nil {
		t.Fatalf("tasks.create Handler: %v", err)
	}
	createdTask := createOutput.(jfadk.Task)
	if createdTask.Title != "Review market-open checklist" || createdTask.Status != "TODO" {
		t.Fatalf("createdTask = %#v, want saved task with normalized TODO status", createdTask)
	}

	updateTool, _ := registry.Get("tasks.update")
	updateOutput, err := updateTool.Handler(context.Background(), map[string]any{
		"id":     createdTask.ID,
		"status": "done",
	})
	if err != nil {
		t.Fatalf("tasks.update Handler: %v", err)
	}
	updatedTask := updateOutput.(jfadk.Task)
	if updatedTask.Status != "DONE" {
		t.Fatalf("updatedTask = %#v, want normalized DONE status", updatedTask)
	}

	listTool, _ := registry.Get("tasks.list")
	listOutput, err := listTool.Handler(context.Background(), map[string]any{"limit": 10, "offset": 0})
	if err != nil {
		t.Fatalf("tasks.list Handler: %v", err)
	}
	listPayload := listOutput.(map[string]any)
	page := listPayload["page"].(map[string]any)
	if page["returned"] != 1 || page["hasMore"] != false {
		t.Fatalf("tasks.list page = %#v, want single task page", page)
	}

	deleteTool, _ := registry.Get("tasks.delete")
	deleteOutput, err := deleteTool.Handler(context.Background(), map[string]any{"id": createdTask.ID})
	if err != nil {
		t.Fatalf("tasks.delete Handler: %v", err)
	}
	if deleteOutput.(map[string]any)["deleted"] != true {
		t.Fatalf("tasks.delete output = %#v, want deleted=true", deleteOutput)
	}

	memoryRememberTool, _ := registry.Get("memory.remember")
	memoryOutput, err := memoryRememberTool.Handler(context.Background(), map[string]any{
		"scope": "workspace",
		"key":   "preferred_market",
		"value": "US",
	})
	if err != nil {
		t.Fatalf("memory.remember Handler: %v", err)
	}
	entry := memoryOutput.(jfadk.MemoryEntry)
	if entry.Key != "preferred_market" || entry.Value != "US" {
		t.Fatalf("memory entry = %#v, want remembered workspace preference", entry)
	}

	memoryListTool, _ := registry.Get("memory.list")
	memoryListOutput, err := memoryListTool.Handler(context.Background(), map[string]any{"scope": "workspace"})
	if err != nil {
		t.Fatalf("memory.list Handler: %v", err)
	}
	entries := memoryListOutput.(map[string]any)["entries"].([]jfadk.MemoryEntry)
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("memory.list entries = %#v, want remembered entry", entries)
	}

	memoryForgetTool, _ := registry.Get("memory.forget")
	if _, err := memoryForgetTool.Handler(context.Background(), map[string]any{"id": entry.ID}); err != nil {
		t.Fatalf("memory.forget Handler: %v", err)
	}

	if len(auditKinds) < 4 || auditKinds[0] != "task.saved" || auditKinds[len(auditKinds)-1] != "memory.deleted" {
		t.Fatalf("auditKinds = %#v, want task/memory audit lifecycle", auditKinds)
	}
}

func TestADKStrategyToolsHandleNegativeAndFallbackScenarios(t *testing.T) {
	store, err := jfadk.NewStore(
		filepath.Join(t.TempDir(), "adk.db"),
		filepath.Join(t.TempDir(), "secrets"),
		filepath.Join(t.TempDir(), "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	registry := jfadk.NewToolRegistry()
	var savedDraft StrategyDraftInput
	viewCalls := 0
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{
		ListStrategyDefinitions: func() []StrategyDefinitionSummary {
			return []StrategyDefinitionSummary{{ID: "def-1", Name: "Mean Revert"}}
		},
		ListStrategyInstances: func() []StrategyInstanceSummary {
			return []StrategyInstanceSummary{{ID: "inst-1", DefinitionID: "def-1", ExecutionMode: "live"}}
		},
		SaveStrategyDraft: func(input StrategyDraftInput) (any, error) {
			savedDraft = input
			return map[string]any{"id": "draft-1"}, nil
		},
		StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
			return BacktestRunSummary{ID: "bt-research", Status: "queued"}, nil
		},
		BacktestResultView: func(BacktestResultViewInput) (any, error) {
			viewCalls++
			if viewCalls == 1 {
				return map[string]any{"run": map[string]any{"status": "completed"}}, nil
			}
			return nil, errors.New("view unavailable")
		},
		BacktestKLineSyncProgress: func(string) (*backtest.SyncProgress, bool) {
			return nil, false
		},
	})

	definitionsTool, _ := registry.Get("strategy.definitions")
	definitionsOutput, err := definitionsTool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("strategy.definitions Handler: %v", err)
	}
	definitionsPayload := definitionsOutput.(map[string]any)
	if definitionsPayload["definitionCount"] != 1 || definitionsPayload["instanceCount"] != 1 {
		t.Fatalf("strategy.definitions payload = %#v, want summarized counts", definitionsPayload)
	}

	saveDraftTool, _ := registry.Get("strategy.save_draft")
	if _, err := saveDraftTool.Handler(context.Background(), map[string]any{"name": "Implicit Skeleton Draft"}); err != nil {
		t.Fatalf("strategy.save_draft Handler: %v", err)
	}
	if strings.TrimSpace(savedDraft.Script) == "" || !strings.Contains(savedDraft.Script, "strategy(") {
		t.Fatalf("savedDraft = %#v, want skeleton script fallback", savedDraft)
	}
	if _, err := saveDraftTool.Handler(context.Background(), map[string]any{"script": `strategy("broken")
fast =`}); err == nil || !strings.Contains(err.Error(), "Pine Script v6") {
		t.Fatalf("strategy.save_draft invalid script error = %v, want Pine validation failure", err)
	}

	researchTool, _ := registry.Get("strategy.research_backtest")
	researchOutput, err := researchTool.Handler(context.Background(), map[string]any{
		"script":              strategypinespec.Skeleton(),
		"market":              "US",
		"symbol":              "US.AAPL",
		"waitForCompletionMs": 30000,
		"resultView":          map[string]any{"view": "summary"},
	})
	if err != nil {
		t.Fatalf("strategy.research_backtest Handler: %v", err)
	}
	researchPayload := researchOutput.(map[string]any)
	if researchPayload["status"] != "completed" || researchPayload["resultViewError"] != "view unavailable" {
		t.Fatalf("research payload = %#v, want completed status plus result view error", researchPayload)
	}

	saveDefinitionTool, _ := registry.Get("strategy.save_definition")
	if _, err := saveDefinitionTool.Handler(context.Background(), map[string]any{
		"script": strategypinespec.Skeleton(),
	}); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("strategy.save_definition missing name error = %v, want name validation", err)
	}

	updateModeTool, _ := registry.Get("strategy.update_instance_mode")
	if _, err := updateModeTool.Handler(context.Background(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "instanceId") {
		t.Fatalf("strategy.update_instance_mode missing instanceId error = %v, want validation", err)
	}
	if _, err := updateModeTool.Handler(context.Background(), map[string]any{
		"instanceId":    "inst-1",
		"executionMode": "paper",
	}); err == nil || !strings.Contains(err.Error(), "executionMode") {
		t.Fatalf("strategy.update_instance_mode invalid mode error = %v, want validation", err)
	}

	resultViewTool, _ := registry.Get("backtest.result_view")
	if _, err := resultViewTool.Handler(context.Background(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "runId") {
		t.Fatalf("backtest.result_view missing runId error = %v, want runId validation", err)
	}

	klineStatusTool, _ := registry.Get("backtest.kline_sync_status")
	if _, err := klineStatusTool.Handler(context.Background(), map[string]any{}); err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Fatalf("backtest.kline_sync_status missing taskId error = %v, want taskId validation", err)
	}
	if _, err := klineStatusTool.Handler(context.Background(), map[string]any{"taskId": "sync-missing"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("backtest.kline_sync_status missing task error = %v, want not found", err)
	}
}

func TestADKStrategyToolContractsCoverUnavailableAndSuccessfulViewScenarios(t *testing.T) {
	store, err := jfadk.NewStore(
		filepath.Join(t.TempDir(), "adk.db"),
		filepath.Join(t.TempDir(), "secrets"),
		filepath.Join(t.TempDir(), "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	registry := jfadk.NewToolRegistry()
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{})

	researchTool, _ := registry.Get("strategy.research_backtest")
	if _, err := researchTool.Handler(context.Background(), map[string]any{"script": strategypinespec.Skeleton()}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("strategy.research_backtest unavailable error = %v, want unavailable", err)
	}

	resultViewTool, _ := registry.Get("backtest.result_view")
	if _, err := resultViewTool.Handler(context.Background(), map[string]any{"runId": "bt-1"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("backtest.result_view unavailable error = %v, want unavailable", err)
	}

	klineStatusTool, _ := registry.Get("backtest.kline_sync_status")
	if _, err := klineStatusTool.Handler(context.Background(), map[string]any{"taskId": "sync-1"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("backtest.kline_sync_status unavailable error = %v, want unavailable", err)
	}

	registry = jfadk.NewToolRegistry()
	var researchViewInput BacktestResultViewInput
	var resultViewInput BacktestResultViewInput
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{
		EnsureResearchBacktestData: func(ResearchBacktestInput) (BacktestDataReadiness, error) {
			return BacktestDataReadiness{Status: "ready", Ready: true}, nil
		},
		StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
			return BacktestRunSummary{ID: "bt-success", Status: "queued"}, nil
		},
		BacktestResultView: func(input BacktestResultViewInput) (any, error) {
			if input.RunID == "bt-success" {
				researchViewInput = input
				return map[string]any{"run": map[string]any{"status": "running"}, "view": input.View}, nil
			}
			resultViewInput = input
			return map[string]any{"run": map[string]any{"status": "completed"}, "window": map[string]any{"limit": input.Limit}}, nil
		},
		BacktestKLineSyncProgress: func(taskID string) (*backtest.SyncProgress, bool) {
			return &backtest.SyncProgress{TaskID: taskID, Status: "completed", Symbol: "US.AAPL"}, taskID == "sync-ready"
		},
	})

	researchTool, _ = registry.Get("strategy.research_backtest")
	researchOutput, err := researchTool.Handler(context.Background(), map[string]any{
		"script":     strategypinespec.Skeleton(),
		"market":     "US",
		"symbol":     "US.AAPL",
		"resultView": map[string]any{"view": "orders", "limit": 2, "include": "trades"},
	})
	if err != nil {
		t.Fatalf("strategy.research_backtest success: %v", err)
	}
	researchPayload := researchOutput.(map[string]any)
	if researchPayload["status"] != "running" || researchPayload["runId"] != "bt-success" {
		t.Fatalf("research payload = %#v, want running bt-success", researchPayload)
	}
	if _, ok := researchPayload["resultView"].(map[string]any); !ok {
		t.Fatalf("research resultView = %#v, want populated result view", researchPayload["resultView"])
	}
	if researchViewInput.RunID != "bt-success" || researchViewInput.View != "orders" || researchViewInput.Limit != 2 || len(researchViewInput.Include) != 1 || researchViewInput.Include[0] != "trades" {
		t.Fatalf("research view input = %#v, want normalized nested result view input", researchViewInput)
	}
	researchOutput, err = researchTool.Handler(context.Background(), map[string]any{
		"script": strategypinespec.Skeleton(),
		"market": "US",
		"symbol": "US.AAPL",
	})
	if err != nil {
		t.Fatalf("strategy.research_backtest default result view: %v", err)
	}
	researchPayload = researchOutput.(map[string]any)
	if researchPayload["status"] != "running" || researchViewInput.View != "summary" {
		t.Fatalf("research payload=%#v researchViewInput=%#v, want default summary result view", researchPayload, researchViewInput)
	}

	resultViewTool, _ = registry.Get("backtest.result_view")
	resultOutput, err := resultViewTool.Handler(context.Background(), map[string]any{
		"runId":      "bt-direct",
		"view":       "chart",
		"resolution": "5m",
		"include":    "candles,trades",
		"limit":      3,
	})
	if err != nil {
		t.Fatalf("backtest.result_view success: %v", err)
	}
	resultPayload := resultOutput.(map[string]any)
	if resultPayload["window"] == nil {
		t.Fatalf("result payload = %#v, want passthrough payload", resultPayload)
	}
	if resultViewInput.RunID != "bt-direct" || resultViewInput.View != "chart" || resultViewInput.Resolution != "5m" || resultViewInput.Limit != 3 || len(resultViewInput.Include) != 2 {
		t.Fatalf("result view input = %#v, want normalized direct result view input", resultViewInput)
	}

	klineStatusTool, _ = registry.Get("backtest.kline_sync_status")
	statusOutput, err := klineStatusTool.Handler(context.Background(), map[string]any{"taskId": "sync-ready", "waitForCompletionMs": 99999})
	if err != nil {
		t.Fatalf("backtest.kline_sync_status success: %v", err)
	}
	statusPayload := statusOutput.(map[string]any)
	if statusPayload["status"] != "completed" || statusPayload["taskId"] != "sync-ready" || statusPayload["readyToRetry"] != true {
		t.Fatalf("kline status payload = %#v, want completed retry-ready payload", statusPayload)
	}

	saveDefinitionTool, _ := registry.Get("strategy.save_definition")
	if _, err := saveDefinitionTool.Handler(context.Background(), map[string]any{
		"name": "Broken Strategy",
		"script": `strategy("broken")
fast =`,
	}); err == nil || !strings.Contains(err.Error(), "Pine Script v6") {
		t.Fatalf("strategy.save_definition invalid script error = %v, want Pine validation failure", err)
	}

	var optimized []string
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{
		EnsureBacktestData: func(ids []string, _ BacktestStartInput) (BacktestDataReadiness, error) {
			return BacktestDataReadiness{Status: "ready", Ready: true}, nil
		},
		EnqueueBacktest: func(input BacktestStartInput) (BacktestRunRef, error) {
			optimized = append(optimized, input.DefinitionID)
			return BacktestRunRef{ID: "run-" + input.DefinitionID, Status: "queued"}, nil
		},
		CancelBacktest: func(string) {},
	})
	optimizeTool, _ := registry.Get("strategy.optimize")
	if _, err := optimizeTool.Handler(context.Background(), map[string]any{"market": "US", "symbol": "US.AAPL"}); err == nil || !strings.Contains(err.Error(), "definitionIds is required") {
		t.Fatalf("strategy.optimize missing definitions error = %v, want validation", err)
	}
	optimizeOutput, err := optimizeTool.Handler(context.Background(), map[string]any{
		"definitionId": "def-solo",
		"market":       "US",
		"symbol":       "US.AAPL",
	})
	if err != nil {
		t.Fatalf("strategy.optimize single definitionId: %v", err)
	}
	optimizePayload := optimizeOutput.(map[string]any)
	if optimizePayload["status"] != "queued" || len(optimized) != 1 || optimized[0] != "def-solo" {
		t.Fatalf("optimize payload=%#v optimized=%#v, want fallback single definition candidate", optimizePayload, optimized)
	}
}

func TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure(t *testing.T) {
	store, err := jfadk.NewStore(
		filepath.Join(t.TempDir(), "adk.db"),
		filepath.Join(t.TempDir(), "secrets"),
		filepath.Join(t.TempDir(), "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("store.Close: %v", closeErr)
		}
	})

	registry := jfadk.NewToolRegistry()
	var enqueued []string
	var cancelled []string
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{
		EnsureBacktestData: func(ids []string, _ BacktestStartInput) (BacktestDataReadiness, error) {
			return BacktestDataReadiness{Status: "ready", Ready: true}, nil
		},
		EnqueueBacktest: func(input BacktestStartInput) (BacktestRunRef, error) {
			enqueued = append(enqueued, input.DefinitionID)
			return BacktestRunRef{ID: "run-" + input.DefinitionID, Status: "queued"}, nil
		},
		CancelBacktest: func(runID string) {
			cancelled = append(cancelled, runID)
		},
	})

	optimizeTool, _ := registry.Get("strategy.optimize")
	output, err := optimizeTool.Handler(context.Background(), map[string]any{
		"definitionIds": []any{"def-a", "def-b"},
		"market":        "US",
		"symbol":        "US.AAPL",
		"objective":     "sharpe",
	})
	if err != nil {
		t.Fatalf("strategy.optimize Handler: %v", err)
	}
	payload := output.(map[string]any)
	if payload["status"] != "queued" || payload["objective"] != "sharpe" {
		t.Fatalf("strategy.optimize payload = %#v, want queued persisted task", payload)
	}
	runs := payload["runs"].([]map[string]any)
	if len(runs) != 2 || runs[0]["runId"] != "run-def-a" {
		t.Fatalf("strategy.optimize runs = %#v, want queued run refs", runs)
	}
	tasks, err := store.ListOptimizationTasks(context.Background())
	if err != nil {
		t.Fatalf("ListOptimizationTasks: %v", err)
	}
	if len(tasks) != 1 || len(tasks[0].Runs) != 2 {
		t.Fatalf("optimization tasks = %#v, want persisted queued task", tasks)
	}
	if len(cancelled) != 0 {
		t.Fatalf("cancelled = %#v, want no cancellation on success", cancelled)
	}

	registry = jfadk.NewToolRegistry()
	enqueued = nil
	cancelled = nil
	registerJFTradeADKStrategyTools(store, registry, ToolDeps{
		EnqueueBacktest: func(input BacktestStartInput) (BacktestRunRef, error) {
			enqueued = append(enqueued, input.DefinitionID)
			if input.DefinitionID == "def-b" {
				return BacktestRunRef{}, errors.New("queue down")
			}
			return BacktestRunRef{ID: "run-" + input.DefinitionID, Status: "queued"}, nil
		},
		CancelBacktest: func(runID string) {
			cancelled = append(cancelled, runID)
		},
	})
	optimizeTool, _ = registry.Get("strategy.optimize")
	if _, err := optimizeTool.Handler(context.Background(), map[string]any{
		"definitionIds": []any{"def-a", "def-b"},
		"market":        "US",
		"symbol":        "US.AAPL",
	}); err == nil || !strings.Contains(err.Error(), "queue candidate") {
		t.Fatalf("strategy.optimize queue failure error = %v, want wrapped queue failure", err)
	}
	if len(enqueued) != 2 || len(cancelled) != 1 || cancelled[0] != "run-def-a" {
		t.Fatalf("enqueued=%#v cancelled=%#v, want prior queued run cancelled on failure", enqueued, cancelled)
	}

	if _, err := optimizeTool.Handler(context.Background(), map[string]any{
		"definitionIds": []any{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"},
	}); err == nil || !strings.Contains(err.Error(), "at most 12") {
		t.Fatalf("strategy.optimize candidate limit error = %v, want max candidate validation", err)
	}
}
