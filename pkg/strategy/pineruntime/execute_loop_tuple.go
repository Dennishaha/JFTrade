package pineruntime

import (
	"errors"
	"fmt"
	"math"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeTupleStatement(statement *strategyir.TupleStmt, scope *evaluationScope) error {
	if statement == nil || len(statement.Names) != len(statement.Expressions) {
		return fmt.Errorf("invalid tuple statement")
	}
	values := make([]any, len(statement.Expressions))
	for index, expression := range statement.Expressions {
		value, err := evaluateExpression(expression, scope)
		if err != nil {
			return fmt.Errorf("pine line %d: tuple element %d: %w", statement.Range.StartLine, index+1, err)
		}
		values[index] = value
	}
	for index, name := range statement.Names {
		if name == "_" {
			continue
		}
		value := values[index]
		switch statement.Mode {
		case strategyir.AssignmentModeVar:
			if r != nil && r.persistentValues != nil {
				if previous, ok := r.persistentValues[name]; ok {
					value = previous
				} else {
					r.persistentValues[name] = persistentValue(nil, value)
					value = r.persistentValues[name]
				}
			}
			scope.setVariable(name, value)
		case strategyir.AssignmentModeReassign:
			scope.assignVariable(name, value)
		default:
			scope.setVariable(name, value)
		}
	}
	return nil
}

func (r *strategyRuntime) executeLoopStatement(statement *strategyir.LoopStmt, scope *evaluationScope) (bool, error) {
	if statement == nil {
		return false, nil
	}
	limit := statement.MaxIterations
	if limit <= 0 {
		limit = 1000
	}
	if strings.TrimSpace(statement.Collection) != "" {
		value, err := evaluateExpression(statement.Collection, scope)
		if err != nil {
			return false, fmt.Errorf("pine line %d: collection for source: %w", statement.Range.StartLine, err)
		}
		array, ok := value.(*pineArray)
		if !ok || array == nil {
			return false, fmt.Errorf("pine line %d: collection for supports arrays only; use map.keys() or map.values() for maps", statement.Range.StartLine)
		}
		values := append([]any(nil), array.values...)
		for index, item := range values {
			if index+1 > limit {
				return false, fmt.Errorf("pine line %d: collection for loop exceeded %d iterations on one bar", statement.Range.StartLine, limit)
			}
			if statement.IndexVariable != "" && statement.IndexVariable != "_" {
				scope.setVariable(statement.IndexVariable, float64(index))
			}
			if statement.Variable != "" && statement.Variable != "_" {
				scope.setVariable(statement.Variable, item)
			}
			stopped, loopErr := r.executeStatements(statement.Body, scope)
			if errors.Is(loopErr, errLoopBreak) {
				return false, nil
			}
			if errors.Is(loopErr, errLoopContinue) {
				continue
			}
			if loopErr != nil || stopped {
				return stopped, loopErr
			}
		}
		return false, nil
	}
	if strings.TrimSpace(statement.WhileCondition) != "" {
		for iteration := 0; iteration < limit; iteration++ {
			condition, err := evaluateBoolExpression(statement.WhileCondition, scope)
			if err != nil {
				return false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
			}
			if !condition {
				return false, nil
			}
			stopped, err := r.executeStatements(statement.Body, scope)
			if errors.Is(err, errLoopBreak) {
				return false, nil
			}
			if errors.Is(err, errLoopContinue) {
				continue
			}
			if err != nil || stopped {
				return stopped, err
			}
		}
		return false, fmt.Errorf("pine line %d: while loop exceeded %d iterations on one bar", statement.Range.StartLine, limit)
	}

	start, err := evaluateLoopInteger(statement.StartExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: for start: %w", statement.Range.StartLine, err)
	}
	end, err := evaluateLoopInteger(statement.EndExpression, scope)
	if err != nil {
		return false, fmt.Errorf("pine line %d: for end: %w", statement.Range.StartLine, err)
	}
	step := 1
	if strings.TrimSpace(statement.StepExpression) != "" {
		step, err = evaluateLoopInteger(statement.StepExpression, scope)
		if err != nil {
			return false, fmt.Errorf("pine line %d: for step: %w", statement.Range.StartLine, err)
		}
	}
	if step == 0 {
		return false, fmt.Errorf("pine line %d: for loop step cannot be 0", statement.Range.StartLine)
	}
	iterations := 0
	for value := start; (step > 0 && value <= end) || (step < 0 && value >= end); value += step {
		iterations++
		if iterations > limit {
			return false, fmt.Errorf("pine line %d: for loop exceeded %d iterations on one bar", statement.Range.StartLine, limit)
		}
		scope.setVariable(statement.Variable, float64(value))
		stopped, loopErr := r.executeStatements(statement.Body, scope)
		if errors.Is(loopErr, errLoopBreak) {
			return false, nil
		}
		if errors.Is(loopErr, errLoopContinue) {
			continue
		}
		if loopErr != nil || stopped {
			return stopped, loopErr
		}
	}
	return false, nil
}

func evaluateLoopInteger(expression string, scope *evaluationScope) (int, error) {
	value, err := evaluateExpression(expression, scope)
	if err != nil {
		return 0, err
	}
	number, ok := coerceFloatValue(value)
	if !ok || math.Trunc(number) != number {
		return 0, fmt.Errorf("loop bound must be an integer")
	}
	return int(number), nil
}
