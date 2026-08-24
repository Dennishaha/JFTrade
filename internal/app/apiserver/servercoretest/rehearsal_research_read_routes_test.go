package servercoretest

import "testing"

func TestResearchReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/research/instruments/{instrumentId}",
		"GET /api/v1/research/financials/{instrumentId}",
		"GET /api/v1/research/valuation/{instrumentId}",
		"GET /api/v1/research/analyst/{instrumentId}",
		"GET /api/v1/research/ownership/{instrumentId}",
		"GET /api/v1/research/corporate-actions/{instrumentId}",
		"GET /api/v1/research/short-interest/{instrumentId}",
		"GET /api/v1/research/technical-indicators/{instrumentId}",
		"GET /api/v1/research/screens",
		"GET /api/v1/research/calendars",
		"GET /api/v1/research/macro",
		"GET /api/v1/research/rankings",
		"GET /api/v1/research/institutions",
		"GET /api/v1/research/industries",
	}
	paths := []string{
		"/api/v1/research/instruments/US.AAPL?brokerId=missing",
		"/api/v1/research/financials/US.AAPL?brokerId=missing",
		"/api/v1/research/valuation/US.AAPL?brokerId=missing",
		"/api/v1/research/analyst/US.AAPL?brokerId=missing",
		"/api/v1/research/ownership/US.AAPL?brokerId=missing",
		"/api/v1/research/corporate-actions/US.AAPL?brokerId=missing",
		"/api/v1/research/short-interest/US.AAPL?brokerId=missing",
		"/api/v1/research/technical-indicators/US.AAPL?brokerId=missing",
		"/api/v1/research/screens?brokerId=missing&market=US",
		"/api/v1/research/calendars?brokerId=missing&market=US",
		"/api/v1/research/macro?brokerId=missing&market=US",
		"/api/v1/research/rankings?brokerId=missing&market=US",
		"/api/v1/research/institutions?brokerId=missing&market=US",
		"/api/v1/research/industries?brokerId=missing&market=US",
	}
	operationPaths := make(map[string]string, len(operations))
	for index, operation := range operations {
		operationPaths[operation] = paths[index]
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "research-read",
		operations:      operations,
		paths:           paths,
		operationPaths:  operationPaths,
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}, "asOf": {}, "resolvedAt": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
