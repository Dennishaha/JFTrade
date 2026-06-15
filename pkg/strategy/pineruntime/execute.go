package pineruntime

import (
	"errors"
	"fmt"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var errLoopBreak = errors.New("pine loop break")
var errLoopContinue = errors.New("pine loop continue")

func (r *strategyRuntime) executeStatements(statements []strategyir.Statement, scope *evaluationScope) (bool, error) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.LetStmt:
			if err := r.executeLetStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.CollectionStmt:
			if err := r.executeCollectionStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.TupleStmt:
			if err := r.executeTupleStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.LoopStmt:
			stopped, err := r.executeLoopStatement(typed, scope)
			if err != nil || stopped {
				return stopped, err
			}
		case *strategyir.ObjectStmt:
			if err := r.executeObjectStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.BreakStmt:
			return false, errLoopBreak
		case *strategyir.ContinueStmt:
			return false, errLoopContinue
		case *strategyir.IfStmt:
			condition, err := evaluateBoolExpression(typed.Condition, scope)
			if err != nil {
				return false, fmt.Errorf("pine line %d: %w", typed.Range.StartLine, err)
			}
			plan := ifScopePlan{thenNeedsClone: true, elseNeedsClone: true}
			if r != nil && r.ifScopePlans != nil {
				if cached, ok := r.ifScopePlans[typed]; ok {
					plan = cached
				}
			}
			if condition {
				branchScope := scope
				if plan.thenNeedsClone {
					branchScope = scope.clone()
				}
				stopped, err := r.executeStatements(typed.Then, branchScope)
				if err != nil || stopped {
					return stopped, err
				}
				continue
			}
			branchScope := scope
			if plan.elseNeedsClone {
				branchScope = scope.clone()
			}
			stopped, err := r.executeStatements(typed.Else, branchScope)
			if err != nil || stopped {
				return stopped, err
			}
		case *strategyir.LogStmt:
			r.log(typed.Message)
		case *strategyir.NotifyStmt:
			r.notify(typed.Message)
		case *strategyir.OrderStmt:
			if err := r.executeOrderStatement(typed, scope); err != nil {
				return false, err
			}
		case *strategyir.ExitStmt:
			stopped, err := r.executeExitStatement(typed, scope)
			if err != nil || stopped {
				return stopped, err
			}
		case *strategyir.CancelStmt:
			r.executeCancelStatement(typed)
		case *strategyir.ProtectStmt:
			stopped, err := r.executeProtectStatement(typed, scope)
			if err != nil || stopped {
				return stopped, err
			}
		default:
			return false, fmt.Errorf("unsupported IR statement type %T", statement)
		}
	}
	return false, nil
}
