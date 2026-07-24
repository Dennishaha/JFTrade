package servercore

import (
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
)

func strategyPluginIDForDefinition(definition stratsrv.Definition) string {
	return instanceview.PluginIDForDefinition(definition)
}

func strategyRuntimeFromParams(params map[string]any) string {
	return instanceview.RuntimeFromParams(params)
}

func strategySourceFormatFromParams(params map[string]any) string {
	return instanceview.SourceFormatFromParams(params)
}

func strategyInstanceStartable(instance stratsrv.ManagedInstance) bool {
	return instanceview.Startable(instance)
}

func strategyToListItem(strategy stratsrv.ManagedInstance) stratsrv.InstanceView {
	return instanceview.ToInstanceView(strategy)
}

func normalizeManagedStrategyInstance(input stratsrv.ManagedInstance) stratsrv.ManagedInstance {
	return instanceview.NormalizeManagedInstance(input)
}

func buildStrategyInstanceID(definitionID string) string {
	return instanceview.BuildInstanceID(definitionID, time.Now().UTC())
}

func IDPinePlanPlugin() string {
	return instanceview.DefaultPluginID
}
