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
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	var baseTableCount int
	if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, KLineTable).Scan(&baseTableCount); err != nil {
		t.Fatalf("count base tables: %v", err)
	}
	if baseTableCount != 0 {
		t.Fatalf("expected no shared %s table, found %d", KLineTable, baseTableCount)
	}

	input := types.KLine{
		StartTime: types.Time(time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.May, 26, 9, 30, 59, 999000000, time.UTC)),
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

	tableName := KLineTableName(input.Symbol, input.Interval, "forward")

	rows, err := store.DB().QueryContext(t.Context(), `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer func() { jftradeCheckTestError(t, rows.Close()) }()

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
		t.Fatalf("unexpected local_klines columns:\n got: %#v\nwant: %#v", got, want)
	}

	var ddl string
	if err := store.DB().QueryRowContext(t.Context(), `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&ddl); err != nil {
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

func TestNewFutuKLineStoreUsesSeparateTablesPerDimension(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	rows := []struct {
		kline     types.KLine
		rehabType string
	}{
		{types.KLine{StartTime: types.Time(baseStart), EndTime: types.Time(baseStart.Add(time.Minute - time.Millisecond)), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(101), Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(100.5), Volume: fixedpoint.NewFromFloat(1000)}, "forward"},
		{types.KLine{StartTime: types.Time(baseStart), EndTime: types.Time(baseStart.Add(5*time.Minute - time.Millisecond)), Interval: types.Interval5m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(102), Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(101), Volume: fixedpoint.NewFromFloat(5000)}, "forward"},
		{types.KLine{StartTime: types.Time(baseStart), EndTime: types.Time(baseStart.Add(time.Minute - time.Millisecond)), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(80), High: fixedpoint.NewFromFloat(81), Low: fixedpoint.NewFromFloat(79), Close: fixedpoint.NewFromFloat(80.5), Volume: fixedpoint.NewFromFloat(900)}, "none"},
	}
	for _, row := range rows {
		if err := store.InsertKLine(row.kline, row.rehabType); err != nil {
			t.Fatalf("InsertKLine(%s, %s): %v", row.kline.Interval, row.rehabType, err)
		}
	}

	for _, row := range rows {
		tableName := KLineTableName(row.kline.Symbol, row.kline.Interval, row.rehabType)
		var count int
		if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("count table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist once, got %d", tableName, count)
		}
	}
}

func TestFutuKLineStoreSeparatesScopedSyncVersions(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	startAt := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	endAt := startAt.Add(time.Minute - time.Millisecond)
	regular := types.KLine{
		StartTime: types.Time(startAt),
		EndTime:   types.Time(endAt),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(101),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(100.5),
		Volume:    fixedpoint.NewFromFloat(1000),
	}
	extended := regular
	extended.Open = fixedpoint.NewFromFloat(80)
	extended.High = fixedpoint.NewFromFloat(81)
	extended.Low = fixedpoint.NewFromFloat(79)
	extended.Close = fixedpoint.NewFromFloat(80.5)

	store.SetWriteSessionScope(KLineSessionScopeRegular)
	if err := store.InsertKLine(regular, "forward"); err != nil {
		t.Fatalf("InsertKLine(regular): %v", err)
	}
	store.SetWriteSessionScope(KLineSessionScopeExtended)
	if err := store.InsertKLine(extended, "forward"); err != nil {
		t.Fatalf("InsertKLine(extended): %v", err)
	}

	for _, sessionScope := range []string{KLineSessionScopeRegular, KLineSessionScopeExtended} {
		tableName := KLineTableNameForSessionScope(regular.Symbol, regular.Interval, "forward", sessionScope)
		var count int
		if err := store.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
			t.Fatalf("count scoped table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("expected scoped table %s to exist once, got %d", tableName, count)
		}
	}

	store.SetRehabType("forward")
	store.SetReadSessionScope(KLineSessionScopeRegular)
	gotRegular, err := store.QueryKLine(nil, regular.Symbol, regular.Interval, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine(regular): %v", err)
	}
	if gotRegular == nil || gotRegular.Close.Compare(regular.Close) != 0 {
		t.Fatalf("regular scoped close = %v, want %s", gotRegular, regular.Close.String())
	}

	store.SetReadSessionScope(KLineSessionScopeExtended)
	gotExtended, err := store.QueryKLine(nil, extended.Symbol, extended.Interval, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine(extended): %v", err)
	}
	if gotExtended == nil || gotExtended.Close.Compare(extended.Close) != 0 {
		t.Fatalf("extended scoped close = %v, want %s", gotExtended, extended.Close.String())
	}
}

func TestFutuKLineStoreRegularScopeFallsBackToLegacyWhenCoverageIsIncomplete(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	firstEnd := baseStart.Add(time.Minute - time.Millisecond)
	secondStart := baseStart.Add(time.Minute)
	secondEnd := secondStart.Add(time.Minute - time.Millisecond)

	legacyRows := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(firstEnd),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(101),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(100.5),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(secondStart),
			EndTime:   types.Time(secondEnd),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(101),
			High:      fixedpoint.NewFromFloat(102),
			Low:       fixedpoint.NewFromFloat(100),
			Close:     fixedpoint.NewFromFloat(101.5),
			Volume:    fixedpoint.NewFromFloat(1001),
		},
	}
	if err := store.InsertKLines(legacyRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(legacy): %v", err)
	}

	store.SetWriteSessionScope(KLineSessionScopeRegular)
	if err := store.InsertKLine(types.KLine{
		StartTime: types.Time(baseStart),
		EndTime:   types.Time(firstEnd),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(80),
		High:      fixedpoint.NewFromFloat(81),
		Low:       fixedpoint.NewFromFloat(79),
		Close:     fixedpoint.NewFromFloat(80.5),
		Volume:    fixedpoint.NewFromFloat(900),
	}, "forward"); err != nil {
		t.Fatalf("InsertKLine(regular partial): %v", err)
	}

	store.SetRehabType("forward")
	store.SetReadSessionScope(KLineSessionScopeRegular)
	got, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, firstEnd, 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryKLinesForward row count = %d, want 2", len(got))
	}
	if got[0].Close.Compare(legacyRows[0].Close) != 0 || got[1].Close.Compare(legacyRows[1].Close) != 0 {
		t.Fatalf("expected fallback to legacy rows, got closes %s and %s", got[0].Close.String(), got[1].Close.String())
	}
}

func TestFutuKLineStoreQueryKLinesChPrefersRegularScopedRangeWhenCoverageIsComplete(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	firstEnd := baseStart.Add(time.Minute - time.Millisecond)
	secondStart := baseStart.Add(time.Minute)
	secondEnd := secondStart.Add(time.Minute - time.Millisecond)

	legacyRows := []types.KLine{
		{StartTime: types.Time(baseStart), EndTime: types.Time(firstEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(90), High: fixedpoint.NewFromFloat(91), Low: fixedpoint.NewFromFloat(89), Close: fixedpoint.NewFromFloat(90.5), Volume: fixedpoint.NewFromFloat(900)},
		{StartTime: types.Time(secondStart), EndTime: types.Time(secondEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(91), High: fixedpoint.NewFromFloat(92), Low: fixedpoint.NewFromFloat(90), Close: fixedpoint.NewFromFloat(91.5), Volume: fixedpoint.NewFromFloat(901)},
	}
	if err := store.InsertKLines(legacyRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(legacy): %v", err)
	}

	store.SetWriteSessionScope(KLineSessionScopeRegular)
	regularRows := []types.KLine{
		{StartTime: types.Time(baseStart), EndTime: types.Time(firstEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(101), Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(100.5), Volume: fixedpoint.NewFromFloat(1000)},
		{StartTime: types.Time(secondStart), EndTime: types.Time(secondEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(101), High: fixedpoint.NewFromFloat(102), Low: fixedpoint.NewFromFloat(100), Close: fixedpoint.NewFromFloat(101.5), Volume: fixedpoint.NewFromFloat(1001)},
	}
	if err := store.InsertKLines(regularRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(regular): %v", err)
	}

	store.SetRehabType("forward")
	store.SetReadSessionScope(KLineSessionScopeRegular)
	ch, errCh := store.QueryKLinesCh(baseStart, secondEnd, nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m})
	got, err := collectKLinesFromChannels(ch, errCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryKLinesCh row count = %d, want 2", len(got))
	}
	if got[0].Close.Compare(regularRows[0].Close) != 0 || got[1].Close.Compare(regularRows[1].Close) != 0 {
		t.Fatalf("expected regular scoped channel rows, got closes %s and %s", got[0].Close.String(), got[1].Close.String())
	}
}

func TestFutuKLineStoreQueryKLinesChFallsBackToLegacyWhenRegularRangeIsIncomplete(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	firstEnd := baseStart.Add(time.Minute - time.Millisecond)
	secondStart := baseStart.Add(time.Minute)
	secondEnd := secondStart.Add(time.Minute - time.Millisecond)

	legacyRows := []types.KLine{
		{StartTime: types.Time(baseStart), EndTime: types.Time(firstEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(90), High: fixedpoint.NewFromFloat(91), Low: fixedpoint.NewFromFloat(89), Close: fixedpoint.NewFromFloat(90.5), Volume: fixedpoint.NewFromFloat(900)},
		{StartTime: types.Time(secondStart), EndTime: types.Time(secondEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(91), High: fixedpoint.NewFromFloat(92), Low: fixedpoint.NewFromFloat(90), Close: fixedpoint.NewFromFloat(91.5), Volume: fixedpoint.NewFromFloat(901)},
	}
	if err := store.InsertKLines(legacyRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(legacy): %v", err)
	}

	store.SetWriteSessionScope(KLineSessionScopeRegular)
	if err := store.InsertKLine(types.KLine{StartTime: types.Time(baseStart), EndTime: types.Time(firstEnd), Interval: types.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(101), Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(100.5), Volume: fixedpoint.NewFromFloat(1000)}, "forward"); err != nil {
		t.Fatalf("InsertKLine(regular partial): %v", err)
	}

	store.SetRehabType("forward")
	store.SetReadSessionScope(KLineSessionScopeRegular)
	ch, errCh := store.QueryKLinesCh(baseStart, secondEnd, nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m})
	got, err := collectKLinesFromChannels(ch, errCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryKLinesCh row count = %d, want 2", len(got))
	}
	if got[0].Close.Compare(legacyRows[0].Close) != 0 || got[1].Close.Compare(legacyRows[1].Close) != 0 {
		t.Fatalf("expected legacy fallback channel rows, got closes %s and %s", got[0].Close.String(), got[1].Close.String())
	}
}

func TestFutuKLineStoreRoundTripsCompactRows(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

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

	var startTimeValue, endTimeValue int64
	var openValue, highValue, lowValue, closeValue, volumeValue string
	var startType, endType, openType, highType, lowType, closeType, volumeType string
	tableName := KLineTableName(input.Symbol, input.Interval, "forward")
	if err := store.DB().QueryRowContext(t.Context(),
		`SELECT start_time, end_time, open, high, low, close, volume, typeof(start_time), typeof(end_time), typeof(open), typeof(high), typeof(low), typeof(close), typeof(volume) FROM `+tableName+` LIMIT 1`,
	).Scan(&startTimeValue, &endTimeValue, &openValue, &highValue, &lowValue, &closeValue, &volumeValue, &startType, &endType, &openType, &highType, &lowType, &closeType, &volumeType); err != nil {
		t.Fatalf("inspect stored row: %v", err)
	}
	if startTimeValue != startAt.UnixMilli() {
		t.Fatalf("start_time storage value = %d, want %d", startTimeValue, startAt.UnixMilli())
	}
	if endTimeValue != endAt.UnixMilli() {
		t.Fatalf("end_time storage value = %d, want %d", endTimeValue, endAt.UnixMilli())
	}
	if startType != "integer" || endType != "integer" {
		t.Fatalf("expected integer storage classes, got start=%s end=%s", startType, endType)
	}
	if openType != "text" || highType != "text" || lowType != "text" || closeType != "text" || volumeType != "text" {
		t.Fatalf("expected text storage classes, got open=%s high=%s low=%s close=%s volume=%s", openType, highType, lowType, closeType, volumeType)
	}
	if openValue != input.Open.String() || highValue != input.High.String() || lowValue != input.Low.String() || closeValue != input.Close.String() || volumeValue != input.Volume.String() {
		t.Fatalf("stored decimal text does not match input strings: got open=%s high=%s low=%s close=%s volume=%s", openValue, highValue, lowValue, closeValue, volumeValue)
	}
}

func TestFutuKLineStoreFiltersByRehabType(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

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
	defer func() { jftradeCheckTestError(t, store.Close()) }()

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
	defer func() { jftradeCheckTestError(t, store.Close()) }()

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

func TestFutuKLineStoreSynthesizesFiveMinuteFromOneMinute(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	minuteKLines := make([]types.KLine, 0, 5)
	prices := []float64{100, 101, 99.5, 102, 101.5}
	for index, closePrice := range prices {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		minuteKLines = append(minuteKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100 + float64(index)*0.1),
			High:      fixedpoint.NewFromFloat(closePrice + 0.5),
			Low:       fixedpoint.NewFromFloat(closePrice - 0.5),
			Close:     fixedpoint.NewFromFloat(closePrice),
			Volume:    fixedpoint.NewFromFloat(float64(100 + index)),
		})
	}
	if err := store.InsertKLines(minuteKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval5m, baseStart.Add(5*time.Minute), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(5m): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized 5m kline, got %d", len(got))
	}
	if got[0].StartTime.Time() != baseStart {
		t.Fatalf("synthesized start = %s, want %s", got[0].StartTime.Time(), baseStart)
	}
	if got[0].Interval != types.Interval5m {
		t.Fatalf("synthesized interval = %s, want 5m", got[0].Interval)
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(102.5)) != 0 {
		t.Fatalf("synthesized high = %s, want 102.5", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(99)) != 0 {
		t.Fatalf("synthesized low = %s, want 99", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(101.5)) != 0 {
		t.Fatalf("synthesized close = %s, want 101.5", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(510)) != 0 {
		t.Fatalf("synthesized volume = %s, want 510", got[0].Volume.String())
	}
}

func TestFutuKLineStoreSynthesizesFifteenMinuteFromFiveMinute(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	fiveMinuteKLines := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(baseStart.Add(5*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(103),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(102),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(baseStart.Add(5 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(10*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(102),
			High:      fixedpoint.NewFromFloat(105),
			Low:       fixedpoint.NewFromFloat(101),
			Close:     fixedpoint.NewFromFloat(104),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
		{
			StartTime: types.Time(baseStart.Add(10 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(15*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(104),
			High:      fixedpoint.NewFromFloat(106),
			Low:       fixedpoint.NewFromFloat(100),
			Close:     fixedpoint.NewFromFloat(105),
			Volume:    fixedpoint.NewFromFloat(1400),
		},
	}
	if err := store.InsertKLines(fiveMinuteKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(5m): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval15m, baseStart.Add(15*time.Minute), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(15m): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized 15m kline, got %d", len(got))
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("synthesized open = %s, want 100", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(106)) != 0 {
		t.Fatalf("synthesized high = %s, want 106", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(99)) != 0 {
		t.Fatalf("synthesized low = %s, want 99", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(105)) != 0 {
		t.Fatalf("synthesized close = %s, want 105", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(3600)) != 0 {
		t.Fatalf("synthesized volume = %s, want 3600", got[0].Volume.String())
	}
}

func TestFutuKLineStoreSynthesizesTwoHourFromUSSessionAwareBuckets(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	halfHourKLines := make([]types.KLine, 0, 13)
	for index := range 13 {
		startAt := baseStart.Add(time.Duration(index) * 30 * time.Minute)
		openValue := 100 + float64(index)
		halfHourKLines = append(halfHourKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(30*time.Minute - time.Millisecond)),
			Interval:  types.Interval30m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(openValue + 1),
			Low:       fixedpoint.NewFromFloat(openValue - 1),
			Close:     fixedpoint.NewFromFloat(openValue + 0.5),
			Volume:    fixedpoint.NewFromFloat(100 + float64(index)*10),
		})
	}
	if err := store.InsertKLines(halfHourKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(30m): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval2h, time.Date(2026, time.May, 26, 20, 0, 0, 0, time.UTC), 4)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(2h): %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected four synthesized 2h klines, got %d", len(got))
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)) {
		t.Fatalf("first 2h start = %s, want 2026-05-26T13:30:00Z", got[0].StartTime.Time())
	}
	if !got[1].StartTime.Time().Equal(time.Date(2026, time.May, 26, 15, 30, 0, 0, time.UTC)) {
		t.Fatalf("second 2h start = %s, want 2026-05-26T15:30:00Z", got[1].StartTime.Time())
	}
	if !got[3].StartTime.Time().Equal(time.Date(2026, time.May, 26, 19, 30, 0, 0, time.UTC)) {
		t.Fatalf("last 2h start = %s, want 2026-05-26T19:30:00Z", got[3].StartTime.Time())
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("first 2h open = %s, want 100", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(104)) != 0 {
		t.Fatalf("first 2h high = %s, want 104", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(99)) != 0 {
		t.Fatalf("first 2h low = %s, want 99", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(103.5)) != 0 {
		t.Fatalf("first 2h close = %s, want 103.5", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(460)) != 0 {
		t.Fatalf("first 2h volume = %s, want 460", got[0].Volume.String())
	}
	if !got[3].EndTime.Time().Equal(time.Date(2026, time.May, 26, 19, 59, 59, 999000000, time.UTC)) {
		t.Fatalf("last 2h end = %s, want 2026-05-26T19:59:59.999Z", got[3].EndTime.Time())
	}
	if got[3].Volume.Compare(fixedpoint.NewFromFloat(220)) != 0 {
		t.Fatalf("last 2h volume = %s, want 220", got[3].Volume.String())
	}
}

func TestFutuKLineStoreSynthesizesTwoHourAcrossHKLunchBreak(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	starts := []time.Time{
		time.Date(2026, time.May, 26, 1, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 2, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 2, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 3, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 3, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 5, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 5, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 6, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 6, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 26, 7, 30, 0, 0, time.UTC),
	}
	halfHourKLines := make([]types.KLine, 0, len(starts))
	for index, startAt := range starts {
		openValue := 200 + float64(index)
		halfHourKLines = append(halfHourKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(30*time.Minute - time.Millisecond)),
			Interval:  types.Interval30m,
			Symbol:    "HK.00700",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(openValue + 1),
			Low:       fixedpoint.NewFromFloat(openValue - 1),
			Close:     fixedpoint.NewFromFloat(openValue + 0.5),
			Volume:    fixedpoint.NewFromFloat(100 + float64(index)*10),
		})
	}
	if err := store.InsertKLines(halfHourKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(HK 30m): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "HK.00700", types.Interval2h, time.Date(2026, time.May, 26, 8, 0, 0, 0, time.UTC), 4)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(HK 2h): %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected four synthesized HK 2h klines, got %d", len(got))
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.May, 26, 1, 30, 0, 0, time.UTC)) {
		t.Fatalf("HK first 2h start = %s, want 2026-05-26T01:30:00Z", got[0].StartTime.Time())
	}
	if !got[1].StartTime.Time().Equal(time.Date(2026, time.May, 26, 3, 30, 0, 0, time.UTC)) {
		t.Fatalf("HK second 2h start = %s, want 2026-05-26T03:30:00Z", got[1].StartTime.Time())
	}
	if !got[2].StartTime.Time().Equal(time.Date(2026, time.May, 26, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("HK third 2h start = %s, want 2026-05-26T05:00:00Z", got[2].StartTime.Time())
	}
	if !got[3].StartTime.Time().Equal(time.Date(2026, time.May, 26, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("HK fourth 2h start = %s, want 2026-05-26T07:00:00Z", got[3].StartTime.Time())
	}
	if got[2].Volume.Compare(fixedpoint.NewFromFloat(660)) != 0 {
		t.Fatalf("HK afternoon 2h volume = %s, want 660", got[2].Volume.String())
	}
	if !got[3].EndTime.Time().Equal(time.Date(2026, time.May, 26, 7, 59, 59, 999000000, time.UTC)) {
		t.Fatalf("HK final 2h end = %s, want 2026-05-26T07:59:59.999Z", got[3].EndTime.Time())
	}
}

func TestFutuKLineStoreSynthesizesTwoHourForwardFromUSSessionAwareBuckets(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	halfHourKLines := make([]types.KLine, 0, 13)
	for index := range 13 {
		startAt := baseStart.Add(time.Duration(index) * 30 * time.Minute)
		openValue := 100 + float64(index)
		halfHourKLines = append(halfHourKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(30*time.Minute - time.Millisecond)),
			Interval:  types.Interval30m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(openValue + 1),
			Low:       fixedpoint.NewFromFloat(openValue - 1),
			Close:     fixedpoint.NewFromFloat(openValue + 0.5),
			Volume:    fixedpoint.NewFromFloat(100 + float64(index)*10),
		})
	}
	if err := store.InsertKLines(halfHourKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(30m forward 2h): %v", err)
	}

	got, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval2h, time.Date(2026, time.May, 26, 14, 0, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward(2h): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two synthesized forward 2h klines, got %d", len(got))
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)) {
		t.Fatalf("forward first 2h start = %s, want 2026-05-26T13:30:00Z", got[0].StartTime.Time())
	}
	if !got[1].StartTime.Time().Equal(time.Date(2026, time.May, 26, 15, 30, 0, 0, time.UTC)) {
		t.Fatalf("forward second 2h start = %s, want 2026-05-26T15:30:00Z", got[1].StartTime.Time())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(460)) != 0 {
		t.Fatalf("forward first 2h volume = %s, want 460", got[0].Volume.String())
	}
	if got[1].Volume.Compare(fixedpoint.NewFromFloat(620)) != 0 {
		t.Fatalf("forward second 2h volume = %s, want 620", got[1].Volume.String())
	}
}

func TestFutuKLineStoreQueryBackwardSessionAwarePaginationMatchesRangeSeries(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseRows := buildBenchmarkSessionAwareHalfHourKLines(time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC), 40)
	if err := store.InsertKLines(baseRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(session-aware pagination): %v", err)
	}

	until := baseRows[len(baseRows)-1].EndTime.Time().Add(time.Second)
	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval2h, until, 64)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(session-aware pagination): %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("QueryKLinesBackward(session-aware pagination) len = %d, want 64", len(got))
	}

	ch, errCh := store.QueryKLinesCh(baseRows[0].StartTime.Time(), until, nil, []string{"US.AAPL"}, []types.Interval{types.Interval2h})
	var fullSeries []types.KLine
	for row := range ch {
		fullSeries = append(fullSeries, row)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("QueryKLinesCh(session-aware pagination): %v", err)
	}
	if len(fullSeries) < len(got) {
		t.Fatalf("full session-aware series len = %d, want at least %d", len(fullSeries), len(got))
	}

	want := fullSeries[len(fullSeries)-len(got):]
	for index := range got {
		if !got[index].StartTime.Time().Equal(want[index].StartTime.Time()) {
			t.Fatalf("row %d start = %s, want %s", index, got[index].StartTime.Time(), want[index].StartTime.Time())
		}
		if !got[index].EndTime.Time().Equal(want[index].EndTime.Time()) {
			t.Fatalf("row %d end = %s, want %s", index, got[index].EndTime.Time(), want[index].EndTime.Time())
		}
		if got[index].Open.Compare(want[index].Open) != 0 {
			t.Fatalf("row %d open = %s, want %s", index, got[index].Open.String(), want[index].Open.String())
		}
		if got[index].Close.Compare(want[index].Close) != 0 {
			t.Fatalf("row %d close = %s, want %s", index, got[index].Close.String(), want[index].Close.String())
		}
		if got[index].Volume.Compare(want[index].Volume) != 0 {
			t.Fatalf("row %d volume = %s, want %s", index, got[index].Volume.String(), want[index].Volume.String())
		}
	}
}

func TestFutuKLineStoreSynthesizesDailyFromUSRegularTradingHours(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 13, 0, 0, 0, time.UTC)
	oneHourKLines := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(baseStart.Add(time.Hour - time.Millisecond)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(90),
			High:      fixedpoint.NewFromFloat(91),
			Low:       fixedpoint.NewFromFloat(89),
			Close:     fixedpoint.NewFromFloat(90.5),
			Volume:    fixedpoint.NewFromFloat(500),
		},
		{
			StartTime: types.Time(baseStart.Add(30 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(90*time.Minute - time.Millisecond)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(104),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(103),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(baseStart.Add(90 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(150*time.Minute - time.Millisecond)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(103),
			High:      fixedpoint.NewFromFloat(106),
			Low:       fixedpoint.NewFromFloat(102),
			Close:     fixedpoint.NewFromFloat(105),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
	}
	if err := store.InsertKLines(oneHourKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(1h): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1d, time.Date(2026, time.May, 27, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(1d): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized daily kline, got %d", len(got))
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("daily open = %s, want 100", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(106)) != 0 {
		t.Fatalf("daily high = %s, want 106", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(99)) != 0 {
		t.Fatalf("daily low = %s, want 99", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(105)) != 0 {
		t.Fatalf("daily close = %s, want 105", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(2200)) != 0 {
		t.Fatalf("daily volume = %s, want 2200", got[0].Volume.String())
	}
}

func TestFutuKLineStoreSynthesizesDailyAcrossHKLunchBreak(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	morningStart := time.Date(2026, time.May, 26, 1, 30, 0, 0, time.UTC)
	hkKLines := []types.KLine{
		{
			StartTime: types.Time(morningStart),
			EndTime:   types.Time(morningStart.Add(time.Hour - time.Millisecond)),
			Interval:  types.Interval1h,
			Symbol:    "HK.00700",
			Open:      fixedpoint.NewFromFloat(400),
			High:      fixedpoint.NewFromFloat(405),
			Low:       fixedpoint.NewFromFloat(399),
			Close:     fixedpoint.NewFromFloat(404),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(morningStart.Add(time.Hour)),
			EndTime:   types.Time(morningStart.Add(2*time.Hour - time.Millisecond)),
			Interval:  types.Interval1h,
			Symbol:    "HK.00700",
			Open:      fixedpoint.NewFromFloat(404),
			High:      fixedpoint.NewFromFloat(406),
			Low:       fixedpoint.NewFromFloat(403),
			Close:     fixedpoint.NewFromFloat(405),
			Volume:    fixedpoint.NewFromFloat(1100),
		},
		{
			StartTime: types.Time(time.Date(2026, time.May, 26, 5, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 26, 5, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "HK.00700",
			Open:      fixedpoint.NewFromFloat(405),
			High:      fixedpoint.NewFromFloat(410),
			Low:       fixedpoint.NewFromFloat(404),
			Close:     fixedpoint.NewFromFloat(409),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
	}
	if err := store.InsertKLines(hkKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(HK 1h): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "HK.00700", types.Interval1d, time.Date(2026, time.May, 27, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(HK 1d): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized HK daily kline, got %d", len(got))
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(400)) != 0 {
		t.Fatalf("HK daily open = %s, want 400", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(410)) != 0 {
		t.Fatalf("HK daily high = %s, want 410", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(399)) != 0 {
		t.Fatalf("HK daily low = %s, want 399", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(409)) != 0 {
		t.Fatalf("HK daily close = %s, want 409", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(3300)) != 0 {
		t.Fatalf("HK daily volume = %s, want 3300", got[0].Volume.String())
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.May, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("HK daily start = %s, want 2026-05-26T00:00:00Z", got[0].StartTime.Time())
	}
	if !got[0].EndTime.Time().Equal(time.Date(2026, time.May, 26, 23, 59, 59, 999000000, time.UTC)) {
		t.Fatalf("HK daily end = %s, want 2026-05-26T23:59:59.999Z", got[0].EndTime.Time())
	}
}

func TestFutuKLineStoreSynthesizesWeeklyFromDailyTradingDays(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	dailyRows := []types.KLine{
		{
			StartTime: types.Time(time.Date(2026, time.May, 25, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 25, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(104),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(103),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(time.Date(2026, time.May, 26, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 26, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(103),
			High:      fixedpoint.NewFromFloat(106),
			Low:       fixedpoint.NewFromFloat(102),
			Close:     fixedpoint.NewFromFloat(105),
			Volume:    fixedpoint.NewFromFloat(1100),
		},
		{
			StartTime: types.Time(time.Date(2026, time.May, 27, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 27, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(105),
			High:      fixedpoint.NewFromFloat(107),
			Low:       fixedpoint.NewFromFloat(101),
			Close:     fixedpoint.NewFromFloat(102),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
		{
			StartTime: types.Time(time.Date(2026, time.May, 28, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 28, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(102),
			High:      fixedpoint.NewFromFloat(108),
			Low:       fixedpoint.NewFromFloat(100),
			Close:     fixedpoint.NewFromFloat(107),
			Volume:    fixedpoint.NewFromFloat(1300),
		},
		{
			StartTime: types.Time(time.Date(2026, time.May, 29, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.May, 29, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(107),
			High:      fixedpoint.NewFromFloat(110),
			Low:       fixedpoint.NewFromFloat(106),
			Close:     fixedpoint.NewFromFloat(109),
			Volume:    fixedpoint.NewFromFloat(1400),
		},
	}
	if err := store.InsertKLines(dailyRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(1d): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1w, time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(1w): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized weekly kline, got %d", len(got))
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.May, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly start = %s, want 2026-05-25T00:00:00Z", got[0].StartTime.Time())
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("weekly open = %s, want 100", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(110)) != 0 {
		t.Fatalf("weekly high = %s, want 110", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(99)) != 0 {
		t.Fatalf("weekly low = %s, want 99", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(109)) != 0 {
		t.Fatalf("weekly close = %s, want 109", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(6000)) != 0 {
		t.Fatalf("weekly volume = %s, want 6000", got[0].Volume.String())
	}
}

func TestFutuKLineStoreSynthesizesMonthlyFromDailyTradingDays(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	dailyRows := []types.KLine{
		{
			StartTime: types.Time(time.Date(2026, time.January, 28, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 28, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(90),
			High:      fixedpoint.NewFromFloat(94),
			Low:       fixedpoint.NewFromFloat(88),
			Close:     fixedpoint.NewFromFloat(93),
			Volume:    fixedpoint.NewFromFloat(800),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 29, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 29, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(93),
			High:      fixedpoint.NewFromFloat(97),
			Low:       fixedpoint.NewFromFloat(92),
			Close:     fixedpoint.NewFromFloat(96),
			Volume:    fixedpoint.NewFromFloat(900),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 30, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 30, 23, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1d,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(96),
			High:      fixedpoint.NewFromFloat(101),
			Low:       fixedpoint.NewFromFloat(95),
			Close:     fixedpoint.NewFromFloat(100),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
	}
	if err := store.InsertKLines(dailyRows, "forward"); err != nil {
		t.Fatalf("InsertKLines(1d month): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1mo, time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(1mo): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized monthly kline, got %d", len(got))
	}
	if !got[0].StartTime.Time().Equal(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("monthly start = %s, want 2026-01-01T00:00:00Z", got[0].StartTime.Time())
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(90)) != 0 {
		t.Fatalf("monthly open = %s, want 90", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(101)) != 0 {
		t.Fatalf("monthly high = %s, want 101", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(88)) != 0 {
		t.Fatalf("monthly low = %s, want 88", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("monthly close = %s, want 100", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(2700)) != 0 {
		t.Fatalf("monthly volume = %s, want 2700", got[0].Volume.String())
	}
}

func TestFutuKLineStorePrefersFiveMinuteSourceForFifteenMinuteQuery(t *testing.T) {
	store := newTestFutuKLineStore(t)
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	oneMinuteKLines := make([]types.KLine, 0, 15)
	for index := range 15 {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		oneMinuteKLines = append(oneMinuteKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(10 + float64(index)),
			High:      fixedpoint.NewFromFloat(20 + float64(index)),
			Low:       fixedpoint.NewFromFloat(5 + float64(index)),
			Close:     fixedpoint.NewFromFloat(15 + float64(index)),
			Volume:    fixedpoint.NewFromFloat(100 + float64(index)),
		})
	}
	fiveMinuteKLines := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(baseStart.Add(5*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(200),
			High:      fixedpoint.NewFromFloat(210),
			Low:       fixedpoint.NewFromFloat(190),
			Close:     fixedpoint.NewFromFloat(205),
			Volume:    fixedpoint.NewFromFloat(2000),
		},
		{
			StartTime: types.Time(baseStart.Add(5 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(10*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(205),
			High:      fixedpoint.NewFromFloat(230),
			Low:       fixedpoint.NewFromFloat(195),
			Close:     fixedpoint.NewFromFloat(225),
			Volume:    fixedpoint.NewFromFloat(2300),
		},
		{
			StartTime: types.Time(baseStart.Add(10 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(15*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(225),
			High:      fixedpoint.NewFromFloat(240),
			Low:       fixedpoint.NewFromFloat(180),
			Close:     fixedpoint.NewFromFloat(235),
			Volume:    fixedpoint.NewFromFloat(2500),
		},
	}
	if err := store.InsertKLines(oneMinuteKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(1m): %v", err)
	}
	if err := store.InsertKLines(fiveMinuteKLines, "forward"); err != nil {
		t.Fatalf("InsertKLines(5m): %v", err)
	}

	got, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval15m, baseStart.Add(15*time.Minute), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(15m): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one synthesized 15m kline, got %d", len(got))
	}
	if got[0].Open.Compare(fixedpoint.NewFromFloat(200)) != 0 {
		t.Fatalf("preferred source open = %s, want 200 from 5m data", got[0].Open.String())
	}
	if got[0].High.Compare(fixedpoint.NewFromFloat(240)) != 0 {
		t.Fatalf("preferred source high = %s, want 240 from 5m data", got[0].High.String())
	}
	if got[0].Low.Compare(fixedpoint.NewFromFloat(180)) != 0 {
		t.Fatalf("preferred source low = %s, want 180 from 5m data", got[0].Low.String())
	}
	if got[0].Close.Compare(fixedpoint.NewFromFloat(235)) != 0 {
		t.Fatalf("preferred source close = %s, want 235 from 5m data", got[0].Close.String())
	}
	if got[0].Volume.Compare(fixedpoint.NewFromFloat(6800)) != 0 {
		t.Fatalf("preferred source volume = %s, want 6800 from 5m data", got[0].Volume.String())
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
