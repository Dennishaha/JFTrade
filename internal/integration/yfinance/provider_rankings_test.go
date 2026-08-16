package yfinance

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientRankingsEncodesMarketKindAndLimit(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/rankings", testkit.Response{Body: `{
		"market":"US","kind":"active",
		"entries":[{"instrument_id":"us.aapl","name":"Apple Inc.","price":232.1,"change_rate":1.25,
			"change_amount":2.86,"volume":5.5e7,"turnover":null,"turnover_ratio":null,
			"pe_ttm":31.2,"market_cap":3.5e12}],
		"source":"yfinance-rankings"}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.rankings(context.Background(), "US", "active", 25)
	if err != nil {
		t.Fatalf("rankings: %v", err)
	}
	if response.Kind != "active" || len(response.Entries) != 1 ||
		response.Entries[0].PETTM == nil || response.Entries[0].Turnover != nil {
		t.Fatalf("rankings response = %#v", response)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].Path != "/providers/yfinance/rankings" ||
		requests[0].Query.Get("market") != "US" || requests[0].Query.Get("kind") != "active" ||
		requests[0].Query.Get("limit") != "25" {
		t.Fatalf("rankings request = %#v", requests)
	}
}

func TestProviderRankingsConvertsEntriesAndAppliesDefaultLimit(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/rankings", testkit.Response{Body: `{
		"market":"US","kind":"gainers",
		"entries":[{"instrument_id":"us.aapl","name":" Apple Inc. ","price":232.1,"change_rate":1.25}],
		"source":""}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Rankings(context.Background(), "us", " Gainers ", 0)
	if err != nil {
		t.Fatalf("Rankings: %v", err)
	}
	if response.Market != "US" || response.Kind != "gainers" || response.Source != "yfinance-rankings" ||
		len(response.Entries) != 1 {
		t.Fatalf("rankings response = %#v", response)
	}
	entry := response.Entries[0]
	if entry.InstrumentID != "US.AAPL" || entry.Name != "Apple Inc." ||
		entry.ChangeRate == nil || *entry.ChangeRate != "1.25" {
		t.Fatalf("entry = %#v", entry)
	}
	requests := server.Requests()
	if got := requests[0].Query.Get("limit"); got != "20" {
		t.Fatalf("default limit query = %q", got)
	}
}

func TestProviderRankingsRejectsNonUSMarketsWithoutSidecarCall(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	for _, market := range []string{"HK", "SH", "SZ", "CN"} {
		if _, err := provider.Rankings(ctx, market, "gainers", 20); !errors.Is(err, ErrUnsupported) ||
			!errors.Is(err, marketdata.ErrCapabilityUnsupported) {
			t.Fatalf("rankings market %s error = %v", market, err)
		}
	}
	if _, err := provider.Rankings(ctx, "US", "breakout", 20); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown kind error = %v", err)
	}
	if len(server.Requests()) != 0 {
		t.Fatalf("rejected requests reached the sidecar: %#v", server.Requests())
	}
}

func TestProviderRankingsRejectsKindMismatch(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/rankings", testkit.Response{Body: `{
		"market":"US","kind":"losers","entries":[],"source":"yfinance-rankings"}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Rankings(context.Background(), "US", "gainers", 20); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("kind mismatch error = %v", err)
	}
}
