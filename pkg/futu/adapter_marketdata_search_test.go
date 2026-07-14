package futu

import (
	"testing"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
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
