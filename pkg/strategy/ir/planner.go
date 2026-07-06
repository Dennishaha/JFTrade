package ir

import (
	"fmt"
	"maps"
	"sort"
)

type Requirements struct {
	Indicators                []IndicatorRequirement
	RequiresPosition          bool
	RequiresTotalAccountValue bool
}

type IndicatorRequirement struct {
	Alias string
	Kind  string
	Key   string
}

type plannedBinding struct {
	Alias string
	Kind  string
	Key   string
	Args  []string
}

func PlanRequirements(program *Program) (Requirements, error) {
	if program == nil {
		return Requirements{}, fmt.Errorf("strategy program is required")
	}

	result := Requirements{}
	indicatorByKey := map[string]IndicatorRequirement{}

	for _, hook := range program.Hooks {
		bindings := map[string]plannedBinding{}
		if err := planStatements(hook.Statements, bindings, indicatorByKey, &result); err != nil {
			return Requirements{}, err
		}
	}

	keys := make([]string, 0, len(indicatorByKey))
	for key := range indicatorByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result.Indicators = make([]IndicatorRequirement, 0, len(keys))
	for _, key := range keys {
		result.Indicators = append(result.Indicators, indicatorByKey[key])
	}

	return result, nil
}

func planStatements(
	statements []Statement,
	bindings map[string]plannedBinding,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	for _, statement := range statements {
		if err := planStatement(statement, bindings, indicatorByKey, result); err != nil {
			return err
		}
	}

	return nil
}

func cloneBindings(input map[string]plannedBinding) map[string]plannedBinding {
	cloned := make(map[string]plannedBinding, len(input))
	maps.Copy(cloned, input)
	return cloned
}
