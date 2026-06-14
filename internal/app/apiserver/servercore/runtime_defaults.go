package servercore

import (
	"strings"

	apruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
)

const (
	defaultDevelopmentAPIBind = apruntime.DefaultDevelopmentAPIBind
	defaultReleaseAPIBind     = apruntime.DefaultReleaseAPIBind
	defaultReleaseGUIBind     = apruntime.DefaultReleaseGUIBind
	defaultRuntimeDir         = apruntime.DefaultRuntimeDir
	defaultSettingsFilename   = apruntime.DefaultSettingsFilename
	defaultBacktestDBFilename = apruntime.DefaultBacktestDBFilename
)

func resolveLaunchDefaults(embeddedFrontend bool) LaunchDefaults {
	return apruntime.ResolveLaunchDefaults(embeddedFrontend)
}

// ResolveLaunchDefaults returns startup defaults for embedded or development mode.
func ResolveLaunchDefaults(embeddedFrontend bool) LaunchDefaults {
	return resolveLaunchDefaults(embeddedFrontend)
}

func LaunchDefaultsForExecutableDir(embeddedFrontend bool, executableDir string) LaunchDefaults {
	return apruntime.LaunchDefaultsForExecutableDir(embeddedFrontend, executableDir)
}

func apiBaseURLForBind(bind string) string {
	return apruntime.APIBaseURLForBind(bind)
}

// APIBaseURLForBind returns the browser-accessible API base URL for a bind address.
func APIBaseURLForBind(bind string) string {
	return apiBaseURLForBind(bind)
}

func normalizeBrowserHost(host string) string {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(host)
	}
}

func portFromBind(bind string, fallback int) int {
	return apruntime.PortFromBind(bind, fallback)
}

// PortFromBind extracts the numeric port from a bind address.
func PortFromBind(bind string, fallback int) int {
	return portFromBind(bind, fallback)
}
