package backtest

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestNewFutuKLineStoreCreatesCompactSchema(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer store.Close()

	rows, err := store.DB().Query(`PRAGMA table_info(` + KLineTable + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		got = append(got, fmt.Sprintf("%s:%s:%d", name, dataType, pk))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info: %v", err)
	}

	want := expectedKLineSchemaColumns()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected futu_klines columns:\n got: %#v\nwant: %#v", got, want)
	}

	var ddl string
	if err := store.DB().QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, KLineTable).Scan(&ddl); err != nil {
		t.Fatalf("load sqlite_master ddl: %v", err)
	}
	upperDDL := strings.ToUpper(ddl)
	if !strings.Contains(upperDDL, "WITHOUT ROWID") {
		t.Fatalf("expected compact WITHOUT ROWID table, got DDL %q", ddl)
	}
	for _, removed := range []string{"GID", "EXCHANGE", "MARKET", "TURNOVER", "CLOSED", "LAST_TRADE_ID", "NUM_TRADES"} {
		if strings.Contains(upperDDL, removed) {
			t.Fatalf("expected compact schema to exclude %s, got DDL %q", removed, ddl)
		}
	}
}

func TestNewFutuKLineStoreRejectsLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	_, err = rawDB.Exec(`CREATE TABLE futu_klines (
		gid INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL DEFAULT 'futu',
		start_time TEXT NOT NULL,
		end_time TEXT NOT NULL,
		interval TEXT NOT NULL,
		symbol TEXT NOT NULL,
		market TEXT NOT NULL DEFAULT '',
		rehab_type TEXT NOT NULL DEFAULT 'forward',
		open REAL NOT NULL,
		high REAL NOT NULL,
		low REAL NOT NULL,
		close REAL NOT NULL DEFAULT 0.0,
		volume REAL NOT NULL DEFAULT 0.0,
		turnover REAL NOT NULL DEFAULT 0.0,
		closed INTEGER NOT NULL DEFAULT 1,
		last_trade_id INTEGER NOT NULL DEFAULT 0,
		num_trades INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		_ = rawDB.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewFutuKLineStore(dbPath)
	if err == nil {
		_ = store.Close()
		t.Fatal("expected legacy schema to be rejected")
	}
	if !strings.Contains(err.Error(), "rebuild the backtest database") {
		t.Fatalf("expected rebuild hint, got %v", err)
	}
}

func TestFutuKLineStoreRoundTripsCompactRows(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer store.Close()

	startAt := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	endAt := startAt.Add(time.Minute - time.Millisecond)
	input := types.KLine{
		StartTime: types.Time(startAt),
		EndTime:   types.Time(endAt),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(100.25),
		High:      fixedpoint.NewFromFloat(101.5),
		Low:       fixedpoint.NewFromFloat(99.75),
		Close:     fixedpoint.NewFromFloat(100.875),
		Volume:    fixedpoint.NewFromFloat(1234.5),
	}

	if err := store.InsertKLine(input, "forward"); err != nil {
		t.Fatalf("InsertKLine: %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, input.Symbol, input.Interval, endAt.Add(time.Second), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one kline, got %d", len(got))
	}

	loaded := got[0]
	if !loaded.StartTime.Time().Equal(startAt) {
		t.Fatalf("StartTime = %s, want %s", loaded.StartTime.Time(), startAt)
	}
	if !loaded.EndTime.Time().Equal(endAt) {
		t.Fatalf("EndTime = %s, want %s", loaded.EndTime.Time(), endAt)
	}
	if loaded.Interval != input.Interval {
		t.Fatalf("Interval = %s, want %s", loaded.Interval, input.Interval)
	}
	if loaded.Symbol != input.Symbol {
		t.Fatalf("Symbol = %s, want %s", loaded.Symbol, input.Symbol)
	}
	if loaded.Open.Compare(input.Open) != 0 || loaded.High.Compare(input.High) != 0 || loaded.Low.Compare(input.Low) != 0 || loaded.Close.Compare(input.Close) != 0 || loaded.Volume.Compare(input.Volume) != 0 {
		t.Fatalf("loaded kline values do not match input: got=%+v want=%+v", loaded, input)
	}
	if !loaded.Closed {
		t.Fatal("expected compact historical rows to always load as closed")
	}
	if loaded.LastTradeID != 0 || loaded.NumberOfTrades != 0 {
		t.Fatalf("expected removed trade metadata to read back as zero values, got lastTradeID=%d numTrades=%d", loaded.LastTradeID, loaded.NumberOfTrades)
	}

	var intervalValue, rehabValue, startTimeValue, endTimeValue int64
	var intervalType, rehabType, startType, endType string
	if err := store.DB().QueryRow(
		`SELECT interval, rehab_type, start_time, end_time, typeof(interval), typeof(rehab_type), typeof(start_time), typeof(end_time) FROM `+KLineTable+` WHERE symbol = ?`,
		input.Symbol,
	).Scan(&intervalValue, &rehabValue, &startTimeValue, &endTimeValue, &intervalType, &rehabType, &startType, &endType); err != nil {
		t.Fatalf("inspect stored row: %v", err)
	}

	if intervalValue != 60 {
		t.Fatalf("interval storage value = %d, want 60", intervalValue)
	}
	if rehabValue != rehabTypeForwardCode {
		t.Fatalf("rehab storage value = %d, want %d", rehabValue, rehabTypeForwardCode)
	}
	if startTimeValue != startAt.UnixMilli() {
		t.Fatalf("start_time storage value = %d, want %d", startTimeValue, startAt.UnixMilli())
	}
	if endTimeValue != endAt.UnixMilli() {
		t.Fatalf("end_time storage value = %d, want %d", endTimeValue, endAt.UnixMilli())
	}
	if intervalType != "integer" || rehabType != "integer" || startType != "integer" || endType != "integer" {
		t.Fatalf("expected integer storage classes, got interval=%s rehab=%s start=%s end=%s", intervalType, rehabType, startType, endType)
	}
}

func TestFutuKLineStoreFiltersByRehabType(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer store.Close()

	startAt := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	endAt := startAt.Add(time.Minute - time.Millisecond)
	forward := types.KLine{
		StartTime: types.Time(startAt),
		EndTime:   types.Time(endAt),
		Interval:  types.Interval1m,
		Symbol:    "US.TSLA",
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(101),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(100.5),
		Volume:    fixedpoint.NewFromFloat(1000),
	}
	none := forward
	none.Open = fixedpoint.NewFromFloat(80)
	none.High = fixedpoint.NewFromFloat(82)
	none.Low = fixedpoint.NewFromFloat(79)
	none.Close = fixedpoint.NewFromFloat(81.5)

	if err := store.InsertKLine(forward, "forward"); err != nil {
		t.Fatalf("InsertKLine(forward): %v", err)
	}
	if err := store.InsertKLine(none, "none"); err != nil {
		t.Fatalf("InsertKLine(none): %v", err)
	}

	store.SetRehabType("forward")
	gotForward, err := store.QueryKLine(nil, forward.Symbol, forward.Interval, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine(forward): %v", err)
	}
	if gotForward == nil {
		t.Fatal("expected forward rehab kline")
	}
	if gotForward.Close.Compare(forward.Close) != 0 {
		t.Fatalf("forward close = %s, want %s", gotForward.Close.String(), forward.Close.String())
	}
	forwardRows, err := store.QueryKLinesForward(nil, forward.Symbol, forward.Interval, startAt.Add(-time.Second), 10)
	if err != nil {
		t.Fatalf("QueryKLinesForward(forward): %v", err)
	}
	if len(forwardRows) != 1 {
		t.Fatalf("forward row count = %d, want 1", len(forwardRows))
	}
	if forwardRows[0].Close.Compare(forward.Close) != 0 {
		t.Fatalf("forward row close = %s, want %s", forwardRows[0].Close.String(), forward.Close.String())
	}

	store.SetRehabType("none")
	gotNone, err := store.QueryKLine(nil, none.Symbol, none.Interval, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine(none): %v", err)
	}
	if gotNone == nil {
		t.Fatal("expected none rehab kline")
	}
	if gotNone.Close.Compare(none.Close) != 0 {
		t.Fatalf("none close = %s, want %s", gotNone.Close.String(), none.Close.String())
	}
	noneRows, err := store.QueryKLinesForward(nil, none.Symbol, none.Interval, startAt.Add(-time.Second), 10)
	if err != nil {
		t.Fatalf("QueryKLinesForward(none): %v", err)
	}
	if len(noneRows) != 1 {
		t.Fatalf("none row count = %d, want 1", len(noneRows))
	}
	if noneRows[0].Close.Compare(none.Close) != 0 {
		t.Fatalf("none row close = %s, want %s", noneRows[0].Close.String(), none.Close.String())
	}

	store.SetRehabType("backward")
	gotBackward, err := store.QueryKLine(nil, none.Symbol, none.Interval, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine(backward): %v", err)
	}
	if gotBackward != nil {
		t.Fatalf("expected backward rehab query to be empty, got %+v", gotBackward)
	}
}

func TestFutuKLineStoreVerifyAcceptsOverlappingCoverage(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer store.Close()

	windowStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Second - time.Millisecond)
	insertVerifyCoverageRows(t, store, "US.NVDA", windowStart, func(interval types.Interval) bool {
		return true
	})

	if err := store.Verify(nil, []string{"US.NVDA"}, windowStart, windowEnd); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestFutuKLineStoreVerifyReportsMissingCoverage(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer store.Close()

	windowStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Second - time.Millisecond)
	insertVerifyCoverageRows(t, store, "US.NVDA", windowStart, func(interval types.Interval) bool {
		return interval != types.Interval1s
	})

	err := store.Verify(nil, []string{"US.NVDA"}, windowStart, windowEnd)
	if err == nil {
		t.Fatal("expected Verify() to report missing coverage")
	}
	if !strings.Contains(err.Error(), "US.NVDA") {
		t.Fatalf("expected Verify() error to mention symbol, got %v", err)
	}
	if !strings.Contains(err.Error(), string(types.Interval1s)) {
		t.Fatalf("expected Verify() error to mention missing 1s coverage, got %v", err)
	}
}

func TestIntervalStorageValueCoversSupportedIntervals(t *testing.T) {
	for interval := range types.SupportedIntervals {
		value := intervalStorageValue(interval)
		if value <= 0 {
			t.Fatalf("interval %s encoded to non-positive value %d", interval, value)
		}

		restored, err := intervalFromStorageValue(value)
		if err != nil {
			t.Fatalf("interval %s failed to round-trip from %d: %v", interval, value, err)
		}
		if restored != interval {
			t.Fatalf("interval round-trip mismatch: got %s, want %s", restored, interval)
		}
	}
}

func newTestFutuKLineStore(t *testing.T) *FutuKLineStore {
	t.Helper()

	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "backtest.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	return store
}

func insertVerifyCoverageRows(t *testing.T, store *FutuKLineStore, symbol string, windowStart time.Time, include func(interval types.Interval) bool) {
	t.Helper()

	for interval := range types.SupportedIntervals {
		if include != nil && !include(interval) {
			continue
		}
		endAt := windowStart.Add(interval.Duration()).Add(-time.Millisecond)
		kline := types.KLine{
			StartTime: types.Time(windowStart),
			EndTime:   types.Time(endAt),
			Interval:  interval,
			Symbol:    symbol,
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(101),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(100.5),
			Volume:    fixedpoint.NewFromFloat(1000),
		}
		if err := store.InsertKLine(kline, "forward"); err != nil {
			t.Fatalf("InsertKLine(%s): %v", interval, err)
		}
	}
}
