package akshare

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func writeIndexConstituentsFixture(writer http.ResponseWriter, market, symbol string) {
	_, _ = writer.Write([]byte(`{
		"market":"` + market + `","symbol":"` + symbol + `","instrument_id":"` + market + `.` + symbol + `",
		"constituents":[
			{"code":"600519","name":"贵州茅台","weight":null},
			{"code":"300750","name":"宁德时代","weight":3.21}
		],
		"source":"akshare-index-constituents"}`))
}

func TestProviderIndexConstituentsConvertsEntriesAndAppliesDefaultLimit(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeIndexConstituentsFixture(writer, "SH", "000300")
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.IndexConstituents(context.Background(), "cn", "SH.000300", 0)
	if err != nil {
		t.Fatalf("IndexConstituents: %v", err)
	}
	if response.InstrumentID != "SH.000300" || response.Market != "SH" || response.Symbol != "000300" ||
		response.Source != "akshare-index-constituents" || len(response.Constituents) != 2 {
		t.Fatalf("index constituents response = %#v", response)
	}
	if response.Constituents[0].Weight != nil || response.Constituents[1].Weight == nil ||
		response.Constituents[1].Weight.String() != "3.21" {
		t.Fatalf("constituent weights = %#v", response.Constituents)
	}
	if got := requests()[0].query.Get("limit"); got != "200" {
		t.Fatalf("default limit query = %q", got)
	}
}

func TestProviderIndexConstituentsRejectsMalformedPayloads(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"market":"SH","symbol":"000300","instrument_id":"SH.000300",
			"constituents":[{"code":"","name":"无名","weight":null}],
			"source":"akshare-index-constituents"}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.IndexConstituents(context.Background(), "SH", "000300", 5); !errors.Is(err, ErrInvalidResponse) ||
		!strings.Contains(err.Error(), "index constituent 0") {
		t.Fatalf("empty code error = %v", err)
	}
}

func TestProviderIndexConstituentsRejectsIdentityMismatch(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"market":"SH","symbol":"000001","instrument_id":"SH.000001",
			"constituents":[],"source":"akshare-index-constituents"}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.IndexConstituents(context.Background(), "SH", "000300", 5); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("identity mismatch error = %v", err)
	}
}

func TestProviderIndexConstituentsSurfacesUnsupportedMarkets(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(400)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UNSUPPORTED","message":"instrument is not a covered index"}}`))
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.IndexConstituents(context.Background(), "US", "SPX", 5); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("US index constituents error = %v", err)
	}
	if _, err := provider.IndexConstituents(context.Background(), "HK", "HSI", 5); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("HK index constituents error = %v", err)
	}
}

func TestProviderIndexConstituentsMeetsOptionalCapabilityContract(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	var _ marketdata.IndexConstituentsSource = provider
}
