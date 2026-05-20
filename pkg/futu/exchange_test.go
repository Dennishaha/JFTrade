package futu

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/exchange"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
	qotgetklpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetkl"
	historypb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotrequesthistorykl"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
)

func TestRegistration(t *testing.T) {
	if !types.ExchangeName("futu").IsValid() {
		t.Fatal("futu should be registered as a valid bbgo exchange via init()")
	}
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{"OPEND_ADDR": "127.0.0.1:11110"})
	if err != nil {
		t.Fatalf("exchange.New: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestConstructorFallsBackToDefaultAddress(t *testing.T) {
	t.Setenv("FUTU_OPEND_ADDR", "")
	ex, err := exchange.New(types.ExchangeName("futu"), exchange.Options{})
	if err != nil {
		t.Fatalf("expected default OpenD address fallback, got error: %v", err)
	}
	if ex.Name() != Name {
		t.Fatalf("ex.Name() = %s", ex.Name())
	}
}

func TestQueryMarketsReturnsBootstrapMarket(t *testing.T) {
	ex := NewExchange("127.0.0.1:11110")
	markets, err := ex.QueryMarkets(t.Context())
	if err != nil {
		t.Fatalf("QueryMarkets: %v", err)
	}
	market, ok := markets["HK.00700"]
	if !ok {
		t.Fatalf("expected bootstrap market HK.00700, got %#v", markets)
	}
	if market.Exchange != Name || market.QuoteCurrency != "HKD" {
		t.Fatalf("unexpected bootstrap market: %#v", market)
	}
}

func TestQueryTickerReusesSingleOpenDConnection(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	firstTicker, err := ex.QueryTicker(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("first QueryTicker: %v", err)
	}
	secondTicker, err := ex.QueryTicker(t.Context(), "HK.00700")
	if err != nil {
		t.Fatalf("second QueryTicker: %v", err)
	}
	if firstTicker == nil || secondTicker == nil {
		t.Fatal("expected non-nil tickers")
	}
	if got := server.acceptCount(); got != 1 {
		t.Fatalf("expected one OpenD TCP session, got %d", got)
	}
	if got := server.subCallCount(); got != 1 {
		t.Fatalf("expected one Qot_Sub call, got %d", got)
	}
	if got := server.basicQotCallCount(); got != 2 {
		t.Fatalf("expected two GetBasicQot calls, got %d", got)
	}
	if firstTicker.Last.Float64() != secondTicker.Last.Float64() {
		t.Fatalf("expected stable quote price, got %f and %f", firstTicker.Last.Float64(), secondTicker.Last.Float64())
	}
}

func TestQueryTickersBatchesBasicQotRequests(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

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
	if got := server.subCallCount(); got != 1 {
		t.Fatalf("expected one batched Qot_Sub call, got %d", got)
	}
	if got := server.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one batched GetBasicQot call, got %d", got)
	}
	if _, ok := tickers["US.NVDA"]; !ok {
		t.Fatalf("expected batched quote for US.NVDA, got %#v", tickers)
	}
}

func TestQueryKLinesRequestsUSExtendedHours(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "US.NVDA", types.Interval1m, types.KLineQueryOptions{Limit: 2, StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("QueryKLines: %v", err)
	}
	if len(klines) != 1 {
		t.Fatalf("expected one test kline, got %d", len(klines))
	}
	if got := server.historyKLCallCount(); got != 1 {
		t.Fatalf("expected one RequestHistoryKL call, got %d", got)
	}
	if !server.lastHistoryExtendedTime() {
		t.Fatal("expected US intraday RequestHistoryKL to set extendedTime=true")
	}
	if got := server.lastHistorySession(); got != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("expected Session_ALL, got %d", got)
	}
}

func TestQueryKLinesNormalizesIntradayHistoryLabelToBucketStart(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	labelAt := time.Date(2026, time.May, 20, 10, 55, 0, 0, time.UTC)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := labelAt.Add(-time.Hour)
	end := labelAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 1, StartTime: &start, EndTime: &end})
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
	defer ex.Close()

	start := labelAt.Add(-24 * time.Hour)
	end := labelAt.Add(24 * time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1d, types.KLineQueryOptions{Limit: 1, StartTime: &start, EndTime: &end})
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
	defer ex.Close()

	start := oldAt.Add(-time.Hour)
	end := recentAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{Limit: 2, StartTime: &start, EndTime: &end})
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

func TestQueryKLinesIncludesCurrentRealtimeBucketFromGetKL(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	historyLabelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	currentLabelAt := historyLabelAt.Add(time.Minute)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(historyLabelAt, 100)}})
	server.setCurrentKLines([]*qotcommonpb.KLine{testCurrentKLine(currentLabelAt, 101, 106, 99, 103, 500)})

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer ex.Close()

	start := historyLabelAt.Add(-time.Hour)
	end := currentLabelAt.Add(time.Hour)
	klines, err := ex.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{Limit: 2, StartTime: &start, EndTime: &end})
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
	defer ex.Close()

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
	defer stream.Close()

	select {
	case trade := <-trades:
		if trade.Symbol != "HK.00700" || trade.Price.Float64() != 700 {
			t.Fatalf("unexpected market trade: %+v", trade)
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
	defer ex.Close()

	if _, err := ex.QueryTicker(t.Context(), "HK.00700"); err != nil {
		t.Fatalf("QueryTicker: %v", err)
	}
	if client := ex.Client(); client != nil {
		_ = client.Close()
	}

	stream := NewStream(ex)
	stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := stream.Connect(ctx); err != nil {
		t.Fatalf("stream.Connect after cached client close: %v", err)
	}
	defer stream.Close()

	if got := server.acceptCount(); got < 2 {
		t.Fatalf("expected stream to create a fresh OpenD session, got %d accepts", got)
	}
}

type quoteOpenDServer struct {
	addr              string
	accepts           atomic.Int32
	qotSubCalls       atomic.Int32
	pushSubCalls      atomic.Int32
	basicQotCalls     atomic.Int32
	historyKLCalls    atomic.Int32
	currentKLCalls    atomic.Int32
	historyExtended   atomic.Bool
	historySession    atomic.Int32
	historyMu         sync.Mutex
	historyPages      [][]*qotcommonpb.KLine
	currentKLines     []*qotcommonpb.KLine
	listener          net.Listener
	stopOnce          sync.Once
	shutdownCompleted chan struct{}
}

func (s *quoteOpenDServer) setHistoryPages(pages [][]*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.historyPages = pages
}

func (s *quoteOpenDServer) setCurrentKLines(klines []*qotcommonpb.KLine) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	s.currentKLines = klines
}

func startQuoteOpenDServer(t *testing.T) *quoteOpenDServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &quoteOpenDServer{
		addr:              listener.Addr().String(),
		listener:          listener,
		shutdownCompleted: make(chan struct{}),
	}
	go server.acceptLoop()
	return server
}

func (s *quoteOpenDServer) stop() {
	s.stopOnce.Do(func() {
		_ = s.listener.Close()
		<-s.shutdownCompleted
	})
}

func (s *quoteOpenDServer) acceptLoop() {
	defer close(s.shutdownCompleted)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.accepts.Add(1)
		go s.handleConn(conn)
	}
}

func (s *quoteOpenDServer) handleConn(conn net.Conn) {
	defer conn.Close()
	for {
		header := make([]byte, codec.HeaderLen)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		bodyLen := int(uint32(header[12]) | uint32(header[13])<<8 | uint32(header[14])<<16 | uint32(header[15])<<24)
		packet := make([]byte, codec.HeaderLen+bodyLen)
		copy(packet, header)
		if _, err := io.ReadFull(conn, packet[codec.HeaderLen:]); err != nil {
			return
		}
		frame, err := codec.Decode(packet)
		if err != nil {
			return
		}

		var response proto.Message
		switch frame.Header.ProtoID {
		case opend.ProtoInitConnect:
			response = &initpb.Response{
				RetType: proto.Int32(0),
				S2C: &initpb.S2C{
					ServerVer:         proto.Int32(700),
					LoginUserID:       proto.Uint64(1),
					ConnID:            proto.Uint64(42),
					ConnAESKey:        proto.String("0123456789abcdef"),
					KeepAliveInterval: proto.Int32(10),
				},
			}
		case opend.ProtoQotSub:
			s.qotSubCalls.Add(1)
			request := &qotsubpb.Request{}
			_ = proto.Unmarshal(frame.Body, request)
			isPushSub := request.GetC2S().GetIsRegOrUnRegPush()
			if isPushSub {
				s.pushSubCalls.Add(1)
			}
			response = &qotsubpb.Response{RetType: proto.Int32(0)}
		case opend.ProtoGetBasicQot:
			s.basicQotCalls.Add(1)
			response = s.basicQotResponse(frame.Body)
		case opend.ProtoGetKL:
			s.currentKLCalls.Add(1)
			response = s.currentKLResponse(frame.Body)
		case opend.ProtoRequestHistoryKL:
			s.historyKLCalls.Add(1)
			response = s.historyKLResponse(frame.Body)
		default:
			return
		}

		body, err := proto.Marshal(response)
		if err != nil {
			return
		}
		packet, err = codec.Encode(frame.Header.ProtoID, frame.Header.SerialNo, body)
		if err != nil {
			return
		}
		if _, err := conn.Write(packet); err != nil {
			return
		}
		if frame.Header.ProtoID == opend.ProtoQotSub {
			request := &qotsubpb.Request{}
			_ = proto.Unmarshal(frame.Body, request)
			if request.GetC2S().GetIsRegOrUnRegPush() {
				if err := s.writeBasicQotPush(conn, request.GetC2S().GetSecurityList()); err != nil {
					return
				}
			}
		}
	}
}

func (s *quoteOpenDServer) historyKLResponse(body []byte) *historypb.Response {
	request := &historypb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &historypb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.historyExtended.Store(request.GetC2S().GetExtendedTime())
	s.historySession.Store(request.GetC2S().GetSession())
	s.historyMu.Lock()
	if len(s.historyPages) > 0 {
		pageIndex := int(s.historyKLCalls.Load()) - 1
		if pageIndex < 0 {
			pageIndex = 0
		}
		if pageIndex >= len(s.historyPages) {
			pageIndex = len(s.historyPages) - 1
		}
		response := &historypb.Response{
			RetType: proto.Int32(0),
			S2C: &historypb.S2C{
				Security: request.GetC2S().GetSecurity(),
				KlList:   s.historyPages[pageIndex],
			},
		}
		if pageIndex < len(s.historyPages)-1 {
			response.S2C.NextReqKey = []byte{byte(pageIndex + 1)}
		}
		s.historyMu.Unlock()
		return response
	}
	s.historyMu.Unlock()

	startAt := time.Date(2026, time.May, 20, 8, 0, 0, 0, time.UTC)
	return &historypb.Response{
		RetType: proto.Int32(0),
		S2C: &historypb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList: []*qotcommonpb.KLine{
				{
					Time:       proto.String(startAt.Format("2006-01-02 15:04:05")),
					Timestamp:  proto.Float64(float64(startAt.Unix())),
					IsBlank:    proto.Bool(false),
					OpenPrice:  proto.Float64(100),
					HighPrice:  proto.Float64(101),
					LowPrice:   proto.Float64(99),
					ClosePrice: proto.Float64(100.5),
					Volume:     proto.Int64(1000),
					Turnover:   proto.Float64(100500),
				},
			},
		},
	}
}

func (s *quoteOpenDServer) currentKLResponse(body []byte) *qotgetklpb.Response {
	request := &qotgetklpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetklpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	response := &qotgetklpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetklpb.S2C{
			Security: request.GetC2S().GetSecurity(),
			KlList:   s.currentKLines,
		},
	}
	return response
}

func testHistoryKLine(at time.Time, price float64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       proto.String(at.Format("2006-01-02 15:04:05")),
		Timestamp:  proto.Float64(float64(at.Unix())),
		IsBlank:    proto.Bool(false),
		OpenPrice:  proto.Float64(price),
		HighPrice:  proto.Float64(price + 1),
		LowPrice:   proto.Float64(price - 1),
		ClosePrice: proto.Float64(price + 0.5),
		Volume:     proto.Int64(1000),
		Turnover:   proto.Float64(price * 1000),
	}
}

func testCurrentKLine(at time.Time, open float64, high float64, low float64, close float64, volume int64) *qotcommonpb.KLine {
	return &qotcommonpb.KLine{
		Time:       proto.String(at.Format("2006-01-02 15:04:05")),
		Timestamp:  proto.Float64(float64(at.Unix())),
		IsBlank:    proto.Bool(false),
		OpenPrice:  proto.Float64(open),
		HighPrice:  proto.Float64(high),
		LowPrice:   proto.Float64(low),
		ClosePrice: proto.Float64(close),
		Volume:     proto.Int64(volume),
		Turnover:   proto.Float64(close * float64(volume)),
	}
}

func (s *quoteOpenDServer) writeBasicQotPush(conn net.Conn, securities []*qotcommonpb.Security) error {
	response := &qotupdatebasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C:     &qotupdatebasicqotpb.S2C{BasicQotList: basicQotListForSecurities(securities)},
	}
	body, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	packet, err := codec.Encode(opend.ProtoQotUpdateBasicQot, 0, body)
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func (s *quoteOpenDServer) basicQotResponse(body []byte) *qotgetbasicqotpb.Response {
	request := &qotgetbasicqotpb.Request{}
	if err := proto.Unmarshal(body, request); err != nil {
		return &qotgetbasicqotpb.Response{RetType: proto.Int32(1), RetMsg: proto.String(err.Error())}
	}

	quotes := basicQotListForSecurities(request.GetC2S().GetSecurityList())

	return &qotgetbasicqotpb.Response{
		RetType: proto.Int32(0),
		S2C: &qotgetbasicqotpb.S2C{
			BasicQotList: quotes,
		},
	}
}

func basicQotListForSecurities(securities []*qotcommonpb.Security) []*qotcommonpb.BasicQot {
	quotes := make([]*qotcommonpb.BasicQot, 0, len(securities))
	baseQuoteTime := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	for index, security := range securities {
		price := 700.0 + float64(index)
		quotes = append(quotes, &qotcommonpb.BasicQot{
			Security:        security,
			IsSuspended:     proto.Bool(false),
			ListTime:        proto.String("2020-01-01"),
			PriceSpread:     proto.Float64(0.01),
			UpdateTime:      proto.String(baseQuoteTime.Format("2006-01-02 15:04:05")),
			HighPrice:       proto.Float64(price + 1),
			OpenPrice:       proto.Float64(price - 1),
			LowPrice:        proto.Float64(price - 2),
			CurPrice:        proto.Float64(price),
			LastClosePrice:  proto.Float64(price - 0.5),
			Volume:          proto.Int64(1000 + int64(index)*10),
			Turnover:        proto.Float64(price * 1000),
			TurnoverRate:    proto.Float64(1.25),
			Amplitude:       proto.Float64(2.5),
			UpdateTimestamp: proto.Float64(float64(baseQuoteTime.Unix())),
		})
	}
	return quotes
}

func (s *quoteOpenDServer) acceptCount() int {
	return int(s.accepts.Load())
}

func (s *quoteOpenDServer) subCallCount() int {
	return int(s.qotSubCalls.Load())
}

func (s *quoteOpenDServer) pushSubCallCount() int {
	return int(s.pushSubCalls.Load())
}

func (s *quoteOpenDServer) basicQotCallCount() int {
	return int(s.basicQotCalls.Load())
}

func (s *quoteOpenDServer) historyKLCallCount() int {
	return int(s.historyKLCalls.Load())
}

func (s *quoteOpenDServer) currentKLCallCount() int {
	return int(s.currentKLCalls.Load())
}

func (s *quoteOpenDServer) lastHistoryExtendedTime() bool {
	return s.historyExtended.Load()
}

func (s *quoteOpenDServer) lastHistorySession() int32 {
	return s.historySession.Load()
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for !condition() {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for condition")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
