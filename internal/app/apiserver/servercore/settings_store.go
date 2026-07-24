package servercore

import (
	"sync"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type settingsFile struct {
	Interfaces  *jftsettings.InterfaceSettings        `json:"interfaces,omitempty"`
	Integration *jftsettings.BrokerIntegration        `json:"integration,omitempty"`
	Accounts    []jftsettings.ManagedBrokerAccount    `json:"accounts,omitempty"`
	Appearance  *jftsettings.UIAppearanceSettings     `json:"appearance,omitempty"`
	Onboarding  *jftsettings.OnboardingSettings       `json:"onboarding,omitempty"`
	Execution   *jftsettings.ExecutionSettings        `json:"execution,omitempty"`
	Security    *jftsettings.SecuritySettings         `json:"security,omitempty"`
	ADK         *jftsettings.ADKRuntimeSettings       `json:"adk,omitempty"`
	PineWorker  *jftsettings.PineWorkerSettings       `json:"pineWorker,omitempty"`
	Calendars   *jftsettings.ExchangeCalendarSettings `json:"exchangeCalendars,omitempty"`
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

func (s *SettingsStore) Integration() jftsettings.BrokerIntegration {
	s.mu.RLock()
	if s.data.Integration != nil {
		integration := *s.data.Integration
		s.mu.RUnlock()
		return integration
	}
	s.mu.RUnlock()
	return s.Store.Integration()
}

func (s *SettingsStore) SavedIntegration() *jftsettings.BrokerIntegration {
	s.mu.RLock()
	if s.data.Integration != nil {
		s.mu.RUnlock()
		return new(*s.data.Integration)
	}
	s.mu.RUnlock()
	return s.Store.SavedIntegration()
}

func (s *SettingsStore) SaveIntegration(input jftsettings.BrokerIntegration) (jftsettings.BrokerIntegration, error) {
	integration, err := s.Store.SaveIntegration(input)
	if err != nil {
		return integration, err
	}
	s.mu.Lock()
	s.data.Integration = &integration
	s.mu.Unlock()
	return integration, nil
}

func normalizeExecutionSettings(input jftsettings.ExecutionSettings) jftsettings.ExecutionSettings {
	return settingsfile.NormalizeExecutionSettings(input)
}

func normalizeSecuritySettings(input jftsettings.SecuritySettings) jftsettings.SecuritySettings {
	return settingsfile.NormalizeSecuritySettings(input)
}
