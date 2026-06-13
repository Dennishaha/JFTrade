package pineruntime

import (
	"fmt"
	"math"
	"strconv"

	exprast "github.com/expr-lang/expr/ast"
)

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
