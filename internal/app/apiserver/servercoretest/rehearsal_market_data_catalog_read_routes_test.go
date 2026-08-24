package servercoretest

import "testing"

func TestMarketDataCatalogReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/markets",
		"GET /api/v1/market-data/instruments",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-catalog-read",
		operations: operations,
		paths: []string{
			"/api/v1/market-data/markets",
			"/api/v1/market-data/instruments?query=AAPL&market=US&limit=2",
			"/api/v1/market-data/instruments?market=US",
			"/api/v1/market-data/instruments?query=AAPL&limit=101",
		},
		operationPaths: map[string]string{
			operations[0]: "/api/v1/market-data/markets",
			operations[1]: "/api/v1/market-data/instruments?market=US",
		},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
