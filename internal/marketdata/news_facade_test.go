package marketdata

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type newsCapableProviderStub struct {
	dataProviderStub
	newsResponse  NewsResponse
	newsErr       error
	newsMarket    string
	newsSymbol    string
	newsLimit     int
	actions       CorporateActionsResponse
	actionsErr    error
	actionsMarket string
	actionsSymbol string
	actionsFrom   time.Time
	actionsTo     time.Time
}

func (p *newsCapableProviderStub) News(_ context.Context, market, symbol string, limit int) (NewsResponse, error) {
	p.newsMarket, p.newsSymbol, p.newsLimit = market, symbol, limit
	return p.newsResponse, p.newsErr
}

func (p *newsCapableProviderStub) CorporateActions(
	_ context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (CorporateActionsResponse, error) {
	p.actionsMarket, p.actionsSymbol, p.actionsFrom, p.actionsTo = market, symbol, from, to
	return p.actions, p.actionsErr
}

func TestServiceNewsAndCorporateActionsRejectProvidersWithoutCapability(t *testing.T) {
	service := NewService(&dataProviderStub{})
	ctx := context.Background()

	_, err := service.GetNews(ctx, "US", "AAPL", 10)
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "stub-provider") ||
		!strings.Contains(err.Error(), "news") {
		t.Fatalf("GetNews unsupported error = %v", err)
	}
	_, err = service.GetCorporateActions(ctx, "US", "AAPL", time.Time{}, time.Time{})
	if !errors.Is(err, ErrCapabilityUnsupported) || !strings.Contains(err.Error(), "corporate actions") {
		t.Fatalf("GetCorporateActions unsupported error = %v", err)
	}
}

func TestServiceNewsValidatesLimitAndForwardsNormalizedArguments(t *testing.T) {
	provider := &newsCapableProviderStub{
		newsResponse: NewsResponse{
			Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL",
			Entries: []NewsEntry{}, Source: "yfinance-news",
		},
	}
	service := NewService(provider)
	ctx := context.Background()

	response, err := service.GetNews(ctx, "us", "aapl", 0)
	if err != nil {
		t.Fatalf("GetNews: %v", err)
	}
	if response.InstrumentID != "US.AAPL" || provider.newsMarket != "us" || provider.newsSymbol != "aapl" ||
		provider.newsLimit != DefaultNewsLimit {
		t.Fatalf("GetNews forwarding = %s/%s/%d, response = %#v",
			provider.newsMarket, provider.newsSymbol, provider.newsLimit, response)
	}
	for _, limit := range []int{-1, MaxNewsLimit + 1} {
		if _, err := service.GetNews(ctx, "US", "AAPL", limit); err == nil ||
			!strings.Contains(err.Error(), "limit") {
			t.Fatalf("GetNews limit %d error = %v", limit, err)
		}
	}
}

func TestServiceNewsResolvesChinaAggregateToExchangeLeaf(t *testing.T) {
	provider := &newsCapableProviderStub{
		newsResponse: NewsResponse{Market: "SH", Symbol: "600519", InstrumentID: "SH.600519", Entries: []NewsEntry{}},
	}
	service := NewService(provider)
	if _, err := service.GetNews(context.Background(), "CN", "SH.600519", 5); err != nil {
		t.Fatalf("GetNews CN aggregate: %v", err)
	}
	if provider.newsMarket != "SH" || provider.newsSymbol != "600519" {
		t.Fatalf("news request = %s/%s", provider.newsMarket, provider.newsSymbol)
	}
}

func TestServiceCorporateActionsValidatesRangeAndForwardsArguments(t *testing.T) {
	provider := &newsCapableProviderStub{
		actions: CorporateActionsResponse{
			Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL",
			Events: []CorporateActionEvent{}, Source: "yfinance-actions",
		},
	}
	service := NewService(provider)
	ctx := context.Background()

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.GetCorporateActions(ctx, "US", "AAPL", to, from); err == nil ||
		!strings.Contains(err.Error(), "from") {
		t.Fatalf("reversed range error = %v", err)
	}
	response, err := service.GetCorporateActions(ctx, "US", "AAPL", from, to)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if response.Source != "yfinance-actions" || provider.actionsMarket != "US" ||
		provider.actionsSymbol != "AAPL" || !provider.actionsFrom.Equal(from) || !provider.actionsTo.Equal(to) {
		t.Fatalf("corporate actions forwarding = %#v, response = %#v", provider.actions, response)
	}
	providerErr := errors.New("upstream unavailable")
	provider.actionsErr = providerErr
	if _, err := service.GetCorporateActions(ctx, "US", "AAPL", from, to); !errors.Is(err, providerErr) {
		t.Fatalf("provider error passthrough = %v", err)
	}
}
