package servercoretest

import "testing"

func TestADKJSONReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	operations := []string{
		"GET /api/v1/adk",
		"GET /api/v1/adk/agents",
		"GET /api/v1/adk/approvals",
		"GET /api/v1/adk/audit",
		"GET /api/v1/adk/memory",
		"GET /api/v1/adk/metrics",
		"GET /api/v1/adk/optimization-tasks",
		"GET /api/v1/adk/optimization-tasks/{taskId}",
		"GET /api/v1/adk/providers",
		"GET /api/v1/adk/runs",
		"GET /api/v1/adk/runs/{runId}",
		"GET /api/v1/adk/sessions",
		"GET /api/v1/adk/sessions/{sessionId}",
		"GET /api/v1/adk/sessions/{sessionId}/context",
		"GET /api/v1/adk/skills",
		"GET /api/v1/adk/tasks",
		"GET /api/v1/adk/tasks/{taskId}",
		"GET /api/v1/adk/tools",
		"GET /api/v1/adk/workflow-trigger-logs",
		"GET /api/v1/adk/workflows",
		"GET /api/v1/adk/workflows/{workflowId}",
		"GET /api/v1/adk/workflows/{workflowId}/triggers",
	}
	paths := []string{
		"/api/v1/adk",
		"/api/v1/adk/agents?limit=1&offset=0",
		"/api/v1/adk/approvals",
		"/api/v1/adk/audit",
		"/api/v1/adk/memory",
		"/api/v1/adk/metrics",
		"/api/v1/adk/optimization-tasks?limit=1&offset=0",
		"/api/v1/adk/optimization-tasks/missing",
		"/api/v1/adk/providers",
		"/api/v1/adk/runs",
		"/api/v1/adk/runs/missing",
		"/api/v1/adk/sessions",
		"/api/v1/adk/sessions/missing",
		"/api/v1/adk/sessions/missing/context",
		"/api/v1/adk/skills",
		"/api/v1/adk/tasks",
		"/api/v1/adk/tasks/missing",
		"/api/v1/adk/tools",
		"/api/v1/adk/workflow-trigger-logs",
		"/api/v1/adk/workflows",
		"/api/v1/adk/workflows/missing",
		"/api/v1/adk/workflows/missing/triggers",
	}
	operationPaths := make(map[string]string, len(operations))
	for index, operation := range operations {
		operationPaths[operation] = paths[index]
	}
	runReadRouteRehearsal(t, readRouteRehearsalSpec{
		prefix:         "adk-json-read",
		operations:     operations,
		paths:          paths,
		operationPaths: operationPaths,
		dynamicJSONKeys: map[string]struct{}{
			"timestamp": {}, "checkedAt": {}, "createdAt": {}, "updatedAt": {}, "startedAt": {},
			"finishedAt": {}, "expiresAt": {}, "since": {}, "installPath": {},
		},
		prepareStore: prepareDisabledFutuReadRehearsal,
	})
}
