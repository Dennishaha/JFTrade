package futu

import (
	"context"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestQueryTickersBatchesBasicQotRequests(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()
	for _, symbol := range []string{"HK.00700", "US.NVDA"} {
		if err := ex.SubscribeBasicQuote(t.Context(), symbol, false); err != nil {
			t.Fatalf("SubscribeBasicQuote(%s): %v", symbol, err)
		}
	}

	tickers, err := ex.QueryTickers(t.Context(), "HK.00700", "US.NVDA")
	if err != nil {
		t.Fatalf("QueryTickers: %v", err)
	}
	if len(tickers) != 2 {
		t.Fatalf("expected 2 batched tickers, got %d", len(tickers))
	}
	if got := server.acceptCount(); got != 1 {
		t.Fatalf("expected one OpenD TCP session, got %d", got)
	}
	if got := server.subCallCount(); got != 2 {
		t.Fatalf("explicit leases should create two Qot_Sub calls, got %d", got)
	}
	if got := server.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one batched GetBasicQot call, got %d", got)
	}
	if _, ok := tickers["US.NVDA"]; !ok {
		t.Fatalf("expected batched quote for US.NVDA, got %#v", tickers)
	}
}

func TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setHistoryPagesBySession(map[int32][][]*qotcommonpb.KLine{
		int32(commonpb.Session_Session_RTH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110)},
		},
		int32(commonpb.Session_Session_ETH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 100)},
		},
		int32(commonpb.Session_Session_ALL): {
			{
				testHistoryKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90),
				testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 95),
				testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 105),
			},
		},
	})
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	start := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	klines, err := ex.QueryKLines(t.Context(), "US.NVDA", types.Interval1m, types.KLineQueryOptions{Limit: 3, StartTime: &start, EndTime: new(start.Add(2 * time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("expected three merged session klines, got %d", len(klines))
	}
	if got := server.historyKLCallCount(); got != 3 {
		t.Fatalf("expected three RequestHistoryKL calls, got %d", got)
	}
	if !server.lastHistoryExtendedTime() {
		t.Fatal("expected US intraday RequestHistoryKL to set extendedTime=true")
	}
	if got := server.historySessionCalls(); len(got) != 3 || got[0] != int32(commonpb.Session_Session_RTH) || got[1] != int32(commonpb.Session_Session_ETH) || got[2] != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("expected RTH/ETH/ALL route calls, got %#v", got)
	}
	if got := klines[1].Open.Float64(); got != 100 {
		t.Fatalf("expected ETH route candle to win over ALL duplicate, got %v", got)
	}
	if got := klines[2].Open.Float64(); got != 110 {
		t.Fatalf("expected RTH route candle to win over ALL duplicate, got %v", got)
	}
	if session, ok := ex.ResolveKLineSession(klines[0]); !ok || session != market.SessionOvernight {
		t.Fatalf("expected overnight session tag, got %s ok=%v", session, ok)
	}
	if session, ok := ex.ResolveKLineSession(klines[1]); !ok || session != market.SessionPre {
		t.Fatalf("expected ETH route to resolve pre session, got %s ok=%v", session, ok)
	}
	if session, ok := ex.ResolveKLineSession(klines[2]); !ok || session != market.SessionRegular {
		t.Fatalf("expected RTH route to resolve regular session, got %s ok=%v", session, ok)
	}
}

func TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight(t *testing.T) {
	preClockKLine := types.KLine{
		Symbol:    "US.AAPL",
		StartTime: types.Time(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.May, 20, 10, 0, 59, 0, time.UTC)),
	}
	if session := resolveHistoricalMarketSession(commonpb.Session_Session_RTH, "US.AAPL", preClockKLine); session != market.SessionRegular {
		t.Fatalf("expected RTH route to force regular session, got %s", session)
	}
	if session := resolveHistoricalMarketSession(commonpb.Session_Session_OVERNIGHT, "US.AAPL", preClockKLine); session != market.SessionOvernight {
		t.Fatalf("expected overnight route to force overnight session, got %s", session)
	}
}

func TestQueryKLinesFallsBackToSessionAllWhenHistoricalRouteUnsupported(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setHistoryPagesBySession(map[int32][][]*qotcommonpb.KLine{
		int32(commonpb.Session_Session_RTH): {
			{testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110)},
		},
		int32(commonpb.Session_Session_ALL): {
			{
				testHistoryKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90),
				testHistoryKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 100),
				testHistoryKLine(time.Date(2026, time.May, 20, 15, 30, 0, 0, time.UTC), 110),
			},
		},
	})
	server.setHistorySessionError(int32(commonpb.Session_Session_ETH), 1, "session is invalid")
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	start := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	klines, err := ex.QueryKLines(t.Context(), "US.NVDA", types.Interval1m, types.KLineQueryOptions{Limit: 3, StartTime: &start, EndTime: new(start.Add(2 * time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("expected fallback Session_ALL history to return three klines, got %d", len(klines))
	}
	if got := server.historySessionCalls(); len(got) != 3 || got[0] != int32(commonpb.Session_Session_RTH) || got[1] != int32(commonpb.Session_Session_ETH) || got[2] != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("expected RTH/ETH then fallback Session_ALL, got %#v", got)
	}
	if session, ok := ex.ResolveKLineSession(klines[0]); !ok || session != market.SessionOvernight {
		t.Fatalf("expected fallback ALL route to classify overnight candle, got %s ok=%v", session, ok)
	}
}

func TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage(t *testing.T) {
	plan := historicalKLineRequestPlanAll()
	err := &historicalKLineRequestError{
		session: plan.session,
		retType: 1,
		errCode: 0,
		retMsg:  "获取历史K线的时段仅支持设置 RTH，ETH，ALL",
	}
	if !shouldFallbackHistoricalKLineSplit(err, plan) {
		t.Fatal("expected supported-session-list message to trigger fallback")
	}
}

func TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 10, 55, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 1, StartTime: new(labelAt.Add(-time.Hour)), EndTime: new(labelAt.Add(time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 1 {
		t.Fatalf("expected one kline, got %d", len(klines))
	}

	wantStart := labelAt.Add(-time.Minute)
	wantEnd := labelAt.Add(-time.Millisecond)
	if !klines[0].StartTime.Time().Equal(wantStart) {
		t.Fatalf("StartTime = %s, want %s", klines[0].StartTime.Time(), wantStart)
	}
	if !klines[0].EndTime.Time().Equal(wantEnd) {
		t.Fatalf("EndTime = %s, want %s", klines[0].EndTime.Time(), wantEnd)
	}
}

func TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1d, types.KLineQueryOptions{Limit: 1, StartTime: new(labelAt.Add(-24 * time.Hour)), EndTime: new(labelAt.Add(24 * time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 1 {
		t.Fatalf("expected one kline, got %d", len(klines))
	}
	if !klines[0].StartTime.Time().Equal(labelAt) {
		t.Fatalf("StartTime = %s, want %s", klines[0].StartTime.Time(), labelAt)
	}
}

func TestQueryKLinesFollowsHistoryPaginationAndKeepsLatestLimit(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	oldAt := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.UTC)
	recentAt := time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{
		{
			testHistoryKLine(oldAt, 100),
			testHistoryKLine(oldAt.Add(5*time.Minute), 101),
		},
		{
			testHistoryKLine(recentAt, 200),
			testHistoryKLine(recentAt.Add(5*time.Minute), 201),
		},
	})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{Limit: 2, StartTime: new(oldAt.Add(-time.Hour)), EndTime: new(recentAt.Add(time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.historyKLCallCount(); got != 2 {
		t.Fatalf("expected two paginated RequestHistoryKL calls, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected latest two klines, got %d", len(klines))
	}
	if !klines[0].StartTime.Time().Equal(recentAt.Add(-5*time.Minute)) || !klines[1].StartTime.Time().Equal(recentAt) {
		t.Fatalf("expected latest page to be retained, got %#v", klines)
	}
}

func TestQueryKLinesAllowsMoreThanEightHistoryPages(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	baseAt := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.UTC)
	pages := make([][]*qotcommonpb.KLine, 0, 9)
	for index := range 9 {
		labelAt := baseAt.Add(time.Duration(index) * 5 * time.Minute)
		pages = append(pages, []*qotcommonpb.KLine{testHistoryKLine(labelAt, 100+float64(index))})
	}
	server.setHistoryPages(pages)

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{Limit: 2, StartTime: new(baseAt.Add(-time.Hour)), EndTime: new(baseAt.Add(2 * time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.historyKLCallCount(); got != 9 {
		t.Fatalf("expected nine paginated RequestHistoryKL calls, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected latest two klines, got %d", len(klines))
	}
	if klines[0].Open.Float64() != 107 || klines[1].Open.Float64() != 108 {
		t.Fatalf("expected last two paginated klines, got %#v", klines)
	}
}

func TestQueryKLinesUsesLargerHistoryPageSizeThanRequestedLimit(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	baseAt := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.UTC)
	series := make([]*qotcommonpb.KLine, 0, 401)
	for index := range 401 {
		series = append(series, testHistoryKLine(baseAt.Add(time.Duration(index)*time.Minute), 100+float64(index)))
	}
	server.setHistorySeries(series)

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 2, StartTime: new(baseAt.Add(-time.Minute)), EndTime: new(baseAt.Add(401 * time.Minute))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.historyKLCallCount(); got != 3 {
		t.Fatalf("expected three RequestHistoryKL calls with enlarged page size, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected latest two klines, got %d", len(klines))
	}
	if klines[0].Open.Float64() != 499 || klines[1].Open.Float64() != 500 {
		t.Fatalf("expected last two klines from history series, got %#v", klines)
	}
}

func TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	historyLabelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	currentLabelAt := historyLabelAt.Add(time.Minute)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(historyLabelAt, 100)}})
	server.setCurrentKLines([]*qotcommonpb.KLine{testCurrentKLine(currentLabelAt, 101, 106, 99, 103, 500)})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()
	if err := ex.SubscribeKLine(t.Context(), "HK.00700", types.Interval1m); err != nil {
		t.Fatalf("SubscribeKLine: %v", err)
	}

	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 2, StartTime: new(historyLabelAt.Add(-time.Hour)), EndTime: new(currentLabelAt.Add(time.Hour))})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if got := server.currentKLCallCount(); got != 1 {
		t.Fatalf("expected one GetKL call, got %d", got)
	}
	if len(klines) != 2 {
		t.Fatalf("expected closed and current kline, got %d", len(klines))
	}

	if !klines[0].StartTime.Time().Equal(historyLabelAt.Add(-time.Minute)) {
		t.Fatalf("first StartTime = %s, want %s", klines[0].StartTime.Time(), historyLabelAt.Add(-time.Minute))
	}
	if !klines[1].StartTime.Time().Equal(historyLabelAt) {
		t.Fatalf("current StartTime = %s, want %s", klines[1].StartTime.Time(), historyLabelAt)
	}
	if klines[1].Open.Float64() != 101 || klines[1].High.Float64() != 106 || klines[1].Low.Float64() != 99 || klines[1].Close.Float64() != 103 {
		t.Fatalf("unexpected current kline OHLC: %#v", klines[1])
	}
	if klines[1].Volume.Float64() != 500 {
		t.Fatalf("current Volume = %v, want 500", klines[1].Volume.Float64())
	}
	if klines[1].Closed {
		t.Fatal("expected current GetKL candle to remain open")
	}
}

func TestStreamConnectEmitsBasicQotPushAsBBGOEvents(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()

	stream := NewStream(ex)
	stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
	trades := make(chan types.Trade, 1)
	bookTickers := make(chan types.BookTicker, 1)
	stream.OnMarketTrade(func(trade types.Trade) {
		trades <- trade
	})
	stream.OnBookTickerUpdate(func(bookTicker types.BookTicker) {
		bookTickers <- bookTicker
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := stream.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect: %v", err)
	}
	defer func() { jftradeCheckTestError(t, stream.Close()) }()

	select {
	case trade := <-trades:
		if trade.Symbol != "HK.00700" || trade.Price.Float64() != 700 {
			t.Fatalf("unexpected market trade: %+v", trade)
		}
		if trade.Quantity.Float64() != 0 || trade.CumulativeVolume == nil || trade.CumulativeVolume.Float64() != 1000 {
			t.Fatalf("unexpected market trade volume contract: %+v", trade)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for market trade push")
	}

	select {
	case bookTicker := <-bookTickers:
		if bookTicker.Symbol != "HK.00700" || bookTicker.Buy.Float64() != 700 || bookTicker.Sell.Float64() != 700 {
			t.Fatalf("unexpected book ticker: %+v", bookTicker)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for book ticker push")
	}

	if got := server.pushSubCallCount(); got != 1 {
		t.Fatalf("expected one push Qot_Sub call, got %d", got)
	}
}

func TestStreamConnectRebuildsClosedCachedOpenDClient(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, ex.Close()) }()
	if err := ex.SubscribeBasicQuote(t.Context(), "HK.00700", false); err != nil {
		t.Fatalf("SubscribeBasicQuote: %v", err)
	}

	if _, err := ex.QueryTicker(t.Context(), "HK.00700"); err != nil {
		t.Fatalf("QueryTicker: %v", err)
	}
	if client := ex.Client(); client != nil {
		jftradeErr2 := client.Close()
		jftradeCheckTestError(t, jftradeErr2)
	}

	stream := NewStream(ex)
	stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := stream.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect after cached client close: %v", err)
	}
	defer func() { jftradeCheckTestError(t, stream.Close()) }()

	if got := server.acceptCount(); got < 2 {
		t.Fatalf("expected stream to create a fresh OpenD session, got %d accepts", got)
	}
}
