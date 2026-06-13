package jftradeapi

import (
	"fmt"
	"strings"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func buildStrategyInstanceParams(definition strategyDesignDefinition, compiledAt string) (map[string]any, error) {
	sourceFormat := strategydefinition.NormalizeSourceFormat(definition.SourceFormat)
	if sourceFormat != strategydefinition.SourceFormatPineV6 {
		return nil, fmt.Errorf("unsupported strategy source format: %s", sourceFormat)
	}
	symbol := strings.ToUpper(strings.TrimSpace(definition.Symbol))
	interval := strings.TrimSpace(definition.Interval)
	if interval == "" {
		interval = "5m"
	}
	params := map[string]any{
		"definitionId": definition.ID,
		"sourceFormat": sourceFormat,
		"symbol":       symbol,
		"interval":     interval,
		"script":       definition.Script,
	}
	compilation, err := strategypine.Compile(definition.Script)
	if err != nil {
		return nil, err
	}
	program := compilation.Program
	params["runtime"] = strategyRuntimePinePlan
	params["compiledAt"] = compiledAt
	params["compiledHooks"] = buildCompiledHookKinds(program)
	params["compiledRequirements"] = buildCompiledRequirementsPayload(compilation.Requirements)

	return params, nil
}

func buildCompiledHookKinds(program *strategyir.Program) []string {
	if program == nil {
		return []string{}
	}
	result := make([]string, 0, len(program.Hooks))
	for _, hook := range program.Hooks {
		result = append(result, string(hook.Kind))
	}
	return result
}

func buildCompiledRequirementsPayload(requirements strategyir.Requirements) map[string]any {
	indicators := make([]map[string]any, 0, len(requirements.Indicators))
	for _, indicator := range requirements.Indicators {
		indicators = append(indicators, map[string]any{
			"alias": indicator.Alias,
			"kind":  indicator.Kind,
			"key":   indicator.Key,
		})
	}
	return map[string]any{
		"indicators":                indicators,
		"requiresPosition":          requirements.RequiresPosition,
		"requiresTotalAccountValue": requirements.RequiresTotalAccountValue,
	}
}

func strategyDefinitionIDFromParams(params map[string]any) string {
	definitionID, _ := params["definitionId"].(string)
	return strings.TrimSpace(definitionID)
}

func strategyInstanceUsesDefinition(strategy managedStrategyInstance, definitionID string) bool {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		return false
	}
	if strategyDefinitionIDFromParams(strategy.Params) == definitionID {
		return true
	}
	return strings.TrimSpace(strategy.Definition.StrategyID) == definitionID
}
