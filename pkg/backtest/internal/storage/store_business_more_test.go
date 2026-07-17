package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestInsertKLineReplacesExistingBarAndQueryDefaultsToLatest(t *testing.T) {
	store := newTestKLineStore(t)

	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	first := testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10)
	replacement := testKLine("HK.00700", types.Interval1m, start, time.Minute, 101, 103, 100, 102.5, 25)

	if err := store.InsertKLine(first, "forward"); err != nil {
		t.Fatalf("InsertKLine first: %v", err)
	}
	if err := store.InsertKLine(replacement, "forward"); err != nil {
		t.Fatalf("InsertKLine replacement: %v", err)
	}

	got, err := store.QueryKLine(nil, "HK.00700", types.Interval1m, "invalid-order", 0)
	if err != nil {
		t.Fatalf("QueryKLine: %v", err)
	}
	if got == nil {
		t.Fatal("QueryKLine returned nil")
	}
	assertAggregatedBar(t, *got, types.Interval1m, "HK.00700", replacement.StartTime.Time(), replacement.EndTime.Time(), 101, 103, 100, 102.5, 25)
}

func TestFindMissingRangesUsesLowerIntervalCoverageAndVerifyAllowsOpenWindow(t *testing.T) {
	store := newTestKLineStore(t)

	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	var bars []types.KLine
	for index, closePrice := range []float64{101, 102, 103, 104, 105} {
		bars = append(bars, testKLine(
			"HK.00700",
			types.Interval1m,
			start.Add(time.Duration(index)*time.Minute),
			time.Minute,
			100+float64(index),
			101+float64(index),
			99+float64(index),
			closePrice,
			10+float64(index),
		))
	}
	if err := store.InsertKLines(bars, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	missing, err := store.findMissingRanges("HK.00700", types.Interval5m, start, bars[len(bars)-1].EndTime.Time())
	if err != nil {
		t.Fatalf("findMissingRanges aggregated: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("findMissingRanges aggregated = %#v, want none", missing)
	}

	missing, err = store.findMissingRanges("HK.00005", types.Interval15m, start, start.Add(15*time.Minute).Add(-time.Millisecond))
	if err != nil {
		t.Fatalf("findMissingRanges direct-missing: %v", err)
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "missing") {
		t.Fatalf("findMissingRanges direct-missing = %#v", missing)
	}

	openWindowAt := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	if err := store.Verify(nil, []string{"HK.00700"}, openWindowAt, openWindowAt); err == nil || !strings.Contains(err.Error(), "missing ranges") {
		t.Fatalf("Verify missing coverage error = %v", err)
	}
	if err := store.Sync(t.Context(), nil, "HK.00700", []types.Interval{types.Interval1m}, openWindowAt, openWindowAt.Add(time.Minute)); err != nil {
		t.Fatalf("Sync no-op: %v", err)
	}
}

func TestQueryKLinesForwardAndBackwardAggregateWeeklyTradingPeriods(t *testing.T) {
	store := newTestKLineStore(t)

	weekOne := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	weekTwo := weekOne.AddDate(0, 0, 7)
	dailyBars := []types.KLine{
		testKLine("US.AAPL", types.Interval1d, weekOne, 24*time.Hour, 200, 205, 198, 203, 100),
		testKLine("US.AAPL", types.Interval1d, weekOne.AddDate(0, 0, 1), 24*time.Hour, 203, 206, 202, 205, 110),
		testKLine("US.AAPL", types.Interval1d, weekTwo, 24*time.Hour, 206, 209, 205, 208, 120),
		testKLine("US.AAPL", types.Interval1d, weekTwo.AddDate(0, 0, 1), 24*time.Hour, 208, 210, 207, 209, 130),
	}
	if err := store.InsertKLines(dailyBars, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	forward, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1w, weekOne, 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward weekly: %v", err)
	}
	if len(forward) != 2 {
		t.Fatalf("weekly forward len = %d, want 2", len(forward))
	}
	assertAggregatedBar(t, forward[0], types.Interval1w, "US.AAPL", weekOne, time.Date(2026, time.June, 21, 23, 59, 59, int(999*time.Millisecond), time.UTC), 200, 206, 198, 205, 210)
	assertAggregatedBar(t, forward[1], types.Interval1w, "US.AAPL", weekTwo, time.Date(2026, time.June, 28, 23, 59, 59, int(999*time.Millisecond), time.UTC), 206, 210, 205, 209, 250)

	backward, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1w, time.Date(2026, time.June, 29, 0, 0, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward weekly: %v", err)
	}
	if len(backward) != 2 {
		t.Fatalf("weekly backward len = %d, want 2", len(backward))
	}
	assertAggregatedBar(t, backward[0], types.Interval1w, "US.AAPL", weekOne, time.Date(2026, time.June, 21, 23, 59, 59, int(999*time.Millisecond), time.UTC), 200, 206, 198, 205, 210)
	assertAggregatedBar(t, backward[1], types.Interval1w, "US.AAPL", weekTwo, time.Date(2026, time.June, 28, 23, 59, 59, int(999*time.Millisecond), time.UTC), 206, 210, 205, 209, 250)

	// A backward query inside an unfinished week must return the most recent
	// fully closed week, rather than producing a partial period.
	insideCurrentWeek, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1w, weekTwo.AddDate(0, 0, 8).Add(12*time.Hour), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward inside current week: %v", err)
	}
	if len(insideCurrentWeek) != 1 {
		t.Fatalf("inside-current-week backward len = %d, want 1", len(insideCurrentWeek))
	}
	assertAggregatedBar(t, insideCurrentWeek[0], types.Interval1w, "US.AAPL", weekTwo, time.Date(2026, time.June, 28, 23, 59, 59, int(999*time.Millisecond), time.UTC), 206, 210, 205, 209, 250)

	// K-line timestamps have nanosecond precision while a canonical trading
	// period closes at millisecond precision. A request just after that close
	// belongs to the following period.
	afterFirstWeekClose := time.Date(2026, time.June, 21, 23, 59, 59, 999_999_999, time.UTC)
	forwardAfterClose, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1w, afterFirstWeekClose, 1)
	if err != nil {
		t.Fatalf("QueryKLinesForward after weekly close: %v", err)
	}
	if len(forwardAfterClose) != 1 {
		t.Fatalf("after-close forward len = %d, want 1", len(forwardAfterClose))
	}
	assertAggregatedBar(t, forwardAfterClose[0], types.Interval1w, "US.AAPL", weekTwo, time.Date(2026, time.June, 28, 23, 59, 59, int(999*time.Millisecond), time.UTC), 206, 210, 205, 209, 250)
}

func TestQueryDailyKLinesInRangeAggregatesUSExtendedHoursFromHourlyBars(t *testing.T) {
	store := newTestKLineStore(t)

	base := []types.KLine{
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 8, 0, 0, 0, time.UTC), time.Hour, 200, 202, 199, 201, 10),
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 9, 0, 0, 0, time.UTC), time.Hour, 201, 203, 200, 202, 11),
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 13, 30, 0, 0, time.UTC), time.Hour, 202, 205, 201, 204, 12),
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 14, 30, 0, 0, time.UTC), time.Hour, 204, 206, 203, 205, 13),
	}
	if err := store.InsertKLines(base, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	dayStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	aggregated, err := store.QueryDailyKLinesInRange("US.AAPL", dayStart, dayStart.Add(24*time.Hour).Add(-time.Millisecond), true)
	if err != nil {
		t.Fatalf("QueryDailyKLinesInRange extended: %v", err)
	}
	if len(aggregated) != 1 {
		t.Fatalf("extended daily len = %d, want 1", len(aggregated))
	}
	assertAggregatedBar(t, aggregated[0], types.Interval1d, "US.AAPL", dayStart, dayStart.Add(24*time.Hour).Add(-time.Millisecond), 200, 206, 199, 205, 46)
}

func TestAggregationMissingCoverageMessagesAndDailyFallbacks(t *testing.T) {
	store := newTestKLineStore(t)

	dayStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour).Add(-time.Millisecond)
	if got, err := store.QueryDailyKLinesInRange("US.AAPL", dayEnd.Add(time.Millisecond), dayStart, false); err != nil || got != nil {
		t.Fatalf("empty daily range = %+v err=%v, want nil nil", got, err)
	}
	if _, err := store.QueryDailyKLinesInRange("US.AAPL", dayStart, dayEnd, false); err == nil || !strings.Contains(err.Error(), "download 12h data covering the full range") {
		t.Fatalf("regular daily missing coverage err = %v", err)
	}
	if _, err := store.QueryDailyKLinesInRange("US.AAPL", dayStart, dayEnd, true); err == nil || !strings.Contains(err.Error(), "extended-hours daily aggregation") || !strings.Contains(err.Error(), "download 1h data") {
		t.Fatalf("extended daily missing coverage err = %v", err)
	}
	if _, err := store.QueryTradingPeriodKLinesInRange("US.AAPL", types.Interval1w, dayStart, dayStart.AddDate(0, 0, 7).Add(-time.Millisecond), true); err == nil || !strings.Contains(err.Error(), "extended-hours trading-period aggregation") {
		t.Fatalf("extended trading-period missing coverage err = %v", err)
	}
	if _, err := store.QuerySessionAwareIntradayKLinesInRange("US.AAPL", types.Interval2h, dayStart.Add(13*time.Hour+30*time.Minute), dayStart.Add(15*time.Hour+30*time.Minute), true); err == nil || !strings.Contains(err.Error(), "extended-hours intraday aggregation") {
		t.Fatalf("extended intraday missing coverage err = %v", err)
	}
	if got, err := store.QueryTradingPeriodKLinesInRange("US.AAPL", types.Interval2h, dayStart, dayEnd, false); err != nil || got != nil {
		t.Fatalf("non trading-period aggregate = %+v err=%v, want nil nil", got, err)
	}
	if got, err := store.QuerySessionAwareIntradayKLinesInRange("UNKNOWN", types.Interval2h, dayStart, dayEnd, false); err != nil || got != nil {
		t.Fatalf("unknown symbol session-aware aggregate = %+v err=%v, want nil nil", got, err)
	}

	storedDaily := testKLine("US.AAPL", types.Interval1d, dayStart, 24*time.Hour, 300, 305, 299, 304, 120)
	if err := store.InsertKLine(storedDaily, "forward"); err != nil {
		t.Fatalf("InsertKLine stored daily: %v", err)
	}
	fallback, err := store.QueryDailyKLinesInRange("US.AAPL", dayStart, dayEnd, true)
	if err != nil {
		t.Fatalf("QueryDailyKLinesInRange stored daily fallback: %v", err)
	}
	if len(fallback) != 1 {
		t.Fatalf("stored daily fallback len = %d, want 1", len(fallback))
	}
	assertAggregatedBar(t, fallback[0], types.Interval1d, "US.AAPL", dayStart, dayEnd, 300, 305, 299, 304, 120)
}

func TestAggregationPureHelpersSkipUnsupportedOrUnlabelledRows(t *testing.T) {
	dayStart := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	if got := aggregateTradingPeriodKLinesFromBase("US.AAPL", types.Interval4h, []types.KLine{
		testKLine("US.AAPL", types.Interval1d, dayStart, 24*time.Hour, 100, 101, 99, 100.5, 10),
	}, dayStart, dayStart.Add(24*time.Hour), false); got != nil {
		t.Fatalf("unsupported trading-period aggregation = %#v, want nil", got)
	}

	baseWithZeroStart := types.KLine{
		EndTime:  types.Time(dayStart.Add(15*time.Hour + 30*time.Minute)),
		Interval: types.Interval1h,
		Symbol:   "US.AAPL",
		Open:     fixedpoint.NewFromFloat(100),
		High:     fixedpoint.NewFromFloat(101),
		Low:      fixedpoint.NewFromFloat(99),
		Close:    fixedpoint.NewFromFloat(100.5),
		Volume:   fixedpoint.NewFromFloat(10),
		Closed:   true,
	}
	aggregated := aggregateDailyKLinesFromBase("US.AAPL", []types.KLine{baseWithZeroStart}, dayStart, dayStart.Add(24*time.Hour).Add(-time.Millisecond), false)
	if len(aggregated) != 1 {
		t.Fatalf("daily aggregation from zero-start row len = %d, want 1", len(aggregated))
	}
	if got := dailyAggregationObservedAt(baseWithZeroStart); !got.Equal(baseWithZeroStart.EndTime.Time()) {
		t.Fatalf("dailyAggregationObservedAt zero start = %s, want end time", got)
	}
}
