package servercoretest

import "testing"

func TestMarketDataDerivativesReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/warrants",
		"GET /api/v1/market-data/futures",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-derivatives-read",
		operations: operations,
		paths: []string{
			"/api/v1/market-data/warrants?brokerId=missing&market=US&operation=list&pageSize=20",
			"/api/v1/market-data/futures?brokerId=missing&market=US&pageSize=25",
		},
		operationPaths: map[string]string{
			operations[0]: "/api/v1/market-data/warrants?brokerId=missing&market=US",
			operations[1]: "/api/v1/market-data/futures?brokerId=missing&market=US",
		},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
