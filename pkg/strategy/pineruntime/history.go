package pineruntime

import (
	"fmt"
	"math"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func collectProgramHistoryTargets(program *strategyir.Program) map[string]historyTarget {
	targets := map[string]historyTarget{}
	if program == nil {
		return targets
	}
	for _, hook := range program.Hooks {
		collectStatementHistoryTargets(hook.Statements, targets)
	}
	return targets
}

func collectStatementHistoryTargets(statements []strategyir.Statement, targets map[string]historyTarget) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			collectExpressionHistoryTargets(typed.Expression, targets)
		case *strategyir.IfStmt:
			collectExpressionHistoryTargets(typed.Condition, targets)
			collectStatementHistoryTargets(typed.Then, targets)
			collectStatementHistoryTargets(typed.Else, targets)
		case *strategyir.OrderStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.LimitExpression, targets)
			collectExpressionHistoryTargets(typed.StopExpression, targets)
		case *strategyir.ExitStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.StopExpression, targets)
			collectExpressionHistoryTargets(typed.LimitExpression, targets)
			collectExpressionHistoryTargets(typed.TrailPoints, targets)
			collectExpressionHistoryTargets(typed.TrailOffset, targets)
		case *strategyir.ProtectStmt:
			collectExpressionHistoryTargets(typed.QuantityExpression, targets)
			collectExpressionHistoryTargets(typed.TimeValueExpression, targets)
			collectExpressionHistoryTargets(typed.PercentageExpression, targets)
		}
	}
}

func (r *strategyRuntime) preparseProgramExpressions() error {
	if r == nil || r.program == nil {
		return nil
	}
	scope := &evaluationScope{runtime: r}
	for _, hook := range r.program.Hooks {
		if err := preparseStatementExpressions(hook.Statements, scope); err != nil {
			return err
		}
	}
	return nil
}

func preparseStatementExpressions(statements []strategyir.Statement, scope *evaluationScope) error {
	for _, statement := range statements {
		var expressions []string
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			expressions = []string{typed.Expression}
		case *strategyir.IfStmt:
			expressions = []string{typed.Condition}
			if err := preparseStatementExpressions(typed.Then, scope); err != nil {
				return err
			}
			if err := preparseStatementExpressions(typed.Else, scope); err != nil {
				return err
			}
		case *strategyir.OrderStmt:
			expressions = []string{typed.QuantityExpression, typed.LimitExpression, typed.StopExpression}
		case *strategyir.ExitStmt:
			expressions = []string{typed.QuantityExpression, typed.StopExpression, typed.LimitExpression, typed.TrailPoints, typed.TrailOffset}
		case *strategyir.ProtectStmt:
			expressions = []string{typed.QuantityExpression, typed.TimeValueExpression, typed.PercentageExpression}
		}
		for _, expression := range expressions {
			if strings.TrimSpace(expression) == "" {
				continue
			}
			if _, err := parseExpression(expression, scope); err != nil {
				return fmt.Errorf("line %d: %w", statement.SourceRange().StartLine, err)
			}
		}
	}
	return nil
}

func collectExpressionHistoryTargets(expression string, targets map[string]historyTarget) {
	if strings.TrimSpace(expression) == "" || targets == nil {
		return
	}
	parsed, err := parseExpression(expression, nil)
	if err != nil {
		return
	}
	collectHistoryTargetsFromNode(parsed, targets)
}

func collectHistoryTargetsFromNode(node exprast.Node, targets map[string]historyTarget) {
	switch typed := node.(type) {
	case nil:
		return
	case *exprast.CallNode:
		name, ok := typed.Callee.(*exprast.IdentifierNode)
		if ok && strings.EqualFold(strings.TrimSpace(name.Value), "history") && len(typed.Arguments) == 2 {
			lookback := historyLookbackFromNode(typed.Arguments[1])
			key := expressionNodeKey(typed.Arguments[0])
			if existing, ok := targets[key]; !ok || lookback > existing.maxLookback {
				targets[key] = historyTarget{key: key, expression: typed.Arguments[0], maxLookback: lookback}
			}
		}
		collectHistoryTargetsFromNode(typed.Callee, targets)
		for _, argument := range typed.Arguments {
			collectHistoryTargetsFromNode(argument, targets)
		}
	case *exprast.UnaryNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
	case *exprast.BinaryNode:
		collectHistoryTargetsFromNode(typed.Left, targets)
		collectHistoryTargetsFromNode(typed.Right, targets)
	case *exprast.MemberNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
		collectHistoryTargetsFromNode(typed.Property, targets)
	case *exprast.PredicateNode:
		collectHistoryTargetsFromNode(typed.Node, targets)
	}
}

func historyLookbackFromNode(node exprast.Node) int {
	switch typed := node.(type) {
	case *exprast.IntegerNode:
		return typed.Value
	case *exprast.FloatNode:
		return int(math.Trunc(typed.Value))
	default:
		return 0
	}
}

func (r *strategyRuntime) recordHistorySnapshots(scope *evaluationScope) {
	if r == nil || scope == nil || len(r.historyTargets) == 0 {
		return
	}
	if r.historyValues == nil {
		r.historyValues = buildHistoryBuffers(r.historyTargets)
	}
	for key, target := range r.historyTargets {
		value, err := evaluateAST(target.expression, scope)
		if err != nil {
			value = nil
		}
		buffer := r.historyValues[key]
		if buffer == nil {
			buffer = newHistoryBuffer(target.maxLookback)
			r.historyValues[key] = buffer
		}
		buffer.push(snapshotExpressionValue(value))
	}
}

func buildHistoryBuffers(targets map[string]historyTarget) map[string]*historyBuffer {
	buffers := make(map[string]*historyBuffer, len(targets))
	for key, target := range targets {
		buffers[key] = newHistoryBuffer(target.maxLookback)
	}
	return buffers
}
