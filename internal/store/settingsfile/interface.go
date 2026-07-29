package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

const defaultDevelopmentAPIBind = "127.0.0.1:3000"

func (s *Store) InterfaceSettings(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Interfaces != nil {
		return NormalizeInterfaceSettings(*s.data.Interfaces, defaults)
	}
	return NormalizeInterfaceSettings(InterfaceSettingsFromDefaults(defaults), defaults)
}

func InterfaceSettingsFromDefaults(defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	settings := jfsettings.InterfaceSettings{
		APIBind:                      defaults.APIBind,
		LiveWebSocketConnectionLimit: jfsettings.DefaultLiveWebSocketConnectionLimit,
	}
	if strings.TrimSpace(defaults.GUIBind) != "" {
		settings.GUIBind = defaults.GUIBind
	}
	return settings
}

func NormalizeInterfaceSettings(input jfsettings.InterfaceSettings, defaults jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	settings := input
	settings.APIBind = strings.TrimSpace(settings.APIBind)
	settings.GUIBind = strings.TrimSpace(settings.GUIBind)

	if settings.APIBind == "" {
		settings.APIBind = defaults.APIBind
	}
	if settings.APIBind == "" {
		settings.APIBind = defaultDevelopmentAPIBind
	}
	if settings.GUIBind == "" {
		settings.GUIBind = defaults.GUIBind
	}
	if settings.LiveWebSocketConnectionLimit <= 0 {
		settings.LiveWebSocketConnectionLimit = jfsettings.DefaultLiveWebSocketConnectionLimit
	}
	return settings
}

func interfaceSettingsPointer(value jfsettings.InterfaceSettings) *jfsettings.InterfaceSettings {
	return new(value)
}
