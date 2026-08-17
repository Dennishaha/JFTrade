package yfinance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientScreenPostsConditionSortAndPagingBody(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/screen", testkit.Response{Body: `{
		"entries":[{"instrument_id":"US.AAPL","name":"Apple","symbol":"AAPL",
			"industry":"Technology","quote_currency":"USD",
			"values":{"simple.price":189.25,"simple.volume":1234567}}],
		"total":1,"has_more":false,"as_of":"2026-08-15T20:00:00Z","source":"yfinance-screen"}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.screen(context.Background(), remoteScreenRequest{
		Market: "US",
		Conditions: []remoteScreenCondition{
			{FactorKey: "simple.price", Min: json.Number("100"), Max: json.Number("300")},
			{FactorKey: "simple.pe_ttm", Max: json.Number("40")},
		},
		Sorts:  []remoteScreenSort{{FactorKey: "simple.market_cap", Direction: "desc"}},
		Offset: 50,
		Limit:  25,
	})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if response.Total != 1 || len(response.Entries) != 1 || response.Source != "yfinance-screen" {
		t.Fatalf("response = %#v", response)
	}

	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].Method != http.MethodPost || requests[0].Path != "/providers/yfinance/screen" {
		t.Fatalf("request = %#v", requests[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(requests[0].Body), &body); err != nil {
		t.Fatalf("request body is not JSON: %q", requests[0].Body)
	}
	if body["market"] != "US" || body["offset"] != float64(50) || body["limit"] != float64(25) {
		t.Fatalf("request body = %v", body)
	}
	conditions, ok := body["conditions"].([]any)
	if !ok || len(conditions) != 2 {
		t.Fatalf("conditions = %v", body["conditions"])
	}
	first, ok := conditions[0].(map[string]any)
	if !ok || first["factor_key"] != "simple.price" || first["min"] != float64(100) ||
		first["max"] != float64(300) {
		t.Fatalf("condition[0] = %v", conditions[0])
	}
	if _, oneSided := conditions[1].(map[string]any)["min"]; oneSided {
		t.Fatalf("one-sided condition must omit min: %v", conditions[1])
	}
	sorts, ok := body["sorts"].([]any)
	if !ok || len(sorts) != 1 || sorts[0].(map[string]any)["direction"] != "desc" {
		t.Fatalf("sorts = %v", body["sorts"])
	}
}

func TestProviderScreenConvertsEntriesAndDerivesSymbol(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/screen", testkit.Response{Body: `{
		"entries":[
			{"instrument_id":"us.aapl","name":" Apple ","symbol":null,"industry":null,
				"quote_currency":" usd ","values":{"simple.price":189.25}},
			{"instrument_id":"US.MSFT","name":"Microsoft","symbol":"msft",
				"industry":"Software","quote_currency":"USD","values":{}}],
		"total":7,"has_more":true,"next_offset":2,"as_of":"2026-08-15T20:00:00Z"}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	response, err := provider.Screen(context.Background(), marketdata.ScreenRequest{
		Market: "US",
		Conditions: []marketdata.ScreenConditionRequest{
			{FactorKey: "simple.price", Min: number("100")},
		},
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if response.Total != 7 || !response.HasMore || response.AsOf != "2026-08-15T20:00:00Z" ||
		response.Source != "yfinance-screen-us" {
		t.Fatalf("response envelope = %#v", response)
	}
	if response.NextOffset == nil || *response.NextOffset != 2 {
		t.Fatalf("next offset = %v", response.NextOffset)
	}
	first := response.Entries[0]
	if first.InstrumentID != "US.AAPL" || first.Symbol != "AAPL" || first.Name != "Apple" ||
		first.Industry != nil || first.QuoteCurrency != "USD" {
		t.Fatalf("entry[0] = %#v", first)
	}
	if first.Values["simple.price"] != json.Number("189.25") {
		t.Fatalf("entry[0].values = %#v", first.Values)
	}
	second := response.Entries[1]
	if second.Symbol != "MSFT" || second.Industry == nil || *second.Industry != "Software" {
		t.Fatalf("entry[1] = %#v", second)
	}
}

func TestProviderScreenRejectsNonUSMarketsWithoutSidecarCall(t *testing.T) {
	server := testkit.New(t)
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	for _, market := range []string{"HK", "SH", "SZ"} {
		if _, err := provider.Screen(context.Background(), marketdata.ScreenRequest{
			Market: market, Limit: 10,
		}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: error = %v, want ErrUnsupported", market, err)
		}
	}
	if count := server.Count("/screen"); count != 0 {
		t.Fatalf("sidecar screen calls = %d, want 0", count)
	}
}

func TestProviderScreenClassifiesSidecarErrors(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/screen", testkit.Response{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"unsupported_market","message":"market not covered"}}`,
	})
	server.Queue("/screen", testkit.Response{
		Status: http.StatusNotFound,
		Body:   `{"error":{"code":"NOT_FOUND","message":"no such route"}}`,
	})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.Screen(ctx, marketdata.ScreenRequest{Market: "US", Limit: 10}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported_market: error = %v, want ErrUnsupported", err)
	}
	_, err = provider.Screen(ctx, marketdata.ScreenRequest{Market: "US", Limit: 10})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("404: error = %v, want HTTPError 404", err)
	}
}

func TestProviderScreenRejectsEntriesWithoutInstrumentID(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/screen", testkit.Response{Body: `{
		"entries":[{"instrument_id":" ","name":"Ghost","values":{}}],
		"total":1,"has_more":false,"as_of":"2026-08-15T20:00:00Z"}`})
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.Screen(context.Background(), marketdata.ScreenRequest{Market: "US", Limit: 10})
	if !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "instrument_id") {
		t.Fatalf("error = %v, want ErrInvalidResponse instrument_id", err)
	}
}
