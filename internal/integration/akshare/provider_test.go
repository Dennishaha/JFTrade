package akshare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestProviderExposesPollingOnlyAKShareBoundary(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	descriptor, err := provider.Descriptor(t.Context())
	if err != nil {
		t.Fatalf("Descriptor: %v", err)
	}
	if descriptor.ProviderID != "akshare" || descriptor.BrokerID != "akshare" ||
		descriptor.Source != "akshare" || descriptor.DefaultMarket != "US" ||
		!slices.Equal(descriptor.SupportedMarkets, []string{"US", "HK", "SH", "SZ"}) ||
		!slices.Equal(descriptor.Transports, []string{"http-poll"}) {
		t.Fatalf("descriptor identity = %#v", descriptor)
	}
	capabilities := descriptor.Capabilities
	if !capabilities.Snapshots || !capabilities.HistoricalCandles || !capabilities.InstrumentSearch ||
		capabilities.StreamingQuotes || capabilities.StreamingDepth || capabilities.OrderBookDepth ||
		capabilities.ExtendedHours || !slices.Equal(capabilities.CandleIntervals, candlePeriodOrder) ||
		!slices.Equal(capabilities.Sessions, []string{"regular", "closed"}) {
		t.Fatalf("descriptor capabilities = %#v", capabilities)
	}
	policy := provider.QuotePollingPolicy()
	if policy.Interval != 15*time.Second || policy.Timeout != defaultRequestTimeout {
		t.Fatalf("polling policy = %#v", policy)
	}
	if _, err := provider.GetDepth(t.Context(), "US", "AAPL", 10); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("GetDepth error = %v", err)
	}
}

func TestProviderConvertsNamespacedSidecarContract(t *testing.T) {
	server := newContractServer(t)
	defer server.Close()
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	provider.now = func() time.Time { return time.Date(2026, 8, 3, 10, 30, 0, 0, time.UTC) }

	health, err := provider.Health(t.Context())
	if err != nil || !health.Connected || health.Readiness != marketdata.ProviderReadinessReady ||
		health.StreamMode != "snapshot-poll-delayed" {
		t.Fatalf("Health = %#v, err=%v", health, err)
	}
	markets, err := provider.GetMarkets(t.Context())
	if err != nil || len(markets) != 4 || markets[0]["code"] != "US" ||
		markets[2]["resolvedMarket"] != "CN" {
		t.Fatalf("GetMarkets = %#v, err=%v", markets, err)
	}

	entries, err := provider.SearchInstruments(t.Context(), "apple", 5)
	if err != nil || len(entries) != 1 || entries[0].InstrumentID != "US.AAPL" ||
		!slices.Equal(entries[0].SupportedPeriods, candlePeriodOrder) {
		t.Fatalf("SearchInstruments = %#v, err=%v", entries, err)
	}
	lookup, err := provider.LookupInstrument(t.Context(), "us", "aapl")
	if err != nil || len(lookup) != 1 || lookup[0].InstrumentID != "US.AAPL" ||
		!slices.Equal(lookup[0].SupportedPeriods, candlePeriodOrder) {
		t.Fatalf("LookupInstrument = %#v, err=%v", lookup, err)
	}
	details, err := provider.GetSecurityDetails(t.Context(), "US", "AAPL")
	security, _ := details["security"].(map[string]any)
	if err != nil || security["instrumentId"] != "US.AAPL" ||
		!slices.Equal(security["supportedPeriods"].([]string), candlePeriodOrder) {
		t.Fatalf("GetSecurityDetails = %#v, err=%v", details, err)
	}

	tick, err := provider.QuerySnapshot(t.Context(), "US.AAPL")
	if err != nil || tick == nil || tick.Price.String() != "189.25" || tick.Source != "akshare:eastmoney" ||
		!tick.Availability.Authoritative || tick.Availability.Bid || tick.Availability.Ask ||
		!tick.Availability.Volume || tick.Availability.Turnover {
		t.Fatalf("QuerySnapshot = %#v, err=%v", tick, err)
	}
	snapshot := marketdata.SnapshotJSON(tick)
	if snapshot["bid"] != nil || snapshot["ask"] != nil || snapshot["turnover"] != nil ||
		snapshot["volume"] != "123456700" || snapshot["at"] != nil && snapshot["at"] != "" {
		t.Fatalf("nullable snapshot = %#v", snapshot)
	}

	ticks, err := provider.QueryTickers(t.Context(), []string{"US.AAPL", "HK.00700", "US.AAPL"})
	if err != nil || len(ticks) != 2 || ticks["US.AAPL"].Price.String() != "189.25" ||
		ticks["HK.00700"].Price.String() != "500.5" {
		t.Fatalf("QueryTickers = %#v, err=%v", ticks, err)
	}
	candles, err := provider.GetHistoricalCandles(
		t.Context(), "US", "AAPL", "1d", 2, "2026-08-01T00:00:00Z", "2026-08-03T00:00:00Z",
	)
	rows, _ := candles["candles"].([]map[string]any)
	if err != nil || len(rows) != 2 || rows[0]["open"] != "185.1" || rows[0]["volume"] != "1000" ||
		candles["totalReturned"] != 2 {
		t.Fatalf("GetHistoricalCandles = %#v, err=%v", candles, err)
	}
}

func TestProviderPreservesErrorsAndPartialBatchResults(t *testing.T) {
	var mode string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/providers/akshare/health":
			if mode == "warming" {
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_RUNTIME_WARMING","message":"warming"}}`))
				return
			}
			_, _ = writer.Write([]byte(`{"ok":true,"provider":"akshare","provider_version":"1.18.81","runtime_state":"ready"}`))
		case strings.HasPrefix(request.URL.Path, "/providers/akshare/security/"):
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"missing"}}`))
		case strings.HasPrefix(request.URL.Path, "/providers/akshare/candles/"):
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"code":"UNSUPPORTED_RANGE","message":"outside retention"}}`))
		case request.URL.Path == "/providers/akshare/snapshots":
			_, _ = writer.Write([]byte(`{"entries":[` + snapshotFixture("US", "AAPL", "189.25") +
				`],"errors":[{"instrument_id":"US.MISSING","code":"NOT_FOUND","message":"missing"}]}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"missing"}}`))
		}
	}))
	defer server.Close()
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	provider.client.maxAttempts = 1

	lookup, err := provider.LookupInstrument(t.Context(), "US", "MISSING")
	if err != nil || len(lookup) != 0 {
		t.Fatalf("not-found lookup = %#v, err=%v", lookup, err)
	}
	if _, err := provider.GetHistoricalCandles(t.Context(), "US", "AAPL", "1m", 5, "", ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported range error = %v", err)
	}
	ticks, err := provider.QueryTickers(t.Context(), []string{"US.AAPL", "US.MISSING"})
	if err != nil || len(ticks) != 1 || ticks["US.AAPL"].Price.String() != "189.25" {
		t.Fatalf("partial QueryTickers = %#v, err=%v", ticks, err)
	}
	mode = "warming"
	if _, err := provider.Health(t.Context()); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming health error = %v", err)
	}
}

func TestProviderNormalizesIndexAndExchangeIdentities(t *testing.T) {
	provider, err := NewProvider("http://127.0.0.1:7788")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	for _, test := range []struct {
		input map[string]any
		want  string
	}{
		{map[string]any{"market": "US", "symbol": ".spx"}, "US..SPX"},
		{map[string]any{"instrumentId": "US..DJI"}, "US..DJI"},
		{map[string]any{"market": "HK", "code": "700"}, "HK.00700"},
		{map[string]any{"symbol": "600519.SS"}, "SH.600519"},
		{map[string]any{"symbol": "000001.SZ"}, "SZ.000001"},
	} {
		normalized, err := provider.NormalizeInstrument(t.Context(), test.input)
		if err != nil || normalized["instrumentId"] != test.want {
			t.Fatalf("NormalizeInstrument(%#v) = %#v, err=%v", test.input, normalized, err)
		}
	}
	for _, input := range []map[string]any{
		{"market": "JP", "symbol": "7203"},
		{"market": "HK", "symbol": "BAD/SYMBOL"},
		{"market": "SH", "symbol": "123"},
		{"market": "US", "symbol": "AAPL", "code": "MSFT"},
		{"market": 12, "symbol": "AAPL"},
	} {
		if _, err := provider.NormalizeInstrument(t.Context(), input); err == nil {
			t.Fatalf("NormalizeInstrument(%#v) unexpectedly succeeded", input)
		}
	}
	if _, err := provider.GetHistoricalCandles(t.Context(), "US", "AAPL", "2m", 10, "", ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported period error = %v", err)
	}
}

func TestProviderChunksBatchSnapshotsAtContractLimit(t *testing.T) {
	var mu sync.Mutex
	batchSizes := make([]int, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input remoteBatchRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		batchSizes = append(batchSizes, len(input.InstrumentIDs))
		mu.Unlock()
		entries := make([]string, 0, len(input.InstrumentIDs))
		for _, id := range input.InstrumentIDs {
			parts := strings.SplitN(id, ".", 2)
			entries = append(entries, snapshotFixture(parts[0], parts[1], "1"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"entries":[%s],"errors":[]}`, strings.Join(entries, ","))
	}))
	defer server.Close()
	provider, err := NewProvider(server.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ids := make([]string, 0, 101)
	for index := range 101 {
		ids = append(ids, fmt.Sprintf("US.SYM%03d", index))
	}
	ticks, err := provider.QueryTickers(t.Context(), ids)
	if err != nil || len(ticks) != 101 {
		t.Fatalf("QueryTickers = %d, err=%v", len(ticks), err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(batchSizes, []int{100, 1}) {
		t.Fatalf("batch sizes = %#v", batchSizes)
	}
}

func newContractServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/providers/akshare/health":
			_, _ = writer.Write([]byte(`{"ok":true,"provider":"akshare","provider_version":"1.18.81","runtime_state":"ready","warmup_error":null}`))
		case "/providers/akshare/markets":
			_, _ = writer.Write([]byte(marketsFixture()))
		case "/providers/akshare/search":
			_, _ = writer.Write([]byte(`{"entries":[` + instrumentFixture() + `]}`))
		case "/providers/akshare/security/US/AAPL":
			_, _ = writer.Write([]byte(securityFixture()))
		case "/providers/akshare/snapshot/US/AAPL":
			_, _ = writer.Write([]byte(snapshotFixture("US", "AAPL", "189.25")))
		case "/providers/akshare/snapshots":
			_, _ = writer.Write([]byte(`{"entries":[` + snapshotFixture("US", "AAPL", "189.25") + `,` +
				snapshotFixture("HK", "00700", "500.5") + `],"errors":[]}`))
		case "/providers/akshare/candles/US/AAPL":
			_, _ = writer.Write([]byte(candlesFixture()))
		default:
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"missing fixture"}}`))
		}
	}))
}

func instrumentFixture() string {
	periods, _ := json.Marshal(candlePeriodOrder)
	return `{"market":"US","resolved_market":"US","instrument_id":"US.AAPL","code":"AAPL",` +
		`"symbol":"AAPL","name":"Apple","security_type":"stock","exchange":"NASDAQ",` +
		`"selectable":true,"source":"akshare:eastmoney","supported_periods":` + string(periods) + `}`
}

func securityFixture() string {
	periods, _ := json.Marshal(candlePeriodOrder)
	return `{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","name":"Apple",` +
		`"exchange":"NASDAQ","currency":"USD","timezone":"America/New_York",` +
		`"security_type":"stock","market_cap":"3000000000000","average_volume":"55000000",` +
		`"source":"akshare:eastmoney","supported_periods":` + string(periods) + `}`
}

func snapshotFixture(marketValue, symbol, price string) string {
	return fmt.Sprintf(`{"market":%q,"symbol":%q,"instrument_id":%q,"price":%q,`+
		`"bid":null,"ask":null,"open_price":%q,"high_price":%q,"low_price":%q,`+
		`"previous_close_price":%q,"last_close_price":null,"volume":"123456700",`+
		`"turnover":null,"quote_at":null,"observed_at":"2026-08-03T10:30:00Z",`+
		`"source":"akshare:eastmoney"}`,
		marketValue, symbol, marketValue+"."+symbol, price, price, price, price, price)
}

func candlesFixture() string {
	return `{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL","period":"1d",` +
		`"extended_hours":false,"total_returned":2,"source":"akshare:eastmoney","candles":[` +
		`{"at":"2026-08-01T04:00:00Z","open":"185.1","high":"188.2","low":"184.5","close":"187.8","volume":"1000"},` +
		`{"at":"2026-08-02T04:00:00Z","open":"187.8","high":"190.1","low":"186.9","close":"189.25","volume":null}]}`
}

func marketsFixture() string {
	profiles := make([]string, 0, 4)
	for _, value := range []struct {
		code, resolved, currency, timezone string
	}{
		{"US", "US", "USD", "America/New_York"},
		{"HK", "HK", "HKD", "Asia/Hong_Kong"},
		{"SH", "CN", "CNY", "Asia/Shanghai"},
		{"SZ", "CN", "CNY", "Asia/Shanghai"},
	} {
		profiles = append(profiles, fmt.Sprintf(`{"code":%q,"resolved_market":%q,"preferred_prefix":%q,`+
			`"display_name":%q,"quote_currency":%q,"timezone":%q,"supports_extended_hours":false,`+
			`"requires_exchange_prefix":false,"aliases":[],"regular_sessions":[`+
			`{"start_minute":570,"end_minute":960,"label":"regular"}],`+
			`"precision":{"price":2,"quote":2},"tick_size":0.01}`,
			value.code, value.resolved, value.code, value.code, value.currency, value.timezone))
	}
	return `{"markets":[` + strings.Join(profiles, ",") + `]}`
}

func TestClientRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "127.0.0.1:7788", "file:///tmp/helper"} {
		if _, err := NewClient(endpoint, nil); err == nil {
			t.Fatalf("NewClient(%q) unexpectedly succeeded", endpoint)
		}
	}
	var nilProvider *Provider
	if policy := nilProvider.QuotePollingPolicy(); policy != (marketdata.QuotePollingPolicy{}) {
		t.Fatalf("nil provider policy = %#v", policy)
	}
	if got := nilProvider.currentTime(); time.Since(got) > time.Second {
		t.Fatalf("nil provider currentTime = %s", got)
	}
	if _, err := (*Client)(nil).markets(context.Background()); !errors.Is(err, ErrSidecarUnavailable) {
		t.Fatalf("nil client error = %v", err)
	}
}
