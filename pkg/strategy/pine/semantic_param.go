package pine

import "strings"

func parseMethodDeclaration(value string) (string, *SemanticParameter, []SemanticParameter) {
	trimmed := strings.TrimSpace(value)
	open := strings.Index(trimmed, "(")
	if open < 0 {
		return firstDeclarationName(trimmed), nil, nil
	}
	name := firstDeclarationName(trimmed[:open])
	close := matchingParen(trimmed, open)
	if close < 0 {
		return name, nil, nil
	}
	parameters := parseSemanticParameters(trimmed[open+1 : close])
	if len(parameters) == 0 {
		return name, nil, nil
	}
	return name, new(parameters[0]), parameters
}

func parseSemanticParameters(value string) []SemanticParameter {
	args := splitSemanticParameterArguments(value)
	parameters := make([]SemanticParameter, 0, len(args))
	for _, arg := range args {
		parameter := parseSemanticParameter(arg)
		if parameter.Name == "" && parameter.Type == "" {
			continue
		}
		parameters = append(parameters, parameter)
	}
	return parameters
}

func splitSemanticParameterArguments(value string) []string {
	parts := []string{}
	start := 0
	depth := 0
	angleDepth := 0
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
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case '<':
			if angleDepth > 0 || isCollectionGenericOpen(value, index) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if depth == 0 && angleDepth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	tail := strings.TrimSpace(value[start:])
	if tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func isCollectionGenericOpen(value string, index int) bool {
	start := index
	for start > 0 {
		ch := value[start-1]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			start--
			continue
		}
		break
	}
	switch strings.ToLower(value[start:index]) {
	case "array", "map", "matrix":
		return true
	default:
		return false
	}
}

func parseSemanticParameter(value string) SemanticParameter {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return SemanticParameter{}
	}
	defaultValue := ""
	if index := strings.Index(cleaned, "="); index >= 0 {
		defaultValue = strings.TrimSpace(cleaned[index+1:])
		cleaned = strings.TrimSpace(cleaned[:index])
	}
	fields := strings.Fields(cleaned)
	switch len(fields) {
	case 0:
		return SemanticParameter{}
	case 1:
		return SemanticParameter{Name: strings.Trim(fields[0], ","), Default: defaultValue}
	default:
		return SemanticParameter{
			Type:    strings.Trim(strings.Join(fields[:len(fields)-1], " "), ","),
			Name:    strings.Trim(fields[len(fields)-1], ","),
			Default: defaultValue,
		}
	}
}

func semanticTypeField(value string) (SemanticParameter, bool) {
	field := parseSemanticParameter(value)
	if field.Type == "" || field.Name == "" {
		return SemanticParameter{}, false
	}
	return field, true
}

func importVersion(importPath string) string {
	parts := strings.Split(strings.TrimSpace(importPath), "/")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	for _, ch := range last {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return last
}

func firstDeclarationName(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	name := strings.Trim(fields[0], "()")
	if index := strings.Index(name, "("); index >= 0 {
		name = name[:index]
	}
	return name
}
