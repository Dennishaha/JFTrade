package pineruntime

import (
	"fmt"

	exprast "github.com/expr-lang/expr/ast"
)

func evaluateIfElseExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 3 {
		return nil, fmt.Errorf("ifelse() requires 3 arguments")
	}
	conditionValue, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	condition, ok := strictBoolValue(conditionValue)
	if !ok {
		return nil, fmt.Errorf("ifelse() condition must be boolean")
	}
	if condition {
		return evaluateAST(arguments[1], scope)
	}
	return evaluateAST(arguments[2], scope)
}

func evaluateNZExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 1 || len(arguments) > 2 {
		return nil, fmt.Errorf("nz() requires 1 or 2 arguments")
	}
	value, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	if _, ok := coerceFloatValue(value); ok {
		return value, nil
	}
	if value != nil {
		return value, nil
	}
	if len(arguments) == 2 {
		return evaluateAST(arguments[1], scope)
	}
	return float64(0), nil
}
