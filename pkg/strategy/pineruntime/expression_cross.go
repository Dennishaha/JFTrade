package pineruntime

import (
	"fmt"
	"math"

	exprast "github.com/expr-lang/expr/ast"
)

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
	if !leftSeries.HasCurrent || !leftSeries.HasPrevious || !rightSeries.HasCurrent || !rightSeries.HasPrevious {
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
	binding, ok := scope.binding(aliasIdentifier.Value)
	if !ok {
		return false, nil
	}
	lookback := int(math.Round(lookbackFloat))
	var key string
	if runtime := scope.runtime; runtime != nil {
		key, ok = runtime.cachedDivergenceRequirementKey(binding, direction, lookback)
	} else {
		key, ok = buildDivergenceRequirementKey(binding, direction, lookback)
	}
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
