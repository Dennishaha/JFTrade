package settingsfile

import (
	"net"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
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
