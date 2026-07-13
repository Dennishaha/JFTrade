package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestMarketInstrumentResolverEndpointUsesStaticInfoWithoutQuoteSubscription(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	response, err := jftradeTestHTTPGet(t, httpServer.URL+"/api/v1/market-data/instruments?market=CN&query=000001")
	if err != nil {
		t.Fatalf("GET instrument resolution: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET instrument resolution status = %d", response.StatusCode)
	}
	var envelope struct {
		OK   bool                       `json:"ok"`
		Data mdsrv.InstrumentResolution `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode instrument resolution: %v", err)
	}
	if !envelope.OK || envelope.Data.ResolutionStatus != mdsrv.InstrumentResolutionAmbiguous || envelope.Data.TotalReturned != 2 {
		t.Fatalf("instrument resolution = %+v", envelope)
	}
	if envelope.Data.Entries[0].InstrumentID != "SH.000001" || envelope.Data.Entries[0].ResolvedMarket != "CN" ||
		envelope.Data.Entries[1].InstrumentID != "SZ.000001" {
		t.Fatalf("instrument candidates = %+v", envelope.Data.Entries)
	}
	for _, entry := range envelope.Data.Entries {
		if entry.Name != "Tencent Holdings" || entry.SecurityType != "Eqty" || entry.LotSize != 100 || entry.Source != "bbgo:futu" {
			t.Fatalf("static-info candidate = %+v", entry)
		}
	}
	if got := quoteServer.staticInfoCallCount(); got != 2 {
		t.Fatalf("GetStaticInfo calls = %d, want one per CN leaf", got)
	}
	if got := quoteServer.basicQotCallCount(); got != 0 {
		t.Fatalf("GetBasicQot calls = %d, exact lookup must not request subscribed quotes", got)
	}
	if got := quoteServer.securitySnapshotCallCount(); got != 0 {
		t.Fatalf("GetSecuritySnapshot calls = %d, exact lookup should use static info", got)
	}
}

func TestMarketInstrumentResolverQualifiedInputOnlyQueriesSelectedLeaf(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	result, err := server.marketdataSvc.ResolveInstrument(t.Context(), "CN", "SH.600519")
	if err != nil {
		t.Fatalf("ResolveInstrument: %v", err)
	}
	if result.ResolutionStatus != mdsrv.InstrumentResolutionResolved || result.TotalReturned != 1 || result.Entries[0].InstrumentID != "SH.600519" {
		t.Fatalf("qualified resolution = %+v", result)
	}
	if got := quoteServer.staticInfoCallCount(); got != 1 {
		t.Fatalf("GetStaticInfo calls = %d, want one qualified leaf lookup", got)
	}
	if quoteServer.basicQotCallCount() != 0 || quoteServer.securitySnapshotCallCount() != 0 {
		t.Fatal("qualified static lookup unexpectedly used a quote or snapshot API")
	}
}
