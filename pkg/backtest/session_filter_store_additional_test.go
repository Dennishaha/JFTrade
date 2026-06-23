package backtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/service"
	"github.com/c9s/bbgo/pkg/types"
)

type stubBacktestStore struct {
	verifyCalls int
	verifyErr   error

	syncCalls int
	syncErr   error

	queryKLineCalls int
	queryKLineErr   error

	forwardBatches [][]types.KLine
	forwardCalls   int

	backwardBatches [][]types.KLine
	backwardCalls   int

	queryChRows []types.KLine
	queryChErr  error
}

func (s *stubBacktestStore) Verify(types.Exchange, []string, time.Time, time.Time) error {
	s.verifyCalls++
	return s.verifyErr
}

func (s *stubBacktestStore) Sync(context.Context, types.Exchange, string, []types.Interval, time.Time, time.Time) error {
	s.syncCalls++
	return s.syncErr
}

func (s *stubBacktestStore) QueryKLine(types.Exchange, string, types.Interval, string, int) (*types.KLine, error) {
	s.queryKLineCalls++
	return nil, s.queryKLineErr
}

func (s *stubBacktestStore) QueryKLinesForward(types.Exchange, string, types.Interval, time.Time, int) ([]types.KLine, error) {
	s.forwardCalls++
	if len(s.forwardBatches) == 0 {
		return nil, nil
	}
	batch := s.forwardBatches[0]
	s.forwardBatches = s.forwardBatches[1:]
	return batch, nil
}

func (s *stubBacktestStore) QueryKLinesBackward(types.Exchange, string, types.Interval, time.Time, int) ([]types.KLine, error) {
	s.backwardCalls++
	if len(s.backwardBatches) == 0 {
		return nil, nil
	}
	batch := s.backwardBatches[0]
	s.backwardBatches = s.backwardBatches[1:]
	return batch, nil
}

func (s *stubBacktestStore) QueryKLinesCh(time.Time, time.Time, types.Exchange, []string, []types.Interval) (chan types.KLine, chan error) {
	ch := make(chan types.KLine, len(s.queryChRows))
	errCh := make(chan error, 1)
	for _, row := range s.queryChRows {
		ch <- row
	}
	close(ch)
	if s.queryChErr != nil {
		errCh <- s.queryChErr
	}
	close(errCh)
	return ch, errCh
}

type stubRangeBacktestStore struct {
	*stubBacktestStore
	tradingRangeCalls int
	sessionRangeCalls int
	tradingRows       []types.KLine
	sessionRows       []types.KLine
}

type stubStreamerRangeBacktestStore struct {
	*stubRangeBacktestStore
	streamCalls int
	streamRows  []types.KLine
	streamErr   error
}

func (s *stubRangeBacktestStore) QueryTradingPeriodKLinesInRange(string, types.Interval, time.Time, time.Time, bool) ([]types.KLine, error) {
	s.tradingRangeCalls++
	return append([]types.KLine(nil), s.tradingRows...), nil
}

func (s *stubRangeBacktestStore) QuerySessionAwareIntradayKLinesInRange(string, types.Interval, time.Time, time.Time, bool) ([]types.KLine, error) {
	s.sessionRangeCalls++
	return append([]types.KLine(nil), s.sessionRows...), nil
}

func (s *stubStreamerRangeBacktestStore) StreamKLines(_ time.Time, _ time.Time, _ types.Exchange, _ []string, _ []types.Interval, emit func(types.KLine)) error {
	s.streamCalls++
	if s.streamErr != nil {
		return s.streamErr
	}
	for _, row := range s.streamRows {
		emit(row)
	}
	return nil
}

var _ service.BackTestable = (*stubBacktestStore)(nil)

func TestSessionFilteredStoreDelegatesVerifyAndSync(t *testing.T) {
	base := &stubBacktestStore{}
	if replay := newBacktestReplayStore(base, nil); replay != base {
		t.Fatalf("newBacktestReplayStore(nil) = %T, want base", replay)
	}

	includeExtendedHours := false
	wrapped, ok := newBacktestReplayStore(base, &includeExtendedHours).(*sessionFilteredBacktestStore)
	if !ok {
		t.Fatalf("wrapped store type = %T", newBacktestReplayStore(base, &includeExtendedHours))
	}

	if err := wrapped.Verify(nil, []string{"US.AAPL"}, time.Unix(1, 0), time.Unix(2, 0)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if err := wrapped.Sync(context.Background(), nil, "US.AAPL", []types.Interval{types.Interval1m}, time.Unix(3, 0), time.Unix(4, 0)); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if base.verifyCalls != 1 || base.syncCalls != 1 {
		t.Fatalf("delegate calls = verify %d sync %d", base.verifyCalls, base.syncCalls)
	}
}

func TestSessionFilteredStoreQueryHelpersFilterUSRegularHours(t *testing.T) {
	baseTime := time.Date(2026, time.June, 12, 0, 0, 0, 0, time.UTC)
	pre := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(12*time.Hour), time.Minute, 1)
	regular1 := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+30*time.Minute), time.Minute, 2)
	regular2 := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+31*time.Minute), time.Minute, 3)
	after := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(20*time.Hour), time.Minute, 4)

	t.Run("query kline asc uses filtered forward path", func(t *testing.T) {
		base := &stubBacktestStore{forwardBatches: [][]types.KLine{{pre, regular1, after}}}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}
		kline, err := store.QueryKLine(nil, "US.AAPL", types.Interval1m, "ASC", 1)
		if err != nil {
			t.Fatalf("QueryKLine() error = %v", err)
		}
		if kline == nil || !kline.StartTime.Time().Equal(regular1.StartTime.Time()) {
			t.Fatalf("QueryKLine() = %#v, want regular row", kline)
		}
		if base.queryKLineCalls != 0 || base.forwardCalls != 1 {
			t.Fatalf("base calls = queryKLine %d forward %d", base.queryKLineCalls, base.forwardCalls)
		}
	})

	t.Run("forward pagination skips pre and after hours", func(t *testing.T) {
		fullPage := make([]types.KLine, 0, 64)
		fullPage = append(fullPage, pre, regular1)
		for range 62 {
			fullPage = append(fullPage, after)
		}
		base := &stubBacktestStore{forwardBatches: [][]types.KLine{fullPage, {regular2}}}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}
		rows, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1m, pre.StartTime.Time(), 2)
		if err != nil {
			t.Fatalf("QueryKLinesForward() error = %v", err)
		}
		if len(rows) != 2 || !rows[0].StartTime.Time().Equal(regular1.StartTime.Time()) || !rows[1].StartTime.Time().Equal(regular2.StartTime.Time()) {
			t.Fatalf("QueryKLinesForward() = %#v", rows)
		}
		if base.forwardCalls != 2 {
			t.Fatalf("forwardCalls = %d, want 2", base.forwardCalls)
		}
	})

	t.Run("query kline desc uses filtered backward path", func(t *testing.T) {
		base := &stubBacktestStore{backwardBatches: [][]types.KLine{{after, regular2, pre}}}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}
		kline, err := store.QueryKLine(nil, "US.AAPL", types.Interval1m, "DESC", 1)
		if err != nil {
			t.Fatalf("QueryKLine(desc) error = %v", err)
		}
		if kline == nil || !kline.StartTime.Time().Equal(regular2.StartTime.Time()) {
			t.Fatalf("QueryKLine(desc) = %#v, want latest regular row", kline)
		}
		if base.backwardCalls != 1 {
			t.Fatalf("backwardCalls = %d, want 1", base.backwardCalls)
		}
	})
}

func TestSessionFilteredStoreStreamKLinesFallsBackToChannels(t *testing.T) {
	baseTime := time.Date(2026, time.June, 12, 0, 0, 0, 0, time.UTC)
	pre := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(12*time.Hour), time.Minute, 1)
	regular := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+30*time.Minute), time.Minute, 2)
	hk := testBacktestKLine("HK.00700", types.Interval1m, baseTime.Add(2*time.Hour), time.Minute, 3)

	base := &stubBacktestStore{queryChRows: []types.KLine{pre, regular, hk}}
	store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}
	var got []types.KLine
	if err := store.StreamKLines(baseTime, baseTime.Add(24*time.Hour), nil, []string{"US.AAPL", "HK.00700"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
		got = append(got, kline)
	}); err != nil {
		t.Fatalf("StreamKLines() error = %v", err)
	}
	if len(got) != 2 || !got[0].StartTime.Time().Equal(regular.StartTime.Time()) || got[1].Symbol != "HK.00700" {
		t.Fatalf("StreamKLines() rows = %#v", got)
	}

	expectedErr := errors.New("boom")
	ch := make(chan types.KLine, 1)
	errCh := make(chan error, 1)
	ch <- regular
	close(ch)
	errCh <- expectedErr
	close(errCh)
	if err := emitKLinesFromStoreChannels(ch, errCh, func(types.KLine) {}); !errors.Is(err, expectedErr) {
		t.Fatalf("emitKLinesFromStoreChannels() error = %v", err)
	}
}

func TestSessionFilteredStoreUsesCustomExtendedHoursRangeQueries(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	daily1 := testBacktestKLine("US.AAPL", types.Interval1d, start, 24*time.Hour, 10)
	daily2 := testBacktestKLine("US.AAPL", types.Interval1d, start.Add(24*time.Hour), 24*time.Hour, 11)
	intraday1 := testBacktestKLine("US.AAPL", types.Interval2h, start, 2*time.Hour, 20)
	intraday2 := testBacktestKLine("US.AAPL", types.Interval2h, start.Add(2*time.Hour), 2*time.Hour, 21)

	base := &stubRangeBacktestStore{
		stubBacktestStore: &stubBacktestStore{},
		tradingRows:       []types.KLine{daily1, daily2},
		sessionRows:       []types.KLine{intraday1, intraday2},
	}
	store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: true}

	dailyRows, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval1d, daily1.StartTime.Time(), 1)
	if err != nil {
		t.Fatalf("QueryKLinesForward(daily) error = %v", err)
	}
	if len(dailyRows) != 1 || !dailyRows[0].StartTime.Time().Equal(daily1.StartTime.Time()) {
		t.Fatalf("daily rows = %#v", dailyRows)
	}

	intradayRows, err := store.QueryKLinesForward(nil, "US.AAPL", types.Interval2h, intraday1.StartTime.Time(), 2)
	if err != nil {
		t.Fatalf("QueryKLinesForward(2h) error = %v", err)
	}
	if len(intradayRows) != 2 || !intradayRows[0].StartTime.Time().Equal(intraday1.StartTime.Time()) || !intradayRows[1].StartTime.Time().Equal(intraday2.StartTime.Time()) {
		t.Fatalf("intraday rows = %#v", intradayRows)
	}
	if base.tradingRangeCalls == 0 || base.sessionRangeCalls == 0 {
		t.Fatalf("range calls = trading %d session %d", base.tradingRangeCalls, base.sessionRangeCalls)
	}
}

func TestSessionFilteredStoreQueryKLinesChIncludesCustomExtendedHoursRows(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	us1m := testBacktestKLine("US.AAPL", types.Interval1m, start, time.Minute, 10)
	hk1m := testBacktestKLine("HK.00700", types.Interval1m, start.Add(48*time.Hour), time.Minute, 11)
	baseDaily := testBacktestKLine("US.AAPL", types.Interval1d, start, 24*time.Hour, 12)
	base2h := testBacktestKLine("US.AAPL", types.Interval2h, start, 2*time.Hour, 13)
	custom2h := testBacktestKLine("US.AAPL", types.Interval2h, start, 2*time.Hour, 20)
	customDaily := testBacktestKLine("US.AAPL", types.Interval1d, start, 24*time.Hour, 21)

	base := &stubRangeBacktestStore{
		stubBacktestStore: &stubBacktestStore{queryChRows: []types.KLine{baseDaily, us1m, hk1m, base2h}},
		tradingRows:       []types.KLine{customDaily},
		sessionRows:       []types.KLine{custom2h},
	}
	store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: true}

	ch, errCh := store.QueryKLinesCh(start, start.Add(24*time.Hour), nil, []string{"US.AAPL", "HK.00700"}, []types.Interval{types.Interval1m, types.Interval2h, types.Interval1d})
	rows, err := collectKLinesFromStoreChannels(ch, errCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh() error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("QueryKLinesCh() len = %d, rows=%#v", len(rows), rows)
	}
	if !rows[0].StartTime.Time().Equal(us1m.StartTime.Time()) || rows[0].Interval != types.Interval1m {
		t.Fatalf("rows[0] = %#v, want US 1m base row", rows[0])
	}
	if !rows[1].StartTime.Time().Equal(custom2h.StartTime.Time()) || rows[1].Interval != types.Interval2h {
		t.Fatalf("rows[1] = %#v, want US 2h custom row", rows[1])
	}
	if !rows[2].StartTime.Time().Equal(customDaily.StartTime.Time()) || rows[2].Interval != types.Interval1d {
		t.Fatalf("rows[2] = %#v, want US 1d custom row", rows[2])
	}
	if !rows[3].StartTime.Time().Equal(hk1m.StartTime.Time()) || rows[3].Symbol != "HK.00700" {
		t.Fatalf("rows[3] = %#v, want HK base row", rows[3])
	}
	if base.tradingRangeCalls != 1 || base.sessionRangeCalls != 1 {
		t.Fatalf("range calls = trading %d session %d", base.tradingRangeCalls, base.sessionRangeCalls)
	}
}

func TestSessionFilteredStoreStreamKLinesUsesStreamerAndCustomExtendedHoursRows(t *testing.T) {
	start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
	pre := testBacktestKLine("US.AAPL", types.Interval1m, start.Add(-90*time.Minute), time.Minute, 1)
	regular := testBacktestKLine("US.AAPL", types.Interval1m, start, time.Minute, 2)
	hk := testBacktestKLine("HK.00700", types.Interval1m, start.Add(48*time.Hour), time.Minute, 3)
	baseDaily := testBacktestKLine("US.AAPL", types.Interval1d, start, 24*time.Hour, 4)
	base2h := testBacktestKLine("US.AAPL", types.Interval2h, start, 2*time.Hour, 5)
	custom2h := testBacktestKLine("US.AAPL", types.Interval2h, start, 2*time.Hour, 6)
	customDaily := testBacktestKLine("US.AAPL", types.Interval1d, start, 24*time.Hour, 7)

	t.Run("streamer branch filters premarket when extended hours are excluded", func(t *testing.T) {
		base := &stubStreamerRangeBacktestStore{
			stubRangeBacktestStore: &stubRangeBacktestStore{stubBacktestStore: &stubBacktestStore{}},
			streamRows:             []types.KLine{pre, regular, hk},
		}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: false}

		var rows []types.KLine
		if err := store.StreamKLines(start.Add(-2*time.Hour), start.Add(2*time.Hour), nil, []string{"US.AAPL", "HK.00700"}, []types.Interval{types.Interval1m}, func(kline types.KLine) {
			rows = append(rows, kline)
		}); err != nil {
			t.Fatalf("StreamKLines(filtering) error = %v", err)
		}
		if base.streamCalls != 1 {
			t.Fatalf("streamCalls = %d, want 1", base.streamCalls)
		}
		if len(rows) != 2 || !rows[0].StartTime.Time().Equal(regular.StartTime.Time()) || rows[1].Symbol != "HK.00700" {
			t.Fatalf("filtered rows = %#v", rows)
		}
	})

	t.Run("include extended hours merges custom rows after collecting base stream", func(t *testing.T) {
		base := &stubStreamerRangeBacktestStore{
			stubRangeBacktestStore: &stubRangeBacktestStore{
				stubBacktestStore: &stubBacktestStore{},
				tradingRows:       []types.KLine{customDaily},
				sessionRows:       []types.KLine{custom2h},
			},
			streamRows: []types.KLine{baseDaily, regular, hk, base2h},
		}
		store := &sessionFilteredBacktestStore{base: base, includeExtendedHours: true}

		var rows []types.KLine
		if err := store.StreamKLines(start, start.Add(24*time.Hour), nil, []string{"US.AAPL", "HK.00700"}, []types.Interval{types.Interval1m, types.Interval2h, types.Interval1d}, func(kline types.KLine) {
			rows = append(rows, kline)
		}); err != nil {
			t.Fatalf("StreamKLines(custom) error = %v", err)
		}
		if base.streamCalls != 1 {
			t.Fatalf("streamCalls = %d, want 1", base.streamCalls)
		}
		if base.tradingRangeCalls != 1 || base.sessionRangeCalls != 1 {
			t.Fatalf("range calls = trading %d session %d", base.tradingRangeCalls, base.sessionRangeCalls)
		}
		if len(rows) != 4 {
			t.Fatalf("rows len = %d, rows=%#v", len(rows), rows)
		}
		if !rows[0].StartTime.Time().Equal(regular.StartTime.Time()) || rows[0].Interval != types.Interval1m {
			t.Fatalf("rows[0] = %#v, want US 1m base row", rows[0])
		}
		if !rows[1].StartTime.Time().Equal(custom2h.StartTime.Time()) || rows[1].Interval != types.Interval2h {
			t.Fatalf("rows[1] = %#v, want US 2h custom row", rows[1])
		}
		if !rows[2].StartTime.Time().Equal(customDaily.StartTime.Time()) || rows[2].Interval != types.Interval1d {
			t.Fatalf("rows[2] = %#v, want US 1d custom row", rows[2])
		}
		if !rows[3].StartTime.Time().Equal(hk.StartTime.Time()) || rows[3].Symbol != "HK.00700" {
			t.Fatalf("rows[3] = %#v, want HK base row", rows[3])
		}
	})
}

func TestSessionFilteredStoreHelperFunctions(t *testing.T) {
	baseTime := time.Date(2026, time.June, 12, 0, 0, 0, 0, time.UTC)

	t.Run("collectBaseKLinesForStream prefers streamer and propagates streamer errors", func(t *testing.T) {
		streamRows := []types.KLine{
			testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+30*time.Minute), time.Minute, 1),
			testBacktestKLine("HK.00700", types.Interval1m, baseTime.Add(2*time.Hour), time.Minute, 2),
		}
		base := &stubStreamerRangeBacktestStore{
			stubRangeBacktestStore: &stubRangeBacktestStore{stubBacktestStore: &stubBacktestStore{}},
			streamRows:             streamRows,
		}
		rows, err := collectBaseKLinesForStream(base, baseTime, baseTime.Add(24*time.Hour), nil, []string{"US.AAPL", "HK.00700"}, []types.Interval{types.Interval1m})
		if err != nil {
			t.Fatalf("collectBaseKLinesForStream(streamer) error = %v", err)
		}
		if base.streamCalls != 1 {
			t.Fatalf("streamCalls = %d, want 1", base.streamCalls)
		}
		if len(rows) != 2 || !rows[0].StartTime.Time().Equal(streamRows[0].StartTime.Time()) || rows[1].Symbol != "HK.00700" {
			t.Fatalf("collectBaseKLinesForStream(streamer) = %#v", rows)
		}

		expectedErr := errors.New("stream failed")
		base.streamErr = expectedErr
		if _, err := collectBaseKLinesForStream(base, baseTime, baseTime.Add(24*time.Hour), nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m}); !errors.Is(err, expectedErr) {
			t.Fatalf("collectBaseKLinesForStream(stream error) = %v", err)
		}
	})

	t.Run("collectBaseKLinesForStream falls back to channel query", func(t *testing.T) {
		rowsInput := []types.KLine{
			testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+30*time.Minute), time.Minute, 1),
			testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+31*time.Minute), time.Minute, 2),
		}
		base := &stubBacktestStore{queryChRows: rowsInput}
		rows, err := collectBaseKLinesForStream(base, baseTime, baseTime.Add(24*time.Hour), nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m})
		if err != nil {
			t.Fatalf("collectBaseKLinesForStream(query channel) error = %v", err)
		}
		if len(rows) != 2 || !rows[0].StartTime.Time().Equal(rowsInput[0].StartTime.Time()) || !rows[1].StartTime.Time().Equal(rowsInput[1].StartTime.Time()) {
			t.Fatalf("collectBaseKLinesForStream(query channel) = %#v", rows)
		}

		expectedErr := errors.New("query channel failed")
		base.queryChErr = expectedErr
		if _, err := collectBaseKLinesForStream(base, baseTime, baseTime.Add(24*time.Hour), nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m}); !errors.Is(err, expectedErr) {
			t.Fatalf("collectBaseKLinesForStream(query error) = %v", err)
		}
	})

	t.Run("keepRegularSessionKLine allows non US bars and US bars touching regular session", func(t *testing.T) {
		if !keepRegularSessionKLine(testBacktestKLine("HK.00700", types.Interval1m, baseTime.Add(2*time.Hour), time.Minute, 1)) {
			t.Fatal("keepRegularSessionKLine(HK) = false, want true")
		}

		usPremarket := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(12*time.Hour), time.Minute, 1)
		if keepRegularSessionKLine(usPremarket) {
			t.Fatal("keepRegularSessionKLine(US premarket) = true, want false")
		}

		usRegularStart := testBacktestKLine("US.AAPL", types.Interval1m, baseTime.Add(13*time.Hour+30*time.Minute), time.Minute, 1)
		if !keepRegularSessionKLine(usRegularStart) {
			t.Fatal("keepRegularSessionKLine(US regular) = false, want true")
		}
	})

	t.Run("normalizeKLineLimit and filteredPageSize clamp and scale as intended", func(t *testing.T) {
		if got := normalizeKLineLimit(0); got != 1 {
			t.Fatalf("normalizeKLineLimit(0) = %d, want 1", got)
		}
		if got := normalizeKLineLimit(7); got != 7 {
			t.Fatalf("normalizeKLineLimit(7) = %d, want 7", got)
		}

		tests := []struct {
			limit int
			want  int
		}{
			{limit: 1, want: 64},
			{limit: 16, want: 64},
			{limit: 17, want: 68},
			{limit: 64, want: 256},
			{limit: 65, want: 130},
			{limit: 256, want: 512},
			{limit: 300, want: 300},
		}
		for _, tc := range tests {
			if got := filteredPageSize(tc.limit); got != tc.want {
				t.Fatalf("filteredPageSize(%d) = %d, want %d", tc.limit, got, tc.want)
			}
		}
	})
}

func testBacktestKLine(symbol string, interval types.Interval, start time.Time, duration time.Duration, closeValue float64) types.KLine {
	return types.KLine{
		Symbol:    symbol,
		Interval:  interval,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(duration - time.Millisecond)),
		Open:      fixedpoint.NewFromFloat(closeValue),
		High:      fixedpoint.NewFromFloat(closeValue),
		Low:       fixedpoint.NewFromFloat(closeValue),
		Close:     fixedpoint.NewFromFloat(closeValue),
		Volume:    fixedpoint.NewFromFloat(100),
	}
}
