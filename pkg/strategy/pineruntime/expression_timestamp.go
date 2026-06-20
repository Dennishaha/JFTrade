package pineruntime

import (
	"fmt"
	"math"
	"time"

	exprast "github.com/expr-lang/expr/ast"
)

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
	return float64(time.Date(values[0], time.Month(values[1]), values[2], values[3], values[4], 0, 0, pineExchangeLocation(scope)).UnixMilli()), nil
}
