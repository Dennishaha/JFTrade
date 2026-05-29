package jftradeapi

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestNewStrategyRuntimeStoreCreatesExpectedSchema(t *testing.T) {
	store := newStrategyRuntimeStoreForTest(t)

	for _, tableName := range []string{
		strategyRuntimeLogTable,
		strategyRuntimeAuditTable,
		strategyRuntimeObservationTable,
	} {
		var count int
		if err := store.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("check %s table: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("expected %s table to exist once, got %d", tableName, count)
		}
	}
}

func TestNewStrategyRuntimeStoreRejectsLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-strategy-runtime.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE strategy_log_events (id INTEGER PRIMARY KEY, message TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	_, err = NewStrategyRuntimeStore(dbPath)
	if err == nil {
		t.Fatal("expected legacy strategy runtime schema error")
	}
	if !strings.Contains(err.Error(), "schema is obsolete") {
		t.Fatalf("unexpected legacy schema error: %v", err)
	}
}

func TestStrategyRuntimeStoreRoundTripsLogsAuditAndObservation(t *testing.T) {
	ctx := context.Background()
	store := newStrategyRuntimeStoreForTest(t)

	baseTime := time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)
	for index := range 3 {
		eventAt := baseTime.Add(time.Duration(index) * time.Minute)
		level := "info"
		if index == 1 {
			level = "error"
		}
		if err := store.AppendLog(ctx, strategyRuntimeLogEvent{
			InstanceID: "instance-1",
			At:         eventAt,
			Raw:        eventAt.Format(time.RFC3339Nano) + " runtime event",
			Level:      level,
			Source:     "runtime",
		}); err != nil {
			t.Fatalf("AppendLog(%d): %v", index, err)
		}
		if err := store.AppendAudit(ctx, strategyRuntimeAuditEvent{
			InstanceID: "instance-1",
			Kind:       []string{"started", "runtime_error", "stopped"}[index],
			Detail:     "detail",
			At:         eventAt,
		}); err != nil {
			t.Fatalf("AppendAudit(%d): %v", index, err)
		}
	}

	lastSignalAt := baseTime.Add(3 * time.Minute)
	updatedAt := baseTime.Add(4 * time.Minute)
	if err := store.UpsertObservation(ctx, strategyRuntimeObservationSnapshot{
		InstanceID:    "instance-1",
		ActualStatus:  strategyStatusStopped,
		ActiveSymbols: []string{"US.AAPL", "US.TSLA"},
		LastSignalAt:  &lastSignalAt,
		LastError:     "runtime error US.AAPL: boom",
		UpdatedAt:     &updatedAt,
	}); err != nil {
		t.Fatalf("UpsertObservation: %v", err)
	}

	logs, err := store.ListLogs(ctx, strategyRuntimeLogQuery{InstanceID: "instance-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	if !logs[0].At.After(logs[1].At) || !logs[1].At.After(logs[2].At) {
		t.Fatalf("expected logs to be sorted desc by time, got %+v", logs)
	}

	errorLogs, err := store.ListLogs(ctx, strategyRuntimeLogQuery{InstanceID: "instance-1", Limit: 10, Level: "error"})
	if err != nil {
		t.Fatalf("ListLogs level filter: %v", err)
	}
	if len(errorLogs) != 1 || errorLogs[0].Level != "error" {
		t.Fatalf("unexpected level-filtered logs: %+v", errorLogs)
	}

	auditEntries, err := store.ListAudit(ctx, strategyRuntimeAuditQuery{InstanceID: "instance-1", Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(auditEntries) != 2 {
		t.Fatalf("expected 2 audit entries after offset, got %d", len(auditEntries))
	}
	if auditEntries[0].Kind != "runtime_error" || auditEntries[1].Kind != "started" {
		t.Fatalf("unexpected paged audit entries: %+v", auditEntries)
	}

	observation, ok, err := store.GetObservation(ctx, "instance-1")
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if !ok {
		t.Fatal("expected observation to exist")
	}
	if observation.ActualStatus != strategyStatusStopped {
		t.Fatalf("observation actual status = %s", observation.ActualStatus)
	}
	if len(observation.ActiveSymbols) != 2 || observation.ActiveSymbols[1] != "US.TSLA" {
		t.Fatalf("unexpected observation active symbols: %+v", observation.ActiveSymbols)
	}
	if observation.LastSignalAt == nil || !observation.LastSignalAt.Equal(lastSignalAt) {
		t.Fatalf("unexpected lastSignalAt: %+v", observation.LastSignalAt)
	}
	if observation.UpdatedAt == nil || !observation.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected updatedAt: %+v", observation.UpdatedAt)
	}
	if observation.LastError != "runtime error US.AAPL: boom" {
		t.Fatalf("unexpected lastError: %s", observation.LastError)
	}
}

func newStrategyRuntimeStoreForTest(t *testing.T) *strategyRuntimeStore {
	t.Helper()
	store, err := NewStrategyRuntimeStore(filepath.Join(t.TempDir(), "strategy-runtime-test.db"))
	if err != nil {
		t.Fatalf("NewStrategyRuntimeStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
	return store
}
