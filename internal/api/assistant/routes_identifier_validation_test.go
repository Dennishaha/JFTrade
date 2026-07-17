package assistant

import (
	"net/http"
	"testing"
)

// Blank path identifiers are a meaningful client-input failure mode: URL
// parameters are decoded before the handler sees them, so "%20" must never be
// treated as a persisted ADK identifier.  Exercise every identifier-bearing
// endpoint through the registered router to keep that contract consistent.
func TestAssistantRoutesRejectBlankDecodedIdentifiers(t *testing.T) {
	_, router := newAssistantTestRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"get task", http.MethodGet, "/api/v1/adk/tasks/%20", nil},
		{"update task", http.MethodPut, "/api/v1/adk/tasks/%20", []byte(`{}`)},
		{"delete task", http.MethodDelete, "/api/v1/adk/tasks/%20", nil},
		{"delete memory", http.MethodDelete, "/api/v1/adk/memory/%20", nil},
		{"test provider", http.MethodPost, "/api/v1/adk/providers/%20/test", nil},
		{"set default provider", http.MethodPost, "/api/v1/adk/providers/%20/default", nil},
		{"delete provider", http.MethodDelete, "/api/v1/adk/providers/%20", nil},
		{"update provider", http.MethodPut, "/api/v1/adk/providers/%20", []byte(`{}`)},
		{"delete agent", http.MethodDelete, "/api/v1/adk/agents/%20", nil},
		{"update agent", http.MethodPut, "/api/v1/adk/agents/%20", []byte(`{}`)},
		{"delete skill", http.MethodDelete, "/api/v1/adk/skills/%20", nil},
		{"get session", http.MethodGet, "/api/v1/adk/sessions/%20", nil},
		{"get session context", http.MethodGet, "/api/v1/adk/sessions/%20/context", nil},
		{"compact session context", http.MethodPost, "/api/v1/adk/sessions/%20/context/compact", []byte(`{}`)},
		{"update composer state", http.MethodPatch, "/api/v1/adk/sessions/%20/composer-state", []byte(`{}`)},
		{"rename session", http.MethodPut, "/api/v1/adk/sessions/%20", []byte(`{}`)},
		{"delete session", http.MethodDelete, "/api/v1/adk/sessions/%20", nil},
		{"get stream", http.MethodGet, "/api/v1/adk/streams/%20", nil},
		{"get run stream", http.MethodGet, "/api/v1/adk/runs/%20/stream", nil},
		{"get run", http.MethodGet, "/api/v1/adk/runs/%20", nil},
		{"update run objective", http.MethodPatch, "/api/v1/adk/runs/%20/objective", []byte(`{}`)},
		{"pause run", http.MethodPost, "/api/v1/adk/runs/%20/pause", nil},
		{"resume run", http.MethodPost, "/api/v1/adk/runs/%20/resume", nil},
		{"respond to run input", http.MethodPost, "/api/v1/adk/runs/%20/input-response", []byte(`{}`)},
		{"cancel run", http.MethodPost, "/api/v1/adk/runs/%20/cancel", nil},
		{"approve", http.MethodPost, "/api/v1/adk/approvals/%20/approve", nil},
		{"deny", http.MethodPost, "/api/v1/adk/approvals/%20/deny", nil},
		{"get optimization task", http.MethodGet, "/api/v1/adk/optimization-tasks/%20", nil},
		{"cancel optimization task", http.MethodPost, "/api/v1/adk/optimization-tasks/%20/cancel", nil},
		{"get workflow", http.MethodGet, "/api/v1/adk/workflows/%20", nil},
		{"update workflow", http.MethodPut, "/api/v1/adk/workflows/%20", []byte(`{}`)},
		{"delete workflow", http.MethodDelete, "/api/v1/adk/workflows/%20", nil},
		{"run workflow", http.MethodPost, "/api/v1/adk/workflows/%20/run", nil},
		{"list workflow triggers", http.MethodGet, "/api/v1/adk/workflows/%20/triggers", nil},
		{"save workflow trigger", http.MethodPost, "/api/v1/adk/workflows/%20/triggers", []byte(`{}`)},
		{"update workflow trigger", http.MethodPut, "/api/v1/adk/workflows/%20/triggers/trigger-1", []byte(`{}`)},
		{"delete workflow trigger", http.MethodDelete, "/api/v1/adk/workflows/%20/triggers/trigger-1", nil},
		{"run workflow trigger", http.MethodPost, "/api/v1/adk/workflow-triggers/%20/run", nil},
		{"run workflow webhook", http.MethodPost, "/api/v1/adk/workflow-webhooks/%20", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performAssistantRequest(router, tc.method, tc.path, tc.body)
			assertAssistantErrorCode(t, recorder, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}
