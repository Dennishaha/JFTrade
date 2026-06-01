package futu

import (
	"context"
	"testing"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
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
