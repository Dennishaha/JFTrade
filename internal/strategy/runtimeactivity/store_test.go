package runtimeactivity

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestNewStoreCreatesExpectedSchema(t *testing.T) {
	store := newStoreForTest(t)

	for _, tableName := range []string{LogTable, AuditTable, ObservationTable} {
		var count int
		if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("check %s table: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("expected %s table to exist once, got %d", tableName, count)
		}
	}
}

func TestNewStoreRejectsLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-strategy-runtime.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE strategy_log_events (id INTEGER PRIMARY KEY, message TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	_, err = NewStore(dbPath)
	if err == nil {
		t.Fatal("expected legacy strategy runtime schema error")
	}
	if !strings.Contains(err.Error(), "schema metadata is missing") {
		t.Fatalf("unexpected legacy schema error: %v", err)
	}
}

//nolint:funlen
func TestStoreRoundTripsLogsAuditAndObservation(t *testing.T) {
	ctx := context.Background()
	store := newStoreForTest(t)

	baseTime := time.Date(2026, time.May, 29, 10, 0, 0, 0, time.UTC)
	for index := range 3 {
		eventAt := baseTime.Add(time.Duration(index) * time.Minute)
		level := "info"
		if index == 1 {
			level = "error"
		}
		if err := store.AppendLog(ctx, LogEvent{
			InstanceID: "instance-1",
			At:         eventAt,
			Raw:        eventAt.Format(time.RFC3339Nano) + " runtime event",
			Level:      level,
			Source:     "runtime",
		}); err != nil {
			t.Fatalf("AppendLog(%d): %v", index, err)
		}
		if err := store.AppendAudit(ctx, AuditEvent{
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
	if err := store.UpsertObservation(ctx, ObservationSnapshot{
		InstanceID:    "instance-1",
		ActualStatus:  "STOPPED",
		ActiveSymbols: []string{"US.AAPL", "US.TSLA"},
		LastSignalAt:  &lastSignalAt,
		LastError:     "runtime error US.AAPL: boom",
		UpdatedAt:     &updatedAt,
	}); err != nil {
		t.Fatalf("UpsertObservation: %v", err)
	}

	logs, err := store.ListLogs(ctx, LogQuery{InstanceID: "instance-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	if !logs[0].At.After(logs[1].At) || !logs[1].At.After(logs[2].At) {
		t.Fatalf("expected logs to be sorted desc by time, got %+v", logs)
	}

	errorLogs, err := store.ListLogs(ctx, LogQuery{InstanceID: "instance-1", Limit: 10, Level: "error"})
	if err != nil {
		t.Fatalf("ListLogs level filter: %v", err)
	}
	if len(errorLogs) != 1 || errorLogs[0].Level != "error" {
		t.Fatalf("unexpected level-filtered logs: %+v", errorLogs)
	}
	fromAt := baseTime.Add(time.Minute)
	toAt := baseTime.Add(2 * time.Minute)
	logCount, err := store.CountLogs(ctx, LogQuery{InstanceID: "instance-1", FromAt: &fromAt, ToAt: &toAt})
	if err != nil || logCount != 2 {
		t.Fatalf("CountLogs = %d, %v", logCount, err)
	}
	recentLogs, err := store.ListRecentLogsTail(ctx, "instance-1", 1)
	if err != nil || len(recentLogs) != 1 || recentLogs[0].At != baseTime.Add(2*time.Minute) {
		t.Fatalf("ListRecentLogsTail = %+v, %v", recentLogs, err)
	}

	auditEntries, err := store.ListAudit(ctx, AuditQuery{InstanceID: "instance-1", Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(auditEntries) != 2 {
		t.Fatalf("expected 2 audit entries after offset, got %d", len(auditEntries))
	}
	if auditEntries[0].Kind != "runtime_error" || auditEntries[1].Kind != "started" {
		t.Fatalf("unexpected paged audit entries: %+v", auditEntries)
	}
	auditCount, err := store.CountAudit(ctx, AuditQuery{InstanceID: "instance-1", Kind: "runtime_error", FromAt: &fromAt, ToAt: &toAt})
	if err != nil || auditCount != 1 {
		t.Fatalf("CountAudit = %d, %v", auditCount, err)
	}

	observation, ok, err := store.GetObservation(ctx, "instance-1")
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if !ok {
		t.Fatal("expected observation to exist")
	}
	if observation.ActualStatus != "STOPPED" {
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

func TestStoreInputAndMissingObservationBoundaries(t *testing.T) {
	store := newStoreForTest(t)
	ctx := t.Context()
	if err := store.AppendLog(ctx, LogEvent{}); err == nil || !strings.Contains(err.Error(), "instance id") {
		t.Fatalf("empty log error = %v", err)
	}
	if err := store.AppendLog(ctx, LogEvent{InstanceID: "instance-1"}); err == nil || !strings.Contains(err.Error(), "raw text") {
		t.Fatalf("empty log text error = %v", err)
	}
	if err := store.AppendAudit(ctx, AuditEvent{}); err == nil || !strings.Contains(err.Error(), "instance id") {
		t.Fatalf("empty audit error = %v", err)
	}
	if err := store.AppendAudit(ctx, AuditEvent{InstanceID: "instance-1"}); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("empty audit kind error = %v", err)
	}
	if err := store.UpsertObservation(ctx, ObservationSnapshot{}); err == nil || !strings.Contains(err.Error(), "instance id") {
		t.Fatalf("empty observation error = %v", err)
	}
	if observation, ok, err := store.GetObservation(ctx, " "); err != nil || ok || observation.InstanceID != "" {
		t.Fatalf("empty observation lookup = %#v, %v, %v", observation, ok, err)
	}
	if observation, ok, err := store.GetObservation(ctx, "missing"); err != nil || ok || observation.InstanceID != "" {
		t.Fatalf("missing observation lookup = %#v, %v, %v", observation, ok, err)
	}

	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO `+ObservationTable+` (instance_id, active_symbols_json) VALUES (?, ?)`,
		"corrupt", "{",
	); err != nil {
		t.Fatalf("insert corrupt observation: %v", err)
	}
	if _, _, err := store.GetObservation(ctx, "corrupt"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("corrupt observation error = %v", err)
	}
}

func TestStorePathPaginationAndClosedDatabaseBoundaries(t *testing.T) {
	overridePath := filepath.Join(t.TempDir(), "strategy-runtime-override.db")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", overridePath)
	if got := DeriveDBPath(settingsPath); got != overridePath {
		t.Fatalf("env DB path = %q", got)
	}
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")
	if got := DeriveDBPath("settings.json"); got != DefaultDBFilename {
		t.Fatalf("default DB path = %q", got)
	}
	if got, want := DeriveDBPath(settingsPath), filepath.Join(filepath.Dir(settingsPath), DefaultDBFilename); got != want {
		t.Fatalf("derived DB path = %q", got)
	}
	if NormalizePageSize(0) != DefaultPageSize || NormalizePageSize(MaxPageSize+1) != MaxPageSize || NormalizePageSize(17) != 17 {
		t.Fatal("page size normalization mismatch")
	}
	if NormalizeOffset(-1) != 0 || NormalizeOffset(3) != 3 {
		t.Fatal("offset normalization mismatch")
	}
	if (*Store)(nil).DB() != nil || (*Store)(nil).Close() != nil {
		t.Fatal("nil store boundary mismatch")
	}
	if _, err := NewStore(" "); err == nil {
		t.Fatal("empty database path should fail")
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(filepath.Join(parentFile, "runtime.db")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("invalid parent error = %v", err)
	}

	store := newStoreForTest(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	closedChecks := []struct {
		name string
		run  func() error
	}{
		{name: "append log", run: func() error { return store.AppendLog(t.Context(), LogEvent{InstanceID: "i", Raw: "x"}) }},
		{name: "append audit", run: func() error { return store.AppendAudit(t.Context(), AuditEvent{InstanceID: "i", Kind: "x"}) }},
		{name: "upsert observation", run: func() error { return store.UpsertObservation(t.Context(), ObservationSnapshot{InstanceID: "i"}) }},
		{name: "list logs", run: func() error { _, err := store.ListLogs(t.Context(), LogQuery{InstanceID: "i"}); return err }},
		{name: "count logs", run: func() error { _, err := store.CountLogs(t.Context(), LogQuery{InstanceID: "i"}); return err }},
		{name: "list audit", run: func() error { _, err := store.ListAudit(t.Context(), AuditQuery{InstanceID: "i"}); return err }},
		{name: "count audit", run: func() error { _, err := store.CountAudit(t.Context(), AuditQuery{InstanceID: "i"}); return err }},
		{name: "get observation", run: func() error { _, _, err := store.GetObservation(t.Context(), "i"); return err }},
	}
	for _, check := range closedChecks {
		if err := check.run(); err == nil {
			t.Fatalf("%s should fail after Close", check.name)
		}
	}
}

func newStoreForTest(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "strategy-runtime-test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})
	return store
}
