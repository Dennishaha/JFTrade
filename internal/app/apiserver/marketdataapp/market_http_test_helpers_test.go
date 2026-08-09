package marketdataapp

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	httpserver "github.com/jftrade/jftrade-main/internal/api/httpserver"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type marketDataTestHarness struct {
	Adapters *HTTPAdapters
	Service  *mdsrv.Service
	Runtime  *futuintegration.MarketDataRuntime
}

// newMarketDataQuoteHarness assembles the Futu data plane without the full
// server composition root. It mirrors the production wiring in servercore so
// adapter-level market-data tests stay inside marketdataapp.
func newMarketDataQuoteHarness(t *testing.T, addr string) *marketDataTestHarness {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portText, err)
	}

	runtime := futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
		ConfigSource: func() futuintegration.MarketDataConfig {
			return futuintegration.MarketDataConfig{
				Enabled: true,
				Host:    host,
				APIPort: port,
			}
		},
	})
	t.Cleanup(func() { _ = runtime.Close() })

	var adapters *HTTPAdapters
	provider := NewFutuProvider(FutuProviderDependencies{
		SecurityDetails: func(ctx context.Context, marketCode, symbol string) (mdsrv.SecurityDetails, error) {
			return adapters.SecurityDetailsResponseForInstrument(ctx, marketCode, symbol)
		},
		LookupInstrument: func(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error) {
			return LookupInstrument(ctx, runtime.Broker(), marketCode, code, "bbgo:futu")
		},
		SearchInstruments: func(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
			return SearchInstruments(ctx, runtime.Broker(), query, limit, "bbgo:futu-search")
		},
		QuerySnapshot: runtime.QuerySnapshot,
		QueryTicker:   runtime.QueryTicker,
		HistoricalCandles: func(ctx context.Context, request mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
			marketCode := strings.ToUpper(strings.TrimSpace(request.Market))
			includeSession := ShouldAnnotateHistoricalKLineSession(
				marketCode,
				bbgotypes.Interval(strings.ToLower(strings.TrimSpace(request.Period))),
			)
			return HistoricalCandles(
				ctx, runtime.Broker(), futuintegration.BrokerID, request, includeSession, "bbgo:futu",
			)
		},
		Depth: func(ctx context.Context, marketCode, symbol string, num int) (mdsrv.DepthResponse, error) {
			return adapters.DepthResponseForInstrument(ctx, marketCode, symbol, DepthQuery{
				Num: httpserver.OptionalIntValue{Value: num, Set: true, Valid: true},
			})
		},
		Health: func(ctx context.Context) (mdsrv.HealthStatus, error) {
			return mdsrv.HealthStatus{Connected: runtime.Broker() != nil}, nil
		},
	})

	service := mdsrv.NewService(provider)
	// Adapter-level tests mirror newTestServer's behavior: without a physical
	// reconciler the service serves cached reads directly and cache-miss reads
	// call the provider synchronously.
	service.SetSubscriptionReconciler(nil)
	service.StartCollector(runtime, runtime, func(mdsrv.Tick) {})
	t.Cleanup(func() { _ = service.Close() })

	adapters = NewServerHTTPAdapters(ServerHTTPAdapterDependencies{
		MarketDataService: func() *mdsrv.Service { return service },
		MarketDataRuntime: func() *futuintegration.MarketDataRuntime { return runtime },
		FutuEnabled:       func() bool { return true },
	})
	return &marketDataTestHarness{
		Adapters: adapters,
		Service:  service,
		Runtime:  runtime,
	}
}
