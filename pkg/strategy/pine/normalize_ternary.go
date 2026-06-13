package pine

import "strings"

func normalizeTernaryExpression(expression string) string {
	question, colon := topLevelTernaryIndexes(expression)
	if question < 0 || colon < 0 {
		return expression
	}
	condition := strings.TrimSpace(expression[:question])
	whenTrue := strings.TrimSpace(expression[question+1 : colon])
	whenFalse := strings.TrimSpace(expression[colon+1:])
	if condition == "" || whenTrue == "" || whenFalse == "" {
		return expression
	}
	return "ifelse(" + condition + ", " + whenTrue + ", " + whenFalse + ")"
}

func topLevelTernaryIndexes(expression string) (int, int) {
	depth := 0
	inString := byte(0)
	question := -1
	for index := 0; index < len(expression); index++ {
		ch := expression[index]
		if (ch == '"' || ch == '\'') && (index == 0 || expression[index-1] != '\\') {
			if inString == 0 {
				inString = ch
			} else if inString == ch {
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
		case '?':
			if depth == 0 && question < 0 {
				question = index
			}
		case ':':
			if depth == 0 && question >= 0 {
				return question, index
			}
		}
	}
	return -1, -1
}
