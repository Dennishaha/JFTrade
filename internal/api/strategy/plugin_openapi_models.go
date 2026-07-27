package strategy

import srv "github.com/jftrade/jftrade-main/internal/strategy"

// PluginMutationData documents the install/uninstall response payload.
type PluginMutationData struct {
	Operation srv.PluginOperation `json:"operation"`
}
