package assistant

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	_ "modernc.org/sqlite"
)

// TestCoverage98CatalogReadFaultsExposeStableAPIContracts verifies that the
// administrative catalog endpoints do not turn a persistence outage into an
// apparently successful empty list. Each subtest uses a real SQLite schema
// fault after the API runtime has started, matching the failure mode an
// operator would see when a damaged or partially migrated database is served.
func TestCoverage98CatalogReadFaultsExposeStableAPIContracts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		table      string
		path       string
		statusCode int
		errorCode  string
	}{
		{
			name:       "tasks",
			table:      "adk_tasks",
			path:       "/api/v1/adk/tasks",
			statusCode: http.StatusInternalServerError,
			errorCode:  "ADK_TASK_LIST_FAILED",
		},
		{
			name:       "memory",
			table:      "adk_memory",
			path:       "/api/v1/adk/memory",
			statusCode: http.StatusBadRequest,
			errorCode:  "ADK_MEMORY_LIST_FAILED",
		},
		{
			name:       "agents",
			table:      "adk_agents",
			path:       "/api/v1/adk/agents",
			statusCode: http.StatusInternalServerError,
			errorCode:  "ADK_AGENT_LIST_FAILED",
		},
		{
			name:       "providers",
			table:      "adk_providers",
			path:       "/api/v1/adk/providers",
			statusCode: http.StatusInternalServerError,
			errorCode:  "ADK_PROVIDER_LIST_FAILED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, router, dbPath, _ := newAssistantTestRouterWithDBPath(t)
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatalf("open test database: %v", err)
			}
			t.Cleanup(func() { jftradeCheckTestError(t, db.Close()) })
			if _, err := db.ExecContext(context.Background(), "DROP TABLE "+tc.table); err != nil {
				t.Fatalf("drop %s: %v", tc.table, err)
			}

			assertAssistantErrorCode(t, performAssistantRequest(router, http.MethodGet, tc.path, nil), tc.statusCode, tc.errorCode)
		})
	}
}
