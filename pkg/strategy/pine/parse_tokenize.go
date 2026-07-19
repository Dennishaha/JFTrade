package pine

import (
	"strings"
)

func tokenizeScript(script string) []parsedLine {
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]parsedLine, 0, len(rawLines))
	for index, rawLine := range rawLines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(strings.ToLower(trimmed), "//@version") && !strings.Contains(trimmed, "@jftradeFlow") {
			continue
		}
		code := stripInlineComment(trimmed)
		if code == "" && !strings.HasPrefix(strings.ToLower(trimmed), "//@version") {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " \t"))
		lines = append(lines, parsedLine{number: index + 1, raw: rawLine, trimmed: code, indent: indent})
	}
	return lines
}

func stripInlineComment(line string) string {
	inString := byte(0)
	for index := 0; index+1 < len(line); index++ {
		ch := line[index]
		if (ch == '"' || ch == '\'') && (index == 0 || line[index-1] != '\\') {
			switch inString {
			case 0:
				inString = ch
			case ch:
				inString = 0
			}
			continue
		}
		if inString == 0 && ch == '/' && line[index+1] == '/' && !strings.HasPrefix(strings.ToLower(line), "//@version") {
			return strings.TrimSpace(line[:index])
		}
	}
	return strings.TrimSpace(line)
}
