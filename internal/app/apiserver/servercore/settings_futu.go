package servercore

import (
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func normalizeFutuConfig(config jfsettings.FutuIntegrationConfig) jfsettings.FutuIntegrationConfig {
	return settingsfile.NormalizeFutuConfig(config)
}
