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
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetbasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetbasicqot"
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

type quoteOpenDServer struct {
	addr              string
	accepts           atomic.Int32
	qotSubCalls       atomic.Int32
	pushSubCalls      atomic.Int32
	basicQotCalls     atomic.Int32
	listener          net.Listener
	stopOnce          sync.Once
	shutdownCompleted chan struct{}
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
