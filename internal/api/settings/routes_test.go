package settings_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apisettings "github.com/jftrade/jftrade-main/internal/api/settings"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type routeStore struct {
	execution   jfsettings.ExecutionSettings
	calendars   jfsettings.ExchangeCalendarSettings
	updateErr   error
	deleteErr   error
	integration jfsettings.BrokerIntegration
	created     jfsettings.ManagedBrokerAccount
}

func (s *routeStore) Appearance() jfsettings.UIAppearanceSettings {
	return jfsettings.UIAppearanceSettings{}
}
func (s *routeStore) Onboarding() jfsettings.OnboardingSettings {
	return jfsettings.OnboardingSettings{}
}
func (s *routeStore) ExecutionSettings() jfsettings.ExecutionSettings { return s.execution }
func (s *routeStore) SecuritySettings() jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{}
}
func (s *routeStore) ADKSettings() jfsettings.ADKRuntimeSettings {
	return jfsettings.ADKRuntimeSettings{}
}
func (s *routeStore) ExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	return s.calendars
}
func (s *routeStore) Integration() jfsettings.BrokerIntegration {
	return jfsettings.BrokerIntegration{}
}
func (s *routeStore) SavedIntegration() *jfsettings.BrokerIntegration    { return nil }
func (s *routeStore) ManagedAccounts() []jfsettings.ManagedBrokerAccount { return nil }
func (s *routeStore) InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	return jfsettings.InterfaceSettings{APIBind: defaults.APIBind, GUIBind: defaults.GUIBind}
}
func (s *routeStore) SaveAppearance(input jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error) {
	return input, nil
}
func (s *routeStore) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	return input, nil
}
func (s *routeStore) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	s.execution = input
	return input, nil
}
func (s *routeStore) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	return input, nil
}
func (s *routeStore) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	return input, nil
}
func (s *routeStore) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	s.calendars = input
	return input, nil
}
func (s *routeStore) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	s.integration = input
	return input, nil
}
func (s *routeStore) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	if strings.TrimSpace(input.AccountID) == "" {
		return jfsettings.ManagedBrokerAccount{}, srvsettings.BadRequestError("accountId is required")
	}
	s.created = input
	return input, nil
}
func (s *routeStore) UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	if s.updateErr != nil {
		return jfsettings.ManagedBrokerAccount{}, s.updateErr
	}
	input.ID = id
	return input, nil
}
func (s *routeStore) DeleteManagedAccount(id string) error { return s.deleteErr }
func (s *routeStore) EnsureBootstrapFile(defaults jfsettings.LaunchDefaults) error {
	return nil
}

func TestSettingsRoutesPreserveLegacyResponseShapes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	service := srvsettings.NewService(store)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	t.Run("broker integration returns object directly", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/brokers/futu/integration", strings.NewReader(`{"enabled":true}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var envelope struct {
			OK   bool                         `json:"ok"`
			Data jfsettings.BrokerIntegration `json:"data"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !envelope.OK || envelope.Data.BrokerID != "futu" || !envelope.Data.Enabled {
			t.Fatalf("envelope = %#v, want integration object as data", envelope)
		}
	})

	t.Run("delete account returns deleted flag and id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/settings/broker-accounts/account-1", nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				Deleted bool   `json:"deleted"`
				ID      string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !envelope.OK || !envelope.Data.Deleted || envelope.Data.ID != "account-1" {
			t.Fatalf("envelope = %#v, want deleted flag and id", envelope)
		}
	})
}

func TestManagedAccountRoutesMapMissingRecordsToNotFound(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{updateErr: os.ErrNotExist, deleteErr: os.ErrNotExist}
	service := srvsettings.NewService(store)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	for _, tc := range []struct {
		method string
		body   string
	}{
		{method: http.MethodPut, body: `{"accountId":"missing"}`},
		{method: http.MethodDelete},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, "/api/v1/settings/broker-accounts/missing", strings.NewReader(tc.body))
		if tc.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, body = %s", tc.method, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCreateManagedAccountRejectsMissingAccountID(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	service := srvsettings.NewService(store)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/broker-accounts", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateManagedAccountDropsServerManagedFields(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	service := srvsettings.NewService(store)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/broker-accounts", strings.NewReader(`{"id":"client-id","accountId":"acc-1","createdAt":"client-created","updatedAt":"client-updated"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.created.ID != "" || store.created.CreatedAt != "" || store.created.UpdatedAt != "" {
		t.Fatalf("server managed fields were forwarded: %#v", store.created)
	}
	if store.created.AccountID != "acc-1" {
		t.Fatalf("accountId = %q, want acc-1", store.created.AccountID)
	}
}
func (s *routeStore) HasAppearance() bool { return false }
func (s *routeStore) Path() string        { return "" }

func TestExecutionSettingsRouteUsesInjectedService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	var sideEffect jfsettings.ExecutionSettings
	service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnExecutionChanged: func(settings jfsettings.ExecutionSettings) {
			sideEffect = settings
		},
	}))

	router := gin.New()
	api := router.Group("/api/v1")
	apisettings.RegisterRoutes(api, service)

	body := `{"defaultTradingEnvironment":"SIMULATE","brokerOrderHistoryLookbackDays":30,"seenFillRetentionDays":9}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/execution", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	want := jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      "SIMULATE",
		BrokerOrderHistoryLookbackDays: 30,
		SeenFillRetentionDays:          9,
	}
	if !reflect.DeepEqual(store.execution, want) {
		t.Fatalf("stored execution = %#v, want %#v", store.execution, want)
	}
	if !reflect.DeepEqual(sideEffect, want) {
		t.Fatalf("side effect = %#v, want %#v", sideEffect, want)
	}

	var envelope struct {
		OK   bool                         `json:"ok"`
		Data jfsettings.ExecutionSettings `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK || !reflect.DeepEqual(envelope.Data, want) {
		t.Fatalf("envelope = %#v, want ok with execution settings", envelope)
	}
}

func TestExchangeCalendarSettingsRouteUsesInjectedService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	var sideEffect jfsettings.ExchangeCalendarSettings
	service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnExchangeCalendarsChanged: func(settings jfsettings.ExchangeCalendarSettings) {
			sideEffect = settings
		},
	}))

	router := gin.New()
	api := router.Group("/api/v1")
	apisettings.RegisterRoutes(api, service)

	body := `{"exchangeCalendars":{"autoRefreshEnabled":false,"refreshIntervalHours":12,"warmupMarkets":["US","HK"],"sourcePolicies":[{"market":"US","enabledSourceIds":["nyse_official"],"fallbackToBuiltin":true}]}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/exchange-calendars", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	if !reflect.DeepEqual(store.calendars, sideEffect) {
		t.Fatalf("side effect = %#v, stored = %#v", sideEffect, store.calendars)
	}
	if store.calendars.RefreshIntervalHours != 12 || store.calendars.AutoRefreshEnabled {
		t.Fatalf("stored calendars = %#v", store.calendars)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ExchangeCalendars jfsettings.ExchangeCalendarSettings `json:"exchangeCalendars"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK || envelope.Data.ExchangeCalendars.RefreshIntervalHours != 12 {
		t.Fatalf("envelope = %#v", envelope)
	}
}
