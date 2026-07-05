package servercore

import (
	"testing"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestSyncTaskStoreGetReturnsSnapshot(t *testing.T) {
	store := newBacktestSyncTaskStore()
	queuedAt := time.Date(2026, time.May, 25, 10, 0, 0, 0, time.UTC)
	progress := backtest.NewSyncProgress("sync-1", "HK.00700", queuedAt)
	progress.SetRunning(2, queuedAt)
	progress.BeginInterval(bbgotypes.Interval("1m"), 0, queuedAt.Add(time.Second))
	store.add("sync-1", progress, func() {})

	snapshot, ok := store.get("sync-1")
	if !ok {
		t.Fatal("expected sync progress snapshot")
	}

	snapshot.Status = "failed"
	snapshot.CompletedBatches = 99

	fresh, ok := store.get("sync-1")
	if !ok {
		t.Fatal("expected fresh sync progress snapshot")
	}
	if fresh.Status != "running" {
		t.Fatalf("stored progress status mutated through snapshot: %s", fresh.Status)
	}
	if fresh.CompletedBatches != 0 {
		t.Fatalf("stored progress batches mutated through snapshot: %d", fresh.CompletedBatches)
	}
}

func TestBacktestSyncTaskStoreCancelMarksProgressCancelled(t *testing.T) {
	store := newBacktestSyncTaskStore()
	queuedAt := time.Date(2026, time.May, 25, 10, 30, 0, 0, time.UTC)
	progress := backtest.NewSyncProgress("sync-2", "HK.00700", queuedAt)

	cancelled := false
	store.add("sync-2", progress, func() { cancelled = true })

	snapshot, ok := store.cancel("sync-2", queuedAt.Add(time.Minute))
	if !ok {
		t.Fatal("expected cancel to succeed")
	}
	if !cancelled {
		t.Fatal("expected cancel func to be called")
	}
	if snapshot == nil {
		t.Fatal("expected cancelled snapshot")
	}
	if snapshot.Status != "cancelled" {
		t.Fatalf("cancel snapshot status = %s", snapshot.Status)
	}

	fresh, ok := store.get("sync-2")
	if !ok {
		t.Fatal("expected stored cancelled snapshot")
	}
	if fresh.Status != "cancelled" {
		t.Fatalf("stored cancelled status = %s", fresh.Status)
	}
	if fresh.UpdatedAt == "" {
		t.Fatal("expected cancelled snapshot updatedAt to be populated")
	}
}
