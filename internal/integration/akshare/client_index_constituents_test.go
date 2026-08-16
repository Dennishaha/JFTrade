package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientIndexConstituentsEncodesLimitAndDecodesNullableWeights(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "SH", "symbol": "000300", "instrument_id": "SH.000300",
			"constituents": []map[string]any{
				{"code": "600519", "name": "贵州茅台", "weight": nil},
				{"code": "300750", "name": "宁德时代", "weight": 3.21},
			},
			"source": "akshare-index-constituents",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.indexConstituents(context.Background(), "SH", "000300", 300)
	if err != nil {
		t.Fatalf("indexConstituents: %v", err)
	}
	if len(response.Constituents) != 2 || response.Constituents[0].Code != "600519" ||
		response.Constituents[0].Weight != nil || response.Constituents[1].Weight == nil ||
		response.Source != "akshare-index-constituents" {
		t.Fatalf("index constituents response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/index-constituents/SH/000300" ||
		seen[0].query.Get("limit") != "300" {
		t.Fatalf("index constituents request = %#v", seen)
	}
}

func TestClientIndexConstituentsMapsUnsupportedInstrumentToCapabilityError(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UNSUPPORTED","message":"HK indices are not covered by akshare"}}`))
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.indexConstituents(context.Background(), "HK", "HSI", 200)
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("unsupported index constituents error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("unsupported request retried: %d calls", len(requests()))
	}
}
