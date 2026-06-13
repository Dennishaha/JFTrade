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
			return expression
		}
		open := start + len(prefix) - 1
		close := matchingParen(expression, open)
		if close < 0 {
			return expression
		}
		expression = expression[:start] + "tostring(" + expression[open+1:close] + ")" + expression[close+1:]
	}
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
