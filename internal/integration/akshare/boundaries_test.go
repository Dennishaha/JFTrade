package akshare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestClientResponseAndErrorBoundaries(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(`{`), []byte(`{} {}`)} {
		var target map[string]any
		if err := decodeResponse(body, &target); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("decodeResponse(%q) error = %v", body, err)
		}
	}
	var target map[string]any
	if err := decodeResponse([]byte(`{"ok":true}`), &target); err != nil || target["ok"] != true {
		t.Fatalf("decodeResponse valid = %#v, err=%v", target, err)
	}

	structured := decodeHTTPError(
		http.StatusBadRequest,
		[]byte(`{"error":{"code":"BAD_INPUT","message":"invalid"}}`),
	)
	var remoteErr *HTTPError
	if !errors.As(structured, &remoteErr) || remoteErr.Code != "BAD_INPUT" ||
		remoteErr.Error() != "AKShare sidecar returned HTTP 400 (BAD_INPUT): invalid" ||
		remoteErr.Unwrap() != nil {
		t.Fatalf("structured HTTP error = %#v / %v", remoteErr, structured)
	}
	serverErr := &HTTPError{StatusCode: http.StatusBadGateway, Message: "upstream"}
	if !errors.Is(serverErr, ErrSidecarUnavailable) ||
		serverErr.Error() != "AKShare sidecar returned HTTP 502: upstream" {
		t.Fatalf("server HTTP error = %v", serverErr)
	}
	if got := (&HTTPError{StatusCode: 404}).Error(); got != "AKShare sidecar returned HTTP 404" {
		t.Fatalf("status-only error = %q", got)
	}
	if got := (*HTTPError)(nil).Error(); got != "" {
		t.Fatalf("nil error string = %q", got)
	}
	plain := decodeHTTPError(http.StatusTeapot, []byte(strings.Repeat("x", 600)))
	if !errors.As(plain, &remoteErr) || len(remoteErr.Message) != 512 {
		t.Fatalf("plain HTTP error = %#v", remoteErr)
	}
	empty := decodeHTTPError(http.StatusNotFound, nil)
	if !errors.As(empty, &remoteErr) || remoteErr.Message != http.StatusText(http.StatusNotFound) {
		t.Fatalf("empty HTTP error = %#v", remoteErr)
	}
}

func TestClientRetryAndTransportBoundaries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		if request.URL.Path == "/retry" && attempts == 1 {
			writer.Header().Set("Retry-After", "0")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"error":{"code":"BUSY","message":"busy"}}`))
			return
		}
		if request.URL.Path == "/large" {
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(bytes.Repeat([]byte{'x'}, maxResponseBytes+1))
			return
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	var response map[string]any
	if err := client.get(t.Context(), []string{"retry"}, nil, &response); err != nil || attempts != 2 {
		t.Fatalf("retry request = %#v attempts=%d err=%v", response, attempts, err)
	}
	if err := client.get(t.Context(), []string{"large"}, nil, &response); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("oversized response error = %v", err)
	}
	if err := client.post(t.Context(), []string{"post"}, make(chan int), &response); err == nil {
		t.Fatal("post accepted a non-JSON request")
	}

	transportErr := errors.New("dial failed")
	client, err = NewClient("http://example.test", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, transportErr }),
	})
	if err != nil {
		t.Fatalf("NewClient custom transport: %v", err)
	}
	client.maxAttempts = 1
	if err := client.get(t.Context(), []string{"health"}, nil, &response); !errors.Is(err, ErrSidecarUnavailable) || !errors.Is(err, transportErr) {
		t.Fatalf("transport error = %v", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForRetry(canceled, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled retry wait = %v", err)
	}
	if err := waitForRetry(t.Context(), 0); err != nil {
		t.Fatalf("zero retry wait = %v", err)
	}
	if got := retryWait(http.Header{"Retry-After": {"5"}}, time.Second, 1); got != maxRetryDelay {
		t.Fatalf("Retry-After cap = %s", got)
	}
	if got := retryWait(http.Header{}, 100*time.Millisecond, 2); got != 200*time.Millisecond {
		t.Fatalf("incremental retry = %s", got)
	}
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests, 500} {
		if !isRetryableStatus(status) {
			t.Fatalf("status %d is not retryable", status)
		}
	}
	if isRetryableStatus(http.StatusBadRequest) {
		t.Fatal("400 is retryable")
	}
}

func TestClientDoesNotRetryExplicitPoolBackpressure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_POOL_BUSY","message":"busy"}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.retryDelay = 0
	var response map[string]any
	err = client.get(t.Context(), []string{"snapshot"}, nil, &response)
	if !errors.Is(err, marketdata.ErrProviderBusy) || attempts != 1 {
		t.Fatalf("pool backpressure error=%v attempts=%d", err, attempts)
	}
}

func TestClientHealthContractBoundaries(t *testing.T) {
	responses := []string{
		`{"ok":true,"provider_version":"","runtime_state":"ready"}`,
		`{"ok":true,"provider_version":"1.18.91","runtime_state":"unknown"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(responses[0]))
		responses = responses[1:]
	}))
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for range 2 {
		if _, err := client.health(t.Context()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid health error = %v", err)
		}
	}

	for _, input := range []remoteHealth{
		{OK: true, AKShareVersion: "1.18.91", RuntimeState: "ready"},
		{OK: true, ProviderVersion: "1.18.91", RuntimeState: "warming"},
		{OK: false, Version: "1.18.91", RuntimeState: "failed"},
	} {
		body, _ := json.Marshal(input)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(body)
		}))
		client, _ := NewClient(server.URL, nil)
		if _, err := client.health(t.Context()); err != nil {
			t.Fatalf("health variant %#v: %v", input, err)
		}
		server.Close()
	}
}

func TestConversionRejectsInvalidMarketAndInstrumentContracts(t *testing.T) {
	if _, err := convertMarkets(nil); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("empty markets error = %v", err)
	}
	valid := validMarketProfile()
	if _, err := convertMarkets([]remoteMarketProfile{valid, valid}); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("duplicate markets error = %v", err)
	}
	for _, mutate := range []func(*remoteMarketProfile){
		func(value *remoteMarketProfile) { value.Code = "JP" },
		func(value *remoteMarketProfile) { value.PreferredPrefix = "HK" },
		func(value *remoteMarketProfile) { value.ResolvedMarket = "CN" },
		func(value *remoteMarketProfile) { value.RegularSessions[0].EndMinute = 0 },
		func(value *remoteMarketProfile) { value.TickSize = number("-1") },
	} {
		profile := validMarketProfile()
		mutate(&profile)
		if _, err := convertMarkets([]remoteMarketProfile{profile}); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid market %#v error = %v", profile, err)
		}
	}

	validInstrument := remoteInstrument{
		Market: "US", ResolvedMarket: "US", InstrumentID: "US.AAPL", Code: "AAPL", Symbol: "AAPL",
		Name: "Apple", Selectable: true, SupportedPeriods: append([]string(nil), candlePeriodOrder...),
	}
	if entries, err := convertCandidates([]remoteInstrument{validInstrument, validInstrument}); err != nil || len(entries) != 1 {
		t.Fatalf("candidate dedup = %#v, err=%v", entries, err)
	}
	for _, mutate := range []func(*remoteInstrument){
		func(value *remoteInstrument) { value.InstrumentID = "US.MSFT" },
		func(value *remoteInstrument) { value.Code = "MSFT" },
		func(value *remoteInstrument) { value.ResolvedMarket = "CN" },
		func(value *remoteInstrument) { value.SupportedPeriods = []string{"2m"} },
	} {
		entry := validInstrument
		mutate(&entry)
		if _, err := convertCandidates([]remoteInstrument{entry}); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid candidate %#v error = %v", entry, err)
		}
	}
	nonSelectable := validInstrument
	nonSelectable.Selectable = false
	entries, err := convertCandidates([]remoteInstrument{nonSelectable})
	if err != nil || entries[0].UnavailableReason == "" || entries[0].Source != sourceID {
		t.Fatalf("non-selectable candidate = %#v, err=%v", entries, err)
	}
}

func TestConversionRejectsInvalidSecurityAndSnapshotContracts(t *testing.T) {
	expected, err := normalizeIdentity("US", "AAPL", "")
	if err != nil {
		t.Fatalf("normalizeIdentity: %v", err)
	}
	security := remoteSecurity{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Name: "Apple",
		SupportedPeriods: []string{"1d"}, MarketCap: number("1"), AverageVolume: number("2"),
	}
	details, err := convertSecurity(security, expected, time.Now())
	if err != nil || details["meta"].(map[string]any)["source"] != sourceID {
		t.Fatalf("convertSecurity = %#v, err=%v", details, err)
	}
	for _, mutate := range []func(*remoteSecurity){
		func(value *remoteSecurity) { value.InstrumentID = "US.MSFT" },
		func(value *remoteSecurity) { value.MarketCap = number("-1") },
		func(value *remoteSecurity) { value.AverageVolume = number("bad") },
		func(value *remoteSecurity) { value.SupportedPeriods = []string{"2m"} },
	} {
		value := security
		mutate(&value)
		if _, err := convertSecurity(value, expected, time.Now()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid security %#v error = %v", value, err)
		}
	}

	valid := validRemoteSnapshot()
	tick, err := convertSnapshot(valid, expected, time.Now())
	if err != nil || tick.Source != sourceID || tick.Availability.Bid || tick.Availability.Volume {
		t.Fatalf("minimal snapshot = %#v, err=%v", tick, err)
	}
	for _, mutate := range []func(*remoteSnapshot){
		func(value *remoteSnapshot) { value.InstrumentID = "US.MSFT" },
		func(value *remoteSnapshot) { value.Price = nil },
		func(value *remoteSnapshot) { value.Bid = number("-1") },
		func(value *remoteSnapshot) { value.Ask = number("bad") },
		func(value *remoteSnapshot) { value.Volume = number("-1") },
		func(value *remoteSnapshot) { value.Turnover = number("bad") },
		func(value *remoteSnapshot) { value.OpenPrice = number("0") },
		func(value *remoteSnapshot) { value.HighPrice = number("1"); value.LowPrice = number("2") },
		func(value *remoteSnapshot) { value.PreviousClosePrice = number("-1") },
		func(value *remoteSnapshot) { value.LastClosePrice = number("bad") },
		func(value *remoteSnapshot) { value.ObservedAt = "bad" },
		func(value *remoteSnapshot) { value.QuoteAt = "bad" },
	} {
		value := valid
		mutate(&value)
		if _, err := convertSnapshot(value, expected, time.Now()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid snapshot %#v error = %v", value, err)
		}
	}
}

func TestConversionRejectsInvalidCandleContracts(t *testing.T) {
	expected, _ := normalizeIdentity("US", "AAPL", "")
	valid := remoteCandles{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Period: "1d",
		TotalReturned: 1, HasMore: new(false), Candles: []remoteCandle{validRemoteCandle()},
	}
	response, err := convertCandles(valid, expected, "1d", 1, time.Now())
	if err != nil || response["totalReturned"] != 1 {
		t.Fatalf("convertCandles = %#v, err=%v", response, err)
	}
	for _, mutate := range []func(*remoteCandles){
		func(value *remoteCandles) { value.InstrumentID = "US.MSFT" },
		func(value *remoteCandles) { value.ExtendedHours = true },
		func(value *remoteCandles) { value.TotalReturned = 2 },
	} {
		value := valid
		mutate(&value)
		if _, err := convertCandles(value, expected, "1d", 1, time.Now()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid candles %#v error = %v", value, err)
		}
	}
	for _, mutate := range []func(*remoteCandle){
		func(value *remoteCandle) { value.At = "bad" },
		func(value *remoteCandle) { value.Open = number("0") },
		func(value *remoteCandle) { value.High = number("1") },
		func(value *remoteCandle) { value.Volume = number("-1") },
	} {
		value := valid
		value.Candles = []remoteCandle{validRemoteCandle()}
		mutate(&value.Candles[0])
		if _, err := convertCandles(value, expected, "1d", 1, time.Now()); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid candle %#v error = %v", value.Candles[0], err)
		}
	}
}

func TestConversionRejectsInvalidCandlePaginationMetadata(t *testing.T) {
	expected, _ := normalizeIdentity("US", "AAPL", "")
	valid := remoteCandles{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Period: "1d",
		TotalReturned: 1, HasMore: new(false), Candles: []remoteCandle{validRemoteCandle()},
	}

	for _, test := range []struct {
		name   string
		limit  int
		mutate func(*remoteCandles)
	}{
		{
			name: "missing has more",
			mutate: func(value *remoteCandles) {
				value.HasMore = nil
			},
		},
		{
			name: "terminal cursor",
			mutate: func(value *remoteCandles) {
				value.NextBefore = value.Candles[0].At
			},
		},
		{
			name: "continued page missing cursor",
			mutate: func(value *remoteCandles) {
				value.HasMore = new(true)
			},
		},
		{
			name: "continued page cursor mismatches earliest candle",
			mutate: func(value *remoteCandles) {
				value.HasMore = new(true)
				value.NextBefore = "2026-08-01T04:01:00Z"
			},
		},
		{
			name:  "terminal page exceeds limit",
			limit: 1,
			mutate: func(value *remoteCandles) {
				value.TotalReturned = 2
				value.Candles = append(value.Candles, remoteCandle{
					At: "2026-08-02T04:00:00Z", Open: number("11"), High: number("13"),
					Low: number("10"), Close: number("12"), Volume: number("110"),
				})
			},
		},
		{
			name:  "timestamps are not strictly ordered",
			limit: 2,
			mutate: func(value *remoteCandles) {
				value.TotalReturned = 2
				value.Candles = append(value.Candles, value.Candles[0])
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			value.Candles = append([]remoteCandle(nil), valid.Candles...)
			test.mutate(&value)
			limit := test.limit
			if limit == 0 {
				limit = 10
			}
			if _, err := convertCandles(value, expected, "1d", limit, time.Now()); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("convertCandles error = %v", err)
			}
		})
	}

	continued := valid
	continued.HasMore = new(true)
	continued.NextBefore = continued.Candles[0].At
	response, err := convertCandles(continued, expected, "1d", 10, time.Now())
	if err != nil {
		t.Fatalf("valid continued page: %v", err)
	}
	if pagination := response["pagination"].(map[string]any); pagination["hasMore"] != true ||
		pagination["nextBefore"] != continued.Candles[0].At {
		t.Fatalf("continued pagination = %#v", pagination)
	}

	terminal, err := convertCandles(valid, expected, "1d", 10, time.Now())
	if err != nil {
		t.Fatalf("valid terminal page: %v", err)
	}
	if _, err := validateHistoricalCandleResponse(terminal, valid.Candles[0].At, "", ""); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("cursor boundary error = %v", err)
	}
	if _, err := validateHistoricalCandleResponse(response, "", "2026-08-01T00:00:00Z", ""); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("bounded continuation error = %v", err)
	}
}

func TestProviderInputAndUnavailableBoundaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte(`{"error":{"code":"AKSHARE_UPSTREAM_ERROR","message":"down"}}`))
	}))
	defer server.Close()
	provider, _ := NewProvider(server.URL)
	provider.client.maxAttempts = 1
	if _, err := provider.SearchInstruments(t.Context(), " ", 0); err == nil {
		t.Fatal("empty search succeeded")
	}
	if ticks, err := provider.QueryTickers(t.Context(), nil); err != nil || len(ticks) != 0 {
		t.Fatalf("empty QueryTickers = %#v, err=%v", ticks, err)
	}
	if _, err := provider.QueryTickers(t.Context(), []string{"bad"}); err == nil {
		t.Fatal("invalid batch identity succeeded")
	}
	for _, call := range []func() error{
		func() error { _, err := provider.GetMarkets(t.Context()); return err },
		func() error { _, err := provider.GetSecurityDetails(t.Context(), "US", "AAPL"); return err },
		func() error { _, err := provider.SearchInstruments(t.Context(), "apple", 5); return err },
		func() error { _, err := provider.QueryTicker(t.Context(), "US.AAPL"); return err },
		func() error { _, err := provider.QueryTickers(t.Context(), []string{"US.AAPL"}); return err },
	} {
		if err := call(); !errors.Is(err, ErrSidecarUnavailable) {
			t.Fatalf("unavailable provider error = %v", err)
		}
	}
}

func TestHelperBoundaries(t *testing.T) {
	if periods, err := normalizeSupportedPeriods([]string{"1d", "1D", "1m"}); err != nil ||
		len(periods) != 2 || periods[0] != "1m" || periods[1] != "1d" {
		t.Fatalf("normalizeSupportedPeriods = %#v, err=%v", periods, err)
	}
	if value, err := optionalNonNegativeNumber("value", nil); err != nil || value != nil {
		t.Fatalf("optional nil number = %#v, err=%v", value, err)
	}
	if _, err := positiveFloat("value", nil); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("nil positive float error = %v", err)
	}
	if got, err := responseTime("at", "", time.Unix(0, 0)); err != nil || got != "1970-01-01T00:00:00Z" {
		t.Fatalf("fallback time = %q, err=%v", got, err)
	}
	if source := defaultSource(" akshare:eastmoney "); source != "akshare:eastmoney" {
		t.Fatalf("defaultSource = %q", source)
	}
	if got := (&remoteBatchSnapshots{Snapshots: []remoteSnapshot{{Symbol: "AAPL"}}}).values(); len(got) != 1 {
		t.Fatalf("legacy batch values = %#v", got)
	}
	if got := batchRemoteErrors([]remoteBatchError{{InstrumentID: "US.AAPL"}}); len(got) != 1 ||
		!strings.Contains(got[0].Error(), "snapshot unavailable") {
		t.Fatalf("default batch error = %#v", got)
	}
	plain := errors.New("plain")
	if got := classifyRuntimeError(plain); !errors.Is(got, plain) {
		t.Fatal("plain runtime error was rewritten")
	}
}

func validMarketProfile() remoteMarketProfile {
	return remoteMarketProfile{
		Code: "US", ResolvedMarket: "US", PreferredPrefix: "US", DisplayName: "US",
		QuoteCurrency: "USD", Timezone: "America/New_York", TickSize: number("0.01"),
		RegularSessions: []remoteTradingWindow{{StartMinute: 570, EndMinute: 960, Label: "regular"}},
	}
}

func validRemoteSnapshot() remoteSnapshot {
	return remoteSnapshot{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Price: number("100"),
		ObservedAt: "2026-08-03T10:30:00Z",
	}
}

func validRemoteCandle() remoteCandle {
	return remoteCandle{
		At: "2026-08-01T04:00:00Z", Open: number("10"), High: number("12"),
		Low: number("9"), Close: number("11"), Volume: number("100"),
	}
}

func number(value string) *json.Number {
	number := json.Number(value)
	return &number
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClientRequestOnceReadFailure(t *testing.T) {
	readErr := errors.New("read failed")
	client, err := NewClient("http://example.test", &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK, Header: make(http.Header),
				Body: io.NopCloser(errorReader{err: readErr}),
			}, nil
		},
	)})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.maxAttempts = 1
	var response map[string]any
	if err := client.get(t.Context(), []string{"read"}, url.Values{}, &response); !errors.Is(err, ErrSidecarUnavailable) || !errors.Is(err, readErr) {
		t.Fatalf("read failure error = %v", err)
	}
}

type errorReader struct{ err error }

func (reader errorReader) Read([]byte) (int, error) { return 0, reader.err }

func TestRemainingProviderAndConversionBranches(t *testing.T) {
	if _, err := NewProvider("bad endpoint"); err == nil {
		t.Fatal("NewProvider accepted an invalid endpoint")
	}
	if err := waitForRetry(t.Context(), time.Millisecond); err != nil {
		t.Fatalf("positive retry wait = %v", err)
	}
	profile := validMarketProfile()
	profile.ResolvedMarket = ""
	profile.PreferredPrefix = ""
	profile.TickSize = nil
	if markets, err := convertMarkets([]remoteMarketProfile{profile}); err != nil || markets[0]["tickSize"] != nil {
		t.Fatalf("defaulted market = %#v, err=%v", markets, err)
	}
	entry := remoteInstrument{
		Market: "US", InstrumentID: "US.AAPL", Symbol: "AAPL", Name: "Apple", Selectable: true,
	}
	if entries, err := convertCandidates([]remoteInstrument{entry}); err != nil ||
		entries[0].ResolvedMarket != "US" || entries[0].Source != sourceID {
		t.Fatalf("defaulted candidate = %#v, err=%v", entries, err)
	}
	if !symbolMatchesCode("US.AAPL", "aapl", "") || normalizeLimit(0, 20, 100) != 20 ||
		normalizeLimit(200, 20, 100) != 100 || !isSupportedLeafMarket("hk") ||
		isSupportedLeafMarket("JP") {
		t.Fatal("identity helper boundary mismatch")
	}
	if instrument, err := normalizeIdentity("HK", "HSMAIN", ""); err != nil || instrument.id != "HK.HSMAIN" {
		t.Fatalf("HK catalog index identity = %#v, err=%v", instrument, err)
	}
	for _, marketValue := range []string{"CN", "NYSE", "HKEX", "SSE", "SZSE"} {
		if _, err := canonicalMarket(marketValue); err != nil {
			t.Fatalf("canonicalMarket(%q): %v", marketValue, err)
		}
	}
	if _, err := normalizeIdentity("US", "", ""); err == nil {
		t.Fatal("empty identity succeeded")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/providers/akshare/security/US/AAPL":
			_, _ = writer.Write([]byte(`{"market":"US","symbol":"MSFT","instrument_id":"US.MSFT",` +
				`"name":"Microsoft","timezone":"America/New_York","supported_periods":["1d"]}`))
		case "/providers/akshare/snapshot/US/AAPL":
			_, _ = writer.Write([]byte(`{"market":"US","symbol":"AAPL","instrument_id":"US.AAPL",` +
				`"price":null,"observed_at":"2026-08-03T10:30:00Z"}`))
		case "/providers/akshare/snapshots":
			value := snapshotFixture("US", "AAPL", "100")
			_, _ = writer.Write([]byte(`{"entries":[` + value + `,` + value + `,` +
				snapshotFixture("US", "MSFT", "100") + `],"errors":[]}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider, _ := NewProvider(server.URL)
	if _, err := provider.GetSecurityDetails(t.Context(), "US", "AAPL"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("mismatched security error = %v", err)
	}
	if _, err := provider.LookupInstrument(t.Context(), "JP", "7203"); err == nil {
		t.Fatal("invalid exact lookup succeeded")
	}
	if _, err := provider.QueryTicker(t.Context(), "US.AAPL"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("invalid ticker error = %v", err)
	}
	instrument, _ := normalizeIdentity("US", "AAPL", "")
	ticks, failures := provider.queryTickerBatch(t.Context(), []normalizedInstrument{instrument})
	if len(ticks) != 1 || len(failures) < 2 {
		t.Fatalf("duplicate/unexpected batch = ticks %#v failures %#v", ticks, failures)
	}
}
