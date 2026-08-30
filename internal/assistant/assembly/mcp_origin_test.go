package assembly

import (
	"net/http"
	"net/http/httptest"
	"testing"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestMCPOriginPolicyMatchesAbsentSameOriginAndRejectionCases(t *testing.T) {
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, jfadkruntime.NewToolRegistry()))
	manager.settings = jfsettings.MCPServerSettings{Enabled: true, AuthMode: "none"}
	handler := manager.authorizedHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name   string
		origin string
		status int
	}{
		{name: "absent", status: http.StatusNoContent},
		{name: "same origin", origin: "http://localhost", status: http.StatusNoContent},
		{name: "same origin with slash", origin: "http://localhost/", status: http.StatusNoContent},
		{name: "malformed", origin: "://bad", status: http.StatusForbidden},
		{name: "null", origin: "null", status: http.StatusForbidden},
		{name: "cross origin", origin: "http://evil.example", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", nil)
			request.RemoteAddr = "127.0.0.1:1"
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("Origin %q status = %d, want %d", tc.origin, recorder.Code, tc.status)
			}
		})
	}
}
