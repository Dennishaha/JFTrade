package pine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const SourceFormatPineV6 = "pine-v6"
const maxHistoryLookback = 500
const maxUDFExpansionDepth = 8
const maxStaticForIterations = 100
const maxStaticForDepth = 2
const maxRuntimeLoopDepth = 4
const maxRuntimeLoopIterations = 1000

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
	lines                []parsedLine
	warnings             []string
	longEntryIDs         map[string]bool
	shortEntryIDs        map[string]bool
	udfs                 map[string]pineUDF
	expressionAliases    map[string]string
	sourceAliases        map[string]string
	valueAliases         map[string]string
	collectionNamespaces map[string]string
	udtTypes             map[string]strategyir.TypeDefinition
	udtMethods           map[string][]strategyir.MethodDefinition
	objectTypes          map[string]string
	objectPersistent     map[string]bool
	typeDefinitions      []strategyir.TypeDefinition
	methodDefinitions    []strategyir.MethodDefinition
	loopVariables        map[string]bool
	forDepth             int
	runtimeLoopDepth     int
	normalizationErr     error
	entryPolicyCache     map[int]string
	strategyMetadata     strategyir.StrategyMetadata
	regexpCache          map[string]*regexp.Regexp
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

//nolint:funlen
func compileLoweredAST(script string, lines []parsedLine, ast *AST) (Compilation, error) {
	state := &parseState{
		lines:                nil,
		longEntryIDs:         map[string]bool{},
		shortEntryIDs:        map[string]bool{},
		udfs:                 map[string]pineUDF{},
		expressionAliases:    map[string]string{},
		sourceAliases:        map[string]string{},
		valueAliases:         map[string]string{},
		collectionNamespaces: map[string]string{},
		udtTypes:             map[string]strategyir.TypeDefinition{},
		udtMethods:           map[string][]strategyir.MethodDefinition{},
		objectTypes:          map[string]string{},
		objectPersistent:     map[string]bool{},
		loopVariables:        map[string]bool{},
		entryPolicyCache:     buildEntryPolicyCache(script),
		regexpCache:          map[string]*regexp.Regexp{},
	}
	state.lines = parsedLinesFromStructuredAST(ast, lines)
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
	program.Metadata = state.strategyMetadata
	program.Types = append([]strategyir.TypeDefinition(nil), state.typeDefinitions...)
	program.Methods = append([]strategyir.MethodDefinition(nil), state.methodDefinitions...)
	program.Hooks = []strategyir.HookBlock{{
		Kind:       strategyir.HookKLineClose,
		Range:      strategyir.SourceRange{StartLine: statements[0].SourceRange().StartLine, EndLine: statements[len(statements)-1].SourceRange().EndLine},
		Statements: statements,
	}}
	return Compilation{Program: program, Warnings: state.warnings}, nil
}

func parsedLinesFromStructuredAST(ast *AST, fallback []parsedLine) []parsedLine {
	if ast == nil || len(ast.Nodes) == 0 {
		return fallback
	}
	byLine := make(map[int]parsedLine, len(fallback))
	for _, line := range fallback {
		byLine[line.number] = line
	}
	result := make([]parsedLine, 0, len(fallback))
	var appendNodes func([]ASTNode)
	appendNodes = func(nodes []ASTNode) {
		for _, node := range nodes {
			if line, ok := byLine[node.Line.Line]; ok {
				result = append(result, line)
			}
			appendNodes(node.Children)
		}
	}
	appendNodes(ast.Nodes)
	if len(result) != len(fallback) {
		return fallback
	}
	return result
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
	if len(args) > 0 && !strings.Contains(args[0], "=") {
		value := args[0]
		if percent, ok := pineEquityPercent(value); ok {
			return "account_position_percent", percent, true
		}
		if amount, ok := pineAmountQuantity(value); ok {
			return "amount", amount, true
		}
		return "shares", value, true
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
	if len(args) > 0 && !strings.Contains(args[0], "=") {
		return "shares", args[0]
	}
	if len(args) > 1 && !strings.Contains(args[1], "=") {
		return "symbol_position_percent", args[1]
	}
	return "symbol_position_percent", "100"
}

func pineExitQuantity(args []string) (string, string) {
	return pineCloseQuantity(args, "")
}

func rejectConflictingQuantityArgs(lineNumber int, functionName string, args []string) error {
	if hasNamedArg(args, "qty") && hasNamedArg(args, "qty_percent") {
		return fmt.Errorf("pine line %d: %s supports qty or qty_percent, not both", lineNumber, functionName)
	}
	return nil
}

func rejectUnsupportedNamedArgs(lineNumber int, functionName string, args []string, allowedNames ...string) error {
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	for _, arg := range args {
		key, _, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		if _, exists := allowed[strings.ToLower(strings.TrimSpace(key))]; exists {
			continue
		}
		return fmt.Errorf("pine line %d: %s argument %s is not supported by JFTrade", lineNumber, functionName, key)
	}
	return nil
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

func pineCloseAllMetadata(lineNumber int, args []string) (string, string, bool, bool, error) {
	comment, alertMessage, disableAlert, immediate, err := pineCloseMetadata(lineNumber, "strategy.close_all", args)
	if err != nil {
		return "", "", false, false, err
	}
	if _, ok := namedArgValue(args, "immediately"); !ok && len(args) > 0 && !strings.Contains(args[0], "=") {
		value, valid := parseBoolConstant(args[0])
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all immediately must be true or false", lineNumber)
		}
		immediate = value
	}
	if _, ok := namedArgValue(args, "comment"); !ok && len(args) > 1 && !strings.Contains(args[1], "=") {
		comment = unquote(strings.TrimSpace(args[1]))
	}
	if _, ok := namedArgValue(args, "alert_message"); !ok && len(args) > 2 && !strings.Contains(args[2], "=") {
		alertMessage = unquote(strings.TrimSpace(args[2]))
	}
	if _, ok := namedArgValue(args, "disable_alert"); !ok && len(args) > 3 && !strings.Contains(args[3], "=") {
		value, valid := parseBoolConstant(args[3])
		if !valid {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all disable_alert must be true or false", lineNumber)
		}
		disableAlert = value
	}
	for index := 4; index < len(args); index++ {
		if !strings.Contains(args[index], "=") {
			return "", "", false, false, fmt.Errorf("pine line %d: strategy.close_all supports positional immediately, comment, alert_message, and disable_alert only", lineNumber)
		}
	}
	return comment, alertMessage, disableAlert, immediate, nil
}

type pineExitMetadataFields struct {
	comment         string
	commentProfit   string
	commentLoss     string
	commentTrailing string
	alertMessage    string
	alertProfit     string
	alertLoss       string
	alertTrailing   string
	disableAlert    bool
}

func pineExitMetadata(lineNumber int, args []string) (pineExitMetadataFields, error) {
	comment, alertMessage, disableAlert, err := pineOrderMetadata(lineNumber, "strategy.exit", args, false)
	if err != nil {
		return pineExitMetadataFields{}, err
	}
	commentProfit := ""
	if raw, ok := namedArgValue(args, "comment_profit"); ok {
		commentProfit = unquote(strings.TrimSpace(raw))
	}
	commentLoss := ""
	if raw, ok := namedArgValue(args, "comment_loss"); ok {
		commentLoss = unquote(strings.TrimSpace(raw))
	}
	commentTrailing := ""
	if raw, ok := namedArgValue(args, "comment_trailing"); ok {
		commentTrailing = unquote(strings.TrimSpace(raw))
	}
	alertProfit := ""
	if raw, ok := namedArgValue(args, "alert_profit"); ok {
		alertProfit = unquote(strings.TrimSpace(raw))
	}
	alertLoss := ""
	if raw, ok := namedArgValue(args, "alert_loss"); ok {
		alertLoss = unquote(strings.TrimSpace(raw))
	}
	alertTrailing := ""
	if raw, ok := namedArgValue(args, "alert_trailing"); ok {
		alertTrailing = unquote(strings.TrimSpace(raw))
	}
	return pineExitMetadataFields{
		comment:         comment,
		commentProfit:   commentProfit,
		commentLoss:     commentLoss,
		commentTrailing: commentTrailing,
		alertMessage:    alertMessage,
		alertProfit:     alertProfit,
		alertLoss:       alertLoss,
		alertTrailing:   alertTrailing,
		disableAlert:    disableAlert,
	}, nil
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
		case '(':
			depth++
		case ')':
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
