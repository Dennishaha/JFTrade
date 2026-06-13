package pine

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	strategyexpression "github.com/jftrade/jftrade-main/pkg/strategy/expression"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const SourceFormatPineV6 = "pine-v6"
const maxHistoryLookback = 500
const maxUDFExpansionDepth = 8
const maxStaticForIterations = 100
const maxStaticForDepth = 2

type Compilation struct {
	Program      *strategyir.Program
	Requirements strategyir.Requirements
	Warnings     []string
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
	udfs              map[string]pineUDF
	expressionAliases map[string]string
	sourceAliases     map[string]string
	valueAliases      map[string]string
	loopVariables     map[string]bool
	forDepth          int
	normalizationErr  error
	entryPolicyCache  map[int]string
	strategyMetadata  strategyir.StrategyMetadata
	regexpCache       map[string]*regexp.Regexp
}

type pineUDF struct {
	Name string
	Args []string
	Body string
	Line int
}

func (s *parseState) cachedRegexp(key string, pattern string) *regexp.Regexp {
	if s == nil {
		return regexp.MustCompile(pattern)
	}
	if compiled, ok := s.regexpCache[key]; ok {
		return compiled
	}
	if s.regexpCache == nil {
		s.regexpCache = map[string]*regexp.Regexp{}
	}
	compiled := regexp.MustCompile(pattern)
	s.regexpCache[key] = compiled
	return compiled
}

var (
	strategyTitlePattern    = regexp.MustCompile(`(?i)^strategy\s*\(\s*("[^"]*"|'[^']*'|[^,\)]*)`)
	assignmentPattern       = regexp.MustCompile(`^(?:(var|varip|const)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(:=|=)\s*(.+)$`)
	tupleAssignmentPattern  = regexp.MustCompile(`^\[\s*([A-Za-z_][A-Za-z0-9_]*)(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?(?:\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?\s*\]\s*(:=|=)\s*(.+)$`)
	inputCallPattern        = regexp.MustCompile(`(?i)^input\.[A-Za-z_][A-Za-z0-9_]*\s*\(\s*([^,\)]+)`)
	equityQuantityPattern   = regexp.MustCompile(`(?i)^\(?\s*strategy\.equity\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?\s*/\s*close$`)
	amountQuantityPattern   = regexp.MustCompile(`(?i)^\(?\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*close\s*\)?$`)
	entryPolicyAnnotPattern = regexp.MustCompile(`@entry_policy\s+(\S+)`)
	exitPricePattern        = regexp.MustCompile(`(?i)^close\s*\*\s*\(?\s*1\s*[+-]\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100\s*\)?$`)
	exitTrailPattern        = regexp.MustCompile(`(?i)^close\s*\*\s*([0-9]+(?:\.[0-9]+)?)\s*/\s*100$`)
	historyReferencePattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*\[\s*([0-9]+)\s*\]`)
	callHistoryPattern      = regexp.MustCompile(`\)\s*\[\s*[0-9]+\s*\]`)
	udfPattern              = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*=>\s*(.*)$`)
	forLoopPattern          = regexp.MustCompile(`(?i)^for\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+?)\s+to\s+(.+?)(?:\s+by\s+(.+))?\s*$`)
	identifierPattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	memberPattern           = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	numberPattern           = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
	taTRPattern             = regexp.MustCompile(`(?i)\bta\.tr\b`)
)

func Compile(script string) (Compilation, error) {
	analysis := AnalyzeScript(script, AnalysisOptions{})
	if !analysis.OK {
		if err := diagnosticError(analysis.Diagnostics); err != nil {
			return Compilation{}, err
		}
		return Compilation{}, fmt.Errorf("pine script is not valid")
	}
	return Compilation{
		Program:      analysis.Program,
		Requirements: analysis.Requirements,
		Warnings:     analysis.Warnings,
	}, nil
}

func compileLoweredAST(script string, lines []parsedLine, _ *AST) (Compilation, error) {
	state := &parseState{
		lines:             nil,
		longEntryIDs:      map[string]bool{},
		shortEntryIDs:     map[string]bool{},
		udfs:              map[string]pineUDF{},
		expressionAliases: map[string]string{},
		sourceAliases:     map[string]string{},
		valueAliases:      map[string]string{},
		loopVariables:     map[string]bool{},
		entryPolicyCache:  buildEntryPolicyCache(script),
		regexpCache:       map[string]*regexp.Regexp{},
	}
	state.lines = lines
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
			metadata, warnings := parseStrategyDeclaration(line.trimmed)
			state.warnings = append(state.warnings, warnings...)
			program.Metadata = metadata
			state.strategyMetadata = metadata
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
	_, err := Compile(script)
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
		if strings.HasPrefix(strings.ToLower(line.trimmed), "for ") {
			loopStatements, nextIndex, err := s.parseStaticForLoop(index)
			if err != nil {
				return nil, index, err
			}
			statements = append(statements, loopStatements...)
			index = nextIndex
			continue
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
	if ok, nextIndex, err := s.parseUDFDefinition(index); ok || err != nil {
		return nil, nextIndex, err
	}
	if lower == "break" || lower == "continue" {
		return nil, index, fmt.Errorf("pine line %d: %s is not supported in JFTrade static for loops yet", line.number, line.trimmed)
	}
	if err := rejectUnsupported(line); err != nil {
		return nil, index, err
	}
	if strings.HasPrefix(lower, "if ") {
		condition := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "if "))
		condition = strings.TrimSuffix(condition, ":")
		condition = s.normalizeExpression(condition)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, index, err
		}
		if err := validateExpression(line.number, "if condition", condition); err != nil {
			return nil, index, err
		}
		thenStatements, nextIndex, err := s.parseBlock(index+1, line.indent)
		if err != nil {
			return nil, index, err
		}
		endLine := line.number
		if len(thenStatements) > 0 {
			endLine = thenStatements[len(thenStatements)-1].SourceRange().EndLine
		}
		statement := &strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: line.number, EndLine: endLine},
			Condition: condition,
			Then:      thenStatements,
		}
		if nextIndex < len(s.lines) && s.lines[nextIndex].indent == line.indent && strings.EqualFold(s.lines[nextIndex].trimmed, "else") {
			elseStatements, afterElse, elseErr := s.parseBlock(nextIndex+1, line.indent)
			if elseErr != nil {
				return nil, index, elseErr
			}
			if len(elseStatements) > 0 {
				statement.Range.EndLine = elseStatements[len(elseStatements)-1].SourceRange().EndLine
			}
			statement.Else = elseStatements
			if len(statement.Then) == 0 && len(statement.Else) == 0 {
				return nil, afterElse, nil
			}
			return statement, afterElse, nil
		}
		if len(statement.Then) == 0 {
			return nil, nextIndex, nil
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

func (s *parseState) takeNormalizationErr(lineNumber int) error {
	if s == nil || s.normalizationErr == nil {
		return nil
	}
	err := s.normalizationErr
	s.normalizationErr = nil
	return fmt.Errorf("pine line %d: %w", lineNumber, err)
}

func (s *parseState) parseUDFDefinition(index int) (bool, int, error) {
	line := s.lines[index]
	match := udfPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return false, index, nil
	}
	if line.indent != 0 {
		return true, index, fmt.Errorf("pine line %d: user-defined functions are supported only at top level", line.number)
	}
	name := strings.TrimSpace(match[1])
	if isReservedUDFName(name) {
		return true, index, fmt.Errorf("pine line %d: user-defined function %q conflicts with a JFTrade/Pine built-in", line.number, name)
	}
	args, err := parseUDFArgs(line.number, match[2])
	if err != nil {
		return true, index, err
	}
	body := strings.TrimSpace(match[3])
	nextIndex := index + 1
	if body == "" {
		if nextIndex >= len(s.lines) || s.lines[nextIndex].indent <= line.indent {
			return true, index, fmt.Errorf("pine line %d: user-defined function %q requires one expression body", line.number, name)
		}
		bodyLine := s.lines[nextIndex]
		body = strings.TrimSpace(bodyLine.trimmed)
		nextIndex++
		if nextIndex < len(s.lines) && s.lines[nextIndex].indent > line.indent {
			return true, index, fmt.Errorf("pine line %d: multi-statement user-defined functions are not supported by JFTrade yet", line.number)
		}
	}
	if body == "" || strings.HasPrefix(strings.ToLower(body), "if ") || strings.HasPrefix(strings.ToLower(body), "for ") {
		return true, index, fmt.Errorf("pine line %d: user-defined function %q must have a single expression body", line.number, name)
	}
	if strings.Contains(body, "=>") {
		return true, index, fmt.Errorf("pine line %d: nested user-defined functions are not supported by JFTrade yet", line.number)
	}
	s.udfs[name] = pineUDF{Name: name, Args: args, Body: body, Line: line.number}
	return true, nextIndex, nil
}

func parseUDFArgs(lineNumber int, raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parts := splitArguments(trimmed)
	args := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if !identifierPattern.MatchString(name) {
			return nil, fmt.Errorf("pine line %d: invalid user-defined function argument %q", lineNumber, part)
		}
		if seen[name] {
			return nil, fmt.Errorf("pine line %d: duplicate user-defined function argument %q", lineNumber, name)
		}
		seen[name] = true
		args = append(args, name)
	}
	return args, nil
}

func isReservedUDFName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "strategy", "ta", "math", "input", "request", "color", "dayofweek", "month", "barstate", "session", "syminfo", "timeframe",
		"plot", "plotshape", "plotchar", "hline", "bgcolor", "barcolor", "fill", "alert", "alertcondition", "notify", "log",
		"ifelse", "nz", "timestamp":
		return true
	default:
		return false
	}
}

func (s *parseState) parseStaticForLoop(index int) ([]strategyir.Statement, int, error) {
	line := s.lines[index]
	if s.forDepth >= maxStaticForDepth {
		return nil, index, fmt.Errorf("pine line %d: nested for loops deeper than %d are not supported by JFTrade yet", line.number, maxStaticForDepth)
	}
	loopHeader := strings.TrimSpace(strings.TrimSuffix(line.trimmed, ":"))
	match := forLoopPattern.FindStringSubmatch(loopHeader)
	if match == nil {
		return nil, index, fmt.Errorf("pine line %d: for loop must use 'for i = start to end [by step]'", line.number)
	}
	loopVar := strings.TrimSpace(match[1])
	start, err := s.parseStaticIntExpression(line.number, match[2], "for start")
	if err != nil {
		return nil, index, err
	}
	end, err := s.parseStaticIntExpression(line.number, match[3], "for end")
	if err != nil {
		return nil, index, err
	}
	step := 1
	if strings.TrimSpace(match[4]) != "" {
		step, err = s.parseStaticIntExpression(line.number, match[4], "for step")
		if err != nil {
			return nil, index, err
		}
	}
	if step == 0 {
		return nil, index, fmt.Errorf("pine line %d: for loop step cannot be 0", line.number)
	}
	if (step > 0 && start > end) || (step < 0 && start < end) {
		return nil, index, fmt.Errorf("pine line %d: for loop step does not reach the end value", line.number)
	}
	values := make([]int, 0)
	for value := start; ; value += step {
		if (step > 0 && value > end) || (step < 0 && value < end) {
			break
		}
		values = append(values, value)
		if len(values) > maxStaticForIterations {
			return nil, index, fmt.Errorf("pine line %d: for loop expands to more than %d iterations", line.number, maxStaticForIterations)
		}
		if value == end {
			break
		}
	}
	if len(values) == 0 || values[len(values)-1] != end {
		return nil, index, fmt.Errorf("pine line %d: for loop step does not reach the end value", line.number)
	}

	statements := make([]strategyir.Statement, 0)
	nextIndex := index + 1
	previousValue, hadValue := s.valueAliases[loopVar]
	previousLoopVar := s.loopVariables[loopVar]
	s.loopVariables[loopVar] = true
	s.forDepth++
	defer func() {
		s.forDepth--
		if hadValue {
			s.valueAliases[loopVar] = previousValue
		} else {
			delete(s.valueAliases, loopVar)
		}
		if previousLoopVar {
			s.loopVariables[loopVar] = true
		} else {
			delete(s.loopVariables, loopVar)
		}
	}()

	for _, value := range values {
		s.valueAliases[loopVar] = strconv.Itoa(value)
		bodyStatements, afterBody, err := s.parseBlock(index+1, line.indent)
		if err != nil {
			return nil, index, err
		}
		nextIndex = afterBody
		statements = append(statements, bodyStatements...)
	}
	return statements, nextIndex, nil
}

func (s *parseState) parseStaticIntExpression(lineNumber int, expression string, label string) (int, error) {
	normalized := strings.TrimSpace(lowerInputCalls(expression))
	normalized = s.resolveValueAliases(normalized)
	normalized = stripWrappingParens(normalized)
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil || value != float64(int(value)) {
		return 0, fmt.Errorf("pine line %d: %s must be a static integer constant or input.int default", lineNumber, label)
	}
	return int(value), nil
}

func (s *parseState) parseTupleAssignment(line parsedLine) (strategyir.Statement, bool, error) {
	match := tupleAssignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	expression := strings.TrimSpace(match[5])
	lower := strings.ToLower(expression)
	args := splitArguments(callArgs(expression))
	if strings.HasPrefix(lower, "ta.bb(") {
		if len(args) < 3 {
			return nil, true, fmt.Errorf("pine line %d: ta.bb(source, length, mult) requires three arguments", line.number)
		}
		basisAlias := strings.TrimSpace(match[1])
		upperAlias := strings.TrimSpace(match[2])
		lowerAlias := strings.TrimSpace(match[3])
		expr := fmt.Sprintf("bollinger(%s, %s)", s.normalizeExpression(args[1]), s.normalizeExpression(args[2]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if upperAlias != "" {
			s.expressionAliases[upperAlias] = basisAlias + ".upper"
		}
		if lowerAlias != "" {
			s.expressionAliases[lowerAlias] = basisAlias + ".lower"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       basisAlias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if strings.HasPrefix(lower, "ta.dmi(") {
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: ta.dmi(diLength, adxSmoothing) requires two arguments", line.number)
		}
		alias := strings.TrimSpace(match[1])
		minusAlias := strings.TrimSpace(match[2])
		adxAlias := strings.TrimSpace(match[3])
		expr := fmt.Sprintf("dmi(%s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if minusAlias != "" {
			s.expressionAliases[minusAlias] = alias + ".minus"
		}
		if adxAlias != "" {
			s.expressionAliases[adxAlias] = alias + ".adx"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       alias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if strings.HasPrefix(lower, "ta.supertrend(") {
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: ta.supertrend(factor, atrPeriod) requires two arguments", line.number)
		}
		alias := strings.TrimSpace(match[1])
		directionAlias := strings.TrimSpace(match[2])
		expr := fmt.Sprintf("supertrend(%s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if directionAlias != "" {
			s.expressionAliases[directionAlias] = alias + ".direction"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       alias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if !strings.HasPrefix(lower, "ta.macd(") {
		return nil, true, fmt.Errorf("pine line %d: tuple assignment is supported only for ta.macd(...), ta.bb(...), ta.dmi(...), or ta.supertrend(...)", line.number)
	}
	if len(args) < 4 {
		return nil, true, fmt.Errorf("pine line %d: ta.macd(source, fast, slow, signal) requires four arguments", line.number)
	}
	alias := strings.TrimSpace(match[1])
	signalAlias := strings.TrimSpace(match[2])
	histAlias := strings.TrimSpace(match[3])
	expr := fmt.Sprintf("macd(%s, %s, %s)", s.normalizeExpression(args[1]), s.normalizeExpression(args[2]), s.normalizeExpression(args[3]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, true, err
	}
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
	if s.loopVariables[name] {
		return nil, true, fmt.Errorf("pine line %d: loop variable %q is read-only", line.number, name)
	}
	operator := strings.TrimSpace(match[3])
	expression := s.normalizeExpression(strings.TrimSpace(match[4]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, true, err
	}
	if expression == "" {
		return nil, true, fmt.Errorf("pine line %d: assignment expression is required", line.number)
	}
	if err := validateExpression(line.number, "assignment expression", expression); err != nil {
		return nil, true, err
	}
	if isOHLCVSource(expression) {
		s.sourceAliases[name] = strings.TrimSpace(expression)
	} else {
		delete(s.sourceAliases, name)
	}
	if isSimpleAliasExpression(expression) {
		s.valueAliases[name] = strings.TrimSpace(expression)
	} else {
		delete(s.valueAliases, name)
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       name,
		Expression: expression,
		Mode:       assignmentMode(strings.TrimSpace(match[1]), operator),
	}, true, nil
}

func assignmentMode(keyword string, operator string) strategyir.AssignmentMode {
	switch {
	case strings.EqualFold(strings.TrimSpace(keyword), "var"), strings.EqualFold(strings.TrimSpace(keyword), "varip"):
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
		if err := rejectUnsupportedOrderArgs(line.number, "strategy.entry", args[2:]); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.entry", args[2:], false)
		if err != nil {
			return nil, true, err
		}
		quantityMode, quantityExpr := s.pineEntryQuantity(args[2:])
		orderType, limitExpr, stopExpr := pineOrderPrices(args[2:])
		if strings.TrimSpace(limitExpr) != "" && strings.TrimSpace(stopExpr) != "" {
			return nil, true, fmt.Errorf("pine line %d: strategy.entry stop-limit orders are not supported by JFTrade yet", line.number)
		}
		action := strategyir.OrderActionBuy
		if strings.Contains(direction, "short") {
			action = strategyir.OrderActionShort
			s.shortEntryIDs[id] = true
		} else {
			s.longEntryIDs[id] = true
		}
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		stopExpr = s.normalizeExpression(stopExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(stopExpr) != "" {
			if err := validateExpression(line.number, "order stop expression", stopExpr); err != nil {
				return nil, true, err
			}
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentEntry,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        s.readEntryPolicyForLine(line.number),
			OrderType:          orderType,
			LimitExpression:    limitExpr,
			StopExpression:     stopExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.order("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: strategy.order(id, direction, ...) requires at least two arguments", line.number)
		}
		id := unquote(strings.TrimSpace(args[0]))
		if err := rejectUnsupportedOrderArgs(line.number, "strategy.order", args[2:]); err != nil {
			return nil, true, err
		}
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.order", args[2:], false)
		if err != nil {
			return nil, true, err
		}
		direction := strings.ToLower(strings.TrimSpace(args[1]))
		quantityMode, quantityExpr := s.pineEntryQuantity(args[2:])
		orderType, limitExpr, stopExpr := pineOrderPrices(args[2:])
		if strings.TrimSpace(limitExpr) != "" && strings.TrimSpace(stopExpr) != "" {
			return nil, true, fmt.Errorf("pine line %d: strategy.order stop-limit orders are not supported by JFTrade yet", line.number)
		}
		action := strategyir.OrderActionBuy
		if strings.Contains(direction, "short") {
			action = strategyir.OrderActionSell
		}
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		stopExpr = s.normalizeExpression(stopExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(stopExpr) != "" {
			if err := validateExpression(line.number, "order stop expression", stopExpr); err != nil {
				return nil, true, err
			}
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentNet,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        "allow",
			OrderType:          orderType,
			LimitExpression:    limitExpr,
			StopExpression:     stopExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.close_all("):
		args := splitArguments(callArgs(line.trimmed))
		comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close_all", args)
		if err != nil {
			return nil, true, err
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Intent:             strategyir.OrderIntentFlatten,
			QuantityMode:       "symbol_position_percent",
			QuantityExpression: "100",
			EntryPolicy:        "same_direction",
			OrderType:          "MARKET",
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
			Immediate:          immediate,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.close("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) == 0 {
			return nil, true, fmt.Errorf("pine line %d: strategy.close(id) requires an entry id", line.number)
		}
		comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(line.number, "strategy.close", args[1:])
		if err != nil {
			return nil, true, err
		}
		id := unquote(strings.TrimSpace(args[0]))
		action := strategyir.OrderActionSell
		if s.shortEntryIDs[id] {
			action = strategyir.OrderActionCover
		}
		quantityMode, quantityExpr := pineCloseQuantity(args[1:], id)
		orderType, limitExpr, _ := pineOrderPrices(args[1:])
		quantityExpr = s.normalizeExpression(quantityExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "order quantity expression", quantityExpr); err != nil {
			return nil, true, err
		}
		limitExpr = s.normalizeExpression(limitExpr)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(limitExpr) != "" {
			if err := validateExpression(line.number, "order limit expression", limitExpr); err != nil {
				return nil, true, err
			}
		}
		return &strategyir.OrderStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 id,
			Action:             action,
			Intent:             strategyir.OrderIntentClose,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			EntryPolicy:        "same_direction",
			OrderType:          orderType,
			LimitExpression:    limitExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
			Immediate:          immediate,
		}, true, nil
	case strings.HasPrefix(lower, "strategy.exit("):
		statement, err := s.parseStrategyExit(line)
		if err != nil {
			return nil, true, err
		}
		return statement, true, nil
	case strings.HasPrefix(lower, "strategy.cancel_all("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) > 0 {
			return nil, true, fmt.Errorf("pine line %d: strategy.cancel_all arguments are not supported by JFTrade yet", line.number)
		}
		return &strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, All: true}, true, nil
	case strings.HasPrefix(lower, "strategy.cancel("):
		args := splitArguments(callArgs(line.trimmed))
		if len(args) != 1 {
			return nil, true, fmt.Errorf("pine line %d: strategy.cancel(id) requires one order id", line.number)
		}
		return &strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}, ID: unquote(strings.TrimSpace(args[0]))}, true, nil
	default:
		return nil, false, nil
	}
}

func (s *parseState) parseStrategyExit(line parsedLine) (strategyir.Statement, error) {
	args := splitArguments(callArgs(line.trimmed))
	if len(args) < 1 {
		return nil, fmt.Errorf("pine line %d: strategy.exit(id, ...) requires an exit id", line.number)
	}
	exitID := unquote(strings.TrimSpace(args[0]))
	fromEntry := ""
	orderArgs := args[1:]
	if named, ok := namedArgValue(orderArgs, "from_entry"); ok {
		fromEntry = unquote(strings.TrimSpace(named))
	} else if len(orderArgs) > 0 && !strings.Contains(orderArgs[0], "=") {
		fromEntry = unquote(strings.TrimSpace(orderArgs[0]))
		orderArgs = orderArgs[1:]
	}
	triggerCount := 0
	for _, name := range []string{"stop", "limit", "trail_points"} {
		if _, ok := namedArgValue(orderArgs, name); ok {
			triggerCount++
		}
	}
	hasStop := hasNamedArg(orderArgs, "stop")
	hasLimit := hasNamedArg(orderArgs, "limit")
	hasTrail := hasNamedArg(orderArgs, "trail_points")
	if hasTrail && (hasStop || hasLimit) {
		return nil, fmt.Errorf("pine line %d: strategy.exit trail with stop/limit is not supported by JFTrade yet", line.number)
	}
	if triggerCount == 0 {
		return nil, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", line.number)
	}
	fromEntryLower := strings.ToLower(fromEntry)
	direction := "long"
	if strings.Contains(fromEntryLower, "short") {
		direction = "short"
	}
	if strings.TrimSpace(fromEntry) == "" {
		direction = "auto"
	}
	quantityMode, quantityExpr := pineExitQuantity(orderArgs)
	quantityExpr = s.normalizeExpression(quantityExpr)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, err
	}
	if err := validateExpression(line.number, "exit quantity expression", quantityExpr); err != nil {
		return nil, err
	}
	stopExpr := ""
	if raw, ok := namedArgValue(orderArgs, "stop"); ok {
		stopExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit stop expression", stopExpr); err != nil {
			return nil, err
		}
	}
	limitExpr := ""
	if raw, ok := namedArgValue(orderArgs, "limit"); ok {
		limitExpr = s.normalizeExpression(raw)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(line.number, "exit limit expression", limitExpr); err != nil {
			return nil, err
		}
	}
	if stopExpr != "" || limitExpr != "" {
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.exit", orderArgs, false)
		if err != nil {
			return nil, err
		}
		return &strategyir.ExitStmt{
			Range:              strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			ID:                 exitID,
			FromEntry:          fromEntry,
			Direction:          direction,
			QuantityMode:       quantityMode,
			QuantityExpression: quantityExpr,
			StopExpression:     stopExpr,
			LimitExpression:    limitExpr,
			Comment:            comment,
			AlertMessage:       alertMessage,
			DisableAlert:       disableAlert,
		}, nil
	}
	if trailPoints, ok := namedArgValue(orderArgs, "trail_points"); ok {
		trailOffset, hasOffset := namedArgValue(orderArgs, "trail_offset")
		if !hasOffset || strings.TrimSpace(trailOffset) == "" {
			return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop requires trail_offset", line.number)
		}
		percentage, ok := pineExitPercentage(trailPoints, exitTrailPattern)
		offsetPercentage, offsetOK := pineExitPercentage(trailOffset, exitTrailPattern)
		if !ok || !offsetOK || percentage != offsetPercentage {
			return nil, fmt.Errorf("pine line %d: strategy.exit trailing stop is supported only as matching close * pct / 100 trail_points and trail_offset", line.number)
		}
		comment, alertMessage, disableAlert, err := pineOrderMetadata(line.number, "strategy.exit", orderArgs, false)
		if err != nil {
			return nil, err
		}
		statement := pineProtectStatement(line.number, direction, "trailingStop", percentage, quantityMode, quantityExpr)
		statement.Comment = comment
		statement.AlertMessage = alertMessage
		statement.DisableAlert = disableAlert
		return statement, nil
	}
	return nil, fmt.Errorf("pine line %d: strategy.exit advanced exit semantics are not supported by JFTrade yet", line.number)
}

func pineProtectStatement(lineNumber int, direction string, mode string, percentage string, quantityMode string, quantityExpression string) *strategyir.ProtectStmt {
	return &strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: lineNumber, EndLine: lineNumber},
		Direction:            direction,
		Mode:                 mode,
		QuantityMode:         quantityMode,
		QuantityExpression:   quantityExpression,
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
	result, err := s.normalizeExpressionDepth(expression, 0, map[string]bool{})
	if err != nil && s.normalizationErr == nil {
		s.normalizationErr = err
	}
	return result
}

func (s *parseState) normalizeExpressionDepth(expression string, depth int, stack map[string]bool) (string, error) {
	result := strings.TrimSpace(expression)
	result = lowerInputCalls(result)
	result = s.resolveValueAliases(result)
	result = s.resolveSourceAliases(result)
	if unsupportedCallHistoryReference(result) {
		return result, fmt.Errorf("history references are supported only on identifiers or object fields; assign the function result first")
	}
	var err error
	result, err = s.expandUDFCalls(result, depth, stack)
	if err != nil {
		return result, err
	}
	result = strings.ReplaceAll(result, "strategy.position_avg_price", "position_avg_price")
	result = strings.ReplaceAll(result, "strategy.position_size", "position_size")
	result = strings.ReplaceAll(result, "strategy.equity", "equity")
	result = replaceSupportedRequestSecurity(result)
	result = strings.ReplaceAll(result, "syminfo.tickerid", "syminfo_tickerid")
	result = strings.ReplaceAll(result, "syminfo.prefix", "syminfo_prefix")
	result = strings.ReplaceAll(result, "timeframe.period", "timeframe_period")
	result = strings.ReplaceAll(result, "timeframe.isintraday", "timeframe_isintraday")
	result = strings.ReplaceAll(result, "timeframe.isminutes", "timeframe_isminutes")
	result = strings.ReplaceAll(result, "timeframe.isdaily", "timeframe_isdaily")
	result = strings.ReplaceAll(result, "timeframe.isweekly", "timeframe_isweekly")
	result = strings.ReplaceAll(result, "timeframe.ismonthly", "timeframe_ismonthly")
	result = replaceStringNamespace(result)
	result = replacePineNamespaceConstants(result)
	result = replaceColorFunctions(result)
	result = replaceTAMovingAverageFunction(result, "ema", "EMA")
	result = replaceTAMovingAverageFunction(result, "sma", "SMA")
	result = replaceTAMovingAverageFunction(result, "rma", "SMMA")
	result = replaceTAMovingAverageFunction(result, "wma", "LWMA")
	result = replaceTAMovingAverageFunction(result, "hma", "HMA")
	result = replaceTAMovingAverageFunction(result, "vwma", "VWMA")
	result = replaceTASourceLengthFunction(result, "rsi", "rsi", "close", "14")
	result = replaceTAMacd(result)
	result = replaceTAFunction(result, "atr", "atr(${period})")
	result = replaceTASourceLengthFunction(result, "stdev", "stdev", "close", "20")
	result = replaceTASourceLengthFunction(result, "variance", "variance", "close", "20")
	result = replaceTASourceLengthFunction(result, "cci", "cci", "hlc3", "20")
	result = replaceTAWindowFunction(result, "highest")
	result = replaceTAWindowFunction(result, "lowest")
	result = replaceTAWindowFunction(result, "change")
	result = replaceTAWindowFunction(result, "mom")
	result = replaceTAWindowFunction(result, "roc")
	result = replaceTAWindowFunction(result, "rising")
	result = replaceTAWindowFunction(result, "falling")
	result = replaceTAWindowFunction(result, "sum")
	result = replaceTAExtremaBarsFunction(result, "highestbars")
	result = replaceTAExtremaBarsFunction(result, "lowestbars")
	result = replaceTASourceRequiredFunction(result, "cum", "cum")
	result = replaceTAFunction(result, "wpr", "williams_r(${period})")
	result = replaceTASourceOptionalFunction(result, "vwap", "vwap", "hlc3")
	result = replaceTASourceLengthFunction(result, "mfi", "mfi", "hlc3", "14")
	result = replaceTAStoch(result)
	result = replaceTAFunction(result, "dmi", "dmi(${left}, ${right})")
	result = replaceTAFunction(result, "adx", "dmi(${left}, ${left}).adx")
	result = replaceTAFunction(result, "supertrend", "supertrend(${left}, ${right})")
	result = replaceTAFunction(result, "sar", "sar(${left}, ${right}, ${third})")
	result = replaceTATr(result)
	result = replaceTAStateFunction(result, "barssince")
	result = replaceTAStateFunction(result, "valuewhen")
	result = replaceTAFunction(result, "crossover", "cross_over(${left}, ${right})")
	result = replaceTAFunction(result, "crossunder", "cross_under(${left}, ${right})")
	result = replaceTAFunction(result, "cross", "(cross_over(${left}, ${right}) || cross_under(${left}, ${right}))")
	result = replaceMathNamespace(result)
	for alias, target := range s.expressionAliases {
		result = s.cachedRegexp("word:"+alias, `\b`+regexp.QuoteMeta(alias)+`\b`).ReplaceAllString(result, target)
	}
	result = normalizeHistoryReferences(result)
	result = normalizeTernaryExpression(result)
	result = strings.ReplaceAll(result, " and ", " && ")
	result = strings.ReplaceAll(result, " or ", " || ")
	return stripWrappingParens(strings.TrimSpace(result)), nil
}

func (s *parseState) expandUDFCalls(expression string, depth int, stack map[string]bool) (string, error) {
	if s == nil || len(s.udfs) == 0 {
		return expression, nil
	}
	if depth > maxUDFExpansionDepth {
		return expression, fmt.Errorf("user-defined function expansion exceeded depth %d", maxUDFExpansionDepth)
	}
	result := expression
	for {
		changed := false
		for name, udf := range s.udfs {
			start := s.findUDFCallStart(result, name)
			if start < 0 {
				continue
			}
			open := start + len(name)
			for open < len(result) && (result[open] == ' ' || result[open] == '\t') {
				open++
			}
			close := matchingParen(result, open)
			if close < 0 {
				return result, fmt.Errorf("invalid call to user-defined function %q", name)
			}
			args := splitArguments(result[open+1 : close])
			if len(args) == 1 && strings.TrimSpace(args[0]) == "" {
				args = nil
			}
			if len(args) != len(udf.Args) {
				return result, fmt.Errorf("user-defined function %q expects %d arguments, got %d", name, len(udf.Args), len(args))
			}
			if stack[name] {
				return result, fmt.Errorf("recursive user-defined function %q is not supported by JFTrade yet", name)
			}
			stack[name] = true
			replacements := make(map[string]string, len(args))
			for index, arg := range args {
				normalizedArg, err := s.normalizeExpressionDepth(strings.TrimSpace(arg), depth+1, stack)
				if err != nil {
					delete(stack, name)
					return result, err
				}
				replacements[udf.Args[index]] = udfArgumentReplacement(normalizedArg)
			}
			body := udf.Body
			for _, argName := range udf.Args {
				body = s.cachedRegexp("word:"+argName, `\b`+regexp.QuoteMeta(argName)+`\b`).ReplaceAllString(body, replacements[argName])
			}
			expanded, err := s.expandUDFCalls(body, depth+1, stack)
			delete(stack, name)
			if err != nil {
				return result, err
			}
			result = result[:start] + "(" + expanded + ")" + result[close+1:]
			changed = true
			break
		}
		if !changed {
			return result, nil
		}
	}
}

func (s *parseState) findUDFCallStart(expression string, name string) int {
	pattern := s.cachedRegexp("call:"+name, `\b`+regexp.QuoteMeta(name)+`\s*\(`)
	matches := pattern.FindAllStringIndex(expression, -1)
	for _, match := range matches {
		if match[0] > 0 && expression[match[0]-1] == '.' {
			continue
		}
		return match[0]
	}
	return -1
}

func udfArgumentReplacement(value string) string {
	trimmed := strings.TrimSpace(value)
	if memberPattern.MatchString(trimmed) ||
		numberPattern.MatchString(trimmed) ||
		trimmed == "true" || trimmed == "false" || trimmed == "na" {
		return trimmed
	}
	return "(" + trimmed + ")"
}

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

func lowerInputCalls(expression string) string {
	for {
		lower := strings.ToLower(expression)
		start := strings.Index(lower, "input(")
		dotStart := strings.Index(lower, "input.")
		if start < 0 || (dotStart >= 0 && dotStart < start) {
			start = dotStart
		}
		if start < 0 {
			return expression
		}
		open := strings.Index(expression[start:], "(")
		if open < 0 {
			return expression
		}
		open += start
		close := matchingParen(expression, open)
		if close < 0 {
			return expression
		}
		args := splitArguments(expression[open+1 : close])
		replacement := inputDefaultValue(args)
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceStringNamespace(expression string) string {
	prefix := "str.tostring("
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
		expression = expression[:start] + "tostring(" + expression[open+1:close] + ")" + expression[close+1:]
	}
}

func inputDefaultValue(args []string) string {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if ok && strings.EqualFold(strings.TrimSpace(key), "defval") {
			return strings.TrimSpace(value)
		}
	}
	if len(args) == 0 {
		return "na"
	}
	return strings.TrimSpace(args[0])
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
		source := defaultSource
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			source = strings.TrimSpace(args[0])
		}
		replacement := fmt.Sprintf("%s(%s)", target, source)
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

func replaceColorFunctions(expression string) string {
	expression = replaceColorNewFunction(expression)
	return replaceColorRGBFunction(expression)
}

func replaceColorNewFunction(expression string) string {
	prefix := "color.new("
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
		replacement := "\"#000000\""
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			replacement = strings.TrimSpace(args[0])
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func replaceColorRGBFunction(expression string) string {
	prefix := "color.rgb("
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
		replacement := "\"#000000\""
		if len(args) >= 3 {
			if red, redOK := parsePineColorComponent(args[0]); redOK {
				if green, greenOK := parsePineColorComponent(args[1]); greenOK {
					if blue, blueOK := parsePineColorComponent(args[2]); blueOK {
						replacement = fmt.Sprintf("\"#%02x%02x%02x\"", red, green, blue)
					}
				}
			}
		}
		expression = expression[:start] + replacement + expression[close+1:]
	}
}

func parsePineColorComponent(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	if parsed < 0 {
		parsed = 0
	}
	if parsed > 255 {
		parsed = 255
	}
	return parsed, true
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

func isOHLCVSource(expression string) bool {
	switch strings.TrimSpace(expression) {
	case "open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4":
		return true
	default:
		return false
	}
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

func replaceMathNamespace(expression string) string {
	for _, name := range []string{"abs", "min", "max", "round", "floor", "ceil", "sqrt", "pow", "log", "sign"} {
		expression = strings.ReplaceAll(expression, "math."+name, name)
	}
	return expression
}

func replacePineNamespaceConstants(expression string) string {
	replacements := map[string]string{
		"barstate.isfirst":       "barstate_isfirst",
		"barstate.isnew":         "barstate_isnew",
		"barstate.isconfirmed":   "barstate_isconfirmed",
		"barstate.ishistory":     "barstate_ishistory",
		"barstate.isrealtime":    "barstate_isrealtime",
		"barstate.islast":        "barstate_islast",
		"session.ismarket":       "session_ismarket",
		"session.ispremarket":    "session_ispremarket",
		"session.ispostmarket":   "session_ispostmarket",
		"dayofweek.sunday":       "1",
		"dayofweek.monday":       "2",
		"dayofweek.tuesday":      "3",
		"dayofweek.wednesday":    "4",
		"dayofweek.thursday":     "5",
		"dayofweek.friday":       "6",
		"dayofweek.saturday":     "7",
		"month.january":          "1",
		"month.february":         "2",
		"month.march":            "3",
		"month.april":            "4",
		"month.may":              "5",
		"month.june":             "6",
		"month.july":             "7",
		"month.august":           "8",
		"month.september":        "9",
		"month.october":          "10",
		"month.november":         "11",
		"month.december":         "12",
		"color.black":            "\"#000000\"",
		"color.white":            "\"#ffffff\"",
		"color.red":              "\"#ff5252\"",
		"color.green":            "\"#4caf50\"",
		"color.blue":             "\"#2196f3\"",
		"color.yellow":           "\"#ffeb3b\"",
		"color.orange":           "\"#ff9800\"",
		"color.purple":           "\"#9c27b0\"",
		"color.gray":             "\"#787b86\"",
		"color.grey":             "\"#787b86\"",
		"color.aqua":             "\"#00bcd4\"",
		"color.lime":             "\"#00e676\"",
		"color.maroon":           "\"#880e4f\"",
		"color.navy":             "\"#311b92\"",
		"color.olive":            "\"#808000\"",
		"color.silver":           "\"#b2b5be\"",
		"color.teal":             "\"#00897b\"",
		"color.fuchsia":          "\"#e040fb\"",
		"format.mintick":         "\"mintick\"",
		"format.percent":         "\"percent\"",
		"format.volume":          "\"volume\"",
		"barmerge.gaps_off":      "\"gaps_off\"",
		"barmerge.lookahead_off": "\"lookahead_off\"",
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return len(keys[left]) > len(keys[right])
	})
	result := expression
	for _, key := range keys {
		result = regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(key)+`\b`).ReplaceAllString(result, replacements[key])
	}
	return result
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
	if !supportedRequestSecurityMergeArgs(args[3:]) {
		return "", false
	}
	inner := strings.TrimSpace(args[2])
	if strings.Contains(strings.ToLower(inner), "request.security(") {
		return "", false
	}
	if source, lookback, ok := supportedRequestSecuritySourceHistory(inner); ok {
		if lookback > 0 {
			return fmt.Sprintf("security_source(%s, %s, %d)", source, timeUnit, lookback), true
		}
		return fmt.Sprintf("security_source(%s, %s)", source, timeUnit), true
	}
	name, innerArgs, ok := parseTACall(inner)
	if !ok || len(innerArgs) < 2 {
		return "", false
	}
	source, ok := supportedRequestSecuritySource(strings.TrimSpace(innerArgs[0]))
	if !ok {
		return "", false
	}
	maType, ok := pineMovingAverageType(name)
	if !ok {
		return "", false
	}
	if source == "close" {
		return fmt.Sprintf("ma(%s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit), true
	}
	return fmt.Sprintf("ma(%s, %s, %s, %s)", maType, strings.TrimSpace(innerArgs[1]), timeUnit, source), true
}

func supportedRequestSecurityMergeArgs(args []string) bool {
	for index, arg := range args {
		key, value, named := splitNamedArg(arg)
		if !named {
			switch index {
			case 0:
				key = "gaps"
				value = arg
			case 1:
				key = "lookahead"
				value = arg
			default:
				return false
			}
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "gaps":
			if !strings.EqualFold(strings.TrimSpace(value), "barmerge.gaps_off") {
				return false
			}
		case "lookahead":
			if !strings.EqualFold(strings.TrimSpace(value), "barmerge.lookahead_off") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func supportedRequestSecuritySourceHistory(expression string) (string, int, bool) {
	if source, ok := supportedRequestSecuritySource(expression); ok {
		return source, 0, true
	}
	matches := historyReferencePattern.FindStringSubmatch(strings.TrimSpace(expression))
	if len(matches) != 3 || strings.TrimSpace(matches[0]) != strings.TrimSpace(expression) {
		return "", 0, false
	}
	source, ok := supportedRequestSecuritySource(matches[1])
	if !ok {
		return "", 0, false
	}
	lookback, err := strconv.Atoi(strings.TrimSpace(matches[2]))
	if err != nil || lookback < 0 || lookback > maxHistoryLookback {
		return "", 0, false
	}
	return source, lookback, true
}

func supportedRequestSecuritySource(expression string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(expression)) {
	case "open":
		return "open", true
	case "high":
		return "high", true
	case "low":
		return "low", true
	case "close":
		return "close", true
	case "volume":
		return "volume", true
	case "hl2":
		return "hl2", true
	case "hlc3":
		return "hlc3", true
	case "ohlc4":
		return "ohlc4", true
	default:
		return "", false
	}
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

func rejectUnsupported(line parsedLine) error {
	if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
		return fmt.Errorf("pine line %d: %s", diagnostic.Line, diagnostic.Message)
	}
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.Contains(lower, "request.security("):
		if strings.Contains(strings.ToLower(replaceSupportedRequestSecurity(line.trimmed)), "request.security(") {
			if requestSecurityUsesTimeframeAlias(line.trimmed) {
				return nil
			}
			switch {
			case strings.Contains(lower, "barmerge.lookahead_on"):
				return fmt.Errorf("pine line %d: request.security() lookahead_on is not supported by JFTrade; use default lookahead_off", line.number)
			case strings.Contains(lower, "barmerge.gaps_on"):
				return fmt.Errorf("pine line %d: request.security() gaps_on is not supported by JFTrade; use default gaps_off", line.number)
			default:
				return fmt.Errorf("pine line %d: request.security() is supported only for syminfo.tickerid with OHLCV/hl2/hlc3/ohlc4 sources, source history, or source-aware moving averages on supported higher timeframes", line.number)
			}
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
	case unsupportedTAFunctionName(lower) != "":
		name := unsupportedTAFunctionName(lower)
		return diagnosticForLine(DiagnosticSeverityError, "PINE_TA_FUNCTION_UNSUPPORTED", fmt.Sprintf("ta.%s() is not supported by JFTrade yet", name), line), true
	case strings.HasPrefix(lower, "while "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_WHILE_UNSUPPORTED", "while loops are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "switch"):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_SWITCH_UNSUPPORTED", "switch statements are parsed but not executable in this JFTrade Pine v6 version", line), true
	case strings.HasPrefix(lower, "type "), strings.HasPrefix(lower, "method "), strings.HasPrefix(lower, "import "):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_DECLARATION_UNSUPPORTED", "Pine declarations, libraries, and methods are not executable in this JFTrade Pine v6 version", line), true
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return diagnosticForLine(DiagnosticSeverityError, "PINE_COLLECTION_UNSUPPORTED", "Pine collection namespaces array/matrix/map are not executable in this JFTrade Pine v6 version", line), true
	case historyDiagnosticMessage(line.trimmed) != "":
		return diagnosticForLine(DiagnosticSeverityError, "PINE_HISTORY_REF_UNSUPPORTED", historyDiagnosticMessage(line.trimmed), line), true
	default:
		return Diagnostic{}, false
	}
}

func requestSecurityUsesTimeframeAlias(expression string) bool {
	prefix := "request.security("
	start := strings.Index(strings.ToLower(expression), prefix)
	if start < 0 {
		return false
	}
	open := start + len(prefix) - 1
	close := matchingParen(expression, open)
	if close < 0 {
		return false
	}
	args := splitArguments(expression[open+1 : close])
	if len(args) < 2 {
		return false
	}
	timeframe := strings.TrimSpace(args[1])
	return identifierPattern.MatchString(timeframe)
}

func unsupportedTAFunctionName(lower string) string {
	for _, name := range []string{"linreg", "pivothigh", "pivotlow", "obv"} {
		if strings.Contains(lower, "ta."+name+"(") {
			return name
		}
	}
	return ""
}

func historyDiagnosticMessage(expression string) string {
	if unsupportedCallHistoryReference(expression) {
		return "history references are supported only on identifiers or object fields; assign the function result first"
	}
	for _, match := range historyReferenceMatchesOutsideStringLiterals(expression) {
		if len(match) != 3 {
			continue
		}
		lookback, err := strconv.Atoi(strings.TrimSpace(match[2]))
		if err != nil || lookback < 0 {
			return "history reference lookback must be a non-negative integer"
		}
		if lookback > maxHistoryLookback {
			return fmt.Sprintf("history reference lookback %d exceeds JFTrade maximum %d", lookback, maxHistoryLookback)
		}
	}
	return ""
}

func unsupportedCallHistoryReference(expression string) bool {
	hasCallHistory := false
	rewriteOutsideStringLiterals(expression, func(segment string) string {
		if callHistoryPattern.MatchString(segment) {
			hasCallHistory = true
		}
		return segment
	})
	return hasCallHistory
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

func parseStrategyDeclaration(line string) (strategyir.StrategyMetadata, []string) {
	metadata := strategyir.StrategyMetadata{
		Name:            "Pine Strategy",
		DefaultQtyMode:  "fixed",
		DefaultQtyValue: "1",
		Pyramiding:      1,
	}
	warnings := []string{}
	args := splitArguments(callArgs(line))
	if len(args) > 0 {
		if key, value, ok := splitNamedArg(args[0]); ok {
			if strings.EqualFold(key, "title") {
				if title := unquote(strings.TrimSpace(value)); title != "" {
					metadata.Name = title
				}
			}
		} else if title := unquote(strings.TrimSpace(args[0])); title != "" {
			metadata.Name = title
		}
	}
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "title":
			if title := unquote(strings.TrimSpace(value)); title != "" {
				metadata.Name = title
			}
		case "default_qty_type":
			if mode, ok := normalizeStrategyDefaultQtyMode(value); ok {
				metadata.DefaultQtyMode = mode
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy default_qty_type %q is not supported by JFTrade; using strategy.fixed", strings.TrimSpace(value)))
			}
		case "default_qty_value":
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				metadata.DefaultQtyValue = trimmed
			}
		case "pyramiding":
			if parsed, ok := parseStrategyPyramiding(value); ok {
				metadata.Pyramiding = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy pyramiding %q is not a supported constant integer; using 1", strings.TrimSpace(value)))
			}
		case "initial_capital":
			if parsed, ok := parsePositiveFloatConstant(value); ok {
				metadata.InitialCapital = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy initial_capital %q must be a positive constant number", strings.TrimSpace(value)))
			}
		case "commission_type":
			if parsed, ok := normalizeStrategyCommissionType(value); ok {
				metadata.CommissionType = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy commission_type %q is not supported by JFTrade", strings.TrimSpace(value)))
			}
		case "commission_value":
			if parsed, ok := parseNonNegativeFloatConstant(value); ok {
				metadata.CommissionValue = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy commission_value %q must be a non-negative constant number", strings.TrimSpace(value)))
			}
		case "slippage":
			if parsed, ok := parseNonNegativeIntConstant(value); ok {
				metadata.Slippage = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy slippage %q must be a non-negative constant integer", strings.TrimSpace(value)))
			}
		case "process_orders_on_close":
			if parsed, ok := parseBoolConstant(value); ok {
				metadata.ProcessOnClose = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("pine strategy process_orders_on_close %q must be true or false", strings.TrimSpace(value)))
			}
		}
	}
	return metadata, warnings
}

func parsePositiveFloatConstant(value string) (float64, bool) {
	parsed, ok := parseNonNegativeFloatConstant(value)
	return parsed, ok && parsed > 0
}

func parseNonNegativeFloatConstant(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(stripWrappingParens(value)), 64)
	return parsed, err == nil && parsed >= 0
}

func parseNonNegativeIntConstant(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(stripWrappingParens(value)))
	return parsed, err == nil && parsed >= 0
}

func parseBoolConstant(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(stripWrappingParens(value))) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func normalizeStrategyCommissionType(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.commission.")
	switch normalized {
	case "percent", "cash_per_order", "cash_per_contract":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeStrategyDefaultQtyMode(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "strategy.")
	switch normalized {
	case "", "fixed":
		return "fixed", true
	case "cash":
		return "cash", true
	case "percent_of_equity":
		return "percent_of_equity", true
	default:
		return "", false
	}
}

func parseStrategyPyramiding(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(stripWrappingParens(value)))
	if err != nil || parsed < 0 {
		return 1, false
	}
	if parsed == 0 {
		return 1, true
	}
	return parsed, true
}

func (s *parseState) pineEntryQuantity(args []string) (string, string) {
	if quantityMode, quantityExpression, ok := pineExplicitQuantity(args); ok {
		return quantityMode, quantityExpression
	}
	mode := s.strategyMetadata.DefaultQtyMode
	if strings.TrimSpace(mode) == "" {
		mode = "fixed"
	}
	value := strings.TrimSpace(s.strategyMetadata.DefaultQtyValue)
	if value == "" {
		value = "1"
	}
	switch mode {
	case "percent_of_equity":
		return "account_position_percent", value
	case "cash":
		return "amount", value
	case "fixed":
		fallthrough
	default:
		return "shares", value
	}
}

func pineQuantity(args []string) (string, string) {
	if quantityMode, quantityExpression, ok := pineExplicitQuantity(args); ok {
		return quantityMode, quantityExpression
	}
	return "shares", "1"
}

func pineExplicitQuantity(args []string) (string, string, bool) {
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "qty_percent":
			return "account_position_percent", value, true
		case "qty":
			if percent, ok := pineEquityPercent(value); ok {
				return "account_position_percent", percent, true
			}
			if amount, ok := pineAmountQuantity(value); ok {
				return "amount", amount, true
			}
			return "shares", value, true
		}
	}
	return "", "", false
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
		case "qty_percent":
			return "symbol_position_percent", value
		case "qty":
			return "shares", value
		}
	}
	return "symbol_position_percent", "100"
}

func pineExitQuantity(args []string) (string, string) {
	return pineCloseQuantity(args, "")
}

func rejectUnsupportedOrderArgs(lineNumber int, functionName string, args []string) error {
	for _, name := range []string{"oca_name", "oca_type"} {
		if hasNamedArg(args, name) {
			return fmt.Errorf("pine line %d: %s argument %s is parsed but not executable by JFTrade yet", lineNumber, functionName, name)
		}
	}
	return nil
}

func pineOrderMetadata(lineNumber int, functionName string, args []string, allowImmediate bool) (string, string, bool, error) {
	comment := ""
	if raw, ok := namedArgValue(args, "comment"); ok {
		comment = unquote(strings.TrimSpace(raw))
	}
	alertMessage := ""
	if raw, ok := namedArgValue(args, "alert_message"); ok {
		alertMessage = unquote(strings.TrimSpace(raw))
	}
	disableAlert := false
	if raw, ok := namedArgValue(args, "disable_alert"); ok {
		value, valid := parseBoolConstant(raw)
		if !valid {
			return "", "", false, fmt.Errorf("pine line %d: %s disable_alert must be true or false", lineNumber, functionName)
		}
		disableAlert = value
	}
	if hasNamedArg(args, "immediately") && !allowImmediate {
		return "", "", false, fmt.Errorf("pine line %d: %s does not support immediately", lineNumber, functionName)
	}
	return comment, alertMessage, disableAlert, nil
}

func pineCloseMetadata(lineNumber int, functionName string, args []string) (string, string, bool, bool, error) {
	comment, alertMessage, disableAlert, err := pineOrderMetadata(lineNumber, functionName, args, true)
	if err != nil {
		return "", "", false, false, err
	}
	immediate := false
	if raw, ok := namedArgValue(args, "immediately"); ok {
		value, valid := parseBoolConstant(raw)
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: %s immediately must be true or false", lineNumber, functionName)
		}
		immediate = value
	}
	return comment, alertMessage, disableAlert, immediate, nil
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

func pineOrderPrices(args []string) (string, string, string) {
	orderType := "MARKET"
	limitExpr := ""
	stopExpr := ""
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "limit") {
			orderType = "LIMIT"
			limitExpr = strings.TrimSpace(value)
		}
		if strings.EqualFold(key, "stop") {
			stopExpr = strings.TrimSpace(value)
		}
	}
	return orderType, limitExpr, stopExpr
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
		strings.HasPrefix(lower, "barcolor(") ||
		strings.HasPrefix(lower, "fill(") ||
		strings.HasPrefix(lower, "alertcondition(") ||
		strings.HasPrefix(lower, "label.new(") ||
		strings.HasPrefix(lower, "line.new(") ||
		strings.HasPrefix(lower, "box.new(") ||
		strings.HasPrefix(lower, "table.")
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
