package settings_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/middleware"
	apisettings "github.com/jftrade/jftrade-main/internal/api/settings"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestSettingWriteRoutesRejectMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := settingsRouter(&routeStore{})
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "appearance", method: http.MethodPut, path: "/api/v1/settings/ui"},
		{name: "onboarding", method: http.MethodPut, path: "/api/v1/settings/onboarding"},
		{name: "security", method: http.MethodPut, path: "/api/v1/settings/security"},
		{name: "system notifications", method: http.MethodPut, path: "/api/v1/settings/system-notifications"},
		{name: "adk", method: http.MethodPut, path: "/api/v1/settings/adk"},
		{name: "pine worker", method: http.MethodPut, path: "/api/v1/settings/pine-worker"},
		{name: "managed account create", method: http.MethodPost, path: "/api/v1/settings/broker-accounts"},
		{name: "managed account update", method: http.MethodPut, path: "/api/v1/settings/broker-accounts/account-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performSettingsRequest(t, router, test.method, test.path, `{`)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSettingWriteRoutesMapPersistenceFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := settingsRouter(&routeStore{
		saveErr:   errors.New("settings file is read-only"),
		createErr: errors.New("account store is read-only"),
		deleteErr: errors.New("account delete failed"),
	})
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "appearance", method: http.MethodPut, path: "/api/v1/settings/ui", body: `{"appearance":{"upColor":"#00ff00"}}`},
		{name: "onboarding", method: http.MethodPut, path: "/api/v1/settings/onboarding", body: `{"completed":true}`},
		{name: "execution", method: http.MethodPut, path: "/api/v1/settings/execution", body: `{}`},
		{name: "security", method: http.MethodPut, path: "/api/v1/settings/security", body: `{}`},
		{name: "system notifications", method: http.MethodPut, path: "/api/v1/settings/system-notifications", body: `{}`},
		{name: "adk", method: http.MethodPut, path: "/api/v1/settings/adk", body: `{}`},
		{name: "pine worker", method: http.MethodPut, path: "/api/v1/settings/pine-worker", body: `{}`},
		{name: "exchange calendar", method: http.MethodPut, path: "/api/v1/settings/exchange-calendars", body: `{"exchangeCalendars":{}}`},
		{name: "broker integration", method: http.MethodPut, path: "/api/v1/settings/brokers/futu/integration", body: `{}`},
		{name: "managed account create", method: http.MethodPost, path: "/api/v1/settings/broker-accounts", body: `{"accountId":"acc-1"}`},
		{name: "managed account delete", method: http.MethodDelete, path: "/api/v1/settings/broker-accounts/account-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performSettingsRequest(t, router, test.method, test.path, test.body)
			if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"SETTINGS_SAVE_FAILED"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSystemNotificationRoutesReadAndSaveSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &routeStore{
		systemNotifications: jfsettings.SystemNotificationSettings{
			Enabled:      true,
			Mode:         "important",
			Levels:       []string{"error"},
			Categories:   []string{"trading"},
			SoundEnabled: true,
		},
	}
	router := settingsRouter(store)

	getResponse := performSettingsRequest(t, router, http.MethodGet, "/api/v1/settings/system-notifications", "")
	if getResponse.Code != http.StatusOK ||
		!strings.Contains(getResponse.Body.String(), `"mode":"important"`) ||
		!strings.Contains(getResponse.Body.String(), `"categories":["trading"]`) {
		t.Fatalf("get response = %d %s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := performSettingsRequest(
		t,
		router,
		http.MethodPut,
		"/api/v1/settings/system-notifications",
		`{"enabled":false,"mode":"off","levels":["warn"],"categories":["system"],"soundEnabled":false}`,
	)
	if putResponse.Code != http.StatusOK {
		t.Fatalf("put response = %d %s", putResponse.Code, putResponse.Body.String())
	}
	if store.systemNotifications.Enabled ||
		store.systemNotifications.Mode != "off" ||
		len(store.systemNotifications.Levels) != 1 ||
		store.systemNotifications.Levels[0] != "warn" ||
		len(store.systemNotifications.Categories) != 1 ||
		store.systemNotifications.Categories[0] != "system" ||
		store.systemNotifications.SoundEnabled {
		t.Fatalf("stored system notification settings = %#v", store.systemNotifications)
	}
}

func TestOnboardingCanBeResetWithoutLosingLastBroker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &routeStore{onboarding: jfsettings.OnboardingSettings{
		Completed:    true,
		CompletedAt:  "2026-01-02T03:04:05Z",
		DismissedAt:  "2026-01-02T03:04:05Z",
		LastBrokerID: "futu",
	}}
	router := settingsRouter(store)

	response := performSettingsRequest(t, router, http.MethodPut, "/api/v1/settings/onboarding", `{"completed":false,"dismissed":false,"lastBrokerId":" "}`)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if store.onboarding.Completed || store.onboarding.CompletedAt != "" || store.onboarding.DismissedAt != "" {
		t.Fatalf("onboarding was not reset: %#v", store.onboarding)
	}
	if store.onboarding.LastBrokerID != "futu" {
		t.Fatalf("last broker = %q, want futu", store.onboarding.LastBrokerID)
	}
}

func TestDataMigrationStatusMapsCallbackFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srvsettings.NewService(&routeStore{})
	dataManagementSvc := dmsrv.NewService(routeDataManagementBackend{
		overview: func(context.Context, dmsrv.OverviewRequest) (any, error) {
			return nil, errors.New("runtime unavailable")
		},
	})
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service, dataManagementSvc)

	response := performSettingsRequest(t, router, http.MethodGet, "/api/v1/settings/data-migration/databases", "")
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"DATABASE_STATUS_FAILED"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestSettingsAndDataManagementRoutesExposeOperationalFailureContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("security rejects an invalid port and reports listener rollback failures", func(t *testing.T) {
		store := &routeStore{}
		service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
			OnSecurityChanged: func(jfsettings.SecuritySettings) error {
				return errors.New("listener is busy")
			},
		}))
		router := gin.New()
		apisettings.RegisterRoutes(router.Group("/api/v1"), service)

		invalidPort := performSettingsRequest(t, router, http.MethodPut, "/api/v1/settings/security", `{"webPort":80}`)
		if invalidPort.Code != http.StatusBadRequest || !strings.Contains(invalidPort.Body.String(), `"code":"INVALID_WEB_ACCESS_PORT"`) {
			t.Fatalf("invalid security port response = %d %s", invalidPort.Code, invalidPort.Body.String())
		}

		listenerFailure := performSettingsRequest(t, router, http.MethodPut, "/api/v1/settings/security", `{}`)
		if listenerFailure.Code != http.StatusConflict || !strings.Contains(listenerFailure.Body.String(), `"code":"WEB_ACCESS_LISTENER_UPDATE_FAILED"`) {
			t.Fatalf("security listener failure response = %d %s", listenerFailure.Code, listenerFailure.Body.String())
		}

		untrustedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/security", strings.NewReader(`{}`))
		untrustedRequest.Header.Set("Content-Type", "application/json")
		untrustedResponse := httptest.NewRecorder()
		router.ServeHTTP(untrustedResponse, untrustedRequest)
		if untrustedResponse.Code != http.StatusForbidden || !strings.Contains(untrustedResponse.Body.String(), `"code":"WEB_ACCESS_SETTINGS_DESKTOP_ONLY"`) {
			t.Fatalf("untrusted security response = %d %s", untrustedResponse.Code, untrustedResponse.Body.String())
		}

		invalidPassword := performSettingsRequest(t, router, http.MethodPut, "/api/v1/settings/security", `{"webAccessEnabled":true,"newPassword":"too-short"}`)
		if invalidPassword.Code != http.StatusBadRequest || !strings.Contains(invalidPassword.Body.String(), `"code":"INVALID_WEB_ACCESS_PASSWORD"`) {
			t.Fatalf("invalid security password response = %d %s", invalidPassword.Code, invalidPassword.Body.String())
		}
	})

	t.Run("MCP validation and token persistence errors stay distinguishable", func(t *testing.T) {
		router := settingsRouter(&routeStore{saveErr: errors.New("settings file is read-only")})

		malformed := performSettingsRequest(t, router, http.MethodPut, "/api/v1/settings/adk/mcp", `{`)
		if malformed.Code != http.StatusBadRequest || !strings.Contains(malformed.Body.String(), `"code":"BAD_REQUEST"`) {
			t.Fatalf("malformed MCP response = %d %s", malformed.Code, malformed.Body.String())
		}

		reset := performSettingsRequest(t, router, http.MethodPost, "/api/v1/settings/adk/mcp/token/reset", "")
		if reset.Code != http.StatusInternalServerError || !strings.Contains(reset.Body.String(), `"code":"MCP_SERVER_TOKEN_RESET_FAILED"`) {
			t.Fatalf("MCP token persistence response = %d %s", reset.Code, reset.Body.String())
		}
	})

	t.Run("cleanup preview returns a rejection when the data service cannot inspect storage", func(t *testing.T) {
		service := srvsettings.NewService(&routeStore{})
		dataManagementSvc := dmsrv.NewService(routeDataManagementBackend{})
		router := gin.New()
		apisettings.RegisterRoutes(router.Group("/api/v1"), service, dataManagementSvc)

		response := performSettingsRequest(t, router, http.MethodPost, "/api/v1/settings/data-management/cleanup/preview", `{"kind":"backtest-history","databaseId":"backtest-runs","olderThanDays":30,"keepLatest":20}`)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"DATABASE_CLEANUP_PREVIEW_REJECTED"`) {
			t.Fatalf("cleanup preview failure response = %d %s", response.Code, response.Body.String())
		}
	})
}

func settingsRouter(store *routeStore) *gin.Engine {
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), srvsettings.NewService(store))
	return router
}

func performSettingsRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if method == http.MethodPut && path == "/api/v1/settings/security" {
		request = middleware.MarkRequestTrustedHost(request)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
