package dslruntime

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
)

type seriesNumber struct {
	Current     float64
	Previous    float64
	HasCurrent  bool
	HasPrevious bool
}

func evaluateFloatExpression(expression string, scope *evaluationScope) (float64, error) {
	value, err := evaluateExpression(expression, scope)
	if err != nil {
		return 0, err
	}
	result, ok := coerceFloatValue(value)
	if !ok {
		return 0, fmt.Errorf("expression %q is not numeric", expression)
	}
	return result, nil
}

func evaluateBoolExpression(expression string, scope *evaluationScope) (bool, error) {
	value, err := evaluateExpression(expression, scope)
	if err != nil {
		return false, err
	}
	result, ok := coerceBoolValue(value)
	if !ok {
		return false, fmt.Errorf("expression %q is not boolean", expression)
	}
	return result, nil
}

func evaluateExpression(expression string, scope *evaluationScope) (any, error) {
	parsed, err := parseExpression(expression, scope)
	if err != nil {
		return nil, fmt.Errorf("invalid expression %q: %w", expression, err)
	}
	return evaluateAST(parsed, scope)
}

func parseExpression(expression string, scope *evaluationScope) (exprast.Node, error) {
	trimmed := strings.TrimSpace(expression)
	if scope != nil && scope.runtime != nil && scope.runtime.expressionCache != nil {
		if cached, ok := scope.runtime.expressionCache[trimmed]; ok {
			return cached, nil
		}
		parsed, err := strategydsl.ParseExpression(trimmed)
		if err != nil {
			return nil, err
		}
		scope.runtime.expressionCache[trimmed] = parsed
		return parsed, nil
	}
	return strategydsl.ParseExpression(trimmed)
}

func evaluateAST(node exprast.Node, scope *evaluationScope) (any, error) {
	switch typed := node.(type) {
	case *exprast.IntegerNode:
		return float64(typed.Value), nil
	case *exprast.FloatNode:
		return typed.Value, nil
	case *exprast.StringNode:
		return typed.Value, nil
	case *exprast.BoolNode:
		return typed.Value, nil
	case *exprast.NilNode:
		return nil, nil
	case *exprast.IdentifierNode:
		return evaluateIdentifier(typed, scope)
	case *exprast.UnaryNode:
		return evaluateUnaryExpression(typed, scope)
	case *exprast.BinaryNode:
		return evaluateBinaryExpression(typed, scope)
	case *exprast.CallNode:
		return evaluateCallExpression(typed, scope)
	case *exprast.BuiltinNode:
		return evaluateBuiltinExpression(typed, scope)
	case *exprast.MemberNode:
		return evaluateMemberExpression(typed, scope)
	default:
		return nil, fmt.Errorf("unsupported expression node %T", node)
	}
}

func evaluateIdentifier(identifier *exprast.IdentifierNode, scope *evaluationScope) (any, error) {
	if identifier == nil {
		return nil, fmt.Errorf("identifier is required")
	}
	if scope != nil && scope.variables != nil {
		if value, ok := scope.variables[identifier.Value]; ok {
			return value, nil
		}
	}
	switch strings.ToLower(strings.TrimSpace(identifier.Value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "nil", "null":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown identifier %q", identifier.Value)
	}
}

func evaluateUnaryExpression(expression *exprast.UnaryNode, scope *evaluationScope) (any, error) {
	value, err := evaluateAST(expression.Node, scope)
	if err != nil {
		return nil, err
	}
	switch expression.Operator {
	case "!", "not":
		result, ok := coerceBoolValue(value)
		if !ok {
			return nil, fmt.Errorf("operator %s requires a boolean expression", expression.Operator)
		}
		return !result, nil
	case "-":
		result, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("operator - requires a numeric expression")
		}
		return -result, nil
	case "+":
		result, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("operator + requires a numeric expression")
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported unary operator %s", expression.Operator)
	}
}

func evaluateBinaryExpression(expression *exprast.BinaryNode, scope *evaluationScope) (any, error) {
	left, err := evaluateAST(expression.Left, scope)
	if err != nil {
		return nil, err
	}
	right, err := evaluateAST(expression.Right, scope)
	if err != nil {
		return nil, err
	}
	switch expression.Operator {
	case "&&", "and", "||", "or":
		leftValue, ok := coerceBoolValue(left)
		if !ok {
			return nil, fmt.Errorf("logical operator %s requires boolean operands", expression.Operator)
		}
		rightValue, ok := coerceBoolValue(right)
		if !ok {
			return nil, fmt.Errorf("logical operator %s requires boolean operands", expression.Operator)
		}
		if expression.Operator == "&&" || expression.Operator == "and" {
			return leftValue && rightValue, nil
		}
		return leftValue || rightValue, nil
	case "+", "-", "*", "/":
		leftValue, ok := coerceFloatValue(left)
		if !ok {
			return nil, fmt.Errorf("arithmetic operator %s requires numeric operands", expression.Operator)
		}
		rightValue, ok := coerceFloatValue(right)
		if !ok {
			return nil, fmt.Errorf("arithmetic operator %s requires numeric operands", expression.Operator)
		}
		switch expression.Operator {
		case "+":
			return leftValue + rightValue, nil
		case "-":
			return leftValue - rightValue, nil
		case "*":
			return leftValue * rightValue, nil
		default:
			if rightValue == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return leftValue / rightValue, nil
		}
	case ">", ">=", "<", "<=", "==", "!=":
		return compareValues(left, right, expression.Operator)
	default:
		return nil, fmt.Errorf("unsupported binary operator %s", expression.Operator)
	}
}

func evaluateMemberExpression(expression *exprast.MemberNode, scope *evaluationScope) (any, error) {
	base, err := evaluateAST(expression.Node, scope)
	if err != nil {
		return nil, err
	}
	property, ok := memberPropertyName(expression.Property)
	if !ok {
		return nil, fmt.Errorf("unsupported member property %T", expression.Property)
	}
	values, ok := base.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("selector %s requires an object expression", property)
	}
	current, ok := values[property]
	if !ok {
		return nil, fmt.Errorf("unknown field %q", property)
	}
	if previousField := previousFieldName(property); previousField != "" {
		if previous, ok := values[previousField]; ok {
			currentFloat, currentOK := coerceFloatValue(current)
			previousFloat, previousOK := coerceFloatValue(previous)
			if currentOK && previousOK {
				return seriesNumber{Current: currentFloat, Previous: previousFloat, HasCurrent: true, HasPrevious: true}, nil
			}
		}
	}
	return current, nil
}

func memberPropertyName(node exprast.Node) (string, bool) {
	switch typed := node.(type) {
	case *exprast.StringNode:
		return typed.Value, true
	case *exprast.IdentifierNode:
		return typed.Value, true
	default:
		return "", false
	}
}

func evaluateBuiltinExpression(expression *exprast.BuiltinNode, scope *evaluationScope) (any, error) {
	switch strings.ToLower(strings.TrimSpace(expression.Name)) {
	case "abs":
		if len(expression.Arguments) != 1 {
			return nil, fmt.Errorf("abs() requires 1 argument")
		}
		value, err := evaluateAST(expression.Arguments[0], scope)
		if err != nil {
			return nil, err
		}
		floatValue, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("abs() requires a numeric argument")
		}
		return math.Abs(floatValue), nil
	default:
		return nil, fmt.Errorf("unsupported builtin function %q", expression.Name)
	}
}

func evaluateCallExpression(expression *exprast.CallNode, scope *evaluationScope) (any, error) {
	name, ok := expression.Callee.(*exprast.IdentifierNode)
	if !ok {
		return nil, fmt.Errorf("unsupported call target %T", expression.Callee)
	}
	functionName := strings.ToLower(strings.TrimSpace(name.Value))
	switch functionName {
	case "cross_over":
		return evaluateCrossExpression(expression.Arguments, scope, true)
	case "cross_under":
		return evaluateCrossExpression(expression.Arguments, scope, false)
	case "divergence_top":
		return evaluateDivergenceExpression(expression.Arguments, scope, "top")
	case "divergence_bottom":
		return evaluateDivergenceExpression(expression.Arguments, scope, "bottom")
	case "abs":
		if len(expression.Arguments) != 1 {
			return nil, fmt.Errorf("abs() requires 1 argument")
		}
		value, err := evaluateAST(expression.Arguments[0], scope)
		if err != nil {
			return nil, err
		}
		floatValue, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("abs() requires a numeric argument")
		}
		return math.Abs(floatValue), nil
	default:
		return nil, fmt.Errorf("unsupported function %q", name.Value)
	}
}

func evaluateCrossExpression(arguments []exprast.Node, scope *evaluationScope, isCrossOver bool) (bool, error) {
	if len(arguments) != 2 {
		if isCrossOver {
			return false, fmt.Errorf("cross_over() requires 2 arguments")
		}
		return false, fmt.Errorf("cross_under() requires 2 arguments")
	}
	leftValue, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return false, err
	}
	rightValue, err := evaluateAST(arguments[1], scope)
	if err != nil {
		return false, err
	}
	leftSeries, ok := coerceSeriesNumber(leftValue)
	if !ok {
		return false, nil
	}
	rightSeries, ok := coerceSeriesNumber(rightValue)
	if !ok {
		return false, nil
	}
	if !(leftSeries.HasCurrent && leftSeries.HasPrevious && rightSeries.HasCurrent && rightSeries.HasPrevious) {
		return false, nil
	}
	if isCrossOver {
		return leftSeries.Previous <= rightSeries.Previous && leftSeries.Current > rightSeries.Current, nil
	}
	return leftSeries.Previous >= rightSeries.Previous && leftSeries.Current < rightSeries.Current, nil
}

func evaluateDivergenceExpression(arguments []exprast.Node, scope *evaluationScope, direction string) (bool, error) {
	if len(arguments) != 2 {
		return false, fmt.Errorf("divergence_%s() requires 2 arguments", direction)
	}
	aliasIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
	if !ok {
		return false, fmt.Errorf("divergence_%s() requires an indicator alias as the first argument", direction)
	}
	lookbackValue, err := evaluateAST(arguments[1], scope)
	if err != nil {
		return false, err
	}
	lookbackFloat, ok := coerceFloatValue(lookbackValue)
	if !ok || lookbackFloat <= 0 {
		return false, fmt.Errorf("divergence_%s() lookback must be a positive integer", direction)
	}
	binding, ok := scope.bindings[aliasIdentifier.Value]
	if !ok {
		return false, nil
	}
	key, ok := buildDivergenceRequirementKey(binding, direction, int(math.Round(lookbackFloat)))
	if !ok {
		return false, nil
	}
	value, ok := scope.indicators[key]
	if !ok {
		return false, nil
	}
	result, _ := coerceBoolValue(value)
	return result, nil
}

func compareValues(left any, right any, operator string) (bool, error) {
	leftFloat, leftFloatOK := coerceFloatValue(left)
	rightFloat, rightFloatOK := coerceFloatValue(right)
	if leftFloatOK && rightFloatOK {
		switch operator {
		case ">":
			return leftFloat > rightFloat, nil
		case ">=":
			return leftFloat >= rightFloat, nil
		case "<":
			return leftFloat < rightFloat, nil
		case "<=":
			return leftFloat <= rightFloat, nil
		case "==":
			return leftFloat == rightFloat, nil
		default:
			return leftFloat != rightFloat, nil
		}
	}
	leftBool, leftBoolOK := coerceBoolValue(left)
	rightBool, rightBoolOK := coerceBoolValue(right)
	if leftBoolOK && rightBoolOK {
		switch operator {
		case "==":
			return leftBool == rightBool, nil
		case "!=":
			return leftBool != rightBool, nil
		default:
			return false, fmt.Errorf("operator %s requires numeric operands", operator)
		}
	}
	leftText := fmt.Sprintf("%v", left)
	rightText := fmt.Sprintf("%v", right)
	switch operator {
	case "==":
		return leftText == rightText, nil
	case "!=":
		return leftText != rightText, nil
	default:
		return false, fmt.Errorf("operator %s requires numeric operands", operator)
	}
}

func coerceFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case seriesNumber:
		if !typed.HasCurrent {
			return 0, false
		}
		return typed.Current, true
	case map[string]any:
		for _, key := range []string{"value", "diff", "signal", "histogram", "k", "d", "j", "middle", "upper", "lower", "changePercent", "triggerPercent"} {
			if nested, ok := typed[key]; ok {
				if parsed, ok := coerceFloatValue(nested); ok {
					return parsed, true
				}
			}
		}
		return 0, false
	default:
		return 0, false
	}
}

func coerceBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case nil:
		return false, true
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		switch normalized {
		case "", "0", "false", "nil", "null":
			return false, true
		default:
			return true, true
		}
	case seriesNumber:
		if !typed.HasCurrent {
			return false, true
		}
		return typed.Current != 0, true
	default:
		if numeric, ok := coerceFloatValue(value); ok {
			return numeric != 0, true
		}
		return false, false
	}
}

func coerceSeriesNumber(value any) (seriesNumber, bool) {
	switch typed := value.(type) {
	case seriesNumber:
		return typed, true
	case map[string]any:
		for _, pair := range [][2]string{{"value", "previous"}, {"diff", "previousDiff"}, {"signal", "previousSignal"}, {"histogram", "previousHistogram"}, {"k", "previousK"}, {"d", "previousD"}, {"j", "previousJ"}} {
			current, currentOK := typed[pair[0]]
			previous, previousOK := typed[pair[1]]
			if !currentOK || !previousOK {
				continue
			}
			currentFloat, currentFloatOK := coerceFloatValue(current)
			previousFloat, previousFloatOK := coerceFloatValue(previous)
			if currentFloatOK && previousFloatOK {
				return seriesNumber{Current: currentFloat, Previous: previousFloat, HasCurrent: true, HasPrevious: true}, true
			}
		}
		return seriesNumber{}, false
	default:
		current, ok := coerceFloatValue(value)
		if !ok {
			return seriesNumber{}, false
		}
		return seriesNumber{Current: current, Previous: current, HasCurrent: true, HasPrevious: true}, true
	}
}

func previousFieldName(field string) string {
	switch field {
	case "value":
		return "previous"
	case "diff":
		return "previousDiff"
	case "signal":
		return "previousSignal"
	case "histogram":
		return "previousHistogram"
	case "k":
		return "previousK"
	case "d":
		return "previousD"
	case "j":
		return "previousJ"
	default:
		return ""
	}
}
