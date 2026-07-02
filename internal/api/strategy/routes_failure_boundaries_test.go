package strategy

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestAnalyzePineRouteCoversSuccessMalformedAndAnalysisFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		body     string
		analyzer func(srv.PineAnalyzeInput) (srv.PineAnalysisResult, error)
		status   int
		code     string
	}{
		{
			name:   "malformed JSON",
			body:   `{`,
			status: http.StatusBadRequest,
			code:   "BAD_REQUEST",
		},
		{
			name: "analysis failure",
			body: `{"script":"//@version=6","sourceFormat":"pine-v6"}`,
			analyzer: func(srv.PineAnalyzeInput) (srv.PineAnalysisResult, error) {
				return srv.PineAnalysisResult{}, errors.New("parser unavailable")
			},
			status: http.StatusBadRequest,
			code:   "PINE_ANALYSIS_FAILED",
		},
		{
			name: "success",
			body: `{"script":"//@version=6","sourceFormat":"pine-v6","includeAst":true}`,
			analyzer: func(input srv.PineAnalyzeInput) (srv.PineAnalysisResult, error) {
				return srv.PineAnalysisResult{"sourceFormat": input.SourceFormat, "includeAst": input.IncludeAST}, nil
			},
			status: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := []srv.Option{}
			if test.analyzer != nil {
				options = append(options, srv.WithPineAnalyzer(test.analyzer))
			}
			service := srv.NewService(nil, nil, nil, options...)
			router := gin.New()
			RegisterRoutes(router.Group("/api/v1"), service)

			response := strategyRequest(t, router, http.MethodPost, "/api/v1/strategy-pine/analyze", test.body)
			if response.Code != test.status {
				t.Fatalf("response = %d %s, want %d", response.Code, response.Body.String(), test.status)
			}
			if test.code != "" && !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response = %s, want code %s", response.Body.String(), test.code)
			}
		})
	}
}

func TestDefinitionRoutesMapReadWriteAndDeleteFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantErr := errors.New("definition store unavailable")

	t.Run("read failure", func(t *testing.T) {
		router := strategyRouter(&routeLifecycleDesignStore{err: wantErr}, &routeLifecycleCatalogStore{})
		response := strategyRequest(t, router, http.MethodGet, "/api/v1/strategy-definitions/def-1", "")
		assertStrategyResponse(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
	})

	t.Run("valid preview derives warmup", func(t *testing.T) {
		design := &routeLifecycleDesignStore{found: true, definition: srv.Definition{
			ID:       "def-1",
			Symbol:   "US.AAPL",
			Interval: "5m",
			Script: `//@version=6
strategy("Warmup", overlay=true)
slow = ta.sma(close, 20)`,
		}}
		router := strategyRouter(design, &routeLifecycleCatalogStore{})
		response := strategyRequest(t, router, http.MethodGet, "/api/v1/strategy-definitions/def-1?interval=5m&symbol=US.AAPL", "")
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"derivedWarmupBars":20`) {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
	})

	t.Run("create malformed", func(t *testing.T) {
		response := strategyRequest(t, strategyRouter(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}), http.MethodPost, "/api/v1/strategy-definitions", `{`)
		assertStrategyResponse(t, response, http.StatusBadRequest, "BAD_REQUEST")
	})

	t.Run("create store failure", func(t *testing.T) {
		design := &routeLifecycleDesignStore{saveErr: wantErr}
		response := strategyRequest(t, strategyRouter(design, &routeLifecycleCatalogStore{}), http.MethodPost, "/api/v1/strategy-definitions", `{"id":"client-id","name":"Draft"}`)
		assertStrategyResponse(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
		if design.saveInput.ID != "" {
			t.Fatalf("create forwarded client ID %q", design.saveInput.ID)
		}
	})

	t.Run("update malformed", func(t *testing.T) {
		response := strategyRequest(t, strategyRouter(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}), http.MethodPut, "/api/v1/strategy-definitions/def-1", `{`)
		assertStrategyResponse(t, response, http.StatusBadRequest, "BAD_REQUEST")
	})

	t.Run("update store failure", func(t *testing.T) {
		design := &routeLifecycleDesignStore{saveErr: wantErr}
		response := strategyRequest(t, strategyRouter(design, &routeLifecycleCatalogStore{}), http.MethodPut, "/api/v1/strategy-definitions/def-1", `{"name":"Updated"}`)
		assertStrategyResponse(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
		if design.saveInput.ID != "def-1" {
			t.Fatalf("update ID = %q, want def-1", design.saveInput.ID)
		}
	})

	t.Run("delete store failure", func(t *testing.T) {
		design := &routeLifecycleDesignStore{deleteErr: wantErr}
		response := strategyRequest(t, strategyRouter(design, &routeLifecycleCatalogStore{}), http.MethodDelete, "/api/v1/strategy-definitions/def-1", "")
		assertStrategyResponse(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
	})
}

func TestDefinitionOrchestrationRoutesMapBusinessFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantErr := errors.New("catalog unavailable")

	tests := []struct {
		name    string
		path    string
		design  *routeLifecycleDesignStore
		catalog *routeLifecycleCatalogStore
		status  int
		code    string
	}{
		{
			name:    "apply linked catalog failure",
			path:    "/api/v1/strategy-definitions/def-1/apply-linked-instances",
			design:  &routeLifecycleDesignStore{found: true, definition: srv.Definition{ID: "def-1"}},
			catalog: &routeLifecycleCatalogStore{applyErr: wantErr},
			status:  http.StatusInternalServerError,
			code:    "STRATEGY_FAILED",
		},
		{
			name:    "instantiate definition read failure",
			path:    "/api/v1/strategy-definitions/def-1/instantiate",
			design:  &routeLifecycleDesignStore{err: wantErr},
			catalog: &routeLifecycleCatalogStore{},
			status:  http.StatusBadRequest,
			code:    "BAD_REQUEST",
		},
		{
			name:    "instantiate missing definition",
			path:    "/api/v1/strategy-definitions/missing/instantiate",
			design:  &routeLifecycleDesignStore{found: false},
			catalog: &routeLifecycleCatalogStore{},
			status:  http.StatusNotFound,
			code:    "NOT_FOUND",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := strategyRequest(t, strategyRouter(test.design, test.catalog), http.MethodPost, test.path, "")
			assertStrategyResponse(t, response, test.status, test.code)
		})
	}
}

func TestInstanceMutationRoutesMapCatalogFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantErr := errors.New("instance store unavailable")
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		catalog *routeLifecycleCatalogStore
	}{
		{
			name:    "update binding",
			method:  http.MethodPut,
			path:    "/api/v1/strategies/inst-1",
			body:    `{"symbols":["US.AAPL"],"interval":"1m"}`,
			catalog: &routeLifecycleCatalogStore{updateErr: wantErr},
		},
		{
			name:    "update runtime risk",
			method:  http.MethodPut,
			path:    "/api/v1/strategies/inst-1/runtime-risk",
			body:    `{"mode":"close_only","closeOnly":true}`,
			catalog: &routeLifecycleCatalogStore{updateRuntimeRiskErr: wantErr},
		},
		{
			name:    "pause transition",
			method:  http.MethodPost,
			path:    "/api/v1/strategies/inst-1/pause",
			catalog: &routeLifecycleCatalogStore{transitionErr: wantErr},
		},
		{
			name:    "stop transition",
			method:  http.MethodPost,
			path:    "/api/v1/strategies/inst-1/stop",
			catalog: &routeLifecycleCatalogStore{transitionErr: wantErr},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := strategyRequest(t, strategyRouter(&routeLifecycleDesignStore{}, test.catalog), test.method, test.path, test.body)
			assertStrategyResponse(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
		})
	}
}

func TestPluginRoutesRejectWhitespaceIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPluginRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{}))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "operation", method: http.MethodGet, path: "/api/v1/plugins/operations/%20"},
		{name: "install", method: http.MethodPost, path: "/api/v1/plugins/%20/install"},
		{name: "guidance", method: http.MethodGet, path: "/api/v1/plugins/%20/uninstall-guidance"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := strategyRequest(t, router, test.method, test.path, "")
			assertStrategyResponse(t, response, http.StatusBadRequest, "BAD_REQUEST")
		})
	}
}

func TestWriteStrategyErrorIgnoresNil(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	writeStrategyError(context, nil, http.StatusInternalServerError, "STRATEGY_FAILED", "failed")
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("nil error wrote response: %d %s", response.Code, response.Body.String())
	}
}

func strategyRouter(design *routeLifecycleDesignStore, catalog *routeLifecycleCatalogStore) *gin.Engine {
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(design, catalog, &routeLifecycleRuntime{}))
	return router
}

func strategyRequest(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertStrategyResponse(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || !strings.Contains(response.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("response = %d %s, want status %d code %s", response.Code, response.Body.String(), status, code)
	}
}
