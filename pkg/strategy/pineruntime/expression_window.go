package pineruntime

import (
	"fmt"
	"math"
	"strconv"

	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

func evaluateWindowNumericExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	key, err := windowIndicatorLookupKey(functionName, arguments, scope)
	if err != nil {
		return nil, err
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateWindowBoolExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	key, err := windowIndicatorLookupKey(functionName, arguments, scope)
	if err != nil {
		return nil, err
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	result, resultOK := strictBoolValue(value)
	if !resultOK {
		return nil, fmt.Errorf("%s() indicator value is not boolean", functionName)
	}
	return result, nil
}

func windowIndicatorLookupKey(functionName string, arguments []exprast.Node, scope *evaluationScope) (string, error) {
	if len(arguments) != 2 {
		return "", fmt.Errorf("%s() requires source and length arguments", functionName)
	}
	sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
	if !ok {
		return "", fmt.Errorf("%s() source must be open/high/low/close/volume/hl2/hlc3/ohlc4", functionName)
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value)
	if !sourceOK {
		return "", fmt.Errorf("%s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", functionName, sourceIdentifier.Value)
	}
	period, ok, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return "", err
	}
	if !ok || period <= 0 || math.Trunc(period) != period {
		return "", fmt.Errorf("%s() length must be a positive integer", functionName)
	}
	return functionName + ":" + source + ":" + strconv.Itoa(int(period)), nil
}
