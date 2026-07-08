package settingsfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type fileData struct {
	Interfaces          *jfsettings.InterfaceSettings          `json:"interfaces,omitempty"`
	Integration         *jfsettings.BrokerIntegration          `json:"integration,omitempty"`
	Accounts            []jfsettings.ManagedBrokerAccount      `json:"accounts,omitempty"`
	Appearance          *jfsettings.UIAppearanceSettings       `json:"appearance,omitempty"`
	Onboarding          *jfsettings.OnboardingSettings         `json:"onboarding,omitempty"`
	Execution           *jfsettings.ExecutionSettings          `json:"execution,omitempty"`
	Security            *jfsettings.SecuritySettings           `json:"security,omitempty"`
	SystemNotifications *jfsettings.SystemNotificationSettings `json:"systemNotifications,omitempty"`
	ADK                 *jfsettings.ADKRuntimeSettings         `json:"adk,omitempty"`
	PineWorker          *jfsettings.PineWorkerSettings         `json:"pineWorker,omitempty"`
	Calendars           *jfsettings.ExchangeCalendarSettings   `json:"exchangeCalendars,omitempty"`
}

// Pine worker defaults remain documented here for the PineTS hard-cut audit:
// DefaultPineWorkerSettings, NormalizePineWorkerSettings,
// BacktestWorkerLimit: 2, InstanceWorkerLimit: 10, NodeBinaryPath, max 1000.
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
