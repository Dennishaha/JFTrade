package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	bbgoexchange "github.com/jftrade/jftrade-main/pkg/bbgo/exchange"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	initpb "github.com/jftrade/jftrade-main/pkg/futu/pb/initconnect"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestWithClientReplayPolicyForRecoverableErrors(t *testing.T) {
	_, exchange := coverageMarginExchange(t)
	calls := 0
	err := exchange.withClient(t.Context(), func(*opend.Client) error {
		calls++
		return opend.ErrClosed
	})
	if !errors.Is(err, opend.ErrClosed) || calls != 1 {
		t.Fatalf("single-attempt calls = %d, %v; want one call without replay", calls, err)
	}

	calls = 0
	err = exchange.withRetryingClient(t.Context(), func(*opend.Client) error {
		calls++
		return opend.ErrClosed
	})
	if !errors.Is(err, opend.ErrClosed) || calls != 2 {
		t.Fatalf("replay-safe calls = %d, %v; want one retry", calls, err)
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
	besteffort.LogError(errors.New("best effort"))
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
	exchange.dispatchSystemNotifyFrom(client, &notifypb.Response{})
	if seenNotify {
		t.Fatal("stale-client system notification was dispatched")
	}
	exchange.dispatchSystemNotifyFrom(replacement, &notifypb.Response{})
	if !seenNotify {
		t.Fatal("current-client system notification was not dispatched")
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

func TestReconnectDoesNotDeadlockWithInFlightNotification(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setNotifyAfterGlobalState(&notifypb.Response{
		RetType: new(int32(0)),
		S2C: &notifypb.S2C{
			Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
			ConnectStatus: &notifypb.ConnectStatus{
				QotLogined: new(true),
				TrdLogined: new(true),
			},
		},
	})
	t.Cleanup(server.stop)

	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: time.Second})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerDone := make(chan struct{})
	exchange.OnSystemNotify(func(*notifypb.Response) {
		close(handlerStarted)
		<-releaseHandler
		_ = exchange.ConnectionGeneration()
		close(handlerDone)
	})
	if err := exchange.Connect(t.Context()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("OpenD notification handler did not start")
	}
	server.setNotifyAfterGlobalState(nil)

	client := exchange.Client()
	closeDone := make(chan error, 1)
	go func() { closeDone <- client.Close() }()
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("OpenD client did not begin closing")
	}

	reconnectDone := make(chan error, 1)
	go func() { reconnectDone <- exchange.Connect(t.Context()) }()
	waitForDetachedOpenDClient(t, exchange)
	close(releaseHandler)

	select {
	case err := <-reconnectDone:
		if err != nil {
			t.Fatalf("Connect() after transport close error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenD reconnect deadlocked while closing the notification worker")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("client.Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenD client close did not join the notification worker")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("OpenD notification handler did not finish")
	}
	if generation := exchange.ConnectionGeneration(); generation < 3 {
		t.Fatalf("connection generation = %d, want reconnect generation", generation)
	}
}

func waitForDetachedOpenDClient(t *testing.T, exchange *Exchange) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for exchange.activeClient.Load() != nil {
		select {
		case <-ctx.Done():
			t.Fatal("OpenD client was not detached")
		case <-ticker.C:
		}
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
