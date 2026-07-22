package futu

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsearchquotepb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsearchquote"
)

func TestFutuSearchMarketCodePreservesEveryStableDisplayMarket(t *testing.T) {
	tests := []struct {
		market qotcommonpb.QotMarket
		want   string
	}{
		{qotcommonpb.QotMarket_QotMarket_HK_Security, "HK"},
		{qotcommonpb.QotMarket_QotMarket_US_Security, "US"},
		{qotcommonpb.QotMarket_QotMarket_CNSH_Security, "SH"},
		{qotcommonpb.QotMarket_QotMarket_CNSZ_Security, "SZ"},
		{qotcommonpb.QotMarket_QotMarket_SG_Security, "SG"},
		{qotcommonpb.QotMarket_QotMarket_JP_Security, "JP"},
		{qotcommonpb.QotMarket_QotMarket_AU_Security, "AU"},
		{qotcommonpb.QotMarket_QotMarket_MY_Security, "MY"},
		{qotcommonpb.QotMarket_QotMarket_CA_Security, "CA"},
		{qotcommonpb.QotMarket_QotMarket_HK_Future, "HK_FUTURE"},
		{qotcommonpb.QotMarket_QotMarket_FX_Security, "FX"},
		{qotcommonpb.QotMarket_QotMarket_CC_Security, "CRYPTO"},
		{qotcommonpb.QotMarket(999), "UNKNOWN"},
	}
	for _, test := range tests {
		if got := futuSearchMarketCode(test.market); got != test.want {
			t.Errorf("futuSearchMarketCode(%d) = %q, want %q", test.market, got, test.want)
		}
	}
}

func TestCanonicalSearchQuoteSymbolHandlesOpenDPrefixedCodes(t *testing.T) {
	for _, test := range []struct {
		market string
		code   string
		want   string
	}{
		{"US", "US.AAPL", "US.AAPL"},
		{"US", "AAPL", "US.AAPL"},
		{"US", "BRK.B", "US.BRK.B"},
		{"US", "US.BRK.B", "US.BRK.B"},
		{"HK", "hk:00700", "HK.00700"},
		{"SH", "CNSH.600519", "SH.600519"},
		{"JP", "JP.7203", "JP.7203"},
	} {
		if got := canonicalSearchQuoteSymbol(test.market, test.code); got != test.want {
			t.Errorf("canonicalSearchQuoteSymbol(%q, %q) = %q, want %q", test.market, test.code, got, test.want)
		}
	}
}

func TestBrokerAdapterSecuritySearchMapsCrossMarketOpenDResults(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setSearchQuotes([]*qotgetsearchquotepb.SearchQuote{
		{
			Market:    new(int32(qotcommonpb.QotMarket_QotMarket_US_Security)),
			Code:      new("US.AAPL"),
			Name:      new(" Apple Inc. "),
			SecType:   new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
			IsWatched: new(true),
		},
		{
			Market:  new(int32(qotcommonpb.QotMarket_QotMarket_CNSH_Security)),
			Code:    new("CNSH.600519"),
			Name:    new("贵州茅台"),
			SecType: new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
		},
		nil,
		{
			Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   new(" "),
		},
	})
	defer server.stop()

	reader := newTestBrokerAdapter(t, server).MarketData()
	snapshot, err := reader.QuerySecuritySearch(t.Context(), broker.SecuritySearchQuery{
		ReadQuery: broker.ReadQuery{AccountID: "1001"},
		Keyword:   "  apple  ",
		Limit:     8,
	})
	if err != nil {
		t.Fatalf("QuerySecuritySearch: %v", err)
	}
	if snapshot == nil || snapshot.AccountID != "1001" {
		t.Fatalf("snapshot = %#v, want account 1001", snapshot)
		return
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries = %#v, want two usable cross-market results", snapshot.Entries)
	}
	if got := snapshot.Entries[0]; got.Market != "US" || got.Symbol != "US.AAPL" || got.Name != "Apple Inc." || got.SecurityType != "Eqty" || !got.IsWatched {
		t.Fatalf("US entry = %#v", got)
	}
	if got := snapshot.Entries[1]; got.Market != "SH" || got.Symbol != "SH.600519" || got.Name != "贵州茅台" || got.SecurityType != "Eqty" || got.IsWatched {
		t.Fatalf("SH entry = %#v", got)
	}
	keyword, maxCount := server.lastSearchQuoteRequest()
	if keyword != "apple" || maxCount != 8 {
		t.Fatalf("search request = (%q, %d), want (apple, 8)", keyword, maxCount)
	}
	if got := server.searchQuoteCalls.Load(); got != 1 {
		t.Fatalf("Qot_GetSearchQuote calls = %d, want 1", got)
	}
}

func TestBrokerAdapterSecuritySearchRejectsInvalidQueriesBeforeConnecting(t *testing.T) {
	reader := &futuMarketDataReader{}
	for _, query := range []broker.SecuritySearchQuery{
		{Keyword: " "},
		{Keyword: "AAPL", Limit: -1},
		{Keyword: "AAPL", Limit: 101},
	} {
		if _, err := reader.QuerySecuritySearch(t.Context(), query); err == nil || !strings.Contains(err.Error(), "QuerySecuritySearch") {
			t.Fatalf("QuerySecuritySearch(%+v) error = %v", query, err)
		}
	}
}
