package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98AggregateQueryAPIsPropagateCoverageAndSourceFailures(t *testing.T) {
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	until := firstClosedKLineEndAtOrAfter(start, types.Interval5m)

	t.Run("forward and backward reject absent lower-interval coverage", func(t *testing.T) {
		store := newTestKLineStore(t)
		for _, call := range []struct {
			name string
			fn   func() error
		}{
			{
				name: "forward",
				fn: func() error {
					_, err := store.QueryKLinesForward(nil, "TEST.MISSING", types.Interval5m, start, 1)
					return err
				},
			},
			{
				name: "backward",
				fn: func() error {
					_, err := store.QueryKLinesBackward(nil, "TEST.MISSING", types.Interval5m, until.Add(time.Millisecond), 1)
					return err
				},
			},
		} {
			t.Run(call.name, func(t *testing.T) {
				err := call.fn()
				if err == nil || !strings.Contains(err.Error(), "missing K-line coverage") {
					t.Fatalf("%s missing coverage error = %v", call.name, err)
				}
			})
		}

		rows, errCh := store.QueryKLinesCh(start, until, nil, []string{"TEST.MISSING", "TEST.MISSING.2"}, []types.Interval{types.Interval5m})
		for row := range rows {
			t.Fatalf("missing multi-query emitted %#v", row)
		}
		var asyncErr error
		for err := range errCh {
			asyncErr = err
		}
		if asyncErr == nil || !strings.Contains(asyncErr.Error(), "missing K-line coverage") {
			t.Fatalf("missing multi-query error = %v", asyncErr)
		}
		if err := store.StreamKLines(start, until, nil, []string{"TEST.MISSING", "TEST.MISSING.2"}, []types.Interval{types.Interval5m}, func(types.KLine) {}); err == nil || !strings.Contains(err.Error(), "missing K-line coverage") {
			t.Fatalf("missing multi-stream error = %v", err)
		}
	})

	t.Run("query APIs do not hide corrupt lower-interval rows after coverage selection", func(t *testing.T) {
		store := newTestKLineStore(t)
		seedEndTimeOnlyKLineCoverage(t, store, "TEST.CORRUPT", start, until)

		for _, call := range []struct {
			name string
			fn   func() error
		}{
			{
				name: "forward",
				fn: func() error {
					_, err := store.QueryKLinesForward(nil, "TEST.CORRUPT", types.Interval5m, start, 1)
					return err
				},
			},
			{
				name: "backward",
				fn: func() error {
					_, err := store.QueryKLinesBackward(nil, "TEST.CORRUPT", types.Interval5m, until.Add(time.Millisecond), 1)
					return err
				},
			},
			{
				name: "single stream",
				fn: func() error {
					return store.StreamKLines(start, until, nil, []string{"TEST.CORRUPT"}, []types.Interval{types.Interval5m}, func(types.KLine) {
						t.Fatal("corrupt aggregate source emitted a kline")
					})
				},
			},
			{
				name: "multi stream",
				fn: func() error {
					return store.StreamKLines(start, until, nil, []string{"TEST.CORRUPT", "TEST.CORRUPT"}, []types.Interval{types.Interval5m}, func(types.KLine) {
						t.Fatal("corrupt aggregate source emitted a kline")
					})
				},
			},
		} {
			t.Run(call.name, func(t *testing.T) {
				if err := call.fn(); err == nil || !strings.Contains(err.Error(), "start_time") {
					t.Fatalf("%s corrupt-source error = %v", call.name, err)
				}
			})
		}

		rows, errCh := store.QueryKLinesCh(start, until, nil, []string{"TEST.CORRUPT"}, []types.Interval{types.Interval5m})
		for row := range rows {
			t.Fatalf("corrupt aggregate source emitted %#v", row)
		}
		var asyncErr error
		for err := range errCh {
			asyncErr = err
		}
		if asyncErr == nil || !strings.Contains(asyncErr.Error(), "start_time") {
			t.Fatalf("corrupt single-query error = %v", asyncErr)
		}
	})
}

func TestCoverage98StoredReadersSkipEmptyScopedTables(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	for _, scope := range []string{klineSessionScopeLegacy, klineSessionScopeRegular} {
		store.SetWriteSessionScope(scope)
		tableName := store.writeTableName("TEST.EMPTY", types.Interval1m, "forward")
		if err := store.ensureKLineTable(tableName); err != nil {
			t.Fatalf("ensure empty %s table: %v", scope, err)
		}
	}
	store.SetReadSessionScope(klineReadSessionScopeAuto)
	if rows, err := store.queryStoredKLinesForward("TEST.EMPTY", types.Interval1m, "forward", start, 2); err != nil || len(rows) != 0 {
		t.Fatalf("empty scoped forward = %#v/%v", rows, err)
	}
	if rows, err := store.queryStoredKLinesBackward("TEST.EMPTY", types.Interval1m, "forward", start.Add(time.Minute), 2); err != nil || len(rows) != 0 {
		t.Fatalf("empty scoped backward = %#v/%v", rows, err)
	}
}

func seedEndTimeOnlyKLineCoverage(t *testing.T, store *FutuKLineStore, symbol string, since, until time.Time) {
	t.Helper()
	tableName := store.writeTableName(symbol, types.Interval1m, "forward")
	if _, err := store.db.Exec(`CREATE TABLE ` + quoteIdentifier(tableName) + ` (end_time INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create corrupt source table: %v", err)
	}
	for end := firstClosedKLineEndAtOrAfter(since, types.Interval1m); !end.After(until); end = end.Add(time.Minute) {
		if _, err := store.db.Exec(`INSERT INTO `+quoteIdentifier(tableName)+` (end_time) VALUES (?)`, timeToUnixMillis(end)); err != nil {
			t.Fatalf("insert corrupt source coverage at %s: %v", end, err)
		}
	}
}
