package servercore

import (
	"time"

	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
)

func strategyPluginIDForDefinition(definition strategyDesignDefinition) string {
	return instanceview.PluginIDForDefinition(definition)
}

func strategyRuntimeFromParams(params map[string]any) string {
	return instanceview.RuntimeFromParams(params)
}

func strategySourceFormatFromParams(params map[string]any) string {
	return instanceview.SourceFormatFromParams(params)
}

func strategyInstanceStartable(instance managedStrategyInstance) bool {
	return instanceview.Startable(instance)
}

func strategyToListItem(strategy managedStrategyInstance) strategyListItem {
	return instanceview.ToInstanceView(strategy)
}

func normalizeManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	return instanceview.NormalizeManagedInstance(input)
}

func buildStrategyInstanceID(definitionID string) string {
	return instanceview.BuildInstanceID(definitionID, time.Now().UTC())
}

func IDPinePlanPlugin() string {
	return instanceview.DefaultPluginID
}
