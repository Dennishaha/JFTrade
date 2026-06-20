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
