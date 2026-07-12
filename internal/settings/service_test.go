package settings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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
	mcpServer           jfsettings.MCPServerSettings
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
func (s *fakeStore) MCPServerSettings() jfsettings.MCPServerSettings {
	return s.mcpServer
}
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
func (s *fakeStore) SaveMCPServerSettings(input jfsettings.MCPServerSettings) (jfsettings.MCPServerSettings, error) {
	s.mcpServer = input
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
		OnSecurityChanged: func(settings jfsettings.SecuritySettings) error {
			gotSecurity = settings
			return nil
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

	securityUpdate := jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true,
		NewPassword:      "a memorable Web passphrase",
	}
	security, err := svc.SaveSecuritySettings(securityUpdate)
	if err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}
	if !security.WebAccessEnabled || !security.PasswordConfigured ||
		security.WebPort != jfsettings.DefaultWebAccessPort ||
		!strings.HasPrefix(security.PasswordHash, "$argon2id$") ||
		strings.Contains(security.PasswordHash, securityUpdate.NewPassword) {
		t.Fatalf("security result = %#v", security)
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

func TestSaveSecuritySettingsRejectsInvalidWebPort(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	_, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{WebPort: 80})
	if !errors.Is(err, ErrWebAccessPortInvalid) {
		t.Fatalf("SaveSecuritySettings error = %v, want %v", err, ErrWebAccessPortInvalid)
	}
}

func TestSaveSecuritySettingsRollsBackWhenRuntimeListenerUpdateFails(t *testing.T) {
	current := jfsettings.SecuritySettings{
		WebAccessEnabled:   true,
		PasswordConfigured: true,
		PasswordHash:       "stored-hash",
		WebPort:            6688,
	}
	store := &fakeStore{security: current}
	svc := NewService(store, WithSideEffects(SideEffects{
		OnSecurityChanged: func(jfsettings.SecuritySettings) error {
			return errors.New("port occupied")
		},
	}))

	result, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
		WebAccessEnabled: true,
		WebPort:          7443,
	})
	if !errors.Is(err, ErrWebAccessRuntimeUpdate) {
		t.Fatalf("SaveSecuritySettings error = %v", err)
	}
	if !reflect.DeepEqual(result, current) || !reflect.DeepEqual(store.security, current) {
		t.Fatalf("runtime failure did not restore settings: result=%#v stored=%#v", result, store.security)
	}
}

func TestMCPServerTokenResetDoesNotLeakAndInvalidatesPreviousToken(t *testing.T) {
	store := &fakeStore{mcpServer: jfsettings.MCPServerSettings{
		Enabled:   true,
		Port:      jfsettings.DefaultMCPServerPort,
		AuthMode:  "token",
		TokenHash: "old-verifier",
	}}
	var applied []jfsettings.MCPServerSettings
	svc := NewService(store, WithSideEffects(SideEffects{
		OnMCPServerChanged: func(settings jfsettings.MCPServerSettings) error {
			applied = append(applied, settings)
			return nil
		},
	}))
	generated := []string{"first-secret", "second-secret"}
	svc.newMCPToken = func() (string, error) {
		value := generated[0]
		generated = generated[1:]
		return value, nil
	}
	svc.hashPassword = func(value string) (string, error) { return "hash:" + value, nil }

	first, firstToken, err := svc.ResetMCPServerToken()
	if err != nil {
		t.Fatalf("ResetMCPServerToken first: %v", err)
	}
	if firstToken != "first-secret" || first.TokenHash != "hash:first-secret" || !first.TokenConfigured {
		t.Fatalf("first reset = settings=%#v token=%q", first, firstToken)
	}
	second, secondToken, err := svc.ResetMCPServerToken()
	if err != nil {
		t.Fatalf("ResetMCPServerToken second: %v", err)
	}
	if secondToken != "second-secret" || second.TokenHash != "hash:second-secret" || second.TokenHash == first.TokenHash {
		t.Fatalf("second reset = settings=%#v token=%q", second, secondToken)
	}
	if len(applied) != 2 || applied[0].TokenHash == applied[1].TokenHash {
		t.Fatalf("applied settings = %#v", applied)
	}
	encoded, err := json.Marshal(svc.GetMCPServerSettings())
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	for _, secret := range []string{"first-secret", "second-secret", "hash:second-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("settings response leaked %q: %s", secret, encoded)
		}
	}
}

func TestSaveMCPServerSettingsRollsBackWhenListenerUpdateFails(t *testing.T) {
	current := jfsettings.MCPServerSettings{
		Enabled:         true,
		Port:            6697,
		AuthMode:        "token",
		TokenConfigured: true,
		TokenHash:       "stored-hash",
	}
	store := &fakeStore{mcpServer: current}
	svc := NewService(store, WithSideEffects(SideEffects{
		OnMCPServerChanged: func(jfsettings.MCPServerSettings) error {
			return errors.New("port occupied")
		},
	}))

	result, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{
		Enabled: true, Port: 7443, AuthMode: "token",
	})
	if !errors.Is(err, ErrMCPServerRuntimeUpdate) {
		t.Fatalf("SaveMCPServerSettings error = %v", err)
	}
	if !reflect.DeepEqual(result, current) || !reflect.DeepEqual(store.mcpServer, current) {
		t.Fatalf("runtime failure did not restore MCP settings: result=%#v stored=%#v", result, store.mcpServer)
	}
}

func TestSaveMCPServerSettingsValidatesTokenAndPort(t *testing.T) {
	store := &fakeStore{mcpServer: jfsettings.MCPServerSettings{
		Port: jfsettings.DefaultMCPServerPort, AuthMode: "token",
	}}
	svc := NewService(store)

	if _, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{Enabled: true, Port: 80, AuthMode: "token"}); !errors.Is(err, ErrMCPServerPortInvalid) {
		t.Fatalf("invalid port error = %v", err)
	}
	if _, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{Enabled: true, AuthMode: "token"}); !errors.Is(err, ErrMCPServerTokenRequired) {
		t.Fatalf("missing token error = %v", err)
	}
	if _, err := svc.SaveMCPServerSettings(jfsettings.MCPServerSettingsUpdate{Enabled: false, AuthMode: "invalid"}); !errors.Is(err, ErrMCPServerAuthModeInvalid) {
		t.Fatalf("invalid auth mode error = %v", err)
	}
}

func TestConcurrentSecuritySavesPreserveNewestPasswordAndCallbackOrder(t *testing.T) {
	store := &fakeStore{security: jfsettings.SecuritySettings{
		WebAccessEnabled:   true,
		PasswordConfigured: true,
		PasswordHash:       "old-hash",
	}}
	hashStarted := make(chan struct{})
	continueHash := make(chan struct{})
	var callbacksMu sync.Mutex
	callbacks := make([]jfsettings.SecuritySettings, 0, 2)
	svc := NewService(store, WithSideEffects(SideEffects{
		OnSecurityChanged: func(settings jfsettings.SecuritySettings) error {
			callbacksMu.Lock()
			callbacks = append(callbacks, settings)
			callbacksMu.Unlock()
			return nil
		},
	}))
	svc.hashPassword = func(string) (string, error) {
		close(hashStarted)
		<-continueHash
		return "new-hash", nil
	}

	type saveResult struct {
		settings jfsettings.SecuritySettings
		err      error
	}
	firstResult := make(chan saveResult, 1)
	secondResult := make(chan saveResult, 1)
	go func() {
		settings, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
			WebAccessEnabled: true,
			NewPassword:      "replacement browser password",
		})
		firstResult <- saveResult{settings: settings, err: err}
	}()
	select {
	case <-hashStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first password hash did not start")
	}
	go func() {
		settings, err := svc.SaveSecuritySettings(jfsettings.SecuritySettingsUpdate{
			WebAccessEnabled:    true,
			PublicAccessEnabled: true,
		})
		secondResult <- saveResult{settings: settings, err: err}
	}()
	select {
	case result := <-secondResult:
		t.Fatalf("second save bypassed the security lock: %#v", result)
	case <-time.After(50 * time.Millisecond):
	}
	close(continueHash)

	first := <-firstResult
	second := <-secondResult
	if first.err != nil || second.err != nil {
		t.Fatalf("save errors = %v, %v", first.err, second.err)
	}
	if second.settings.PasswordHash != "new-hash" || !second.settings.PublicAccessEnabled {
		t.Fatalf("final save = %#v", second.settings)
	}
	if store.security.PasswordHash != "new-hash" || !store.security.PublicAccessEnabled {
		t.Fatalf("stored security = %#v", store.security)
	}
	callbacksMu.Lock()
	defer callbacksMu.Unlock()
	if len(callbacks) != 2 || callbacks[0].PasswordHash != "new-hash" || callbacks[0].PublicAccessEnabled || !callbacks[1].PublicAccessEnabled {
		t.Fatalf("security callbacks = %#v", callbacks)
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
		security: jfsettings.SecuritySettings{
			WebAccessEnabled:   true,
			PasswordConfigured: true,
			PasswordHash:       "stored-verifier",
		},
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
