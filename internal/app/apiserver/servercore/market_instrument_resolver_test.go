package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestBrokerSearchInstrumentPartsPreservesDottedCodes(t *testing.T) {
	for _, test := range []struct {
		market string
		symbol string
		want   []string
	}{
		{market: "US", symbol: "US.BRK.B", want: []string{"US", "BRK.B"}},
		{market: "US", symbol: "BRK.B", want: []string{"US", "BRK.B"}},
		{market: "SH", symbol: "CNSH.600519", want: []string{"SH", "600519"}},
	} {
		marketCode, code := brokerSearchInstrumentParts(test.market, test.symbol)
		if got := []string{marketCode, code}; !slices.Equal(got, test.want) {
			t.Errorf("brokerSearchInstrumentParts(%q, %q) = %#v, want %#v", test.market, test.symbol, got, test.want)
		}
	}
}

func TestMarketInstrumentResolverEndpointUsesSearchWithoutQuoteSubscription(t *testing.T) {
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
		if entry.Name == "" || entry.SecurityType == "" || entry.Source != "bbgo:futu-search" || !entry.Selectable {
			t.Fatalf("search candidate = %+v", entry)
		}
	}
	if got := quoteServer.searchQuoteCallCount(); got != 1 {
		t.Fatalf("GetSearchQuote calls = %d, want 1", got)
	}
	if got := quoteServer.staticInfoCallCount(); got != 0 {
		t.Fatalf("GetStaticInfo calls = %d, unqualified input must use search", got)
	}
	if got := quoteServer.qotSubCallCount(); got != 0 {
		t.Fatalf("QotSub calls = %d, search must not request a subscription", got)
	}
	if got := quoteServer.basicQotCallCount(); got != 0 {
		t.Fatalf("GetBasicQot calls = %d, search must not request quote data", got)
	}
	if got := quoteServer.securitySnapshotCallCount(); got != 0 {
		t.Fatalf("GetSecuritySnapshot calls = %d, search must not request snapshots", got)
	}
}

func TestMarketInstrumentResolverQualifiedInputOnlyQueriesSelectedLeaf(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.addr)
	result, err := server.marketdataSvc.ResolveInstrument(t.Context(), "CN", "SH.600519", 20)
	if err != nil {
		t.Fatalf("ResolveInstrument: %v", err)
	}
	if result.ResolutionStatus != mdsrv.InstrumentResolutionResolved || result.TotalReturned != 1 || result.Entries[0].InstrumentID != "SH.600519" {
		t.Fatalf("qualified resolution = %+v", result)
	}
	if got := quoteServer.staticInfoCallCount(); got != 1 {
		t.Fatalf("GetStaticInfo calls = %d, want one qualified leaf lookup", got)
	}
	if quoteServer.searchQuoteCallCount() != 0 || quoteServer.qotSubCallCount() != 0 {
		t.Fatal("qualified static lookup unexpectedly used search or quote subscription APIs")
	}
	if quoteServer.basicQotCallCount() != 0 || quoteServer.securitySnapshotCallCount() != 0 {
		t.Fatal("qualified static lookup unexpectedly used a quote or snapshot API")
	}
}
