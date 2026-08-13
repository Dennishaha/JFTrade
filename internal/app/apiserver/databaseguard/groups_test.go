package databaseguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

func TestGroupsDeclareDatabaseAvailabilityPerRouteFamily(t *testing.T) {
	availability := dmsrv.NewAvailabilitySnapshot()
	for _, id := range []dmsrv.DatabaseID{
		dmsrv.DatabaseADK, dmsrv.DatabaseADKSession, dmsrv.DatabaseBacktest,
		dmsrv.DatabaseBacktestRuns, dmsrv.DatabaseExecution, dmsrv.DatabaseResearch,
		dmsrv.DatabaseStrategy, dmsrv.DatabaseWatchlist,
	} {
		availability.Record(id, errors.New("incompatible test database"))
	}
	router, routes := testRoutes(availability)
	for _, test := range routes {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), string(test.databaseID)) {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestGroupsKeepRoutesAvailableWhenDatabasesAreHealthy(t *testing.T) {
	router, routes := testRoutes(dmsrv.NewAvailabilitySnapshot())
	for _, route := range routes {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route.path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d", route.path, response.Code)
		}
	}
}

type guardedRoute struct {
	path       string
	databaseID dmsrv.DatabaseID
}

func testRoutes(availability dmsrv.Availability) (*gin.Engine, []guardedRoute) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	groups := New(router.Group("/api/v1"), availability)
	routes := []struct {
		group      *gin.RouterGroup
		path       string
		databaseID dmsrv.DatabaseID
	}{
		{groups.Assistant, "/api/v1/adk", dmsrv.DatabaseADK},
		{groups.Backtest, "/api/v1/backtests", dmsrv.DatabaseBacktest},
		{groups.Execution, "/api/v1/execution", dmsrv.DatabaseExecution},
		{groups.Research, "/api/v1/research", dmsrv.DatabaseResearch},
		{groups.Strategy, "/api/v1/strategies", dmsrv.DatabaseStrategy},
		{groups.Watchlist, "/api/v1/watchlist", dmsrv.DatabaseWatchlist},
	}
	result := make([]guardedRoute, 0, len(routes))
	for _, route := range routes {
		route.group.GET(strings.TrimPrefix(route.path, "/api/v1"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		result = append(result, guardedRoute{path: route.path, databaseID: route.databaseID})
	}
	return router, result
}
