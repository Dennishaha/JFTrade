package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func TestCompactDatabasePreservesUsableStoreAndReportsUnavailableStore(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	bar := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10)
	if err := store.InsertKLine(bar, "forward"); err != nil {
		t.Fatalf("InsertKLine: %v", err)
	}
	if err := store.CompactDatabase(context.Background()); err != nil {
		t.Fatalf("CompactDatabase: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CompactDatabase(canceled); err == nil || !strings.Contains(err.Error(), "compact backtest database") {
		t.Fatalf("canceled CompactDatabase error = %v", err)
	}
	got, err := store.QueryKLine(nil, bar.Symbol, bar.Interval, "DESC", 1)
	if err != nil || got == nil || got.Close != bar.Close {
		t.Fatalf("QueryKLine after compact = %#v, %v", got, err)
	}

	var nilStore *FutuKLineStore
	if err := nilStore.CompactDatabase(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil CompactDatabase error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.CompactDatabase(context.Background()); err == nil || !strings.Contains(err.Error(), "compact backtest database") {
		t.Fatalf("closed CompactDatabase error = %v", err)
	}
}

func TestTableExistenceCacheRejectsUnexpectedDynamicTypes(t *testing.T) {
	if got := jftradeCheckedTypeAssertion[bool](true); !got {
		t.Fatal("typed table-existence cache value was not preserved")
	}
	defer func() {
		if recovered := recover(); recovered != "unexpected dynamic type" {
			t.Fatalf("cache type guard panic = %#v", recovered)
		}
	}()
	_ = jftradeCheckedTypeAssertion[bool]("true")
}

func TestSelectReadTableNameUsesFirstExistingTableForUnboundedReads(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	bar := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10)
	if err := store.InsertKLine(bar, "forward"); err != nil {
		t.Fatalf("InsertKLine: %v", err)
	}
	store.SetReadSessionScope(klineSessionScopeLegacy)
	tableName := store.writeTableName(bar.Symbol, bar.Interval, "forward")
	if missing, err := store.findSelectionMissingRangesInPhysicalTable(tableName, bar.Interval, bar.StartTime.Time(), bar.EndTime.Time()); err != nil || len(missing) != 0 {
		t.Fatalf("covered table selection missing = %#v, %v", missing, err)
	}
	got, err := store.selectReadTableName(bar.Symbol, bar.Interval, "forward", time.Time{}, time.Time{})
	if err != nil || got != tableName {
		t.Fatalf("selectReadTableName unbounded = %q, %v", got, err)
	}
	got, err = store.selectReadTableName(bar.Symbol, bar.Interval, "forward", bar.StartTime.Time(), bar.EndTime.Time())
	if err != nil || got != tableName {
		t.Fatalf("selectReadTableName covered range = %q, %v", got, err)
	}
}

func TestAggregateQueryHelpersHandleDailyTradingAndIntradayReadSources(t *testing.T) {
	store := newTestKLineStore(t)
	day := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := day.Add(24*time.Hour - time.Millisecond)

	daily := testKLine("US.AAPL", types.Interval1d, day, 24*time.Hour, 100, 105, 99, 104, 20)
	if err := store.InsertKLine(daily, "forward"); err != nil {
		t.Fatalf("InsertKLine daily: %v", err)
	}
	storedDaily, err := store.queryAggregatedKLinesInRange("US.AAPL", types.Interval1d, types.Interval1h, day, dayEnd)
	if err != nil || len(storedDaily) != 1 || storedDaily[0].Close != daily.Close {
		t.Fatalf("queryAggregatedKLinesInRange daily = %#v, %v", storedDaily, err)
	}
	tradingDaily, err := store.queryTradingPeriodKLinesInRangeLocked("US.AAPL", types.Interval1d, day, dayEnd, false)
	if err != nil || len(tradingDaily) != 1 {
		t.Fatalf("queryTradingPeriodKLinesInRangeLocked daily = %#v, %v", tradingDaily, err)
	}
	if got, err := store.queryAggregatedKLinesInRange("US.AAPL", types.Interval1h, types.Interval1h, day, dayEnd); err != nil || got != nil {
		t.Fatalf("equal aggregate interval = %#v, %v", got, err)
	}
	if got, err := store.queryAggregatedKLinesInRange("US.AAPL", types.Interval1m, types.Interval5m, day, dayEnd); err != nil || got != nil {
		t.Fatalf("smaller aggregate interval = %#v, %v", got, err)
	}

	week := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	weeklyBase := []types.KLine{
		testKLine("US.MSFT", types.Interval1d, week, 24*time.Hour, 200, 205, 198, 203, 10),
		testKLine("US.MSFT", types.Interval1d, week.AddDate(0, 0, 1), 24*time.Hour, 203, 207, 202, 206, 11),
	}
	if err := store.InsertKLines(weeklyBase, "forward"); err != nil {
		t.Fatalf("InsertKLines weekly base: %v", err)
	}
	weekly, err := store.queryAggregatedKLinesInRange("US.MSFT", types.Interval1w, types.Interval1d, week, week.AddDate(0, 0, 7).Add(-time.Millisecond))
	if err != nil || len(weekly) != 1 {
		t.Fatalf("queryAggregatedKLinesInRange weekly = %#v, %v", weekly, err)
	}

	hkStart := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	hkBase := []types.KLine{
		testKLine("HK.00700", types.Interval1h, hkStart, time.Hour, 300, 302, 299, 301, 10),
		testKLine("HK.00700", types.Interval1h, hkStart.Add(time.Hour), time.Hour, 301, 304, 300, 303, 11),
	}
	if err := store.InsertKLines(hkBase, "forward"); err != nil {
		t.Fatalf("InsertKLines intraday base: %v", err)
	}
	intraday, err := store.queryAggregatedKLinesInRange("HK.00700", types.Interval2h, types.Interval1h, hkStart, hkStart.Add(2*time.Hour-time.Millisecond))
	if err != nil || len(intraday) != 1 {
		t.Fatalf("queryAggregatedKLinesInRange intraday = %#v, %v", intraday, err)
	}
}

func TestAggregateForwardBackwardQueriesNormalizeLimitsAndUseSynthesizedBars(t *testing.T) {
	store := newTestKLineStore(t)
	week := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	if err := store.InsertKLines([]types.KLine{
		testKLine("US.AAPL", types.Interval1d, week, 24*time.Hour, 100, 103, 99, 102, 10),
		testKLine("US.AAPL", types.Interval1d, week.AddDate(0, 0, 1), 24*time.Hour, 102, 105, 101, 104, 11),
	}, "forward"); err != nil {
		t.Fatalf("InsertKLines weekly bars: %v", err)
	}
	forward, err := store.queryAggregatedTradingPeriodKLinesForwardLocked("US.AAPL", types.Interval1w, week, 0)
	if err != nil || len(forward) != 1 {
		t.Fatalf("weekly forward normalized limit = %#v, %v", forward, err)
	}
	backward, err := store.queryAggregatedTradingPeriodKLinesBackwardLocked("US.AAPL", types.Interval1w, week.AddDate(0, 0, 7), 0)
	if err != nil || len(backward) != 1 {
		t.Fatalf("weekly backward normalized limit = %#v, %v", backward, err)
	}
	if got, err := store.queryAggregatedTradingPeriodKLinesForwardLocked("UNKNOWN", types.Interval1w, week, 1); err != nil || got != nil {
		t.Fatalf("unknown weekly forward = %#v, %v", got, err)
	}
	if got, err := store.queryAggregatedTradingPeriodKLinesBackwardLocked("UNKNOWN", types.Interval1w, week.AddDate(0, 0, 7), 1); err != nil || got != nil {
		t.Fatalf("unknown weekly backward = %#v, %v", got, err)
	}

	hkStart := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	if err := store.InsertKLines([]types.KLine{
		testKLine("HK.00700", types.Interval1h, hkStart, time.Hour, 200, 202, 199, 201, 10),
		testKLine("HK.00700", types.Interval1h, hkStart.Add(time.Hour), time.Hour, 201, 204, 200, 203, 11),
	}, "forward"); err != nil {
		t.Fatalf("InsertKLines HK bars: %v", err)
	}
	intradayForward, err := store.queryAggregatedSessionAwareIntradayKLinesForwardLocked("HK.00700", types.Interval2h, hkStart, 0, false)
	if err != nil || len(intradayForward) != 1 {
		t.Fatalf("intraday forward normalized limit = %#v, %v", intradayForward, err)
	}
	intradayBackward, err := store.queryAggregatedSessionAwareIntradayKLinesBackwardLocked("HK.00700", types.Interval2h, hkStart.Add(2*time.Hour), 0, false)
	if err != nil || len(intradayBackward) != 1 {
		t.Fatalf("intraday backward normalized limit = %#v, %v", intradayBackward, err)
	}
	if got, err := store.queryAggregatedSessionAwareIntradayKLinesForwardLocked("UNKNOWN", types.Interval2h, hkStart, 1, false); err != nil || len(got) != 0 {
		t.Fatalf("unknown intraday forward = %#v, %v", got, err)
	}
	if got, err := store.queryAggregatedSessionAwareIntradayKLinesBackwardLocked("UNKNOWN", types.Interval2h, hkStart.Add(2*time.Hour), 1, false); err != nil || len(got) != 0 {
		t.Fatalf("unknown intraday backward = %#v, %v", got, err)
	}
}

func TestScopedReadFallbackKeepsTheFirstAvailablePartialSeries(t *testing.T) {
	store := newTestKLineStore(t)
	store.SetReadSessionScope(klineReadSessionScopeAuto)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)

	store.SetWriteSessionScope(klineSessionScopeExtended)
	extended := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10)
	if err := store.InsertKLine(extended, "forward"); err != nil {
		t.Fatalf("InsertKLine extended: %v", err)
	}
	store.SetWriteSessionScope(klineSessionScopeRegular)
	regular := testKLine("HK.00700", types.Interval1m, start.Add(time.Minute), time.Minute, 101, 102, 100, 101.5, 11)
	if err := store.InsertKLine(regular, "forward"); err != nil {
		t.Fatalf("InsertKLine regular: %v", err)
	}

	forward, err := store.queryStoredKLinesForward("HK.00700", types.Interval1m, "forward", start, 2)
	if err != nil || len(forward) != 1 || !forward[0].EndTime.Time().Equal(extended.EndTime.Time()) {
		t.Fatalf("partial scoped forward = %#v, %v", forward, err)
	}
	backward, err := store.queryStoredKLinesBackward("HK.00700", types.Interval1m, "forward", regular.EndTime.Time(), 2)
	if err != nil || len(backward) != 1 || !backward[0].EndTime.Time().Equal(extended.EndTime.Time()) {
		t.Fatalf("partial scoped backward = %#v, %v", backward, err)
	}
	if rows, err := store.queryStoredKLinesInRange("HK.00005", types.Interval1m, "forward", start, regular.EndTime.Time()); err != nil || rows != nil {
		t.Fatalf("missing scoped range = %#v, %v", rows, err)
	}
}

func TestBatchInsertFailureRollsBackPriorBars(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	first := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10)
	second := testKLine("HK.00700", types.Interval1m, start.Add(time.Minute), time.Minute, 100.5, 102, 100, 101.5, 11)
	tableName := store.writeTableName(first.Symbol, first.Interval, "forward")
	if err := store.ensureKLineTable(tableName); err != nil {
		t.Fatalf("ensureKLineTable: %v", err)
	}
	trigger := fmt.Sprintf(
		`CREATE TRIGGER reject_second_bar BEFORE INSERT ON %s WHEN NEW.end_time = %d BEGIN SELECT RAISE(ABORT, 'reject second bar'); END`,
		quoteIdentifier(tableName), timeToUnixMillis(second.EndTime.Time()),
	)
	if _, err := store.db.Exec(trigger); err != nil {
		t.Fatalf("create reject trigger: %v", err)
	}
	if err := store.InsertKLines([]types.KLine{first, second}, "forward"); err == nil || !strings.Contains(err.Error(), "reject second bar") {
		t.Fatalf("InsertKLines trigger error = %v", err)
	}
	rows, err := store.queryStoredKLinesInRange(first.Symbol, first.Interval, "forward", first.StartTime.Time(), second.EndTime.Time())
	if err != nil {
		t.Fatalf("query after failed batch: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("failed batch persisted rows = %#v", rows)
	}
}

func TestCoverageSelectionDistinguishesMissingBoundariesAndSynthesis(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	end := start.Add(time.Minute - time.Millisecond)
	tableName := store.writeTableName("HK.00700", types.Interval1m, "forward")

	missing, err := store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, start, end)
	if err != nil || len(missing) != 1 {
		t.Fatalf("missing physical selection = %#v, %v", missing, err)
	}
	if err := store.ensureKLineTable(tableName); err != nil {
		t.Fatalf("ensureKLineTable: %v", err)
	}
	missing, err = store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, start, end)
	if err != nil || len(missing) != 1 {
		t.Fatalf("empty physical selection = %#v, %v", missing, err)
	}
	if missing, err = store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, end.Add(time.Millisecond), start); err != nil || missing != nil {
		t.Fatalf("inverted physical selection = %#v, %v", missing, err)
	}

	bar := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 102, 99, 101, 10)
	if err := store.InsertKLine(bar, "forward"); err != nil {
		t.Fatalf("InsertKLine: %v", err)
	}
	missing, err = store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, start, end)
	if err != nil || missing != nil {
		t.Fatalf("covered physical selection = %#v, %v", missing, err)
	}
	if covered, err := store.hasKLineEndingAtOrAfter(tableName, bar.EndTime.Time().Add(time.Minute)); err != nil || covered {
		t.Fatalf("hasKLineEndingAtOrAfter future = %t, %v", covered, err)
	}
	if covered, err := store.hasKLineEndingAtOrBefore(tableName, bar.StartTime.Time()); err != nil || covered {
		t.Fatalf("hasKLineEndingAtOrBefore past = %t, %v", covered, err)
	}
	if covered, err := store.hasKLineBoundaryPair(tableName, bar.EndTime.Time(), bar.EndTime.Time()); err != nil || !covered {
		t.Fatalf("hasKLineBoundaryPair single = %t, %v", covered, err)
	}

	if missing, err = store.findMissingRanges("HK.00700", types.Interval1m, start, end); err != nil || missing != nil {
		t.Fatalf("direct covered range = %#v, %v", missing, err)
	}
	if missing, err = store.findMissingRanges("HK.00005", types.Interval1s, start, end); err != nil || len(missing) != 1 {
		t.Fatalf("non-aggregatable missing range = %#v, %v", missing, err)
	}
	if missing, err = store.findMissingRanges("HK.00005", types.Interval5m, start, start.Add(5*time.Minute-time.Millisecond)); err != nil || len(missing) != 1 {
		t.Fatalf("preferred aggregate missing range = %#v, %v", missing, err)
	}

	direct, err := store.resolveReadSource("HK.00700", types.Interval1m, start, end)
	if err != nil || direct.synthesize {
		t.Fatalf("direct read source = %#v, %v", direct, err)
	}
	if _, err := store.resolveReadSource("HK.00005", types.Interval1m, start, end); err == nil || !strings.Contains(err.Error(), "download 1m") {
		t.Fatalf("missing direct read source error = %v", err)
	}
	if _, err := store.resolveReadSource("HK.00005", types.Interval5m, start, start.Add(5*time.Minute-time.Millisecond)); err == nil || !strings.Contains(err.Error(), "download 1m") {
		t.Fatalf("missing aggregate read source error = %v", err)
	}
	if covered, err := store.isBatchCovered("HK.00005", types.Interval1m, start, end, "forward"); err != nil || covered {
		t.Fatalf("nonexistent batch coverage = %t, %v", covered, err)
	}
	if covered, err := store.isBatchCovered("HK.00700", types.Interval1m, start, end, "forward"); err != nil || !covered {
		t.Fatalf("stored batch coverage = %t, %v", covered, err)
	}
	if covered, err := store.isBatchCovered("HK.00700", types.Interval1m, bar.EndTime.Time().Add(time.Minute), bar.EndTime.Time().Add(2*time.Minute), "forward"); err != nil || covered {
		t.Fatalf("empty batch coverage = %t, %v", covered, err)
	}
}

func TestAggregationBoundaryHelpersKeepIncompleteAndOutOfWindowBarsOut(t *testing.T) {
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	base := []types.KLine{
		testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 102, 99, 101, 10),
		testKLine("HK.00700", types.Interval1m, start.Add(time.Minute), time.Minute, 101, 103, 98, 102, 11),
	}
	if got := aggregateKLinesFromBase("HK.00700", types.Interval5m, types.Interval1m, nil, start, start.Add(5*time.Minute)); got != nil {
		t.Fatalf("empty generic aggregation = %#v", got)
	}
	if got := aggregateKLinesFromBase("HK.00700", types.Interval1m, types.Interval1m, base, start, start.Add(2*time.Minute)); got != nil {
		t.Fatalf("non-lower generic aggregation = %#v", got)
	}
	if got := aggregateKLinesFromBase("HK.00700", types.Interval5m, types.Interval1m, base, start, start.Add(5*time.Minute)); len(got) != 0 {
		t.Fatalf("incomplete generic aggregation = %#v", got)
	}
	complete := append(append([]types.KLine(nil), base...),
		testKLine("HK.00700", types.Interval1m, start.Add(2*time.Minute), time.Minute, 102, 104, 97, 103, 12),
		testKLine("HK.00700", types.Interval1m, start.Add(3*time.Minute), time.Minute, 103, 105, 96, 104, 13),
		testKLine("HK.00700", types.Interval1m, start.Add(4*time.Minute), time.Minute, 104, 106, 95, 105, 14),
	)
	if got := aggregateKLinesFromBase("HK.00700", types.Interval5m, types.Interval1m, complete, start, start.Add(4*time.Minute)); len(got) != 0 {
		t.Fatalf("out-of-window generic aggregation = %#v", got)
	}

	day := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	dayRows := []types.KLine{
		testKLine("US.AAPL", types.Interval1h, day.Add(13*time.Hour+30*time.Minute), time.Hour, 100, 101, 99, 100, 10),
		testKLine("US.AAPL", types.Interval1h, day.Add(14*time.Hour+30*time.Minute), time.Hour, 100, 102, 98, 101, 11),
		testKLine("US.AAPL", types.Interval1h, day.AddDate(0, 0, 1).Add(13*time.Hour+30*time.Minute), time.Hour, 101, 103, 97, 102, 12),
	}
	if got := aggregateDailyKLinesFromBase("US.AAPL", dayRows, day, day.Add(24*time.Hour-time.Millisecond), false); len(got) != 1 || got[0].Low != dayRows[1].Low {
		t.Fatalf("daily lower-low aggregation = %#v", got)
	}
	weeklyRows := []types.KLine{
		testKLine("US.AAPL", types.Interval1d, day, 24*time.Hour, 100, 101, 99, 100, 10),
		testKLine("US.AAPL", types.Interval1d, day.AddDate(0, 0, 1), 24*time.Hour, 100, 102, 98, 101, 11),
	}
	if got := aggregateTradingPeriodKLinesFromBase("US.AAPL", types.Interval1w, weeklyRows, day, day.AddDate(0, 0, 7).Add(-time.Millisecond), false); len(got) != 1 || got[0].Low != weeklyRows[1].Low {
		t.Fatalf("weekly lower-low aggregation = %#v", got)
	}
	if got := aggregateSessionAwareIntradayKLinesFromBase("US.AAPL", types.Interval2h, dayRows[:2], day.Add(15*time.Hour+30*time.Minute), day.Add(16*time.Hour), false); len(got) != 0 {
		t.Fatalf("out-of-window session aggregation = %#v", got)
	}
	if _, ok := tradingPeriodLabelForBaseKLine("US.AAPL", dayRows[0], "week", false); !ok {
		t.Fatal("intraday trading-period label was not resolved")
	}
}

func TestCoverageResolversReportUnsupportedAggregationAndClosedStoreErrors(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	if _, err := store.resolveSessionAwareIntradayAggregationBaseInterval("HK.00700", types.Interval1m, start, start.Add(time.Minute), false); err == nil {
		t.Fatal("session-aware resolver accepted a non-aggregatable interval")
	}
	if _, err := store.resolveTradingPeriodAggregationBaseInterval("US.AAPL", types.Interval1m, start, start.Add(time.Minute), false); err == nil {
		t.Fatal("trading-period resolver accepted a non-aggregatable interval")
	}
	if _, err := store.resolveSessionAwareIntradayAggregationBaseInterval("HK.00700", types.Interval2h, start, start.Add(2*time.Hour), false); err == nil {
		t.Fatal("session-aware resolver accepted missing data")
	}
	if _, err := store.resolveTradingPeriodAggregationBaseInterval("US.AAPL", types.Interval1w, start, start.AddDate(0, 0, 7), false); err == nil {
		t.Fatal("trading-period resolver accepted missing data")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Verify(nil, []string{"HK.00700"}, start, start.Add(time.Minute)); err == nil {
		t.Fatal("Verify on a closed store returned nil")
	}
	if _, err := store.QueryDailyKLinesInRange("US.AAPL", start, start.Add(24*time.Hour), true); err == nil {
		t.Fatal("extended daily query on a closed store returned nil")
	}
}

func TestAggregatedReadersDistinguishDirectSeriesCorruptBaseDataAndUnavailableStorage(t *testing.T) {
	week := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	directStore := newTestKLineStore(t)
	directWeekly := testKLine("US.AAPL", types.Interval1w, week, 7*24*time.Hour, 100, 105, 99, 104, 20)
	if err := directStore.InsertKLine(directWeekly, "forward"); err != nil {
		t.Fatalf("InsertKLine direct weekly: %v", err)
	}
	if got, err := directStore.queryAggregatedTradingPeriodKLinesForwardLocked("US.AAPL", types.Interval1w, week, 1); err != nil || got != nil {
		t.Fatalf("direct weekly forward should not synthesize: %#v, %v", got, err)
	}
	if got, err := directStore.queryAggregatedTradingPeriodKLinesBackwardLocked("US.AAPL", types.Interval1w, week.AddDate(0, 0, 7), 1); err != nil || got != nil {
		t.Fatalf("direct weekly backward should not synthesize: %#v, %v", got, err)
	}

	brokenStore := newTestKLineStore(t)
	baseTable := brokenStore.writeTableName("US.MSFT", types.Interval1d, "forward")
	if err := brokenStore.ensureKLineTable(baseTable); err != nil {
		t.Fatalf("ensure daily base table: %v", err)
	}
	if _, err := brokenStore.db.Exec(
		`INSERT INTO `+quoteIdentifier(baseTable)+` (end_time, start_time, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		timeToUnixMillis(week.Add(24*time.Hour-time.Millisecond)), timeToUnixMillis(week), "invalid-price", "105", "99", "104", "20",
	); err != nil {
		t.Fatalf("insert corrupt daily base row: %v", err)
	}
	if _, err := brokenStore.queryAggregatedTradingPeriodKLinesForwardLocked("US.MSFT", types.Interval1w, week, 1); err == nil {
		t.Fatal("forward trading aggregation accepted corrupt base data")
	}
	if _, err := brokenStore.queryAggregatedTradingPeriodKLinesBackwardLocked("US.MSFT", types.Interval1w, week.AddDate(0, 0, 7), 1); err == nil {
		t.Fatal("backward trading aggregation accepted corrupt base data")
	}

	closedStore := newTestKLineStore(t)
	if err := closedStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := closedStore.queryAggregatedTradingPeriodKLinesForwardLocked("US.AAPL", types.Interval1w, week, 1); err == nil {
		t.Fatal("forward trading aggregation on closed store returned nil")
	}
	if _, err := closedStore.queryAggregatedTradingPeriodKLinesBackwardLocked("US.AAPL", types.Interval1w, week.AddDate(0, 0, 7), 1); err == nil {
		t.Fatal("backward trading aggregation on closed store returned nil")
	}
	intradayStart := week.Add(13*time.Hour + 30*time.Minute)
	if _, err := closedStore.queryAggregatedSessionAwareIntradayKLinesForwardLocked("US.AAPL", types.Interval2h, intradayStart, 1, false); err == nil {
		t.Fatal("forward intraday aggregation on closed store returned nil")
	}
	if _, err := closedStore.queryAggregatedSessionAwareIntradayKLinesBackwardLocked("US.AAPL", types.Interval2h, intradayStart.Add(2*time.Hour), 1, false); err == nil {
		t.Fatal("backward intraday aggregation on closed store returned nil")
	}
}

func TestStorageInvariantHelpersReportUnexpectedValuesAndRetainTimeContracts(t *testing.T) {
	besteffort.LogError(errors.New("best-effort close failure"))
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("jftradeCheckedTypeAssertion did not reject an unexpected cached value")
		}
	}()
	if got := syncHistoryRequestEndTime(types.Interval("invalid"), time.Date(2026, time.June, 15, 1, 30, 0, 0, time.FixedZone("test", 8*3600))); !got.Equal(time.Date(2026, time.June, 14, 17, 30, 0, 0, time.UTC)) {
		t.Fatalf("zero-duration sync request end = %s", got)
	}
	_ = jftradeCheckedTypeAssertion[bool]("not-a-bool")
}

func TestCoverageQueriesHandleEmptyWindowsMissingTablesAndBrokenTables(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	end := start.Add(time.Minute - time.Millisecond)
	if err := store.Verify(nil, nil, start, end); err != nil {
		t.Fatalf("Verify with no symbols: %v", err)
	}
	if missing, err := store.findMissingRangesInPhysicalTable("does_not_exist", types.Interval1m, start, end); err != nil || len(missing) != 1 {
		t.Fatalf("missing physical table = %#v, %v", missing, err)
	}
	if missing, err := store.findMissingRangesInPhysicalTable("does_not_exist", types.Interval1m, end.Add(time.Millisecond), start); err != nil || len(missing) != 1 {
		t.Fatalf("inverted physical missing table = %#v, %v", missing, err)
	}
	if missing, err := store.findMissingRanges("US.AAPL", types.Interval1w, start, start.AddDate(0, 0, 7)); err != nil || len(missing) != 1 {
		t.Fatalf("trading-period missing ranges = %#v, %v", missing, err)
	}
	if missing, err := store.findMissingRanges("HK.00700", types.Interval2h, start, start.Add(2*time.Hour)); err != nil || len(missing) != 1 {
		t.Fatalf("session-aware missing ranges = %#v, %v", missing, err)
	}

	tableName := store.writeTableName("HK.00700", types.Interval1m, "forward")
	if err := store.ensureKLineTable(tableName); err != nil {
		t.Fatalf("ensureKLineTable: %v", err)
	}
	if missing, err := store.findMissingRangesInPhysicalTable(tableName, types.Interval1m, start, end); err != nil || len(missing) != 1 {
		t.Fatalf("empty physical table range = %#v, %v", missing, err)
	}
	if _, err := store.db.Exec(`DROP TABLE ` + quoteIdentifier(tableName)); err != nil {
		t.Fatalf("drop cached table: %v", err)
	}
	if _, err := store.findSelectionMissingRangesInPhysicalTable(tableName, types.Interval1m, start, end); err == nil {
		t.Fatal("selection over a dropped cached table returned nil")
	}
	if _, err := store.findMissingRangesInTable("HK.00700", types.Interval1m, "forward", start, end); err == nil {
		t.Fatal("missing-ranges lookup over a dropped cached table returned nil")
	}
	if _, err := store.queryStoredKLinesForwardFromTable(tableName, "HK.00700", types.Interval1m, start, 1); err == nil {
		t.Fatal("forward query over a dropped table returned nil")
	}
	if _, err := store.queryStoredKLinesBackwardFromTable(tableName, "HK.00700", types.Interval1m, end, 1); err == nil {
		t.Fatal("backward query over a dropped table returned nil")
	}
	// A table can disappear after it has been cached as available (for example,
	// after an interrupted database repair). The scoped readers must surface the
	// broken storage rather than treating the cached table as an empty history.
	if _, err := store.queryStoredKLinesForward("HK.00700", types.Interval1m, "forward", start, 1); err == nil {
		t.Fatal("scoped forward query over a dropped cached table returned nil")
	}
	if _, err := store.queryStoredKLinesBackward("HK.00700", types.Interval1m, "forward", end, 1); err == nil {
		t.Fatal("scoped backward query over a dropped cached table returned nil")
	}
	if _, err := store.queryStoredKLinesInRangeFromTable(tableName, "HK.00700", types.Interval1m, start, end); err == nil {
		t.Fatal("range query over a dropped table returned nil")
	}
	if err := store.streamStoredKLinesInRangeFromTable(tableName, "HK.00700", types.Interval1m, start, end, func(types.KLine) {}); err == nil {
		t.Fatal("stream over a dropped table returned nil")
	}
}

func TestSessionAwarePagingReportsCoverageGapAfterAPartialBatch(t *testing.T) {
	store := newTestKLineStore(t)
	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	if err := store.InsertKLines([]types.KLine{
		testKLine("HK.00700", types.Interval1h, start, time.Hour, 100, 102, 99, 101, 10),
		testKLine("HK.00700", types.Interval1h, start.Add(time.Hour), time.Hour, 101, 103, 98, 102, 11),
	}, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}
	if _, err := store.queryAggregatedSessionAwareIntradayKLinesForwardLocked("HK.00700", types.Interval2h, start, 2, false); err == nil || !strings.Contains(err.Error(), "missing K-line coverage") {
		t.Fatalf("partial page forward error = %v", err)
	}
	if _, err := store.queryAggregatedSessionAwareIntradayKLinesBackwardLocked("HK.00700", types.Interval2h, start.Add(2*time.Hour), 2, false); err == nil || !strings.Contains(err.Error(), "missing K-line coverage") {
		t.Fatalf("partial page backward error = %v", err)
	}
	if _, err := store.queryAggregatedTradingPeriodKLinesBackwardLocked("US.AAPL", types.Interval1w, time.Date(2026, time.June, 18, 12, 0, 0, 0, time.UTC), 1); err == nil {
		t.Fatal("mid-period backward query without coverage returned nil")
	}
}
