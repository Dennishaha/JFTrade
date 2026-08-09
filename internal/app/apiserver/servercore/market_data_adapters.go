package servercore

import (
	"context"
	"strings"

	httpserver "github.com/jftrade/jftrade-main/internal/api/httpserver"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func newMarketdataProvider(s *Server) mdsrv.Provider {
	return marketdataapp.NewFutuProvider(marketdataapp.FutuProviderDependencies{
		SecurityDetails: func(ctx context.Context, marketCode, symbol string) (mdsrv.SecurityDetails, error) {
			return marketHTTPAdapters(s).SecurityDetailsResponseForInstrument(ctx, marketCode, symbol)
		},
		LookupInstrument: func(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error) {
			selected, err := futuapp.BrokerOrError(s.futuCoordinator())
			if err != nil {
				return nil, err
			}
			return marketdataapp.LookupInstrument(ctx, selected, marketCode, code, "bbgo:futu")
		},
		SearchInstruments: func(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
			selected, err := futuapp.BrokerOrError(s.futuCoordinator())
			if err != nil {
				return nil, err
			}
			return marketdataapp.SearchInstruments(ctx, selected, query, limit, "bbgo:futu-search")
		},
		QuerySnapshot: func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
			return s.runtimes.MarketData().QuerySnapshot(ctx, instrumentID)
		},
		QueryTicker: func(ctx context.Context, instrumentID string) (*mdsrv.Tick, error) {
			return s.runtimes.MarketData().QueryTicker(ctx, instrumentID)
		},
		HistoricalCandles: func(ctx context.Context, request mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
			selected, err := futuapp.BrokerOrError(s.futuCoordinator())
			if err != nil {
				return nil, err
			}
			marketCode := strings.ToUpper(strings.TrimSpace(request.Market))
			includeSession := marketdataapp.ShouldAnnotateHistoricalKLineSession(marketCode, bbgotypes.Interval(strings.ToLower(strings.TrimSpace(request.Period))))
			return marketdataapp.HistoricalCandles(
				ctx, selected, futuintegration.BrokerID, request, includeSession, "bbgo:futu",
			)
		},
		Depth: func(ctx context.Context, marketCode, symbol string, num int) (mdsrv.DepthResponse, error) {
			query := marketdataapp.DepthQuery{Num: httpserver.OptionalIntValue{Value: num, Set: true, Valid: true}}
			return marketHTTPAdapters(s).DepthResponseForInstrument(ctx, marketCode, symbol, query)
		},
		Health: func(ctx context.Context) (mdsrv.HealthStatus, error) {
			return s.futuCoordinator().MarketDataHealth(ctx)
		},
	})
}

func marketHTTPAdapters(s *Server) *marketdataapp.HTTPAdapters {
	return marketdataapp.NewServerHTTPAdapters(marketdataapp.ServerHTTPAdapterDependencies{
		MarketDataService: func() *mdsrv.Service {
			return s.marketdataSvc
		},
		MarketDataRuntime: func() *futuintegration.MarketDataRuntime {
			return s.runtimes.MarketData()
		},
		FutuEnabled: func() bool {
			return s.futuCoordinator().Enabled()
		},
	})
}
