package servercore

import (
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
