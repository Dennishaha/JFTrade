package futu

import (
	"errors"
	"testing"
	"time"

	bbgoexchange "github.com/jftrade/jftrade-main/pkg/bbgo/exchange"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestWithClientRetriesRecoverableErrorsAndReturnsLastError(t *testing.T) {
	_, exchange := coverageMarginExchange(t)
	calls := 0
	err := exchange.withClient(t.Context(), func(*opend.Client) error {
		calls++
		return opend.ErrClosed
	})
	if !errors.Is(err, opend.ErrClosed) || calls != 2 {
		t.Fatalf("recoverable retries = %d, %v", calls, err)
	}
	for _, err := range []error{
		opend.ErrClosed,
		opend.ErrRequestTimeout,
		errors.New("broken pipe"),
		errors.New("connection reset by peer"),
		errors.New("EOF"),
		errors.New("use of closed network connection"),
	} {
		if !isRecoverableOpenDErr(err) {
			t.Fatalf("recoverable OpenD error rejected: %v", err)
		}
	}
	if isRecoverableOpenDErr(nil) || isRecoverableOpenDErr(errors.New("permission denied")) {
		t.Fatal("non-recoverable OpenD error classified as recoverable")
	}
	jftradeLogError(nil, errors.New("best effort"), "ignored")
}

func TestExchangeReconnectsClosedReadyClientAndCoversHandlerBoundaries(t *testing.T) {
	_, exchange := coverageMarginExchange(t)
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close() error = %v", err)
	}
	replacement, err := exchange.ensureClient(t.Context())
	if err != nil || replacement == client {
		t.Fatalf("replacement client = %p, %v", replacement, err)
	}

	exchange.OnSystemNotify(nil)
	exchange.OnOrderBookUpdate(nil)()
	exchange.OnOrderUpdate(nil)()
	exchange.OnOrderFillUpdate(nil)()
	seenSymbol := ""
	remove := exchange.OnOrderBookUpdate(func(symbol string) { seenSymbol = symbol })
	exchange.dispatchOrderBookNotify(nil)
	exchange.dispatchOrderBookNotify(&qotupdateorderbookpb.S2C{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}})
	exchange.dispatchOrderBookNotify(&qotupdateorderbookpb.S2C{Security: testHKSecurity("00700")})
	if seenSymbol != "HK.00700" {
		t.Fatalf("order-book notification symbol = %q", seenSymbol)
	}
	remove()
	seenNotify := false
	exchange.OnSystemNotify(func(*notifypb.Response) { seenNotify = true })
	exchange.dispatchSystemNotify(&notifypb.Response{})
	if !seenNotify {
		t.Fatal("system notification was not dispatched")
	}

	empty := NewExchange("")
	empty.bindTradeUpdateNotifyLocked(nil)
	empty.bindTradeUpdateNotifyLocked(opend.New(opend.Config{}))
	closed := opend.New(opend.Config{})
	if err := closed.Close(); err != nil {
		t.Fatalf("closed client close error = %v", err)
	}
	empty.tradeAccountPushIDs = []uint64{1}
	if err := empty.resubscribeTradeAccountPushLocked(t.Context(), closed); err == nil {
		t.Fatal("closed-client trade push resubscribe error = nil")
	}
}

func TestTradeAccountPushNormalizationSubscriptionAndFactoryEnvironment(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	if err := exchange.SubscribeTradeAccountPush(t.Context(), nil); err != nil {
		t.Fatalf("empty account push subscription error = %v", err)
	}
	if err := exchange.SubscribeTradeAccountPush(t.Context(), []uint64{2, 1, 2}); err != nil {
		t.Fatalf("account push subscription error = %v", err)
	}
	if err := exchange.SubscribeTradeAccountPush(t.Context(), []uint64{1, 2}); err != nil {
		t.Fatalf("idempotent account push subscription error = %v", err)
	}
	if server.tradeAccPushCalls.Load() != 1 {
		t.Fatalf("account push calls = %d", server.tradeAccPushCalls.Load())
	}
	if sameUint64Set([]uint64{1}, []uint64{1, 2}) || sameUint64Set([]uint64{1, 3}, []uint64{1, 2}) {
		t.Fatal("different account ID sets compared equal")
	}

	prefix := "JFTRADE_COVERAGE_FUTU"
	t.Setenv(prefix+"_OPEND_ADDR", "127.0.0.1:12345")
	t.Setenv(prefix+"_OPEND_WEBSOCKET_KEY", "prefix-key")
	created, err := bbgoexchange.NewWithEnvVarPrefix(Name, prefix)
	if err != nil {
		t.Fatalf("factory prefix environment error = %v", err)
	}
	if got := created.(*Exchange); got.addr != "127.0.0.1:12345" || got.webSocketKey != "prefix-key" {
		t.Fatalf("factory exchange = %#v", got)
	}

	t.Setenv(prefix+"_OPEND_ADDR", "")
	t.Setenv(prefix+"_OPEND_WEBSOCKET_KEY", "")
	t.Setenv(EnvOpenDAddr, "")
	t.Setenv(EnvOpenDWebSocketKey, "")
	t.Setenv("JFTRADE_FUTU_WEBSOCKET_KEY", "legacy-key")
	created, err = bbgoexchange.NewWithEnvVarPrefix(Name, prefix)
	if err != nil {
		t.Fatalf("factory default environment error = %v", err)
	}
	if got := created.(*Exchange); got.addr != DefaultOpenDAddr || got.webSocketKey != "legacy-key" {
		t.Fatalf("factory default exchange = %#v", got)
	}

	constructed, err := bbgoexchange.New(Name, bbgoexchange.Options{})
	if err != nil {
		t.Fatalf("factory constructor error = %v", err)
	}
	if got := constructed.(*Exchange); got.addr != DefaultOpenDAddr || got.webSocketKey != "legacy-key" {
		t.Fatalf("constructor fallback exchange = %#v", got)
	}
}

func TestOldOpenDVersionFailsSessionInitialization(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.serverVer.Store(1)
	t.Cleanup(server.stop)
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: time.Second})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })
	if err := exchange.Connect(t.Context()); err == nil {
		t.Fatal("old OpenD version connect error = nil")
	}
}

func TestInitResponseAndSessionTransportFailures(t *testing.T) {
	if _, err := validateInitConnectResponse(&initpb.Response{RetType: new(int32(1)), RetMsg: new("denied")}); err == nil {
		t.Fatal("InitConnect retType error = nil")
	}
	if _, err := validateInitConnectResponse(&initpb.Response{RetType: new(int32(0))}); err == nil {
		t.Fatal("InitConnect missing state error = nil")
	}
	if _, err := validateInitConnectResponse(&initpb.Response{RetType: new(int32(0)), S2C: &initpb.S2C{ServerVer: new(int32(1))}}); err == nil {
		t.Fatal("InitConnect old version error = nil")
	}
	for _, protoID := range []uint32{opend.ProtoInitConnect, opend.ProtoGetGlobalState} {
		server := startQuoteOpenDServer(t)
		server.setDropProto(protoID)
		exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: time.Second})
		if err := exchange.Connect(t.Context()); err == nil {
			t.Fatalf("session protocol %d disconnect error = nil", protoID)
		}
		jftradeCheckTestError(t, exchange.Close())
		server.stop()
	}
}

func TestReconnectTradePushFailureAndTradeHandlerBinding(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setDropProto(opend.ProtoTrdSubAccPush)
	exchange.tradeAccountPushIDs = []uint64{1}
	if _, err := exchange.ensureClient(t.Context()); err == nil {
		t.Fatal("trade push resubscribe disconnect error = nil")
	}

	client := opend.New(opend.Config{})
	exchange = NewExchange("")
	exchange.orderUpdateHandlers = map[uint64]func(*trdcommonpb.TrdHeader, *trdcommonpb.Order){1: func(*trdcommonpb.TrdHeader, *trdcommonpb.Order) {}}
	exchange.bindTradeUpdateNotifyLocked(client)
	if exchange.orderUpdateNotifyClient != client {
		t.Fatal("trade update handlers were not bound")
	}
}
