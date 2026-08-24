package servercoretest

import "testing"

func TestCalendarReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/system/exchange-calendars/sources",
		"GET /api/v1/system/exchange-calendars/status",
	}
	paths := []string{
		"/api/v1/system/exchange-calendars/sources",
		"/api/v1/system/exchange-calendars/status",
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "calendar-read",
		operations: operations,
		paths:      paths,
		operationPaths: map[string]string{
			operations[0]: paths[0],
			operations[1]: paths[1],
		},
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {}, "checkedAt": {}, "lastAlertAt": {}, "lastFailureAt": {},
			"lastProbeAt": {}, "lastProbeFailureAt": {}, "lastProbeSuccessAt": {},
			"lastSnapshotFetchedAt": {}, "lastSuccessAt": {}, "nextRefreshAt": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
