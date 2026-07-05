package backtest

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

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
