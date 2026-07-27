package backtest

import (
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestSyncTaskStoreReturnsSnapshotsAndCancelsProgress(t *testing.T) {
	store := NewSyncTaskStore()
	queuedAt := time.Date(2026, time.May, 25, 10, 0, 0, 0, time.UTC)
	progress := bt.NewSyncProgress("sync-1", "HK.00700", queuedAt)
	progress.SetRunning(2, queuedAt)
	progress.BeginInterval(bbgotypes.Interval("1m"), 0, queuedAt.Add(time.Second))
	cancelled := false
	store.Add("sync-1", progress, func() { cancelled = true })

	snapshot, ok := store.Get("sync-1")
	if !ok || snapshot == nil {
		t.Fatal("sync snapshot missing")
	}
	snapshot.Status = "failed"
	snapshot.CompletedBatches = 99
	fresh, _ := store.Get("sync-1")
	if fresh.Status != "running" || fresh.CompletedBatches != 0 {
		t.Fatalf("stored progress mutated through snapshot: %+v", fresh)
	}
	if reason := store.MaintenanceBusyReason(t.Context()); !strings.Contains(reason, "行情同步") {
		t.Fatalf("busy reason = %q", reason)
	}

	cancelledSnapshot, ok := store.Cancel("sync-1", queuedAt.Add(time.Minute))
	if !ok || !cancelled || cancelledSnapshot == nil || cancelledSnapshot.Status != "cancelled" {
		t.Fatalf("Cancel = %+v ok=%v called=%v", cancelledSnapshot, ok, cancelled)
	}
	if reason := store.MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("finished cancellation remained busy: %q", reason)
	}
	fresh, _ = store.Get("sync-1")
	if fresh.Status != "cancelled" || fresh.UpdatedAt == "" {
		t.Fatalf("stored progress not cancelled: %+v", fresh)
	}
}

func TestSyncTaskStoreFinishAndNilProgressBoundaries(t *testing.T) {
	store := NewSyncTaskStore()
	called := false
	store.Add("nil-progress", nil, func() { called = true })
	if progress, ok := store.Get("nil-progress"); !ok || progress != nil {
		t.Fatalf("nil-progress Get = %+v ok=%v", progress, ok)
	}
	if progress, ok := store.Cancel("nil-progress", time.Now()); !ok || progress != nil || !called {
		t.Fatalf("nil-progress Cancel = %+v ok=%v called=%v", progress, ok, called)
	}
	store.Add("finished", bt.NewSyncProgress("finished", "US.AAPL", time.Now()), func() {})
	store.Finish("finished")
	if _, ok := store.Cancel("finished", time.Now()); ok {
		t.Fatal("finished task remained cancellable")
	}
	if progress, ok := store.Get("missing"); ok || progress != nil {
		t.Fatalf("missing Get = %+v ok=%v", progress, ok)
	}
}
