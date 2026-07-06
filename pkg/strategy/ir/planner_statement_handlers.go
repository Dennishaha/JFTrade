package ir

import (
	"fmt"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

func planStatement(
	statement Statement,
	bindings map[string]plannedBinding,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	switch typed := statement.(type) {
	case *LetStmt:
		return planLetStatement(typed, bindings, indicatorByKey, result)
	case *CollectionStmt:
		return recordExpressions(typed.Range.StartLine, typed.Arguments, indicatorByKey, result)
	case *TupleStmt:
		return recordExpressions(typed.Range.StartLine, typed.Expressions, indicatorByKey, result)
	case *LoopStmt:
		if err := recordExpressions(typed.Range.StartLine, []string{
			typed.StartExpression,
			typed.EndExpression,
			typed.StepExpression,
			typed.WhileCondition,
		}, indicatorByKey, result); err != nil {
			return err
		}
		return planStatements(typed.Body, bindings, indicatorByKey, result)
	case *BreakStmt, *ContinueStmt, *CancelStmt, *LogStmt, *NotifyStmt:
		return nil
	case *ObjectStmt:
		return recordExpressions(typed.Range.StartLine, typed.Arguments, indicatorByKey, result)
	case *IfStmt:
		return planIfStatement(typed, bindings, indicatorByKey, result)
	case *OrderStmt:
		return planOrderStatement(typed, indicatorByKey, result)
	case *ExitStmt:
		return planExitStatement(typed, indicatorByKey, result)
	case *ProtectStmt:
		return planProtectStatement(typed, indicatorByKey, result)
	default:
		return fmt.Errorf("unsupported IR statement type %T", statement)
	}
}

func planLetStatement(
	statement *LetStmt,
	bindings map[string]plannedBinding,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	binding, recognized, err := parseIndicatorBinding(statement)
	if err != nil {
		return err
	}
	if recognized {
		bindings[statement.Name] = binding
		indicatorByKey[binding.Key] = IndicatorRequirement{
			Alias: binding.Alias,
			Kind:  binding.Kind,
			Key:   binding.Key,
		}
	}
	return recordExpressionRequirements(statement.Range.StartLine, statement.Expression, indicatorByKey, result)
}

func planIfStatement(
	statement *IfStmt,
	bindings map[string]plannedBinding,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	if err := recordExpressionRequirements(statement.Range.StartLine, statement.Condition, indicatorByKey, result); err != nil {
		return err
	}
	addIndicatorRequirements(indicatorByKey, collectConditionRequirements(statement.Condition, bindings))

	thenBindings := cloneBindings(bindings)
	if err := planStatements(statement.Then, thenBindings, indicatorByKey, result); err != nil {
		return err
	}

	elseBindings := cloneBindings(bindings)
	return planStatements(statement.Else, elseBindings, indicatorByKey, result)
}

func planOrderStatement(statement *OrderStmt, indicatorByKey map[string]IndicatorRequirement, result *Requirements) error {
	quantityMode, ok := indicatorbinding.ParseQuantityMode(statement.QuantityMode)
	if !ok {
		return fmt.Errorf("pine line %d: unsupported order quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}

	result.RequiresPosition = true
	if quantityMode == "account_position_percent" {
		result.RequiresTotalAccountValue = true
	}

	return recordExpressions(statement.Range.StartLine, []string{
		statement.QuantityExpression,
		statement.LimitExpression,
		statement.StopExpression,
	}, indicatorByKey, result)
}

func planExitStatement(statement *ExitStmt, indicatorByKey map[string]IndicatorRequirement, result *Requirements) error {
	if _, ok := indicatorbinding.ParseQuantityMode(statement.QuantityMode); !ok {
		return fmt.Errorf("pine line %d: unsupported exit quantity mode %q", statement.Range.StartLine, statement.QuantityMode)
	}

	result.RequiresPosition = true
	return recordExpressions(statement.Range.StartLine, []string{
		statement.QuantityExpression,
		statement.StopExpression,
		statement.LimitExpression,
		statement.TrailPrice,
		statement.TrailPoints,
		statement.TrailOffset,
	}, indicatorByKey, result)
}

func planProtectStatement(statement *ProtectStmt, indicatorByKey map[string]IndicatorRequirement, result *Requirements) error {
	result.RequiresPosition = true
	key, err := buildProtectRequirementKey(statement)
	if err != nil {
		return err
	}
	indicatorByKey[key] = IndicatorRequirement{Kind: "protect", Key: key}
	return nil
}

func recordExpressions(
	lineNumber int,
	expressions []string,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	for _, expression := range expressions {
		if err := recordExpressionRequirements(lineNumber, expression, indicatorByKey, result); err != nil {
			return err
		}
	}
	return nil
}

func recordExpressionRequirements(
	lineNumber int,
	expression string,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	if expressionRequiresPosition(expression) {
		result.RequiresPosition = true
	}
	if expressionRequiresTotalAccountValue(expression) {
		result.RequiresTotalAccountValue = true
	}

	requirements, err := collectExpressionRequirements(lineNumber, expression)
	if err != nil {
		return err
	}
	addIndicatorRequirements(indicatorByKey, requirements)
	return nil
}

func addIndicatorRequirements(indicatorByKey map[string]IndicatorRequirement, requirements []IndicatorRequirement) {
	for _, requirement := range requirements {
		indicatorByKey[requirement.Key] = requirement
	}
}
