package yfinance

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientNewsEncodesLimitAndDecodesEntries(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/news/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",
		"entries":[{"title":" Apple beats ","link":"https://example.test/1","publisher":"FixtureWire","published_at":"2026-08-10T15:30:00+02:00","summary":null}],
		"source":"yfinance-news"}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.news(context.Background(), "US", "AAPL", 25)
	if err != nil {
		t.Fatalf("news: %v", err)
	}
	if len(response.Entries) != 1 || response.Entries[0].Title == nil || *response.Entries[0].Title != " Apple beats " ||
		response.Entries[0].Summary != nil || response.Source != "yfinance-news" {
		t.Fatalf("news response = %#v", response)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].Path != "/providers/yfinance/news/US/AAPL" ||
		requests[0].Query.Get("limit") != "25" {
		t.Fatalf("news request = %#v", requests)
	}
}

func TestClientCorporateActionsEncodesInclusiveRange(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/corporate-actions/HK/00700", testkit.Response{Body: `{
		"market":"HK","symbol":"00700","instrument_id":"HK.00700",
		"events":[{"kind":"dividend","ex_date":"2026-05-11","amount":0.25,"ratio":null}],
		"source":"yfinance-actions"}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	from := time.Date(2025, 1, 1, 12, 0, 0, 0, time.FixedZone("HKT", 8*3600))
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	response, err := client.corporateActions(context.Background(), "HK", "00700", from, to)
	if err != nil {
		t.Fatalf("corporateActions: %v", err)
	}
	if len(response.Events) != 1 || response.Events[0].Kind != "dividend" {
		t.Fatalf("corporate actions response = %#v", response)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].Path != "/providers/yfinance/corporate-actions/HK/00700" ||
		requests[0].Query.Get("from") != "2025-01-01T04:00:00Z" ||
		requests[0].Query.Get("to") != "2026-08-01T00:00:00Z" {
		t.Fatalf("corporate actions request = %#v", requests)
	}
}

func TestClientCorporateActionsOmitsUnsetRange(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/corporate-actions/US/AAPL", testkit.Response{Body: `{
		"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","events":[],"source":"yfinance-actions"}`})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.corporateActions(context.Background(), "US", "AAPL", time.Time{}, time.Time{}); err != nil {
		t.Fatalf("corporateActions: %v", err)
	}
	requests := server.Requests()
	if len(requests) != 1 || requests[0].Query.Has("from") || requests[0].Query.Has("to") {
		t.Fatalf("corporate actions request query = %#v", requests[0].Query)
	}
}

func TestClientNewsSurfacesWarmingAndStructuredFailures(t *testing.T) {
	server := testkit.New(t)
	for range defaultMaxAttempts {
		server.Queue("/news/US/AAPL", testkit.Response{
			Status: http.StatusServiceUnavailable,
			Body:   `{"error":{"code":"YFINANCE_RUNTIME_WARMING","message":"runtime loading"}}`,
		})
	}
	server.Queue("/news/US/AAPL", testkit.Response{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"INVALID_LIMIT","message":"limit out of range"}}`,
	})
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	_, err = client.news(context.Background(), "US", "AAPL", 10)
	if !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming news error = %v", err)
	}
	_, err = client.news(context.Background(), "US", "AAPL", 10)
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.Code != "INVALID_LIMIT" ||
		remoteErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("structured news error = %#v", err)
	}
}
