package yfinance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

var testNow = time.Date(2026, time.July, 29, 15, 0, 0, 0, time.UTC)

func newTestProvider(t *testing.T, server *testkit.Server) *Provider {
	t.Helper()
	provider, err := NewProvider(server.URL())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	provider.client.httpClient.Timeout = 2 * time.Second
	provider.client.retryDelay = 0
	provider.now = func() time.Time { return testNow }
	return provider
}

func TestProviderDescriptorReflectsActualYahooPollingBoundary(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	descriptor, err := provider.Descriptor(context.Background())
	if err != nil {
		t.Fatalf("Descriptor: %v", err)
	}
	if descriptor.ProviderID != "yahoo-finance" || descriptor.Source != sourceID ||
		descriptor.DefaultMarket != "US" || !slices.Equal(descriptor.SupportedMarkets, []string{"US", "HK", "SH", "SZ"}) {
		t.Fatalf("Descriptor identity = %#v", descriptor)
	}
	capabilities := descriptor.Capabilities
	if !capabilities.Snapshots || !capabilities.HistoricalCandles || !capabilities.InstrumentSearch ||
		!capabilities.ExtendedHours || capabilities.StreamingQuotes || capabilities.StreamingDepth ||
		capabilities.OrderBookDepth || capabilities.TickCandles {
		t.Fatalf("Descriptor capabilities = %#v", capabilities)
	}
	if descriptor.Constraints.RequiresOpenD || descriptor.Constraints.RequiresMarketDataRight ||
		descriptor.Constraints.UsesSubscriptionQuota {
		t.Fatalf("Descriptor constraints = %#v", descriptor.Constraints)
	}
	policy := provider.QuotePollingPolicy()
	if policy.Interval != 15*time.Second || policy.Timeout != 15*time.Second {
		t.Fatalf("default polling policy = %#v", policy)
	}
	if _, err := NewProvider("not-a-url"); err == nil {
		t.Fatal("NewProvider accepted an invalid endpoint")
	}
}

func TestProviderReadsMarketsSearchLookupAndSecurityThroughSidecar(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	ctx := context.Background()

	profiles, err := provider.GetMarkets(ctx)
	if err != nil || len(profiles) != 4 {
		t.Fatalf("GetMarkets = %#v, err=%v", profiles, err)
	}
	profile := profiles[0]
	precision, _ := profile["precision"].(map[string]any)
	sessions, _ := profile["regularSessions"].([]map[string]any)
	if profile["code"] != "US" || profile["quoteCurrency"] != "USD" ||
		profile["supportsExtendedHours"] != true || precision["price"] != 2 ||
		len(sessions) != 1 || sessions[0]["label"] != "09:30-16:00" {
		t.Fatalf("market profile = %#v", profile)
	}
	profileCodes := make([]string, 0, len(profiles))
	for _, candidate := range profiles {
		if code, ok := candidate["code"].(string); ok {
			profileCodes = append(profileCodes, code)
		}
	}
	if !slices.Equal(profileCodes, []string{"US", "HK", "SH", "SZ"}) {
		t.Fatalf("market profile codes = %#v", profileCodes)
	}

	entries, err := provider.SearchInstruments(ctx, " aapl ", 0)
	if err != nil || len(entries) != 1 || entries[0].InstrumentID != "US.AAPL" ||
		entries[0].Name != "AAPL Incorporated" || !entries[0].Selectable {
		t.Fatalf("SearchInstruments = %#v, err=%v", entries, err)
	}
	searchRequest := requestForPath(t, server, "/search")
	if searchRequest.Query.Get("q") != "aapl" || searchRequest.Query.Get("limit") != "20" {
		t.Fatalf("search query = %v", searchRequest.Query)
	}

	exact, err := provider.LookupInstrument(ctx, "NASDAQ", "aapl")
	if err != nil || len(exact) != 1 || exact[0].InstrumentID != "US.AAPL" {
		t.Fatalf("LookupInstrument = %#v, err=%v", exact, err)
	}

	details, err := provider.GetSecurityDetails(ctx, "US", "AAPL")
	if err != nil {
		t.Fatalf("GetSecurityDetails: %v", err)
	}
	request := details["request"].(map[string]any)
	security := details["security"].(map[string]any)
	meta := details["meta"].(map[string]any)
	if request["instrumentId"] != "US.AAPL" || security["name"] != "AAPL Incorporated" ||
		security["securityType"] != "EQUITY" || meta["source"] != "yfinance" ||
		meta["resolvedAt"] != testNow.Format(time.RFC3339Nano) {
		t.Fatalf("security details = %#v", details)
	}

	server.Queue("/security/US/MISSING", testkit.Response{
		Status: http.StatusNotFound,
		Body:   `{"error":{"code":"NO_DATA","message":"security not found"}}`,
	})
	missing, err := provider.LookupInstrument(ctx, "US", "MISSING")
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing LookupInstrument = %#v, err=%v", missing, err)
	}
}

func TestProviderConvertsSnapshotsCandlesHealthAndUnsupportedDepth(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	ctx := context.Background()

	snapshot, err := provider.QuerySnapshot(ctx, "nasdaq:aapl")
	if err != nil {
		t.Fatalf("QuerySnapshot: %v", err)
	}
	if snapshot.InstrumentID != "US.AAPL" ||
		!snapshot.Price.Equal(decimal.RequireFromString("189.25")) ||
		!snapshot.Volume.Equal(decimal.NewFromInt(1234567)) || snapshot.QuoteAt != "2026-07-29T14:30:00Z" ||
		snapshot.ObservedAt != "2026-07-29T14:45:00Z" || snapshot.Kind != marketdata.TickKindQuote {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	ticker, err := provider.QueryTicker(ctx, "US.AAPL")
	if err != nil || !ticker.Price.Equal(snapshot.Price) {
		t.Fatalf("QueryTicker = %#v, err=%v", ticker, err)
	}

	response, err := provider.GetHistoricalCandles(
		ctx, "NYSE", "AAPL", "1d", 2,
		"2026-07-28T00:00:00Z", "2026-07-30T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("GetHistoricalCandles: %v", err)
	}
	candles := response["candles"].([]map[string]any)
	meta := response["meta"].(map[string]any)
	if len(candles) != 2 || candles[1]["close"] != "189.25" || candles[0]["volume"] != "1000" ||
		meta["source"] != "yfinance" || meta["extendedHours"] != false || meta["session"] != nil {
		t.Fatalf("candles response = %#v", response)
	}
	candlesRequest := requestForPath(t, server, "/candles/US/AAPL")
	if candlesRequest.Query.Get("period") != "1d" || candlesRequest.Query.Get("limit") != "2" ||
		candlesRequest.Query.Get("from") == "" || candlesRequest.Query.Get("to") == "" {
		t.Fatalf("candles query = %v", candlesRequest.Query)
	}

	health, err := provider.Health(ctx)
	if err != nil || !health.Connected || health.StreamMode != "snapshot-poll-delayed" || health.ActiveCount != 0 {
		t.Fatalf("Health = %#v, err=%v", health, err)
	}
	server.Queue("/health", testkit.Response{Body: `{"ok":false,"yfinance_version":"0.2.61"}`})
	health, err = provider.Health(ctx)
	if err != nil || health.Connected {
		t.Fatalf("unhealthy Health = %#v, err=%v", health, err)
	}
	server.Queue("/health", testkit.Response{Body: `{"ok":true}`})
	health, err = provider.Health(ctx)
	if !errors.Is(err, ErrInvalidResponse) || health.Connected {
		t.Fatalf("invalid-contract Health = %#v, err=%v", health, err)
	}

	if _, err := provider.GetDepth(ctx, "US", "AAPL", 10); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("GetDepth error = %v", err)
	}
	if _, err := provider.GetHistoricalCandles(ctx, "US", "AAPL", "tick", 10, "", ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported candle period error = %v", err)
	}
}

func TestProviderQueriesHongKongAndChinaLeafMarkets(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	for input, wantID := range map[string]string{
		"HK.700": "HK.00700", "CN/SH.600519": "SH.600519", "SZ.000001": "SZ.000001",
	} {
		instrumentID := input
		if strings.Contains(input, "/") {
			instrumentID = strings.SplitN(input, "/", 2)[1]
		}
		snapshot, err := provider.QuerySnapshot(context.Background(), instrumentID)
		if err != nil || snapshot.InstrumentID != wantID || snapshot.Market != strings.SplitN(wantID, ".", 2)[0] {
			t.Fatalf("QuerySnapshot(%q) = %#v, err=%v", input, snapshot, err)
		}
	}
	for _, path := range []string{"/snapshot/HK/00700", "/snapshot/SH/600519", "/snapshot/SZ/000001"} {
		if server.Count(path) != 1 {
			t.Fatalf("request %s count = %d", path, server.Count(path))
		}
	}
}

func TestProviderNormalizesUSAliasesAndRejectsInvalidInputs(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	ctx := context.Background()

	normalized, err := provider.NormalizeInstrument(ctx, map[string]any{
		"market": " nasdaq ", "code": " aapl ",
	})
	if err != nil || normalized["market"] != "US" || normalized["code"] != "AAPL" ||
		normalized["symbol"] != "US.AAPL" || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("NormalizeInstrument = %#v, err=%v", normalized, err)
	}
	normalized, err = provider.NormalizeInstrument(ctx, map[string]any{"instrumentId": "usa:msft"})
	if err != nil || normalized["instrumentId"] != "US.MSFT" {
		t.Fatalf("qualified NormalizeInstrument = %#v, err=%v", normalized, err)
	}
	normalized, err = provider.NormalizeInstrument(ctx, map[string]any{
		"market": "US", "symbol": "US.AAPL", "code": "AAPL",
	})
	if err != nil || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("symbol-and-code NormalizeInstrument = %#v, err=%v", normalized, err)
	}

	tests := []map[string]any{
		{},
		{"market": 7, "symbol": "AAPL"},
		{"market": "US", "symbol": "AAPL", "code": "MSFT"},
		{"market": "CN", "symbol": "600519"},
	}
	for _, input := range tests {
		if _, err := provider.NormalizeInstrument(ctx, input); err == nil {
			t.Fatalf("NormalizeInstrument(%#v) error = nil", input)
		}
	}
	if _, err := provider.SearchInstruments(ctx, " ", 10); err == nil {
		t.Fatal("empty SearchInstruments error = nil")
	}
	for input, want := range map[string]string{
		"HK.700": "HK.00700", "SH:600519": "SH.600519", "SZ.000001": "SZ.000001",
	} {
		normalized, err := provider.NormalizeInstrument(ctx, map[string]any{"instrumentId": input})
		if err != nil || normalized["instrumentId"] != want {
			t.Fatalf("NormalizeInstrument(%q) = %#v, err=%v", input, normalized, err)
		}
	}
	china, err := provider.NormalizeInstrument(ctx, map[string]any{
		"market": "CN", "symbol": "SH.600519", "code": "600519",
	})
	if err != nil || china["market"] != "SH" || china["resolvedMarket"] != "CN" || china["instrumentId"] != "SH.600519" {
		t.Fatalf("CN qualified NormalizeInstrument = %#v, err=%v", china, err)
	}
}

func TestGetHistoricalCandlesRejectsOneMinutePeriodBeyondSevenDayWindow(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	ctx := context.Background()

	// testNow = 2026-07-29T15:00:00Z; 7-day cutoff = 2026-07-22T15:00:00Z
	beyondWindow := "2026-07-01T00:00:00Z"
	if _, err := provider.GetHistoricalCandles(ctx, "US", "AAPL", "1m", 10, beyondWindow, ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("1m beyond window: want ErrUnsupported, got %v", err)
	}

	// One second before cutoff should also be rejected.
	justBefore := "2026-07-22T14:59:59Z"
	if _, err := provider.GetHistoricalCandles(ctx, "US", "AAPL", "1m", 10, justBefore, ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("1m at cutoff boundary: want ErrUnsupported, got %v", err)
	}

	// Within the window: Go validation passes; sidecar returns empty candles.
	server.Queue("/candles/US/AAPL", testkit.Response{
		Body: `{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",` +
			`"period":"1m","extended_hours":true,"total_returned":0,"source":"yfinance","candles":[]}`,
	})
	withinWindow := "2026-07-25T00:00:00Z"
	_, err := provider.GetHistoricalCandles(ctx, "US", "AAPL", "1m", 10, withinWindow, "")
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("1m within window should not return ErrUnsupported, got %v", err)
	}

	// Empty from_time: no window check; sidecar handles its own default window.
	server.Queue("/candles/US/AAPL", testkit.Response{
		Body: `{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",` +
			`"period":"1m","extended_hours":true,"total_returned":0,"source":"yfinance","candles":[]}`,
	})
	_, err = provider.GetHistoricalCandles(ctx, "US", "AAPL", "1m", 10, "", "")
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("1m with no from_time should not return ErrUnsupported, got %v", err)
	}
}

func TestProviderQueryTickersReturnsPartialSuccessAndPreservesAllFailureErrors(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)
	ctx := context.Background()

	server.Queue("/snapshot/US/MSFT", testkit.Response{
		Status: http.StatusNotFound,
		Body:   `{"error":{"code":"NO_DATA","message":"snapshot not found"}}`,
	})
	ticks, err := provider.QueryTickers(ctx, []string{"US.AAPL", "US.MSFT", "us.aapl"})
	if err != nil || len(ticks) != 1 || ticks["US.AAPL"].InstrumentID != "US.AAPL" {
		t.Fatalf("partial QueryTickers = %#v, err=%v", ticks, err)
	}
	if server.Count("/snapshot/US/AAPL") != 1 {
		t.Fatalf("duplicate AAPL request count = %d", server.Count("/snapshot/US/AAPL"))
	}

	server.Queue(
		"/snapshot/US/AAPL",
		testkit.Response{Status: http.StatusNotFound, Body: `{"error":{"code":"NO_AAPL","message":"missing"}}`},
	)
	server.Queue(
		"/snapshot/US/MSFT",
		testkit.Response{Status: http.StatusBadGateway, Body: `{"error":{"code":"UPSTREAM","message":"down"}}`},
		testkit.Response{Status: http.StatusBadGateway, Body: `{"error":{"code":"UPSTREAM","message":"down"}}`},
		testkit.Response{Status: http.StatusBadGateway, Body: `{"error":{"code":"UPSTREAM","message":"down"}}`},
	)
	ticks, err = provider.QueryTickers(ctx, []string{"US.AAPL", "US.MSFT"})
	if err == nil || ticks != nil || !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("failed QueryTickers = %#v, err=%v", ticks, err)
	}
	var remoteErr *HTTPError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("failed QueryTickers did not preserve HTTPError: %v", err)
	}

	ticks, err = provider.QueryTickers(ctx, nil)
	if err != nil || len(ticks) != 0 {
		t.Fatalf("empty QueryTickers = %#v, err=%v", ticks, err)
	}
}

func TestProviderSearchClampsLimitAndLookupRejectsMismatchedIdentity(t *testing.T) {
	server := testkit.New(t)
	provider := newTestProvider(t, server)

	if _, err := provider.SearchInstruments(context.Background(), "MSFT", 1000); err != nil {
		t.Fatalf("SearchInstruments: %v", err)
	}
	request := requestForPath(t, server, "/search")
	if request.Query.Get("limit") != "100" {
		t.Fatalf("clamped search limit = %q", request.Query.Get("limit"))
	}

	server.Queue("/security/US/AAPL", testkit.Response{
		Body: `{"market":"US","symbol":"MSFT","instrument_id":"US.MSFT","name":"Microsoft","security_type":"EQUITY","source":"yfinance"}`,
	})
	if _, err := provider.LookupInstrument(context.Background(), "US", "AAPL"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("mismatched lookup error = %v", err)
	}
}

func requestForPath(t *testing.T, server *testkit.Server, path string) testkit.Request {
	t.Helper()
	for _, request := range server.Requests() {
		if request.Path == path {
			if request.Method != http.MethodGet {
				t.Fatalf("%s method = %s", path, request.Method)
			}
			return request
		}
	}
	payload, _ := json.Marshal(server.Requests())
	t.Fatalf("request %s not found in %s", path, payload)
	return testkit.Request{}
}
