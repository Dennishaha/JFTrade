package yfinance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestNewClientValidatesURLAndDefaultTransport(t *testing.T) {
	for _, value := range []string{"", "localhost:7788", "ftp://localhost:7788", "://bad"} {
		if _, err := NewClient(value, nil); err == nil {
			t.Fatalf("NewClient(%q) error = nil", value)
		}
	}
	client, err := NewClient("http://127.0.0.1:7788/base/", nil)
	if err != nil || client.httpClient == nil || client.httpClient.Timeout != 15*time.Second ||
		client.baseURL.Path != "/base" {
		t.Fatalf("NewClient = %#v, err=%v", client, err)
	}
}

func TestClientRetriesSafeServerFailuresThenReturnsDecodedResponse(t *testing.T) {
	server := testkit.New(t)
	server.Queue(
		"/health",
		testkit.Response{Status: http.StatusBadGateway, Body: `{"error":{"code":"UPSTREAM","message":"temporary"}}`},
		testkit.Response{
			Status: http.StatusTooManyRequests,
			Body:   `{"error":{"code":"RATE_LIMIT","message":"slow down"}}`,
			Header: http.Header{"Retry-After": []string{"0"}},
		},
		testkit.Response{Body: `{"ok":true,"yfinance_version":"1.6.0","runtime_state":"ready"}`},
	)
	client, err := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	response, err := client.health(context.Background())
	if err != nil || !response.OK || response.YFinanceVersion != "1.6.0" {
		t.Fatalf("health = %#v, err=%v", response, err)
	}
	if server.Count("/health") != 3 {
		t.Fatalf("health request count = %d", server.Count("/health"))
	}
}

func TestClientHealthRequiresYFinanceVersion(t *testing.T) {
	server := testkit.New(t)
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})

	for _, body := range []string{
		`{"ok":true}`,
		`{"ok":true,"yfinance_version":"   "}`,
	} {
		server.Queue("/health", testkit.Response{Body: body})
		response, err := client.health(context.Background())
		if !errors.Is(err, ErrInvalidResponse) ||
			!strings.Contains(err.Error(), "yfinance_version") ||
			response.OK ||
			response.YFinanceVersion != "" {
			t.Fatalf("health body %s = %#v, err=%v", body, response, err)
		}
	}
}

func TestClientHealthRequiresKnownRuntimeState(t *testing.T) {
	server := testkit.New(t)
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	for _, state := range []string{"", "starting", "READY"} {
		server.Queue("/health", testkit.Response{Body: `{"ok":true,"yfinance_version":"1.6.0","runtime_state":"` + state + `"}`})
		if _, err := client.health(context.Background()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("runtime_state %q error = %v", state, err)
		}
	}
}

func TestClientPreservesStructuredHTTPErrorWithoutRetryingCallerFailures(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/search", testkit.Response{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"INVALID_QUERY","message":"query is required"}}`,
	})
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	_, err := client.search(context.Background(), "", 20)
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != http.StatusBadRequest ||
		remoteErr.Code != "INVALID_QUERY" || remoteErr.Message != "query is required" {
		t.Fatalf("search error = %#v", err)
	}
	if errors.Is(err, ErrSidecarUnavailable) || server.Count("/search") != 1 {
		t.Fatalf("caller failure classification/count = %v/%d", err, server.Count("/search"))
	}

	server.Queue("/health", testkit.Response{Status: http.StatusTeapot, Body: "plain failure"})
	_, err = client.health(context.Background())
	if !errors.As(err, &remoteErr) || remoteErr.Message != "plain failure" {
		t.Fatalf("plain HTTP error = %#v", err)
	}
}

func TestClientClassifiesExhaustedServerAndNetworkFailuresAsUnavailable(t *testing.T) {
	server := testkit.New(t)
	for range defaultMaxAttempts {
		server.Queue("/health", testkit.Response{
			Status: http.StatusBadGateway,
			Body:   `{"error":{"code":"UPSTREAM","message":"unavailable"}}`,
		})
	}
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	client.retryDelay = 0
	_, err := client.health(context.Background())
	if !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("server failure = %v", err)
	}
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.Code != "UPSTREAM" {
		t.Fatalf("server failure HTTPError = %#v", err)
	}

	server.Close()
	_, err = client.health(context.Background())
	if !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("network failure = %v", err)
	}
}

func TestClientRejectsMalformedEmptyTrailingAndOversizedResponses(t *testing.T) {
	server := testkit.New(t)
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	client.retryDelay = 0

	tests := []testkit.Response{
		{Body: `{`},
		{Status: http.StatusNoContent},
		{Body: `{"ok":true} {"extra":true}`},
		{Body: `"` + strings.Repeat("x", maxResponseBytes) + `"`},
	}
	for _, response := range tests {
		server.Queue("/health", response)
		_, err := client.health(context.Background())
		if !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("response status=%d produced error %v", response.Status, err)
		}
	}
}

func TestClientRetryWaitHonorsContextCancellation(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/health", testkit.Response{
		Status: http.StatusServiceUnavailable,
		Body:   `{"error":{"code":"STARTING","message":"not ready"}}`,
		Header: http.Header{"Retry-After": []string{"1"}},
	})
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := client.health(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || server.Count("/health") != 1 {
		t.Fatalf("canceled retry = %v, count=%d", err, server.Count("/health"))
	}
}

func TestClientClassifiesRuntimeWarmingAfterRetryBudget(t *testing.T) {
	server := testkit.New(t)
	server.Queue("/search", testkit.Response{
		Status: http.StatusServiceUnavailable,
		Body:   `{"error":{"code":"YFINANCE_RUNTIME_WARMING","message":"warming"}}`,
		Header: http.Header{"Retry-After": []string{"1"}},
	})
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})
	client.maxAttempts = 1

	_, err := client.search(t.Context(), "AAPL", 1)
	if !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", err)
	}
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("warming HTTP error = %#v", remoteErr)
	}
}

func TestClientTimeoutCoversAllRetryAttempts(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		<-request.Context().Done()
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, &http.Client{Timeout: 30 * time.Millisecond})
	client.retryDelay = 0
	_, err := client.health(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("total retry timeout error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests within one total timeout = %d, want 2", got)
	}
}

func TestClientEndpointMethodsEncodePathAndOptionalCandleQuery(t *testing.T) {
	server := testkit.New(t)
	client, _ := NewClient(server.URL(), &http.Client{Timeout: time.Second})

	if _, err := client.markets(context.Background()); err != nil {
		t.Fatalf("markets: %v", err)
	}
	if _, err := client.security(context.Background(), "US", "AAPL"); err != nil {
		t.Fatalf("security: %v", err)
	}
	if _, err := client.snapshot(context.Background(), "US", "AAPL"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := client.candles(context.Background(), "US", "AAPL", "1d", 5, "", "", ""); err != nil {
		t.Fatalf("candles: %v", err)
	}
	request := requestForPath(t, server, "/candles/US/AAPL")
	if request.Query.Get("from") != "" || request.Query.Get("to") != "" || request.Query.Get("before") != "" ||
		request.Query.Get("period") != "1d" || request.Query.Get("limit") != "5" {
		t.Fatalf("optional candle query = %v", request.Query)
	}
	if _, err := client.candles(context.Background(), "US", "AAPL", "1d", 5, "", "", "2026-07-15T13:30:00Z"); err != nil {
		t.Fatalf("candles before cursor: %v", err)
	}
	requests := server.Requests()
	request = requests[len(requests)-1]
	if request.Query.Get("before") != "2026-07-15T13:30:00Z" {
		t.Fatalf("before cursor query = %v", request.Query)
	}

	var nilClient *Client
	if _, err := nilClient.health(context.Background()); !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("nil client health error = %v", err)
	}
}

func TestHTTPErrorFormattingAndClassification(t *testing.T) {
	tests := []struct {
		err  *HTTPError
		want string
	}{
		{nil, ""},
		{&HTTPError{StatusCode: 400}, "HTTP 400"},
		{&HTTPError{StatusCode: 400, Message: "bad"}, "HTTP 400: bad"},
		{&HTTPError{StatusCode: 400, Code: "BAD", Message: "bad"}, "HTTP 400 (BAD): bad"},
	}
	for _, test := range tests {
		if got := test.err.Error(); !strings.Contains(got, test.want) {
			t.Fatalf("HTTPError.Error() = %q, want containing %q", got, test.want)
		}
	}
	if (&HTTPError{StatusCode: 499}).Unwrap() != nil ||
		!errors.Is(&HTTPError{StatusCode: 500}, ErrSidecarUnavailable) {
		t.Fatal("HTTPError classification is incorrect")
	}
}
