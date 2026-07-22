package futu

import (
	"context"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
	qotupdatebasicqotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdatebasicqot"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestStreamConnectionAndSubscriptionBoundaries(t *testing.T) {
	dead := NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 30 * time.Millisecond})
	t.Cleanup(func() { jftradeCheckTestError(t, dead.Close()) })
	if err := NewStream(dead).Connect(t.Context()); err == nil {
		t.Fatal("stream connect to unavailable OpenD error = nil")
	}

	_, exchange := coverageMarginExchange(t)
	stream := NewStream(exchange)
	if err := stream.Connect(t.Context()); err != nil {
		t.Fatalf("first empty stream connect error = %v", err)
	}
	if err := stream.Connect(t.Context()); err != nil {
		t.Fatalf("replacement stream connect error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream close error = %v", err)
	}

	requests, err := basicQotRequestsFromSubscriptions([]types.Subscription{
		{Channel: types.KLineChannel, Symbol: "BAD"},
		{Channel: types.MarketTradeChannel, Symbol: "HK.00700"},
		{Channel: types.BookTickerChannel, Symbol: "hk.00700"},
	})
	if err != nil || len(requests) != 1 {
		t.Fatalf("basic quote subscription requests = %#v, %v", requests, err)
	}
	if _, err := basicQotRequestsFromSubscriptions([]types.Subscription{{Channel: types.MarketTradeChannel, Symbol: "BAD"}}); err == nil {
		t.Fatal("invalid basic quote subscription error = nil")
	}
	invalid := NewStream(exchange)
	invalid.Subscribe(types.MarketTradeChannel, "BAD", types.SubscribeOptions{})
	if err := invalid.connectOpenDBasicQot(t.Context()); err == nil {
		t.Fatal("invalid stream subscription connect error = nil")
	}
}

func TestStreamPushHandlersRejectInactiveMalformedAndEmptyQuotes(t *testing.T) {
	exchange := NewExchange("")
	stream := NewStream(exchange)
	stream.watchClientLoop(t.Context(), nil)
	if stream.isActive() {
		t.Fatal("new stream unexpectedly active")
	}
	stream.handleBasicQotPush(codec.Frame{Body: []byte("bad protobuf")})
	stream.handleOrderBookPush(nil)

	ctx, cancel := context.WithCancel(context.Background())
	stream.ctx = ctx
	stream.handleBasicQotPush(codec.Frame{Body: []byte("bad protobuf")})
	failureBody, err := proto.Marshal(&qotupdatebasicqotpb.Response{RetType: new(int32(1))})
	if err != nil {
		t.Fatalf("marshal quote failure: %v", err)
	}
	stream.handleBasicQotPush(codec.Frame{Body: failureBody})
	stream.emitBasicQot(&qotcommonpb.BasicQot{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}})
	stream.emitBasicQot(&qotcommonpb.BasicQot{Security: testHKSecurity("00700"), CurPrice: new(0.0)})
	stream.handleOrderBookPush(&qotupdateorderbookpb.S2C{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}})
	cancel()
	if stream.isActive() {
		t.Fatal("canceled stream unexpectedly active")
	}
	stream.watchClientLoop(ctx, opend.New(opend.Config{}))
}

func TestStreamConvertsCumulativeQuoteVolumeToIncrementalTradeQuantity(t *testing.T) {
	stream := NewStream(NewExchange(""))
	hk, err := time.LoadLocation("Asia/Hong_Kong")
	if err != nil {
		t.Fatalf("load Hong Kong timezone: %v", err)
	}
	first := time.Date(2026, time.July, 20, 10, 0, 0, 0, hk)
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first, 1_000); got != 0 {
		t.Fatalf("first cumulative sample quantity = %v, want baseline 0", got)
	}
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first.Add(time.Second), 1_015); got != 15 {
		t.Fatalf("incremental quantity = %v, want 15", got)
	}
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first.Add(2*time.Second), 1_010); got != 0 {
		t.Fatalf("decreasing cumulative quantity = %v, want 0", got)
	}
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first.Add(3*time.Second), math.NaN()); got != 0 {
		t.Fatalf("NaN cumulative quantity = %v, want 0", got)
	}
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first.Add(4*time.Second), math.Inf(1)); got != 0 {
		t.Fatalf("infinite cumulative quantity = %v, want 0", got)
	}
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, first.Add(5*time.Second), 1_020); got != 10 {
		t.Fatalf("valid quantity after invalid samples = %v, want 10", got)
	}
	nextDay := first.AddDate(0, 0, 1)
	if got := stream.nextTradeQuantity("HK.00700", market.SessionRegular, nextDay, 25); got != 0 {
		t.Fatalf("new trading-day baseline quantity = %v, want 0", got)
	}
}

func TestStreamMarketTradeCarriesDeltaAndCumulativeVolume(t *testing.T) {
	stream := NewStream(NewExchange(""))
	trades := make([]types.Trade, 0, 2)
	stream.OnMarketTrade(func(trade types.Trade) {
		trades = append(trades, trade)
	})

	security := testHKSecurity("00700")
	first := basicQotListForSecurities([]*qotcommonpb.Security{security})[0]
	stream.emitBasicQot(first)
	second := proto.Clone(first).(*qotcommonpb.BasicQot)
	second.Volume = new(int64(1_015))
	stream.emitBasicQot(second)

	if len(trades) != 2 {
		t.Fatalf("market trades = %#v, want 2 events", trades)
	}
	if trades[0].Quantity.Float64() != 0 || trades[0].CumulativeVolume == nil || trades[0].CumulativeVolume.Float64() != 1_000 {
		t.Fatalf("first market trade volume contract = %#v", trades[0])
	}
	if trades[1].Quantity.Float64() != 15 || trades[1].CumulativeVolume == nil || trades[1].CumulativeVolume.Float64() != 1_015 {
		t.Fatalf("second market trade volume contract = %#v", trades[1])
	}
}

func TestStreamRejectsNonFiniteSnapshotVolume(t *testing.T) {
	stream := NewStream(NewExchange(""))
	trades := 0
	stream.OnMarketTrade(func(types.Trade) { trades++ })
	basic := basicQotListForSecurities([]*qotcommonpb.Security{testHKSecurity("00700")})[0]
	stream.emitBasicQotSnapshot(basic, "HK.00700", &QuoteSnapshot{
		Symbol:  "HK.00700",
		Volume:  math.NaN(),
		QuoteAt: time.Now(),
		Session: market.SessionRegular,
	})
	if trades != 0 {
		t.Fatalf("market trades = %d, want none for non-finite cumulative volume", trades)
	}
}

func TestBasicQuotePushSubscriptionErrorsAndIdempotency(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	if err := subscribeBasicQotPush(t.Context(), client, nil); err != nil {
		t.Fatalf("empty push subscription error = %v", err)
	}
	security := testHKSecurity("00700")
	failure := &qotsubpb.Response{RetType: new(int32(1)), RetMsg: new("push denied")}
	server.setQotSubResponses(failure)
	if err := subscribeBasicQotPush(t.Context(), client, []*qotcommonpb.Security{security}); err == nil {
		t.Fatal("push subscription protocol error = nil")
	}
	closed := opend.New(opend.Config{})
	if err := closed.Close(); err != nil {
		t.Fatalf("close client error = %v", err)
	}
	if err := subscribeBasicQotPush(t.Context(), closed, []*qotcommonpb.Security{security}); err == nil {
		t.Fatal("closed-client push subscription error = nil")
	}

	request := basicQotRequest{canonical: "HK.00700", security: security}
	server.setQotSubResponses(failure)
	if err := exchange.ensureBasicQotPushSubscriptions(t.Context(), client, []basicQotRequest{request}); err == nil {
		t.Fatal("ensure push subscription error = nil")
	}
	server.setQotSubResponses(&qotsubpb.Response{RetType: new(int32(0))})
	if err := exchange.ensureBasicQotPushSubscriptions(t.Context(), client, []basicQotRequest{request}); err != nil {
		t.Fatalf("ensure push subscription success error = %v", err)
	}
	if err := exchange.ensureBasicQotPushSubscriptions(t.Context(), client, []basicQotRequest{request}); err != nil {
		t.Fatalf("idempotent ensure push error = %v", err)
	}
}

func TestOrderBookStreamConnectionBoundaries(t *testing.T) {
	_, exchange := coverageMarginExchange(t)
	stream := NewStream(exchange)
	if err := stream.connectOpenDOrderBook(t.Context()); err == nil {
		t.Fatal("empty order-book stream connect error = nil")
	}
	stream.Subscribe(types.BookTickerChannel, "BAD", types.SubscribeOptions{})
	if err := stream.connectOpenDOrderBook(t.Context()); err == nil {
		t.Fatal("invalid order-book stream connect error = nil")
	}

	valid := NewStream(exchange)
	valid.ctx = context.Background()
	valid.Subscribe(types.BookTickerChannel, "HK.00700", types.SubscribeOptions{})
	if err := valid.connectOpenDOrderBook(t.Context()); err != nil {
		t.Fatalf("valid order-book stream connect error = %v", err)
	}
	valid.handleOrderBookPush(&qotupdateorderbookpb.S2C{Security: testHKSecurity("00700")})

	dead := NewStream(NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 30 * time.Millisecond}))
	if err := dead.connectOpenDOrderBook(t.Context()); err == nil {
		t.Fatal("unavailable order-book stream connect error = nil")
	}
}

func TestStreamReconnectAndClientWatcherExitPaths(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	NewStream(NewExchange("")).reconnectLoop(canceledCtx)

	closedStream := NewStream(NewExchange(""))
	close(closedStream.CloseC)
	closedStream.reconnectLoop(context.Background())
	closedStream.watchClientLoop(context.Background(), opend.New(opend.Config{}))

	watched := NewStream(NewExchange(""))
	client := opend.New(opend.Config{})
	if err := client.Close(); err != nil {
		t.Fatalf("watched client close error = %v", err)
	}
	reconnected := make(chan struct{})
	go func() {
		<-watched.ReconnectC
		close(reconnected)
	}()
	watched.watchClientLoop(context.Background(), client)
	select {
	case <-reconnected:
	case <-time.After(time.Second):
		t.Fatal("closed client did not request stream reconnect")
	}

	_, exchange := coverageMarginExchange(t)
	stream := NewStream(exchange)
	stream.ctx = context.Background()
	connected := make(chan struct{}, 1)
	stream.OnConnect(func() { connected <- struct{}{} })
	loopCtx, stopLoop := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() {
		stream.reconnectLoop(loopCtx)
		close(loopDone)
	}()
	stream.ReconnectC <- struct{}{}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("ReconnectC did not reconnect OpenD stream")
	}
	stopLoop()
	select {
	case <-loopDone:
	case <-time.After(time.Second):
		t.Fatal("reconnect loop did not stop")
	}
}

func TestStreamConnectReportsPhysicalSubscriptionFailures(t *testing.T) {
	failure := &qotsubpb.Response{RetType: new(int32(1)), RetMsg: new("denied")}
	t.Run("basic quote", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setQotSubResponses(failure)
		stream := NewStream(exchange)
		stream.ctx = context.Background()
		stream.Subscribe(types.MarketTradeChannel, "HK.00700", types.SubscribeOptions{})
		if err := stream.connectOpenDBasicQot(t.Context()); err == nil {
			t.Fatal("BasicQot physical subscription error = nil")
		}
	})
	t.Run("order book", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setQotSubResponses(failure)
		stream := NewStream(exchange)
		stream.ctx = context.Background()
		stream.Subscribe(types.BookTickerChannel, "HK.00700", types.SubscribeOptions{})
		if err := stream.connectOpenDOrderBook(t.Context()); err == nil {
			t.Fatal("order-book physical subscription error = nil")
		}
	})
}
