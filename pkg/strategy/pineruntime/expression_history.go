package pineruntime

import (
	"fmt"
	"math"

	exprast "github.com/expr-lang/expr/ast"
)

func evaluatePreviousExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf("previous() requires 1 argument")
	}
	value, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	series, ok := coerceSeriesNumber(value)
	if !ok || !series.HasPrevious {
		return nil, nil
	}
	return series.Previous, nil
}

func evaluateHistoryExpression(expression *exprast.CallNode, scope *evaluationScope) (any, error) {
	if expression == nil || len(expression.Arguments) != 2 {
		return nil, fmt.Errorf("history() requires expression and lookback arguments")
	}
	lookbackValue, ok, err := evaluateFloatOperand(expression.Arguments[1], scope)
	if err != nil {
		return nil, err
	}
	if !ok || lookbackValue < 0 || math.Trunc(lookbackValue) != lookbackValue {
		return nil, fmt.Errorf("history() lookback must be a non-negative integer")
	}
	lookback := int(lookbackValue)
	if lookback == 0 {
		return evaluateAST(expression.Arguments[0], scope)
	}
	if scope == nil || scope.runtime == nil || scope.runtime.historyValues == nil {
		return nil, nil
	}
	key := expressionNodeKey(expression.Arguments[0])
	value, ok := scope.runtime.historyValues[key].lookup(lookback)
	if !ok {
		return nil, nil
	}
	return value, nil
}
