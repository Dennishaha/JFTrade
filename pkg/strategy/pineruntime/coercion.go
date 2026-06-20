package pineruntime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func compareValues(left any, right any, operator string) (bool, error) {
	if left == nil || right == nil {
		switch operator {
		case "==":
			return left == nil && right == nil, nil
		case "!=":
			return left != nil || right != nil, nil
		default:
			return false, fmt.Errorf("operator %s requires numeric operands", operator)
		}
	}
	leftFloat, leftFloatOK := coerceFloatValue(left)
	rightFloat, rightFloatOK := coerceFloatValue(right)
	if leftFloatOK && rightFloatOK {
		return compareFloatValues(leftFloat, rightFloat, operator), nil
	}
	leftBool, leftBoolOK := coerceBoolValue(left)
	rightBool, rightBoolOK := coerceBoolValue(right)
	if leftBoolOK && rightBoolOK {
		switch operator {
		case "==":
			return compareBoolValues(leftBool, rightBool, operator), nil
		case "!=":
			return compareBoolValues(leftBool, rightBool, operator), nil
		default:
			return false, fmt.Errorf("operator %s requires numeric operands", operator)
		}
	}
	leftText := fmt.Sprintf("%v", left)
	rightText := fmt.Sprintf("%v", right)
	switch operator {
	case "==":
		return leftText == rightText, nil
	case "!=":
		return leftText != rightText, nil
	default:
		return false, fmt.Errorf("operator %s requires numeric operands", operator)
	}
}

func coerceFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case seriesNumber:
		if !typed.HasCurrent {
			return 0, false
		}
		return typed.Current, true
	case *seriesNumber:
		if typed == nil || !typed.HasCurrent {
			return 0, false
		}
		return typed.Current, true
	case scalarValueReader:
		return typed.ScalarValue()
	case map[string]any:
		return coerceFloatFromMapFields(typed)
	case objectFieldReader:
		return coerceFloatFromObjectFields(typed)
	default:
		return 0, false
	}
}

var scalarObjectFieldCandidates = [...]string{"value", "diff", "signal", "histogram", "k", "d", "j", "middle", "upper", "lower", "plus", "minus", "adx", "line", "direction", "changePercent", "triggerPercent"}

func coerceFloatFromMapFields(values map[string]any) (float64, bool) {
	for _, key := range scalarObjectFieldCandidates {
		nested, ok := values[key]
		if !ok {
			continue
		}
		if parsed, ok := coerceFloatValue(nested); ok {
			return parsed, true
		}
	}
	return 0, false
}

func coerceFloatFromObjectFields(values objectFieldReader) (float64, bool) {
	if preferred, ok := values.(preferredScalarReader); ok {
		if parsed, ok := preferred.PreferredScalarValue(); ok {
			return parsed, true
		}
	}
	for _, key := range scalarObjectFieldCandidates {
		nested, ok := values.FieldValue(key)
		if !ok {
			continue
		}
		if parsed, ok := coerceFloatValue(nested); ok {
			return parsed, true
		}
	}
	return 0, false
}

func coerceBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case nil:
		return false, true
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		switch normalized {
		case "", "0", "false", "nil", "null":
			return false, true
		default:
			return true, true
		}
	case seriesNumber:
		if !typed.HasCurrent {
			return false, true
		}
		return typed.Current != 0, true
	case *seriesNumber:
		if typed == nil || !typed.HasCurrent {
			return false, true
		}
		return typed.Current != 0, true
	default:
		if numeric, ok := coerceFloatValue(value); ok {
			return numeric != 0, true
		}
		return false, false
	}
}

func strictBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case nil:
		return false, true
	default:
		return false, false
	}
}

func coerceSeriesNumber(value any) (seriesNumber, bool) {
	switch typed := value.(type) {
	case seriesNumber:
		return typed, true
	case *seriesNumber:
		if typed == nil {
			return seriesNumber{}, false
		}
		return *typed, true
	case map[string]any, objectFieldReader:
		if values, ok := value.(objectSeriesReader); ok {
			for _, field := range [...]string{"value", "diff", "signal", "histogram", "k", "d", "j"} {
				current, previous, currentOK, previousOK, seriesOK := values.SeriesField(field)
				if !seriesOK || !currentOK || !previousOK {
					continue
				}
				return seriesNumber{Current: current, Previous: previous, HasCurrent: true, HasPrevious: true}, true
			}
		}
		for _, pair := range [][2]string{{"value", "previous"}, {"diff", "previousDiff"}, {"signal", "previousSignal"}, {"histogram", "previousHistogram"}, {"k", "previousK"}, {"d", "previousD"}, {"j", "previousJ"}} {
			current, currentOK := readObjectField(value, pair[0])
			previous, previousOK := readObjectField(value, pair[1])
			if !currentOK || !previousOK || current == missingObjectField || previous == missingObjectField {
				continue
			}
			currentFloat, currentFloatOK := coerceFloatValue(current)
			previousFloat, previousFloatOK := coerceFloatValue(previous)
			if currentFloatOK && previousFloatOK {
				return seriesNumber{Current: currentFloat, Previous: previousFloat, HasCurrent: true, HasPrevious: true}, true
			}
		}
		return seriesNumber{}, false
	default:
		current, ok := coerceFloatValue(value)
		if !ok {
			return seriesNumber{}, false
		}
		return seriesNumber{Current: current, Previous: current, HasCurrent: true, HasPrevious: true}, true
	}
}

func previousFieldName(field string) string {
	switch field {
	case "value":
		return "previous"
	case "diff":
		return "previousDiff"
	case "signal":
		return "previousSignal"
	case "histogram":
		return "previousHistogram"
	case "k":
		return "previousK"
	case "d":
		return "previousD"
	case "j":
		return "previousJ"
	default:
		return ""
	}
}
