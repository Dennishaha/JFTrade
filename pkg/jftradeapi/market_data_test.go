package jftradeapi

import (
	"testing"
	"time"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setHistoryPagesBySession(map[int32][][]*qotcommonpb.KLine{
		int32(commonpb.Session_Session_RTH): {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 10, 0, 0, 0, time.UTC), 110, 111, 109, 110.5, 1000)},
		},
		int32(commonpb.Session_Session_ETH): {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 21, 0, 0, 0, time.UTC), 120, 121, 119, 120.5, 1000)},
		},
		int32(commonpb.Session_Session_ALL): {
			{testMarketDataProtoKLine(time.Date(2026, time.May, 20, 2, 0, 0, 0, time.UTC), 90, 91, 89, 90.5, 1000)},
		},
	})

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	defer server.Close()
	start := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 20, 23, 0, 0, 0, time.UTC)
	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/US/NVDA",
		map[string][]string{
			"period":   {"1m"},
			"limit":    {"3"},
			"fromTime": {start.Format(time.RFC3339Nano)},
			"toTime":   {end.Format(time.RFC3339Nano)},
		},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != 3 {
		t.Fatalf("len(candles) = %d, want 3", len(candles))
	}
	sessionsByOpen := make(map[string]string, len(candles))
	for _, candle := range candles {
		open, ok := candle["open"].(string)
		if !ok {
			t.Fatalf("open payload type = %T", candle["open"])
		}
		session, ok := candle["session"].(string)
		if !ok {
			t.Fatalf("session payload type = %T", candle["session"])
		}
		sessionsByOpen[open] = session
	}
	if got := sessionsByOpen["90"]; got != "overnight" {
		t.Fatalf("overnight candle session = %q, want overnight", got)
	}
	if got := sessionsByOpen["110"]; got != "regular" {
		t.Fatalf("RTH-routed candle session = %q, want regular", got)
	}
	if got := sessionsByOpen["120"]; got != "after" {
		t.Fatalf("ETH-routed candle session = %q, want after", got)
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["session"]; got != "all" {
		t.Fatalf("meta session = %v, want all", got)
	}
	if got := meta["extendedHours"]; got != true {
		t.Fatalf("extendedHours = %v, want true", got)
	}
}

func TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	labelAt := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	quoteServer.setHistoryPages([][]*qotcommonpb.KLine{{
		testMarketDataProtoKLine(labelAt, 100, 101, 99, 100.5, 1000),
	}})

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	defer server.Close()
	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/US/NVDA",
		map[string][]string{
			"period":   {"1d"},
			"limit":    {"1"},
			"fromTime": {labelAt.Add(-24 * time.Hour).Format(time.RFC3339Nano)},
			"toTime":   {labelAt.Add(24 * time.Hour).Format(time.RFC3339Nano)},
		},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != 1 {
		t.Fatalf("len(candles) = %d, want 1", len(candles))
	}
	if _, exists := candles[0]["session"]; exists {
		t.Fatalf("expected daily candle to omit session, got %v", candles[0]["session"])
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if _, exists := meta["session"]; exists {
		t.Fatalf("expected daily candle meta to omit session, got %v", meta["session"])
	}
	if got := meta["extendedHours"]; got != false {
		t.Fatalf("extendedHours = %v, want false", got)
	}
}
