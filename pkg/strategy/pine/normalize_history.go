package pine

import "strings"

func normalizeHistoryReferences(expression string) string {
	return rewriteOutsideStringLiterals(expression, func(segment string) string {
		return historyReferencePattern.ReplaceAllStringFunc(segment, func(match string) string {
			parts := historyReferencePattern.FindStringSubmatch(match)
			if len(parts) != 3 {
				return match
			}
			return "history(" + strings.TrimSpace(parts[1]) + ", " + strings.TrimSpace(parts[2]) + ")"
		})
	})
}

func rewriteOutsideStringLiterals(expression string, rewrite func(string) string) string {
	if expression == "" {
		return expression
	}
	var builder strings.Builder
	segmentStart := 0
	inString := byte(0)
	for index := 0; index < len(expression); index++ {
		ch := expression[index]
		if (ch != '"' && ch != '\'') || (index > 0 && expression[index-1] == '\\') {
			continue
		}
		if inString == 0 {
			builder.WriteString(rewrite(expression[segmentStart:index]))
			inString = ch
			segmentStart = index
			continue
		}
		if inString == ch {
			builder.WriteString(expression[segmentStart : index+1])
			inString = 0
			segmentStart = index + 1
		}
	}
	if inString == 0 {
		builder.WriteString(rewrite(expression[segmentStart:]))
	} else {
		builder.WriteString(expression[segmentStart:])
	}
	return builder.String()
}

func historyReferenceMatchesOutsideStringLiterals(expression string) [][]string {
	matches := make([][]string, 0)
	rewriteOutsideStringLiterals(expression, func(segment string) string {
		matches = append(matches, historyReferencePattern.FindAllStringSubmatch(segment, -1)...)
		return segment
	})
	return matches
}
