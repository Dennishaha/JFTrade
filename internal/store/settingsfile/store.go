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
	Security            *storedSecuritySettings                `json:"security,omitempty"`
	SystemNotifications *jfsettings.SystemNotificationSettings `json:"systemNotifications,omitempty"`
	ADK                 *jfsettings.ADKRuntimeSettings         `json:"adk,omitempty"`
	PineWorker          *jfsettings.PineWorkerSettings         `json:"pineWorker,omitempty"`
	Calendars           *jfsettings.ExchangeCalendarSettings   `json:"exchangeCalendars,omitempty"`
}

// storedSecuritySettings deliberately differs from the public API model so a
// password hash can be persisted without ever being returned by an endpoint.
// AdminAuthRequired is accepted only to migrate old settings files to the new
// safe default (Web access disabled).
type storedSecuritySettings struct {
	WebAccessEnabled    bool   `json:"webAccessEnabled"`
	PublicAccessEnabled bool   `json:"publicAccessEnabled"`
	WebPort             int    `json:"webPort,omitempty"`
	PasswordHash        string `json:"passwordHash,omitempty"`
	AdminAuthRequired   *bool  `json:"adminAuthRequired,omitempty"`
}

// Pine worker defaults remain documented here for the PineTS hard-cut audit:
// DefaultPineWorkerSettings, NormalizePineWorkerSettings,
// BacktestWorkerLimit: 2, InstanceWorkerLimit: 10, NodeBinaryPath, max 1000.
type Store struct {
	path        string
	mu          sync.RWMutex
	data        fileData
	replaceFile func(source string, destination string) error
}

func New(path string) (*Store, error) {
	store := &Store{path: path, replaceFile: replaceFile}
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
	if directory := filepath.Dir(s.path); directory != "." {
		if err := os.Chmod(directory, 0o700); err != nil {
			return err
		}
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		s.data = fileData{}
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}
	if s.data.Security != nil && s.data.Security.AdminAuthRequired != nil {
		s.data.Security.AdminAuthRequired = nil
		return s.persistLocked()
	}
	return nil
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if directory != "." {
		if err := os.Chmod(directory, 0o700); err != nil {
			return err
		}
	}
	temporary, err := os.CreateTemp(directory, ".settings-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	replace := s.replaceFile
	if replace == nil {
		replace = replaceFile
	}
	if err := replace(temporaryPath, s.path); err != nil {
		return err
	}
	return syncSettingsDirectory(directory)
}
