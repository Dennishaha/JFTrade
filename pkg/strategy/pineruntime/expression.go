package pineruntime

import (
	"fmt"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
)

var missingObjectField = struct{}{}

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
	result, ok := strictBoolValue(value)
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
		parsed, err := strategyexpression.ParseExpression(trimmed)
		if err != nil {
			return nil, err
		}
		scope.runtime.expressionCache[trimmed] = parsed
		return parsed, nil
	}
	return strategyexpression.ParseExpression(trimmed)
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
	case *exprast.ConditionalNode:
		return evaluateConditionalExpression(typed, scope)
	case *exprast.CallNode:
		return evaluateCallExpression(typed, scope)
	case *exprast.BuiltinNode:
		return evaluateBuiltinExpression(typed, scope)
	case *exprast.MemberNode:
		return evaluateMemberExpression(typed, scope)
	case *exprast.PredicateNode:
		return evaluateAST(typed.Node, scope)
	default:
		return nil, fmt.Errorf("unsupported expression node %T", node)
	}
}

func evaluateConditionalExpression(expression *exprast.ConditionalNode, scope *evaluationScope) (any, error) {
	conditionValue, err := evaluateAST(expression.Cond, scope)
	if err != nil {
		return nil, err
	}
	condition, ok := strictBoolValue(conditionValue)
	if !ok {
		return nil, fmt.Errorf("conditional expression requires a boolean condition")
	}
	if condition {
		return evaluateAST(expression.Exp1, scope)
	}
	return evaluateAST(expression.Exp2, scope)
}

func evaluateIdentifier(identifier *exprast.IdentifierNode, scope *evaluationScope) (any, error) {
	if identifier == nil {
		return nil, fmt.Errorf("identifier is required")
	}
	if value, ok := scope.variable(identifier.Value); ok {
		return value, nil
	}
	switch strings.ToLower(strings.TrimSpace(identifier.Value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "nil", "null", "na":
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
	switch expression.Operator {
	case "&&", "and", "||", "or":
		leftValue, ok, err := evaluateStrictBoolOperand(expression.Left, scope)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("logical operator %s requires boolean operands", expression.Operator)
		}
		if expression.Operator == "&&" || expression.Operator == "and" {
			if !leftValue {
				return false, nil
			}
			rightValue, ok, err := evaluateStrictBoolOperand(expression.Right, scope)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("logical operator %s requires boolean operands", expression.Operator)
			}
			return leftValue && rightValue, nil
		}
		if leftValue {
			return true, nil
		}
		rightValue, ok, err := evaluateStrictBoolOperand(expression.Right, scope)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("logical operator %s requires boolean operands", expression.Operator)
		}
		return leftValue || rightValue, nil
	case "+", "-", "*", "/":
		leftRaw, err := evaluateAST(expression.Left, scope)
		if err != nil {
			return nil, err
		}
		rightRaw, err := evaluateAST(expression.Right, scope)
		if err != nil {
			return nil, err
		}
		if leftRaw == nil || rightRaw == nil {
			return nil, nil
		}
		leftValue, ok := coerceFloatValue(leftRaw)
		if !ok {
			return nil, fmt.Errorf("arithmetic operator %s requires numeric operands", expression.Operator)
		}
		rightValue, ok := coerceFloatValue(rightRaw)
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
		left, err := evaluateAST(expression.Left, scope)
		if err != nil {
			return nil, err
		}
		right, err := evaluateAST(expression.Right, scope)
		if err != nil {
			return nil, err
		}
		if (left == nil || right == nil) && expression.Operator != "==" && expression.Operator != "!=" {
			return false, nil
		}
		return compareValues(left, right, expression.Operator)
	default:
		return nil, fmt.Errorf("unsupported binary operator %s", expression.Operator)
	}
}

func compareFloatValues(left float64, right float64, operator string) bool {
	switch operator {
	case ">":
		return left > right
	case ">=":
		return left >= right
	case "<":
		return left < right
	case "<=":
		return left <= right
	case "==":
		return left == right
	default:
		return left != right
	}
}

func compareBoolValues(left bool, right bool, operator string) bool {
	if operator == "==" {
		return left == right
	}
	return left != right
}

func evaluateFloatOperand(node exprast.Node, scope *evaluationScope) (float64, bool, error) {
	switch typed := node.(type) {
	case *exprast.IntegerNode:
		return float64(typed.Value), true, nil
	case *exprast.FloatNode:
		return typed.Value, true, nil
	case *exprast.UnaryNode:
		if typed.Operator == "+" || typed.Operator == "-" {
			value, ok, err := evaluateFloatOperand(typed.Node, scope)
			if err != nil || !ok {
				return 0, ok, err
			}
			if typed.Operator == "-" {
				return -value, true, nil
			}
			return value, true, nil
		}
	}
	value, err := evaluateAST(node, scope)
	if err != nil {
		return 0, false, err
	}
	result, ok := coerceFloatValue(value)
	return result, ok, nil
}

func evaluateBoolOperand(node exprast.Node, scope *evaluationScope) (bool, bool, error) {
	switch typed := node.(type) {
	case *exprast.BoolNode:
		return typed.Value, true, nil
	case *exprast.UnaryNode:
		if typed.Operator == "!" || typed.Operator == "not" {
			value, ok, err := evaluateBoolOperand(typed.Node, scope)
			if err != nil || !ok {
				return false, ok, err
			}
			return !value, true, nil
		}
	}
	value, err := evaluateAST(node, scope)
	if err != nil {
		return false, false, err
	}
	result, ok := coerceBoolValue(value)
	return result, ok, nil
}

func evaluateStrictBoolOperand(node exprast.Node, scope *evaluationScope) (bool, bool, error) {
	value, err := evaluateAST(node, scope)
	if err != nil {
		return false, false, err
	}
	result, ok := strictBoolValue(value)
	return result, ok, nil
}

func evaluateMemberExpression(expression *exprast.MemberNode, scope *evaluationScope) (any, error) {
	base, err := evaluateAST(expression.Node, scope)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return nil, nil
	}
	property, ok := memberPropertyName(expression.Property)
	if !ok {
		return nil, fmt.Errorf("unsupported member property %T", expression.Property)
	}
	if values, ok := base.(objectSeriesReader); ok {
		current, previous, currentOK, previousOK, seriesOK := values.SeriesField(property)
		if seriesOK {
			if currentOK && previousOK {
				return seriesNumber{Current: current, Previous: previous, HasCurrent: true, HasPrevious: true}, nil
			}
			if currentOK {
				return current, nil
			}
			return nil, nil
		}
	}
	current, ok := readObjectField(base, property)
	if !ok {
		return nil, fmt.Errorf("selector %s requires an object expression", property)
	}
	if current == missingObjectField {
		return nil, nil
	}
	if previousField := previousFieldName(property); previousField != "" {
		if previous, ok := readObjectField(base, previousField); ok && previous != missingObjectField {
			currentFloat, currentOK := coerceFloatValue(current)
			previousFloat, previousOK := coerceFloatValue(previous)
			if currentOK && previousOK {
				return seriesNumber{Current: currentFloat, Previous: previousFloat, HasCurrent: true, HasPrevious: true}, nil
			}
		}
	}
	return current, nil
}
