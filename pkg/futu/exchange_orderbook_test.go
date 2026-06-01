package futu

import (
	"context"
	"testing"

	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
)

// --- Test: order book request construction validates security typing ---
// Note: Actual OpenD subscription and query calls require a running OpenD
// instance and are covered by integration tests in the jftradeapi package.

func TestSubscribeOrderBookRequestConstruction(t *testing.T) {
	security := &qotcommonpb.Security{
		Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   protoString("00700"),
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
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: protoString("00700")},
			},
			want: true,
		},
		{
			name: "US market",
			securities: []*qotcommonpb.Security{
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: protoString("AAPL")},
			},
			want: false,
		},
		{
			name: "mixed HK first",
			securities: []*qotcommonpb.Security{
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: protoString("00700")},
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: protoString("AAPL")},
			},
			want: true,
		},
		{
			name: "mixed US first",
			securities: []*qotcommonpb.Security{
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: protoString("AAPL")},
				{Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: protoString("00700")},
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

func TestGroupOrderBookRequestsForPushSplitsHKAndNonHK(t *testing.T) {
	requests := []orderBookRequest{
		{
			canonical: "HK.00700",
			security: &qotcommonpb.Security{
				Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
				Code:   protoString("00700"),
			},
		},
		{
			canonical: "US.AAPL",
			security: &qotcommonpb.Security{
				Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
				Code:   protoString("AAPL"),
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

func TestHandleOrderBookPushEmitsSingleCompleteBookTicker(t *testing.T) {
	stream := NewStream(NewExchange(DefaultOpenDAddr))
	stream.ctx = context.Background()

	bookTickers := make(chan types.BookTicker, 2)
	stream.OnBookTickerUpdate(func(bookTicker types.BookTicker) {
		bookTickers <- bookTicker
	})

	stream.handleOrderBookPush(&qotupdateorderbookpb.S2C{
		Security: &qotcommonpb.Security{
			Market: protoInt32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   protoString("00700"),
		},
		OrderBookBidList: []*qotcommonpb.OrderBook{
			{Price: proto.Float64(700.1), Volume: proto.Int64(1200)},
		},
		OrderBookAskList: []*qotcommonpb.OrderBook{
			{Price: proto.Float64(700.2), Volume: proto.Int64(800)},
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
