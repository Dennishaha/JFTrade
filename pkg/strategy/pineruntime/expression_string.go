package pineruntime

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

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

func evaluateStringHelperExpression(name string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	switch name {
	case "str_length":
		value, err := requiredStringArgument(arguments, scope, 0, "str.length")
		if err != nil {
			return nil, err
		}
		return float64(utf8.RuneCountInString(value)), nil
	case "str_contains":
		value, err := requiredStringArgument(arguments, scope, 0, "str.contains")
		if err != nil {
			return nil, err
		}
		needle, err := requiredStringArgument(arguments, scope, 1, "str.contains")
		if err != nil {
			return nil, err
		}
		return strings.Contains(value, needle), nil
	case "str_pos":
		value, err := requiredStringArgument(arguments, scope, 0, "str.pos")
		if err != nil {
			return nil, err
		}
		needle, err := requiredStringArgument(arguments, scope, 1, "str.pos")
		if err != nil {
			return nil, err
		}
		return float64(strings.Index(value, needle)), nil
	case "str_substring":
		value, err := requiredStringArgument(arguments, scope, 0, "str.substring")
		if err != nil {
			return nil, err
		}
		begin, err := requiredStringIndex(arguments, scope, 1, "str.substring")
		if err != nil {
			return nil, err
		}
		end := utf8.RuneCountInString(value)
		if len(arguments) > 2 {
			end, err = requiredStringIndex(arguments, scope, 2, "str.substring")
			if err != nil {
				return nil, err
			}
		}
		runes := []rune(value)
		if begin < 0 || begin > end || end > len(runes) {
			return nil, fmt.Errorf("str.substring range [%d,%d) is out of bounds for length %d", begin, end, len(runes))
		}
		return string(runes[begin:end]), nil
	case "str_replace":
		value, err := requiredStringArgument(arguments, scope, 0, "str.replace")
		if err != nil {
			return nil, err
		}
		target, err := requiredStringArgument(arguments, scope, 1, "str.replace")
		if err != nil {
			return nil, err
		}
		replacement, err := requiredStringArgument(arguments, scope, 2, "str.replace")
		if err != nil {
			return nil, err
		}
		return strings.ReplaceAll(value, target, replacement), nil
	case "str_upper", "str_lower":
		value, err := requiredStringArgument(arguments, scope, 0, name)
		if err != nil {
			return nil, err
		}
		if name == "str_upper" {
			return strings.ToUpper(value), nil
		}
		return strings.ToLower(value), nil
	case "str_format":
		if len(arguments) == 0 {
			return nil, fmt.Errorf("str.format requires a format string")
		}
		format, err := requiredStringArgument(arguments, scope, 0, "str.format")
		if err != nil {
			return nil, err
		}
		result := format
		for index := 1; index < len(arguments); index++ {
			value, err := evaluateAST(arguments[index], scope)
			if err != nil {
				return nil, err
			}
			text := stringifyPineValue(value)
			placeholder := "{" + strconv.Itoa(index-1) + "}"
			result = strings.ReplaceAll(result, placeholder, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported string helper %q", name)
	}
}

func requiredStringArgument(arguments []exprast.Node, scope *evaluationScope, index int, name string) (string, error) {
	if len(arguments) <= index {
		return "", fmt.Errorf("%s requires argument %d", name, index+1)
	}
	value, err := evaluateAST(arguments[index], scope)
	if err != nil {
		return "", err
	}
	return stringifyPineValue(value), nil
}

func requiredStringIndex(arguments []exprast.Node, scope *evaluationScope, index int, name string) (int, error) {
	if len(arguments) <= index {
		return 0, fmt.Errorf("%s requires index argument", name)
	}
	value, err := evaluateAST(arguments[index], scope)
	if err != nil {
		return 0, err
	}
	numeric, ok := coerceFloatValue(value)
	if !ok || numeric < 0 || numeric != float64(int(numeric)) {
		return 0, fmt.Errorf("%s index must be a non-negative integer", name)
	}
	return int(numeric), nil
}

func stringifyPineValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "na"
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		if numeric, ok := coerceFloatValue(value); ok {
			return strconv.FormatFloat(numeric, 'f', -1, 64)
		}
		return fmt.Sprintf("%v", value)
	}
}
