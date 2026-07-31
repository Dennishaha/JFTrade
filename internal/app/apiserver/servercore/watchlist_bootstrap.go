package servercore

import (
	"context"
	"fmt"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	futuwatchlist "github.com/jftrade/jftrade-main/internal/watchlist/futu"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (b *serverBootstrap) loadWatchlistStore() *watchliststore.Store {
	store, err := watchliststore.Open(context.Background(), apiruntime.DeriveWatchlistDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseWatchlist, err)
		return nil
	}
	return store
}

func (s *Server) initializeBootstrapState(store SidecarSettingsStore, bootstrap serverBootstrap, state serverPersistentState) {
	s.initializeSecurityAndCalendars(store, bootstrap.settingsPath)
	s.initializeWatchlistService()
	s.initializeResearchService()
	s.initializeMarketdataRuntime()
	s.startLiveNotifications()
	s.initializeRealTradeControl(bootstrap)
	s.tradingSvc = s.newTradingService()
	s.registerResource("trading order updates", s.stopTradingOrderUpdates)
	s.initializeBacktestService(state)
	liveWebSocket := apilive.NewHandler(liveWebSocketBackend{server: s}, apilive.Options{
		DataInterval:            liveTickDispatchInterval,
		SecurityDetailsInterval: marketSecurityDetailsStreamInterval,
		DepthRefreshInterval:    marketDepthStreamRefreshInterval,
	})
	s.initializeMarketdataService()
	strategyRuntime := liveruntime.NewManager(newStrategyRuntimeDependencies(s))
	s.runtimes.SetStrategyRuntime(strategyRuntime, strategyRuntime)
	s.reconcileStrategyRuntimeStates()
	s.initializeStrategyService(state)
	s.runtimes.SetLiveWebSocket(liveWebSocket)
	s.initializeSystemService(bootstrap)
	s.initializeADKRuntime(bootstrap)
	s.initializeRuntimeServices(store)
	s.startAssistantWorkflowScheduler()
}

func (s *serverApplication) initializeWatchlistService() {
	if s == nil || s.stores.Watchlist == nil {
		return
	}
	s.watchlistSvc = watchlist.NewService(s.stores.Watchlist)
	s.watchlistSvc.RegisterSourceReader(futuwatchlist.SourceID, futuwatchlist.NewSourceReader(
		s.futuWatchlistGroupReader,
		s.futuIntegrationEnabled,
		s.probeFutuWatchlistSource,
	))
	futuSnapshots := futuwatchlist.NewBatchSnapshotSource(s.futuWatchlistBatchSnapshotSource)
	s.watchlistSvc.RegisterBatchSnapshotSource(marketdataapp.NewWatchlistSnapshotSource(
		func() marketdataapp.WatchlistQuoteRuntime {
			return marketdataapp.RuntimeFromService(s.marketdataSvc)
		},
		futuSnapshots,
	))
}

func (s *serverApplication) probeFutuWatchlistSource(ctx context.Context) error {
	return futuWatchlistProbeError(s.futuCoordinator().Probe(ctx))
}

func futuWatchlistProbeError(probe opendProbe) error {
	if probe.Connectivity != "connected" {
		if probe.LastError != nil && *probe.LastError != "" {
			return fmt.Errorf("%w: %s", watchlist.ErrUnavailable, *probe.LastError)
		}
		return fmt.Errorf("%w: Futu OpenD is not connected", watchlist.ErrUnavailable)
	}
	if probe.QuoteLoggedIn != nil && !*probe.QuoteLoggedIn {
		return fmt.Errorf("%w: Futu OpenD quote service is not logged in", watchlist.ErrUnavailable)
	}
	return nil
}

func (s *serverApplication) futuWatchlistBroker() (broker.Broker, error) {
	if s == nil || !s.futuIntegrationEnabled() {
		return nil, fmt.Errorf("%w: Futu integration is disabled", watchlist.ErrUnavailable)
	}
	marketDataRuntime := s.runtimes.MarketData()
	brokers := s.runtimes.Brokers()
	if marketDataRuntime == nil || marketDataRuntime.Ensure() == nil || brokers == nil {
		return nil, fmt.Errorf("%w: Futu OpenD runtime is unavailable", watchlist.ErrUnavailable)
	}
	value := brokers.Lookup("futu")
	if value == nil {
		return nil, fmt.Errorf("%w: Futu broker adapter is unavailable", watchlist.ErrUnavailable)
	}
	return value, nil
}

func (s *serverApplication) futuWatchlistGroupReader() (broker.WatchlistGroupReader, error) {
	value, err := s.futuWatchlistBroker()
	if err != nil {
		return nil, err
	}
	reader, ok := value.(broker.WatchlistGroupReader)
	if !ok {
		return nil, fmt.Errorf("%w: Futu watchlist group reads are unsupported", watchlist.ErrUnavailable)
	}
	return reader, nil
}

func (s *serverApplication) futuWatchlistBatchSnapshotSource() (broker.BatchSnapshotSource, error) {
	value, err := s.futuWatchlistBroker()
	if err != nil {
		return nil, err
	}
	reader := value.MarketData()
	if reader == nil {
		return nil, fmt.Errorf("%w: Futu SecuritySnapshot is unavailable", watchlist.ErrUnavailable)
	}
	return reader, nil
}
