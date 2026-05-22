package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

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
				Open   float64 `json:"open"`
				High   float64 `json:"high"`
				Low    float64 `json:"low"`
				Close  float64 `json:"close"`
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
	if envelope.Data.Candles[1].Open != 101 || envelope.Data.Candles[1].High != 106 || envelope.Data.Candles[1].Low != 99 || envelope.Data.Candles[1].Close != 103 {
		t.Fatalf("unexpected current candle OHLC: %+v", envelope.Data.Candles[1])
	}
	if envelope.Data.Candles[1].Volume != 500 {
		t.Fatalf("current candle volume = %v, want 500", envelope.Data.Candles[1].Volume)
	}
}

func TestMarketSnapshotResponseUsesFreshCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID:       instrumentID,
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("321.4"),
		Bid:                decimal.RequireFromString("321.3"),
		Ask:                decimal.RequireFromString("321.5"),
		PreviousClosePrice: decimalPtr(float64Ptr(318.9)),
		Volume:             1282100,
		Turnover:           411020000,
		QuoteAt:            now.Format(time.RFC3339Nano),
		ObservedAt:         now.Format(time.RFC3339Nano),
		Source:             "bbgo:futu:stream",
		Session:            "regular",
	})

	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, instrumentID, true, "bbgo:futu:stream")
	if got := response["snapshot"].(map[string]any)["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("snapshot at = %v", got)
	}
}

func TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := response["snapshot"].(map[string]any)["price"]; got != json.Number("321.4") {
		t.Fatalf("snapshot price = %v", got)
	}
}

func TestMarketSnapshotResponseForceRefreshBypassesCache(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("999.9"),
		Bid:          decimal.RequireFromString("999.8"),
		Ask:          decimal.RequireFromString("1000.0"),
		Volume:       1,
		QuoteAt:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		ObservedAt:   time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := server.marketSnapshotResponse(
		t.Context(),
		"/api/v1/market-data/snapshots/HK/00700",
		map[string][]string{"refresh": {"true"}},
	)
	if err != nil {
		t.Fatalf("marketSnapshotResponse: %v", err)
	}

	assertSnapshotResponse(t, response, "HK.00700", false, "bbgo:futu")
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one forced GetBasicQot call, got %d", got)
	}
	if got := response["snapshot"].(map[string]any)["price"]; got != json.Number("321.4") {
		t.Fatalf("forced refresh snapshot price = %v", got)
	}
}

func TestMarketSecurityDetailsResponseQueriesSecuritySnapshot(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	response, err := server.marketSecurityDetailsResponse(
		t.Context(),
		"/api/v1/market-data/securities/HK/00700",
	)
	if err != nil {
		t.Fatalf("marketSecurityDetailsResponse: %v", err)
	}

	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	if got := request["instrumentId"]; got != "HK.00700" {
		t.Fatalf("instrumentId = %v", got)
	}
	security, ok := response["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", response["security"])
	}
	if got := security["name"]; got != "Tencent Holdings" {
		t.Fatalf("security name = %v", got)
	}
	if got := security["exchangeType"]; got != "HK_HKEX" {
		t.Fatalf("exchangeType = %v", got)
	}
	if got := security["currentPrice"]; got != json.Number("321.4") {
		t.Fatalf("currentPrice = %v", got)
	}
	equity, ok := security["equity"].(map[string]any)
	if !ok {
		t.Fatalf("equity payload type = %T", security["equity"])
	}
	if got := equity["peRate"]; got != json.Number("16.7") {
		t.Fatalf("peRate = %v", got)
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != false {
		t.Fatalf("fromCache = %v", got)
	}
	if got := quoteServer.securitySnapshotCallCount(); got != 1 {
		t.Fatalf("expected one GetSecuritySnapshot call, got %d", got)
	}
	if got := quoteServer.staticInfoCallCount(); got != 1 {
		t.Fatalf("expected one GetStaticInfo call, got %d", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesWarrantBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/21164")
	warrant := assertSecurityTypedBlock(t, security, "warrant")
	if got := security["securityType"]; got != "Warrant" {
		t.Fatalf("securityType = %v", got)
	}
	if got := warrant["warrantType"]; got != "Bull" {
		t.Fatalf("warrantType = %v", got)
	}
	owner, ok := warrant["owner"].(map[string]any)
	if !ok {
		t.Fatalf("owner payload type = %T", warrant["owner"])
	}
	if got := owner["instrumentId"]; got != "HK.00700" {
		t.Fatalf("owner instrumentId = %v", got)
	}
	if got := warrant["issuerCode"]; got != "SG" {
		t.Fatalf("issuerCode = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesOptionBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/US/AAPL250117C00200000")
	option := assertSecurityTypedBlock(t, security, "option")
	if got := security["securityType"]; got != "Drvt" {
		t.Fatalf("securityType = %v", got)
	}
	if got := option["optionType"]; got != "Call" {
		t.Fatalf("optionType = %v", got)
	}
	owner, ok := option["owner"].(map[string]any)
	if !ok {
		t.Fatalf("owner payload type = %T", option["owner"])
	}
	if got := owner["instrumentId"]; got != "US.AAPL" {
		t.Fatalf("owner instrumentId = %v", got)
	}
	if got := option["expiryDateDistance"]; got != int32(45) {
		t.Fatalf("expiryDateDistance = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesFutureBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/HSIMAIN")
	future := assertSecurityTypedBlock(t, security, "future")
	if got := security["securityType"]; got != "Future" {
		t.Fatalf("securityType = %v", got)
	}
	if got := future["isMainContract"]; got != true {
		t.Fatalf("isMainContract = %v", got)
	}
	if got := future["position"]; got != int32(182233) {
		t.Fatalf("position = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesTrustBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/US/SPY")
	trust := assertSecurityTypedBlock(t, security, "trust")
	if got := security["securityType"]; got != "Trust" {
		t.Fatalf("securityType = %v", got)
	}
	if got := trust["assetClass"]; got != "Stock" {
		t.Fatalf("assetClass = %v", got)
	}
	if got := trust["aum"]; got != json.Number("580000000000") {
		t.Fatalf("aum = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesIndexBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/HSI")
	index := assertSecurityTypedBlock(t, security, "index")
	if got := security["securityType"]; got != "Index" {
		t.Fatalf("securityType = %v", got)
	}
	if got := index["raiseCount"]; got != int32(58) {
		t.Fatalf("raiseCount = %v", got)
	}
	if got := index["fallCount"]; got != int32(21) {
		t.Fatalf("fallCount = %v", got)
	}
}

func TestMarketSecurityDetailsResponseIncludesPlateBlock(t *testing.T) {
	security := marketSecurityDetailsResponseForPath(t, "/api/v1/market-data/securities/HK/TECH")
	plate := assertSecurityTypedBlock(t, security, "plate")
	if got := security["securityType"]; got != "Plate" {
		t.Fatalf("securityType = %v", got)
	}
	if got := plate["raiseCount"]; got != int32(42) {
		t.Fatalf("raiseCount = %v", got)
	}
	if got := plate["equalCount"]; got != int32(5) {
		t.Fatalf("equalCount = %v", got)
	}
}

func TestMarketCandlesTickResponseUsesFreshCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)

	instrumentID := "HK.00700"
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       1282100,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:stream",
		Session:      "regular",
	})

	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := response["candles"].([]map[string]any)[0]["at"]; got != now.Format(time.RFC3339Nano) {
		t.Fatalf("tick candle at = %v", got)
	}
}

func TestMarketCandlesTickResponseQueriesTickerOnCacheMiss(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	response, err := server.marketCandlesResponse(
		t.Context(),
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse: %v", err)
	}

	assertTickCandlesResponse(t, response, "HK.00700", false, 1)
	if got := quoteServer.basicQotCallCount(); got != 1 {
		t.Fatalf("expected one GetBasicQot call, got %d", got)
	}
	if got := response["candles"].([]map[string]any)[0]["period"]; got != "tick" {
		t.Fatalf("tick candle period = %v", got)
	}
}

func TestMarketCandlesTickResponseFallsBackToCachedCandlesOnTickerError(t *testing.T) {
	server := newMarketDataTestServerWithQuoteRuntime(t, "127.0.0.1:1")

	instrumentID := "HK.00700"
	observedAt := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Second)
	seedCachedTickSample(server, marketTickSample{
		InstrumentID: instrumentID,
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       1282100,
		QuoteAt:      observedAt.Format(time.RFC3339Nano),
		ObservedAt:   observedAt.Format(time.RFC3339Nano),
		Source:       "bbgo:futu:fallback",
		Session:      "regular",
	})

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	response, err := server.marketCandlesResponse(
		ctx,
		"/api/v1/market-data/candles/HK/00700",
		map[string][]string{"period": {"tick"}, "limit": {"2"}},
	)
	if err != nil {
		t.Fatalf("marketCandlesResponse fallback: %v", err)
	}

	assertTickCandlesResponse(t, response, instrumentID, true, 1)
	if got := response["candles"].([]map[string]any)[0]["at"]; got != observedAt.Format(time.RFC3339Nano) {
		t.Fatalf("fallback tick candle at = %v", got)
	}
}

func marketSecurityDetailsResponseForPath(t *testing.T, path string) map[string]any {
	t.Helper()
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	response, err := server.marketSecurityDetailsResponse(t.Context(), path)
	if err != nil {
		t.Fatalf("marketSecurityDetailsResponse(%s): %v", path, err)
	}
	security, ok := response["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", response["security"])
	}
	return security
}

func assertSecurityTypedBlock(t *testing.T, security map[string]any, key string) map[string]any {
	t.Helper()
	typed, ok := security[key].(map[string]any)
	if !ok {
		t.Fatalf("%s payload type = %T", key, security[key])
	}
	return typed
}
