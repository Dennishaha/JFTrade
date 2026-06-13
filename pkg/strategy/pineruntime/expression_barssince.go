package pineruntime

import (
	"fmt"

	exprast "github.com/expr-lang/expr/ast"
)

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
