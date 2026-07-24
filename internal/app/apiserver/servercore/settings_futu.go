package servercore

import (
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func defaultFutuConfig() jfsettings.FutuIntegrationConfig {
	return settingsfile.DefaultFutuConfig()
}

func normalizeFutuConfig(config jfsettings.FutuIntegrationConfig) jfsettings.FutuIntegrationConfig {
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
