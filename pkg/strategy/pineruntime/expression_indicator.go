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

func evaluateAdvancedSourcePeriodExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 2 && len(arguments) != 3 {
		return nil, fmt.Errorf("%s() requires source, length, and optional timeframe", functionName)
	}
	sourceText, ok := expressionLiteralArgument(arguments[0])
	if !ok {
		return nil, fmt.Errorf("%s() source must be a literal source", functionName)
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
	if !sourceOK {
		return nil, fmt.Errorf("%s() source %q is not supported", functionName, sourceText)
	}
	period, err := positiveIntArgument(functionName, arguments[1], scope)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s:%s:%d", functionName, source, period)
	if len(arguments) == 3 {
		timeUnitText, literal := expressionLiteralArgument(arguments[2])
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !literal || !timeUnitOK || timeUnit == "" {
			return nil, fmt.Errorf("%s() timeframe %q is not supported", functionName, timeUnitText)
		}
		key += ":" + timeUnit
	}
	return indicatorSnapshotValue(scope, key), nil
}

func evaluateAnchoredVWAPExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 2 {
		return nil, fmt.Errorf("anchored_vwap() requires source and anchor unit")
	}
	sourceText, sourceLiteral := expressionLiteralArgument(arguments[0])
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
	unit, unitLiteral := expressionLiteralArgument(arguments[1])
	unit = strings.ToLower(strings.TrimSpace(unit))
	if !sourceLiteral || !sourceOK || !unitLiteral || (unit != "day" && unit != "week" && unit != "month") {
		return nil, fmt.Errorf("anchored_vwap() supports OHLCV/derived source and day/week/month anchors")
	}
	return indicatorSnapshotValue(scope, "anchored_vwap:"+unit+":"+source), nil
}

func evaluateBBWExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 3 && len(arguments) != 4 {
		return nil, fmt.Errorf("bbw() requires source, length, multiplier, and optional timeframe")
	}
	sourceText, ok := expressionLiteralArgument(arguments[0])
	if !ok {
		return nil, fmt.Errorf("bbw() source must be a literal source")
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
	if !sourceOK {
		return nil, fmt.Errorf("bbw() source %q is not supported", sourceText)
	}
	period, err := positiveIntArgument("bbw", arguments[1], scope)
	if err != nil {
		return nil, err
	}
	multiplier, multiplierOK, err := evaluateFloatOperand(arguments[2], scope)
	if err != nil {
		return nil, err
	}
	if !multiplierOK || multiplier <= 0 {
		return nil, fmt.Errorf("bbw() multiplier must be positive")
	}
	key := fmt.Sprintf("bbw:%s:%d:%s", source, period, strconv.FormatFloat(multiplier, 'f', -1, 64))
	if len(arguments) == 4 {
		timeUnitText, literal := expressionLiteralArgument(arguments[3])
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !literal || !timeUnitOK || timeUnit == "" {
			return nil, fmt.Errorf("bbw() timeframe %q is not supported", timeUnitText)
		}
		key += ":" + timeUnit
	}
	return indicatorSnapshotValue(scope, key), nil
}

func evaluateMovingAverageExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 2 || len(arguments) > 4 {
		return nil, fmt.Errorf("ma() requires type, period, optional time unit, and optional source")
	}
	averageTypeText, ok := expressionLiteralArgument(arguments[0])
	if !ok {
		return nil, fmt.Errorf("ma() type must be a literal moving-average name")
	}
	averageType, ok := indicatorbinding.ParseMovingAverageType(averageTypeText)
	if !ok {
		return nil, fmt.Errorf("ma() type %q is not supported", strings.TrimSpace(averageTypeText))
	}
	periodValue, periodOK, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	if !periodOK || periodValue <= 0 || math.Trunc(periodValue) != periodValue {
		return nil, fmt.Errorf("ma() period must be a positive integer")
	}
	optionalArgs := make([]string, 0, len(arguments)-2)
	for _, argument := range arguments[2:] {
		value, ok := expressionLiteralArgument(argument)
		if !ok {
			return nil, fmt.Errorf("ma() optional time unit/source arguments must be literals")
		}
		optionalArgs = append(optionalArgs, value)
	}
	timeUnit, source, err := indicatorbinding.ParseMovingAverageOptionalArgs(optionalArgs)
	if err != nil {
		return nil, err
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	key := indicatorbinding.BuildMovingAverageKeyWithSource(averageType, int(periodValue), timeUnit, source)
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func evaluateSecuritySourceExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 2 || len(arguments) > 3 {
		return nil, fmt.Errorf("security_source() requires source, time unit, and optional lookback")
	}
	sourceText, ok := expressionLiteralArgument(arguments[0])
	if !ok {
		return nil, fmt.Errorf("security_source() source must be a literal source")
	}
	source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
	if !sourceOK {
		return nil, fmt.Errorf("security_source() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", sourceText)
	}
	timeUnitText, ok := expressionLiteralArgument(arguments[1])
	if !ok {
		return nil, fmt.Errorf("security_source() time unit must be a literal")
	}
	timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
	if !timeUnitOK || timeUnit == "" {
		return nil, fmt.Errorf("security_source() time unit %q is not supported", timeUnitText)
	}
	lookback := 0
	if len(arguments) == 3 {
		lookbackValue, lookbackOK, err := evaluateFloatOperand(arguments[2], scope)
		if err != nil {
			return nil, err
		}
		if !lookbackOK || lookbackValue < 0 || math.Trunc(lookbackValue) != lookbackValue {
			return nil, fmt.Errorf("security_source() lookback must be a non-negative integer")
		}
		lookback = int(lookbackValue)
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	key := "security_source:" + timeUnit + ":" + source
	if lookback > 0 {
		key += ":" + strconv.Itoa(lookback)
	}
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

func expressionLiteralArgument(node exprast.Node) (string, bool) {
	switch typed := node.(type) {
	case *exprast.IdentifierNode:
		return strings.TrimSpace(typed.Value), strings.TrimSpace(typed.Value) != ""
	case *exprast.StringNode:
		return strings.TrimSpace(typed.Value), true
	default:
		return "", false
	}
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
	case 3:
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
		timeUnitText, ok := expressionLiteralArgument(arguments[2])
		if !ok {
			return "", fmt.Errorf("%s() timeframe must be a literal", functionName)
		}
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !timeUnitOK || timeUnit == "" {
			return "", fmt.Errorf("%s() timeframe %q is not supported", functionName, timeUnitText)
		}
		return fmt.Sprintf("%s:%s:%s:%s", functionName, source, periodText, timeUnit), nil
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

func evaluateMACDExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 3 && len(arguments) != 5 {
		return nil, fmt.Errorf("macd() requires fast, slow, signal, optional timeframe, and optional source")
	}
	fast, slow, signal, err := positiveIntTriple("macd", arguments[0], arguments[1], arguments[2], scope)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("macd:%d:%d:%d", fast, slow, signal)
	if len(arguments) == 5 {
		timeUnitText, ok := expressionLiteralArgument(arguments[3])
		if !ok {
			return nil, fmt.Errorf("macd() timeframe must be a literal")
		}
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		sourceText, ok := expressionLiteralArgument(arguments[4])
		if !ok {
			return nil, fmt.Errorf("macd() source must be a literal source")
		}
		source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
		if !timeUnitOK || timeUnit == "" || !sourceOK {
			return nil, fmt.Errorf("macd() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe")
		}
		key = fmt.Sprintf("macd:%s:%d:%d:%d:%s", source, fast, slow, signal, timeUnit)
	}
	return indicatorSnapshotValue(scope, key), nil
}

func evaluateATRExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 && len(arguments) != 2 {
		return nil, fmt.Errorf("atr() requires period and optional timeframe")
	}
	period, err := positiveIntArgument("atr", arguments[0], scope)
	if err != nil {
		return nil, err
	}
	key := "atr:" + strconv.Itoa(period)
	if len(arguments) == 2 {
		timeUnitText, ok := expressionLiteralArgument(arguments[1])
		if !ok {
			return nil, fmt.Errorf("atr() timeframe must be a literal")
		}
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !timeUnitOK || timeUnit == "" {
			return nil, fmt.Errorf("atr() timeframe %q is not supported", timeUnitText)
		}
		key += ":" + timeUnit
	}
	return indicatorSnapshotValue(scope, key), nil
}

func evaluateBollingerExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 2 && len(arguments) != 4 {
		return nil, fmt.Errorf("bollinger() requires period, multiplier, optional timeframe, and optional source")
	}
	period, err := positiveIntArgument("bollinger", arguments[0], scope)
	if err != nil {
		return nil, err
	}
	multiplier, ok, err := evaluateFloatOperand(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	if !ok || multiplier <= 0 {
		return nil, fmt.Errorf("bollinger() multiplier must be a positive number")
	}
	multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
	key := "bollinger:" + strconv.Itoa(period) + ":" + multiplierText
	if len(arguments) == 4 {
		timeUnitText, ok := expressionLiteralArgument(arguments[2])
		if !ok {
			return nil, fmt.Errorf("bollinger() timeframe must be a literal")
		}
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		sourceText, ok := expressionLiteralArgument(arguments[3])
		if !ok {
			return nil, fmt.Errorf("bollinger() source must be a literal source")
		}
		source, sourceOK := indicatorbinding.ParsePriceSource(sourceText)
		if !timeUnitOK || timeUnit == "" || !sourceOK {
			return nil, fmt.Errorf("bollinger() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe")
		}
		key = fmt.Sprintf("bollinger:%s:%d:%s:%s", source, period, multiplierText, timeUnit)
	}
	return indicatorSnapshotValue(scope, key), nil
}

func positiveIntTriple(functionName string, leftNode, middleNode, rightNode exprast.Node, scope *evaluationScope) (int, int, int, error) {
	left, err := positiveIntArgument(functionName, leftNode, scope)
	if err != nil {
		return 0, 0, 0, err
	}
	middle, err := positiveIntArgument(functionName, middleNode, scope)
	if err != nil {
		return 0, 0, 0, err
	}
	right, err := positiveIntArgument(functionName, rightNode, scope)
	if err != nil {
		return 0, 0, 0, err
	}
	return left, middle, right, nil
}

func positiveIntArgument(functionName string, node exprast.Node, scope *evaluationScope) (int, error) {
	value, ok, err := evaluateFloatOperand(node, scope)
	if err != nil {
		return 0, err
	}
	if !ok || value <= 0 || math.Trunc(value) != value {
		return 0, fmt.Errorf("%s() arguments must be positive integers", functionName)
	}
	return int(value), nil
}

func indicatorSnapshotValue(scope *evaluationScope, key string) any {
	if scope == nil || scope.indicators == nil {
		return nil
	}
	value, ok := scope.indicators[key]
	if !ok || value == nil {
		return nil
	}
	return value
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
	if len(arguments) != 4 && len(arguments) != 5 {
		return nil, fmt.Errorf("stoch() requires source, high, low, length, and optional time unit arguments")
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
	if len(arguments) == 5 {
		timeUnitText, ok := expressionLiteralArgument(arguments[4])
		if !ok {
			return nil, fmt.Errorf("stoch() time unit must be a literal")
		}
		timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !ok {
			return nil, fmt.Errorf("stoch() time unit %q is not supported", timeUnitText)
		}
		key += ":" + timeUnit
	}
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
	if len(arguments) != 2 && len(arguments) != 3 {
		return nil, fmt.Errorf("supertrend() requires factor, atrPeriod, and optional timeframe")
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
	if len(arguments) == 3 {
		timeUnitText, ok := expressionLiteralArgument(arguments[2])
		if !ok {
			return nil, fmt.Errorf("supertrend() timeframe must be a literal")
		}
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(timeUnitText)
		if !timeUnitOK || timeUnit == "" {
			return nil, fmt.Errorf("supertrend() timeframe %q is not supported", timeUnitText)
		}
		key += ":" + timeUnit
	}
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
