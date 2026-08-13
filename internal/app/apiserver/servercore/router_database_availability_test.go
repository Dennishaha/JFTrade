package servercore

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func TestDatabaseRequirementsAreDeclaredByRouteRegistration(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	for _, id := range []dmsrv.DatabaseID{
		dmsrv.DatabaseBacktest, dmsrv.DatabaseBacktestRuns, dmsrv.DatabaseStrategy,
		dmsrv.DatabaseExecution, dmsrv.DatabaseADK, dmsrv.DatabaseADKSession,
		dmsrv.DatabaseWatchlist, dmsrv.DatabaseResearch,
	} {
		server.unavailableDatabases.Record(id, errors.New("incompatible test database"))
	}

	for _, test := range []struct {
		path       string
		databaseID dmsrv.DatabaseID
	}{
		{path: "/api/v1/backtests", databaseID: dmsrv.DatabaseBacktest},
		{path: "/api/v1/strategies", databaseID: dmsrv.DatabaseStrategy},
		{path: "/api/v1/execution/orders", databaseID: dmsrv.DatabaseExecution},
		{path: "/api/v1/adk", databaseID: dmsrv.DatabaseADK},
		{path: "/api/v1/watchlist/groups", databaseID: dmsrv.DatabaseWatchlist},
		{path: "/api/v1/research/screens/presets", databaseID: dmsrv.DatabaseResearch},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "DATABASE_INCOMPATIBLE") ||
				!strings.Contains(response.Body.String(), string(test.databaseID)) {
				t.Fatalf("database error body = %s", response.Body.String())
			}
		})
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unrelated system route status = %d body=%s", response.Code, response.Body.String())
	}
}
