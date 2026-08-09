package backtest

import (
	"path/filepath"
	"testing"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
)

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("cleanup failed: %v", err)
	}
}

func TestBacktestRunStoreDirectlyImplementsDomainLifecycle(t *testing.T) {
	store := NewInMemory()
	adapter := store
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
		Result:    &btsrv.RunResult{PnL: 12, Logs: []string{"queued"}},
		CreatedAt: "2025-01-01T09:30:00Z",
		UpdatedAt: "2025-01-01T09:30:00Z",
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
		state.Result = &btsrv.RunResult{PnL: 25}
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

func TestBacktestSyncTaskStoreLifecycle(t *testing.T) {
	syncStore := NewSyncTaskStore()
	syncAdapter := syncStore
	cancelled := false
	progress := btsrv.NewSyncProgress("sync-1", "US.AAPL", time.Now())
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

	syncAdapter.Add("sync-2", btsrv.NewSyncProgress("sync-2", "US.TSLA", time.Now()), func() {})
	syncAdapter.Finish("sync-2")
	if _, ok := syncAdapter.Cancel("sync-2", time.Now()); ok {
		t.Fatal("Cancel(sync-2 after Finish) = true, want false")
	}
}

func TestBacktestRunStoreMigration(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "backtest.db"))
	if err != nil {
		t.Fatalf("New backtest store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	// 验证空列表
	runs := store.ListLightweight()
	if runs == nil {
		t.Fatal("listLightweight returned nil")
	}
}
