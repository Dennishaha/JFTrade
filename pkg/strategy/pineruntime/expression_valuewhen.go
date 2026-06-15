package pineruntime

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
)

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
			return state.values[len(state.values)-1-occurrence], nil
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
		state.values = append(state.values, snapshotExpressionValue(sourceValue))
	}
	state.lastBarIndex = scope.barIndex
	state.hasCached = true
	if occurrence < len(state.values) {
		state.cached = state.values[len(state.values)-1-occurrence]
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
	case *pineArray:
		if typed == nil {
			return nil
		}
		values := make([]any, len(typed.values))
		for index, value := range typed.values {
			values[index] = snapshotExpressionValue(value)
		}
		return &pineArray{elementType: typed.elementType, values: values}
	case *pineMap:
		if typed == nil {
			return nil
		}
		values := make(map[any]any, len(typed.values))
		for key, value := range typed.values {
			values[snapshotExpressionValue(key)] = snapshotExpressionValue(value)
		}
		return &pineMap{keyType: typed.keyType, valueType: typed.valueType, values: values}
	case *pineMatrix:
		if typed == nil {
			return nil
		}
		values := make([]any, len(typed.values))
		for index, value := range typed.values {
			values[index] = snapshotExpressionValue(value)
		}
		return &pineMatrix{elementType: typed.elementType, rows: typed.rows, columns: typed.columns, values: values}
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
