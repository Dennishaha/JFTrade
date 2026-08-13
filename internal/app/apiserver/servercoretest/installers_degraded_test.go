package servercoretest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

func TestInstallersPreserveDegradedStartupWhenAssistantDatabasesAreUnavailable(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JFTRADE_ADK_DB", filepath.Join(blocker, "adk.db"))
	t.Setenv("JFTRADE_ADK_SESSION_DB", filepath.Join(blocker, "adk-session.db"))
	settings, err := servercore.NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := servercore.NewServer(settings)
	t.Cleanup(func() { _ = server.Close() })

	for path, want := range map[string]int{
		"/api/v1/adk":           http.StatusServiceUnavailable,
		"/api/v1/system/status": http.StatusOK,
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != want {
			t.Fatalf("%s status = %d body=%s", path, response.Code, response.Body.String())
		}
	}
}
