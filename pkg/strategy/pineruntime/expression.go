package pineruntime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	exprast "github.com/expr-lang/expr/ast"
	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

type seriesNumber struct {
	Current     float64
	Previous    float64
	HasCurrent  bool
	HasPrevious bool
}

type objectFieldReader interface {
	FieldValue(name string) (any, bool)
}

type objectSeriesReader interface {
	SeriesField(name string) (current float64, previous float64, hasCurrent bool, hasPrevious bool, ok bool)
}

type preferredScalarReader interface {
	PreferredScalarValue() (float64, bool)
}

type scalarValueReader interface {
	ScalarValue() (float64, bool)
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
		return nil, fmt.Errorf("unknown field %q", property)
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

var missingObjectField = struct{}{}

func readObjectField(base any, property string) (any, bool) {
	switch values := base.(type) {
	case map[string]any:
		value, ok := values[property]
		if !ok {
			return missingObjectField, true
		}
		return value, true
	case objectFieldReader:
		value, ok := values.FieldValue(property)
		if !ok {
			return missingObjectField, true
		}
		return value, true
	default:
		return nil, false
	}
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
		return evaluateMathExpression("abs", expression.Arguments, scope)
	case "min", "max", "round", "floor", "ceil", "sqrt", "pow", "log", "sign":
		return evaluateMathExpression(strings.ToLower(strings.TrimSpace(expression.Name)), expression.Arguments, scope)
	case "sum":
		return evaluateWindowNumericExpression("sum", expression.Arguments, scope)
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
	case "previous":
		return evaluatePreviousExpression(expression.Arguments, scope)
	case "history":
		return evaluateHistoryExpression(expression, scope)
	case "ifelse":
		return evaluateIfElseExpression(expression.Arguments, scope)
	case "nz":
		return evaluateNZExpression(expression.Arguments, scope)
	case "abs":
		return evaluateMathExpression(functionName, expression.Arguments, scope)
	case "min", "max", "round", "floor", "ceil", "sqrt", "pow", "log", "sign":
		return evaluateMathExpression(functionName, expression.Arguments, scope)
	case "stdev":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "20")
	case "rsi":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "14")
	case "variance":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "20")
	case "cci":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "hlc3", "20")
	case "vwap":
		return evaluateSourceIndicatorExpression(functionName, expression.Arguments, scope, "hlc3")
	case "cum":
		return evaluateRequiredSourceIndicatorExpression(functionName, expression.Arguments, scope)
	case "mfi":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "hlc3", "14")
	case "stoch":
		return evaluateStochExpression(expression.Arguments, scope)
	case "dmi":
		return evaluateDMIExpression(expression.Arguments, scope)
	case "supertrend":
		return evaluateSupertrendExpression(expression.Arguments, scope)
	case "sar":
		return evaluateSARExpression(expression.Arguments, scope)
	case "tr":
		return evaluateTrueRangeExpression(expression.Arguments, scope)
	case "timestamp":
		return evaluateTimestampExpression(expression.Arguments, scope)
	case "barssince":
		return evaluateBarsSinceExpression(expression, scope)
	case "valuewhen":
		return evaluateValueWhenExpression(expression, scope)
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "sum":
		return evaluateWindowNumericExpression(functionName, expression.Arguments, scope)
	case "rising", "falling":
		return evaluateWindowBoolExpression(functionName, expression.Arguments, scope)
	case "tostring":
		return evaluateToStringExpression(expression.Arguments, scope)
	default:
		return nil, fmt.Errorf("unsupported function %q", name.Value)
	}
}

func evaluateMathExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	unary := func(fn func(float64) float64) (any, error) {
		if len(arguments) != 1 {
			return nil, fmt.Errorf("%s() requires 1 argument", functionName)
		}
		value, ok, err := evaluateFloatOperand(arguments[0], scope)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s() requires a numeric argument", functionName)
		}
		return fn(value), nil
	}
	switch functionName {
	case "abs":
		return unary(math.Abs)
	case "round":
		return unary(math.Round)
	case "floor":
		return unary(math.Floor)
	case "ceil":
		return unary(math.Ceil)
	case "sqrt":
		return unary(math.Sqrt)
	case "log":
		return unary(math.Log)
	case "sign":
		return unary(func(value float64) float64 {
			switch {
			case value > 0:
				return 1
			case value < 0:
				return -1
			default:
				return 0
			}
		})
	case "pow":
		if len(arguments) != 2 {
			return nil, fmt.Errorf("pow() requires 2 arguments")
		}
		left, leftOK, err := evaluateFloatOperand(arguments[0], scope)
		if err != nil {
			return nil, err
		}
		right, rightOK, err := evaluateFloatOperand(arguments[1], scope)
		if err != nil {
			return nil, err
		}
		if !leftOK || !rightOK {
			return nil, fmt.Errorf("pow() requires numeric arguments")
		}
		return math.Pow(left, right), nil
	case "min", "max":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("%s() requires at least 2 arguments", functionName)
		}
		result, ok, err := evaluateFloatOperand(arguments[0], scope)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s() requires numeric arguments", functionName)
		}
		for _, argument := range arguments[1:] {
			value, valueOK, valueErr := evaluateFloatOperand(argument, scope)
			if valueErr != nil {
				return nil, valueErr
			}
			if !valueOK {
				return nil, fmt.Errorf("%s() requires numeric arguments", functionName)
			}
			if functionName == "min" {
				result = math.Min(result, value)
			} else {
				result = math.Max(result, value)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported math function %q", functionName)
	}
}

func evaluateStdDevExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf("stdev() requires 1 argument")
	}
	period, ok, err := evaluateFloatOperand(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	if !ok || period <= 0 || math.Trunc(period) != period {
		return nil, fmt.Errorf("stdev() period must be a positive integer")
	}
	if scope == nil || scope.indicators == nil {
		return nil, nil
	}
	value, ok := scope.indicators["stdev:"+strconv.Itoa(int(period))]
	if !ok || value == nil {
		return nil, nil
	}
	return value, nil
}

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

func evaluateTimestampExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 3 || len(arguments) > 5 {
		return nil, fmt.Errorf("timestamp() requires year, month, day, optional hour, and optional minute")
	}
	values := make([]int, 5)
	values[3] = 0
	values[4] = 0
	for index, argument := range arguments {
		value, ok, err := evaluateFloatOperand(argument, scope)
		if err != nil {
			return nil, err
		}
		if !ok || math.Trunc(value) != value {
			return nil, fmt.Errorf("timestamp() arguments must be integers")
		}
		values[index] = int(value)
	}
	return float64(time.Date(values[0], time.Month(values[1]), values[2], values[3], values[4], 0, 0, time.UTC).UnixMilli()), nil
}

func evaluateBarsSinceExpression(expression *exprast.CallNode, scope *evaluationScope) (any, error) {
	if len(expression.Arguments) != 1 {
		return nil, fmt.Errorf("barssince() requires one condition argument")
	}
	if scope == nil || scope.runtime == nil {
		return nil, nil
	}
	key := expressionNodeKey(expression)
	state := scope.runtime.barssinceStates[key]
	if state == nil {
		state = &barssinceState{lastBarIndex: -1}
		scope.runtime.barssinceStates[key] = state
	}
	if state.hasCached && state.lastBarIndex == scope.barIndex {
		return state.cached, nil
	}
	conditionValue, err := evaluateAST(expression.Arguments[0], scope)
	if err != nil {
		return nil, err
	}
	condition, ok := strictBoolValue(conditionValue)
	if !ok {
		return nil, fmt.Errorf("barssince() condition must be boolean")
	}
	if condition {
		state.seen = true
		state.value = 0
		state.cached = float64(0)
	} else if state.seen {
		state.value++
		state.cached = float64(state.value)
	} else {
		state.cached = nil
	}
	state.lastBarIndex = scope.barIndex
	state.hasCached = true
	return state.cached, nil
}

func evaluateValueWhenExpression(expression *exprast.CallNode, scope *evaluationScope) (any, error) {
	if len(expression.Arguments) != 3 {
		return nil, fmt.Errorf("valuewhen() requires condition, source, and occurrence")
	}
	if scope == nil || scope.runtime == nil {
		return nil, nil
	}
	key := "valuewhen:" + expressionNodeKey(expression.Arguments[0]) + ":" + expressionNodeKey(expression.Arguments[1])
	state := scope.runtime.valuewhenStates[key]
	if state == nil {
		state = &valuewhenState{lastBarIndex: -1}
		scope.runtime.valuewhenStates[key] = state
	}
	occurrenceValue, ok, err := evaluateFloatOperand(expression.Arguments[2], scope)
	if err != nil {
		return nil, err
	}
	if !ok || occurrenceValue < 0 || math.Trunc(occurrenceValue) != occurrenceValue {
		return nil, fmt.Errorf("valuewhen() occurrence must be a non-negative integer")
	}
	occurrence := int(occurrenceValue)
	if state.hasCached && state.lastBarIndex == scope.barIndex {
		if occurrence < len(state.values) {
			return state.values[occurrence], nil
		}
		return nil, nil
	}
	conditionValue, err := evaluateAST(expression.Arguments[0], scope)
	if err != nil {
		return nil, err
	}
	condition, ok := strictBoolValue(conditionValue)
	if !ok {
		return nil, fmt.Errorf("valuewhen() condition must be boolean")
	}
	if condition {
		sourceValue, sourceErr := evaluateAST(expression.Arguments[1], scope)
		if sourceErr != nil {
			return nil, sourceErr
		}
		state.values = append([]any{snapshotExpressionValue(sourceValue)}, state.values...)
	}
	state.lastBarIndex = scope.barIndex
	state.hasCached = true
	if occurrence < len(state.values) {
		state.cached = state.values[occurrence]
		return state.cached, nil
	}
	state.cached = nil
	return nil, nil
}

func snapshotExpressionValue(value any) any {
	switch typed := value.(type) {
	case *seriesNumber:
		if typed == nil {
			return nil
		}
		return *typed
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, value := range typed {
			copied[key] = snapshotExpressionValue(value)
		}
		return copied
	default:
		return value
	}
}

func expressionNodeKey(node exprast.Node) string {
	switch typed := node.(type) {
	case nil:
		return "<nil>"
	case *exprast.IdentifierNode:
		return "id:" + typed.Value
	case *exprast.IntegerNode:
		return "int:" + strconv.Itoa(typed.Value)
	case *exprast.FloatNode:
		return "float:" + strconv.FormatFloat(typed.Value, 'f', -1, 64)
	case *exprast.StringNode:
		return "string:" + typed.Value
	case *exprast.BoolNode:
		if typed.Value {
			return "bool:true"
		}
		return "bool:false"
	case *exprast.NilNode:
		return "nil"
	case *exprast.UnaryNode:
		return "unary:" + typed.Operator + ":" + expressionNodeKey(typed.Node)
	case *exprast.BinaryNode:
		return "binary:" + typed.Operator + ":" + expressionNodeKey(typed.Left) + ":" + expressionNodeKey(typed.Right)
	case *exprast.MemberNode:
		return "member:" + expressionNodeKey(typed.Node) + "." + expressionNodeKey(typed.Property)
	case *exprast.PredicateNode:
		return "predicate:" + expressionNodeKey(typed.Node)
	case *exprast.CallNode:
		parts := make([]string, 0, len(typed.Arguments)+1)
		parts = append(parts, "call:"+expressionNodeKey(typed.Callee))
		for _, argument := range typed.Arguments {
			parts = append(parts, expressionNodeKey(argument))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%T", node)
	}
}

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

func evaluateToStringExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 1 || len(arguments) > 2 {
		return nil, fmt.Errorf("tostring() requires value and optional format")
	}
	value, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	if len(arguments) == 2 {
		if _, err := evaluateAST(arguments[1], scope); err != nil {
			return nil, err
		}
	}
	switch typed := value.(type) {
	case nil:
		return "na", nil
	case string:
		return typed, nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	default:
		if numeric, ok := coerceFloatValue(value); ok {
			return strconv.FormatFloat(numeric, 'f', -1, 64), nil
		}
		return fmt.Sprintf("%v", value), nil
	}
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
	values := scope.runtime.historyValues[key]
	if lookback > len(values) {
		return nil, nil
	}
	return values[len(values)-lookback], nil
}

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

func compareValues(left any, right any, operator string) (bool, error) {
	if left == nil || right == nil {
		switch operator {
		case "==":
			return left == nil && right == nil, nil
		case "!=":
			return !(left == nil && right == nil), nil
		default:
			return false, fmt.Errorf("operator %s requires numeric operands", operator)
		}
	}
	leftFloat, leftFloatOK := coerceFloatValue(left)
	rightFloat, rightFloatOK := coerceFloatValue(right)
	if leftFloatOK && rightFloatOK {
		return compareFloatValues(leftFloat, rightFloat, operator), nil
	}
	leftBool, leftBoolOK := coerceBoolValue(left)
	rightBool, rightBoolOK := coerceBoolValue(right)
	if leftBoolOK && rightBoolOK {
		switch operator {
		case "==":
			return compareBoolValues(leftBool, rightBool, operator), nil
		case "!=":
			return compareBoolValues(leftBool, rightBool, operator), nil
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
	case *seriesNumber:
		if typed == nil || !typed.HasCurrent {
			return 0, false
		}
		return typed.Current, true
	case scalarValueReader:
		return typed.ScalarValue()
	case map[string]any:
		return coerceFloatFromMapFields(typed)
	case objectFieldReader:
		return coerceFloatFromObjectFields(typed)
	default:
		return 0, false
	}
}

var scalarObjectFieldCandidates = [...]string{"value", "diff", "signal", "histogram", "k", "d", "j", "middle", "upper", "lower", "plus", "minus", "adx", "line", "direction", "changePercent", "triggerPercent"}

func coerceFloatFromMapFields(values map[string]any) (float64, bool) {
	for _, key := range scalarObjectFieldCandidates {
		nested, ok := values[key]
		if !ok {
			continue
		}
		if parsed, ok := coerceFloatValue(nested); ok {
			return parsed, true
		}
	}
	return 0, false
}

func coerceFloatFromObjectFields(values objectFieldReader) (float64, bool) {
	if preferred, ok := values.(preferredScalarReader); ok {
		if parsed, ok := preferred.PreferredScalarValue(); ok {
			return parsed, true
		}
	}
	for _, key := range scalarObjectFieldCandidates {
		nested, ok := values.FieldValue(key)
		if !ok {
			continue
		}
		if parsed, ok := coerceFloatValue(nested); ok {
			return parsed, true
		}
	}
	return 0, false
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
	case *seriesNumber:
		if typed == nil || !typed.HasCurrent {
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

func strictBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case nil:
		return false, true
	default:
		return false, false
	}
}

func coerceSeriesNumber(value any) (seriesNumber, bool) {
	switch typed := value.(type) {
	case seriesNumber:
		return typed, true
	case *seriesNumber:
		if typed == nil {
			return seriesNumber{}, false
		}
		return *typed, true
	case map[string]any, objectFieldReader:
		if values, ok := value.(objectSeriesReader); ok {
			for _, field := range [...]string{"value", "diff", "signal", "histogram", "k", "d", "j"} {
				current, previous, currentOK, previousOK, seriesOK := values.SeriesField(field)
				if !seriesOK || !currentOK || !previousOK {
					continue
				}
				return seriesNumber{Current: current, Previous: previous, HasCurrent: true, HasPrevious: true}, true
			}
		}
		for _, pair := range [][2]string{{"value", "previous"}, {"diff", "previousDiff"}, {"signal", "previousSignal"}, {"histogram", "previousHistogram"}, {"k", "previousK"}, {"d", "previousD"}, {"j", "previousJ"}} {
			current, currentOK := readObjectField(value, pair[0])
			previous, previousOK := readObjectField(value, pair[1])
			if !currentOK || !previousOK || current == missingObjectField || previous == missingObjectField {
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
