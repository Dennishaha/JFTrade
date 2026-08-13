package marketdataapp

import (
	"context"
	"strings"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

// FutuProviderDependencies are the composition-root callbacks needed to
// assemble the Futu OpenD provider without reaching into Server internals.
type FutuProviderDependencies struct {
	SecurityDetails   func(ctx context.Context, marketCode, symbol string) (mdsrv.SecurityDetails, error)
	LookupInstrument  func(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error)
	SearchInstruments func(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error)
	QuerySnapshot     func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error)
	QueryTicker       func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error)
	HistoricalCandles func(ctx context.Context, request mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error)
	Depth             func(ctx context.Context, marketCode, symbol string, num int) (mdsrv.DepthResponse, error)
	Health            func(ctx context.Context) (mdsrv.HealthStatus, error)
}

// NewFutuProvider assembles the Futu OpenD provider from narrow callbacks.
// The composition root keeps ownership of service handles; this package owns
// descriptor/market projection and provider construction.
func NewFutuProvider(deps FutuProviderDependencies) mdsrv.Provider {
	return NewProvider(ProviderOptions{
		Descriptor:          FutuProviderDescriptor,
		Markets:             futuProviderMarkets,
		NormalizeInstrument: NormalizeInstrument,
		SecurityDetails:     deps.SecurityDetails,
		LookupInstrument:    deps.LookupInstrument,
		SearchInstruments:   deps.SearchInstruments,
		QuerySnapshot:       deps.QuerySnapshot,
		QueryTicker:         deps.QueryTicker,
		HistoricalCandles:   deps.HistoricalCandles,
		Depth:               deps.Depth,
		Health:              deps.Health,
	})
}

// ServerHTTPAdapterDependencies are the server-owned handles consumed by the
// HTTP adapter binding. Keeping this in marketdataapp lets the composition
// root pass closures instead of exposing service fields.
type ServerHTTPAdapterDependencies struct {
	MarketDataService func() *mdsrv.Service
	MarketDataRuntime func() *futuintegration.MarketDataRuntime
	FutuEnabled       func() bool
}

func NewServerHTTPAdapters(deps ServerHTTPAdapterDependencies) *HTTPAdapters {
	return NewHTTPAdapters(HTTPAdapterDependencies{
		Service:           deps.MarketDataService,
		MarketDataRuntime: deps.MarketDataRuntime,
		FutuEnabled:       deps.FutuEnabled,
	})
}

func FutuProviderDescriptor(context.Context) (mdsrv.ProviderDescriptor, error) {
	dtos := MarketProfileDTOs()
	supportedMarkets := make([]string, 0, len(dtos))
	for _, profile := range dtos {
		supportedMarkets = append(supportedMarkets, strings.ToUpper(strings.TrimSpace(profile.Code)))
	}
	return mdsrv.ProviderDescriptor{
		SelectionID:      ProviderFutu,
		ProviderID:       "futu-opend",
		DisplayName:      "Futu OpenD",
		BrokerID:         "futu",
		Source:           "bbgo:futu",
		DefaultMarket:    "HK",
		SupportedMarkets: supportedMarkets,
		Transports:       []string{"opend-tcp", "push-stream", "snapshot-poll-fallback"},
		Capabilities: mdsrv.ProviderCapabilities{
			Snapshots: true, StreamingQuotes: true, StreamingCandles: true, StreamingDepth: true,
			HistoricalCandles: true, TickCandles: true, OrderBookDepth: true,
			InstrumentSearch: true, ExtendedHours: true,
			CandleIntervals:  []string{"tick", "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"},
			OrderBookLevels:  []int{1, 5, 10, 25, 50},
			Sessions:         []string{"RTH", "ETH", "ALL", "OVERNIGHT"},
			PriceAdjustments: []string{"none", "forward", "backward"},
		},
		Constraints: mdsrv.ProviderConstraints{
			RequiresOpenD: true, RequiresMarketDataRight: true, UsesSubscriptionQuota: true,
		},
		Notes: []string{
			"Futu-first provider; data entitlement and subscription quota are enforced by Futu OpenD.",
			"Historical candles and real-time pushes can diverge during extended sessions; UI surfaces observed timestamps and transport mode.",
		},
	}, nil
}

func futuProviderMarkets(context.Context) ([]mdsrv.MarketProfile, error) {
	dtos := UserMarketProfileDTOs()
	profiles := make([]mdsrv.MarketProfile, 0, len(dtos))
	for _, d := range dtos {
		profiles = append(profiles, mdsrv.MarketProfile{
			"code": d.Code, "resolvedMarket": d.ResolvedMarket, "preferredPrefix": d.PreferredPrefix,
			"displayName": d.DisplayName, "quoteCurrency": d.QuoteCurrency, "timezone": d.Timezone,
			"supportsExtendedHours":  d.SupportsExtendedHours,
			"requiresExchangePrefix": d.RequiresExchangePrefix, "aliases": d.Aliases,
			"regularSessions": d.RegularSessions, "precision": d.Precision, "tickSize": d.TickSize,
		})
	}
	return profiles, nil
}
