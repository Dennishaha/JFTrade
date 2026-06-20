package servercore

import (
	"sync"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// The following type aliases keep backward-compatibility while allowing
// internal/settings and internal/api/settings to import from the cycle-free
// jftsettings package.
type (
	FutuIntegrationConfig    = jftsettings.FutuIntegrationConfig
	BrokerIntegration        = jftsettings.BrokerIntegration
	ManagedBrokerAccount     = jftsettings.ManagedBrokerAccount
	InterfaceSettings        = jftsettings.InterfaceSettings
	UIAppearanceSettings     = jftsettings.UIAppearanceSettings
	OnboardingSettings       = jftsettings.OnboardingSettings
	ExecutionSettings        = jftsettings.ExecutionSettings
	SecuritySettings         = jftsettings.SecuritySettings
	ADKRuntimeSettings       = jftsettings.ADKRuntimeSettings
	ExchangeCalendarSettings = jftsettings.ExchangeCalendarSettings
	LaunchDefaults           = jftsettings.LaunchDefaults
)

type settingsFile struct {
	Interfaces  *InterfaceSettings        `json:"interfaces,omitempty"`
	Integration *BrokerIntegration        `json:"integration,omitempty"`
	Accounts    []ManagedBrokerAccount    `json:"accounts,omitempty"`
	Appearance  *UIAppearanceSettings     `json:"appearance,omitempty"`
	Onboarding  *OnboardingSettings       `json:"onboarding,omitempty"`
	Execution   *ExecutionSettings        `json:"execution,omitempty"`
	Security    *SecuritySettings         `json:"security,omitempty"`
	ADK         *ADKRuntimeSettings       `json:"adk,omitempty"`
	Calendars   *ExchangeCalendarSettings `json:"exchangeCalendars,omitempty"`
}

type SettingsStore struct {
	*settingsfile.Store
	path string
	mu   sync.RWMutex
	data settingsFile
}

func NewSettingsStore(path string) (*SettingsStore, error) {
	store, err := settingsfile.New(path)
	if err != nil {
		return nil, err
	}
	return &SettingsStore{Store: store, path: store.Path()}, nil
}

func (s *SettingsStore) Integration() BrokerIntegration {
	s.mu.RLock()
	if s.data.Integration != nil {
		integration := *s.data.Integration
		s.mu.RUnlock()
		return integration
	}
	s.mu.RUnlock()
	integration := s.Store.Integration()
	if s.Store.SavedIntegration() == nil {
		return apiruntime.IntegrationWithEnvDefaults(integration)
	}
	return integration
}

func (s *SettingsStore) SavedIntegration() *BrokerIntegration {
	s.mu.RLock()
	if s.data.Integration != nil {
		s.mu.RUnlock()
		return new(*s.data.Integration)
	}
	s.mu.RUnlock()
	return s.Store.SavedIntegration()
}

func (s *SettingsStore) SaveIntegration(input BrokerIntegration) (BrokerIntegration, error) {
	integration, err := s.Store.SaveIntegration(input)
	if err != nil {
		return integration, err
	}
	s.mu.Lock()
	s.data.Integration = &integration
	s.mu.Unlock()
	apiruntime.ApplyIntegrationEnv(integration)
	return integration, nil
}

func normalizeExecutionSettings(input ExecutionSettings) ExecutionSettings {
	return settingsfile.NormalizeExecutionSettings(input)
}

func normalizeSecuritySettings(input SecuritySettings) SecuritySettings {
	return settingsfile.NormalizeSecuritySettings(input)
}
