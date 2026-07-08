package settings

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type fakeStore struct {
	appearance          jfsettings.UIAppearanceSettings
	onboarding          jfsettings.OnboardingSettings
	execution           jfsettings.ExecutionSettings
	security            jfsettings.SecuritySettings
	systemNotifications jfsettings.SystemNotificationSettings
	adk                 jfsettings.ADKRuntimeSettings
	pineWorker          jfsettings.PineWorkerSettings
	calendars           jfsettings.ExchangeCalendarSettings
	integration         jfsettings.BrokerIntegration
	managedAccounts     []jfsettings.ManagedBrokerAccount
	path                string
	hasAppearance       bool
	bootstrapCalls      int
	bootstrapArg        jfsettings.LaunchDefaults
}

func (s *fakeStore) Appearance() jfsettings.UIAppearanceSettings { return s.appearance }
func (s *fakeStore) Onboarding() jfsettings.OnboardingSettings   { return s.onboarding }
func (s *fakeStore) ExecutionSettings() jfsettings.ExecutionSettings {
	return s.execution
}
func (s *fakeStore) SecuritySettings() jfsettings.SecuritySettings { return s.security }
func (s *fakeStore) SystemNotificationSettings() jfsettings.SystemNotificationSettings {
	return s.systemNotifications
}
func (s *fakeStore) ADKSettings() jfsettings.ADKRuntimeSettings { return s.adk }
func (s *fakeStore) PineWorkerSettings() jfsettings.PineWorkerSettings {
	return s.pineWorker
}
func (s *fakeStore) ExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	return s.calendars
}
func (s *fakeStore) Integration() jfsettings.BrokerIntegration { return s.integration }
func (s *fakeStore) SavedIntegration() *jfsettings.BrokerIntegration {
	if s.integration.BrokerID == "" {
		return nil
	}
	return new(s.integration)
}
func (s *fakeStore) ManagedAccounts() []jfsettings.ManagedBrokerAccount {
	return append([]jfsettings.ManagedBrokerAccount(nil), s.managedAccounts...)
}
func (s *fakeStore) InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	return jfsettings.InterfaceSettings{APIBind: defaults.APIBind, GUIBind: defaults.GUIBind}
}
func (s *fakeStore) SaveAppearance(input jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error) {
	s.appearance = input
	s.hasAppearance = true
	return input, nil
}
func (s *fakeStore) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	s.onboarding = input
	return input, nil
}
func (s *fakeStore) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	s.execution = input
	return input, nil
}
func (s *fakeStore) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	s.security = input
	return input, nil
}
func (s *fakeStore) SaveSystemNotificationSettings(input jfsettings.SystemNotificationSettings) (jfsettings.SystemNotificationSettings, error) {
	s.systemNotifications = input
	return input, nil
}
func (s *fakeStore) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	s.adk = input
	return input, nil
}
func (s *fakeStore) SavePineWorkerSettings(input jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error) {
	s.pineWorker = input
	return input, nil
}
func (s *fakeStore) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	s.calendars = input
	return input, nil
}
func (s *fakeStore) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	s.integration = input
	return input, nil
}
func (s *fakeStore) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	s.managedAccounts = append(s.managedAccounts, input)
	return input, nil
}
func (s *fakeStore) UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	input.ID = id
	for i := range s.managedAccounts {
		if s.managedAccounts[i].ID == id {
			s.managedAccounts[i] = input
			return input, nil
		}
	}
	s.managedAccounts = append(s.managedAccounts, input)
	return input, nil
}
func (s *fakeStore) DeleteManagedAccount(id string) error {
	for i := range s.managedAccounts {
		if s.managedAccounts[i].ID == id {
			s.managedAccounts = append(s.managedAccounts[:i], s.managedAccounts[i+1:]...)
			return nil
		}
	}
	return nil
}
func (s *fakeStore) EnsureBootstrapFile(defaults jfsettings.LaunchDefaults) error {
	s.bootstrapCalls++
	s.bootstrapArg = defaults
	return nil
}
func (s *fakeStore) HasAppearance() bool { return s.hasAppearance }
func (s *fakeStore) Path() string        { return s.path }

func TestSaveSettingsTriggersSideEffects(t *testing.T) {
	store := &fakeStore{}
	var gotExecution jfsettings.ExecutionSettings
	var gotSecurity jfsettings.SecuritySettings
	var gotCalendars jfsettings.ExchangeCalendarSettings
	var gotIntegration jfsettings.BrokerIntegration
	var gotPineWorker jfsettings.PineWorkerSettings

	svc := NewService(store, WithSideEffects(SideEffects{
		OnExecutionChanged: func(settings jfsettings.ExecutionSettings) {
			gotExecution = settings
		},
		OnSecurityChanged: func(settings jfsettings.SecuritySettings) {
			gotSecurity = settings
		},
		OnExchangeCalendarsChanged: func(settings jfsettings.ExchangeCalendarSettings) {
			gotCalendars = settings
		},
		OnIntegrationChanged: func(settings jfsettings.BrokerIntegration) {
			gotIntegration = settings
		},
		OnPineWorkerChanged: func(settings jfsettings.PineWorkerSettings) {
			gotPineWorker = settings
		},
	}))

	execution := jfsettings.ExecutionSettings{DefaultTradingEnvironment: "SIMULATE", SeenFillRetentionDays: 7}
	if _, err := svc.SaveExecutionSettings(execution); err != nil {
		t.Fatalf("SaveExecutionSettings: %v", err)
	}
	if !reflect.DeepEqual(gotExecution, execution) {
		t.Fatalf("execution side effect = %#v, want %#v", gotExecution, execution)
	}

	security := jfsettings.SecuritySettings{AdminAuthRequired: true}
	if _, err := svc.SaveSecuritySettings(security); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}
	if !reflect.DeepEqual(gotSecurity, security) {
		t.Fatalf("security side effect = %#v, want %#v", gotSecurity, security)
	}

	calendars := jfsettings.ExchangeCalendarSettings{AutoRefreshEnabled: true, RefreshIntervalHours: 24}
	if _, err := svc.SaveExchangeCalendarSettings(calendars); err != nil {
		t.Fatalf("SaveExchangeCalendarSettings: %v", err)
	}
	if !reflect.DeepEqual(gotCalendars, calendars) {
		t.Fatalf("calendar side effect = %#v, want %#v", gotCalendars, calendars)
	}

	integration := jfsettings.BrokerIntegration{BrokerID: "futu", Enabled: true}
	if _, err := svc.SaveIntegration(integration); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if !reflect.DeepEqual(gotIntegration, integration) {
		t.Fatalf("integration side effect = %#v, want %#v", gotIntegration, integration)
	}

	pineWorker := jfsettings.PineWorkerSettings{BacktestWorkerLimit: 3, InstanceWorkerLimit: 8}
	if _, err := svc.SavePineWorkerSettings(pineWorker); err != nil {
		t.Fatalf("SavePineWorkerSettings: %v", err)
	}
	if !reflect.DeepEqual(gotPineWorker, pineWorker) {
		t.Fatalf("pine worker side effect = %#v, want %#v", gotPineWorker, pineWorker)
	}
}

func TestDefaultCallbacksReturnEmptyMaps(t *testing.T) {
	svc := NewService(&fakeStore{})

	if got := svc.OnboardingState(context.Background()); len(got) != 0 {
		t.Fatalf("OnboardingState = %#v, want empty map", got)
	}
	if got := svc.BrokerSettings(); len(got) != 0 {
		t.Fatalf("BrokerSettings = %#v, want empty map", got)
	}
}

func TestSaveIntegrationSideEffectAppliesRuntimeEnv(t *testing.T) {
	t.Setenv("FUTU_OPEND_ADDR", "before")
	t.Setenv("JFTRADE_FUTU_WEBSOCKET_PORT", "before")

	store, err := settingsfile.New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settingsfile.New: %v", err)
	}
	svc := NewService(store, WithSideEffects(SideEffects{
		OnIntegrationChanged: apiruntime.ApplyIntegrationEnv,
	}))

	_, err = svc.SaveIntegration(jfsettings.BrokerIntegration{
		Config: jfsettings.FutuIntegrationConfig{
			Host:          "127.0.0.3",
			APIPort:       23333,
			WebSocketPort: 23334,
		},
	})
	if err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if got := os.Getenv("FUTU_OPEND_ADDR"); got != "127.0.0.3:23333" {
		t.Fatalf("FUTU_OPEND_ADDR = %q", got)
	}
	if got := os.Getenv("JFTRADE_FUTU_WEBSOCKET_PORT"); got != "23334" {
		t.Fatalf("JFTRADE_FUTU_WEBSOCKET_PORT = %q", got)
	}
}

func TestServiceDelegatesGettersAndSimpleSavers(t *testing.T) {
	store := &fakeStore{
		appearance: jfsettings.UIAppearanceSettings{UpColor: "#00ff00", DownColor: "#ff0000"},
		onboarding: jfsettings.OnboardingSettings{Completed: true},
		execution: jfsettings.ExecutionSettings{
			DefaultTradingEnvironment: "REAL",
			SeenFillRetentionDays:     30,
		},
		security:      jfsettings.SecuritySettings{AdminAuthRequired: true},
		adk:           jfsettings.ADKRuntimeSettings{RunTimeoutMs: 15000, StreamIdleTimeoutMs: 5000},
		pineWorker:    jfsettings.PineWorkerSettings{BacktestWorkerLimit: 2, InstanceWorkerLimit: 10},
		calendars:     jfsettings.ExchangeCalendarSettings{AutoRefreshEnabled: true, RefreshIntervalHours: 8},
		integration:   jfsettings.BrokerIntegration{BrokerID: "futu", Enabled: true},
		hasAppearance: true,
	}
	svc := NewService(store)

	if got := svc.GetAppearance(); !reflect.DeepEqual(got, store.appearance) {
		t.Fatalf("GetAppearance() = %#v, want %#v", got, store.appearance)
	}
	if got := svc.GetOnboarding(); !reflect.DeepEqual(got, store.onboarding) {
		t.Fatalf("GetOnboarding() = %#v, want %#v", got, store.onboarding)
	}
	if got := svc.GetExecutionSettings(); !reflect.DeepEqual(got, store.execution) {
		t.Fatalf("GetExecutionSettings() = %#v, want %#v", got, store.execution)
	}
	if got := svc.GetSecuritySettings(); !reflect.DeepEqual(got, store.security) {
		t.Fatalf("GetSecuritySettings() = %#v, want %#v", got, store.security)
	}
	if got := svc.GetADKRuntimeSettings(); !reflect.DeepEqual(got, store.adk) {
		t.Fatalf("GetADKRuntimeSettings() = %#v, want %#v", got, store.adk)
	}
	if got := svc.GetPineWorkerSettings(); !reflect.DeepEqual(got, store.pineWorker) {
		t.Fatalf("GetPineWorkerSettings() = %#v, want %#v", got, store.pineWorker)
	}
	if got := svc.GetExchangeCalendarSettings(); !reflect.DeepEqual(got, store.calendars) {
		t.Fatalf("GetExchangeCalendarSettings() = %#v, want %#v", got, store.calendars)
	}
	if got := svc.GetIntegration(); !reflect.DeepEqual(got, store.integration) {
		t.Fatalf("GetIntegration() = %#v, want %#v", got, store.integration)
	}
	if got := svc.GetSavedIntegration(); got == nil || !reflect.DeepEqual(*got, store.integration) {
		t.Fatalf("GetSavedIntegration() = %#v, want %#v", got, store.integration)
	}
	if !svc.HasAppearance() {
		t.Fatal("HasAppearance() = false, want true")
	}

	updatedAppearance := jfsettings.UIAppearanceSettings{UpColor: "#111111", DownColor: "#222222"}
	if got, err := svc.SaveAppearance(updatedAppearance); err != nil || !reflect.DeepEqual(got, updatedAppearance) {
		t.Fatalf("SaveAppearance() = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(store.appearance, updatedAppearance) {
		t.Fatalf("stored appearance = %#v, want %#v", store.appearance, updatedAppearance)
	}

	updatedOnboarding := jfsettings.OnboardingSettings{Completed: false}
	if got, err := svc.SaveOnboarding(updatedOnboarding); err != nil || !reflect.DeepEqual(got, updatedOnboarding) {
		t.Fatalf("SaveOnboarding() = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(store.onboarding, updatedOnboarding) {
		t.Fatalf("stored onboarding = %#v, want %#v", store.onboarding, updatedOnboarding)
	}

	updatedADK := jfsettings.ADKRuntimeSettings{RunTimeoutMs: 25000, StreamIdleTimeoutMs: 10000}
	if got, err := svc.SaveADKRuntimeSettings(updatedADK); err != nil || !reflect.DeepEqual(got, updatedADK) {
		t.Fatalf("SaveADKRuntimeSettings() = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(store.adk, updatedADK) {
		t.Fatalf("stored ADK settings = %#v, want %#v", store.adk, updatedADK)
	}

	updatedPineWorker := jfsettings.PineWorkerSettings{BacktestWorkerLimit: 4, InstanceWorkerLimit: 12}
	if got, err := svc.SavePineWorkerSettings(updatedPineWorker); err != nil || !reflect.DeepEqual(got, updatedPineWorker) {
		t.Fatalf("SavePineWorkerSettings() = %#v, %v", got, err)
	}
	if !reflect.DeepEqual(store.pineWorker, updatedPineWorker) {
		t.Fatalf("stored pine worker settings = %#v, want %#v", store.pineWorker, updatedPineWorker)
	}
}

func TestServiceDelegatesProvidersAndLifecycle(t *testing.T) {
	store := &fakeStore{
		managedAccounts: []jfsettings.ManagedBrokerAccount{
			{ID: "managed-1", AccountID: "acc-1"},
			{ID: "managed-2", AccountID: "acc-2"},
		},
	}
	svc := NewService(
		store,
		WithBrokerSettings(func() map[string]any {
			return map[string]any{"connected": true, "brokerId": "futu"}
		}),
		WithOnboardingState(func(context.Context) map[string]any {
			return map[string]any{"step": "accounts", "complete": false}
		}),
	)

	if got := svc.BrokerSettings(); !reflect.DeepEqual(got, map[string]any{"connected": true, "brokerId": "futu"}) {
		t.Fatalf("BrokerSettings() = %#v", got)
	}
	if got := svc.OnboardingState(context.Background()); !reflect.DeepEqual(got, map[string]any{"step": "accounts", "complete": false}) {
		t.Fatalf("OnboardingState() = %#v", got)
	}

	accounts := svc.ListManagedAccounts()
	if len(accounts) != 2 || accounts[0].ID != "managed-1" || accounts[1].ID != "managed-2" {
		t.Fatalf("ListManagedAccounts() = %#v", accounts)
	}

	updated, err := svc.UpdateManagedAccount("managed-2", jfsettings.ManagedBrokerAccount{
		ID:                 "client-supplied",
		AccountID:          "acc-2",
		DisplayName:        "Primary",
		TradingEnvironment: "SIMULATE",
	})
	if err != nil {
		t.Fatalf("UpdateManagedAccount() error = %v", err)
	}
	if updated.ID != "managed-2" || updated.DisplayName != "Primary" {
		t.Fatalf("UpdateManagedAccount() = %#v", updated)
	}
	if store.managedAccounts[1].ID != "managed-2" || store.managedAccounts[1].DisplayName != "Primary" {
		t.Fatalf("stored managed accounts after update = %#v", store.managedAccounts)
	}

	if err := svc.DeleteManagedAccount("managed-1"); err != nil {
		t.Fatalf("DeleteManagedAccount() error = %v", err)
	}
	if len(store.managedAccounts) != 1 || store.managedAccounts[0].ID != "managed-2" {
		t.Fatalf("stored managed accounts after delete = %#v", store.managedAccounts)
	}

	defaults := jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:5173"}
	if err := svc.EnsureBootstrap(defaults); err != nil {
		t.Fatalf("EnsureBootstrap() error = %v", err)
	}
	if store.bootstrapCalls != 1 || !reflect.DeepEqual(store.bootstrapArg, defaults) {
		t.Fatalf("EnsureBootstrap delegation = calls:%d arg:%#v", store.bootstrapCalls, store.bootstrapArg)
	}
}
