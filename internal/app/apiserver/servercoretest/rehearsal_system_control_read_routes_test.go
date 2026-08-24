package servercoretest

import "testing"

func TestSystemControlReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	paths := []string{
		"/api/v1/system/futu-opend/install-guide",
		"/api/v1/system/real-trade-approvals",
		"/api/v1/system/real-trade-hard-stop-events",
		"/api/v1/system/real-trade-hard-stops",
		"/api/v1/system/real-trade-kill-switch",
		"/api/v1/system/real-trade-kill-switch-events",
		"/api/v1/system/real-trade-risk-events",
		"/api/v1/system/real-trade-risk-limits",
	}
	operations := make([]string, 0, len(paths))
	operationPaths := make(map[string]string, len(paths))
	for _, path := range paths {
		operation := "GET " + path
		operations = append(operations, operation)
		operationPaths[operation] = path
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "system-control-read",
		operations:      operations,
		paths:           paths,
		operationPaths:  operationPaths,
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}, "checkedAt": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
