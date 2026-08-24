package servercoretest

import "testing"

var settingsReadRouteOperations = []string{
	"GET /api/v1/settings/adk",
	"GET /api/v1/settings/adk/mcp",
	"GET /api/v1/settings/backtest-market-data-provider",
	"GET /api/v1/settings/brokers",
	"GET /api/v1/settings/exchange-calendars",
	"GET /api/v1/settings/execution",
	"GET /api/v1/settings/market-data-provider",
	"GET /api/v1/settings/onboarding",
	"GET /api/v1/settings/pine-worker",
	"GET /api/v1/settings/security",
	"GET /api/v1/settings/system-notifications",
}

func TestSettingsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	paths := []string{
		"/api/v1/settings/adk",
		"/api/v1/settings/adk/mcp",
		"/api/v1/settings/backtest-market-data-provider",
		"/api/v1/settings/brokers",
		"/api/v1/settings/exchange-calendars",
		"/api/v1/settings/execution",
		"/api/v1/settings/market-data-provider",
		"/api/v1/settings/onboarding",
		"/api/v1/settings/pine-worker",
		"/api/v1/settings/security",
		"/api/v1/settings/system-notifications",
	}
	operationPaths := make(map[string]string, len(settingsReadRouteOperations))
	for _, operation := range settingsReadRouteOperations {
		for _, path := range paths {
			if operation == "GET "+path {
				operationPaths[operation] = path
				break
			}
		}
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "settings-read",
		operations:      settingsReadRouteOperations,
		paths:           paths,
		operationPaths:  operationPaths,
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
