package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

// --- Test: order book request construction validates security typing ---
// Note: Actual OpenD subscription and query calls require a running OpenD
// instance and are covered by integration tests in the jftradeapi package.

func TestSubscribeOrderBookRequestConstruction(t *testing.T) {
	security := &qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   new("00700"),
	}
	securities := []*qotcommonpb.Security{security}

	// Verify security construction for order book operations doesn't panic.
	// Actual subscription calls use client.SubscribeQuotes via the opend layer.
	_ = securities
	_ = context.Background()
}

func TestIsHKMarket(t *testing.T) {
	tests := []struct {
		name       string
		securities []*qotcommonpb.Security
		want       bool
	}{
		{
			name: "HK market",
			securities: []*qotcommonpb.Security{
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
			},
			want: true,
		},
		{
			name: "US market",
			securities: []*qotcommonpb.Security{
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")},
			},
			want: false,
		},
		{
			name: "mixed HK first",
			securities: []*qotcommonpb.Security{
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")},
			},
			want: true,
		},
		{
			name: "mixed US first",
			securities: []*qotcommonpb.Security{
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")},
				{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
			},
			want: true,
		},
		{
			name:       "empty list",
			securities: []*qotcommonpb.Security{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHKMarket(tt.securities); got != tt.want {
				t.Errorf("isHKMarket() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionRegistryOrderBook(t *testing.T) {
	r := newSubscriptionRegistry()

	key := "HK.00700"
	if r.hasOrderBook(key) {
		t.Error("expected false for fresh registry")
	}
	if r.hasOrderBookPush(key) {
		t.Error("expected false for fresh registry")
	}

	r.markOrderBook(key)
	if !r.hasOrderBook(key) {
		t.Error("expected true after markOrderBook")
	}
	if r.hasOrderBookPush(key) {
		t.Error("orderBook push should be independent of orderBook")
	}

	r.markOrderBookPush(key)
	if !r.hasOrderBookPush(key) {
		t.Error("expected true after markOrderBookPush")
	}
	r.unmarkOrderBook(key)
	r.unmarkOrderBookPush(key)
	if r.hasOrderBook(key) || r.hasOrderBookPush(key) {
		t.Fatal("order-book marks remained after unmark")
	}
}

func TestSubscriptionRegistryOrderBookReset(t *testing.T) {
	r := newSubscriptionRegistry()

	r.markOrderBook("HK.00700")
	r.markOrderBookPush("US.AAPL")
	r.reset()

	if r.hasOrderBook("HK.00700") {
		t.Error("expected false after reset")
	}
	if r.hasOrderBookPush("US.AAPL") {
		t.Error("expected false after reset")
	}
}

func TestSubscriptionRegistryOrderBookEnsure(t *testing.T) {
	r := subscriptionRegistry{}
	// Ensure should lazily initialize maps
	r.ensure()
	if r.orderBook == nil {
		t.Error("orderBook map should be initialized after ensure")
	}
	if r.orderBookPush == nil {
		t.Error("orderBookPush map should be initialized after ensure")
	}
}

func TestSubscriptionRegistryQuoteAndKLineFamiliesAreIndependent(t *testing.T) {
	r := subscriptionRegistry{}
	key := "US.AAPL"
	klineKey := "US.AAPL|1m"

	if r.hasBasicQot(key) || r.hasBasicQotPush(key) || r.hasKLine(klineKey) {
		t.Fatal("fresh lazily-initialized registry reported existing quote/kline subscriptions")
	}
	r.markBasicQot(key)
	if !r.hasBasicQot(key) || r.hasBasicQotPush(key) || r.hasKLine(klineKey) {
		t.Fatalf("basic quote mark leaked into other families: %#v", r)
	}
	r.markBasicQotPush(key)
	if !r.hasBasicQotPush(key) || r.hasKLine(klineKey) {
		t.Fatalf("basic quote push mark leaked into kline family: %#v", r)
	}
	r.markKLine(klineKey)
	if !r.hasKLine(klineKey) {
		t.Fatalf("kline mark was not recorded: %#v", r)
	}
}

func TestGroupOrderBookRequestsForPushSplitsHKAndNonHK(t *testing.T) {
	requests := []orderBookRequest{
		{
			canonical: "HK.00700",
			security: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
				Code:   new("00700"),
			},
		},
		{
			canonical: "US.AAPL",
			security: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
				Code:   new("AAPL"),
			},
		},
	}

	batches := groupOrderBookRequestsForPush(requests)
	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(batches))
	}
	if !batches[0].withDetail {
		t.Fatal("first batch should enable HK order-book detail")
	}
	if len(batches[0].requests) != 1 || batches[0].requests[0].canonical != "HK.00700" {
		t.Fatalf("unexpected HK batch: %#v", batches[0].requests)
	}
	if batches[1].withDetail {
		t.Fatal("second batch should not enable HK order-book detail")
	}
	if len(batches[1].requests) != 1 || batches[1].requests[0].canonical != "US.AAPL" {
		t.Fatalf("unexpected non-HK batch: %#v", batches[1].requests)
	}
}

func TestGroupOrderBookRequestsForPushSingleHKBatchNeedsDetail(t *testing.T) {
	requests := []orderBookRequest{
		{
			canonical: "HK.00700",
			security: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
				Code:   new("00700"),
			},
		},
	}

	batches := groupOrderBookRequestsForPush(requests)
	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want 1", len(batches))
	}
	if !batches[0].withDetail {
		t.Fatal("HK order-book batch should enable detail subscription")
	}
	if len(batches[0].requests) != 1 || batches[0].requests[0].canonical != "HK.00700" {
		t.Fatalf("unexpected HK batch: %#v", batches[0].requests)
	}
}

func TestEnsureOrderBookPushSubscriptionsSplitsDetailsAndDeduplicates(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	ex := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	hkSecurity, hkCanonical, err := futuSecurityFromSymbol("HK.00700")
	if err != nil {
		t.Fatalf("futuSecurityFromSymbol HK: %v", err)
	}
	usSecurity, usCanonical, err := futuSecurityFromSymbol("US.AAPL")
	if err != nil {
		t.Fatalf("futuSecurityFromSymbol US: %v", err)
	}
	requests := []orderBookRequest{
		{canonical: hkCanonical, security: hkSecurity},
		{canonical: usCanonical, security: usSecurity},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ex.withClient(ctx, func(client *opend.Client) error {
		return ex.ensureOrderBookPushSubscriptions(ctx, client, requests)
	}); err != nil {
		t.Fatalf("ensureOrderBookPushSubscriptions: %v", err)
	}

	if server.pushSubCallCount() != 2 {
		t.Fatalf("push subscription calls = %d, want split HK/US batches", server.pushSubCallCount())
	}
	if !ex.subscriptions.hasOrderBook(hkCanonical) || !ex.subscriptions.hasOrderBookPush(hkCanonical) ||
		!ex.subscriptions.hasOrderBook(usCanonical) || !ex.subscriptions.hasOrderBookPush(usCanonical) {
		t.Fatalf("subscription registry did not mark order-book push state: %#v", ex.subscriptions)
	}

	if err := ex.withClient(ctx, func(client *opend.Client) error {
		return ex.ensureOrderBookPushSubscriptions(ctx, client, requests)
	}); err != nil {
		t.Fatalf("second ensureOrderBookPushSubscriptions: %v", err)
	}
	if got := server.pushSubCallCount(); got != 2 {
		t.Fatalf("deduplicated push subscription call count = %d, want 2", got)
	}
}

func TestOrderBookSubscriptionLifecycleRequiresLeaseAndUnsubscribes(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()

	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if result, err := exchange.QueryOrderBook(ctx, "US.AAPL", 5); result != nil || !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("query without lease = %#v, %v", result, err)
	}
	if got := server.subCallCount(); got != 0 {
		t.Fatalf("query without lease sent %d subscription calls", got)
	}
	if err := exchange.SubscribeOrderBook(ctx, "BAD", false); err == nil {
		t.Fatal("SubscribeOrderBook invalid symbol error = nil")
	}
	if err := exchange.UnsubscribeOrderBook(ctx, "BAD"); err == nil {
		t.Fatal("UnsubscribeOrderBook invalid symbol error = nil")
	}

	if err := exchange.SubscribeOrderBook(ctx, "US.AAPL", false); err != nil {
		t.Fatalf("SubscribeOrderBook: %v", err)
	}
	if err := exchange.SubscribeOrderBook(ctx, "us.aapl", false); err != nil {
		t.Fatalf("duplicate SubscribeOrderBook: %v", err)
	}
	requests := server.capturedQotSubRequests()
	if len(requests) != 1 || !requests[0].GetIsSubOrUnSub() || requests[0].GetIsRegOrUnRegPush() {
		t.Fatalf("subscribe requests = %#v", requests)
	}
	if got := requests[0].GetSubTypeList(); len(got) != 1 || got[0] != int32(qotcommonpb.SubType_SubType_OrderBook) {
		t.Fatalf("subscribe subtype = %#v", got)
	}
	if err := exchange.SubscribeOrderBook(ctx, "US.AAPL", true); err != nil {
		t.Fatalf("SubscribeOrderBook push upgrade: %v", err)
	}
	if err := exchange.SubscribeOrderBook(ctx, "us.aapl", true); err != nil {
		t.Fatalf("duplicate SubscribeOrderBook push upgrade: %v", err)
	}
	requests = server.capturedQotSubRequests()
	if len(requests) != 2 || !requests[1].GetIsSubOrUnSub() || !requests[1].GetIsRegOrUnRegPush() {
		t.Fatalf("push subscribe requests = %#v", requests)
	}

	if err := exchange.UnsubscribeOrderBook(ctx, "US.AAPL"); err != nil {
		t.Fatalf("UnsubscribeOrderBook: %v", err)
	}
	if err := exchange.UnsubscribeOrderBook(ctx, "US.AAPL"); err != nil {
		t.Fatalf("duplicate UnsubscribeOrderBook: %v", err)
	}
	requests = server.capturedQotSubRequests()
	if len(requests) != 3 || requests[2].GetIsSubOrUnSub() || requests[2].GetIsRegOrUnRegPush() {
		t.Fatalf("unsubscribe requests = %#v", requests)
	}
	if exchange.subscriptions.hasOrderBook("US.AAPL") || exchange.subscriptions.hasOrderBookPush("US.AAPL") {
		t.Fatalf("registry retained released order book: %#v", exchange.subscriptions)
	}
	if result, err := exchange.QueryOrderBook(ctx, "US.AAPL", 5); result != nil || !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("query after release = %#v, %v", result, err)
	}

	if err := exchange.SubscribeOrderBook(ctx, "HK.00700", true); err != nil {
		t.Fatalf("SubscribeOrderBook HK: %v", err)
	}
	if err := exchange.UnsubscribeOrderBook(ctx, "HK.00700"); err != nil {
		t.Fatalf("UnsubscribeOrderBook HK: %v", err)
	}
	requests = server.capturedQotSubRequests()
	if len(requests) != 5 || !requests[4].GetIsSubOrderBookDetail() {
		t.Fatalf("HK unsubscribe detail request = %#v", requests)
	}

	if err := exchange.SubscribeOrderBook(ctx, "US.AAPL", false); err != nil {
		t.Fatalf("SubscribeOrderBook before disconnect: %v", err)
	}
	server.setDropProto(opend.ProtoQotSub)
	if err := exchange.UnsubscribeOrderBook(ctx, "US.AAPL"); err == nil {
		t.Fatal("UnsubscribeOrderBook disconnect error = nil")
	}
}

func TestHandleOrderBookPushEmitsSingleCompleteBookTicker(t *testing.T) {
	stream := NewStream(NewExchange(DefaultOpenDAddr))
	stream.ctx = context.Background()

	bookTickers := make(chan types.BookTicker, 2)
	stream.OnBookTickerUpdate(func(bookTicker types.BookTicker) {
		bookTickers <- bookTicker
	})

	stream.handleOrderBookPush(&qotupdateorderbookpb.S2C{
		Security: &qotcommonpb.Security{
			Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   new("00700"),
		},
		OrderBookBidList: []*qotcommonpb.OrderBook{
			{Price: new(700.1), Volume: new(int64(1200))},
		},
		OrderBookAskList: []*qotcommonpb.OrderBook{
			{Price: new(700.2), Volume: new(int64(800))},
		},
	})

	select {
	case ticker := <-bookTickers:
		if ticker.Symbol != "HK.00700" {
			t.Fatalf("Symbol = %q, want HK.00700", ticker.Symbol)
		}
		if ticker.Buy.Float64() != 700.1 || ticker.BuySize.Float64() != 1200 {
			t.Fatalf("unexpected bid side: %+v", ticker)
		}
		if ticker.Sell.Float64() != 700.2 || ticker.SellSize.Float64() != 800 {
			t.Fatalf("unexpected ask side: %+v", ticker)
		}
	default:
		t.Fatal("expected one book ticker update")
	}

	select {
	case ticker := <-bookTickers:
		t.Fatalf("unexpected extra book ticker update: %+v", ticker)
	default:
	}
}
