package settings_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apisettings "github.com/jftrade/jftrade-main/internal/api/settings"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
)

func TestManagedAccountUpdateUsesPathIDAndSurfacesServerErrors(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	t.Run("path id stays authoritative on success", func(t *testing.T) {
		store := &routeStore{}
		service := srvsettings.NewService(store)
		router := gin.New()
		apisettings.RegisterRoutes(router.Group("/api/v1"), service)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/api/v1/settings/broker-accounts/record-1",
			strings.NewReader(`{"id":"client-id","accountId":"acct-9","displayName":"Primary"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"id":"record-1"`) {
			t.Fatalf("body = %s, want response to use path account id", recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), `"id":"client-id"`) {
			t.Fatalf("body = %s, want client-provided id to be ignored", recorder.Body.String())
		}
	})

	t.Run("unexpected update error maps to 500", func(t *testing.T) {
		store := &routeStore{updateErr: errors.New("write failed")}
		service := srvsettings.NewService(store)
		router := gin.New()
		apisettings.RegisterRoutes(router.Group("/api/v1"), service)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/api/v1/settings/broker-accounts/record-1",
			strings.NewReader(`{"accountId":"acct-9"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500, body = %s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"code":"SETTINGS_SAVE_FAILED"`) {
			t.Fatalf("body = %s, want SETTINGS_SAVE_FAILED", recorder.Body.String())
		}
	})
}

func TestDataMigrationRebuildRejectsMalformedAndRejectedRequests(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	service := srvsettings.NewService(&routeStore{})
	dataManagementSvc := dmsrv.NewService(routeDataManagementBackend{
		rebuild: func(context.Context, dmsrv.RebuildRequest) (any, error) {
			return nil, errors.New("confirmation mismatch")
		},
	})
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service, dataManagementSvc)

	t.Run("malformed json returns bad request", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/settings/data-migration/databases/rebuild", strings.NewReader(`{"mode":`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
			t.Fatalf("body = %s, want BAD_REQUEST", recorder.Body.String())
		}
	})

	t.Run("service rejection stays a domain 400", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/api/v1/settings/data-migration/databases/rebuild",
			strings.NewReader(`{"mode":"single","databaseId":"adk","confirmation":"wrong"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"code":"DATABASE_REBUILD_REJECTED"`) {
			t.Fatalf("body = %s, want DATABASE_REBUILD_REJECTED", recorder.Body.String())
		}
	})
}
