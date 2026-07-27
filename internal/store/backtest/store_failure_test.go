package backtest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
)

func TestStoreRejectsUnavailableIncompatibleAndCorruptDatabases(t *testing.T) {
	if _, err := New(" "); err == nil {
		t.Fatal("blank path accepted")
	}
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(blocker, "runs.db")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("blocked directory error = %v", err)
	}
	if _, err := New(root); err == nil {
		t.Fatal("directory database path accepted")
	}

	incompatiblePath := filepath.Join(root, "incompatible.db")
	db, err := sqliteconn.OpenX(incompatiblePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE backtest_runs (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(incompatiblePath); err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("incompatible schema error = %v", err)
	}

	corruptPath := filepath.Join(root, "corrupt.db")
	store, err := openStore(corruptPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) VALUES ('corrupt', 'completed', '{', '', '', '')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(corruptPath); err == nil || !strings.Contains(err.Error(), "load") {
		t.Fatalf("corrupt row load error = %v", err)
	}
}

func TestStoreRollsBackMemoryWhenClosedDatabaseRejectsWrites(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	run := &btsrv.RunState{ID: "run", Status: "completed", Request: btsrv.StartRequest{Symbol: "US.AAPL"}}
	if err := store.Add(run); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&btsrv.RunState{ID: "new", Status: "queued"}); err == nil {
		t.Fatal("closed database accepted new run")
	}
	if _, ok := store.Get("new"); ok {
		t.Fatal("failed new run remained in memory")
	}
	if err := store.Add(&btsrv.RunState{ID: "run", Status: "failed"}); err == nil {
		t.Fatal("closed database accepted replacement")
	}
	if got, _ := store.Get("run"); got.Status != "completed" {
		t.Fatalf("failed replacement changed memory: %+v", got)
	}
	if ok, err := store.Update("run", func(state *btsrv.RunState) { state.Status = "failed" }); !ok || err == nil {
		t.Fatalf("closed Update = %v, %v", ok, err)
	}
	if got, _ := store.Get("run"); got.Status != "completed" {
		t.Fatalf("failed update changed memory: %+v", got)
	}
	if _, ok, err := store.Delete("run"); !ok || err == nil {
		t.Fatalf("closed Delete ok=%v err=%v", ok, err)
	}
}

func TestStoreFullReadHandlesMissingRowsAndInvalidResults(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.runs["memory-only"] = &btsrv.RunState{ID: "memory-only", Status: "completed"}
	if run, ok, err := store.GetFull("memory-only"); err != nil || !ok || run.ID != "memory-only" {
		t.Fatalf("memory-only GetFull = %+v ok=%v err=%v", run, ok, err)
	}
	if run, ok, err := store.GetFull("missing"); err != nil || ok || run != nil {
		t.Fatalf("missing GetFull = %+v ok=%v err=%v", run, ok, err)
	}
	if err := store.Add(&btsrv.RunState{ID: "invalid-result", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE backtest_runs SET result_json = '{' WHERE id = 'invalid-result'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetFull("invalid-result"); err == nil || !ok {
		t.Fatalf("invalid result GetFull ok=%v err=%v", ok, err)
	}
}

func TestStoreCanceledMaintenanceDoesNotMutateRuns(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Add(&btsrv.RunState{ID: "completed", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PurgeTerminalRuns(ctx, []string{"completed"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled purge error = %v", err)
	}
	if _, ok := store.Get("completed"); !ok {
		t.Fatal("canceled purge removed memory state")
	}
}

func TestPersistenceDecodersRejectInvalidPayloads(t *testing.T) {
	if cloneRunState(nil) != nil {
		t.Fatal("nil clone was non-nil")
	}
	if _, err := runStateFromRow(runStateRow{ID: "bad-request", RequestJSON: "{"}); err == nil {
		t.Fatal("invalid request JSON accepted")
	}
	if _, err := runStateFromRow(runStateRow{ID: "bad-result", RequestJSON: `{}`, ResultJSON: "{"}); err == nil {
		t.Fatal("invalid result JSON accepted")
	}
	result, err := decodeResultJSON("valid", `{"symbol":"US.AAPL"}`)
	if err != nil || result == nil || result.Symbol != "US.AAPL" {
		t.Fatalf("decoded result = %+v err=%v", result, err)
	}
	if markRecoveredRun(nil, "now") || markRecoveredRun(&btsrv.RunState{Status: "completed"}, "now") {
		t.Fatal("non-transient run was recovered")
	}
}
