package pine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const SourceFormatPineV6 = "pine-v6"

type Compilation struct {
	Program  *strategyir.Program
	Warnings []string
}

type parsedLine struct {
	number  int
	raw     string
	trimmed string
	indent  int
}

type parseState struct {
	lines             []parsedLine
	warnings          []string
	longEntryIDs      map[string]bool
	shortEntryIDs     map[string]bool
	expressionAliases map[string]string
	entryPolicyCache  map[int]string
}

var (
	strategyTitlePattern    = regexp.MustCompile(`(?i)^strategy\s*\(\s*("[^"]*"|'[^']*'|[^,\)]*)`)
	assignmentPattern       = regexp.MustCompile(`^(var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(:=|=)\s*(.+)$`)
	tupleAssignmentPattern  = regexp.MustCompile(`^\[\s*([A-Za-z_][A-Za-z0-9_]*)(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?\s*\]\s*(:=|=)\s*(.+)$`)
	inputCallPattern        = regexp.MustCompile(`(?i)^input\.[A-Za-z_][A-Za-z0-9_]*\s*\(\s*([^,\)]+)`)
	equityQuantityPattern   = regexp.MustCompile(`(?i)^\(?\s*strategy\.equity\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?\s*/\s*close$`)
	amountQuantityPattern   = regexp.MustCompile(`(?i)^\(?\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*close\s*\)?$`)
	entryPolicyAnnotPattern = regexp.MustCompile(`@entry_policy\s+(\S+)`)
	exitPricePattern        = regexp.MustCompile(`(?i)^close\s*\*\s*\(?\s*1\s*[+-]\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?$`)
	exitTrailPattern        = regexp.MustCompile(`(?i)^close\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100$`)
	historyReferencePattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*\[\s*([0-9]+)\s*\]`)
)

func Compile(script string) (Compilation, error) {
	analysis := AnalyzeScript(script, AnalysisOptions{})
	if !analysis.OK {
		if err := diagnosticError(analysis.Diagnostics); err != nil {
			return Compilation{}, err
		}
		return Compilation{}, fmt.Errorf("pine script is not valid")
	}
	return Compilation{Program: analysis.Program, Warnings: analysis.Warnings}, nil
}

func compileLoweredAST(script string, _ *AST) (Compilation, error) {
	state := &parseState{
		lines:             nil,
		longEntryIDs:      map[string]bool{},
		shortEntryIDs:     map[string]bool{},
		expressionAliases: map[string]string{},
		entryPolicyCache:  buildEntryPolicyCache(script),
	}
	state.lines = tokenizeScript(script)
	if len(state.lines) == 0 {
		return Compilation{}, fmt.Errorf("pine script is required")
	}

	program := &strategyir.Program{SourceFormat: SourceFormatPineV6}
	versionSeen := false
	strategySeen := false
	executableStart := 0

	for index, line := range state.lines {
		if line.indent > 0 {
			executableStart = index
			break
		}
		lower := strings.ToLower(line.trimmed)
		switch {
		case strings.HasPrefix(lower, "//@version"):
			if !strings.Contains(strings.ReplaceAll(lower, " ", ""), "//@version=6") {
				return Compilation{}, fmt.Errorf("pine line %d: JFTrade requires //@version=6", line.number)
			}
			versionSeen = true
			executableStart = index + 1
		case strings.HasPrefix(lower, "strategy("):
			title := parseStrategyTitle(line.trimmed)
			if title == "" {
				title = "Pine Strategy"
			}
			program.Metadata.Name = title
			strategySeen = true
			executableStart = index + 1
		case strings.HasPrefix(lower, "indicator("), strings.HasPrefix(lower, "study("), strings.HasPrefix(lower, "library("):
			return Compilation{}, fmt.Errorf("pine line %d: JFTrade can execute strategy(...) scripts only", line.number)
		case isVisualOnlyCall(lower):
			state.warnings = append(state.warnings, fmt.Sprintf("pine line %d: visual-only call %q is ignored by JFTrade", line.number, callName(line.trimmed)))
			executableStart = index + 1
		default:
			executableStart = index
			goto metadataDone
		}
	}

metadataDone:
	if !versionSeen {
		return Compilation{}, fmt.Errorf("pine script requires //@version=6")
	}
	if !strategySeen {
		return Compilation{}, fmt.Errorf("pine script requires strategy(...) declaration")
	}

	statements, _, err := state.parseBlock(executableStart, -1)
	if err != nil {
		return Compilation{}, err
	}
	if len(statements) == 0 {
		statements = []strategyir.Statement{&strategyir.LogStmt{
			Range:   strategyir.SourceRange{StartLine: 1, EndLine: 1},
			Message: "pine strategy has no executable statements",
		}}
	}
	program.Hooks = []strategyir.HookBlock{{
		Kind:       strategyir.HookKLineClose,
		Range:      strategyir.SourceRange{StartLine: statements[0].SourceRange().StartLine, EndLine: statements[len(statements)-1].SourceRange().EndLine},
		Statements: statements,
	}}
	return Compilation{Program: program, Warnings: state.warnings}, nil
}

func ParseScript(script string) (*strategyir.Program, error) {
	compilation, err := Compile(script)
	if err != nil {
		return nil, err
	}
	return compilation.Program, nil
}

func ValidateScript(script string) error {
	program, err := ParseScript(script)
	if err != nil {
		return err
	}
	_, err = strategyir.PlanRequirements(program)
	return err
}

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
			if inString == 0 {
				inString = ch
			} else if inString == ch {
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

func buildEntryPolicyCache(script string) map[int]string {
	cache := map[int]string{}
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	for lineNumMinus1, rawLine := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(trimmed, "//") {
			continue
		}
		match := entryPolicyAnnotPattern.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		cache[lineNumMinus1+2] = strings.ToLower(strings.TrimSpace(match[1])) // +2 = current line + next line
	}
	return cache
}

func (s *parseState) readEntryPolicyForLine(lineNumber int) string {
	if policy, ok := s.entryPolicyCache[lineNumber]; ok {
		return policy
	}
	return "same_direction"
}

func (s *parseState) parseBlock(startIndex int, parentIndent int) ([]strategyir.Statement, int, error) {
	statements := make([]strategyir.Statement, 0)
	index := startIndex
	for index < len(s.lines) {
		line := s.lines[index]
		if line.indent <= parentIndent {
			return statements, index, nil
		}
		statement, nextIndex, err := s.parseStatement(index)
		if err != nil {
			return nil, index, err
		}
		if statement != nil {
			statements = append(statements, statement)
		}
		index = nextIndex
	}
	return statements, index, nil
}

func (s *parseState) parseStatement(index int) (strategyir.Statement, int, error) {
	line := s.lines[index]
	lower := strings.ToLower(line.trimmed)
	if err := rejectUnsupported(line); err != nil {
		return nil, index, err
	}
	if strings.HasPrefix(lower, "if ") {
		condition := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "if "))
		condition = strings.TrimSuffix(condition, ":")
		condition = s.normalizeExpression(condition)
		if err := validateExpression(line.number, "if condition", condition); err != nil {
			return nil, index, err
		}
		thenStatements, nextIndex, err := s.parseBlock(index+1, line.indent)
		if err != nil {
			return nil, index, err
		}
		if len(thenStatements) == 0 {
			return nil, index, fmt.Errorf("pine line %d: if branch requires at least one executable statement", line.number)
		}
		statement := &strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: line.number, EndLine: thenStatements[len(thenStatements)-1].SourceRange().EndLine},
			Condition: condition,
			Then:      thenStatements,
		}
		if nextIndex < len(s.lines) && s.lines[nextIndex].indent == line.indent && strings.EqualFold(s.lines[nextIndex].trimmed, "else") {
			elseStatements, afterElse, elseErr := s.parseBlock(nextIndex+1, line.indent)
			if elseErr != nil {
				return nil, index, elseErr
			}
			if len(elseStatements) == 0 {
				return nil, index, fmt.Errorf("pine line %d: else branch requires at least one executable statement", s.lines[nextIndex].number)
			}
			statement.Else = elseStatements
			statement.Range.EndLine = elseStatements[len(elseStatements)-1].SourceRange().EndLine
			return statement, afterElse, nil
		}
		return statement, nextIndex, nil
	}
	if strings.EqualFold(line.trimmed, "else") {
		return nil, index, fmt.Errorf("pine line %d: else must follow an if block", line.number)
	}
	if isVisualOnlyCall(lower) {
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: visual-only call %q is ignored by JFTrade", line.number, callName(line.trimmed)))
		return nil, index + 1, nil
	}
	if statement, ok, err := s.parseTupleAssignment(line); ok || err != nil {
		return statement, index + 1, err
	}
	if statement, ok, err := s.parseAssignment(line); ok || err != nil {
		return statement, index + 1, err
	}
	if statement, ok, err := s.parseStrategyCall(line); ok || err != nil {
		return statement, index + 1, err
	}
	if statement, ok := parseLogOrAlert(line); ok {
		return statement, index + 1, nil
	}
	return nil, index, fmt.Errorf("pine line %d: unsupported executable statement %q", line.number, line.trimmed)
}

func (s *parseState) parseTupleAssignment(line parsedLine) (strategyir.Statement, bool, error) {
	match := tupleAssignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	expression := strings.TrimSpace(match[5])
	lower := strings.ToLower(expression)
	if !strings.HasPrefix(lower, "ta.macd(") {
		return nil, true, fmt.Errorf("pine line %d: tuple assignment is supported only for ta.macd(...)", line.number)
	}
	args := splitArguments(callArgs(expression))
	if len(args) < 4 {
		return nil, true, fmt.Errorf("pine line %d: ta.macd(source, fast, slow, signal) requires four arguments", line.number)
	}
	alias := strings.TrimSpace(match[1])
	signalAlias := strings.TrimSpace(match[2])
	histAlias := strings.TrimSpace(match[3])
	expr := fmt.Sprintf("macd(%s, %s, %s)", s.normalizeExpression(args[1]), s.normalizeExpression(args[2]), s.normalizeExpression(args[3]))
	if signalAlias != "" {
		s.expressionAliases[signalAlias] = alias + ".signal"
	}
	if histAlias != "" {
		s.expressionAliases[histAlias] = alias + ".histogram"
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       alias,
		Expression: expr,
	}, true, validateExpression(line.number, "assignment expression", expr)
}

func (s *parseState) parseAssignment(line parsedLine) (strategyir.Statement, bool, error) {
	match := assignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	name := strings.TrimSpace(match[2])
	operator := strings.TrimSpace(match[3])
	expression := s.normalizeExpression(strings.TrimSpace(match[4]))
	if expression == "" {
		return nil, true, fmt.Errorf("pine line %d: assignment expression is required", line.number)
	}
	if err := validateExpression(line.number, "assignment expression", expression); err != nil {
		return nil, true, err
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       name,
		Expression: expression,
		Mode:       assignmentMode(strings.TrimSpace(match[1]) != "", operator),
	}, true, nil
}

func assignmentMode(isVar bool, operator string) strategyir.AssignmentMode {
	switch {
	case isVar:
		return strategyir.AssignmentModeVar
	case operator == ":=":
		return strategyir.AssignmentModeReassign
	default:
		return strategyir.AssignmentModeLet
	}
}

func (s *parseState) parseStrategyCall(line parsedLine) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.HasPrefix(lower, "strategy.entry("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: strategy.entry(id, direction, ...) requires at least two arguments", line.number)
		}
		id := unquote(strings.TrimSpace(args[0]))
		direction := strings.ToLower(strings.TrimSpace(args[1]))
		if hasNamedArg(args[2:], "qty_percent") {
			return nil, true, fmt.Errorf("pine line %d: strategy.entry qty_percent is not supported; use qty=(strategy.equity * pct / 100) / close", line.number)
		}
		quantityMode, quantityExpr := pineQuantity(args[2:])
		orderType, limitExpr := pineOrderType(args[2:])
		action := strategyir.OrderActionBuy
		if strings.Contains(direction, "short") {
			action = strategyir.OrderActionShort
			s.shortEntryIDs[id] = true
		} else {
			s.longEntryIDs[id] = true
		}
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Action:             action,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        s.readEntryPolicyForLine(line.number),
			OrderType:          orderType,
			LimitExpression:    limitExpr,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.close("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) == 0 {
			return nil, true, fmt.Errorf("pine line %d: strategy.close(id) requires an entry id", line.number)
		}
		id := unquote(strings.TrimSpace(args[0]))
		action := strategyir.OrderActionSell
		if s.shortEntryIDs[id] {
			action = strategyir.OrderActionCover
		}
		quantityMode, quantityExpr := pineCloseQuantity(args[1:], id)
		orderType, limitExpr := pineOrderType(args[1:])
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Action:             action,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        "same_direction",
			OrderType:          orderType,
			LimitExpression:    limitExpr,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.exit("):
		statement, err := parseStrategyExit(line)
		if err != nil {
			return nil, true, err
		}
		return statement, true, nil
	default:
		return nil, false, nil
	}
}

func parseStrategyExit(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 2 {
		return nil, fmt.Errorf("pine line %d: strategy.exit(id, from_entry, ...) requires an entry id", line.number)
	}
	fromEntry := strings.ToLower(unquote(strings.TrimSpace(args[1])))
	direction := "long"
	if strings.Contains(fromEntry, "short") {
		direction = "short"
	}
	if stopExpr, ok := namedArgValue(args[2:], "stop"); ok {
		percentage, ok := pineExitPercentage(stopExpr, exitPricePattern)
		if !ok {
			return nil, fmt.Errorf("pine line %d: strategy.exit stop is supported only as close * (1 +/- pct / 100)", line.number)
		}
		return pineProtectStatement(line.number, direction, "stopLoss", percentage), nil
	}
	if limitExpr, ok := namedArgValue(args[2:], "limit"); ok {
		percentage, ok := pineExitPercentage(limitExpr, exitPricePattern)
		if !ok {
			return nil, fmt.Errorf("pine line %d: strategy.exit limit is supported only as close * (1 +/- pct / 100)", line.number)
		}
		return pineProtectStatement(line.number, direction, "takeProfit", percentage), nil
	}
	if trailPoints, ok := namedArgValue(args[2:], "trail_points"); ok {
		trailOffset, hasOffset := namedArgValue(args[2:], "trail_offset")
		if !hasOffset || strings.TrimSpace(trailOffset) == "" {
			return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop requires trail_offset", line.number)
		}
		percentage, ok := pineExitPercentage(trailPoints, exitTrailPattern)
		offsetPercentage, offsetOK := pineExitPercentage(trailOffset, exitTrailPattern)
		if !ok || !offsetOK || percentage != offsetPercentage {
			return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop is supported only as matching close * pct / 100 trail_points and trail_offset", line.number)
		}
		return pineProtectStatement(line.number, direction, "trailingStop", percentage), nil
	}
	return nil, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", line.number)
}

func pineProtectStatement(lineNumber int, direction string, mode string, percentage string) *strategyir.ProtectStmt {
	return &strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: lineNumber, EndLine: lineNumber},
		Direction:            direction,
		Mode:                 mode,
		TimeValueExpression:  "1",
		TimeUnit:             "bar",
		PercentageExpression: percentage,
		WindowPolicy:         "continuous",
	}
}

func pineExitPercentage(expression string, pattern *regexp.Regexp) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(expression))
	match := pattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func (s *parseState) normalizeExpression(expression string) string {
	result := strings.TrimSpace(expression)
	if match := inputCallPattern.FindStringSubmatch(result); match != nil {
		result = strings.TrimSpace(match[1])
	}
	result = strings.ReplaceAll(result, "strategy.position_avg_price", "position_avg_price")
	result = strings.ReplaceAll(result, "strategy.position_size", "position_size")
	result = replaceSupportedRequestSecurity(result)
	result = replaceTAFunction(result, "ema", "ma(EMA, ${period})")
	result = replaceTAFunction(result, "sma", "ma(SMA, ${period})")
	result = replaceTAFunction(result, "rma", "ma(SMMA, ${period})")
	result = replaceTAFunction(result, "wma", "ma(LWMA, ${period})")
	result = replaceTAFunction(result, "hma", "ma(HMA, ${period})")
	result = replaceTAFunction(result, "vwma", "ma(VWMA, ${period})")
	result = replaceTAFunction(result, "rsi", "rsi(${period})")
	result = replaceTAMacd(result)
	result = replaceTAFunction(result, "atr", "atr(${period})")
	result = replaceTAFunction(result, "stdev", "stdev(${period})")
	result = replaceTAFunction(result, "cci", "cci(${period})")
	result = replaceTAFunction(result, "crossover", "cross_over(${left}, ${right})")
	result = replaceTAFunction(result, "crossunder", "cross_under(${left}, ${right})")
	result = strings.ReplaceAll(result, "math.abs", "abs")
	for alias, target := range s.expressionAliases {
		result = regexp.MustCompile(`\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, target)
	}
	result = normalizeHistoryReferences(result)
	result = normalizeTernaryExpression(result)
	result = strings.ReplaceAll(result, " and ", " && ")
	result = strings.ReplaceAll(result, " or ", " || ")
	return strings.TrimSpace(result)
}

func normalizeHistoryReferences(expression string) string {
	return rewriteOutsideStringLiterals(expression, func(segment string) string {
		return historyReferencePattern.ReplaceAllStringFunc(segment, func(match string) string {
			parts := historyReferencePattern.FindStringSubmatch(match)
			if len(parts) != 3 || strings.TrimSpace(parts[2]) != "1" {
				return match
			}
			return "previous(" + strings.TrimSpace(parts[1]) + ")"
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

func replaceSupportedRequestSecurity(expression string) string {
	prefix := "request.security("
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
		replacement, ok := lowerSupportedRequestSecurity(args)
		if !ok {
			return expression
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func lowerSupportedRequestSecurity(args []string) (string, bool) {
	if len(args) < 3 || strings.TrimSpace(args[0]) != "syminfo.tickerid" {
		return "", false
	}
	timeUnit, ok := pineTimeframeUnit(unquote(strings.TrimSpace(args[1])))
	if !ok {
		return "", false
	}
	name, innerArgs, ok := parseTACall(strings.TrimSpace(args[2]))
	if !ok || len(innerArgs) < 2 || strings.TrimSpace(innerArgs[0]) != "close" {
		return "", false
	}
	maType, ok := pineMovingAverageType(name)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("ma(%s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit), true
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
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "1":
		return "minute", true
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
			left, right := "close", "close"
			if len(args) >= 1 {
				left = strings.TrimSpace(args[0])
			}
			if len(args) >= 2 {
				right = strings.TrimSpace(args[1])
			}
			replacement = strings.ReplaceAll(replacement, "${left}", left)
			replacement = strings.ReplaceAll(replacement, "${right}", right)
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func rejectUnsupported(line parsedLine) error {
	if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
		return fmt.Errorf("pine line %d: %s", diagnostic.Line, diagnostic.Message)
	}
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.Contains(lower, "request.security("):
		if strings.Contains(strings.ToLower(replaceSupportedRequestSecurity(line.trimmed)), "request.security(") {
			return fmt.Errorf("pine line %d: request.security() is supported only for syminfo.tickerid moving-average calls generated by JFTrade", line.number)
		}
	case strings.HasPrefix(lower, "runtime.error("):
		return fmt.Errorf("pine line %d: %s", line.number, firstStringArgument(line.trimmed))
	case strings.HasPrefix(lower, "import "):
		return fmt.Errorf("pine line %d: Pine libraries/imports are not supported by JFTrade yet", line.number)
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return fmt.Errorf("pine line %d: Pine collection namespaces array/matrix/map are not supported by JFTrade yet", line.number)
	default:
		return nil
	}
	return nil
}

func unsupportedSyntaxDiagnostic(line parsedLine) (Diagnostic, bool) {
	lower := strings.ToLower(strings.TrimSpace(line.trimmed))
	switch {
	case strings.HasPrefix(lower, "for "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_FOR_UNSUPPORTED", "for loops are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "while "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_WHILE_UNSUPPORTED", "while loops are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "switch"):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_SWITCH_UNSUPPORTED", "switch statements are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "type "), strings.HasPrefix(lower, "method "), strings.HasPrefix(lower, "import "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_DECLARATION_UNSUPPORTED", "Pine declarations, libraries, and methods are not executable in this JFTrade Pine v6 version", line), true
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_COLLECTION_UNSUPPORTED", "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version", line), true
	case regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*\([^)]*\)\s*=>`).MatchString(line.trimmed):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_FUNCTION_UNSUPPORTED", "user-defined functions are parsed but not executable in this JFTrade Pine v6 version", line), true
	case hasUnsupportedHistoryReference(line.trimmed):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_HISTORY_REF_UNSUPPORTED", "only one-bar history references like close[1] are supported in this JFTrade Pine v6 version", line), true
	default:
		return Diagnostic{}, false
	}
}

func hasUnsupportedHistoryReference(expression string) bool {
	for _, match := range historyReferenceMatchesOutsideStringLiterals(expression) {
		if len(match) == 3 && strings.TrimSpace(match[2]) != "1" {
			return true
		}
	}
	return false
}

func validateExpression(lineNumber int, label string, expression string) error {
	if _, err := strategyexpression.ParseExpression(expression); err != nil {
		return fmt.Errorf("pine line %d: invalid %s %q: %w", lineNumber, label, strings.TrimSpace(expression), err)
	}
	return nil
}

func parseLogOrAlert(line parsedLine) (strategyir.Statement, bool) {
	lower := strings.ToLower(line.trimmed)
	if strings.HasPrefix(lower, "alert(") || strings.HasPrefix(lower, "notify(") {
		return &strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	if strings.HasPrefix(lower, "log.info(") || strings.HasPrefix(lower, "log.warning(") || strings.HasPrefix(lower, "log.error(") {
		return &strategyir.LogStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, Message: firstStringArgument(line.trimmed)}, true
	}
	return nil, false
}

func parseStrategyTitle(line string) string {
	match := strategyTitlePattern.FindStringSubmatch(line)
	if match == nil {
		return ""
	}
	return unquote(strings.TrimSpace(match[1]))
}

func pineQuantity(args []string) (string, string) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "qty":
			if percent, ok := pineEquityPercent(value); ok {
				return "account_position_percent", percent
			}
			if amount, ok := pineAmountQuantity(value); ok {
				return "amount", amount
			}
			return "shares", value
		}
	}
	return "shares", "1"
}

func pineAmountQuantity(value string) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(value))
	match := amountQuantityPattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func pineCloseQuantity(args []string, entryID string) (string, string) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "qty":
			if percent, ok := pineEquityPercent(value); ok {
				return "account_position_percent", percent
			}
			if amount, ok := pineAmountQuantity(value); ok {
				return "amount", amount
			}
			return "shares", value
		}
	}
	return "symbol_position_percent", "100"
}

func hasNamedArg(args []string, name string) bool {
	for _, arg := range args {
		key, _, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func namedArgValue(args []string, name string) (string, bool) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(key, name) {
			return value, true
		}
	}
	return "", false
}

func pineOrderType(args []string) (string, string) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "limit") {
			return "LIMIT", strings.TrimSpace(value)
		}
	}
	return "MARKET", ""
}

func pineEquityPercent(value string) (string, bool) {
	normalized := stripWrappingParens(strings.TrimSpace(value))
	match := equityQuantityPattern.FindStringSubmatch(normalized)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

func stripWrappingParens(value string) string {
	result := strings.TrimSpace(value)
	for len(result) >= 2 && result[0] == '(' && result[len(result)-1] == ')' && wrappingParensCoverExpression(result) {
		result = strings.TrimSpace(result[1 : len(result)-1])
	}
	return result
}

func wrappingParensCoverExpression(value string) bool {
	depth := 0
	inString := byte(0)
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
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
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && index < len(value)-1 {
				return false
			}
		}
	}
	return depth == 0
}

func splitNamedArg(value string) (string, string, bool) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func isVisualOnlyCall(lower string) bool {
	return strings.HasPrefix(lower, "plot(") ||
		strings.HasPrefix(lower, "plotshape(") ||
		strings.HasPrefix(lower, "plotchar(") ||
		strings.HasPrefix(lower, "hline(") ||
		strings.HasPrefix(lower, "bgcolor(") ||
		strings.HasPrefix(lower, "barcolor(")
}

func callName(line string) string {
	if index := strings.Index(line, "("); index > 0 {
		return strings.TrimSpace(line[:index])
	}
	return line
}

func callArgs(line string) string {
	open := strings.Index(line, "(")
	if open < 0 {
		return ""
	}
	close := matchingParen(line, open)
	if close < 0 {
		return line[open+1:]
	}
	return line[open+1 : close]
}

func matchingParen(value string, open int) int {
	depth := 0
	inString := byte(0)
	for index := open; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
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
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitArguments(value string) []string {
	parts := []string{}
	start := 0
	depth := 0
	inString := byte(0)
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch == '"' || ch == '\'') && (index == 0 || value[index-1] != '\\') {
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
		if ch == '(' || ch == '[' {
			depth++
		} else if ch == ')' || ch == ']' {
			depth--
		} else if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	tail := strings.TrimSpace(value[start:])
	if tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func firstStringArgument(line string) string {
	args := splitArguments(callArgs(line))
	if len(args) == 0 {
		return ""
	}
	return unquote(args[0])
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' || first == '\'') && first == last {
			return value[1 : len(value)-1]
		}
	}
	return value
}
