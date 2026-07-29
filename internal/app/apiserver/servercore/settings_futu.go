package servercore

import (
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func normalizeFutuConfig(config jfsettings.FutuIntegrationConfig) jfsettings.FutuIntegrationConfig {
	return settingsfile.NormalizeFutuConfig(config)
}
