package ir

import (
	"fmt"
	"sort"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
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
		switch typed := statement.(type) {
		case *LetStmt:
			if expressionRequiresPosition(typed.Expression) {
				result.RequiresPosition = true
			}
			if expressionRequiresTotalAccountValue(typed.Expression) {
				result.RequiresTotalAccountValue = true
			}
			binding, recognized, err := parseIndicatorBinding(typed)
			if err != nil {
				return err
			}
			if recognized {
				bindings[typed.Name] = binding
				indicatorByKey[binding.Key] = IndicatorRequirement{
					Alias: binding.Alias,
					Kind:  binding.Kind,
					Key:   binding.Key,
				}
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.Expression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
		case *CollectionStmt:
			for _, expression := range typed.Arguments {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
		case *TupleStmt:
			for _, expression := range typed.Expressions {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
		case *LoopStmt:
			for _, expression := range []string{typed.StartExpression, typed.EndExpression, typed.StepExpression, typed.WhileCondition} {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
			if err := planStatements(typed.Body, bindings, indicatorByKey, result); err != nil {
				return err
			}
		case *BreakStmt, *ContinueStmt:
			continue
		case *ObjectStmt:
			for _, expression := range typed.Arguments {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
		case *IfStmt:
			if expressionRequiresPosition(typed.Condition) {
				result.RequiresPosition = true
			}
			if expressionRequiresTotalAccountValue(typed.Condition) {
				result.RequiresTotalAccountValue = true
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.Condition)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			for _, requirement := range collectConditionRequirements(typed.Condition, bindings) {
				indicatorByKey[requirement.Key] = requirement
			}

			thenBindings := cloneBindings(bindings)
			if err := planStatements(typed.Then, thenBindings, indicatorByKey, result); err != nil {
				return err
			}

			elseBindings := cloneBindings(bindings)
			if err := planStatements(typed.Else, elseBindings, indicatorByKey, result); err != nil {
				return err
			}
		case *OrderStmt:
			quantityMode, ok := indicatorbinding.ParseQuantityMode(typed.QuantityMode)
			if !ok {
				return fmt.Errorf("pine line %d: unsupported order quantity mode %q", typed.Range.StartLine, typed.QuantityMode)
			}
			result.RequiresPosition = true
			switch quantityMode {
			case "account_position_percent":
				result.RequiresTotalAccountValue = true
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.QuantityExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			requirements, err = collectExpressionRequirements(typed.Range.StartLine, typed.LimitExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			requirements, err = collectExpressionRequirements(typed.Range.StartLine, typed.StopExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
		case *ExitStmt:
			if _, ok := indicatorbinding.ParseQuantityMode(typed.QuantityMode); !ok {
				return fmt.Errorf("pine line %d: unsupported exit quantity mode %q", typed.Range.StartLine, typed.QuantityMode)
			}
			result.RequiresPosition = true
			for _, expression := range []string{typed.QuantityExpression, typed.StopExpression, typed.LimitExpression, typed.TrailPrice, typed.TrailPoints, typed.TrailOffset} {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
		case *CancelStmt:
			continue
		case *ProtectStmt:
			result.RequiresPosition = true
			key, err := buildProtectRequirementKey(typed)
			if err != nil {
				return err
			}
			indicatorByKey[key] = IndicatorRequirement{Kind: "protect", Key: key}
		case *LogStmt, *NotifyStmt:
			continue
		default:
			return fmt.Errorf("unsupported IR statement type %T", statement)
		}
	}

	return nil
}

func cloneBindings(input map[string]plannedBinding) map[string]plannedBinding {
	cloned := make(map[string]plannedBinding, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
