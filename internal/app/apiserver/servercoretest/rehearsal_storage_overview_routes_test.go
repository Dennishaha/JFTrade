package servercoretest

import "testing"

func TestStorageOverviewReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/system/storage/overview"
	path := "/api/v1/system/storage/overview"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:          "storage-overview-read",
		operations:      []string{operation},
		paths:           []string{path},
		operationPaths:  map[string]string{operation: path},
		dynamicJSONKeys: map[string]struct{}{"timestamp": {}},
		prepareStore:    prepareDisabledFutuReadRehearsal,
	})
}
