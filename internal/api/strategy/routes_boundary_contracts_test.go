package strategy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestStrategyDefinitionPreviewQueryRejectsInvalidBooleans(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := strategyRouter(
		&routeLifecycleDesignStore{definition: srv.Definition{ID: "def-1"}, found: true},
		&routeLifecycleCatalogStore{},
	)

	response := strategyRequest(t, router, http.MethodGet, "/api/v1/strategy-definitions/def-1?useExtendedHours=maybe", "")
	assertStrategyResponse(t, response, http.StatusBadRequest, "BAD_REQUEST")
	if !strings.Contains(response.Body.String(), "invalid strategy definition query") {
		t.Fatalf("response = %s, want invalid query message", response.Body.String())
	}
}

func TestStrategyRoutesRejectMissingBoundURIParamsAtHandlerBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := srv.NewService(nil, nil, nil)
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler gin.HandlerFunc
	}{
		{name: "get definition", method: http.MethodGet, path: "/strategy-definitions/", handler: handleGetDefinition(service)},
		{name: "update definition", method: http.MethodPut, path: "/strategy-definitions/", body: `{}`, handler: handleUpdateDefinition(service)},
		{name: "delete definition", method: http.MethodDelete, path: "/strategy-definitions/", handler: handleDeleteDefinition(service)},
		{name: "apply linked", method: http.MethodPost, path: "/strategy-definitions//apply-linked-instances", handler: handleApplyLinked(service)},
		{name: "instantiate", method: http.MethodPost, path: "/strategy-definitions//instantiate", handler: handleInstantiate(service)},
		{name: "update instance", method: http.MethodPut, path: "/strategies/", body: `{}`, handler: handleUpdateInstance(service)},
		{name: "update runtime risk", method: http.MethodPut, path: "/strategies//runtime-risk", body: `{}`, handler: handleUpdateInstanceRuntimeRisk(service)},
		{name: "delete instance", method: http.MethodDelete, path: "/strategies/", handler: handleDeleteInstance(service)},
		{name: "start instance", method: http.MethodPost, path: "/strategies//start", handler: handleStartInstance(service)},
		{name: "refresh definition", method: http.MethodPost, path: "/strategies//refresh-definition", handler: handleRefreshDefinition(service)},
		{name: "pause instance", method: http.MethodPost, path: "/strategies//pause", handler: handlePauseInstance(service)},
		{name: "stop instance", method: http.MethodPost, path: "/strategies//stop", handler: handleStopInstance(service)},
		{name: "logs", method: http.MethodGet, path: "/strategies//logs", handler: handleGetLogs(service)},
		{name: "audit", method: http.MethodGet, path: "/strategies//audit", handler: handleGetAudit(service)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			request := httptest.NewRequestWithContext(t.Context(), test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			context.Request = request

			test.handler(context)

			assertStrategyResponse(t, response, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

func TestStrategyStartRouteMapsPreflightRuntimeAndTransitionBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		catalog   *routeLifecycleCatalogStore
		runtime   *routeLifecycleRuntime
		status    int
		code      string
		wantStops []string
	}{
		{
			name:    "missing instance",
			catalog: &routeLifecycleCatalogStore{instanceFound: false},
			runtime: &routeLifecycleRuntime{},
			status:  http.StatusNotFound,
			code:    "NOT_FOUND",
		},
		{
			name: "startability guard",
			catalog: &routeLifecycleCatalogStore{
				instance:             srv.ManagedInstance{ID: "inst-1"},
				instanceFound:        true,
				validateStartableErr: srv.BadRequestError("strategy is not configured"),
			},
			runtime: &routeLifecycleRuntime{},
			status:  http.StatusBadRequest,
			code:    "BAD_REQUEST",
		},
		{
			name: "worker capacity is a business busy response",
			catalog: &routeLifecycleCatalogStore{
				instance:      srv.ManagedInstance{ID: "inst-1"},
				instanceFound: true,
			},
			runtime: &routeLifecycleRuntime{startErr: pineworker.ErrCapacityExceeded},
			status:  http.StatusBadRequest,
			code:    "BAD_REQUEST",
		},
		{
			name: "transition failure stops already started runtime",
			catalog: &routeLifecycleCatalogStore{
				instance:      srv.ManagedInstance{ID: "inst-1"},
				instanceFound: true,
				transitionErr: errors.New("status store unavailable"),
			},
			runtime:   &routeLifecycleRuntime{},
			status:    http.StatusBadGateway,
			code:      "STRATEGY_RUNTIME_START_FAILED",
			wantStops: []string{"inst-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, test.catalog, test.runtime))

			response := strategyRequest(t, router, http.MethodPost, "/api/v1/strategies/inst-1/start", "")
			assertStrategyResponse(t, response, test.status, test.code)
			if strings.Join(test.runtime.stopIDs, ",") != strings.Join(test.wantStops, ",") {
				t.Fatalf("runtime stops = %#v, want %#v", test.runtime.stopIDs, test.wantStops)
			}
		})
	}
}

func TestStrategyActivityRoutesTreatMalformedPaginationAsDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	catalog := &routeLifecycleCatalogStore{
		logsExists:  true,
		auditExists: true,
	}
	router := strategyRouter(&routeLifecycleDesignStore{}, catalog)

	logsResponse := strategyRequest(t, router, http.MethodGet, "/api/v1/strategies/inst-1/logs?limit=bogus", "")
	if logsResponse.Code != http.StatusOK {
		t.Fatalf("logs response = %d %s, want 200", logsResponse.Code, logsResponse.Body.String())
	}
	if catalog.logsQuery.Limit != 500 || catalog.logsQuery.Offset != 0 {
		t.Fatalf("logs query = %#v, want default pagination", catalog.logsQuery)
	}

	auditResponse := strategyRequest(t, router, http.MethodGet, "/api/v1/strategies/inst-1/audit?offset=bogus", "")
	if auditResponse.Code != http.StatusOK {
		t.Fatalf("audit response = %d %s, want 200", auditResponse.Code, auditResponse.Body.String())
	}
	if catalog.auditQuery.Limit != 500 || catalog.auditQuery.Offset != 0 {
		t.Fatalf("audit query = %#v, want default pagination", catalog.auditQuery)
	}
}
