package pine

import "strings"

func lowerInputCalls(expression string) string {
	for {
		lower := strings.ToLower(expression)
		start := strings.Index(lower, "input(")
		dotStart := strings.Index(lower, "input.")
		if start < 0 || (dotStart >= 0 && dotStart < start) {
			start = dotStart
		}
		if start < 0 {
			return expression
		}
		open := strings.Index(expression[start:], "(")
		if open < 0 {
			return expression
		}
		open += start
		close := matchingParen(expression, open)
		if close < 0 {
			return expression
		}
		args := splitArguments(expression[open+1 : close])
		replacement := inputDefaultValue(args)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceStringNamespace(expression string) string {
	prefix := "str.tostring("
	for {
		start := strings.Index(strings.ToLower(expression), prefix)
		if start < 0 {
			break
		}
		open := start + len(prefix) - 1
		close := matchingParen(expression, open)
		if close < 0 {
			return expression
		}
		expression = expression[:start] + "tostring(" + expression[open+1:close] + ")" + expression[close+1:]
	}
	for _, name := range []string{"length", "contains", "pos", "substring", "replace", "upper", "lower", "format"} {
		prefix := "str." + name + "("
		for {
			start := strings.Index(strings.ToLower(expression), prefix)
			if start < 0 {
				break
			}
			open := start + len(prefix) - 1
			close := matchingParen(expression, open)
			if close < 0 {
				return expression
			}
			expression = expression[:start] + "str_" + name + "(" + expression[open+1:close] + ")" + expression[close+1:]
		}
	}
	return expression
}

func replaceTimeframeNamespace(expression string) string {
	for _, mapping := range []struct {
		pine string
		ir   string
	}{
		{pine: "change", ir: "timeframe_change"},
		{pine: "in_seconds", ir: "timeframe_in_seconds"},
	} {
		prefix := "timeframe." + mapping.pine + "("
		for {
			start := strings.Index(strings.ToLower(expression), prefix)
			if start < 0 {
				break
			}
			open := start + len(prefix) - 1
			close := matchingParen(expression, open)
			if close < 0 {
				return expression
			}
			expression = expression[:start] + mapping.ir + "(" + expression[open+1:close] + ")" + expression[close+1:]
		}
	}
	return expression
}

func inputDefaultValue(args []string) string {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(strings.TrimSpace(key), "defval") {
			return strings.TrimSpace(value)
		}
	}
	if len(args) == 0 {
		return "na"
	}
	return strings.TrimSpace(args[0])
}
