package servercore

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerCloseClosesAssistantHTTPTransport(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if err := server.Close(); err != nil {
		t.Fatalf("Server.Close: %v", err)
	}

	request := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/v1/adk/chat/stream",
		strings.NewReader(`{"clientRequestId":"00000000-0000-4000-8000-000000000001","message":"must not start after shutdown"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if !strings.Contains(response.Body.String(), "assistant transport is shutting down") {
		t.Fatalf("post-shutdown stream response = %q", response.Body.String())
	}
}
