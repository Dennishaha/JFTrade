package pineruntime

import (
	"fmt"
	"math"

	exprast "github.com/expr-lang/expr/ast"
)

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
	case "round_to_mintick":
		if len(arguments) != 1 {
			return nil, fmt.Errorf("round_to_mintick() requires 1 argument")
		}
		value, ok, err := evaluateFloatOperand(arguments[0], scope)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("round_to_mintick() requires a numeric argument")
		}
		tick := 0.01
		if scope != nil && scope.runtime != nil && scope.runtime.session != nil && scope.runtime.symbol != "" {
			if market, ok := scope.runtime.session.Market(scope.runtime.symbol); ok && market.TickSize.Sign() > 0 {
				tick = market.TickSize.Float64()
			}
		}
		if tick <= 0 {
			return value, nil
		}
		return math.Round(value/tick) * tick, nil
	case "avg", "min", "max":
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
			switch functionName {
			case "avg":
				result += value
			case "min":
				result = math.Min(result, value)
			default:
				result = math.Max(result, value)
			}
		}
		if functionName == "avg" {
			result /= float64(len(arguments))
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported math function %q", functionName)
	}
}
