package servercoretest

import "testing"

func TestSystemStatusReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/system/status"
	path := "/api/v1/system/status"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:         "system-status-read",
		operations:     []string{operation},
		paths:          []string{path},
		operationPaths: map[string]string{operation: path},
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {}, "checkedAt": {}, "startedAt": {}, "uptimeMs": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
