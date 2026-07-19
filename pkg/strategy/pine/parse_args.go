package pine

import (
	"strconv"
	"strings"
)

func hasNamedArg(args []string, name string) bool {
	for _, arg := range args {
		key, _, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func namedArgValue(args []string, name string) (string, bool) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, name) {
			return value, true
		}
	}
	return "", false
}

func pineOrderPrices(args []string) (string, string, string) {
	orderType := "MARKET"
	limitExpr := ""
	stopExpr := ""
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "limit") {
			orderType = "LIMIT"
			limitExpr = strings.TrimSpace(value)
		}
		if strings.EqualFold(key, "stop") {
			stopExpr = strings.TrimSpace(value)
		}
	}
	return orderType, limitExpr, stopExpr
}

func pineEquityPercent(value string) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(value))
	match := equityQuantityPattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func stripWrappingParens(value string) string {
	result := strings.TrimSpace(value)
	for len(result) >= 2 && result[0] == '(' && result[len(result)-1] == ')' && wrappingParensCoverExpression(result) {
		result = strings.TrimSpace(result[1 : len(result)-1])
	}
	return result
}

func wrappingParensCoverExpression(value string) bool {
	depth := 0
	inString := byte(0)
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
			switch inString {
			case 0:
				inString = ch
			case ch:
				inString = 0
			}
			continue
		}
		if inString != 0 {
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && index < len(value)-1 {
				return false
			}
		}
	}
	return depth == 0
}

func splitNamedArg(value string) (string, string, bool) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func callArgs(line string) string {
	open := strings.Index(line, "(")
	if open < 0 {
		return ""
	}
	close := matchingParen(line, open)
	if close < 0 {
		return line[open+1:]
	}
	return line[open+1 : close]
}

func matchingParen(value string, open int) int {
	depth := 0
	inString := byte(0)
	for index := open; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
			switch inString {
			case 0:
				inString = ch
			case ch:
				inString = 0
			}
			continue
		}
		if inString != 0 {
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitArguments(value string) []string {
	parts := []string{}
	start := 0
	depth := 0
	inString := byte(0)
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
			switch inString {
			case 0:
				inString = ch
			case ch:
				inString = 0
			}
			continue
		}
		if inString != 0 {
			continue
		}
		if ch == '(' || ch == '[' {
			depth++
		} else if ch == ')' || ch == ']' {
			depth--
		} else if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	tail := strings.TrimSpace(value[start:])
	if tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func firstStringArgument(line string) string {
	args := splitArguments(callArgs(line))
	if len(args) == 0 {
		return ""
	}
	return unquote(args[0])
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' || first == '\'') && first == last {
			return value[1 : len(value)-1]
		}
	}
	return value
}
