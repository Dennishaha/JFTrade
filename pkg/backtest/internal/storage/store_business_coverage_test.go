package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func TestSchemaHelpersExposeStableStorageContracts(t *testing.T) {
	if got := ExpectedKLineSchemaColumns(); len(got) != 7 || got[0] != "end_time:INTEGER:1" {
		t.Fatalf("ExpectedKLineSchemaColumns = %#v", got)
	}

	for _, interval := range []types.Interval{types.Interval1m, types.Interval1h, types.Interval1d, types.Interval1w} {
		stored := IntervalStorageValue(interval)
		roundTrip, err := IntervalFromStorageValue(stored)
		if err != nil {
			t.Fatalf("IntervalFromStorageValue(%d) error = %v", stored, err)
		}
		if roundTrip != interval {
			t.Fatalf("IntervalFromStorageValue(%d) = %s, want %s", stored, roundTrip, interval)
		}
	}
	if _, err := IntervalFromStorageValue(12345); err == nil || !strings.Contains(err.Error(), "unsupported stored interval value") {
		t.Fatalf("IntervalFromStorageValue(12345) error = %v", err)
	}

	legacy := KLineTableName("US:AAPL", types.Interval1m, "forward")
	if !strings.HasPrefix(legacy, KLineTable+"__us_aapl__1m__forward__") || strings.Contains(legacy, "__r__") || strings.Contains(legacy, "__x__") {
		t.Fatalf("legacy KLineTableName = %q", legacy)
	}

	regular := KLineTableNameForSessionScope("US:AAPL", types.Interval1m, "backward", KLineSessionScopeRegular)
	if !strings.Contains(regular, "__r__") || !strings.Contains(regular, "__backward__") {
		t.Fatalf("regular scoped table name = %q", regular)
	}
	extended := KLineTableNameForSessionScope("US:AAPL", types.Interval1m, "none", KLineSessionScopeExtended)
	if !strings.Contains(extended, "__x__") || !strings.Contains(extended, "__none__") {
		t.Fatalf("extended scoped table name = %q", extended)
	}

	if got := NormalizeKLineSessionScopeName(" regular "); got != KLineSessionScopeRegular {
		t.Fatalf("NormalizeKLineSessionScopeName(regular) = %q", got)
	}
	if got := NormalizeKLineSessionScopeName("unknown"); got != KLineSessionScopeLegacy {
		t.Fatalf("NormalizeKLineSessionScopeName(unknown) = %q", got)
	}
}

func TestSessionAwareIntradayAggregationAcrossHKTradingSessions(t *testing.T) {
	store := newTestKLineStore(t)

	morningStart := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	afternoonStart := time.Date(2026, time.June, 15, 5, 0, 0, 0, time.UTC)
	bars := []types.KLine{
		testKLine("HK.00700", types.Interval1h, morningStart, time.Hour, 100, 103, 99, 102, 10),
		testKLine("HK.00700", types.Interval1h, morningStart.Add(time.Hour), time.Hour, 102, 104, 101, 103, 11),
		testKLine("HK.00700", types.Interval1h, afternoonStart, time.Hour, 103, 105, 102, 104, 12),
		testKLine("HK.00700", types.Interval1h, afternoonStart.Add(time.Hour), time.Hour, 104, 106, 103, 105, 13),
	}
	if err := store.InsertKLines(bars, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	until := bars[len(bars)-1].EndTime.Time()
	aggregated, err := store.QuerySessionAwareIntradayKLinesInRange("HK.00700", types.Interval2h, morningStart, until, false)
	if err != nil {
		t.Fatalf("QuerySessionAwareIntradayKLinesInRange: %v", err)
	}
	if len(aggregated) != 2 {
		t.Fatalf("session-aware aggregated len = %d, want 2", len(aggregated))
	}
	assertAggregatedBar(t, aggregated[0], types.Interval2h, "HK.00700", morningStart, morningStart.Add(2*time.Hour).Add(-time.Millisecond), 100, 104, 99, 103, 21)
	assertAggregatedBar(t, aggregated[1], types.Interval2h, "HK.00700", afternoonStart, afternoonStart.Add(2*time.Hour).Add(-time.Millisecond), 103, 106, 102, 105, 25)

	forward, err := store.QueryKLinesForward(nil, "HK.00700", types.Interval2h, morningStart, 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward session-aware: %v", err)
	}
	if len(forward) != 2 {
		t.Fatalf("forward session-aware len = %d, want 2", len(forward))
	}
	assertAggregatedBar(t, forward[0], types.Interval2h, "HK.00700", morningStart, morningStart.Add(2*time.Hour).Add(-time.Millisecond), 100, 104, 99, 103, 21)
	assertAggregatedBar(t, forward[1], types.Interval2h, "HK.00700", afternoonStart, afternoonStart.Add(2*time.Hour).Add(-time.Millisecond), 103, 106, 102, 105, 25)

	backward, err := store.QueryKLinesBackward(nil, "HK.00700", types.Interval2h, until.Add(time.Millisecond), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward session-aware: %v", err)
	}
	if len(backward) != 2 {
		t.Fatalf("backward session-aware len = %d, want 2", len(backward))
	}
	assertAggregatedBar(t, backward[0], types.Interval2h, "HK.00700", morningStart, morningStart.Add(2*time.Hour).Add(-time.Millisecond), 100, 104, 99, 103, 21)
	assertAggregatedBar(t, backward[1], types.Interval2h, "HK.00700", afternoonStart, afternoonStart.Add(2*time.Hour).Add(-time.Millisecond), 103, 106, 102, 105, 25)
}

func TestQuerySessionAwareIntradayKLinesInRangeSupportsUSExtendedHours(t *testing.T) {
	store := newTestKLineStore(t)

	preStart := time.Date(2026, time.June, 12, 8, 0, 0, 0, time.UTC)
	regularStart := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	bars := []types.KLine{
		testKLine("US.AAPL", types.Interval1h, preStart, time.Hour, 200, 202, 199, 201, 10),
		testKLine("US.AAPL", types.Interval1h, preStart.Add(time.Hour), time.Hour, 201, 203, 200, 202, 11),
		testKLine("US.AAPL", types.Interval1h, regularStart, time.Hour, 202, 205, 201, 204, 12),
		testKLine("US.AAPL", types.Interval1h, regularStart.Add(time.Hour), time.Hour, 204, 206, 203, 205, 13),
	}
	if err := store.InsertKLines(bars, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	until := bars[len(bars)-1].EndTime.Time()
	aggregated, err := store.QuerySessionAwareIntradayKLinesInRange("US.AAPL", types.Interval2h, preStart, until, true)
	if err != nil {
		t.Fatalf("QuerySessionAwareIntradayKLinesInRange extended: %v", err)
	}
	if len(aggregated) != 2 {
		t.Fatalf("extended session-aware aggregated len = %d, want 2", len(aggregated))
	}
	assertAggregatedBar(t, aggregated[0], types.Interval2h, "US.AAPL", preStart, preStart.Add(2*time.Hour).Add(-time.Millisecond), 200, 203, 199, 202, 21)
	assertAggregatedBar(t, aggregated[1], types.Interval2h, "US.AAPL", regularStart, regularStart.Add(2*time.Hour).Add(-time.Millisecond), 202, 206, 201, 205, 25)
}

func TestQueryAPIsSortMultiSymbolAndMixedIntervalResults(t *testing.T) {
	store := newTestKLineStore(t)

	sameStart := time.Date(2026, time.June, 15, 1, 34, 0, 0, time.UTC)
	if err := store.InsertKLines([]types.KLine{
		testKLine("HK.00005", types.Interval1m, sameStart, time.Minute, 50, 51, 49, 50.5, 10),
		testKLine("HK.00700", types.Interval1m, sameStart, time.Minute, 100, 101, 99, 100.5, 20),
	}, "forward"); err != nil {
		t.Fatalf("InsertKLines multi-symbol: %v", err)
	}

	ch, errCh := store.QueryKLinesCh(sameStart, sameStart.Add(time.Minute).Add(-time.Millisecond), nil, []string{"HK.00700", "HK.00005"}, []types.Interval{types.Interval1m})
	channelRows := consumeKLineChannel(t, ch, errCh)
	if len(channelRows) != 2 {
		t.Fatalf("channelRows len = %d, want 2", len(channelRows))
	}
	if channelRows[0].Symbol != "HK.00005" || channelRows[1].Symbol != "HK.00700" {
		t.Fatalf("channelRows order = %#v", channelRows)
	}

	streamStore := newTestKLineStore(t)
	streamStart := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	var minuteBars []types.KLine
	for index, closePrice := range []float64{101, 102, 103, 104, 105} {
		minuteBars = append(minuteBars, testKLine(
			"HK.00700",
			types.Interval1m,
			streamStart.Add(time.Duration(index)*time.Minute),
			time.Minute,
			100+float64(index),
			101+float64(index),
			99+float64(index),
			closePrice,
			10+float64(index),
		))
	}
	if err := streamStore.InsertKLines(minuteBars, "forward"); err != nil {
		t.Fatalf("InsertKLines streamStore: %v", err)
	}

	var streamed []types.KLine
	until := minuteBars[len(minuteBars)-1].EndTime.Time()
	if err := streamStore.StreamKLines(streamStart, until, nil, []string{"HK.00700"}, []types.Interval{types.Interval5m, types.Interval1m}, func(kline types.KLine) {
		streamed = append(streamed, kline)
	}); err != nil {
		t.Fatalf("StreamKLines mixed intervals: %v", err)
	}
	if len(streamed) != 6 {
		t.Fatalf("streamed len = %d, want 6", len(streamed))
	}
	lastMinute := streamed[len(streamed)-2]
	aggregatedFiveMinute := streamed[len(streamed)-1]
	if lastMinute.Interval != types.Interval1m || aggregatedFiveMinute.Interval != types.Interval5m {
		t.Fatalf("expected 1m row before 5m aggregate at same end time, got %#v", streamed[len(streamed)-2:])
	}
	if !lastMinute.EndTime.Time().Equal(aggregatedFiveMinute.EndTime.Time()) {
		t.Fatalf("expected tied end times for interval sort, got %s and %s", lastMinute.EndTime.Time(), aggregatedFiveMinute.EndTime.Time())
	}
}
