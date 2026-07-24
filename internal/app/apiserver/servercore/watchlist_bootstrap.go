package servercore

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	"github.com/jftrade/jftrade-main/internal/watchlist"
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
	s.initializeADKRuntime(bootstrap)
	s.initializeAssistantService()
	s.strategyRuntimeManager = newStrategyRuntimeManager(s)
	s.initializeMarketdataRuntime()
	s.reconcileStrategyRuntimeStates()
	s.startLiveNotifications()
	s.initializeRealTradeControl(bootstrap)
	s.initializeSystemService(bootstrap)
	s.initializeBacktestService(state)
	s.initializeStrategyService(state)
	s.initializeMarketdataService()
	s.startAssistantWorkflowScheduler()
	s.initializeRuntimeServices(store)
}

func (s *Server) initializeWatchlistService() {
	if s == nil || s.watchlistStore == nil {
		return
	}
	s.watchlistSvc = watchlist.NewService(s.watchlistStore)
	s.watchlistSvc.RegisterSourceReader(futuWatchlistSourceID, newFutuWatchlistReader(
		s.futuWatchlistGroupReader,
		s.futuIntegrationEnabled,
		s.probeFutuWatchlistSource,
	))
	s.watchlistSvc.RegisterBatchSnapshotSource(newFutuWatchlistSnapshotSource(s.futuWatchlistBatchSnapshotSource))
}

func (s *Server) probeFutuWatchlistSource(ctx context.Context) error {
	return futuWatchlistProbeError(s.probeOpenD(ctx))
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

func (s *Server) futuWatchlistBroker() (broker.Broker, error) {
	if s == nil || !s.futuIntegrationEnabled() {
		return nil, fmt.Errorf("%w: Futu integration is disabled", watchlist.ErrUnavailable)
	}
	if s.marketdataRuntime == nil || s.marketdataRuntime.Ensure() == nil || s.brokers == nil {
		return nil, fmt.Errorf("%w: Futu OpenD runtime is unavailable", watchlist.ErrUnavailable)
	}
	value := s.brokers.Lookup("futu")
	if value == nil {
		return nil, fmt.Errorf("%w: Futu broker adapter is unavailable", watchlist.ErrUnavailable)
	}
	return value, nil
}

func (s *Server) futuWatchlistGroupReader() (broker.WatchlistGroupReader, error) {
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

func (s *Server) futuWatchlistBatchSnapshotSource() (broker.BatchSnapshotSource, error) {
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
