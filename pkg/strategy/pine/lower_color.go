package pine

import (
	"fmt"
	"strconv"
	"strings"
)

func replaceColorFunctions(expression string) string {
	expression = replaceColorNewFunction(expression)
	return replaceColorRGBFunction(expression)
}

func replaceColorNewFunction(expression string) string {
	prefix := "color.new("
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
		args := splitArguments(expression[open+1 : close])
		replacement := "\"#000000\""
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			replacement = strings.TrimSpace(args[0])
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceColorRGBFunction(expression string) string {
	prefix := "color.rgb("
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
		args := splitArguments(expression[open+1 : close])
		replacement := "\"#000000\""
		if len(args) >= 3 {
			if red, redOK := parsePineColorComponent(args[0]); redOK {
				if green, greenOK := parsePineColorComponent(args[1]); greenOK {
					if blue, blueOK := parsePineColorComponent(args[2]); blueOK {
						replacement = fmt.Sprintf("\"#%02x%02x%02x\"", red, green, blue)
					}
				}
			}
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func parsePineColorComponent(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	if parsed < 0 {
		parsed = 0
	}
	if parsed > 255 {
		parsed = 255
	}
	return parsed, true
}
