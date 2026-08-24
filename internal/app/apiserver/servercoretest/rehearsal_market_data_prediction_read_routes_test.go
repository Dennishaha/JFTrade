package servercoretest

import "testing"

func TestMarketDataPredictionReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/market-data/prediction/categories",
		"GET /api/v1/market-data/prediction/combos/eligible-events",
		"GET /api/v1/market-data/prediction/competitions",
		"GET /api/v1/market-data/prediction/contracts/{code}/candles",
		"GET /api/v1/market-data/prediction/contracts/{code}/candles/history",
		"GET /api/v1/market-data/prediction/contracts/{code}/milestones",
		"GET /api/v1/market-data/prediction/contracts/{code}/order-book",
		"GET /api/v1/market-data/prediction/contracts/{code}/snapshot",
		"GET /api/v1/market-data/prediction/contracts/{code}/ticks",
		"GET /api/v1/market-data/prediction/events",
		"GET /api/v1/market-data/prediction/events/{eventId}/contracts",
		"GET /api/v1/market-data/prediction/series",
	}
	paths := []string{
		"/api/v1/market-data/prediction/categories?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/combos/eligible-events?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/competitions?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/candles?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/candles/history?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/milestones?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/order-book?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/snapshot?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/contracts/US.EC-42/ticks?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/events?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/events/EVENT-42/contracts?brokerId=missing&market=US",
		"/api/v1/market-data/prediction/series?brokerId=missing&market=US",
	}
	operationPaths := make(map[string]string, len(operations))
	for index, operation := range operations {
		operationPaths[operation] = paths[index]
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "market-data-prediction-read",
		operations:      operations,
		paths:           paths,
		operationPaths:  operationPaths,
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}, "asOf": {}, "resolvedAt": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
