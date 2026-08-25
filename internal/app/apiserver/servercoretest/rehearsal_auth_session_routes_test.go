package servercoretest

import "testing"

func TestAuthSessionRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operation := "GET /api/v1/auth/session"
	path := "/api/v1/auth/session"
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:         "auth-session",
		operations:     []string{operation},
		paths:          []string{path},
		operationPaths: map[string]string{operation: path},
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {},
			"expiresAt": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
