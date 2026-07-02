package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func TestStoreAPIsPropagateClosedDatabaseFailures(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	since := time.Date(2026, time.July, 1, 9, 30, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	kline := testKLine("US.AAPL", types.Interval1m, since, time.Minute, 100, 101, 99, 100.5, 10)
	assertError := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil", name)
		}
	}

	assertError("InsertKLine", store.InsertKLine(kline, "forward"))
	assertError("InsertKLines", store.InsertKLines([]types.KLine{kline, kline}, "forward"))
	if _, err := store.QueryKLine(nil, "US.AAPL", types.Interval1m, "DESC", 1); err == nil {
		t.Fatal("QueryKLine error = nil")
	}
	if _, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, since, 2); err == nil {
		t.Fatal("QueryKLinesForward error = nil")
	}
	if _, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, until, 2); err == nil {
		t.Fatal("QueryKLinesBackward error = nil")
	}
	assertError("StreamKLines", store.StreamKLines(since, until, nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m}, func(types.KLine) {}))
	rows, errCh := store.QueryKLinesCh(since, until, nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m})
	for range rows {
		t.Fatal("QueryKLinesCh emitted a row from closed database")
	}
	if err := <-errCh; err == nil {
		t.Fatal("QueryKLinesCh error = nil")
	}

	if _, err := store.QueryTradingPeriodKLinesInRange("US.AAPL", types.Interval1w, since, until, false); err == nil {
		t.Fatal("QueryTradingPeriodKLinesInRange error = nil")
	}
	if _, err := store.QuerySessionAwareIntradayKLinesInRange("HK.00700", types.Interval2h, since, until, false); err == nil {
		t.Fatal("QuerySessionAwareIntradayKLinesInRange error = nil")
	}
	if _, err := store.QueryDailyKLinesInRange("US.AAPL", since, until, false); err == nil {
		t.Fatal("QueryDailyKLinesInRange error = nil")
	}
	assertError("EnsureCoverage", store.EnsureCoverage("US.AAPL", types.Interval5m, since, until))
	if _, err := store.isBatchCovered("US.AAPL", types.Interval1m, since, until, "forward"); err == nil {
		t.Fatal("isBatchCovered error = nil")
	}
}

func TestClosedDatabaseFailuresPropagateThroughStorageHelpers(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "closed-helpers.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	since := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(48 * time.Hour)
	tableName := store.writeTableName("US.AAPL", types.Interval1m, "forward")
	kline := testKLine("US.AAPL", types.Interval1m, since, time.Minute, 100, 101, 99, 100, 10)

	checks := []struct {
		name string
		call func() error
	}{
		{"ensure table", func() error { return store.ensureKLineTable(tableName) }},
		{"single insert with ensured map", func() error { return store.insertKLineLocked(kline, "forward", map[string]struct{}{}) }},
		{"single insert direct", func() error { return store.insertKLinesLocked([]types.KLine{kline}, "forward") }},
		{"select table", func() error {
			_, err := store.selectReadTableName("US.AAPL", types.Interval1m, "forward", since, until)
			return err
		}},
		{"selection missing", func() error {
			_, err := store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, since, until)
			return err
		}},
		{"missing ranges", func() error { _, err := store.findMissingRanges("US.AAPL", types.Interval5m, since, until); return err }},
		{"missing ranges table", func() error {
			_, err := store.findMissingRangesInTable("US.AAPL", types.Interval1m, "forward", since, until)
			return err
		}},
		{"missing physical", func() error {
			_, err := store.findMissingRangesInPhysicalTable(tableName, types.Interval1m, since, until)
			return err
		}},
		{"ending after", func() error { _, err := store.hasKLineEndingAtOrAfter(tableName, since); return err }},
		{"ending before", func() error { _, err := store.hasKLineEndingAtOrBefore(tableName, until); return err }},
		{"boundary pair", func() error { _, err := store.hasKLineBoundaryPair(tableName, since, until); return err }},
		{"stored forward", func() error {
			_, err := store.queryStoredKLinesForward("US.AAPL", types.Interval1m, "forward", since, 2)
			return err
		}},
		{"stored backward", func() error {
			_, err := store.queryStoredKLinesBackward("US.AAPL", types.Interval1m, "forward", until, 2)
			return err
		}},
		{"stored range", func() error {
			_, err := store.queryStoredKLinesInRange("US.AAPL", types.Interval1m, "forward", since, until)
			return err
		}},
		{"stream range", func() error {
			return store.streamStoredKLinesInRange("US.AAPL", types.Interval1m, "forward", since, until, func(types.KLine) {})
		}},
		{"aggregate generic", func() error {
			_, err := store.queryAggregatedKLinesInRange("US.AAPL", types.Interval5m, types.Interval1m, since, until)
			return err
		}},
		{"aggregate intraday", func() error {
			_, err := store.queryAggregatedSessionAwareIntradayKLinesInRangeLocked("HK.00700", types.Interval5m, types.Interval1m, since, until, false)
			return err
		}},
		{"aggregate trading period", func() error {
			_, err := store.queryAggregatedTradingPeriodKLinesInRangeLocked("US.AAPL", types.Interval1w, types.Interval1d, since, until, false)
			return err
		}},
		{"aggregate daily", func() error {
			_, err := store.queryAggregatedDailyKLinesInRangeLocked("US.AAPL", types.Interval1h, since, until, false)
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil {
				t.Fatalf("%s error = nil", check.name)
			}
		})
	}

	if err := store.insertKLinesLocked(nil, "forward"); err != nil {
		t.Fatalf("insertKLinesLocked(empty) error = %v", err)
	}
	var nilStore *FutuKLineStore
	if err := nilStore.InsertKLine(kline, "forward"); err != nil {
		t.Fatalf("nil InsertKLine error = %v", err)
	}
	if err := nilStore.InsertKLines([]types.KLine{kline}, "forward"); err != nil {
		t.Fatalf("nil InsertKLines error = %v", err)
	}
	if err := store.InsertKLines(nil, "forward"); err != nil {
		t.Fatalf("empty InsertKLines error = %v", err)
	}
}

func TestCompactSchemaRejectsLegacyKLineTableShapes(t *testing.T) {
	tests := []struct {
		name string
		ddl  string
		want string
	}{
		{name: "missing columns", ddl: `CREATE TABLE %s (end_time INTEGER PRIMARY KEY)`, want: "schema is obsolete"},
		{name: "wrong column order", ddl: `CREATE TABLE %s (start_time INTEGER NOT NULL, end_time INTEGER NOT NULL PRIMARY KEY, open TEXT NOT NULL, high TEXT NOT NULL, low TEXT NOT NULL, close TEXT NOT NULL, volume TEXT NOT NULL) WITHOUT ROWID`, want: "schema is obsolete"},
		{name: "rowid table", ddl: `CREATE TABLE %s (end_time INTEGER NOT NULL PRIMARY KEY, start_time INTEGER NOT NULL, open TEXT NOT NULL, high TEXT NOT NULL, low TEXT NOT NULL, close TEXT NOT NULL, volume TEXT NOT NULL)`, want: "schema is obsolete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestKLineStore(t)
			tableName := store.writeTableName("US.AAPL", types.Interval1m, "forward")
			if _, err := store.db.Exec(strings.ReplaceAll(tt.ddl, "%s", quoteIdentifier(tableName))); err != nil {
				t.Fatalf("create legacy table: %v", err)
			}
			err := store.ensureKLineTable(tableName)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ensureKLineTable() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReadTableNamesHonorExplicitSessionScopePriority(t *testing.T) {
	store := newTestKLineStore(t)
	for _, tc := range []struct {
		scope string
		count int
		first string
	}{
		{klineSessionScopeLegacy, 1, "__forward__"},
		{klineSessionScopeRegular, 3, "__forward__r__"},
		{klineSessionScopeExtended, 3, "__forward__x__"},
		{klineReadSessionScopeAuto, 3, "__forward__"},
	} {
		store.SetReadSessionScope(tc.scope)
		names, count := store.readTableNames("US.AAPL", types.Interval1m, "forward")
		if count != tc.count || !strings.Contains(names[0], tc.first) {
			t.Fatalf("readTableNames(%q) = (%#v, %d)", tc.scope, names, count)
		}
	}
}

func TestStreamAndQueryShortCircuitEmptyInputs(t *testing.T) {
	store := newTestKLineStore(t)
	if err := store.StreamKLines(time.Time{}, time.Time{}, nil, nil, nil, nil); err != nil {
		t.Fatalf("StreamKLines(empty) error = %v", err)
	}
	rows, errCh := store.QueryKLinesCh(time.Time{}, time.Time{}, nil, nil, []types.Interval{types.Interval1m})
	if _, ok := <-rows; ok {
		t.Fatal("QueryKLinesCh(empty symbols) emitted row")
	}
	if _, ok := <-errCh; ok {
		t.Fatal("QueryKLinesCh(empty symbols) emitted error")
	}
	if err := store.Sync(context.Background(), nil, "US.AAPL", nil, time.Time{}, time.Time{}); err != nil {
		t.Fatalf("Sync(no-op) error = %v", err)
	}
}
