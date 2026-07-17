package assistant

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCoverage98AssistantQueryRoutesRejectMalformedEncoding verifies that a
// malformed percent escape is not silently converted to an empty query by
// net/url. The table covers each ADK read route that accepts query fields, so
// callers receive the same explicit 400 contract before any business action.
func TestCoverage98AssistantQueryRoutesRejectMalformedEncoding(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	for _, endpoint := range []struct {
		name string
		path string
	}{
		{name: "tasks", path: "/api/v1/adk/tasks"},
		{name: "memory", path: "/api/v1/adk/memory"},
		{name: "agents", path: "/api/v1/adk/agents"},
		{name: "workflows", path: "/api/v1/adk/workflows"},
		{name: "workflow trigger logs", path: "/api/v1/adk/workflow-trigger-logs"},
		{name: "audit", path: "/api/v1/adk/audit"},
		{name: "optimization tasks", path: "/api/v1/adk/optimization-tasks"},
		{name: "sessions", path: "/api/v1/adk/sessions"},
		{name: "runs", path: "/api/v1/adk/runs"},
		{name: "approvals", path: "/api/v1/adk/approvals"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, endpoint.path, nil)
			request.URL.RawQuery = "%zz"
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assertAssistantErrorCode(t, response, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}
