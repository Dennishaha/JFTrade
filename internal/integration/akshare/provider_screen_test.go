package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientScreenPostsConditionSortAndPagingBody(t *testing.T) {
	var captured string
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/providers/akshare/screen" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(request.Body)
		captured = string(body)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"entries": []map[string]any{{
				"instrument_id": "SH.600519", "name": "贵州茅台", "symbol": "600519",
				"industry": "白酒", "quote_currency": "CNY",
				"values": map[string]any{"simple.price": 1500.5, "simple.pe_ttm": 24.3},
			}},
			"total": 1, "has_more": false, "as_of": "2026-08-15T08:00:00Z", "source": "akshare-screen",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.screen(context.Background(), remoteScreenRequest{
		Market: "CN",
		Conditions: []remoteScreenCondition{
			{FactorKey: "simple.market_cap", Min: json.Number("100000000000")},
			{FactorKey: "simple.change_pct", Min: json.Number("-5"), Max: json.Number("5")},
		},
		Sorts:  []remoteScreenSort{{FactorKey: "simple.volume", Direction: "desc"}},
		Offset: 50,
		Limit:  25,
	})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if response.Total != 1 || len(response.Entries) != 1 || response.Source != "akshare-screen" {
		t.Fatalf("response = %#v", response)
	}

	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/screen" {
		t.Fatalf("requests = %#v", seen)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(captured), &body); err != nil {
		t.Fatalf("request body is not JSON: %q", captured)
	}
	if body["market"] != "CN" || body["offset"] != float64(50) || body["limit"] != float64(25) {
		t.Fatalf("request body = %v", body)
	}
	conditions, ok := body["conditions"].([]any)
	if !ok || len(conditions) != 2 {
		t.Fatalf("conditions = %v", body["conditions"])
	}
	first, ok := conditions[0].(map[string]any)
	if !ok || first["factor_key"] != "simple.market_cap" {
		t.Fatalf("condition[0] = %v", conditions[0])
	}
	if _, oneSided := first["max"]; oneSided {
		t.Fatalf("one-sided condition must omit max: %v", conditions[0])
	}
	sorts, ok := body["sorts"].([]any)
	if !ok || len(sorts) != 1 || sorts[0].(map[string]any)["direction"] != "desc" {
		t.Fatalf("sorts = %v", body["sorts"])
	}
}

func TestProviderScreenConvertsCNEntriesAndDerivesSymbol(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"entries": []map[string]any{
				{
					"instrument_id": "sh.600519", "name": " 贵州茅台 ", "symbol": nil,
					"industry": nil, "quote_currency": " cny ",
					"values": map[string]any{"simple.price": 1500.5},
				},
				{
					"instrument_id": "SZ.000001", "name": "平安银行", "symbol": "000001",
					"industry": "银行", "quote_currency": "CNY", "values": map[string]any{},
				},
			},
			"total": 9, "has_more": true, "as_of": "2026-08-15T08:00:00Z",
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Screen(context.Background(), marketdata.ScreenRequest{
		Market: "cn",
		Conditions: []marketdata.ScreenConditionRequest{
			{FactorKey: "simple.price", Min: number("100")},
		},
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if response.Total != 9 || !response.HasMore || response.AsOf != "2026-08-15T08:00:00Z" ||
		response.Source != "akshare-screen-cn" {
		t.Fatalf("response envelope = %#v", response)
	}
	first := response.Entries[0]
	if first.InstrumentID != "SH.600519" || first.Symbol != "600519" || first.Name != "贵州茅台" ||
		first.Industry != nil || first.QuoteCurrency != "CNY" {
		t.Fatalf("entry[0] = %#v", first)
	}
	second := response.Entries[1]
	if second.Symbol != "000001" || second.Industry == nil || *second.Industry != "银行" {
		t.Fatalf("entry[1] = %#v", second)
	}
}

func TestProviderScreenAcceptsCNSHSZHKAndRejectsOthers(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"entries": []map[string]any{}, "total": 0, "has_more": false,
			"as_of": "2026-08-15T08:00:00Z",
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	for _, market := range []string{"CN", "SH", "SZ", "HK"} {
		if _, err := provider.Screen(ctx, marketdata.ScreenRequest{Market: market, Limit: 10}); err != nil {
			t.Fatalf("%s: Screen: %v", market, err)
		}
	}
	for _, market := range []string{"US", "MO", "BJ"} {
		if _, err := provider.Screen(ctx, marketdata.ScreenRequest{Market: market, Limit: 10}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: error = %v, want ErrUnsupported", market, err)
		}
	}
	if got := len(requests()); got != 4 {
		t.Fatalf("sidecar screen calls = %d, want 4", got)
	}
}

func TestProviderScreenClassifiesSidecarErrors(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.RawQuery, "unsupported") {
			return
		}
		writer.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error": map[string]any{"code": "unsupported_market", "message": "market not covered"},
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Screen(context.Background(), marketdata.ScreenRequest{Market: "CN", Limit: 10}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported_market: error = %v, want ErrUnsupported", err)
	}
}

func TestProviderScreenRejectsEntriesWithoutInstrumentID(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"entries": []map[string]any{{"instrument_id": " ", "name": "Ghost"}},
			"total":   1, "has_more": false, "as_of": "2026-08-15T08:00:00Z",
		})
	})
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.Screen(context.Background(), marketdata.ScreenRequest{Market: "CN", Limit: 10})
	if !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "instrument_id") {
		t.Fatalf("error = %v, want ErrInvalidResponse instrument_id", err)
	}
}
