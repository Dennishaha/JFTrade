package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

type newsCapableForwardingProviderStub struct {
	forwardingProviderStub
	newsMarket  string
	newsSymbol  string
	newsLimit   int
	actionsFrom time.Time
	actionsTo   time.Time
	newsErr     error
}

func (p *newsCapableForwardingProviderStub) News(
	_ context.Context,
	market string,
	symbol string,
	limit int,
) (marketdata.NewsResponse, error) {
	p.record("News")
	p.newsMarket, p.newsSymbol, p.newsLimit = market, symbol, limit
	return marketdata.NewsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Entries: []marketdata.NewsEntry{}, Source: "stub-news",
	}, p.newsErr
}

func (p *newsCapableForwardingProviderStub) CorporateActions(
	_ context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (marketdata.CorporateActionsResponse, error) {
	p.record("CorporateActions")
	p.actionsFrom, p.actionsTo = from, to
	return marketdata.CorporateActionsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Events: []marketdata.CorporateActionEvent{}, Source: "stub-actions",
	}, p.newsErr
}

func TestRuntimeForwardsNewsAndCorporateActionsToCapableActiveProvider(t *testing.T) {
	provider := &newsCapableForwardingProviderStub{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	news, err := runtime.News(ctx, "US", "AAPL", 7)
	if err != nil || news.InstrumentID != "US.AAPL" || news.Source != "stub-news" {
		t.Fatalf("News = %#v, err=%v", news, err)
	}
	if provider.newsMarket != "US" || provider.newsSymbol != "AAPL" || provider.newsLimit != 7 {
		t.Fatalf("news forwarding = %s/%s/%d", provider.newsMarket, provider.newsSymbol, provider.newsLimit)
	}
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	actions, err := runtime.CorporateActions(ctx, "US", "AAPL", from, to)
	if err != nil || actions.Source != "stub-actions" {
		t.Fatalf("CorporateActions = %#v, err=%v", actions, err)
	}
	if !provider.actionsFrom.Equal(from) || !provider.actionsTo.Equal(to) {
		t.Fatalf("actions forwarding = %v/%v", provider.actionsFrom, provider.actionsTo)
	}
	if provider.calls["News"] != 1 || provider.calls["CorporateActions"] != 1 {
		t.Fatalf("calls = %#v", provider.calls)
	}

	provider.newsErr = errors.New("news upstream failed")
	if _, err := runtime.News(ctx, "US", "AAPL", 7); !errors.Is(err, provider.newsErr) {
		t.Fatalf("News error passthrough = %v", err)
	}
	if _, err := runtime.CorporateActions(ctx, "US", "AAPL", from, to); !errors.Is(err, provider.newsErr) {
		t.Fatalf("CorporateActions error passthrough = %v", err)
	}
}

func TestRuntimeNewsAndCorporateActionsRejectProvidersWithoutCapability(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	_, err = runtime.News(ctx, "US", "AAPL", 10)
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) ||
		!strings.Contains(err.Error(), ProviderFutu) || !strings.Contains(err.Error(), "news") {
		t.Fatalf("News unsupported error = %v", err)
	}
	_, err = runtime.CorporateActions(ctx, "US", "AAPL", time.Time{}, time.Time{})
	if !errors.Is(err, marketdata.ErrCapabilityUnsupported) ||
		!strings.Contains(err.Error(), ProviderFutu) || !strings.Contains(err.Error(), "corporate actions") {
		t.Fatalf("CorporateActions unsupported error = %v", err)
	}
}
