package backtest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func TestMaintenancePurgesOnlyExactTerminalSetAndCompacts(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, run := range []*btsrv.RunState{
		{ID: "completed", Status: "completed"},
		{ID: "failed", Status: "failed"},
		{ID: "running", Status: "running"},
	} {
		if err := store.Add(run); err != nil {
			t.Fatal(err)
		}
	}
	if reason := store.MaintenanceBusyReason(t.Context()); reason == "" {
		t.Fatal("active run did not block maintenance")
	}
	deleted, err := store.PurgeMaintenanceCandidates(t.Context(), []dmsrv.CleanupCandidate{
		{ID: "completed"},
		{ID: "failed"},
	})
	if err != nil || deleted != 2 {
		t.Fatalf("PurgeMaintenanceCandidates = %d, %v", deleted, err)
	}
	if _, ok := store.Get("completed"); ok {
		t.Fatal("purged run remains in memory")
	}
	if _, err := store.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: "running"}},
	); !errors.Is(err, dmsrv.ErrCleanupCandidatesChanged) {
		t.Fatalf("maintenance running candidate error = %v", err)
	}
	if _, ok := store.Get("running"); !ok {
		t.Fatal("stale purge removed running state")
	}
	if err := store.CompactMaintenanceResource(t.Context()); err != nil {
		t.Fatalf("CompactMaintenanceResource: %v", err)
	}
}

func TestStoreConcurrentReadersAndWritersKeepIndependentState(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	const runs = 24
	var writers sync.WaitGroup
	for index := range runs {
		writers.Go(func() {
			id := fmt.Sprintf("run-%02d", index)
			if addErr := store.Add(&btsrv.RunState{ID: id, Status: "queued"}); addErr != nil {
				t.Errorf("Add %s: %v", id, addErr)
				return
			}
			if _, updateErr := store.Update(id, func(run *btsrv.RunState) { run.Status = "completed" }); updateErr != nil {
				t.Errorf("Update %s: %v", id, updateErr)
			}
		})
	}
	for range runs {
		writers.Go(func() {
			_ = store.ListLightweight()
		})
	}
	writers.Wait()
	if got := len(store.List()); got != runs {
		t.Fatalf("run count = %d, want %d", got, runs)
	}
	for _, run := range store.List() {
		if run.Status != "completed" {
			t.Fatalf("run not completed: %+v", run)
		}
	}
}

func TestUnavailableStoreMaintenanceFailsClosed(t *testing.T) {
	if reason := (*Store)(nil).MaintenanceBusyReason(context.Background()); reason != "" {
		t.Fatalf("nil busy reason = %q", reason)
	}
	var nilPurger dmsrv.CandidatePurger = (*Store)(nil)
	if _, err := nilPurger.PurgeMaintenanceCandidates(context.Background(), nil); err == nil {
		t.Fatal("nil store purge succeeded")
	}
	if err := NewInMemory().CompactMaintenanceResource(context.Background()); err == nil {
		t.Fatal("in-memory store compact succeeded")
	}
}

func TestKLineDatabaseMaintenanceBoundaryOpensAndCompacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backtest.db")
	database, err := OpenKLineDatabase(path)
	if err != nil {
		t.Fatalf("OpenKLineDatabase: %v", err)
	}
	if err := database.CompactDatabase(t.Context()); err != nil {
		t.Fatalf("CompactDatabase: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := NewKLineMaintenance(path).CompactMaintenanceResource(t.Context()); err != nil {
		t.Fatalf("KLineMaintenance.CompactMaintenanceResource: %v", err)
	}
	if err := (*KLineMaintenance)(nil).CompactMaintenanceResource(t.Context()); err == nil {
		t.Fatal("nil KLineMaintenance compact succeeded")
	}
}
