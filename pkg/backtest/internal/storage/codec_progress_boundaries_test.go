package storage

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jmoiron/sqlx"
)

func TestSyncProgressLifecycleAndNilReceiverBoundaries(t *testing.T) {
	var nilProgress *SyncProgress
	if nilProgress.Snapshot() != nil {
		t.Fatal("nil Snapshot() != nil")
	}
	now := time.Date(2026, time.July, 2, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	nilProgress.SetRunning(1, now)
	nilProgress.BeginInterval(types.Interval1m, 0, now)
	nilProgress.CompleteInterval(1)
	nilProgress.IncrementCompletedBatches(now)
	nilProgress.IncrementRetries()
	nilProgress.MarkFailed(errors.New("ignored"), now)
	nilProgress.MarkCancelled(now)
	nilProgress.MarkCompleted(1, now)

	progress := NewSyncProgress("task-1", "US.AAPL", now)
	progress.SetRunning(2, now.Add(time.Minute))
	progress.BeginInterval(types.Interval5m, 0, now.Add(2*time.Minute))
	progress.IncrementCompletedBatches(now.Add(3 * time.Minute))
	progress.IncrementRetries()
	progress.CompleteInterval(1)
	progress.MarkFailed(nil, now.Add(4*time.Minute))
	if progress.Status != "failed" || progress.Error != "" || progress.CompletedIntervals != 1 || progress.CompletedBatches != 1 || progress.Retries != 1 {
		t.Fatalf("failed progress = %#v", progress)
	}
	progress.MarkCancelled(now.Add(5 * time.Minute))
	if progress.Status != "cancelled" {
		t.Fatalf("cancelled status = %q", progress.Status)
	}
	progress.MarkCompleted(2, now.Add(6*time.Minute))
	snapshot := progress.Snapshot()
	if snapshot.Status != "completed" || snapshot.CompletedIntervals != 2 || snapshot.CurrentInterval != "" || snapshot.StartedAt == "" || snapshot.UpdatedAt == "" {
		t.Fatalf("completed snapshot = %#v", snapshot)
	}
	snapshot.Status = "mutated"
	if progress.Status != "completed" {
		t.Fatal("Snapshot() returned mutable alias")
	}
}

func TestStoredFixedDecimalBoundaryMatrix(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{"", "0"},
		{"+1.25", "1.25"},
		{"-1.25", "-1.25"},
		{".5", "0.5"},
	}
	for _, tt := range valid {
		value, ok := parseStoredFixedDecimal(tt.input)
		if !ok || value.String() != tt.want {
			t.Fatalf("parseStoredFixedDecimal(%q) = (%s, %t), want %s", tt.input, value.String(), ok, tt.want)
		}
	}
	invalid := []string{
		"+",
		"-",
		"1.2.3",
		"1x",
		"1.123456789",
		"999999999999999999999999999999999999",
		"92233720368.54775808",
	}
	for _, input := range invalid {
		if _, ok := parseStoredFixedDecimal(input); ok {
			t.Fatalf("parseStoredFixedDecimal(%q) ok = true", input)
		}
	}
	if got := rawBytesToString(nil); got != "" {
		t.Fatalf("rawBytesToString(nil) = %q", got)
	}
}

func TestStoredKLineScanningClassifiesMissingMalformedAndValidRows(t *testing.T) {
	store := newTestKLineStore(t)
	missing, err := scanKLine(store.db.QueryRow(`SELECT 1, 2, '1', '2', '0', '1', '10' WHERE 0`), "US.AAPL", types.Interval1m)
	if err != nil || missing != nil {
		t.Fatalf("scanKLine(missing) = (%#v, %v)", missing, err)
	}
	if _, err := scanKLine(store.db.QueryRow(`SELECT 1`), "US.AAPL", types.Interval1m); err == nil {
		t.Fatal("scanKLine(short row) error = nil")
	}

	fields := []string{"open", "high", "low", "close", "volume"}
	for invalidIndex, field := range fields {
		values := []any{"1", "2", "0", "1", "10"}
		values[invalidIndex] = "not-a-number"
		row := store.db.QueryRow(`SELECT 1, 2, ?, ?, ?, ?, ?`, values...)
		if _, err := scanKLine(row, "US.AAPL", types.Interval1m); err == nil {
			t.Fatalf("scanKLine(invalid %s) error = nil", field)
		}
	}

	row := store.db.QueryRow(`SELECT 1000, 2000, '1', '2', '0.5', '1.5', '10'`)
	kline, err := scanKLine(row, "US.AAPL", types.Interval1m)
	if err != nil || kline == nil || kline.Symbol != "US.AAPL" || !kline.Closed || kline.Close.String() != "1.5" {
		t.Fatalf("scanKLine(valid) = (%#v, %v)", kline, err)
	}

	rows, err := store.db.Query(`SELECT 1`)
	if err != nil {
		t.Fatalf("query short rows: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close short rows: %v", err)
		}
	}()
	if _, err := scanKLinesWithCapacity(rows, "US.AAPL", types.Interval1m, 1); err == nil {
		t.Fatal("scanKLinesWithCapacity(short row) error = nil")
	}
}

func TestStorageSchemaFallbackBoundaries(t *testing.T) {
	if RehabTypeName(1) != "forward" || RehabTypeName(2) != "backward" || RehabTypeName(99) != "none" {
		t.Fatalf("RehabTypeName mapping changed")
	}
	if sanitizeIdentifierComponent("") != "value" || sanitizeIdentifierComponent("---") != "value" || sanitizeIdentifierComponent("A  B") != "a_b" {
		t.Fatalf("sanitizeIdentifierComponent fallback changed")
	}
	at := time.Date(2026, time.July, 2, 9, 31, 45, 0, time.FixedZone("CST", 8*60*60))
	invalid := types.Interval("invalid")
	if got := alignTimeToIntervalStart(at, invalid); !got.Equal(at.UTC()) {
		t.Fatalf("alignTimeToIntervalStart(invalid) = %s", got)
	}
	if got := firstClosedKLineEndAtOrAfter(at, invalid); !got.Equal(at.UTC()) {
		t.Fatalf("firstClosedKLineEndAtOrAfter(invalid) = %s", got)
	}
	if got := latestClosedKLineEndAtOrBefore(at, invalid); !got.Equal(at.UTC()) {
		t.Fatalf("latestClosedKLineEndAtOrBefore(invalid) = %s", got)
	}
}

func TestNewFutuKLineStoreRejectsBlankAndLegacyDatabase(t *testing.T) {
	if store, err := NewFutuKLineStore(" \t "); err == nil || store != nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("NewFutuKLineStore(blank) = (%#v, %v)", store, err)
	}

	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE legacy (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	store, err := NewFutuKLineStore(path)
	if err == nil || store != nil || !strings.Contains(err.Error(), "validate sqlite backtest store") {
		t.Fatalf("NewFutuKLineStore(legacy) = (%#v, %v)", store, err)
	}
}
