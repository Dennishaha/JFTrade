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
	appearance      jfsettings.UIAppearanceSettings
	onboarding      jfsettings.OnboardingSettings
	execution       jfsettings.ExecutionSettings
	security        jfsettings.SecuritySettings
	adk             jfsettings.ADKRuntimeSettings
	integration     jfsettings.BrokerIntegration
	managedAccounts []jfsettings.ManagedBrokerAccount
	path            string
	hasAppearance   bool
}

func (s *fakeStore) Appearance() jfsettings.UIAppearanceSettings { return s.appearance }
func (s *fakeStore) Onboarding() jfsettings.OnboardingSettings   { return s.onboarding }
func (s *fakeStore) ExecutionSettings() jfsettings.ExecutionSettings {
	return s.execution
}
func (s *fakeStore) SecuritySettings() jfsettings.SecuritySettings { return s.security }
func (s *fakeStore) ADKSettings() jfsettings.ADKRuntimeSettings    { return s.adk }
func (s *fakeStore) Integration() jfsettings.BrokerIntegration     { return s.integration }
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
func (s *fakeStore) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	s.adk = input
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
	return nil
}
func (s *fakeStore) HasAppearance() bool { return s.hasAppearance }
func (s *fakeStore) Path() string        { return s.path }

func TestSaveSettingsTriggersSideEffects(t *testing.T) {
	store := &fakeStore{}
	var gotExecution jfsettings.ExecutionSettings
	var gotSecurity jfsettings.SecuritySettings
	var gotIntegration jfsettings.BrokerIntegration

	svc := NewService(store, WithSideEffects(SideEffects{
		OnExecutionChanged: func(settings jfsettings.ExecutionSettings) {
			gotExecution = settings
		},
		OnSecurityChanged: func(settings jfsettings.SecuritySettings) {
			gotSecurity = settings
		},
		OnIntegrationChanged: func(settings jfsettings.BrokerIntegration) {
			gotIntegration = settings
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

	integration := jfsettings.BrokerIntegration{BrokerID: "futu", Enabled: true}
	if _, err := svc.SaveIntegration(integration); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if !reflect.DeepEqual(gotIntegration, integration) {
		t.Fatalf("integration side effect = %#v, want %#v", gotIntegration, integration)
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
