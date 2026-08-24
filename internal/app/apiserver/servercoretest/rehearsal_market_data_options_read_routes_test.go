package servercoretest

import "testing"

func TestMarketDataOptionsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/options/chains/{instrumentId}",
		"GET /api/v1/market-data/options/expirations/{instrumentId}",
		"GET /api/v1/market-data/options/screens",
		"GET /api/v1/market-data/options/analysis/{instrumentId}",
		"GET /api/v1/market-data/options/events",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-options-read",
		operations: operations,
		paths: []string{
			"/api/v1/market-data/options/chains/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/options/expirations/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/options/screens?brokerId=missing&market=US",
			"/api/v1/market-data/options/analysis/US.AAPL?brokerId=missing&market=US",
			"/api/v1/market-data/options/events?brokerId=missing&market=US",
		},
		operationPaths: map[string]string{
			operations[0]: "/api/v1/market-data/options/chains/US.AAPL?brokerId=missing&market=US",
			operations[1]: "/api/v1/market-data/options/expirations/US.AAPL?brokerId=missing&market=US",
			operations[2]: "/api/v1/market-data/options/screens?brokerId=missing&market=US",
			operations[3]: "/api/v1/market-data/options/analysis/US.AAPL?brokerId=missing&market=US",
			operations[4]: "/api/v1/market-data/options/events?brokerId=missing&market=US",
		},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
