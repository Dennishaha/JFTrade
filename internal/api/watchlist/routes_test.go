package watchlist

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

func TestUnavailableServiceReturns503Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/watchlist/groups", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != "WATCHLIST_UNAVAILABLE" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestUnavailableServiceExercisesAllRouteErrorBranches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), nil)
	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodPost, "/api/v1/watchlist/groups", `{"name":"group"}`, http.StatusServiceUnavailable},
		{http.MethodPatch, "/api/v1/watchlist/groups/group-1", `{"name":"group","expectedRevision":1}`, http.StatusServiceUnavailable},
		{http.MethodDelete, "/api/v1/watchlist/groups/group-1", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/items", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/instruments/US/AAPL/memberships", "", http.StatusServiceUnavailable},
		{http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", `{"groupIds":[]}`, http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/sources", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/sources/missing/groups", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/bindings", "", http.StatusServiceUnavailable},
		{http.MethodDelete, "/api/v1/watchlist/bindings?bindingId=binding-1", "", http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/watchlist/quotes/batch", `{"instrumentIds":["US.AAPL"]}`, http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/watchlist/imports/preview", `{"sourceId":"missing","remoteGroupId":"group"}`, http.StatusServiceUnavailable},
		{http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", `{}`, http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/watchlist/import-runs", "", http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
		if test.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s status = %d, want %d; body=%s", test.method, test.path, response.Code, test.status, response.Body.String())
		}
	}
}

func TestInvalidListLimitReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/watchlist/items?limit=nope", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestWatchlistListAndBindingRoutesRejectMalformedQueryEncoding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), nil)
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/watchlist/items"},
		{http.MethodGet, "/api/v1/watchlist/bindings"},
		{http.MethodDelete, "/api/v1/watchlist/bindings"},
		{http.MethodGet, "/api/v1/watchlist/import-runs"},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.URL.RawQuery = "%zz"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestWatchlistRoutesRejectMissingURIValuesBeforeExecutingBusinessOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Route matching normally supplies these parameters. Clearing them in a
	// middleware models a malformed route hand-off and verifies that each
	// state-changing/read handler rejects the request before it reaches the
	// watchlist service.
	router.Use(func(context *gin.Context) { context.Params = nil })
	RegisterRoutes(router.Group("/api/v1"), nil)

	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodPatch, "/api/v1/watchlist/groups/group-1"},
		{http.MethodDelete, "/api/v1/watchlist/groups/group-1"},
		{http.MethodGet, "/api/v1/watchlist/instruments/US/AAPL/memberships"},
		{http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships"},
		{http.MethodGet, "/api/v1/watchlist/sources/futu:default/groups"},
		{http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit"},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}
