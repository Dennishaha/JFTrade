package servercore

import (
	"os"
	"strings"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func defaultFutuConfig() FutuIntegrationConfig {
	integration := apiruntime.IntegrationWithEnvDefaults(BrokerIntegration{
		Config: settingsfile.DefaultFutuConfig(),
	})
	return integration.Config
}

func normalizeFutuConfig(config FutuIntegrationConfig) FutuIntegrationConfig {
	return settingsfile.NormalizeFutuConfig(config)
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
