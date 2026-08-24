package servercoretest

import "testing"

func TestDataManagementReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/settings/data-management/databases"
	path := "/api/v1/settings/data-management/databases"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:     "data-management-read",
		operations: []string{operation},
		paths: []string{
			path,
			path + "?summaryOnly=TRUE",
			path + "?databaseId=%20strategy%20",
			path + "?databaseId=unknown",
		},
		operationPaths: map[string]string{operation: path},
		dynamicJSONKeys: map[string]struct{}{
			"checkedAt": {}, "timestamp": {},
			"mainBytes": {}, "walBytes": {}, "shmBytes": {},
			"totalBytes": {}, "freePageBytes": {}, "reclaimableBytes": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
