package backtestapp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/shopspring/decimal"
)

type backtestProviderStub struct {
	marketdata.Provider
	descriptor    marketdata.ProviderDescriptor
	descriptorErr error
	candles       marketdata.CandlesResponse
	candlesErr    error
	details       marketdata.SecurityDetails
	detailsErr    error
	detailsCtx    context.Context
	lastQuery     marketdata.HistoricalCandlesQuery
}

func (s *backtestProviderStub) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	return s.descriptor, s.descriptorErr
}

func (s *backtestProviderStub) GetHistoricalCandles(
	_ context.Context,
	query marketdata.HistoricalCandlesQuery,
) (marketdata.CandlesResponse, error) {
	s.lastQuery = query
	return s.candles, s.candlesErr
}

func (s *backtestProviderStub) GetSecurityDetails(ctx context.Context, _ string, _ string) (marketdata.SecurityDetails, error) {
	s.detailsCtx = ctx
	return s.details, s.detailsErr
}

func newBacktestRuntime(t *testing.T, provider marketdata.Provider) *marketdataapp.Runtime {
	t.Helper()
	runtime, err := marketdataapp.NewRuntime(marketdataapp.RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func TestProviderHistoricalSourceMapsExtendedSessionsToProviderCapabilities(t *testing.T) {
	yahoo := &providerHistoricalSource{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "yfinance",
		Capabilities: marketdata.ProviderCapabilities{
			ExtendedHours: true,
			Sessions:      []string{"regular", "pre", "after", "closed"},
		},
	}}
	requested := []string{"regular", "extended", "overnight"}
	if got, want := yahoo.providerSessions(requested), []marketdata.CandleSession{"regular", "extended"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("yfinance sessions = %#v, want %#v", got, want)
	}

	futu := &providerHistoricalSource{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "futu",
		Capabilities: marketdata.ProviderCapabilities{
			ExtendedHours: true,
			Sessions:      []string{"RTH", "ETH", "ALL", "OVERNIGHT"},
		},
	}}
	if got, want := futu.providerSessions(requested), []marketdata.CandleSession{"regular", "extended", "overnight"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("futu sessions = %#v, want %#v", got, want)
	}
}

func TestProviderHistoricalSourceRejectsExtendedSessionsOutsideUSIntraday(t *testing.T) {
	source := &providerHistoricalSource{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "futu",
		Capabilities: marketdata.ProviderCapabilities{
			ExtendedHours: true, CandleIntervals: []string{"1m", "1d"},
			PriceAdjustments: []string{"none"},
		},
	}}
	query := backtestservice.HistoricalCandleQuery{
		Market: "HK", Interval: "1m", Adjustment: backtestservice.RehabTypeNone,
		Sessions: []string{"regular", "extended"}, Since: time.Now().UTC().Add(-time.Hour),
	}
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("HK extended-session validation error = nil")
	}
	query.Market, query.Interval = "US", "1d"
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("US daily extended-session validation error = nil")
	}
}

func TestProviderHistoricalSourceValidatesAdjustmentAndLookback(t *testing.T) {
	source := &providerHistoricalSource{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "yfinance",
		Capabilities: marketdata.ProviderCapabilities{
			CandleIntervals:        []string{"1m"},
			PriceAdjustments:       []string{"none"},
			HistoricalLookbackDays: map[string]int{"1m": 7},
		},
	}}
	query := backtestservice.HistoricalCandleQuery{Interval: "1m", Adjustment: backtestservice.RehabTypeForward}
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("forward adjustment validation error = nil")
	}
	query.Adjustment = backtestservice.RehabTypeNone
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("historical lookback validation error = nil")
	}
	query.Since = time.Now().UTC()
	query.Interval = "5m"
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("unsupported interval validation error = nil")
	}
	query.Interval = "1m"
	query.Sessions = []string{"regular", "extended"}
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("unsupported extended-session validation error = nil")
	}
	query.Sessions = []string{"regular"}
	if err := source.ValidateHistoricalCandleQuery(query); err != nil {
		t.Fatalf("supported historical query rejected: %v", err)
	}
}

func TestProviderHistoricalSourceAppliesMarketScopedLookback(t *testing.T) {
	source := &providerHistoricalSource{descriptor: marketdata.ProviderDescriptor{
		SelectionID: "akshare",
		Capabilities: marketdata.ProviderCapabilities{
			CandleIntervals: []string{"5m"}, PriceAdjustments: []string{"none"},
			HistoricalLookbackDays: map[string]int{"US:5m": 5},
		},
	}}
	query := backtestservice.HistoricalCandleQuery{
		Market: "US", Interval: "5m", Adjustment: backtestservice.RehabTypeNone,
		Sessions: []string{"regular"}, Since: time.Now().UTC().AddDate(0, 0, -6),
	}
	if err := source.ValidateHistoricalCandleQuery(query); err == nil {
		t.Fatal("AKShare US 5m request beyond five days was accepted")
	}
	query.Market = "HK"
	if err := source.ValidateHistoricalCandleQuery(query); err != nil {
		t.Fatalf("AKShare HK 5m request inherited US-only limit: %v", err)
	}
}

func TestKLineSyncPreflightRejectsStaticCapabilityMismatchAndUnknownProvider(t *testing.T) {
	runtime := newBacktestRuntime(t, &backtestProviderStub{})
	preflight := NewKLineSyncPreflight(runtime)
	now := time.Now().UTC()
	err := preflight(t.Context(), backtestservice.KLineSyncParams{
		Market: "US", MarketDataProvider: marketdataapp.ProviderAKShare, Symbol: "US.AAPL",
		Intervals: []bbgotypes.Interval{bbgotypes.Interval5m}, Since: now.AddDate(0, 0, -6),
		Until: now, RehabType: backtestservice.RehabTypeNone, SessionScope: "regular",
	})
	if !backtestservice.IsRequestError(err) ||
		!strings.Contains(err.Error(), "provider akshare limits 5m history to 5 days") {
		t.Fatalf("AKShare preflight error = %v", err)
	}

	err = preflight(t.Context(), backtestservice.KLineSyncParams{MarketDataProvider: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "market-data provider unknown is unavailable") {
		t.Fatalf("unknown provider preflight error = %v", err)
	}
}

func TestProviderHistoricalSourceFetchesAndParsesProviderPage(t *testing.T) {
	at := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	provider := &backtestProviderStub{candles: marketdata.CandlesResponse{
		"candles": []map[string]any{{
			"at": at.Format(time.RFC3339Nano), "open": json.Number("100.25"),
			"high": decimal.RequireFromString("102.5"), "low": float32(99.5),
			"close": float64(101.75), "volume": int64(1200), "session": "regular",
		}},
		"pagination": map[string]any{"hasMore": true, "nextBefore": at.Format(time.RFC3339Nano)},
	}}
	source := &providerHistoricalSource{provider: provider, descriptor: marketdata.ProviderDescriptor{
		SelectionID: "futu",
		Capabilities: marketdata.ProviderCapabilities{
			CandleIntervals: []string{"1m"}, PriceAdjustments: []string{"none"},
			ExtendedHours: true, Sessions: []string{"regular", "overnight"},
		},
	}}
	page, err := source.FetchHistoricalCandles(t.Context(), backtestservice.HistoricalCandleQuery{
		Market: "US", Symbol: "US.AAPL", Interval: "1m", Adjustment: backtestservice.RehabTypeNone,
		Before: at.Add(time.Minute), Since: at.Add(-time.Hour), Sessions: []string{"regular", "overnight"}, Limit: 25,
	})
	if err != nil {
		t.Fatalf("FetchHistoricalCandles: %v", err)
	}
	if len(page.Candles) != 1 || page.Candles[0].Close != "101.75" || page.Candles[0].Volume != "1200" ||
		!page.HasMore || !page.NextBefore.Equal(at) {
		t.Fatalf("historical page = %+v", page)
	}
	if provider.lastQuery.Adjustment != "none" || len(provider.lastQuery.Sessions) != 2 || provider.lastQuery.Limit != 25 {
		t.Fatalf("provider query = %+v", provider.lastQuery)
	}
	provider.candlesErr = errors.New("provider unavailable")
	if _, err := source.FetchHistoricalCandles(t.Context(), backtestservice.HistoricalCandleQuery{
		Market: "US", Symbol: "US.AAPL", Interval: "1m", Adjustment: backtestservice.RehabTypeNone,
		Before: at, Since: at.Add(-time.Hour), Sessions: []string{"regular"},
	}); !errors.Is(err, provider.candlesErr) {
		t.Fatalf("provider fetch error = %v", err)
	}
}

func TestHistoricalPageParsingRejectsMalformedProviderValues(t *testing.T) {
	at := time.Now().UTC().Format(time.RFC3339Nano)
	tests := []struct {
		name     string
		response marketdata.CandlesResponse
	}{
		{name: "candles", response: marketdata.CandlesResponse{"candles": "bad"}},
		{name: "timestamp", response: marketdata.CandlesResponse{"candles": []map[string]any{{"at": "bad"}}}},
		{name: "open", response: historicalPageFixture(at, struct{}{}, 2, 1, 2, nil)},
		{name: "high", response: historicalPageFixture(at, 1, struct{}{}, 1, 2, nil)},
		{name: "low", response: historicalPageFixture(at, 1, 2, struct{}{}, 2, nil)},
		{name: "close", response: historicalPageFixture(at, 1, 2, 1, struct{}{}, nil)},
		{name: "volume", response: historicalPageFixture(at, 1, 2, 1, 2, struct{}{})},
		{name: "cursor", response: marketdata.CandlesResponse{
			"candles": []map[string]any{}, "pagination": map[string]any{"nextBefore": "bad"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseHistoricalPage(test.response); err == nil {
				t.Fatalf("parseHistoricalPage(%s) error = nil", test.name)
			}
		})
	}
}

func historicalPageFixture(at string, open, high, low, closeValue, volume any) marketdata.CandlesResponse {
	return marketdata.CandlesResponse{"candles": []map[string]any{{
		"at": at, "open": open, "high": high, "low": low, "close": closeValue, "volume": volume,
	}}}
}

func TestDecimalStringAcceptsProviderNumericRepresentations(t *testing.T) {
	values := []any{"1.25", json.Number("2.5"), decimal.RequireFromString("3.75"), float64(4.5), float32(5.25), int(6), int64(7)}
	for _, value := range values {
		if result, err := decimalString(value); err != nil || result == "" {
			t.Fatalf("decimalString(%T) = %q, %v", value, result, err)
		}
	}
	if _, err := decimalString(true); err == nil {
		t.Fatal("decimalString(bool) error = nil")
	}
}

func TestBacktestProviderSyncerPinsFutuAndClosesOnFailures(t *testing.T) {
	at := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	provider := &backtestProviderStub{
		descriptor: marketdata.ProviderDescriptor{SelectionID: "futu", Capabilities: marketdata.ProviderCapabilities{
			HistoricalCandles: true, CandleIntervals: []string{"1m"}, PriceAdjustments: []string{"none"},
		}},
		candles: historicalPageFixture(at.Format(time.RFC3339Nano), "1", "2", "1", "2", 10),
	}
	runtime := newBacktestRuntime(t, provider)
	syncer, err := NewKLineSyncer(t.Context(), runtime, filepath.Join(t.TempDir(), "backtest.db"), "futu")
	if err != nil {
		t.Fatalf("NewKLineSyncer: %v", err)
	}
	if err := syncer.Sync(t.Context(), backtestservice.KLineSyncParams{
		Market: "US", MarketDataProvider: "futu", Symbol: "US.AAPL", Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
		Since: at, Until: at.Add(time.Minute), RehabType: backtestservice.RehabTypeNone, SessionScope: "regular",
	}, nil); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := syncer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	provider.descriptor.Capabilities.HistoricalCandles = false
	if _, err := NewKLineSyncer(t.Context(), runtime, filepath.Join(t.TempDir(), "unsupported.db"), "futu"); err == nil {
		t.Fatal("NewKLineSyncer without historical capability error = nil")
	}
	provider.descriptorErr = errors.New("descriptor unavailable")
	if _, err := NewKLineSyncer(t.Context(), runtime, filepath.Join(t.TempDir(), "descriptor.db"), "futu"); err == nil {
		t.Fatal("NewKLineSyncer descriptor error = nil")
	}
	provider.descriptorErr = nil
	provider.descriptor.Capabilities.HistoricalCandles = true
	if _, err := NewKLineSyncer(t.Context(), runtime, t.TempDir(), "futu"); err == nil {
		t.Fatal("NewKLineSyncer invalid database path error = nil")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close: %v", err)
	}
	if _, err := NewKLineSyncer(t.Context(), runtime, filepath.Join(t.TempDir(), "closed.db"), "futu"); !errors.Is(err, marketdataapp.ErrRuntimeClosed) {
		t.Fatalf("NewKLineSyncer closed runtime error = %v", err)
	}
}

func TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks(t *testing.T) {
	provider := &backtestProviderStub{descriptor: marketdata.ProviderDescriptor{SelectionID: "futu"}, details: marketdata.SecurityDetails{
		"security": map[string]any{"currency": "hkd", "lotSize": "500", "priceSpread": float32(0.2)},
	}}
	runtime := newBacktestRuntime(t, provider)
	resolver := NewInstrumentSpecResolver(runtime)
	spec, err := resolver(t.Context(), "futu", "HK", "HK.00700")
	if err != nil || spec.QuoteCurrency != "HKD" || spec.LotSize != 500 || spec.QuantityStep != 500 ||
		spec.TickSize < 0.19 || spec.TickSize > 0.21 || spec.MissingCriticalRules || len(spec.Warnings) != 0 {
		t.Fatalf("resolved instrument spec = %+v, %v", spec, err)
	}
	if deadline, ok := provider.detailsCtx.Deadline(); !ok || time.Until(deadline) < 10*time.Second {
		t.Fatalf("instrument rule lookup deadline = %v, %t", deadline, ok)
	}

	provider.detailsErr = errors.New("rules unavailable")
	spec, err = ResolveInstrumentSpec(t.Context(), runtime, "futu", "HK", "HK.00700")
	if err != nil || !spec.MissingCriticalRules || len(spec.Warnings) == 0 {
		t.Fatalf("HK conservative spec = %+v, %v", spec, err)
	}
	if sh := defaultInstrumentSpec("SH.600000"); sh.LotSize != 100 || sh.QuantityStep != 100 {
		t.Fatalf("A-share conservative spec = %+v", sh)
	}
	if us := conservativeInstrumentSpec(backtestserviceInstrumentSpec("US.AAPL")); us.LotSize != 1 || us.TickSize != 0.01 {
		t.Fatalf("US conservative spec = %+v", us)
	}
	if options := ProviderOptions(runtime, func() string { return "db" }, func() string { return "futu" }); len(options) != 6 {
		t.Fatalf("provider options = %d, want 6", len(options))
	}
}

func TestInstrumentSpecRequiresReadyPythonProviders(t *testing.T) {
	t.Parallel()
	for _, providerID := range []string{
		marketdataapp.ProviderYFinance,
		" YFINANCE ",
		marketdataapp.ProviderAKShare,
	} {
		if !instrumentRulesRequireReady(providerID) {
			t.Errorf("provider %q did not require ready instrument rules", providerID)
		}
	}
	if instrumentRulesRequireReady(marketdataapp.ProviderFutu) {
		t.Fatal("Futu instrument rules unexpectedly require provider readiness")
	}
}

func TestProviderOptionsRequireMarketDataRuntime(t *testing.T) {
	defer func() {
		if got := recover(); got != "assemble backtest service: market-data provider runtime is unavailable" {
			t.Fatalf("ProviderOptions panic = %v", got)
		}
	}()
	ProviderOptions(nil, func() string { return "db" }, func() string { return "futu" })
}

func backtestserviceInstrumentSpec(symbol string) backteststore.InstrumentSpec {
	return backteststore.InstrumentSpec{Symbol: symbol}
}

func TestPositiveFloatRecognizesSupportedRuleTypes(t *testing.T) {
	for _, value := range []any{float64(1), float32(2), int(3), int32(4), int64(5), "6.5"} {
		if result, ok := positiveFloat(value); !ok || result <= 0 {
			t.Fatalf("positiveFloat(%T) = %v, %v", value, result, ok)
		}
	}
	for _, value := range []any{0, "bad", true} {
		if _, ok := positiveFloat(value); ok {
			t.Fatalf("positiveFloat(%v) accepted", value)
		}
	}
}
