package pine

import (
	"regexp"
	"strings"
)

func tupleNamesFromASTLine(line ASTLine) []string {
	match := generalTuplePattern.FindStringSubmatch(line.Text)
	if match == nil {
		return nil
	}
	names := splitArguments(match[1])
	for index := range names {
		names[index] = strings.TrimSpace(names[index])
	}
	return names
}

func duplicateNames(names []string) []string {
	seen := map[string]bool{}
	duplicates := make([]string, 0)
	for _, name := range names {
		if name == "_" {
			continue
		}
		if seen[name] {
			duplicates = append(duplicates, name)
			continue
		}
		seen[name] = true
	}
	return duplicates
}

func semanticTupleReturnCount(expression string) (int, bool) {
	trimmed := strings.TrimSpace(expression)
	if args, ok := requestSecurityCallArgs(trimmed); ok {
		if lowered, tupleOK := lowerSupportedRequestSecurityTupleGeneral(args); tupleOK {
			return len(lowered), true
		}
	}
	if len(trimmed) >= 2 && trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']' {
		values := splitArguments(trimmed[1 : len(trimmed)-1])
		return len(values), len(values) >= 2 && len(values) <= 8
	}
	name, args, ok := parseTACall(trimmed)
	if !ok {
		if lowered := replaceSupportedRequestSecurity(trimmed); lowered != trimmed {
			lowered = stripWrappingParens(lowered)
			if callName, callArgs, callOK := parseFunctionCallText(lowered); callOK {
				name, args, ok = callName, callArgs, true
			}
		}
	}
	if !ok {
		return 0, false
	}
	switch strings.ToLower(name) {
	case "macd", "bb", "dmi", "kc", "bollinger":
		return 3, true
	case "supertrend":
		return 2, true
	default:
		return len(args), false
	}
}

func inferTupleElementKind(expression string) SemanticValueKind {
	if strings.Contains(strings.ToLower(expression), "request.security(") {
		return SemanticValueSeries
	}
	return inferSemanticValueKind(expression)
}

func inferSemanticValueKind(expression string) SemanticValueKind {
	trimmed := strings.TrimSpace(stripWrappingParens(expression))
	if trimmed == "" {
		return SemanticValueUnknown
	}
	if isSimpleAliasExpression(trimmed) {
		return SemanticValueConst
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "ta.") ||
		strings.Contains(lower, "request.security(") ||
		historyReferencePattern.MatchString(trimmed) ||
		containsSeriesIdentifier(trimmed) {
		if strings.Contains(lower, "ta.macd(") || strings.Contains(lower, "ta.bb(") ||
			strings.Contains(lower, "ta.supertrend(") || strings.Contains(lower, "ta.kc(") ||
			strings.Contains(lower, "ta.dmi(") {
			return SemanticValueObject
		}
		return SemanticValueSeries
	}
	if strings.Contains(lower, "input.") || strings.Contains(lower, "timestamp(") || strings.Contains(lower, "math.") {
		return SemanticValueSimple
	}
	return SemanticValueUnknown
}

func containsSeriesIdentifier(expression string) bool {
	for _, source := range []string{"open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4", "time", "bar_index"} {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(source) + `\b`)
		if pattern.MatchString(expression) {
			return true
		}
	}
	return false
}

type semanticCall struct {
	name     string
	argCount int
}

func semanticFunctionCalls(text string) []semanticCall {
	calls := make([]semanticCall, 0)
	index := 0
	for index < len(text) {
		open := strings.Index(text[index:], "(")
		if open < 0 {
			break
		}
		open += index
		nameStart := open
		for nameStart > 0 {
			ch := text[nameStart-1]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
				nameStart--
				continue
			}
			break
		}
		name := strings.ToLower(strings.TrimSpace(text[nameStart:open]))
		close := matchingParen(text, open)
		if close < 0 {
			break
		}
		if name != "" && strings.Contains(name, ".") {
			args := splitArguments(text[open+1 : close])
			calls = append(calls, semanticCall{name: name, argCount: len(args)})
		}
		index = close + 1
	}
	return calls
}

func parseFunctionCallText(expression string) (string, []string, bool) {
	trimmed := strings.TrimSpace(expression)
	open := strings.Index(trimmed, "(")
	if open <= 0 {
		return "", nil, false
	}
	close := matchingParen(trimmed, open)
	if close != len(trimmed)-1 {
		return "", nil, false
	}
	return strings.ToLower(strings.TrimSpace(trimmed[:open])), splitArguments(trimmed[open+1 : close]), true
}
