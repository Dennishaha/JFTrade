package settings_test

import (
	"context"
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
	appearance  jfsettings.UIAppearanceSettings
	onboarding  jfsettings.OnboardingSettings
	execution   jfsettings.ExecutionSettings
	security    jfsettings.SecuritySettings
	adk         jfsettings.ADKRuntimeSettings
	pineWorker  jfsettings.PineWorkerSettings
	calendars   jfsettings.ExchangeCalendarSettings
	updateErr   error
	deleteErr   error
	saveErr     error
	createErr   error
	integration jfsettings.BrokerIntegration
	created     jfsettings.ManagedBrokerAccount
}

func (s *routeStore) Appearance() jfsettings.UIAppearanceSettings {
	return s.appearance
}
func (s *routeStore) Onboarding() jfsettings.OnboardingSettings {
	return s.onboarding
}
func (s *routeStore) ExecutionSettings() jfsettings.ExecutionSettings { return s.execution }
func (s *routeStore) SecuritySettings() jfsettings.SecuritySettings {
	return s.security
}
func (s *routeStore) ADKSettings() jfsettings.ADKRuntimeSettings {
	return s.adk
}
func (s *routeStore) PineWorkerSettings() jfsettings.PineWorkerSettings {
	return s.pineWorker
}
func (s *routeStore) ExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	return s.calendars
}
func (s *routeStore) Integration() jfsettings.BrokerIntegration {
	return s.integration
}
func (s *routeStore) SavedIntegration() *jfsettings.BrokerIntegration    { return nil }
func (s *routeStore) ManagedAccounts() []jfsettings.ManagedBrokerAccount { return nil }
func (s *routeStore) InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	return jfsettings.InterfaceSettings{APIBind: defaults.APIBind, GUIBind: defaults.GUIBind}
}
func (s *routeStore) SaveAppearance(input jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error) {
	if s.saveErr != nil {
		return jfsettings.UIAppearanceSettings{}, s.saveErr
	}
	s.appearance = input
	return input, nil
}
func (s *routeStore) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	if s.saveErr != nil {
		return jfsettings.OnboardingSettings{}, s.saveErr
	}
	s.onboarding = input
	return input, nil
}
func (s *routeStore) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	if s.saveErr != nil {
		return jfsettings.ExecutionSettings{}, s.saveErr
	}
	s.execution = input
	return input, nil
}
func (s *routeStore) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	if s.saveErr != nil {
		return jfsettings.SecuritySettings{}, s.saveErr
	}
	s.security = input
	return input, nil
}
func (s *routeStore) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	if s.saveErr != nil {
		return jfsettings.ADKRuntimeSettings{}, s.saveErr
	}
	s.adk = input
	return input, nil
}
func (s *routeStore) SavePineWorkerSettings(input jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error) {
	if s.saveErr != nil {
		return jfsettings.PineWorkerSettings{}, s.saveErr
	}
	s.pineWorker = input
	return input, nil
}
func (s *routeStore) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	if s.saveErr != nil {
		return jfsettings.ExchangeCalendarSettings{}, s.saveErr
	}
	s.calendars = input
	return input, nil
}
func (s *routeStore) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	if s.saveErr != nil {
		return jfsettings.BrokerIntegration{}, s.saveErr
	}
	s.integration = input
	return input, nil
}
func (s *routeStore) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	if strings.TrimSpace(input.AccountID) == "" {
		return jfsettings.ManagedBrokerAccount{}, srvsettings.BadRequestError("accountId is required")
	}
	if s.createErr != nil {
		return jfsettings.ManagedBrokerAccount{}, s.createErr
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
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/brokers/futu/integration", strings.NewReader(`{"enabled":true}`))
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
		request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/settings/broker-accounts/account-1", nil)
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
		request := httptest.NewRequestWithContext(t.Context(), tc.method, "/api/v1/settings/broker-accounts/missing", strings.NewReader(tc.body))
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
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/settings/broker-accounts", strings.NewReader(`{}`))
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
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/settings/broker-accounts", strings.NewReader(`{"id":"client-id","accountId":"acc-1","createdAt":"client-created","updatedAt":"client-updated"}`))
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
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/execution", strings.NewReader(body))
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

func TestAppearanceOnboardingSecurityAndADKRoutesCoverSaveFlows(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{
		appearance: jfsettings.UIAppearanceSettings{UpColor: "#111111", DownColor: "#222222"},
		onboarding: jfsettings.OnboardingSettings{LastBrokerID: "futu"},
	}
	var securitySideEffect jfsettings.SecuritySettings
	service := srvsettings.NewService(
		store,
		srvsettings.WithOnboardingState(func(context.Context) map[string]any {
			return map[string]any{
				"completed":    store.onboarding.Completed,
				"completedAt":  store.onboarding.CompletedAt,
				"dismissedAt":  store.onboarding.DismissedAt,
				"lastBrokerId": store.onboarding.LastBrokerID,
			}
		}),
		srvsettings.WithSideEffects(srvsettings.SideEffects{
			OnSecurityChanged: func(settings jfsettings.SecuritySettings) {
				securitySideEffect = settings
			},
		}),
	)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	uiGetRec := httptest.NewRecorder()
	router.ServeHTTP(uiGetRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/ui", nil))
	if uiGetRec.Code != http.StatusOK || !strings.Contains(uiGetRec.Body.String(), `"upColor":"#111111"`) {
		t.Fatalf("ui get = %d %s", uiGetRec.Code, uiGetRec.Body.String())
	}

	uiPutRec := httptest.NewRecorder()
	uiPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/ui", strings.NewReader(`{"appearance":{"upColor":"#00ff00","downColor":"#ff0000"}}`))
	uiPutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(uiPutRec, uiPutReq)
	if uiPutRec.Code != http.StatusOK {
		t.Fatalf("ui put = %d %s", uiPutRec.Code, uiPutRec.Body.String())
	}
	if store.appearance.UpColor != "#00ff00" || store.appearance.DownColor != "#ff0000" {
		t.Fatalf("appearance = %#v", store.appearance)
	}

	onboardingGetRec := httptest.NewRecorder()
	router.ServeHTTP(onboardingGetRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/onboarding", nil))
	if onboardingGetRec.Code != http.StatusOK || !strings.Contains(onboardingGetRec.Body.String(), `"lastBrokerId":"futu"`) {
		t.Fatalf("onboarding get = %d %s", onboardingGetRec.Code, onboardingGetRec.Body.String())
	}

	onboardingPutRec := httptest.NewRecorder()
	onboardingPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/onboarding", strings.NewReader(`{"completed":true,"dismissed":true,"lastBrokerId":"   "}`))
	onboardingPutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(onboardingPutRec, onboardingPutReq)
	if onboardingPutRec.Code != http.StatusOK {
		t.Fatalf("onboarding put = %d %s", onboardingPutRec.Code, onboardingPutRec.Body.String())
	}
	if !store.onboarding.Completed || store.onboarding.LastBrokerID != "futu" || store.onboarding.CompletedAt == "" || store.onboarding.DismissedAt == "" {
		t.Fatalf("onboarding stored = %#v", store.onboarding)
	}
	if !strings.Contains(onboardingPutRec.Body.String(), `"completed":true`) {
		t.Fatalf("onboarding response = %s", onboardingPutRec.Body.String())
	}

	securityGetRec := httptest.NewRecorder()
	router.ServeHTTP(securityGetRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/security", nil))
	if securityGetRec.Code != http.StatusOK || !strings.Contains(securityGetRec.Body.String(), `"adminAuthRequired":false`) {
		t.Fatalf("security get = %d %s", securityGetRec.Code, securityGetRec.Body.String())
	}

	securityPutRec := httptest.NewRecorder()
	securityPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/security", strings.NewReader(`{"adminAuthRequired":true}`))
	securityPutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(securityPutRec, securityPutReq)
	if securityPutRec.Code != http.StatusOK {
		t.Fatalf("security put = %d %s", securityPutRec.Code, securityPutRec.Body.String())
	}
	if !store.security.AdminAuthRequired || !securitySideEffect.AdminAuthRequired {
		t.Fatalf("security store=%#v sideEffect=%#v", store.security, securitySideEffect)
	}

	adkGetRec := httptest.NewRecorder()
	router.ServeHTTP(adkGetRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/adk", nil))
	if adkGetRec.Code != http.StatusOK || !strings.Contains(adkGetRec.Body.String(), `"runTimeoutMs":0`) {
		t.Fatalf("adk get = %d %s", adkGetRec.Code, adkGetRec.Body.String())
	}

	adkPutRec := httptest.NewRecorder()
	adkPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/adk", strings.NewReader(`{"runTimeoutMs":120000,"streamIdleTimeoutMs":30000}`))
	adkPutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(adkPutRec, adkPutReq)
	if adkPutRec.Code != http.StatusOK {
		t.Fatalf("adk put = %d %s", adkPutRec.Code, adkPutRec.Body.String())
	}
	if store.adk.RunTimeoutMs != 120000 || store.adk.StreamIdleTimeoutMs != 30000 {
		t.Fatalf("adk settings = %#v", store.adk)
	}

	pineWorkerGetRec := httptest.NewRecorder()
	router.ServeHTTP(pineWorkerGetRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/pine-worker", nil))
	if pineWorkerGetRec.Code != http.StatusOK || !strings.Contains(pineWorkerGetRec.Body.String(), `"backtestWorkerLimit":0`) || !strings.Contains(pineWorkerGetRec.Body.String(), `"instanceWorkerLimit":0`) {
		t.Fatalf("pine worker get = %d %s", pineWorkerGetRec.Code, pineWorkerGetRec.Body.String())
	}

	pineWorkerPutRec := httptest.NewRecorder()
	pineWorkerPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/pine-worker", strings.NewReader(`{"backtestWorkerLimit":4,"instanceWorkerLimit":9,"nodeBinaryPath":"/opt/node/bin/node"}`))
	pineWorkerPutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(pineWorkerPutRec, pineWorkerPutReq)
	if pineWorkerPutRec.Code != http.StatusOK {
		t.Fatalf("pine worker put = %d %s", pineWorkerPutRec.Code, pineWorkerPutRec.Body.String())
	}
	if store.pineWorker.BacktestWorkerLimit != 4 || store.pineWorker.InstanceWorkerLimit != 9 || store.pineWorker.NodeBinaryPath != "/opt/node/bin/node" {
		t.Fatalf("pine worker settings = %#v", store.pineWorker)
	}
}

func TestExecutionExchangeCalendarAndBrokerRoutesCoverReadAndValidation(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	store := &routeStore{
		execution: jfsettings.ExecutionSettings{
			DefaultTradingEnvironment:      "REAL",
			BrokerOrderHistoryLookbackDays: 15,
			SeenFillRetentionDays:          7,
		},
		calendars: jfsettings.ExchangeCalendarSettings{
			AutoRefreshEnabled:        true,
			ErrorNotificationsEnabled: true,
			RefreshIntervalHours:      12,
			WarmupMarkets:             []string{"US", "HK"},
		},
	}
	var calendarSideEffect jfsettings.ExchangeCalendarSettings
	service := srvsettings.NewService(
		store,
		srvsettings.WithBrokerSettings(func() map[string]any {
			return map[string]any{"brokerId": "futu", "connected": true}
		}),
		srvsettings.WithSideEffects(srvsettings.SideEffects{
			OnExchangeCalendarsChanged: func(settings jfsettings.ExchangeCalendarSettings) {
				calendarSideEffect = settings
			},
		}),
	)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	t.Run("execution get returns persisted settings", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/execution", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"defaultTradingEnvironment":"REAL"`) {
			t.Fatalf("body = %s, want execution settings payload", rec.Body.String())
		}
	})

	t.Run("execution put rejects malformed payload", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/execution", strings.NewReader(`{"defaultTradingEnvironment":`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("exchange calendar get and put keep wrapped shape", func(t *testing.T) {
		getRec := httptest.NewRecorder()
		getReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/exchange-calendars", nil)
		router.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
		}
		if !strings.Contains(getRec.Body.String(), `"exchangeCalendars"`) || !strings.Contains(getRec.Body.String(), `"warmupMarkets":["US","HK"]`) || !strings.Contains(getRec.Body.String(), `"errorNotificationsEnabled":true`) {
			t.Fatalf("get body = %s, want wrapped calendar payload", getRec.Body.String())
		}

		putRec := httptest.NewRecorder()
		putReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/exchange-calendars", strings.NewReader(`{"exchangeCalendars":{"autoRefreshEnabled":false,"errorNotificationsEnabled":false,"refreshIntervalHours":6,"warmupMarkets":["US"]}}`))
		putReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(putRec, putReq)

		if putRec.Code != http.StatusOK {
			t.Fatalf("put status = %d, want 200, body=%s", putRec.Code, putRec.Body.String())
		}
		if store.calendars.AutoRefreshEnabled || store.calendars.ErrorNotificationsEnabled || store.calendars.RefreshIntervalHours != 6 || !reflect.DeepEqual(store.calendars.WarmupMarkets, []string{"US"}) {
			t.Fatalf("stored calendars = %#v", store.calendars)
		}
		if !reflect.DeepEqual(calendarSideEffect, store.calendars) {
			t.Fatalf("calendar side effect = %#v, want %#v", calendarSideEffect, store.calendars)
		}
		if !strings.Contains(putRec.Body.String(), `"exchangeCalendars"`) {
			t.Fatalf("put body = %s, want wrapped calendar payload", putRec.Body.String())
		}
	})

	t.Run("exchange calendar put rejects malformed payload", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/exchange-calendars", strings.NewReader(`{"exchangeCalendars":`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("broker settings get and integration put cover read and validation", func(t *testing.T) {
		getRec := httptest.NewRecorder()
		getReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/brokers", nil)
		router.ServeHTTP(getRec, getReq)

		if getRec.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200, body=%s", getRec.Code, getRec.Body.String())
		}
		if !strings.Contains(getRec.Body.String(), `"brokerId":"futu"`) || !strings.Contains(getRec.Body.String(), `"connected":true`) {
			t.Fatalf("get body = %s, want broker settings payload", getRec.Body.String())
		}

		badPutRec := httptest.NewRecorder()
		badPutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/brokers/futu/integration", strings.NewReader(`{"enabled":`))
		badPutReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(badPutRec, badPutReq)

		if badPutRec.Code != http.StatusBadRequest {
			t.Fatalf("put status = %d, want 400, body=%s", badPutRec.Code, badPutRec.Body.String())
		}
	})
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

	body := `{"exchangeCalendars":{"autoRefreshEnabled":false,"errorNotificationsEnabled":false,"refreshIntervalHours":12,"warmupMarkets":["US","HK"],"sourcePolicies":[{"market":"US","enabledSourceIds":["nyse_official"],"fallbackToBuiltin":true}]}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/settings/exchange-calendars", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	if !reflect.DeepEqual(store.calendars, sideEffect) {
		t.Fatalf("side effect = %#v, stored = %#v", sideEffect, store.calendars)
	}
	if store.calendars.RefreshIntervalHours != 12 || store.calendars.AutoRefreshEnabled || store.calendars.ErrorNotificationsEnabled {
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
	if !envelope.OK || envelope.Data.ExchangeCalendars.RefreshIntervalHours != 12 || envelope.Data.ExchangeCalendars.ErrorNotificationsEnabled {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestDataMigrationRoutesUseInjectedCallbacks(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	var rebuildPayload any
	service := srvsettings.NewService(store, srvsettings.WithDataMigration(
		func(context.Context) (any, error) {
			return map[string]any{"databases": []map[string]any{{"id": "adk", "status": "incompatible"}}}, nil
		},
		func(_ context.Context, payload any) (any, error) {
			rebuildPayload = payload
			return map[string]any{"scheduled": true, "restartRequired": true}, nil
		},
	))
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	statusRecorder := httptest.NewRecorder()
	router.ServeHTTP(statusRecorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/data-migration/databases", nil))
	if statusRecorder.Code != http.StatusOK || !strings.Contains(statusRecorder.Body.String(), `"id":"adk"`) {
		t.Fatalf("status response = %d %s", statusRecorder.Code, statusRecorder.Body.String())
	}

	rebuildRecorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/settings/data-migration/databases/rebuild", strings.NewReader(`{"mode":"single","databaseId":"adk","confirmation":"REBUILD adk"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rebuildRecorder, request)
	if rebuildRecorder.Code != http.StatusOK || !strings.Contains(rebuildRecorder.Body.String(), `"restartRequired":true`) {
		t.Fatalf("rebuild response = %d %s", rebuildRecorder.Code, rebuildRecorder.Body.String())
	}
	payload, ok := rebuildPayload.(map[string]any)
	if !ok || payload["databaseId"] != "adk" {
		t.Fatalf("rebuild payload = %#v", rebuildPayload)
	}
}
