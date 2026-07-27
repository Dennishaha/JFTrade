package backtest

import (
	"path/filepath"
	"strings"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestStoreSnapshotsDoNotMutateRuns(t *testing.T) {
	store := NewInMemory()
	original := completeRun("bt-snapshot", "completed")
	if err := store.Add(original); err != nil {
		t.Fatalf("Add: %v", err)
	}

	snapshot, ok := store.Get(original.ID)
	if !ok {
		t.Fatal("stored run missing")
	}
	snapshot.Status = "failed"
	snapshot.Request.Symbol = "US.TSLA"
	snapshot.Result.FinalBalance = 42
	snapshot.Result.Trades[0].Price = "999"
	snapshot.Result.PnLCurve[0].Equity = 12
	snapshot.Result.Logs[0] = "changed"

	fresh, ok := store.Get(original.ID)
	if !ok || fresh.Status != "completed" || fresh.Request.Symbol != "HK.00700" {
		t.Fatalf("stored run mutated through Get: %+v", fresh)
	}
	if fresh.Result.FinalBalance != 123456 || fresh.Result.Trades[0].Price != "100" ||
		fresh.Result.PnLCurve[0].Equity != 100000 || fresh.Result.Logs[0] != "ready" {
		t.Fatalf("stored result mutated through Get: %+v", fresh.Result)
	}

	listed := store.List()
	listed[0].Status = "running"
	listed[0].Result.Logs[0] = "list mutation"
	fresh, _ = store.Get(original.ID)
	if fresh.Status != "completed" || fresh.Result.Logs[0] != "ready" {
		t.Fatalf("stored run mutated through List: %+v", fresh)
	}
	lightweight := store.ListLightweight()
	if len(lightweight) != 1 || lightweight[0].Result != nil {
		t.Fatalf("lightweight runs = %+v", lightweight)
	}
}

func TestStorePersistsResultsAndRecoversTransientRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backtest-runs.db")
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !store.Available() {
		t.Fatal("durable store reported unavailable")
	}
	completed := completeRun("bt-completed", "completed")
	completed.Request.StartDate = "2026-05-01"
	completed.Request.EndDate = "2026-05-02"
	completed.Request.MarketTimezone = "America/New_York"
	running := &btsrv.RunState{
		ID: "bt-running", Status: "running",
		Request: btsrv.StartRequest{
			Symbol: "US.TSLA", Interval: "1m",
			StartTime: "2026-05-03T00:00:00Z", EndTime: "2026-05-04T00:00:00Z",
		},
		CreatedAt: "2026-05-30T00:00:02Z", UpdatedAt: "2026-05-30T00:00:03Z",
	}
	if err := store.Add(completed); err != nil {
		t.Fatalf("Add completed: %v", err)
	}
	if err := store.Add(running); err != nil {
		t.Fatalf("Add running: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close before reload: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	reloaded, err := New(dbPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := reloaded.Close(); closeErr != nil {
			t.Errorf("Close reloaded: %v", closeErr)
		}
	})

	metadata, ok := reloaded.Get(completed.ID)
	if !ok || metadata.Status != "completed" || metadata.Result != nil {
		t.Fatalf("reloaded metadata = %+v", metadata)
	}
	if metadata.Request.StartDate != completed.Request.StartDate ||
		metadata.Request.EndDate != completed.Request.EndDate ||
		metadata.Request.MarketTimezone != completed.Request.MarketTimezone {
		t.Fatalf("date metadata changed: %+v", metadata.Request)
	}
	full, ok, err := reloaded.GetFull(completed.ID)
	if err != nil || !ok || full.Result == nil || full.Result.FinalBalance != 123456 {
		t.Fatalf("reloaded full run = %+v ok=%v err=%v", full, ok, err)
	}
	recovered, ok := reloaded.Get(running.ID)
	if !ok || recovered.Status != "failed" || recovered.Result == nil ||
		!strings.Contains(recovered.Result.Error, RecoveredRunErrorText) {
		t.Fatalf("recovered transient run = %+v", recovered)
	}
	if recovered.Result.Symbol != running.Request.Symbol || recovered.Result.Interval != running.Request.Interval {
		t.Fatalf("recovered result identity = %+v", recovered.Result)
	}

	deleted, ok, err := reloaded.Delete(completed.ID)
	if err != nil || !ok || deleted.ID != completed.ID {
		t.Fatalf("Delete = %+v ok=%v err=%v", deleted, ok, err)
	}
	if err := reloaded.Close(); err != nil {
		t.Fatalf("Close before second reload: %v", err)
	}
	reloadedAgain, err := New(dbPath)
	if err != nil {
		t.Fatalf("second reload: %v", err)
	}
	t.Cleanup(func() { _ = reloadedAgain.Close() })
	if _, ok := reloadedAgain.Get(completed.ID); ok {
		t.Fatal("deleted run returned after reload")
	}
}

func TestStoredRequestDoesNotInferMissingDateMetadata(t *testing.T) {
	run, err := runStateFromRow(runStateRow{
		ID: "without-date-metadata", Status: "completed",
		RequestJSON: `{"symbol":"US.AAPL","startTime":"2026-05-01T23:30:00-05:00","endTime":"2026-05-02T23:30:00-05:00"}`,
	})
	if err != nil {
		t.Fatalf("runStateFromRow: %v", err)
	}
	if run.Request.StartDate != "" || run.Request.EndDate != "" || run.Request.MarketTimezone != "" {
		t.Fatalf("missing date metadata inferred: %+v", run.Request)
	}
	if run.Request.StartTime != "2026-05-01T23:30:00-05:00" || run.Request.EndTime != "2026-05-02T23:30:00-05:00" {
		t.Fatalf("stored timestamps changed: %+v", run.Request)
	}
}

func TestDerivePathHonorsOverrideAndSettingsDirectory(t *testing.T) {
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", " custom-runs.db ")
	if got := DerivePath("settings.json"); got != "custom-runs.db" {
		t.Fatalf("override path = %q", got)
	}
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", "")
	if got := DerivePath("settings.json"); got != defaultRunDBFilename {
		t.Fatalf("default path = %q", got)
	}
	if got := DerivePath(filepath.Join("var", "settings.json")); got != filepath.Join("var", defaultRunDBFilename) {
		t.Fatalf("settings-relative path = %q", got)
	}
}

func TestInMemoryStoreImplementsRunLifecycleAndCancellation(t *testing.T) {
	store := NewInMemory()
	if store.Available() {
		t.Fatal("in-memory store reported durable availability")
	}
	if NewInMemory().UpdateMemoryOnly("missing", func(*btsrv.RunState) {}) {
		t.Fatal("missing memory update succeeded")
	}
	run := &btsrv.RunState{ID: "lifecycle", Status: "queued"}
	if err := store.Add(run); err != nil {
		t.Fatal(err)
	}
	if updated := store.UpdateMemoryOnly(run.ID, func(state *btsrv.RunState) { state.Status = "running" }); !updated {
		t.Fatal("memory update failed")
	}
	if got, _ := store.Get(run.ID); got.Status != "running" {
		t.Fatalf("updated run = %+v", got)
	}
	cancelled := false
	store.SetCancel(run.ID, func() { cancelled = true })
	if !store.Cancel(run.ID) || !cancelled {
		t.Fatal("registered cancellation was not invoked")
	}
	store.SetCancel(run.ID, nil)
	if store.Cancel(run.ID) || store.Cancel("missing") {
		t.Fatal("cleared or missing cancellation succeeded")
	}
	deleted, ok, err := store.Delete(run.ID)
	if err != nil || !ok || deleted.ID != run.ID {
		t.Fatalf("Delete = %+v ok=%v err=%v", deleted, ok, err)
	}
	if deleted, ok, err := store.Delete("missing"); err != nil || ok || deleted != nil {
		t.Fatalf("missing Delete = %+v ok=%v err=%v", deleted, ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("in-memory Close: %v", err)
	}
	if err := (*Store)(nil).Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

func completeRun(id, status string) *btsrv.RunState {
	return &btsrv.RunState{
		ID: id, Status: status,
		Request: btsrv.StartRequest{DefinitionID: "def-1", Symbol: "HK.00700", Interval: "1m"},
		Result: &bt.RunResult{
			Symbol: "HK.00700", Interval: "1m", FinalBalance: 123456,
			Trades:   []bt.TradeEvent{{Time: "2026-01-02T00:00:00Z", Side: "BUY", Price: "100", Qty: "1"}},
			PnLCurve: []bt.PnLPoint{{Time: "2026-01-02T00:00:00Z", Equity: 100000}},
			Logs:     []string{"ready"},
		},
		CreatedAt: "2026-05-30T00:00:00Z", UpdatedAt: "2026-05-30T00:00:01Z",
	}
}
