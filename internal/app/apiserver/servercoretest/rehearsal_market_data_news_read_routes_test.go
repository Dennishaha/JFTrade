package servercoretest

import "testing"

func TestMarketDataNewsSearchReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/market-data/news"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-news-search-read",
		operations: []string{operation},
		paths: []string{
			"/api/v1/market-data/news?brokerId=missing&market=US&limit=4",
		},
		operationPaths: map[string]string{
			operation: "/api/v1/market-data/news?brokerId=missing&market=US",
		},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}

func TestMarketDataNewsActionsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/news/{market}/{symbol}",
		"GET /api/v1/market-data/corporate-actions/{market}/{symbol}",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "market-data-news-actions-read",
		operations: operations,
		paths: []string{
			"/api/v1/market-data/news/US/AAPL?limit=0",
			"/api/v1/market-data/news/US/AAPL?limit=abc",
			"/api/v1/market-data/corporate-actions/US/AAPL?from=not-a-time",
			"/api/v1/market-data/corporate-actions/US/AAPL?to=not-a-time",
		},
		operationPaths: map[string]string{
			operations[0]: "/api/v1/market-data/news/US/AAPL?limit=0",
			operations[1]: "/api/v1/market-data/corporate-actions/US/AAPL?from=not-a-time",
		},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
