package servercore

import (
	"path/filepath"
	"testing"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestRunStoreAdapterRoundTripsAndDelegatesLifecycle(t *testing.T) {
	store := newBacktestRunStore()
	adapter := &backtestRunStoreAdapter{store: store}
	useExtendedHours := true
	run := &btsrv.RunState{
		ID:     "run-1",
		Status: "queued",
		Request: btsrv.StartRequest{
			DefinitionID:      "def-1",
			DefinitionVersion: "0.1.0",
			Market:            "US",
			Code:              "AAPL",
			Symbol:            "US.AAPL",
			Interval:          "1m",
			StartTime:         "2025-01-01T09:30:00Z",
			EndTime:           "2025-01-01T09:35:00Z",
			InitialBalance:    10000,
			RehabType:         "forward",
			UseExtendedHours:  &useExtendedHours,
		},
		Result:    &backtest.RunResult{PnL: 12, Logs: []string{"queued"}},
		CreatedAt: "2025-01-01T09:30:00Z",
		UpdatedAt: "2025-01-01T09:30:00Z",
	}

	if got := toSrvRunState(nil); got != nil {
		t.Fatalf("toSrvRunState(nil) = %#v, want nil", got)
	}
	if got := toBacktestRunState(nil); got != nil {
		t.Fatalf("toBacktestRunState(nil) = %#v, want nil", got)
	}
	if err := adapter.Add(run); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := adapter.Get("run-1")
	if !ok || got == nil || got.Request.Symbol != "US.AAPL" || got.Result == nil || got.Result.PnL != 12 {
		t.Fatalf("Get(run-1) = %#v ok=%v, want stored run", got, ok)
	}
	if got, ok := adapter.Get("missing"); ok || got != nil {
		t.Fatalf("Get(missing) = %#v ok=%v, want nil,false", got, ok)
	}

	full, ok, err := adapter.GetFull("run-1")
	if err != nil || !ok || full == nil || full.Result == nil || full.Result.PnL != 12 {
		t.Fatalf("GetFull(run-1) = %#v ok=%v err=%v, want stored run", full, ok, err)
	}

	list := adapter.List()
	if len(list) != 1 || list[0].ID != "run-1" {
		t.Fatalf("List() = %#v, want single stored run", list)
	}
	lightweight := adapter.ListLightweight()
	if len(lightweight) != 1 || lightweight[0].Result != nil {
		t.Fatalf("ListLightweight() = %#v, want result omitted", lightweight)
	}

	updated, err := adapter.Update("run-1", func(state *btsrv.RunState) {
		state.Status = "running"
		state.UpdatedAt = "2025-01-01T09:31:00Z"
	})
	if err != nil || !updated {
		t.Fatalf("Update(run-1) updated=%v err=%v, want true,nil", updated, err)
	}
	if stored, ok := adapter.Get("run-1"); !ok || stored.Status != "running" {
		t.Fatalf("Get(run-1) after Update = %#v ok=%v, want running", stored, ok)
	}

	if updated := adapter.UpdateMemoryOnly("run-1", func(state *btsrv.RunState) {
		state.Status = "completed"
		state.Result = &backtest.RunResult{PnL: 25}
	}); !updated {
		t.Fatal("UpdateMemoryOnly(run-1) = false, want true")
	}
	if stored, ok := adapter.Get("run-1"); !ok || stored.Status != "completed" || stored.Result == nil || stored.Result.PnL != 25 {
		t.Fatalf("Get(run-1) after UpdateMemoryOnly = %#v ok=%v, want completed with updated result", stored, ok)
	}
	if updated := adapter.UpdateMemoryOnly("missing", func(*btsrv.RunState) {}); updated {
		t.Fatal("UpdateMemoryOnly(missing) = true, want false")
	}

	cancelled := false
	adapter.SetCancel("run-1", func() { cancelled = true })
	if !adapter.Cancel("run-1") || !cancelled {
		t.Fatalf("Cancel(run-1) cancelled=%v, want delegated cancellation", cancelled)
	}
	if adapter.Cancel("missing") {
		t.Fatal("Cancel(missing) = true, want false")
	}

	deleted, ok, err := adapter.Delete("run-1")
	if err != nil || !ok || deleted == nil || deleted.ID != "run-1" {
		t.Fatalf("Delete(run-1) = %#v ok=%v err=%v, want deleted run", deleted, ok, err)
	}
	if deleted, ok, err := adapter.Delete("missing"); err != nil || ok || deleted != nil {
		t.Fatalf("Delete(missing) = %#v ok=%v err=%v, want nil,false,nil", deleted, ok, err)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
}

func TestBacktestSyncTaskStoreAndStrategyProviderAdapters(t *testing.T) {
	syncStore := newBacktestSyncTaskStore()
	syncAdapter := &backtestSyncTaskStoreAdapter{store: syncStore}
	cancelled := false
	progress := backtest.NewSyncProgress("sync-1", "US.AAPL", time.Now())
	syncAdapter.Add("sync-1", progress, func() { cancelled = true })

	got, ok := syncAdapter.Get("sync-1")
	if !ok || got == nil || got.TaskID != "sync-1" {
		t.Fatalf("Get(sync-1) = %#v ok=%v, want stored sync task", got, ok)
	}
	cancelledProgress, ok := syncAdapter.Cancel("sync-1", time.Now())
	if !ok || cancelledProgress == nil || cancelledProgress.Status != "cancelled" || !cancelled {
		t.Fatalf("Cancel(sync-1) progress=%#v ok=%v cancelled=%v, want cancelled snapshot", cancelledProgress, ok, cancelled)
	}
	if _, ok := syncAdapter.Cancel("missing", time.Now()); ok {
		t.Fatal("Cancel(missing) = true, want false")
	}

	syncAdapter.Add("sync-2", backtest.NewSyncProgress("sync-2", "US.TSLA", time.Now()), func() {})
	syncAdapter.Finish("sync-2")
	if _, ok := syncAdapter.Cancel("sync-2", time.Now()); ok {
		t.Fatal("Cancel(sync-2 after Finish) = true, want false")
	}

	defStore, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := defStore.Close(); closeErr != nil {
			t.Fatalf("defStore.Close: %v", closeErr)
		}
	})
	definition, err := defStore.saveDefinition(strategyDesignDefinition{
		Name:         "Adapter Strategy",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: SourceFormatPineV6(),
		Symbol:       "US.AAPL",
		Interval:     "5m",
		Script: `//@version=6
strategy("Adapter Strategy")
log.info("ok")`,
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	provider := &strategyProviderAdapter{store: defStore}
	if got, ok, err := provider.Definition(definition.ID); err != nil || !ok || got.ID != definition.ID || got.Script != definition.Script {
		t.Fatalf("Definition(found) = %#v ok=%v err=%v, want stored strategy definition", got, ok, err)
	}
	if got, ok, err := provider.Definition("missing"); err != nil || ok || got != (btsrv.StrategyDef{}) {
		t.Fatalf("Definition(missing) = %#v ok=%v err=%v, want zero,false,nil", got, ok, err)
	}
}

func TestNewADKRuntimeCoversSuccessAndDegradedPaths(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		runtime := NewADKRuntime(settingsPath, RuntimeDeps{})
		if runtime == nil || runtime.Store() == nil || runtime.Tools() == nil {
			t.Fatalf("NewADKRuntime(success) = %#v, want initialized runtime", runtime)
		}
		if closeErr := runtime.Close(); closeErr != nil {
			t.Fatalf("runtime.Close: %v", closeErr)
		}
	})

	t.Run("store failure returns nil", func(t *testing.T) {
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		t.Setenv("JFTRADE_ADK_DB", t.TempDir())
		if runtime := NewADKRuntime(settingsPath, RuntimeDeps{}); runtime != nil {
			t.Fatalf("NewADKRuntime(store failure) = %#v, want nil", runtime)
		}
	})

	t.Run("session failure returns nil", func(t *testing.T) {
		settingsPath := filepath.Join(t.TempDir(), "settings.json")
		t.Setenv("JFTRADE_ADK_DB", filepath.Join(t.TempDir(), "adk.db"))
		t.Setenv("JFTRADE_ADK_SESSION_DB", t.TempDir())
		if runtime := NewADKRuntime(settingsPath, RuntimeDeps{}); runtime != nil {
			if closeErr := runtime.Close(); closeErr != nil {
				t.Fatalf("runtime.Close after unexpected success: %v", closeErr)
			}
			t.Fatalf("NewADKRuntime(session failure) = %#v, want nil", runtime)
		}
	})

	t.Run("server wrapper", func(t *testing.T) {
		store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
		if err != nil {
			t.Fatalf("NewSettingsStore: %v", err)
		}
		server := newTestServer(t, store)
		runtime := newADKRuntime(server, filepath.Join(t.TempDir(), "settings.json"))
		if runtime == nil || runtime.Tools() == nil {
			t.Fatalf("newADKRuntime(server) = %#v, want initialized runtime", runtime)
		}
		if closeErr := runtime.Close(); closeErr != nil {
			t.Fatalf("wrapped runtime.Close: %v", closeErr)
		}
	})
}

func TestADKAdapterSaveHelpersAndVisualModelValidation(t *testing.T) {
	var nilServer *Server
	validation, err := ValidateADKStrategyScript("test", `//@version=6
strategy("Adapter Save Helper", overlay=true)
log.info("ok")`)
	if err != nil {
		t.Fatalf("ValidateADKStrategyScript: %v", err)
	}

	if _, err := nilServer.adkSaveStrategyDraft(StrategyDraftInput{Validation: validation}); err == nil {
		t.Fatal("nilServer.adkSaveStrategyDraft error = nil, want unavailable store error")
	}
	if _, err := nilServer.adkSaveStrategyDefinition(StrategyDefinitionInput{Name: "x", Validation: validation}); err == nil {
		t.Fatal("nilServer.adkSaveStrategyDefinition error = nil, want unavailable store error")
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	draftAny, err := server.adkSaveStrategyDraft(StrategyDraftInput{Validation: validation})
	if err != nil {
		t.Fatalf("adkSaveStrategyDraft(default name): %v", err)
	}
	draft := draftAny.(strategyDesignDefinition)
	if draft.Name != "ADK 策略草稿" || draft.SourceFormat != SourceFormatPineV6() || draft.Runtime != strategyRuntimePinePlan {
		t.Fatalf("draft = %#v, want default draft naming plus Pine runtime defaults", draft)
	}

	model, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []map[string]any{{"id": "n1", "type": "note"}},
	})
	if err != nil {
		t.Fatalf("strategyVisualModelFromInput(valid): %v", err)
	}
	if model == nil || model.Engine != "logic-flow" || model.Version != 1 || len(model.Edges) != 0 || model.Nodes[0].Properties == nil {
		t.Fatalf("strategyVisualModelFromInput(valid) = %#v, want normalized model defaults", model)
	}
	if _, err := strategyVisualModelFromInput("not-an-object"); err == nil {
		t.Fatal("strategyVisualModelFromInput(string) error = nil, want validation error")
	}
	if _, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []map[string]any{{
			"id":         "n1",
			"type":       "note",
			"properties": map[string]any{"blockKind": "codeBlock"},
		}},
	}); err == nil {
		t.Fatal("strategyVisualModelFromInput(legacy block) error = nil, want unsupported legacy block error")
	}

	if _, err := server.adkSaveStrategyDefinition(StrategyDefinitionInput{
		Name:        "Invalid Visual Strategy",
		Validation:  validation,
		VisualModel: "bad-shape",
	}); err == nil {
		t.Fatal("adkSaveStrategyDefinition(invalid visual model) error = nil, want validation error")
	}
}
