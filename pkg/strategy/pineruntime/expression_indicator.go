package pineruntime

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

func evaluateSourcePeriodIndicatorExpression(functionName string, arguments []exprast.Node, scope *evaluationScope, defaultSource string, defaultPeriod string) (any, error) {
	key, err := sourcePeriodIndicatorLookupKey(functionName, arguments, scope, defaultSource, defaultPeriod)
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

func sourcePeriodIndicatorLookupKey(functionName string, arguments []exprast.Node, scope *evaluationScope, defaultSource string, defaultPeriod string) (string, error) {
	source := defaultSource
	periodText := defaultPeriod
	switch len(arguments) {
	case 1:
		if sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode); ok {
			if parsedSource, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value); sourceOK {
				source = parsedSource
				break
			}
		}
		periodValue, ok, err := evaluateFloatOperand(arguments[0], scope)
		if err != nil {
			return "", err
		}
		if !ok || periodValue <= 0 || math.Trunc(periodValue) != periodValue {
			return "", fmt.Errorf("%s() length must be a positive integer", functionName)
		}
		periodText = strconv.Itoa(int(periodValue))
	case 2:
		sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
		if !ok {
			return "", fmt.Errorf("%s() source must be open/high/low/close/volume/hl2/hlc3/ohlc4", functionName)
		}
		parsedSource, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value)
		if !sourceOK {
			return "", fmt.Errorf("%s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", functionName, sourceIdentifier.Value)
		}
		source = parsedSource
		periodValue, ok, err := evaluateFloatOperand(arguments[1], scope)
		if err != nil {
			return "", err
		}
		if !ok || periodValue <= 0 || math.Trunc(periodValue) != periodValue {
			return "", fmt.Errorf("%s() length must be a positive integer", functionName)
		}
		periodText = strconv.Itoa(int(periodValue))
	default:
		return "", fmt.Errorf("%s() requires source and length arguments", functionName)
	}
	period, err := strconv.Atoi(periodText)
	if err != nil || period <= 0 {
		return "", fmt.Errorf("%s() length must be a positive integer", functionName)
	}
	legacySource := "close"
	if functionName == "cci" {
		legacySource = "hlc3"
	}
	if source == legacySource {
		return functionName + ":" + strconv.Itoa(period), nil
	}
	return functionName + ":" + source + ":" + strconv.Itoa(period), nil
}

func evaluateSourceIndicatorExpression(functionName string, arguments []exprast.Node, scope *evaluationScope, defaultSource string) (any, error) {
	source := defaultSource
	if len(arguments) > 1 {
		return nil, fmt.Errorf("%s() requires at most one source argument", functionName)
	}
	if len(arguments) == 1 {
		sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
		if !ok {
			return nil, fmt.Errorf("%s() source must be open/high/low/close/volume/hl2/hlc3/ohlc4", functionName)
		}
		parsedSource, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value)
		if !sourceOK {
			return nil, fmt.Errorf("%s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", functionName, sourceIdentifier.Value)
		}
		source = parsedSource
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators[functionName+":"+source]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateRequiredSourceIndicatorExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf("%s() requires one source argument", functionName)
	}
	sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
	if !ok {
		return nil, fmt.Errorf("%s() source must be open/high/low/close/volume/hl2/hlc3/ohlc4", functionName)
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value)
	if !sourceOK {
		return nil, fmt.Errorf("%s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", functionName, sourceIdentifier.Value)
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators[functionName+":"+source]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateStochExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 4 {
		return nil, fmt.Errorf("stoch() requires source, high, low, and length arguments")
	}
	sourceIdentifier, ok := arguments[0].(*exprast.IdentifierNode)
	if !ok {
		return nil, fmt.Errorf("stoch() source must be open/high/low/close/hl2/hlc3/ohlc4")
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceIdentifier.Value)
	if !sourceOK || source == "volume" {
		return nil, fmt.Errorf("stoch() source %q is not supported; use open/high/low/close/hl2/hlc3/ohlc4", sourceIdentifier.Value)
	}
	highIdentifier, highOK := arguments[1].(*exprast.IdentifierNode)
	lowIdentifier, lowOK := arguments[2].(*exprast.IdentifierNode)
	if !highOK || !strings.EqualFold(strings.TrimSpace(highIdentifier.Value), "high") || !lowOK || !strings.EqualFold(strings.TrimSpace(lowIdentifier.Value), "low") {
		return nil, fmt.Errorf("stoch() currently supports literal high and low arguments only")
	}
	period, ok, err := evaluateFloatOperand(arguments[3], scope)
	if err != nil {
		return nil, err
	}
	if !ok || period <= 0 || math.Trunc(period) != period {
		return nil, fmt.Errorf("stoch() length must be a positive integer")
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	key := "stoch:" + source + ":" + strconv.Itoa(int(period))
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateDMIExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 2 {
		return nil, fmt.Errorf("dmi() requires diLength and adxSmoothing")
	}
	left, ok, err := evaluateFloatOperand(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	right, rightOK, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	if !ok || !rightOK || left <= 0 || right <= 0 || math.Trunc(left) != left || math.Trunc(right) != right {
		return nil, fmt.Errorf("dmi() arguments must be positive integers")
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators["dmi:"+strconv.Itoa(int(left))+":"+strconv.Itoa(int(right))]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateSupertrendExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 2 {
		return nil, fmt.Errorf("supertrend() requires factor and atrPeriod")
	}
	factor, ok, err := evaluateFloatOperand(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	period, periodOK, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	if !ok || !periodOK || factor <= 0 || period <= 0 || math.Trunc(period) != period {
		return nil, fmt.Errorf("supertrend() requires a positive factor and positive integer atrPeriod")
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	key := "supertrend:" + strconv.FormatFloat(factor, 'f', -1, 64) + ":" + strconv.Itoa(int(period))
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateSARExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 3 {
		return nil, fmt.Errorf("sar() requires start, increment, and max")
	}
	start, startOK, err := evaluateFloatOperand(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	increment, incrementOK, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	maximum, maximumOK, err := evaluateFloatOperand(arguments[2], scope)
	if err != nil {
		return nil, err
	}
	if !startOK || !incrementOK || !maximumOK || start <= 0 || increment <= 0 || maximum <= 0 {
		return nil, fmt.Errorf("sar() arguments must be positive numbers")
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	key := "sar:" +
		strconv.FormatFloat(start, 'f', -1, 64) + ":" +
		strconv.FormatFloat(increment, 'f', -1, 64) + ":" +
		strconv.FormatFloat(maximum, 'f', -1, 64)
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateTrueRangeExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) > 1 {
		return nil, fmt.Errorf("tr() requires zero arguments or one boolean argument")
	}
	if len(arguments) == 1 {
		if _, _, err := evaluateBoolOperand(arguments[0], scope); err != nil {
			return nil, err
		}
	}
	if scope == nil || !scope.hasBarData {
		return nil, nil
	}
	high := scope.highSeries.Current
	low := scope.lowSeries.Current
	if !scope.closeSeries.HasPrevious {
		return high - low, nil
	}
	previousClose := scope.closeSeries.Previous
	return math.Max(high-low, math.Max(math.Abs(high-previousClose), math.Abs(low-previousClose))), nil
}
