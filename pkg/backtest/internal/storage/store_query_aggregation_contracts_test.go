package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestStoreRoundTripsScopedSeriesQueries(t *testing.T) {
	store := newTestKLineStore(t)
	store.SetWriteSessionScope("regular")
	store.SetReadSessionScope("regular")
	store.SetRehabType("backward")

	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	bars := []types.KLine{
		testKLine("HK.00700", types.Interval1m, start, time.Minute, 100, 101, 99, 100.5, 10),
		testKLine("HK.00700", types.Interval1m, start.Add(time.Minute), time.Minute, 100.5, 102, 100, 101.5, 11),
		testKLine("HK.00700", types.Interval1m, start.Add(2*time.Minute), time.Minute, 101.5, 103, 101, 102.5, 12),
	}
	if err := store.InsertKLines(bars, "backward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}

	last, err := store.QueryKLine(nil, "HK.00700", types.Interval1m, "DESC", 1)
	if err != nil {
		t.Fatalf("QueryKLine DESC: %v", err)
	}
	if last == nil || !last.EndTime.Time().Equal(bars[2].EndTime.Time()) || last.Close != bars[2].Close {
		t.Fatalf("last kline = %#v", last)
	}

	first, err := store.QueryKLine(nil, "HK.00700", types.Interval1m, "ASC", 1)
	if err != nil {
		t.Fatalf("QueryKLine ASC: %v", err)
	}
	if first == nil || !first.EndTime.Time().Equal(bars[0].EndTime.Time()) || first.Open != bars[0].Open {
		t.Fatalf("first kline = %#v", first)
	}

	forward, err := store.QueryKLinesForward(nil, "HK.00700", types.Interval1m, bars[1].StartTime.Time(), 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward: %v", err)
	}
	if len(forward) != 2 || !forward[0].EndTime.Time().Equal(bars[1].EndTime.Time()) || !forward[1].EndTime.Time().Equal(bars[2].EndTime.Time()) {
		t.Fatalf("forward = %#v", forward)
	}

	backward, err := store.QueryKLinesBackward(nil, "HK.00700", types.Interval1m, bars[1].EndTime.Time(), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward: %v", err)
	}
	if len(backward) != 2 || !backward[0].EndTime.Time().Equal(bars[0].EndTime.Time()) || !backward[1].EndTime.Time().Equal(bars[1].EndTime.Time()) {
		t.Fatalf("backward = %#v", backward)
	}

	var streamed []types.KLine
	if err := store.StreamKLines(bars[0].StartTime.Time(), bars[1].EndTime.Time(), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
		streamed = append(streamed, kline)
	}); err != nil {
		t.Fatalf("StreamKLines: %v", err)
	}
	if len(streamed) != 2 || !streamed[0].EndTime.Time().Equal(bars[0].EndTime.Time()) || !streamed[1].EndTime.Time().Equal(bars[1].EndTime.Time()) {
		t.Fatalf("streamed = %#v", streamed)
	}

	ch, errCh := store.QueryKLinesCh(bars[0].StartTime.Time(), bars[1].EndTime.Time(), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m})
	channelRows := consumeKLineChannel(t, ch, errCh)
	if len(channelRows) != 2 || !channelRows[0].EndTime.Time().Equal(bars[0].EndTime.Time()) || !channelRows[1].EndTime.Time().Equal(bars[1].EndTime.Time()) {
		t.Fatalf("channel rows = %#v", channelRows)
	}

	if err := store.EnsureCoverage("HK.00700", types.Interval1m, bars[0].StartTime.Time(), bars[2].EndTime.Time()); err != nil {
		t.Fatalf("EnsureCoverage: %v", err)
	}
}

func TestStoreAggregatesFiveMinuteBarsFromOneMinuteCoverage(t *testing.T) {
	store := newTestKLineStore(t)

	start := time.Date(2026, time.June, 15, 1, 30, 0, 0, time.UTC)
	var bars []types.KLine
	for index, closePrice := range []float64{101, 103, 102, 104, 105} {
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

	since := bars[0].StartTime.Time()
	until := bars[len(bars)-1].EndTime.Time()
	if err := store.EnsureCoverage("HK.00700", types.Interval5m, since, until); err != nil {
		t.Fatalf("EnsureCoverage aggregated interval: %v", err)
	}

	forward, err := store.QueryKLinesForward(nil, "HK.00700", types.Interval5m, since, 1)
	if err != nil {
		t.Fatalf("QueryKLinesForward aggregated: %v", err)
	}
	if len(forward) != 1 {
		t.Fatalf("aggregated forward len = %d, want 1", len(forward))
	}
	assertAggregatedBar(t, forward[0], types.Interval5m, "HK.00700", since, until, 100, 105, 99, 105, 60)

	backward, err := store.QueryKLinesBackward(nil, "HK.00700", types.Interval5m, until.Add(time.Millisecond), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward aggregated: %v", err)
	}
	if len(backward) != 1 {
		t.Fatalf("aggregated backward len = %d, want 1", len(backward))
	}
	assertAggregatedBar(t, backward[0], types.Interval5m, "HK.00700", since, until, 100, 105, 99, 105, 60)

	var streamed []types.KLine
	if err := store.StreamKLines(since, until, nil, []string{"HK.00700"}, []types.Interval{types.Interval5m}, func(kline types.KLine) {
		streamed = append(streamed, kline)
	}); err != nil {
		t.Fatalf("StreamKLines aggregated: %v", err)
	}
	if len(streamed) != 1 {
		t.Fatalf("streamed aggregated len = %d, want 1", len(streamed))
	}
	assertAggregatedBar(t, streamed[0], types.Interval5m, "HK.00700", since, until, 100, 105, 99, 105, 60)

	ch, errCh := store.QueryKLinesCh(since, until, nil, []string{"HK.00700"}, []types.Interval{types.Interval1m, types.Interval5m})
	channelRows := consumeKLineChannel(t, ch, errCh)
	if len(channelRows) != 6 {
		t.Fatalf("multi-interval channel rows len = %d, want 6: %#v", len(channelRows), channelRows)
	}
	lastMinute := channelRows[len(channelRows)-2]
	aggregatedFiveMinute := channelRows[len(channelRows)-1]
	if lastMinute.Interval != types.Interval1m || aggregatedFiveMinute.Interval != types.Interval5m {
		t.Fatalf("expected 1m row before tied 5m aggregate, got %#v", channelRows[len(channelRows)-2:])
	}
	if !lastMinute.EndTime.Time().Equal(aggregatedFiveMinute.EndTime.Time()) {
		t.Fatalf("expected tied end times for interval sort, got %s and %s", lastMinute.EndTime.Time(), aggregatedFiveMinute.EndTime.Time())
	}

	emptyCh, emptyErrCh := store.QueryKLinesCh(since, until, nil, nil, []types.Interval{types.Interval1m})
	if emptyRows := consumeKLineChannel(t, emptyCh, emptyErrCh); len(emptyRows) != 0 {
		t.Fatalf("empty symbol channel rows = %#v", emptyRows)
	}
}

func TestStoreAggregatesDailyAndWeeklyBarsFromLowerIntervals(t *testing.T) {
	store := newTestKLineStore(t)

	usDay := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	hourlyBars := []types.KLine{
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 13, 30, 0, 0, time.UTC), time.Hour, 100, 103, 99, 102, 10),
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 14, 30, 0, 0, time.UTC), time.Hour, 102, 104, 101, 103, 11),
		testKLine("US.AAPL", types.Interval1h, time.Date(2026, time.June, 15, 15, 30, 0, 0, time.UTC), time.Hour, 103, 105, 102, 104, 12),
	}
	if err := store.InsertKLines(hourlyBars, "forward"); err != nil {
		t.Fatalf("InsertKLines hourly: %v", err)
	}

	dailyBars, err := store.QueryDailyKLinesInRange("US.AAPL", usDay, usDay.Add(24*time.Hour).Add(-time.Millisecond), false)
	if err != nil {
		t.Fatalf("QueryDailyKLinesInRange: %v", err)
	}
	if len(dailyBars) != 1 {
		t.Fatalf("daily bars len = %d, want 1", len(dailyBars))
	}
	assertAggregatedBar(t, dailyBars[0], types.Interval1d, "US.AAPL", usDay, usDay.Add(24*time.Hour).Add(-time.Millisecond), 100, 105, 99, 104, 33)

	weeklyBase := []types.KLine{
		testKLine("US.AAPL", types.Interval1d, usDay, 24*time.Hour, 200, 205, 198, 203, 100),
		testKLine("US.AAPL", types.Interval1d, usDay.Add(24*time.Hour), 24*time.Hour, 203, 206, 202, 205, 110),
	}
	if err := store.InsertKLines(weeklyBase, "forward"); err != nil {
		t.Fatalf("InsertKLines daily: %v", err)
	}

	weeklyBars, err := store.QueryTradingPeriodKLinesInRange("US.AAPL", types.Interval1w, usDay, usDay.AddDate(0, 0, 6).Add(24*time.Hour).Add(-time.Millisecond), false)
	if err != nil {
		t.Fatalf("QueryTradingPeriodKLinesInRange: %v", err)
	}
	if len(weeklyBars) != 1 {
		t.Fatalf("weekly bars len = %d, want 1", len(weeklyBars))
	}
	assertAggregatedBar(t, weeklyBars[0], types.Interval1w, "US.AAPL", usDay, time.Date(2026, time.June, 21, 23, 59, 59, int(999*time.Millisecond), time.UTC), 200, 206, 198, 205, 210)
}

func newTestKLineStore(t *testing.T) *FutuKLineStore {
	t.Helper()
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close store: %v", err)
		}
	})
	return store
}

func testKLine(symbol string, interval types.Interval, start time.Time, duration time.Duration, open, high, low, close, volume float64) types.KLine {
	return types.KLine{
		StartTime: types.Time(start.UTC()),
		EndTime:   types.Time(start.UTC().Add(duration).Add(-time.Millisecond)),
		Interval:  interval,
		Symbol:    symbol,
		Open:      fixedpoint.NewFromFloat(open),
		High:      fixedpoint.NewFromFloat(high),
		Low:       fixedpoint.NewFromFloat(low),
		Close:     fixedpoint.NewFromFloat(close),
		Volume:    fixedpoint.NewFromFloat(volume),
		Closed:    true,
	}
}

func consumeKLineChannel(t *testing.T, ch <-chan types.KLine, errCh <-chan error) []types.KLine {
	t.Helper()
	var rows []types.KLine
	for row := range ch {
		rows = append(rows, row)
	}
	for err := range errCh {
		if err != nil {
			t.Fatalf("QueryKLinesCh error: %v", err)
		}
	}
	return rows
}

func assertAggregatedBar(t *testing.T, kline types.KLine, interval types.Interval, symbol string, start, end time.Time, open, high, low, close, volume float64) {
	t.Helper()
	if kline.Interval != interval || kline.Symbol != symbol {
		t.Fatalf("kline interval/symbol = %s/%s, want %s/%s", kline.Interval, kline.Symbol, interval, symbol)
	}
	if !kline.StartTime.Time().Equal(start.UTC()) || !kline.EndTime.Time().Equal(end.UTC()) {
		t.Fatalf("kline time range = %s..%s, want %s..%s", kline.StartTime.Time(), kline.EndTime.Time(), start.UTC(), end.UTC())
	}
	if kline.Open != fixedpoint.NewFromFloat(open) ||
		kline.High != fixedpoint.NewFromFloat(high) ||
		kline.Low != fixedpoint.NewFromFloat(low) ||
		kline.Close != fixedpoint.NewFromFloat(close) ||
		kline.Volume != fixedpoint.NewFromFloat(volume) {
		t.Fatalf("aggregated kline = %#v", kline)
	}
}
