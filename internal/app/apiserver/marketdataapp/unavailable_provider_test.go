package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestUnavailableProviderRejectsAllMarketDataOperations(t *testing.T) {
	ctx := context.Background()
	provider := newUnavailableProvider("custom", nil)
	descriptor, err := provider.Descriptor(ctx)
	if err != nil || descriptor.SelectionID != "custom" || descriptor.ProviderID != "custom" {
		t.Fatalf("custom unavailable descriptor = %#v, err=%v", descriptor, err)
	}

	assertUnavailable := func(name string, call func() error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "custom market-data provider is unavailable") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	assertUnavailable("markets", func() error {
		_, err := provider.GetMarkets(ctx)
		return err
	})
	assertUnavailable("normalize", func() error {
		_, err := provider.NormalizeInstrument(ctx, map[string]any{"instrumentId": "US.AAPL"})
		return err
	})
	assertUnavailable("security details", func() error {
		_, err := provider.GetSecurityDetails(ctx, "US", "AAPL")
		return err
	})
	assertUnavailable("lookup", func() error {
		_, err := provider.LookupInstrument(ctx, "US", "AAPL")
		return err
	})
	assertUnavailable("search", func() error {
		_, err := provider.SearchInstruments(ctx, "apple", 5)
		return err
	})
	assertUnavailable("snapshot", func() error {
		_, err := provider.QuerySnapshot(ctx, "US.AAPL")
		return err
	})
	assertUnavailable("ticker", func() error {
		_, err := provider.QueryTicker(ctx, "US.AAPL")
		return err
	})
	assertUnavailable("candles", func() error {
		_, err := provider.GetHistoricalCandles(ctx, marketdata.HistoricalCandlesQuery{Market: "US", Symbol: "AAPL"})
		return err
	})
	assertUnavailable("depth", func() error {
		_, err := provider.GetDepth(ctx, "US", "AAPL", 5)
		return err
	})

	health, err := provider.Health(ctx)
	if err != nil || health.Connected || health.Readiness != marketdata.ProviderReadinessFailed ||
		!strings.Contains(health.LastError, "provider activation failed") {
		t.Fatalf("unavailable health = %#v, err=%v", health, err)
	}
}

func TestRuntimeUnavailableProviderGuardsAndRecordsRetry(t *testing.T) {
	var nilRuntime *Runtime
	nilRuntime.MarkProviderUnavailable(ProviderAKShare, errors.New("ignored"))
	if nilRuntime.NeedsProviderActivation(ProviderAKShare) {
		t.Fatal("nil runtime reported provider activation is needed")
	}

	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	runtime.unavailable = nil
	runtime.MarkProviderUnavailable(" AKSHARE ", nil)
	if runtime.ActiveProviderID() != ProviderAKShare ||
		!runtime.NeedsProviderActivation(ProviderAKShare) {
		t.Fatalf("recorded unavailable provider = %q, needs retry=%v",
			runtime.ActiveProviderID(), runtime.NeedsProviderActivation(ProviderAKShare))
	}

	runtime.MarkProviderUnavailable(ProviderFutu, errors.New("ignored"))
	if runtime.ActiveProviderID() != ProviderAKShare {
		t.Fatalf("Futu guard changed unavailable provider = %q", runtime.ActiveProviderID())
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("runtime.Close: %v", err)
	}
	runtime.MarkProviderUnavailable(ProviderYFinance, errors.New("ignored"))
	if runtime.ActiveProviderID() != ProviderAKShare {
		t.Fatalf("closed runtime changed unavailable provider = %q", runtime.ActiveProviderID())
	}
}
