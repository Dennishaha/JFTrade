package pine

import (
	"fmt"
	"strconv"
	"strings"
)

func replaceTAFunction(expression string, name string, template string) string {
	prefix := "ta." + name + "("
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
		replacement := template
		if strings.Contains(template, "${period}") {
			period := "14"
			if len(args) == 1 {
				period = args[0]
			} else if len(args) >= 2 {
				period = args[1]
			}
			replacement = strings.ReplaceAll(replacement, "${period}", strings.TrimSpace(period))
		}
		if strings.Contains(template, "${left}") {
			left, right, third := "close", "close", "close"
			if len(args) >= 1 {
				left = strings.TrimSpace(args[0])
			}
			if len(args) >= 2 {
				right = strings.TrimSpace(args[1])
			}
			if len(args) >= 3 {
				third = strings.TrimSpace(args[2])
			}
			replacement = strings.ReplaceAll(replacement, "${left}", left)
			replacement = strings.ReplaceAll(replacement, "${right}", right)
			replacement = strings.ReplaceAll(replacement, "${third}", third)
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAMacd(expression string) string {
	prefix := "ta.macd("
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
		replacement := "macd(12, 26, 9)"
		if len(args) >= 4 {
			replacement = fmt.Sprintf("macd(%s, %s, %s)", strings.TrimSpace(args[1]), strings.TrimSpace(args[2]), strings.TrimSpace(args[3]))
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAMovingAverageFunction(expression string, name string, averageType string) string {
	prefix := "ta." + name + "("
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
		source, period := pineSourceLengthArgs(args, "close", "20")
		replacement := fmt.Sprintf("ma(%s, %s)", averageType, period)
		if source != "close" {
			replacement = fmt.Sprintf("ma(%s, %s, %s)", averageType, period, source)
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTASourceLengthFunction(expression string, name string, target string, defaultSource string, defaultPeriod string) string {
	prefix := "ta." + name + "("
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
		source, period := pineSourceLengthArgs(args, defaultSource, defaultPeriod)
		replacement := fmt.Sprintf("%s(%s, %s)", target, source, period)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTASourceOptionalFunction(expression string, name string, target string, defaultSource string) string {
	prefix := "ta." + name + "("
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
		if len(args) > 1 {
			return expression
		}
		source := defaultSource
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			source = strings.TrimSpace(args[0])
		}
		replacement := fmt.Sprintf("%s(%s)", target, source)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAAnchoredVWAP(expression string) string {
	const prefix = "ta.vwap("
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
		if len(args) != 2 {
			return expression
		}
		call, anchorArgs, ok := parseFunctionCallText(strings.TrimSpace(args[1]))
		if !ok || !strings.EqualFold(call, "timeframe.change") || len(anchorArgs) != 1 {
			return expression
		}
		timeUnit, ok := pineTimeframeUnit(unquote(strings.TrimSpace(anchorArgs[0])))
		if !ok || (timeUnit != "day" && timeUnit != "week" && timeUnit != "month") {
			return expression
		}
		source := strings.TrimSpace(args[0])
		if source == "" {
			source = "hlc3"
		}
		replacement := fmt.Sprintf("anchored_vwap(%s, %s)", source, timeUnit)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTASourceRequiredFunction(expression string, name string, target string) string {
	prefix := "ta." + name + "("
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
		if len(args) != 1 {
			return expression
		}
		replacement := fmt.Sprintf("%s(%s)", target, strings.TrimSpace(args[0]))
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAStateFunction(expression string, name string) string {
	prefix := "ta." + name + "("
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
		replacement := fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAExtremaBarsFunction(expression string, name string) string {
	prefix := "ta." + name + "("
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
		if len(args) != 2 {
			return expression
		}
		replacement := fmt.Sprintf("%s(%s, %s)", name, strings.TrimSpace(args[0]), strings.TrimSpace(args[1]))
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTAStoch(expression string) string {
	prefix := "ta.stoch("
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
		if len(args) != 4 {
			return expression
		}
		replacement := fmt.Sprintf("stoch(%s, %s, %s, %s)",
			strings.TrimSpace(args[0]),
			strings.TrimSpace(args[1]),
			strings.TrimSpace(args[2]),
			strings.TrimSpace(args[3]),
		)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceTATr(expression string) string {
	for {
		lower := strings.ToLower(expression)
		start := strings.Index(lower, "ta.tr(")
		if start < 0 {
			break
		}
		open := start + len("ta.tr(") - 1
		close := matchingParen(expression, open)
		if close < 0 {
			break
		}
		expression = expression[:start] + "tr()" + expression[close+1:]
	}
	return taTRPattern.ReplaceAllString(expression, "tr()")
}

func replaceTAWindowFunction(expression string, name string) string {
	prefix := "ta." + name + "("
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
		source, period := pineWindowFunctionArgs(name, args)
		replacement := fmt.Sprintf("%s(%s, %s)", name, source, period)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func pineWindowFunctionArgs(name string, args []string) (string, string) {
	defaultSource := "close"
	if name == "highest" {
		defaultSource = "high"
	}
	if name == "lowest" {
		defaultSource = "low"
	}
	defaultPeriod := "1"
	switch name {
	case "highest", "lowest", "mom", "roc", "rising", "falling":
		defaultPeriod = "14"
	}
	if len(args) == 0 {
		return defaultSource, defaultPeriod
	}
	if len(args) == 1 {
		if name == "highest" || name == "lowest" {
			return defaultSource, strings.TrimSpace(args[0])
		}
		return strings.TrimSpace(args[0]), defaultPeriod
	}
	return strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
}

func pineSourceLengthArgs(args []string, defaultSource string, defaultPeriod string) (string, string) {
	if len(args) == 0 {
		return defaultSource, defaultPeriod
	}
	if len(args) == 1 {
		return defaultSource, strings.TrimSpace(args[0])
	}
	return strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
}

func parseTACall(expression string) (string, []string, bool) {
	lower := strings.ToLower(expression)
	if !strings.HasPrefix(lower, "ta.") {
		return "", nil, false
	}
	open := strings.Index(expression, "(")
	if open <= len("ta.") {
		return "", nil, false
	}
	close := matchingParen(expression, open)
	if close != len(expression)-1 {
		return "", nil, false
	}
	name := strings.ToLower(strings.TrimSpace(expression[len("ta."):open]))
	return name, splitArguments(expression[open+1 : close]), true
}

func pineMovingAverageType(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ema":
		return "EMA", true
	case "sma":
		return "SMA", true
	case "rma":
		return "SMMA", true
	case "wma":
		return "LWMA", true
	case "hma":
		return "HMA", true
	case "vwma":
		return "VWMA", true
	default:
		return "", false
	}
}

func pineTimeframeUnit(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	switch strings.ToUpper(trimmed) {
	case "1":
		return "minute", true
	case "5", "15", "30", "45", "120", "240":
		return strconv.Quote(trimmed + "m"), true
	case "60":
		return "hour", true
	case "D", "1D":
		return "day", true
	case "W", "1W":
		return "week", true
	case "M", "1M":
		return "month", true
	default:
		return "", false
	}
}

func isOHLCVSource(expression string) bool {
	switch strings.TrimSpace(expression) {
	case "open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4":
		return true
	default:
		return false
	}
}
