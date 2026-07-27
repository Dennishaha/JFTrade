package servercore

import strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"

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
