package jftradeapi

import (
	"strings"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func strategyPluginIDForDefinition(definition strategyDesignDefinition) string {
	_ = definition
	return IDPinePlanPlugin()
}

func strategyRuntimeFromParams(params map[string]any) string {
	if runtime, ok := params["runtime"].(string); ok {
		normalized := strings.TrimSpace(strings.ToLower(runtime))
		if normalized != "" {
			return normalized
		}
	}
	return strategyRuntimePinePlan
}

func strategySourceFormatFromParams(params map[string]any) string {
	if sourceFormat, ok := params["sourceFormat"].(string); ok {
		return strategydefinition.NormalizeSourceFormat(sourceFormat)
	}
	return strategydefinition.SourceFormatPineV6
}

func strategyInstanceStartable(instance managedStrategyInstance) bool {
	sourceFormat := strategySourceFormatFromParams(instance.Params)
	runtime := strategyRuntimeFromParams(instance.Params)
	return sourceFormat == strategydefinition.SourceFormatPineV6 && runtime == strategyRuntimePinePlan
}

func strategyToListItem(strategy managedStrategyInstance) strategyListItem {
	strategy = normalizeManagedStrategyInstance(strategy)
	return strategyListItem{
		ID:           strategy.ID,
		PluginID:     strategy.PluginID,
		Definition:   strategy.Definition,
		Runtime:      strategyRuntimeFromParams(strategy.Params),
		SourceFormat: strategySourceFormatFromParams(strategy.Params),
		Startable:    strategyInstanceStartable(strategy),
		Binding:      strategy.Binding,
		Params:       copyMap(strategy.Params),
		Status:       strategy.Status,
		CreatedAt:    strategy.CreatedAt,
		Logs:         []string{},
	}
}

func normalizeManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	applyStrategyBindingParams(&input)
	return input
}

func buildStrategyInstanceID(definitionID string) string {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		definitionID = IDPinePlanPlugin()
	}
	return definitionID + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func IDPinePlanPlugin() string {
	return "pine-go-plan"
}
