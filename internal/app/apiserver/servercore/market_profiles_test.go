package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarketProfilesEndpoint(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/markets")
	if err != nil {
		t.Fatalf("GET markets: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			DefaultMarket string             `json:"defaultMarket"`
			Markets       []marketProfileDTO `json:"markets"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK || envelope.Data.DefaultMarket != "HK" {
		t.Fatalf("envelope = %#v", envelope)
	}
	byCode := make(map[string]marketProfileDTO, len(envelope.Data.Markets))
	for _, profile := range envelope.Data.Markets {
		byCode[profile.Code] = profile
	}
	if !byCode["US"].SupportsExtendedHours || byCode["US"].QuoteCurrency != "USD" {
		t.Fatalf("US profile = %#v", byCode["US"])
	}
	if byCode["HK"].SupportsExtendedHours || byCode["HK"].Precision.Price != 3 {
		t.Fatalf("HK profile = %#v", byCode["HK"])
	}
	if !byCode["CN"].RequiresExchangePrefix {
		t.Fatalf("CN profile = %#v", byCode["CN"])
	}
}

func TestNormalizeMarketInstrumentEndpoint(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	cases := []struct {
		name         string
		body         string
		wantMarket   string
		wantPrefix   string
		wantCode     string
		wantSymbol   string
		wantHTTPCode int
		wantErrPart  string
	}{
		{"us", `{"market":"US","code":"aapl"}`, "US", "US", "AAPL", "US.AAPL", http.StatusOK, ""},
		{"hk colon", `{"instrumentId":"hk:00700"}`, "HK", "HK", "00700", "HK.00700", http.StatusOK, ""},
		{"sh", `{"market":"SH","code":"600519"}`, "CN", "SH", "600519", "SH.600519", http.StatusOK, ""},
		{"sz", `{"symbol":"SZ.000001"}`, "CN", "SZ", "000001", "SZ.000001", http.StatusOK, ""},
		{"cn bare rejected", `{"market":"CN","code":"600519"}`, "", "", "", "", http.StatusBadRequest, "requires an exchange-qualified symbol"},
		{"cn qualified rejected", `{"instrumentId":"CN.600519"}`, "", "", "", "", http.StatusBadRequest, "requires an exchange-qualified symbol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/market-data/instruments/normalize", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("POST normalize: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
			if resp.StatusCode != tc.wantHTTPCode {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantHTTPCode)
			}
			if tc.wantHTTPCode != http.StatusOK {
				var envelope struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if !strings.Contains(envelope.Error.Message, tc.wantErrPart) {
					t.Fatalf("error message = %q, want containing %q", envelope.Error.Message, tc.wantErrPart)
				}
				return
			}
			var envelope struct {
				OK   bool                              `json:"ok"`
				Data normalizeMarketInstrumentResponse `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !envelope.OK {
				t.Fatal("expected ok envelope")
			}
			if envelope.Data.Market != tc.wantMarket || envelope.Data.Prefix != tc.wantPrefix || envelope.Data.Code != tc.wantCode || envelope.Data.Symbol != tc.wantSymbol || envelope.Data.InstrumentID != tc.wantSymbol {
				t.Fatalf("normalize response = %#v", envelope.Data)
			}
		})
	}
}
