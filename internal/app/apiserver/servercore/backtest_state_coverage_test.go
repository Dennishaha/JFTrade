package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
)

func TestBacktestRunStateRemainingPureBoundaries(t *testing.T) {
	if cloneBacktestRunState(nil) != nil {
		t.Fatal("nil backtest clone was non-nil")
	}
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", " custom-runs.db ")
	if got := deriveBacktestRunDBPath("settings.json"); got != "custom-runs.db" {
		t.Fatalf("environment run DB path = %q", got)
	}
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", "")
	if got := deriveBacktestRunDBPath("settings.json"); got != defaultBacktestRunDBFilename {
		t.Fatalf("default run DB path = %q", got)
	}
	if _, err := newBacktestRunStoreWithDB(" "); err == nil {
		t.Fatal("blank run DB path error = nil")
	}

	var nilStore *backtestRunStore
	if err := nilStore.initializeOrValidateSchema(); err != nil {
		t.Fatal(err)
	}
	if err := nilStore.loadFromDB(); err != nil {
		t.Fatal(err)
	}
	if err := nilStore.persistRunLocked(nil); err != nil {
		t.Fatal(err)
	}

	if _, err := backtestRunStateFromRow(backtestRunStateRow{ID: "bad-request", RequestJSON: "{"}); err == nil {
		t.Fatal("invalid request JSON error = nil")
	}
	if _, err := backtestRunStateFromRow(backtestRunStateRow{ID: "bad-result", RequestJSON: `{}`, ResultJSON: "{"}); err == nil {
		t.Fatal("invalid result JSON error = nil")
	}
	result, err := decodeBacktestResultJSON("valid", `{"symbol":"US.AAPL"}`)
	if err != nil || result == nil || result.Symbol != "US.AAPL" {
		t.Fatalf("decoded result = %#v, %v", result, err)
	}
	if markRecoveredBacktestRun(nil, "now") {
		t.Fatal("nil run was recovered")
	}
	if markRecoveredBacktestRun(&backtestRunState{Status: "completed"}, "now") {
		t.Fatal("terminal run was recovered")
	}
}

func TestBacktestRunStoreRemainingOpenAndLoadFailures(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newBacktestRunStoreWithDB(filepath.Join(blocker, "runs.db")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("blocked directory error = %v", err)
	}
	if _, err := newBacktestRunStoreWithDB(root); err == nil {
		t.Fatal("directory run DB open error = nil")
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
	if _, err := newBacktestRunStoreWithDB(incompatiblePath); err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("incompatible schema error = %v", err)
	}

	corruptPath := filepath.Join(root, "corrupt.db")
	store, err := newBacktestRunStoreWithDB(corruptPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at) VALUES ('corrupt', 'completed', '{', '', '', '')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := newBacktestRunStoreWithDB(corruptPath); err == nil || !strings.Contains(err.Error(), "load") {
		t.Fatalf("corrupt load error = %v", err)
	}
}

func TestBacktestRunStoreRemainingPersistenceRollbackAndFullReads(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	run := &backtestRunState{ID: "run", Status: "completed", Request: backtestStartRequest{Symbol: "US.AAPL"}, CreatedAt: "created", UpdatedAt: "updated"}
	if err := store.add(run); err != nil {
		t.Fatal(err)
	}
	store.runs["memory-only"] = &backtestRunState{ID: "memory-only", Status: "completed"}
	if got, ok, err := store.getFull("memory-only"); err != nil || !ok || got == nil {
		t.Fatalf("memory-only full run = %#v, %v, %v", got, ok, err)
	}
	if got, ok, err := store.getFull("missing"); err != nil || ok || got != nil {
		t.Fatalf("missing full run = %#v, %v, %v", got, ok, err)
	}
	if _, err := store.db.Exec(`UPDATE backtest_runs SET result_json = '{' WHERE id = 'run'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.getFull("run"); err == nil || !ok {
		t.Fatalf("invalid full result error = %v ok=%v", err, ok)
	}

	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.getFull("run"); err == nil || !ok {
		t.Fatalf("closed full read error = %v ok=%v", err, ok)
	}
	if err := store.add(&backtestRunState{ID: "new", Status: "queued"}); err == nil {
		t.Fatal("closed new add error = nil")
	}
	if _, ok := store.runs["new"]; ok {
		t.Fatal("failed new add remained in memory")
	}
	previous := cloneBacktestRunState(store.runs["run"])
	if err := store.add(&backtestRunState{ID: "run", Status: "failed"}); err == nil {
		t.Fatal("closed replacement add error = nil")
	}
	if store.runs["run"].Status != previous.Status {
		t.Fatal("failed replacement did not restore previous run")
	}
	if ok, err := store.update("run", func(run *backtestRunState) { run.Status = "failed" }); !ok || err == nil {
		t.Fatalf("closed update = %v, %v", ok, err)
	}
	if store.runs["run"].Status != previous.Status {
		t.Fatal("failed update did not restore previous run")
	}
	if _, ok, err := store.delete("run"); !ok || err == nil {
		t.Fatalf("closed delete = %v, %v", ok, err)
	}
}

func TestBacktestRunStoreNoRowsAndSyncTaskRemainingBoundaries(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.runs["memory-only"] = &backtestRunState{ID: "memory-only", Status: "completed"}
	if run, ok, err := store.getFull("memory-only"); err != nil || !ok || run.ID != "memory-only" {
		t.Fatalf("no-row full run = %#v, %v, %v", run, ok, err)
	}
	if deleted, ok, err := store.delete("missing"); err != nil || ok || deleted != nil {
		t.Fatalf("missing delete = %#v, %v, %v", deleted, ok, err)
	}

	tasks := newBacktestSyncTaskStore()
	if progress, ok := tasks.get("missing"); ok || progress != nil {
		t.Fatalf("missing task = %#v, %v", progress, ok)
	}
	cancelCalled := false
	tasks.add("nil-progress", nil, func() { cancelCalled = true })
	if progress, ok := tasks.cancel("nil-progress", time.Now()); !ok || progress != nil || !cancelCalled {
		t.Fatalf("nil progress cancellation = %#v, %v, called=%v", progress, ok, cancelCalled)
	}
}

func TestBacktestRunStoreCanceledContextPurgeBegin(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.purgeTerminalRuns(ctx, []string{"missing"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled purge error = %v", err)
	}
}
