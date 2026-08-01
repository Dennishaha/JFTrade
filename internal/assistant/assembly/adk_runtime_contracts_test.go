package assembly

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestADKRuntimeStrategyToolsPreserveOwnerContracts(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	var savedDraft StrategyDraftInput
	var savedDefinition StrategyDefinitionInput
	var updatedInstance string
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		SaveStrategyDraft: func(input StrategyDraftInput) (any, error) {
			savedDraft = input
			return map[string]any{"id": "draft-1"}, nil
		},
		SaveStrategyDefinition: func(input StrategyDefinitionInput) (any, error) {
			savedDefinition = input
			return map[string]any{"id": "definition-1", "name": input.Name}, nil
		},
		UpdateStrategyInstanceMode: func(instanceID, mode string) (any, error) {
			updatedInstance = instanceID + ":" + mode
			return map[string]any{"id": instanceID, "executionMode": mode}, nil
		},
		ListStrategyDefinitions: func() ([]StrategyDefinitionSummary, error) {
			return []StrategyDefinitionSummary{{ID: "definition-1", Name: "Saved", Script: strategypinespec.Skeleton()}}, nil
		},
		ListStrategyInstances: func() []StrategyInstanceSummary {
			return []StrategyInstanceSummary{{ID: "instance-1", DefinitionID: "definition-1", Status: "STOPPED"}}
		},
		ListBacktestRuns: func() []BacktestRunSummary {
			return []BacktestRunSummary{{ID: "run-1", DefinitionID: "definition-1", Status: "completed"}}
		},
		BacktestResultView: func(input BacktestResultViewInput) (any, error) {
			return map[string]any{"runId": input.RunID, "view": input.View}, nil
		},
	})

	validationTool, ok := registry.Get("strategy.validate_pine")
	if !ok {
		t.Fatal("strategy.validate_pine is not registered")
	}
	validation, err := validationTool.Handler(t.Context(), map[string]any{"script": strategypinespec.Skeleton()})
	if err != nil {
		t.Fatalf("strategy.validate_pine: %v", err)
	}
	validationPayload := validation.(map[string]any)
	if validationPayload["ok"] != true || strings.TrimSpace(validationPayload["normalizedScript"].(string)) == "" {
		t.Fatalf("validation payload = %#v", validationPayload)
	}

	draftTool, _ := registry.Get("strategy.save_draft")
	if _, err := draftTool.Handler(t.Context(), map[string]any{"script": strategypinespec.Skeleton()}); err != nil {
		t.Fatalf("strategy.save_draft: %v", err)
	}
	if savedDraft.Validation.Program == nil || savedDraft.Validation.NormalizedScript == "" {
		t.Fatalf("saved draft validation = %#v", savedDraft.Validation)
	}

	definitionTool, _ := registry.Get("strategy.save_definition")
	created, err := definitionTool.Handler(t.Context(), map[string]any{
		"name": "Saved strategy", "script": strategypinespec.Skeleton(), "symbol": "US.AAPL", "interval": "1d",
		"visualModel": map[string]any{"nodes": []map[string]any{{"id": "node-1", "type": "note"}}},
	})
	if err != nil {
		t.Fatalf("strategy.save_definition: %v", err)
	}
	createdPayload := created.(map[string]any)
	if createdPayload["operation"] != "created" || savedDefinition.VisualModel == nil {
		t.Fatalf("created definition payload = %#v input=%#v", createdPayload, savedDefinition)
	}

	modeTool, _ := registry.Get("strategy.update_instance_mode")
	modeResult, err := modeTool.Handler(t.Context(), map[string]any{"instanceId": "instance-1", "executionMode": "notify_only"})
	if err != nil {
		t.Fatalf("strategy.update_instance_mode: %v", err)
	}
	if updatedInstance != "instance-1:notify_only" || modeResult.(map[string]any)["updatedFields"].([]string)[0] != "executionMode" {
		t.Fatalf("mode result = %#v, updated=%q", modeResult, updatedInstance)
	}

	definitionsTool, _ := registry.Get("strategy.definitions")
	definitions, err := definitionsTool.Handler(t.Context(), nil)
	if err != nil {
		t.Fatalf("strategy.definitions: %v", err)
	}
	if definitions.(map[string]any)["definitionCount"] != 1 {
		t.Fatalf("definitions = %#v", definitions)
	}

	runsTool, _ := registry.Get("backtest.runs")
	runs, err := runsTool.Handler(t.Context(), map[string]any{"definitionId": "definition-1", "status": "completed"})
	if err != nil {
		t.Fatalf("backtest.runs: %v", err)
	}
	if runs.(map[string]any)["totalMatched"] != 1 {
		t.Fatalf("filtered runs = %#v", runs)
	}

	viewTool, _ := registry.Get("backtest.result_view")
	view, err := viewTool.Handler(t.Context(), map[string]any{"runId": "run-1", "view": "summary"})
	if err != nil || view.(map[string]any)["runId"] != "run-1" {
		t.Fatalf("backtest.result_view = %#v, err=%v", view, err)
	}
}

func TestADKRuntimeOptimizationPersistsQueuedRunReferences(t *testing.T) {
	dir := t.TempDir()
	store, err := assistanttestkit.NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := assistanttestkit.NewToolRegistry()
	cancelled := []string{}
	RegisterStrategyOptimizationTools(store, registry, ToolDeps{
		EnsureBacktestData: func(ids []string, _ BacktestStartInput) (BacktestDataReadiness, error) {
			if len(ids) != 2 || ids[0] != "def-a" || ids[1] != "def-b" {
				t.Fatalf("definition IDs = %#v", ids)
			}
			return BacktestDataReadiness{Ready: true, Status: "ready"}, nil
		},
		EnqueueBacktest: func(input BacktestStartInput) (BacktestRunRef, error) {
			return BacktestRunRef{ID: "run-" + input.DefinitionID, Status: "queued"}, nil
		},
		CancelBacktest: func(id string) { cancelled = append(cancelled, id) },
	})
	tool, ok := registry.Get("strategy.optimize")
	if !ok {
		t.Fatal("strategy.optimize is not registered")
	}
	output, err := tool.Handler(t.Context(), map[string]any{
		"definitionIds": []any{"def-a", "def-b"}, "market": "US", "symbol": "US.AAPL",
		"startDate": "2025-01-01", "endDate": "2025-01-02", "objective": "return",
	})
	if err != nil {
		t.Fatalf("strategy.optimize: %v", err)
	}
	payload := output.(map[string]any)
	if payload["status"] != "queued" || len(payload["runs"].([]map[string]any)) != 2 || len(cancelled) != 0 {
		t.Fatalf("optimization payload = %#v cancelled=%#v", payload, cancelled)
	}
	taskID := payload["taskId"].(string)
	task, ok, err := store.OptimizationTask(t.Context(), taskID)
	if err != nil || !ok || len(task.Runs) != 2 {
		t.Fatalf("persisted optimization task = %#v ok=%v err=%v", task, ok, err)
	}
}

func TestADKRuntimeResearchToolStopsBeforeRunWhenDataSyncIsPending(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	started := false
	RegisterStrategyResearchTools(registry, ToolDeps{
		EnsureResearchBacktestData: func(ResearchBacktestInput) (BacktestDataReadiness, error) {
			return BacktestDataReadiness{Status: "syncing_data", DataSync: &BacktestDataSync{TaskID: "sync-1", Status: "running"}}, nil
		},
		StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
			started = true
			return BacktestRunSummary{ID: "unexpected"}, nil
		},
	})
	tool, _ := registry.Get("strategy.research_backtest")
	output, err := tool.Handler(context.Background(), map[string]any{"script": strategypinespec.Skeleton(), "market": "US", "symbol": "US.AAPL"})
	if err != nil {
		t.Fatalf("strategy.research_backtest: %v", err)
	}
	payload := output.(map[string]any)
	if started || payload["status"] != "syncing_data" || payload["runId"] != nil {
		t.Fatalf("pending sync payload = %#v started=%v", payload, started)
	}
}
