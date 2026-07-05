package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func (s *Store) Onboarding() jfsettings.OnboardingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Onboarding != nil {
		return NormalizeOnboardingSettings(*s.data.Onboarding)
	}
	return DefaultOnboardingSettings()
}

func (s *Store) SaveOnboarding(input jfsettings.OnboardingSettings) (jfsettings.OnboardingSettings, error) {
	normalized := NormalizeOnboardingSettings(input)

	s.mu.Lock()
	s.data.Onboarding = onboardingSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
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
