package settingsfile

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	defaultFutuHost            = "127.0.0.1"
	defaultFutuAPIPort         = 11110
	defaultFutuWebSocketPort   = 11111
	defaultMaxWebSocketClients = 20
	defaultDevelopmentAPIBind  = "127.0.0.1:3000"
)

type fileData struct {
	Interfaces  *jfsettings.InterfaceSettings     `json:"interfaces,omitempty"`
	Integration *jfsettings.BrokerIntegration     `json:"integration,omitempty"`
	Accounts    []jfsettings.ManagedBrokerAccount `json:"accounts,omitempty"`
	Appearance  *jfsettings.UIAppearanceSettings  `json:"appearance,omitempty"`
	Onboarding  *jfsettings.OnboardingSettings    `json:"onboarding,omitempty"`
	Execution   *jfsettings.ExecutionSettings     `json:"execution,omitempty"`
	Security    *jfsettings.SecuritySettings      `json:"security,omitempty"`
	ADK         *jfsettings.ADKRuntimeSettings    `json:"adk,omitempty"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	data fileData
}

func New(path string) (*Store, error) {
	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) EnsureBootstrapFile(defaults jfsettings.LaunchDefaults) error {
	if _, err := os.Stat(s.path); err == nil {
		if s.HasAppearance() {
			return nil
		}
		_, err := s.SaveAppearance(DefaultUIAppearanceSettings())
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.mu.Lock()
	s.data.Interfaces = interfaceSettingsPointer(NormalizeInterfaceSettings(InterfaceSettingsFromDefaults(defaults), defaults))
	s.data.Appearance = uiAppearanceSettingsPointer(DefaultUIAppearanceSettings())
	err := s.persistLocked()
	s.mu.Unlock()
	return err
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = fileData{}
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = fileData{}
		return nil
	}
	return json.Unmarshal(data, &s.data)
}

func (s *Store) Integration() jfsettings.BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration != nil {
		return *s.data.Integration
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return jfsettings.BrokerIntegration{
		BrokerID:  "futu",
		Enabled:   false,
		Config:    DefaultFutuConfig(),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

func (s *Store) SavedIntegration() *jfsettings.BrokerIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Integration == nil {
		return nil
	}
	return new(*s.data.Integration)
}

func (s *Store) InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Interfaces != nil {
		return NormalizeInterfaceSettings(*s.data.Interfaces, defaults)
	}
	return NormalizeInterfaceSettings(InterfaceSettingsFromDefaults(defaults), defaults)
}

func (s *Store) Appearance() jfsettings.UIAppearanceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Appearance != nil {
		return NormalizeUIAppearanceSettings(*s.data.Appearance)
	}
	return DefaultUIAppearanceSettings()
}

func (s *Store) Onboarding() jfsettings.OnboardingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Onboarding != nil {
		return NormalizeOnboardingSettings(*s.data.Onboarding)
	}
	return DefaultOnboardingSettings()
}

func (s *Store) ExecutionSettings() jfsettings.ExecutionSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Execution != nil {
		return NormalizeExecutionSettings(*s.data.Execution)
	}
	return DefaultExecutionSettings()
}

func (s *Store) SecuritySettings() jfsettings.SecuritySettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Security != nil {
		return NormalizeSecuritySettings(*s.data.Security)
	}
	return DefaultSecuritySettings()
}

func (s *Store) ADKSettings() jfsettings.ADKRuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.ADK != nil {
		return NormalizeADKRuntimeSettings(*s.data.ADK)
	}
	return DefaultADKRuntimeSettings()
}

func (s *Store) HasAppearance() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Appearance != nil
}

func (s *Store) SaveAppearance(input jfsettings.UIAppearanceSettings) (jfsettings.UIAppearanceSettings, error) {
	normalized := NormalizeUIAppearanceSettings(input)

	s.mu.Lock()
	s.data.Appearance = uiAppearanceSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	normalized := NormalizeOnboardingSettings(input)

	s.mu.Lock()
	s.data.Onboarding = onboardingSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	normalized := NormalizeExecutionSettings(input)

	s.mu.Lock()
	s.data.Execution = executionSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	normalized := NormalizeSecuritySettings(input)

	s.mu.Lock()
	s.data.Security = securitySettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	normalized := NormalizeADKRuntimeSettings(input)

	s.mu.Lock()
	s.data.ADK = adkRuntimeSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SaveIntegration(input jfsettings.BrokerIntegration) (jfsettings.BrokerIntegration, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.BrokerID = "futu"
	input.Config = NormalizeFutuConfig(input.Config)
	input.UpdatedAt = now
	if input.CreatedAt == "" {
		if existing := s.SavedIntegration(); existing != nil {
			input.CreatedAt = existing.CreatedAt
		}
		if input.CreatedAt == "" {
			input.CreatedAt = now
		}
	}

	s.mu.Lock()
	if s.data.Interfaces == nil {
		s.data.Interfaces = interfaceSettingsPointer(NormalizeInterfaceSettings(InterfaceSettingsFromDefaults(jfsettings.LaunchDefaults{}), jfsettings.LaunchDefaults{}))
	}
	s.data.Integration = &input
	err := s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return input, err
	}

	return input, nil
}

func (s *Store) ManagedAccounts() []jfsettings.ManagedBrokerAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]jfsettings.ManagedBrokerAccount, len(s.data.Accounts))
	copy(accounts, s.data.Accounts)
	return accounts
}

func (s *Store) CreateManagedAccount(input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	input = NormalizeManagedBrokerAccount(input)
	if input.AccountID == "" {
		return jfsettings.ManagedBrokerAccount{}, errors.New("accountId is required")
	}
	input.ID = ""
	input.CreatedAt = ""
	input.UpdatedAt = ""
	now := time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Accounts {
		account := &s.data.Accounts[index]
		if SameManagedAccountScope(*account, input) {
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
		input.ID = BuildManagedAccountID(input)
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

func (s *Store) UpdateManagedAccount(id string, input jfsettings.ManagedBrokerAccount) (jfsettings.ManagedBrokerAccount, error) {
	input = NormalizeManagedBrokerAccount(input)
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

	return jfsettings.ManagedBrokerAccount{}, os.ErrNotExist
}

func (s *Store) DeleteManagedAccount(id string) error {
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

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func InterfaceSettingsFromDefaults(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	settings := jfsettings.InterfaceSettings{APIBind: defaults.APIBind}
	if strings.TrimSpace(defaults.GUIBind) != "" {
		settings.GUIBind = defaults.GUIBind
		settings.GUIAPIBaseURL = apiBaseURLForBind(defaults.APIBind)
	}
	return settings
}

func NormalizeInterfaceSettings(input jfsettings.InterfaceSettings, defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	settings := input
	settings.APIBind = strings.TrimSpace(settings.APIBind)
	settings.GUIBind = strings.TrimSpace(settings.GUIBind)
	settings.GUIAPIBaseURL = strings.TrimRight(strings.TrimSpace(settings.GUIAPIBaseURL), "/")

	if settings.APIBind == "" {
		settings.APIBind = defaults.APIBind
	}
	if settings.APIBind == "" {
		settings.APIBind = defaultDevelopmentAPIBind
	}
	if settings.GUIBind == "" {
		settings.GUIBind = defaults.GUIBind
	}
	if settings.GUIAPIBaseURL == "" && settings.GUIBind != "" {
		settings.GUIAPIBaseURL = apiBaseURLForBind(settings.APIBind)
	}
	return settings
}

func apiBaseURLForBind(bind string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return ""
	}
	host = normalizeBrowserHost(host)
	if host == "" || port == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

func normalizeBrowserHost(host string) string {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(host)
	}
}

func interfaceSettingsPointer(value jfsettings.InterfaceSettings) *jfsettings.InterfaceSettings {
	return new(value)
}

func DefaultUIAppearanceSettings() jfsettings.UIAppearanceSettings {
	return jfsettings.UIAppearanceSettings{
		UpColor:   "#16c784",
		DownColor: "#ea3943",
	}
}

func NormalizeUIAppearanceSettings(input jfsettings.UIAppearanceSettings) jfsettings.UIAppearanceSettings {
	defaults := DefaultUIAppearanceSettings()
	return jfsettings.UIAppearanceSettings{
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

func uiAppearanceSettingsPointer(value jfsettings.UIAppearanceSettings) *jfsettings.UIAppearanceSettings {
	return new(value)
}

func DefaultOnboardingSettings() jfsettings.OnboardingSettings {
	return jfsettings.OnboardingSettings{
		Completed:    false,
		LastBrokerID: "",
	}
}

func NormalizeOnboardingSettings(input jfsettings.OnboardingSettings) jfsettings.OnboardingSettings {
	settings := input
	settings.LastBrokerID = strings.TrimSpace(settings.LastBrokerID)
	if !settings.Completed {
		settings.CompletedAt = ""
	}
	return settings
}

func onboardingSettingsPointer(value jfsettings.OnboardingSettings) *jfsettings.OnboardingSettings {
	return new(value)
}

func DefaultExecutionSettings() jfsettings.ExecutionSettings {
	return jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      "SIMULATE",
		BrokerOrderHistoryLookbackDays: 30,
		SeenFillRetentionDays:          90,
	}
}

func NormalizeExecutionSettings(input jfsettings.ExecutionSettings) jfsettings.ExecutionSettings {
	defaults := DefaultExecutionSettings()
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

func executionSettingsPointer(value jfsettings.ExecutionSettings) *jfsettings.ExecutionSettings {
	return new(value)
}

func DefaultSecuritySettings() jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{
		AdminAuthRequired: false,
	}
}

func NormalizeSecuritySettings(input jfsettings.SecuritySettings) jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{
		AdminAuthRequired: input.AdminAuthRequired,
	}
}

func securitySettingsPointer(value jfsettings.SecuritySettings) *jfsettings.SecuritySettings {
	return new(value)
}

func DefaultADKRuntimeSettings() jfsettings.ADKRuntimeSettings {
	return jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        600_000,
		StreamIdleTimeoutMs: 300_000,
	}
}

func NormalizeADKRuntimeSettings(input jfsettings.ADKRuntimeSettings) jfsettings.ADKRuntimeSettings {
	defaults := DefaultADKRuntimeSettings()
	return jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        clampOrDefaultInt(input.RunTimeoutMs, defaults.RunTimeoutMs, 60_000, 1_800_000),
		StreamIdleTimeoutMs: clampOrDefaultInt(input.StreamIdleTimeoutMs, defaults.StreamIdleTimeoutMs, 30_000, 900_000),
	}
}

func adkRuntimeSettingsPointer(value jfsettings.ADKRuntimeSettings) *jfsettings.ADKRuntimeSettings {
	return new(value)
}

func clampOrDefaultInt(value int, fallback int, min int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func NormalizeManagedBrokerAccount(input jfsettings.ManagedBrokerAccount) jfsettings.ManagedBrokerAccount {
	input.BrokerID = strings.TrimSpace(strings.ToLower(input.BrokerID))
	if input.BrokerID == "" {
		input.BrokerID = "futu"
	}
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = input.AccountID
	}
	input.TradingEnvironment = strings.ToUpper(strings.TrimSpace(input.TradingEnvironment))
	if input.TradingEnvironment == "" {
		input.TradingEnvironment = "SIMULATE"
	}
	input.Market = strings.ToUpper(strings.TrimSpace(input.Market))
	if input.Market == "" {
		input.Market = "HK"
	}
	if input.SecurityFirm != nil {
		value := strings.TrimSpace(*input.SecurityFirm)
		if value == "" {
			input.SecurityFirm = nil
		} else {
			input.SecurityFirm = &value
		}
	}
	return input
}

func SameManagedAccountScope(left jfsettings.ManagedBrokerAccount, right jfsettings.ManagedBrokerAccount) bool {
	return left.BrokerID == right.BrokerID &&
		left.AccountID == right.AccountID &&
		left.TradingEnvironment == right.TradingEnvironment &&
		left.Market == right.Market
}

func BuildManagedAccountID(input jfsettings.ManagedBrokerAccount) string {
	return strings.Join([]string{input.BrokerID, input.TradingEnvironment, input.AccountID, input.Market}, "|")
}

func DefaultFutuConfig() jfsettings.FutuIntegrationConfig {
	return NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type:                    "futu",
		Host:                    defaultFutuHost,
		APIPort:                 defaultFutuAPIPort,
		WebSocketPort:           defaultFutuWebSocketPort,
		MaxWebSocketConnections: defaultMaxWebSocketClients,
		UseEncryption:           false,
		TradeMarket:             "HK",
		SecurityFirm:            "FUTUSECURITIES",
	})
}

func NormalizeFutuConfig(config jfsettings.FutuIntegrationConfig) jfsettings.FutuIntegrationConfig {
	if config.Type == "" {
		config.Type = "futu"
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = defaultFutuHost
	}
	if config.APIPort <= 0 {
		config.APIPort = defaultFutuAPIPort
	}
	if config.WebSocketPort <= 0 {
		config.WebSocketPort = defaultFutuWebSocketPort
	}
	if config.MaxWebSocketConnections <= 0 {
		config.MaxWebSocketConnections = defaultMaxWebSocketClients
	}
	if strings.TrimSpace(config.TradeMarket) == "" {
		config.TradeMarket = "HK"
	}
	if strings.TrimSpace(config.SecurityFirm) == "" {
		config.SecurityFirm = "FUTUSECURITIES"
	}
	config.UseEncryption = false
	return config
}
