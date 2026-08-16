package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

type recordedNewsRequest struct {
	path  string
	query url.Values
}

func newNewsRecordingServer(t *testing.T, handler func(writer http.ResponseWriter, request *http.Request)) (*httptest.Server, func() []recordedNewsRequest) {
	t.Helper()
	var mu sync.Mutex
	requests := make([]recordedNewsRequest, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, recordedNewsRequest{path: request.URL.Path, query: request.URL.Query()})
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		handler(writer, request)
	}))
	t.Cleanup(server.Close)
	return server, func() []recordedNewsRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedNewsRequest(nil), requests...)
	}
}

func TestClientNewsEncodesLimitAndDecodesNullableEntries(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "SH", "symbol": "600519", "instrument_id": "SH.600519",
			"entries": []map[string]any{{
				"title": "贵州茅台公告", "link": "https://example.test/1",
				"publisher": "FixtureWire", "published_at": "2026-08-10T09:30:00Z", "summary": nil,
			}},
			"source": "akshare-news",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.news(context.Background(), "SH", "600519", 40)
	if err != nil {
		t.Fatalf("news: %v", err)
	}
	if len(response.Entries) != 1 || response.Entries[0].Title == nil || *response.Entries[0].Title != "贵州茅台公告" ||
		response.Entries[0].Summary != nil || response.Source != "akshare-news" {
		t.Fatalf("news response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/news/SH/600519" || seen[0].query.Get("limit") != "40" {
		t.Fatalf("news request = %#v", seen)
	}
}

func TestClientCorporateActionsEncodesRangeAndDecodesEvents(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "SZ", "symbol": "000001", "instrument_id": "SZ.000001",
			"events": []map[string]any{
				{"kind": "split", "ex_date": "2026-06-09", "amount": nil, "ratio": 4},
				{"kind": "dividend", "ex_date": "2026-05-11", "amount": 0.25, "ratio": nil},
			},
			"source": "akshare-actions",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	from := time.Date(2025, 1, 1, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	response, err := client.corporateActions(context.Background(), "SZ", "000001", from, to)
	if err != nil {
		t.Fatalf("corporateActions: %v", err)
	}
	if len(response.Events) != 2 || response.Events[0].Ratio == nil || response.Events[1].Amount == nil {
		t.Fatalf("corporate actions response = %#v", response)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].path != "/providers/akshare/corporate-actions/SZ/000001" ||
		seen[0].query.Get("from") != "2025-01-01T04:00:00Z" || seen[0].query.Get("to") != "2026-08-01T00:00:00Z" {
		t.Fatalf("corporate actions request = %#v", seen)
	}
}

func TestClientCorporateActionsOmitsUnsetRange(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"market": "SH", "symbol": "600519", "instrument_id": "SH.600519",
			"events": []any{}, "source": "akshare-actions",
		})
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.corporateActions(context.Background(), "SH", "600519", time.Time{}, time.Time{}); err != nil {
		t.Fatalf("corporateActions: %v", err)
	}
	seen := requests()
	if len(seen) != 1 || seen[0].query.Has("from") || seen[0].query.Has("to") {
		t.Fatalf("corporate actions query = %#v", seen[0].query)
	}
}

func TestClientNewsMapsUnsupportedMarketToCapabilityError(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UNSUPPORTED","message":"US news is not covered by akshare"}}`))
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.news(context.Background(), "US", "AAPL", 10)
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, marketdata.ErrCapabilityUnsupported) {
		t.Fatalf("unsupported news error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("unsupported request retried: %d calls", len(requests()))
	}
}

func TestClientCorporateActionsSurfacesPoolBusyWithoutRetryStorm(t *testing.T) {
	server, requests := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "2")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_POOL_BUSY","message":"pool saturated"}}`))
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	_, err = client.corporateActions(context.Background(), "SH", "600519", time.Time{}, time.Time{})
	if !errors.Is(err, marketdata.ErrProviderBusy) {
		t.Fatalf("busy corporate actions error = %v", err)
	}
	if len(requests()) != 1 {
		t.Fatalf("busy request retried: %d calls", len(requests()))
	}
}

func TestClientNewsSurfacesColdCacheWarming(t *testing.T) {
	server, _ := newNewsRecordingServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_RUNTIME_WARMING","message":"cold cache warming"}}`))
	})
	client, err := NewClient(server.URL, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	if _, err := client.news(context.Background(), "SH", "600519", 10); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming news error = %v", err)
	}
}
