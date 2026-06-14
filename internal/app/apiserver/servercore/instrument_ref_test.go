package servercore

import (
	"strings"
	"testing"
)

func TestNormalizeInstrumentInput(t *testing.T) {
	tests := []struct {
		name          string
		market        string
		symbol        string
		code          string
		wantMarket    string
		wantPrefix    string
		wantCode      string
		wantSymbol    string
		wantErrSubstr string
	}{
		{
			name:       "qualified symbol",
			symbol:     "us:tme",
			wantMarket: "US",
			wantPrefix: "US",
			wantCode:   "TME",
			wantSymbol: "US.TME",
		},
		{
			name:       "explicit market with code",
			market:     "US",
			code:       "tme",
			wantMarket: "US",
			wantPrefix: "US",
			wantCode:   "TME",
			wantSymbol: "US.TME",
		},
		{
			name:       "explicit sh market with code",
			market:     "SH",
			code:       "600519",
			wantMarket: "CN",
			wantPrefix: "SH",
			wantCode:   "600519",
			wantSymbol: "SH.600519",
		},
		{
			name:          "bare symbol requires market",
			symbol:        "tme",
			wantErrSubstr: "market is required",
		},
		{
			name:          "cn market bare code is ambiguous",
			market:        "CN",
			code:          "600519",
			wantErrSubstr: "requires an exchange-qualified symbol",
		},
		{
			name:          "mismatched market and symbol",
			market:        "HK",
			symbol:        "US.TME",
			wantErrSubstr: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeInstrumentInput(tt.market, tt.symbol, tt.code)
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("normalizeInstrumentInput() error = %v, want substring %q", err, tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeInstrumentInput() error = %v", err)
			}
			if got.Market != tt.wantMarket || got.Prefix != tt.wantPrefix || got.Code != tt.wantCode || got.Symbol != tt.wantSymbol {
				t.Fatalf("normalizeInstrumentInput() = %+v, want market=%q prefix=%q code=%q symbol=%q", got, tt.wantMarket, tt.wantPrefix, tt.wantCode, tt.wantSymbol)
			}
		})
	}
}
