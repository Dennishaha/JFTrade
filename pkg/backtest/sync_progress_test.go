package backtest

import (
	"testing"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestSyncProgressSnapshotReturnsIndependentCopy(t *testing.T) {
	queuedAt := time.Date(2026, time.May, 25, 9, 0, 0, 0, time.UTC)
	progress := NewSyncProgress("sync-1", "HK.00700", queuedAt)
	progress.SetRunning(3, queuedAt.Add(time.Second))
	progress.BeginInterval(bbgotypes.Interval("1m"), 1, queuedAt.Add(2*time.Second))
	progress.CompleteInterval(2)
	progress.IncrementCompletedBatches(queuedAt.Add(3 * time.Second))
	progress.IncrementRetries()

	snapshot := progress.Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	snapshot.Status = "failed"
	snapshot.CompletedBatches = 99
	snapshot.Retries = 42

	fresh := progress.Snapshot()
	if fresh == nil {
		t.Fatal("expected fresh snapshot")
	}
	if fresh.Status != "running" {
		t.Fatalf("progress status mutated through snapshot: %s", fresh.Status)
	}
	if fresh.CompletedBatches != 1 {
		t.Fatalf("progress batches mutated through snapshot: %d", fresh.CompletedBatches)
	}
	if fresh.Retries != 1 {
		t.Fatalf("progress retries mutated through snapshot: %d", fresh.Retries)
	}
	if fresh.CompletedIntervals != 2 {
		t.Fatalf("progress completed intervals = %d", fresh.CompletedIntervals)
	}
}
