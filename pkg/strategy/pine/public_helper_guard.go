package pine

import (
	"fmt"
	"strings"
)

var publicDisabledHelperNames = map[string]string{
	"alma":                            "ta.alma",
	"anchored_vwap":                   "ta.vwap(source, timeframe.change(...))",
	"atr":                             "ta.atr",
	"bbw":                             "ta.bbw",
	"bollinger":                       "ta.bb",
	"barssince":                       "ta.barssince",
	"cci":                             "ta.cci",
	"change":                          "ta.change",
	"cmo":                             "ta.cmo",
	"cog":                             "ta.cog",
	"correlation":                     "ta.correlation",
	"cum":                             "ta.cum",
	"dev":                             "ta.dev",
	"dmi":                             "ta.dmi",
	"falling":                         "ta.falling",
	"highest":                         "ta.highest",
	"highestbars":                     "ta.highestbars",
	"history":                         "series[n]",
	"ifelse":                          "condition ? valueWhenTrue : valueWhenFalse",
	"kc":                              "ta.kc",
	"kcw":                             "ta.kcw",
	"kdj":                             "ta.stoch plus Pine smoothing",
	"linreg":                          "ta.linreg",
	"lowest":                          "ta.lowest",
	"lowestbars":                      "ta.lowestbars",
	"ma":                              "ta.sma/ta.ema/ta.rma/ta.wma/ta.hma/ta.vwma",
	"macd":                            "ta.macd",
	"median":                          "ta.median",
	"mfi":                             "ta.mfi",
	"mode":                            "ta.mode",
	"mom":                             "ta.mom",
	"notify":                          "alert",
	"obv":                             "ta.obv",
	"percentile_linear_interpolation": "ta.percentile_linear_interpolation",
	"percentile_nearest_rank":         "ta.percentile_nearest_rank",
	"percentrank":                     "ta.percentrank",
	"pivothigh":                       "ta.pivothigh",
	"pivotlow":                        "ta.pivotlow",
	"previous":                        "series[1]",
	"range":                           "ta.range",
	"rising":                          "ta.rising",
	"roc":                             "ta.roc",
	"rsi":                             "ta.rsi",
	"sar":                             "ta.sar",
	"security_source":                 "request.security",
	"stdev":                           "ta.stdev",
	"stoch":                           "ta.stoch",
	"sum":                             "ta.sum",
	"supertrend":                      "ta.supertrend",
	"swma":                            "ta.swma",
	"tr":                              "ta.tr",
	"tsi":                             "ta.tsi",
	"variance":                        "ta.variance",
	"valuewhen":                       "ta.valuewhen",
	"vwap":                            "ta.vwap",
	"williams_r":                      "ta.wpr",
	"williamsr":                       "ta.wpr",
	"cross_over":                      "ta.crossover",
	"cross_under":                     "ta.crossunder",
}

var publicDisabledTAHelperNames = map[string]string{
	"adx": "ta.dmi(diLength, adxSmoothing).adx",
}

func rejectPublicDisabledHelperCalls(lineNumber int, expression string) error {
	scan := stripStringLiteralsForHelperScan(expression)
	for index := 0; index < len(scan); {
		if !isHelperIdentifierStart(scan[index]) {
			index++
			continue
		}

		start := index
		index++
		for index < len(scan) && isHelperIdentifierChar(scan[index]) {
			index++
		}

		callIndex := index
		for callIndex < len(scan) && isHelperWhitespace(scan[callIndex]) {
			callIndex++
		}
		if callIndex >= len(scan) || scan[callIndex] != '(' {
			continue
		}
		if start > 0 && scan[start-1] == '.' {
			namespaceStart := start - 2
			for namespaceStart >= 0 && isHelperIdentifierChar(scan[namespaceStart]) {
				namespaceStart--
			}
			namespace := strings.ToLower(scan[namespaceStart+1 : start-1])
			name := strings.ToLower(scan[start:index])
			if namespace == "ta" {
				if replacement, disabled := publicDisabledTAHelperNames[name]; disabled {
					return fmt.Errorf("pine line %d: ta.%s() is a JFTrade-only shortcut; use Pine v6 %s instead", lineNumber, name, replacement)
				}
			}
			continue
		}

		name := strings.ToLower(scan[start:index])
		if replacement, disabled := publicDisabledHelperNames[name]; disabled {
			return fmt.Errorf("pine line %d: %s() is an internal JFTrade helper; use Pine v6 %s instead", lineNumber, name, replacement)
		}
	}
	return nil
}

func isHelperIdentifierStart(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_'
}

func isHelperIdentifierChar(char byte) bool {
	return isHelperIdentifierStart(char) || (char >= '0' && char <= '9')
}

func isHelperWhitespace(char byte) bool {
	switch char {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func stripStringLiteralsForHelperScan(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	inString := rune(0)
	escaped := false
	for _, char := range value {
		if inString != 0 {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == inString {
				inString = 0
			}
			builder.WriteRune(' ')
			continue
		}
		if char == '"' || char == '\'' {
			inString = char
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}
