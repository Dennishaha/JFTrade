package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestBrokerKLinesReturnLatestPageAndUseExclusiveBeforeCursor(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
	server.setHistorySeries([]*qotcommonpb.KLine{
		testHistoryKLine(base.Add(time.Minute), 101),
		testHistoryKLine(base.Add(2*time.Minute), 102),
		testHistoryKLine(base.Add(3*time.Minute), 103),
	})
	reader := newTestBrokerAdapter(t, server).MarketData()

	latest, err := reader.QueryKLines(t.Context(), broker.KLineQuery{
		Symbol: "HK.00700",
		Period: "1m",
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("latest QueryKLines: %v", err)
	}
	if len(latest.KLines) != 2 || !latest.Pagination.HasMore {
		t.Fatalf("latest page = %#v", latest)
	}
	if latest.KLines[0].Close == nil || *latest.KLines[0].Close != 102.5 ||
		latest.KLines[1].Close == nil || *latest.KLines[1].Close != 103.5 {
		t.Fatalf("latest candles = %#v", latest.KLines)
	}
	if latest.Pagination.NextBefore != latest.KLines[0].Time {
		t.Fatalf("nextBefore = %q, first = %q", latest.Pagination.NextBefore, latest.KLines[0].Time)
	}

	older, err := reader.QueryKLines(t.Context(), broker.KLineQuery{
		Symbol:     "HK.00700",
		Period:     "1m",
		BeforeTime: latest.Pagination.NextBefore,
		Limit:      2,
	})
	if err != nil {
		t.Fatalf("older QueryKLines: %v", err)
	}
	if len(older.KLines) != 1 || older.Pagination.HasMore {
		t.Fatalf("older page = %#v", older)
	}
	if older.KLines[0].Close == nil || *older.KLines[0].Close != 101.5 {
		t.Fatalf("older candles = %#v", older.KLines)
	}
	if older.KLines[0].Time >= latest.Pagination.NextBefore {
		t.Fatalf("before cursor was not exclusive: older=%q before=%q", older.KLines[0].Time, latest.Pagination.NextBefore)
	}
}

func TestFutuDeclaredCandlePeriodsMapToHistoricalAndRealtimeTypes(t *testing.T) {
	for _, period := range futuCandlePeriods() {
		interval, err := futuIntervalFromPeriod(period)
		if err != nil {
			t.Fatalf("futuIntervalFromPeriod(%q): %v", period, err)
		}
		if _, err := futuKLTypeFromInterval(interval); err != nil {
			t.Errorf("historical mapping for %q: %v", period, err)
		}
		if _, err := futuSubTypeFromInterval(interval); err != nil {
			t.Errorf("realtime mapping for %q: %v", period, err)
		}
	}

	for _, capability := range futuFeatureCapabilities("HK") {
		if capability.ID != broker.FeatureMarketCandles {
			continue
		}
		if len(capability.SupportedPeriods) != len(futuCandlePeriods()) {
			t.Fatalf("declared periods = %#v", capability.SupportedPeriods)
		}
		return
	}
	t.Fatal("market.candles capability was not declared")
}

func TestBrokerKLineQueryFormatsOpenDWindowInMarketTimeAndReturnsUTC(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 0, 1, 0, 0, time.UTC)
	server.setHistorySeries([]*qotcommonpb.KLine{
		testHistoryKLine(labelAt, 100),
	})
	reader := newTestBrokerAdapter(t, server).MarketData()

	snapshot, err := reader.QueryKLines(t.Context(), broker.KLineQuery{
		Symbol:   "HK.00700",
		Period:   "1m",
		FromTime: "2026-05-20T00:00:00Z",
		ToTime:   "2026-05-20T01:00:00Z",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	beginTime, endTime := server.lastHistoryWindow()
	if beginTime != "2026-05-20 08:00:00" || endTime != "2026-05-20 09:00:00" {
		t.Fatalf("OpenD window = %q..%q, want Hong Kong local time", beginTime, endTime)
	}
	if len(snapshot.KLines) != 1 || snapshot.KLines[0].Time != "2026-05-20T00:00:00Z" {
		t.Fatalf("UTC candles = %#v", snapshot.KLines)
	}
}

func TestNormalizeBrokerKLinePageDeduplicatesSortsAndKeepsLatest(t *testing.T) {
	base := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	makeKLine := func(at time.Time, closePrice int64) bbgotypes.KLine {
		return bbgotypes.KLine{
			StartTime: bbgotypes.Time(at),
			Close:     fixedpoint.NewFromInt(closePrice),
		}
	}
	page := normalizeBrokerKLinePage(
		[]bbgotypes.KLine{
			makeKLine(base.Add(2*time.Minute), 102),
			makeKLine(base, 100),
			makeKLine(base.Add(time.Minute), 101),
			makeKLine(base.Add(time.Minute), 201),
		},
		base.Add(-time.Minute),
		base.Add(3*time.Minute),
		2,
		true,
	)
	if len(page) != 2 ||
		!page[0].StartTime.Time().Equal(base.Add(time.Minute)) ||
		!page[1].StartTime.Time().Equal(base.Add(2*time.Minute)) {
		t.Fatalf("normalized page = %#v", page)
	}
	if got := page[0].Close.Int64(); got != 201 {
		t.Fatalf("duplicate boundary close = %d, want latest value 201", got)
	}

	earliest := normalizeBrokerKLinePage(
		[]bbgotypes.KLine{
			makeKLine(base.Add(2*time.Minute), 102),
			makeKLine(base, 100),
			makeKLine(base.Add(time.Minute), 101),
		},
		base.Add(-time.Minute),
		base.Add(3*time.Minute),
		2,
		false,
	)
	if len(earliest) != 2 || !earliest[0].StartTime.Time().Equal(base) ||
		!earliest[1].StartTime.Time().Equal(base.Add(time.Minute)) {
		t.Fatalf("earliest normalized page = %#v", earliest)
	}
}

func TestBrokerKLineQueryRejectsCursorAndTimeBoundaryErrors(t *testing.T) {
	_, reader := coverageMarginMarketDataReader(t)
	tests := []struct {
		name  string
		query broker.KLineQuery
		want  string
	}{
		{
			name: "cursor with range",
			query: broker.KLineQuery{
				Symbol: "HK.00700", Period: "5m",
				BeforeTime: "2026-07-18T13:40:00Z", FromTime: "2026-07-01",
			},
			want: "beforeTime cannot be combined",
		},
		{
			name: "invalid cursor and capped limit with extended hours",
			query: broker.KLineQuery{
				Symbol: "US.AAPL", Period: "1m", Limit: 501, BeforeTime: "bad",
			},
			want: "invalid beforeTime",
		},
		{
			name: "invalid from",
			query: broker.KLineQuery{
				Symbol: "HK.00700", Period: "5m", FromTime: "bad",
			},
			want: "invalid fromTime",
		},
		{
			name: "invalid to",
			query: broker.KLineQuery{
				Symbol: "HK.00700", Period: "5m",
				FromTime: "2026-07-01T00:00:00Z", ToTime: "bad",
			},
			want: "invalid toTime",
		},
		{
			name: "reversed range",
			query: broker.KLineQuery{
				Symbol: "HK.00700", Period: "5m",
				FromTime: "2026-07-02T00:00:00Z", ToTime: "2026-07-01T00:00:00Z",
			},
			want: "fromTime must be before toTime",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reader.QueryKLines(t.Context(), test.query); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("QueryKLines() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBrokerKLinePaginationHelpersCoverSessionsBoundsAndListingDates(t *testing.T) {
	server, reader := coverageMarginMarketDataReader(t)
	base := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

	sessionKLine := coverageMarginKLine("US.AAPL", base, 5*time.Minute)
	reader.exchange.RegisterKLineSession(sessionKLine, market.SessionAfter)
	sessionSnapshot := reader.buildBrokerKLineSnapshot(
		broker.KLineQuery{Symbol: " us.aapl "},
		bbgotypes.Interval5m,
		[]bbgotypes.KLine{sessionKLine},
		false,
		true,
		"all",
	)
	if len(sessionSnapshot.KLines) != 1 || sessionSnapshot.KLines[0].Session != string(market.SessionAfter) {
		t.Fatalf("extended-hours session snapshot = %#v", sessionSnapshot)
	}

	empty, hasMore, err := reader.queryAdaptiveKLinePage(
		t.Context(), "HK.00700", bbgotypes.Interval1d, base, base, 10,
	)
	if err != nil || hasMore || len(empty) != 0 {
		t.Fatalf("empty lower-bound page = %#v, %t, %v", empty, hasMore, err)
	}

	server.setHistorySeries(nil)
	empty, hasMore, err = reader.queryAdaptiveKLinePage(
		t.Context(),
		"HK.00700",
		bbgotypes.Interval1mo,
		time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		1000,
	)
	if err != nil || hasMore {
		t.Fatalf("wide empty page = %#v, %t, %v", empty, hasMore, err)
	}

	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})
	if got := reader.klineListingLowerBound(t.Context(), "HK.00700", time.UTC); got.Unix() != 1087324800 {
		t.Fatalf("timestamp listing lower bound = %s", got)
	}
	listTimeInfo := testTencentStaticInfo()
	listTimeInfo.Basic.ListTimestamp = nil
	listTimeInfo.Basic.ListTime = new("2004-06-16")
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{listTimeInfo})
	if got := reader.klineListingLowerBound(t.Context(), "HK.00700", time.UTC); got.Format(time.DateOnly) != "2004-06-16" {
		t.Fatalf("date listing lower bound = %s", got)
	}
	invalidListTimeInfo := testTencentStaticInfo()
	invalidListTimeInfo.Basic.ListTimestamp = nil
	invalidListTimeInfo.Basic.ListTime = new("not-a-date")
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{invalidListTimeInfo})
	if got := reader.klineListingLowerBound(t.Context(), "HK.00700", time.UTC); got.Year() != 1900 {
		t.Fatalf("fallback listing lower bound = %s", got)
	}
}

func TestFutuCandlePeriodCatalogSkipsMissingAndUnmappableIntervals(t *testing.T) {
	original := futuCandleIntervalByPeriod["1m"]
	t.Cleanup(func() {
		futuCandleIntervalByPeriod["1m"] = original
	})

	delete(futuCandleIntervalByPeriod, "1m")
	if _, err := futuIntervalFromPeriod("1m"); err == nil {
		t.Fatal("missing 1m interval mapping was accepted")
	}
	_ = futuCandlePeriods()

	futuCandleIntervalByPeriod["1m"] = bbgotypes.Interval("unsupported")
	if _, err := futuIntervalFromPeriod("1m"); err == nil {
		t.Fatal("unmappable 1m interval was accepted")
	}
	_ = futuCandlePeriods()
}
