package marketdataapp

import (
	"context"
	"fmt"
	"log"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// ProviderSettingsStore is the persisted provider selection needed by the
// application data plane.
type ProviderSettingsStore interface {
	ActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider
	SaveActiveMarketDataProvider(jfsettings.ActiveMarketDataProvider) error
}

// QuoteCacheResetter invalidates provider-owned projections outside the core
// market-data service, such as watchlist quote previews.
type QuoteCacheResetter interface {
	ResetQuoteCache()
}

// QuoteProviderSwitcher serializes a provider mutation with quote-flight
// reservation. Watchlist implements this to prevent requests from crossing a
// provider commit boundary.
type QuoteProviderSwitcher interface {
	ChangeQuoteProvider(func() error) error
}

// DataPlane keeps the stable market-data service and its provider router
// together during application assembly.
type DataPlane struct {
	Service *marketdata.Service
	Runtime *Runtime
}

// NewDataPlane assembles a stable service, installs the provider-aware
// subscription reconciler, and restores the persisted selection.
func NewDataPlane(
	options RuntimeOptions,
	store ProviderSettingsStore,
) (*DataPlane, error) {
	runtime, err := NewRuntime(options)
	if err != nil {
		return nil, err
	}
	service := marketdata.NewService(runtime)
	service.SetSubscriptionReconciler(runtime)
	restoreConfiguredProvider(context.Background(), service, store)
	return &DataPlane{Service: service, Runtime: runtime}, nil
}

// RuntimeFromService resolves the application router without adding another
// copy of it to servercore's aggregate application state.
func RuntimeFromService(service *marketdata.Service) *Runtime {
	if service == nil {
		return nil
	}
	runtime, _ := service.ProviderRuntime().(*Runtime)
	return runtime
}

// ApplyProviderSettings atomically serializes provider switching with managed
// subscription leases. A concurrent live strategy either owns its Futu lease
// first and blocks the switch, or observes the poll-only provider and cannot
// acquire a lease.
func ApplyProviderSettings(
	ctx context.Context,
	service *marketdata.Service,
	store ProviderSettingsStore,
	quoteCache QuoteCacheResetter,
	providerID jfsettings.ActiveMarketDataProvider,
	requireHealthy bool,
) error {
	runtime := RuntimeFromService(service)
	if runtime == nil || store == nil {
		return fmt.Errorf("market-data provider runtime is unavailable")
	}
	change := func() error {
		return service.ChangeProvider(func() error {
			return runtime.Activate(ctx, Activation{
				ProviderID:           string(providerID),
				RequireHealthy:       requireHealthy,
				DesiredSubscriptions: service.ActiveSubscriptionDemand(),
			})
		})
	}
	if switcher, ok := quoteCache.(QuoteProviderSwitcher); ok {
		return switcher.ChangeQuoteProvider(change)
	}
	err := change()
	if err != nil {
		return err
	}
	if quoteCache != nil {
		quoteCache.ResetQuoteCache()
	}
	return nil
}

func restoreConfiguredProvider(
	ctx context.Context,
	service *marketdata.Service,
	store ProviderSettingsStore,
) {
	if store == nil {
		return
	}
	configured := store.ActiveMarketDataProvider()
	err := ApplyProviderSettings(ctx, service, store, nil, configured, false)
	if err == nil {
		return
	}
	log.Printf("JFTrade activate configured market-data provider degraded: %v", err)
	if configured == jfsettings.MarketDataProviderFutu {
		return
	}
	if persistErr := store.SaveActiveMarketDataProvider(
		jfsettings.MarketDataProviderFutu,
	); persistErr != nil {
		log.Printf(
			"JFTrade persist Futu market-data fallback after activation failure: %v",
			persistErr,
		)
	}
}
