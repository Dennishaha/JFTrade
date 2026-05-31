package jftradeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestMarketCandlesEndpointIncludesCurrentRealtimeBucket(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	historyLabelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	currentLabelAt := historyLabelAt.Add(time.Minute)
	quoteServer.setHistoryPages([][]*qotcommonpb.KLine{{
		testMarketDataProtoKLine(historyLabelAt, 100, 101, 99, 100.5, 1000),
	}})
	quoteServer.setCurrentKLines([]*qotcommonpb.KLine{
		testMarketDataProtoKLine(currentLabelAt, 101, 106, 99, 103, 500),
	})

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	requestURL := fmt.Sprintf(
		"%s/api/v1/market-data/candles/HK/00700?period=1m&limit=2&fromTime=%s&toTime=%s",
		srv.URL,
		url.QueryEscape(historyLabelAt.Add(-time.Hour).Format(time.RFC3339Nano)),
		url.QueryEscape(currentLabelAt.Add(30*time.Second).Format(time.RFC3339Nano)),
	)
	resp, err := http.Get(requestURL)
	if err != nil {
		t.Fatalf("GET market candles: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET market candles status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Candles []struct {
				Period string  `json:"period"`
				Open   string  `json:"open"`
				High   string  `json:"high"`
				Low    string  `json:"low"`
				Close  string  `json:"close"`
				Volume float64 `json:"volume"`
				At     string  `json:"at"`
			} `json:"candles"`
			TotalReturned int `json:"totalReturned"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode market candles: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := quoteServer.currentKLCallCount(); got != 1 {
		t.Fatalf("expected one GetKL call, got %d", got)
	}
	if got := len(envelope.Data.Candles); got != 2 {
		t.Fatalf("expected two candles, got %d", got)
	}
	if envelope.Data.TotalReturned != 2 {
		t.Fatalf("totalReturned = %d, want 2", envelope.Data.TotalReturned)
	}

	if got := envelope.Data.Candles[0].At; got != historyLabelAt.Add(-time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("first candle at = %s", got)
	}
	if got := envelope.Data.Candles[1].At; got != historyLabelAt.Format(time.RFC3339Nano) {
		t.Fatalf("current candle at = %s", got)
	}
	if envelope.Data.Candles[1].Period != "1m" {
		t.Fatalf("current candle period = %s", envelope.Data.Candles[1].Period)
	}
	if envelope.Data.Candles[1].Open != "101" || envelope.Data.Candles[1].High != "106" || envelope.Data.Candles[1].Low != "99" || envelope.Data.Candles[1].Close != "103" {
		t.Fatalf("unexpected current candle OHLC: %+v", envelope.Data.Candles[1])
	}
	if envelope.Data.Candles[1].Volume != 500 {
		t.Fatalf("current candle volume = %v, want 500", envelope.Data.Candles[1].Volume)
	}
}
