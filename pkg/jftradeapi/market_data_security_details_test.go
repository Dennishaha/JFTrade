package jftradeapi

import (
	"bufio"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if got := security["currentPrice"]; got != "321.4" {
		t.Fatalf("currentPrice = %v", got)
	}
	equity, ok := security["equity"].(map[string]any)
	if !ok {
		t.Fatalf("equity payload type = %T", security["equity"])
	}
	if got := equity["peRate"]; got != "16.7" {
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

func TestMarketSecurityDetailsSSEStreamSendsInitialPayload(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	srv := httptest.NewServer(server)
	defer srv.Close()

	response, err := liveSSERequest(t, srv.URL+"/api/v1/market-data/securities/HK/00700")
	if err != nil {
		t.Fatalf("GET market security details SSE: %v", err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}

	event := readSSEEvent(t, bufio.NewReader(response.Body))
	request, ok := event["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", event["request"])
	}
	if got := request["instrumentId"]; got != "HK.00700" {
		t.Fatalf("instrumentId = %v", got)
	}
	security, ok := event["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", event["security"])
	}
	if got := security["name"]; got != "Tencent Holdings" {
		t.Fatalf("security name = %v", got)
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
	if got := trust["aum"]; got != "580000000000" {
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
