package pineruntime

import (
	"fmt"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeLetStatement(statement *strategyir.LetStmt, scope *evaluationScope) error {
	binding, recognized, err := r.parseIndicatorBinding(statement)
	if err != nil {
		return err
	}
	if recognized {
		scope.setBinding(statement.Name, binding)
		if snapshot, ok := scope.indicators[binding.Key]; ok {
			scope.setVariable(statement.Name, snapshot)
		} else {
			scope.setVariable(statement.Name, nil)
		}
		return nil
	}
	if statement.Mode == strategyir.AssignmentModeVar {
		if r != nil && r.persistentValues != nil {
			if value, ok := r.persistentValues[statement.Name]; ok {
				scope.setVariable(statement.Name, value)
				return nil
			}
		}
	}
	value, err := evaluateExpression(statement.Expression, scope)
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	switch statement.Mode {
	case strategyir.AssignmentModeVar:
		if r != nil && r.persistentValues != nil {
			r.persistentValues[statement.Name] = persistentValue(nil, value)
		}
		scope.setVariable(statement.Name, value)
	case strategyir.AssignmentModeReassign:
		if r != nil && r.persistentValues != nil {
			if previous, ok := r.persistentValues[statement.Name]; ok {
				value = persistentValue(previous, value)
				r.persistentValues[statement.Name] = value
			}
		}
		scope.assignVariable(statement.Name, value)
	default:
		scope.setVariable(statement.Name, value)
	}
	return nil
}

func persistentValue(previous any, current any) any {
	currentFloat, currentOK := coerceFloatValue(current)
	if !currentOK {
		return current
	}
	result := seriesNumber{Current: currentFloat, HasCurrent: true}
	if previousFloat, previousOK := coerceFloatValue(previous); previousOK {
		result.Previous = previousFloat
		result.HasPrevious = true
	}
	return result
}
