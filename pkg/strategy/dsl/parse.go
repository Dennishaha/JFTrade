package dsl

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const sourceFormatDSLV1 = "dsl-v1"

type parsedLine struct {
	number  int
	raw     string
	trimmed string
	indent  int
}

func ParseScript(script string) (*strategyir.Program, error) {
	script = strings.ReplaceAll(script, "\r\n", "\n")
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("strategy script is required")
	}

	lines := tokenizeScript(script)
	if len(lines) == 0 {
		return nil, fmt.Errorf("strategy script is required")
	}

	program := &strategyir.Program{SourceFormat: sourceFormatDSLV1}
	seenHooks := map[string]bool{}
	seenMetadata := map[string]bool{}

	for index := 0; index < len(lines); {
		line := lines[index]
		if line.indent > 0 {
			return nil, fmt.Errorf("dsl line %d: indented statement must be inside a hook block", line.number)
		}

		switch {
		case strings.HasPrefix(line.trimmed, "strategy "):
			if seenMetadata["strategy"] {
				return nil, fmt.Errorf("dsl line %d: duplicate strategy declaration", line.number)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "strategy "))
			if name == "" {
				return nil, fmt.Errorf("dsl line %d: strategy name is required", line.number)
			}
			program.Metadata.Name = name
			seenMetadata["strategy"] = true
			index++
		case strings.HasPrefix(line.trimmed, "version "):
			if seenMetadata["version"] {
				return nil, fmt.Errorf("dsl line %d: duplicate version declaration", line.number)
			}
			version := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "version "))
			if version == "" {
				return nil, fmt.Errorf("dsl line %d: version is required", line.number)
			}
			program.Metadata.Version = version
			seenMetadata["version"] = true
			index++
		case strings.HasPrefix(line.trimmed, "symbol "):
			if seenMetadata["symbol"] {
				return nil, fmt.Errorf("dsl line %d: duplicate symbol declaration", line.number)
			}
			symbol := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "symbol "))
			if symbol == "" {
				return nil, fmt.Errorf("dsl line %d: symbol is required", line.number)
			}
			program.Metadata.Symbol = symbol
			seenMetadata["symbol"] = true
			index++
		case strings.HasPrefix(line.trimmed, "interval "):
			if seenMetadata["interval"] {
				return nil, fmt.Errorf("dsl line %d: duplicate interval declaration", line.number)
			}
			interval := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "interval "))
			if interval == "" {
				return nil, fmt.Errorf("dsl line %d: interval is required", line.number)
			}
			program.Metadata.Interval = interval
			seenMetadata["interval"] = true
			index++
		case line.trimmed == hookInit || line.trimmed == hookKLineClose:
			if seenHooks[line.trimmed] {
				return nil, fmt.Errorf("dsl line %d: duplicate hook %q", line.number, line.trimmed)
			}
			seenHooks[line.trimmed] = true

			statements, nextIndex, err := parseBlock(lines, index+1, line.indent)
			if err != nil {
				return nil, err
			}
			if len(statements) == 0 {
				return nil, fmt.Errorf("dsl line %d: hook %q requires at least one indented statement", line.number, line.trimmed)
			}

			program.Hooks = append(program.Hooks, strategyir.HookBlock{
				Kind:       readHookKind(line.trimmed),
				Range:      strategyir.SourceRange{StartLine: line.number, EndLine: endLineOfStatements(statements, line.number)},
				Statements: statements,
			})
			index = nextIndex
		default:
			return nil, fmt.Errorf("dsl line %d: unsupported top-level statement %q", line.number, line.trimmed)
		}
	}

	if len(program.Hooks) == 0 {
		return nil, fmt.Errorf("dsl strategy requires at least one hook block")
	}

	return program, nil
}

func tokenizeScript(script string) []parsedLine {
	rawLines := strings.Split(script, "\n")
	lines := make([]parsedLine, 0, len(rawLines))
	for index, rawLine := range rawLines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(rawLine) - len(strings.TrimLeft(rawLine, " \t"))
		lines = append(lines, parsedLine{
			number:  index + 1,
			raw:     rawLine,
			trimmed: trimmed,
			indent:  indent,
		})
	}
	return lines
}

func parseBlock(lines []parsedLine, startIndex int, parentIndent int) ([]strategyir.Statement, int, error) {
	if startIndex >= len(lines) || lines[startIndex].indent <= parentIndent {
		return nil, startIndex, nil
	}

	blockIndent := lines[startIndex].indent
	statements := make([]strategyir.Statement, 0)

	for index := startIndex; index < len(lines); {
		line := lines[index]
		if line.indent <= parentIndent {
			return statements, index, nil
		}
		if line.indent < blockIndent {
			return nil, index, fmt.Errorf("dsl line %d: inconsistent indentation inside block", line.number)
		}
		if line.indent > blockIndent {
			return nil, index, fmt.Errorf("dsl line %d: unexpected indentation", line.number)
		}

		statement, nextIndex, err := parseStatement(lines, index, blockIndent)
		if err != nil {
			return nil, index, err
		}
		statements = append(statements, statement)
		index = nextIndex
	}

	return statements, len(lines), nil
}

func parseStatement(lines []parsedLine, index int, currentIndent int) (strategyir.Statement, int, error) {
	line := lines[index]
	trimmed := line.trimmed

	switch {
	case strings.HasPrefix(trimmed, "let "):
		statement, err := parseLetStatement(line)
		if err != nil {
			return nil, index, err
		}
		return statement, index + 1, nil
	case strings.HasPrefix(trimmed, "log "):
		return &strategyir.LogStmt{
			Range:   strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Message: normalizeLiteral(strings.TrimSpace(strings.TrimPrefix(trimmed, "log "))),
		}, index + 1, nil
	case strings.HasPrefix(trimmed, "notify "):
		return &strategyir.NotifyStmt{
			Range:   strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Message: normalizeLiteral(strings.TrimSpace(strings.TrimPrefix(trimmed, "notify "))),
		}, index + 1, nil
	case hasOrderPrefix(trimmed):
		statement, err := parseOrderStatement(line)
		if err != nil {
			return nil, index, err
		}
		return statement, index + 1, nil
	case strings.HasPrefix(trimmed, "protect "):
		statement, err := parseProtectStatement(line)
		if err != nil {
			return nil, index, err
		}
		return statement, index + 1, nil
	case strings.HasPrefix(trimmed, "if ") && strings.HasSuffix(trimmed, ":"):
		condition := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "if "), ":"))
		if condition == "" {
			return nil, index, fmt.Errorf("dsl line %d: if condition is required", line.number)
		}
		if err := validateExpression(line.number, "if condition", condition); err != nil {
			return nil, index, err
		}
		thenStatements, nextIndex, err := parseBlock(lines, index+1, currentIndent)
		if err != nil {
			return nil, index, err
		}
		if len(thenStatements) == 0 {
			return nil, index, fmt.Errorf("dsl line %d: if branch requires at least one indented statement", line.number)
		}

		statement := &strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: line.number, EndLine: endLineOfStatements(thenStatements, line.number)},
			Condition: condition,
			Then:      thenStatements,
		}

		if nextIndex < len(lines) && lines[nextIndex].indent == currentIndent && lines[nextIndex].trimmed == "else:" {
			elseLine := lines[nextIndex]
			elseStatements, afterElseIndex, elseErr := parseBlock(lines, nextIndex+1, currentIndent)
			if elseErr != nil {
				return nil, index, elseErr
			}
			if len(elseStatements) == 0 {
				return nil, index, fmt.Errorf("dsl line %d: else branch requires at least one indented statement", elseLine.number)
			}
			statement.Else = elseStatements
			statement.Range.EndLine = endLineOfStatements(elseStatements, elseLine.number)
			return statement, afterElseIndex, nil
		}

		return statement, nextIndex, nil
	case trimmed == "else:":
		return nil, index, fmt.Errorf("dsl line %d: else must follow an if block", line.number)
	default:
		return nil, index, fmt.Errorf("dsl line %d: unsupported statement %q", line.number, trimmed)
	}
}

func parseLetStatement(line parsedLine) (strategyir.Statement, error) {
	body := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "let "))
	parts := strings.SplitN(body, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("dsl line %d: let statement must use name = expression", line.number)
	}
	name := strings.TrimSpace(parts[0])
	expression := strings.TrimSpace(parts[1])
	if name == "" {
		return nil, fmt.Errorf("dsl line %d: let variable name is required", line.number)
	}
	if expression == "" {
		return nil, fmt.Errorf("dsl line %d: let expression is required", line.number)
	}
	if err := validateExpression(line.number, "let expression", expression); err != nil {
		return nil, err
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       name,
		Expression: expression,
	}, nil
}

func parseOrderStatement(line parsedLine) (strategyir.Statement, error) {
	parts := strings.Fields(line.trimmed)
	if len(parts) < 3 {
		return nil, fmt.Errorf("dsl line %d: order statement requires action, quantity mode, and quantity expression", line.number)
	}

	action := strategyir.OrderAction(parts[0])
	quantityMode := parts[1]
	remaining := parts[2:]
	quantityTokens, options, err := splitOrderExpressionAndOptions(remaining)
	if err != nil {
		return nil, fmt.Errorf("dsl line %d: %w", line.number, err)
	}
	if len(quantityTokens) == 0 {
		return nil, fmt.Errorf("dsl line %d: order quantity expression is required", line.number)
	}
	quantityExpression := strings.Join(quantityTokens, " ")
	if err := validateExpression(line.number, "order quantity expression", quantityExpression); err != nil {
		return nil, err
	}
	if limitExpression := options["limit"]; limitExpression != "" {
		if err := validateExpression(line.number, "order limit expression", limitExpression); err != nil {
			return nil, err
		}
	}

	statement := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Action:             action,
		QuantityMode:       quantityMode,
		QuantityExpression: quantityExpression,
		EntryPolicy:        options["policy"],
		OrderType:          options["type"],
		LimitExpression:    options["limit"],
	}
	if statement.OrderType == "" {
		if statement.LimitExpression != "" {
			statement.OrderType = "LIMIT"
		} else {
			statement.OrderType = "MARKET"
		}
	}
	return statement, nil
}

func parseProtectStatement(line parsedLine) (strategyir.Statement, error) {
	parts := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line.trimmed, "protect ")))
	if len(parts) < 5 {
		return nil, fmt.Errorf("dsl line %d: protect statement requires direction, mode, time value, time unit, and percentage", line.number)
	}

	statement := &strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Direction:            parts[0],
		Mode:                 parts[1],
		TimeValueExpression:  parts[2],
		TimeUnit:             parts[3],
		PercentageExpression: parts[4],
		WindowPolicy:         "continuous",
	}

	for index := 5; index < len(parts); {
		if parts[index] != "window" {
			return nil, fmt.Errorf("dsl line %d: unsupported protect option %q", line.number, parts[index])
		}
		if index+1 >= len(parts) {
			return nil, fmt.Errorf("dsl line %d: protect option %q requires a value", line.number, parts[index])
		}
		statement.WindowPolicy = parts[index+1]
		index += 2
	}

	return statement, nil
}

func splitOrderExpressionAndOptions(parts []string) ([]string, map[string]string, error) {
	options := map[string]string{}
	quantityTokens := make([]string, 0, len(parts))
	index := 0

	for index < len(parts) && !isOrderOptionKeyword(parts[index]) {
		quantityTokens = append(quantityTokens, parts[index])
		index++
	}

	for index < len(parts) {
		key := parts[index]
		if !isOrderOptionKeyword(key) {
			return nil, nil, fmt.Errorf("unsupported order option %q", key)
		}
		if index+1 >= len(parts) {
			return nil, nil, fmt.Errorf("order option %q requires a value", key)
		}
		options[key] = parts[index+1]
		index += 2
	}

	return quantityTokens, options, nil
}

func isOrderOptionKeyword(value string) bool {
	switch value {
	case "policy", "limit", "type":
		return true
	default:
		return false
	}
}

func hasOrderPrefix(value string) bool {
	return strings.HasPrefix(value, string(strategyir.OrderActionBuy)+" ") ||
		strings.HasPrefix(value, string(strategyir.OrderActionSell)+" ") ||
		strings.HasPrefix(value, string(strategyir.OrderActionShort)+" ") ||
		strings.HasPrefix(value, string(strategyir.OrderActionCover)+" ")
}

func readHookKind(value string) strategyir.HookKind {
	if value == hookInit {
		return strategyir.HookInit
	}
	return strategyir.HookKLineClose
}

func endLineOfStatements(statements []strategyir.Statement, fallback int) int {
	if len(statements) == 0 {
		return fallback
	}
	return statements[len(statements)-1].SourceRange().EndLine
}

func normalizeLiteral(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		first := trimmed[0]
		last := trimmed[len(trimmed)-1]
		if (first == '"' || first == '\'' || first == '`') && first == last {
			return trimmed[1 : len(trimmed)-1]
		}
	}
	return trimmed
}
