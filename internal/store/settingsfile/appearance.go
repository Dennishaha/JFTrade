package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func (s *Store) Appearance() jfsettings.UIAppearanceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Appearance != nil {
		return NormalizeUIAppearanceSettings(*s.data.Appearance)
	}
	return DefaultUIAppearanceSettings()
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
