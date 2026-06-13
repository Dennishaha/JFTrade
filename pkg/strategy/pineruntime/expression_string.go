package pineruntime

import (
	"fmt"
	"strconv"

	exprast "github.com/expr-lang/expr/ast"
)

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
