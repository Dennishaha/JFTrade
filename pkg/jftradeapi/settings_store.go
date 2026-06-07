package jftradeapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FutuIntegrationConfig struct {
	Type                    string `json:"type"`
	Host                    string `json:"host"`
	APIPort                 int    `json:"apiPort"`
	WebSocketPort           int    `json:"websocketPort"`
	MaxWebSocketConnections int    `json:"maxWebSocketConnections"`
	UseEncryption           bool   `json:"useEncryption"`
	WebSocketKey            string `json:"websocketKey"`
	TradeMarket             string `json:"tradeMarket"`
	SecurityFirm            string `json:"securityFirm"`
}

type BrokerIntegration struct {
	BrokerID  string                `json:"brokerId"`
	Enabled   bool                  `json:"enabled"`
	Config    FutuIntegrationConfig `json:"config"`
	UpdatedAt string                `json:"updatedAt"`
	CreatedAt string                `json:"createdAt"`
}

type ManagedBrokerAccount struct {
	ID                 string  `json:"id"`
	BrokerID           string  `json:"brokerId"`
	AccountID          string  `json:"accountId"`
	DisplayName        string  `json:"displayName"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	SecurityFirm       *string `json:"securityFirm"`
	Enabled            bool    `json:"enabled"`
	UpdatedAt          string  `json:"updatedAt"`
	CreatedAt          string  `json:"createdAt"`
}

type InterfaceSettings struct {
	APIBind       string `json:"apiBind"`
	GUIBind       string `json:"guiBind,omitempty"`
	GUIAPIBaseURL string `json:"guiApiBaseUrl,omitempty"`
}

type UIAppearanceSettings struct {
	UpColor   string `json:"upColor"`
	DownColor string `json:"downColor"`
}

type OnboardingSettings struct {
	Completed    bool   `json:"completed"`
	CompletedAt  string `json:"completedAt,omitempty"`
	DismissedAt  string `json:"dismissedAt,omitempty"`
	LastBrokerID string `json:"lastBrokerId"`
}

type ExecutionSettings struct {
	DefaultTradingEnvironment      string `json:"defaultTradingEnvironment"`
	BrokerOrderHistoryLookbackDays int    `json:"brokerOrderHistoryLookbackDays"`
	SeenFillRetentionDays          int    `json:"seenFillRetentionDays"`
}

type SecuritySettings struct {
	AdminAuthRequired bool `json:"adminAuthRequired"`
}

type settingsFile struct {
	Interfaces  *InterfaceSettings     `json:"interfaces,omitempty"`
	Integration *BrokerIntegration     `json:"integration,omitempty"`
	Accounts    []ManagedBrokerAccount `json:"accounts,omitempty"`
	Appearance  *UIAppearanceSettings  `json:"appearance,omitempty"`
	Onboarding  *OnboardingSettings    `json:"onboarding,omitempty"`
	Execution   *ExecutionSettings     `json:"execution,omitempty"`
	Security    *SecuritySettings      `json:"security,omitempty"`
}

type SettingsStore struct {
	path string
	mu   sync.RWMutex
	data settingsFile
}

func NewSettingsStore(path string) (*SettingsStore, error) {
	store := &SettingsStore{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SettingsStore) ensureBootstrapFile(defaults launchDefaults) error {
	if _, err := os.Stat(s.path); err == nil {
		if s.hasAppearance() {
			return nil
		}
		_, err := s.saveAppearance(defaultUIAppearanceSettings())
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.mu.Lock()
	s.data.Interfaces = interfaceSettingsPointer(normalizeInterfaceSettings(interfaceSettingsFromDefaults(defaults), defaults))
	s.data.Appearance = uiAppearanceSettingsPointer(defaultUIAppearanceSettings())
	err := s.persistLocked()
	s.mu.Unlock()
	return err
}

func (s *SettingsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = settingsFile{}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = settingsFile{}
		return nil
	}
	return json.Unmarshal(data, &s.data)
}

func (s *SettingsStore) integration() BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration != nil {
		return *s.data.Integration
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return BrokerIntegration{
		BrokerID:  "futu",
		Enabled:   false,
		Config:    defaultFutuConfig(),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

func (s *SettingsStore) savedIntegration() *BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration == nil {
		return nil
	}
	integration := *s.data.Integration
	return &integration
}

func (s *SettingsStore) interfaceSettings(defaults launchDefaults) InterfaceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Interfaces != nil {
		return normalizeInterfaceSettings(*s.data.Interfaces, defaults)
	}
	return normalizeInterfaceSettings(interfaceSettingsFromDefaults(defaults), defaults)
}

func (s *SettingsStore) appearance() UIAppearanceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Appearance != nil {
		return normalizeUIAppearanceSettings(*s.data.Appearance)
	}
	return defaultUIAppearanceSettings()
}

func (s *SettingsStore) onboarding() OnboardingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Onboarding != nil {
		return normalizeOnboardingSettings(*s.data.Onboarding)
	}
	return defaultOnboardingSettings()
}

func (s *SettingsStore) executionSettings() ExecutionSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Execution != nil {
		return normalizeExecutionSettings(*s.data.Execution)
	}
	return defaultExecutionSettings()
}

func (s *SettingsStore) securitySettings() SecuritySettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Security != nil {
		return normalizeSecuritySettings(*s.data.Security)
	}
	return defaultSecuritySettings()
}

func (s *SettingsStore) hasAppearance() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Appearance != nil
}

func (s *SettingsStore) saveAppearance(input UIAppearanceSettings) (UIAppearanceSettings, error) {
	normalized := normalizeUIAppearanceSettings(input)

	s.mu.Lock()
	s.data.Appearance = uiAppearanceSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *SettingsStore) saveOnboarding(input OnboardingSettings) (OnboardingSettings, error) {
	normalized := normalizeOnboardingSettings(input)

	s.mu.Lock()
	s.data.Onboarding = onboardingSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *SettingsStore) saveExecutionSettings(input ExecutionSettings) (ExecutionSettings, error) {
	normalized := normalizeExecutionSettings(input)

	s.mu.Lock()
	s.data.Execution = executionSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *SettingsStore) saveSecuritySettings(input SecuritySettings) (SecuritySettings, error) {
	normalized := normalizeSecuritySettings(input)

	s.mu.Lock()
	s.data.Security = securitySettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *SettingsStore) saveIntegration(input BrokerIntegration) (BrokerIntegration, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.BrokerID = "futu"
	input.Config = normalizeFutuConfig(input.Config)
	input.UpdatedAt = now
	if input.CreatedAt == "" {
		if existing := s.savedIntegration(); existing != nil {
			input.CreatedAt = existing.CreatedAt
		}
		if input.CreatedAt == "" {
			input.CreatedAt = now
		}
	}

	s.mu.Lock()
	if s.data.Interfaces == nil {
		s.data.Interfaces = interfaceSettingsPointer(normalizeInterfaceSettings(interfaceSettingsFromDefaults(launchDefaults{}), launchDefaults{}))
	}
	s.data.Integration = &input
	err := s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return input, err
	}

	s.applyRuntimeEnv()
	return input, nil
}

func (s *SettingsStore) managedAccounts() []ManagedBrokerAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]ManagedBrokerAccount, len(s.data.Accounts))
	copy(accounts, s.data.Accounts)
	return accounts
}

func (s *SettingsStore) createManagedAccount(input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if sameManagedAccountScope(*account, input) {
			input.ID = account.ID
			input.CreatedAt = account.CreatedAt
			if input.CreatedAt == "" {
				input.CreatedAt = now
			}
			s.data.Accounts[index] = input
			if err := s.persistLocked(); err != nil {
				return input, err
			}
			return input, nil
		}
	}

	if input.ID == "" {
		input.ID = buildManagedAccountID(input)
	}
	if input.CreatedAt == "" {
		input.CreatedAt = now
	}
	s.data.Accounts = append(s.data.Accounts, input)
	if err := s.persistLocked(); err != nil {
		return input, err
	}
	return input, nil
}

func (s *SettingsStore) updateManagedAccount(id string, input ManagedBrokerAccount) (ManagedBrokerAccount, error) {
	input = normalizeManagedBrokerAccount(input)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if account.ID != id {
			continue
		}
		input.ID = account.ID
		input.CreatedAt = account.CreatedAt
		input.UpdatedAt = now
		s.data.Accounts[index] = input
		if err := s.persistLocked(); err != nil {
			return input, err
		}
		return input, nil
	}

	return ManagedBrokerAccount{}, os.ErrNotExist
}

func (s *SettingsStore) deleteManagedAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		if s.data.Accounts[index].ID != id {
			continue
		}
		s.data.Accounts = append(s.data.Accounts[:index], s.data.Accounts[index+1:]...)
		return s.persistLocked()
	}
	return os.ErrNotExist
}

func (s *SettingsStore) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func interfaceSettingsFromDefaults(defaults launchDefaults) InterfaceSettings {
	settings := InterfaceSettings{APIBind: defaults.apiBind}
	if strings.TrimSpace(defaults.guiBind) != "" {
		settings.GUIBind = defaults.guiBind
		settings.GUIAPIBaseURL = apiBaseURLForBind(defaults.apiBind)
	}
	return settings
}

func normalizeInterfaceSettings(input InterfaceSettings, defaults launchDefaults) InterfaceSettings {
	settings := input
	settings.APIBind = strings.TrimSpace(settings.APIBind)
	settings.GUIBind = strings.TrimSpace(settings.GUIBind)
	settings.GUIAPIBaseURL = strings.TrimRight(strings.TrimSpace(settings.GUIAPIBaseURL), "/")

	if settings.APIBind == "" {
		settings.APIBind = defaults.apiBind
	}
	if settings.APIBind == "" {
		settings.APIBind = defaultDevelopmentAPIBind
	}
	if settings.GUIBind == "" {
		settings.GUIBind = defaults.guiBind
	}
	if settings.GUIAPIBaseURL == "" && settings.GUIBind != "" {
		settings.GUIAPIBaseURL = apiBaseURLForBind(settings.APIBind)
	}
	return settings
}

func interfaceSettingsPointer(value InterfaceSettings) *InterfaceSettings {
	settings := value
	return &settings
}

func defaultUIAppearanceSettings() UIAppearanceSettings {
	return UIAppearanceSettings{
		UpColor:   "#16c784",
		DownColor: "#ea3943",
	}
}

func normalizeUIAppearanceSettings(input UIAppearanceSettings) UIAppearanceSettings {
	defaults := defaultUIAppearanceSettings()
	return UIAppearanceSettings{
		UpColor:   normalizeHexColor(input.UpColor, defaults.UpColor),
		DownColor: normalizeHexColor(input.DownColor, defaults.DownColor),
	}
}

func normalizeHexColor(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 7 || !strings.HasPrefix(trimmed, "#") {
		return fallback
	}
	for _, r := range trimmed[1:] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return fallback
		}
	}
	return strings.ToLower(trimmed)
}

func uiAppearanceSettingsPointer(value UIAppearanceSettings) *UIAppearanceSettings {
	settings := value
	return &settings
}

func defaultOnboardingSettings() OnboardingSettings {
	return OnboardingSettings{
		Completed:    false,
		LastBrokerID: "",
	}
}

func normalizeOnboardingSettings(input OnboardingSettings) OnboardingSettings {
	settings := input
	settings.LastBrokerID = strings.TrimSpace(settings.LastBrokerID)
	if !settings.Completed {
		settings.CompletedAt = ""
	}
	return settings
}

func onboardingSettingsPointer(value OnboardingSettings) *OnboardingSettings {
	settings := value
	return &settings
}

func defaultExecutionSettings() ExecutionSettings {
	return ExecutionSettings{
		DefaultTradingEnvironment:      "SIMULATE",
		BrokerOrderHistoryLookbackDays: 30,
		SeenFillRetentionDays:          90,
	}
}

func normalizeExecutionSettings(input ExecutionSettings) ExecutionSettings {
	defaults := defaultExecutionSettings()
	settings := input
	settings.DefaultTradingEnvironment = normalizeExecutionTradingEnvironment(settings.DefaultTradingEnvironment)
	if settings.DefaultTradingEnvironment == "" {
		settings.DefaultTradingEnvironment = defaults.DefaultTradingEnvironment
	}
	if settings.BrokerOrderHistoryLookbackDays < 1 {
		settings.BrokerOrderHistoryLookbackDays = defaults.BrokerOrderHistoryLookbackDays
	}
	if settings.BrokerOrderHistoryLookbackDays > 365 {
		settings.BrokerOrderHistoryLookbackDays = 365
	}
	if settings.SeenFillRetentionDays < 1 {
		settings.SeenFillRetentionDays = defaults.SeenFillRetentionDays
	}
	if settings.SeenFillRetentionDays > 3650 {
		settings.SeenFillRetentionDays = 3650
	}
	return settings
}

func normalizeExecutionTradingEnvironment(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SIMULATE", "REAL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}

func executionSettingsPointer(value ExecutionSettings) *ExecutionSettings {
	settings := value
	return &settings
}

func defaultSecuritySettings() SecuritySettings {
	return SecuritySettings{
		AdminAuthRequired: false,
	}
}

func normalizeSecuritySettings(input SecuritySettings) SecuritySettings {
	return SecuritySettings{
		AdminAuthRequired: input.AdminAuthRequired,
	}
}

func securitySettingsPointer(value SecuritySettings) *SecuritySettings {
	settings := value
	return &settings
}
