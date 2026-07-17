package backtest

import (
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestSessionFilteredReplayStoreCoverage98PassThroughAndCursorBoundaries(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	regular := testBacktestKLine("US.AAPL", types.Interval1m, start, time.Minute, 1)
	nextRegular := testBacktestKLine("US.AAPL", types.Interval1m, start.Add(time.Minute), time.Minute, 2)

	t.Run("delegates ordinary or extended-hours queries without replay filtering", func(t *testing.T) {
		base := &stubBacktestStore{
			forwardBatches:  [][]types.KLine{{regular}},
			backwardBatches: [][]types.KLine{{regular}},
		}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: true}
		if _, err := store.QueryKLine(nil, "US.AAPL", types.Interval1m, "DESC", 1); err != nil {
			t.Fatalf("QueryKLine delegated error = %v", err)
		}
		if _, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, start, 1); err != nil {
			t.Fatalf("QueryKLinesForward delegated error = %v", err)
		}
		if _, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, start, 1); err != nil {
			t.Fatalf("QueryKLinesBackward delegated error = %v", err)
		}
		if base.queryKLineCalls != 1 || base.forwardCalls != 1 || base.backwardCalls != 1 {
			t.Fatalf("delegated calls = query=%d forward=%d backward=%d", base.queryKLineCalls, base.forwardCalls, base.backwardCalls)
		}
	})

	t.Run("ends filtered paging cleanly for an empty source and ignores stale cursor rows", func(t *testing.T) {
		empty := &sessionFilteredBacktestStore{base: &stubBacktestStore{}, includeExtendedHours: false}
		if rows, err := empty.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, start, 2); err != nil || len(rows) != 0 {
			t.Fatalf("empty forward rows=%#v err=%v", rows, err)
		}
		if rows, err := empty.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, start, 2); err != nil || len(rows) != 0 {
			t.Fatalf("empty backward rows=%#v err=%v", rows, err)
		}

		cursor := start.Add(30 * time.Minute)
		stale := testBacktestKLine("US.AAPL", types.Interval1m, cursor.Add(-2*time.Minute), time.Minute, 3)
		forward := &sessionFilteredBacktestStore{
			base:                 &stubBacktestStore{forwardBatches: [][]types.KLine{{stale}}},
			includeExtendedHours: false,
		}
		if rows, err := forward.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, cursor, 2); err != nil || len(rows) != 1 {
			t.Fatalf("stale forward rows=%#v err=%v", rows, err)
		}
		backward := &sessionFilteredBacktestStore{
			base:                 &stubBacktestStore{backwardBatches: [][]types.KLine{{nextRegular}}},
			includeExtendedHours: false,
		}
		if rows, err := backward.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, start, 2); err != nil || len(rows) != 1 {
			t.Fatalf("stale backward rows=%#v err=%v", rows, err)
		}
	})

	t.Run("keeps the latest bounded regular-session window when a page contains multiple bars", func(t *testing.T) {
		store := &sessionFilteredBacktestStore{
			base:                 &stubBacktestStore{backwardBatches: [][]types.KLine{{regular, nextRegular}}},
			includeExtendedHours: false,
		}
		rows, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, nextRegular.EndTime.Time().Add(time.Minute), 1)
		if err != nil {
			t.Fatalf("QueryKLinesBackward bounded error = %v", err)
		}
		if len(rows) != 1 || !rows[0].StartTime.Time().Equal(nextRegular.StartTime.Time()) {
			t.Fatalf("bounded backward rows=%#v", rows)
		}
	})
}

func TestSessionFilteredReplayStoreCoverage98StreamingContracts(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	hk := testBacktestKLine("HK.00700", types.Interval1m, start, time.Minute, 1)

	t.Run("leaves non-US query channels untouched when no session policy applies", func(t *testing.T) {
		base := &stubBacktestStore{queryChRows: []types.KLine{hk}}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}
		rows, err := collectKLinesFromStoreChannels(store.QueryKLinesCh(start, start.Add(time.Minute), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m}))
		if err != nil || len(rows) != 1 || rows[0].Symbol != "HK.00700" {
			t.Fatalf("unfiltered channel rows=%#v err=%v", rows, err)
		}
	})

	t.Run("handles nil consumers, direct streamers, and channel fallbacks", func(t *testing.T) {
		streamer := &stubStreamerRangeBacktestStore{
			stubRangeBacktestStore: &stubRangeBacktestStore{stubBacktestStore: &stubBacktestStore{}},
			streamRows:             []types.KLine{hk},
		}
		store := &sessionFilteredBacktestStore{base: streamer, includeExtendedHours: false}
		if err := store.StreamKLines(start, start.Add(time.Minute), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m}, nil); err != nil {
			t.Fatalf("StreamKLines(nil emit) error = %v", err)
		}
		if streamer.streamCalls != 0 {
			t.Fatalf("nil emit should not invoke the base streamer: %d", streamer.streamCalls)
		}
		var streamed []types.KLine
		if err := store.StreamKLines(start, start.Add(time.Minute), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
			streamed = append(streamed, kline)
		}); err != nil {
			t.Fatalf("direct StreamKLines error = %v", err)
		}
		if len(streamed) != 1 || streamer.streamCalls != 1 {
			t.Fatalf("direct stream rows=%#v calls=%d", streamed, streamer.streamCalls)
		}

		channelStore := &sessionFilteredBacktestStore{
			base:                 &stubBacktestStore{queryChRows: []types.KLine{hk}},
			includeExtendedHours: false,
		}
		var fromChannel []types.KLine
		if err := channelStore.StreamKLines(start, start.Add(time.Minute), nil, []string{"HK.00700"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
			fromChannel = append(fromChannel, kline)
		}); err != nil {
			t.Fatalf("channel fallback StreamKLines error = %v", err)
		}
		if len(fromChannel) != 1 || fromChannel[0].Symbol != "HK.00700" {
			t.Fatalf("channel fallback rows=%#v", fromChannel)
		}
	})

	t.Run("propagates the source collection error before custom extended aggregation", func(t *testing.T) {
		expected := errors.New("source stream disconnected")
		base := &stubStreamerRangeBacktestStore{
			stubRangeBacktestStore: &stubRangeBacktestStore{stubBacktestStore: &stubBacktestStore{}},
			streamErr:              expected,
		}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: true}
		if err := store.StreamKLines(start, start.Add(24*time.Hour), nil, []string{"US.AAPL"}, []types.Interval{types.Interval1d}, func(types.KLine) {}); !errors.Is(err, expected) {
			t.Fatalf("custom StreamKLines error=%v, want %v", err, expected)
		}
	})
}

func TestSessionFilteredReplayStoreCoverage98CustomAggregationFailuresAndOrdering(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)

	t.Run("surfaces range errors and empty pages for session-aware replay", func(t *testing.T) {
		tradingFailure := errors.New("daily range unavailable")
		store := &sessionFilteredBacktestStore{base: &stubRangeBacktestStore{
			stubBacktestStore: &stubBacktestStore{}, tradingErr: tradingFailure,
		}, includeExtendedHours: true}
		if _, err := store.queryCustomTradingPeriodForward("US.AAPL", types.Interval1d, start, 2); !errors.Is(err, tradingFailure) {
			t.Fatalf("trading forward error=%v", err)
		}
		if _, err := store.queryCustomTradingPeriodBackward("US.AAPL", types.Interval1d, start.Add(48*time.Hour), 2); !errors.Is(err, tradingFailure) {
			t.Fatalf("trading backward error=%v", err)
		}

		sessionFailure := errors.New("intraday range unavailable")
		store = &sessionFilteredBacktestStore{base: &stubRangeBacktestStore{
			stubBacktestStore: &stubBacktestStore{}, sessionErr: sessionFailure,
		}, includeExtendedHours: true}
		if _, err := store.queryCustomSessionAwareIntradayForward("US.AAPL", types.Interval2h, start, 2); !errors.Is(err, sessionFailure) {
			t.Fatalf("session forward error=%v", err)
		}
		if _, err := store.queryCustomSessionAwareIntradayBackward("US.AAPL", types.Interval2h, start.Add(6*time.Hour), 2); !errors.Is(err, sessionFailure) {
			t.Fatalf("session backward error=%v", err)
		}

		empty := &sessionFilteredBacktestStore{base: &stubRangeBacktestStore{stubBacktestStore: &stubBacktestStore{}}, includeExtendedHours: true}
		if rows, err := empty.queryCustomSessionAwareIntradayForward("US.AAPL", types.Interval2h, start, 2); err != nil || len(rows) != 0 {
			t.Fatalf("empty session forward rows=%#v err=%v", rows, err)
		}
		if rows, err := empty.queryCustomSessionAwareIntradayBackward("US.AAPL", types.Interval2h, start.Add(6*time.Hour), 2); err != nil || len(rows) != 0 {
			t.Fatalf("empty session backward rows=%#v err=%v", rows, err)
		}
	})

	t.Run("emits equal-timestamp bars deterministically by interval then symbol", func(t *testing.T) {
		end := start.Add(2 * time.Hour)
		rows := []types.KLine{
			testBacktestKLine("US.ZZZ", types.Interval2h, end.Add(-2*time.Hour+time.Millisecond), 2*time.Hour, 1),
			testBacktestKLine("US.BBB", types.Interval1h, end.Add(-time.Hour+time.Millisecond), time.Hour, 2),
			testBacktestKLine("US.AAA", types.Interval1h, end.Add(-time.Hour+time.Millisecond), time.Hour, 3),
		}
		for index := range rows {
			if !rows[index].EndTime.Time().Equal(end) {
				t.Fatalf("fixture end time[%d]=%v, want %v", index, rows[index].EndTime.Time(), end)
			}
		}
		sortKLinesForEmission(rows)
		if rows[0].Symbol != "US.AAA" || rows[1].Symbol != "US.BBB" || rows[2].Symbol != "US.ZZZ" {
			t.Fatalf("deterministic order=%#v", rows)
		}
		store := &sessionFilteredBacktestStore{base: &stubBacktestStore{}, includeExtendedHours: true}
		if store.needsCustomHandling([]string{"HK.00700"}, []types.Interval{types.Interval1d}) {
			t.Fatal("non-US extended-hours request should not require custom aggregation")
		}
	})
}
