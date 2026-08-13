package servercore

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	futuwatchlist "github.com/jftrade/jftrade-main/internal/watchlist/futu"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (b *serverBootstrap) loadWatchlistStore() *watchliststore.Store {
	store, err := watchliststore.Open(context.Background(), apiruntime.DeriveWatchlistDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(dmsrv.DatabaseWatchlist, err)
		return nil
	}
	return store
}

func (s *serverApplication) initializeWatchlistService() {
	if s == nil || s.stores.Watchlist == nil {
		return
	}
	s.watchlistSvc = watchlist.NewService(s.stores.Watchlist)
	s.watchlistSvc.RegisterSourceReader(futuwatchlist.SourceID, futuwatchlist.NewSourceReader(
		s.futuWatchlistGroupReader,
		s.futuCoordinator().Enabled,
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
	if s == nil || !s.futuCoordinator().Enabled() {
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
	source, ok := value.(broker.BatchSnapshotSource)
	if ok {
		return source, nil
	}
	reader := value.MarketData()
	source, ok = reader.(broker.BatchSnapshotSource)
	if !ok {
		return nil, fmt.Errorf("%w: Futu SecuritySnapshot is unavailable", watchlist.ErrUnavailable)
	}
	return source, nil
}
