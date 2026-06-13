package pine

import (
	"regexp"
	"strings"
)

func (s *parseState) resolveSourceAliases(expression string) string {
	if s == nil || len(s.sourceAliases) == 0 {
		return expression
	}
	result := expression
	for alias, source := range s.sourceAliases {
		result = s.cachedRegexp("word:"+alias, `\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, source)
	}
	return result
}

func (s *parseState) resolveValueAliases(expression string) string {
	if s == nil || len(s.valueAliases) == 0 {
		return expression
	}
	result := expression
	for alias, value := range s.valueAliases {
		result = s.cachedRegexp("word:"+alias, `\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, value)
	}
	return result
}

func isSimpleAliasExpression(expression string) bool {
	trimmed := strings.TrimSpace(expression)
	if isOHLCVSource(trimmed) {
		return true
	}
	if trimmed == "true" || trimmed == "false" || trimmed == "na" {
		return true
	}
	if numberPattern.MatchString(trimmed) {
		return true
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		return true
	}
	return false
}
