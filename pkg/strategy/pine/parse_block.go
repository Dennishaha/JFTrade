package pine

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var errStaticForBreak = errors.New("static for break")
var errStaticForContinue = errors.New("static for continue")
var errStaticForConditionalControl = errors.New("static for conditional break or continue")

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
			if errors.Is(err, errStaticForBreak) || errors.Is(err, errStaticForContinue) {
				return statements, nextIndex, err
			}
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
	if strings.HasPrefix(lower, "varip ") {
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: varip uses closed-bar var semantics in JFTrade", line.number))
	}
	if strings.HasPrefix(lower, "type ") {
		nextIndex, err := s.parseExecutableTypeDefinition(index)
		return nil, nextIndex, err
	}
	if strings.HasPrefix(lower, "method ") {
		nextIndex, err := s.parseExecutableMethodDefinition(index)
		return nil, nextIndex, err
	}
	if strings.HasPrefix(lower, "export ") {
		return nil, s.skipDeclarationBlock(index), nil
	}
	if ok, nextIndex, err := s.parseUDFDefinition(index); ok || err != nil {
		return nil, nextIndex, err
	}
	if lower == "break" || lower == "continue" {
		if s.runtimeLoopDepth > 0 {
			if lower == "break" {
				return &strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}}, index + 1, nil
			}
			return &strategyir.ContinueStmt{Range: strategyir.SourceRange{StartLine: line.number, EndLine: line.number}}, index + 1, nil
		}
		if s.forDepth > 0 {
			if lower == "break" {
				return nil, index + 1, errStaticForBreak
			}
			return nil, index + 1, errStaticForContinue
		}
		return nil, index, fmt.Errorf("pine line %d: %s is not supported in JFTrade static for loops yet", line.number, line.trimmed)
	}
	if statement, ok, err := s.parseCollectionStatement(line); ok || err != nil {
		return statement, index + 1, err
	}
	if statement, ok, err := s.parseObjectStatement(line); ok || err != nil {
		return statement, index + 1, err
	}
	if strings.HasPrefix(lower, "while ") {
		return s.parseRuntimeWhileLoop(index)
	}
	if err := s.rejectUnsupported(line); err != nil {
		return nil, index, err
	}
	if statement, nextIndex, ok, err := s.parseSwitch(index); ok || err != nil {
		return statement, nextIndex, err
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
			if errors.Is(err, errStaticForBreak) || errors.Is(err, errStaticForContinue) {
				return nil, index, errStaticForConditionalControl
			}
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
				if errors.Is(elseErr, errStaticForBreak) || errors.Is(elseErr, errStaticForContinue) {
					return nil, index, errStaticForConditionalControl
				}
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
	if statement, ok, err := s.parseGeneralTupleAssignment(line); ok || err != nil {
		return statement, index + 1, err
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

func (s *parseState) skipDeclarationBlock(index int) int {
	next := index + 1
	if index < 0 || index >= len(s.lines) {
		return next
	}
	indent := s.lines[index].indent
	for next < len(s.lines) && s.lines[next].indent > indent {
		next++
	}
	return next
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
		endIndex := nextIndex
		for endIndex < len(s.lines) && s.lines[endIndex].indent > line.indent {
			endIndex++
		}
		compiledBody, compileErr := compileUDFBody(s.lines[nextIndex:endIndex])
		if compileErr != nil {
			return true, index, fmt.Errorf("pine line %d: user-defined function %q: %w", line.number, name, compileErr)
		}
		body = compiledBody
		nextIndex = endIndex
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

func compileUDFBody(lines []parsedLine) (string, error) {
	if len(lines) == 0 {
		return "", fmt.Errorf("requires a return expression")
	}
	baseIndent := lines[0].indent
	locals := map[string]string{}
	var result string
	for index := 0; index < len(lines); {
		line := lines[index]
		if line.indent != baseIndent {
			return "", fmt.Errorf("unexpected indentation at line %d", line.number)
		}
		lower := strings.ToLower(strings.TrimSpace(line.trimmed))
		if strings.HasPrefix(lower, "if ") {
			condition := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "if "))
			thenExpression, nextIndex, err := udfBranchExpression(lines, index+1, line.indent, locals)
			if err != nil {
				return "", err
			}
			if nextIndex >= len(lines) || lines[nextIndex].indent != line.indent || !strings.EqualFold(lines[nextIndex].trimmed, "else") {
				return "", fmt.Errorf("if return at line %d requires else", line.number)
			}
			elseExpression, afterElse, err := udfBranchExpression(lines, nextIndex+1, line.indent, locals)
			if err != nil {
				return "", err
			}
			result = fmt.Sprintf("ifelse(%s, %s, %s)",
				substituteUDFArgs(condition, locals),
				thenExpression,
				elseExpression,
			)
			index = afterElse
			if index != len(lines) {
				return "", fmt.Errorf("final if/else return must be the last function statement")
			}
			break
		}
		if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			if strings.TrimSpace(match[3]) != "=" {
				return "", fmt.Errorf("local reassignment is not supported")
			}
			locals[strings.TrimSpace(match[2])] = substituteUDFArgs(strings.TrimSpace(match[4]), locals)
			index++
			continue
		}
		result = substituteUDFArgs(line.trimmed, locals)
		index++
		if index != len(lines) {
			return "", fmt.Errorf("return expression must be the last function statement")
		}
	}
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("requires a return expression")
	}
	return result, nil
}

func udfBranchExpression(lines []parsedLine, start, parentIndent int, locals map[string]string) (string, int, error) {
	if start >= len(lines) || lines[start].indent <= parentIndent {
		return "", start, fmt.Errorf("if/else branch requires an expression")
	}
	branchIndent := lines[start].indent
	index := start
	branchLocals := make(map[string]string, len(locals))
	for key, value := range locals {
		branchLocals[key] = value
	}
	result := ""
	for index < len(lines) && lines[index].indent > parentIndent {
		line := lines[index]
		if line.indent != branchIndent {
			return "", index, fmt.Errorf("nested blocks in UDF branches are not supported")
		}
		if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			branchLocals[strings.TrimSpace(match[2])] = substituteUDFArgs(strings.TrimSpace(match[4]), branchLocals)
		} else {
			result = substituteUDFArgs(line.trimmed, branchLocals)
		}
		index++
	}
	if strings.TrimSpace(result) == "" {
		return "", index, fmt.Errorf("if/else branch requires a return expression")
	}
	return result, index, nil
}

func substituteUDFArgs(expression string, replacements map[string]string) string {
	result := expression
	for name, value := range replacements {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		result = pattern.ReplaceAllString(result, udfArgumentReplacement(value))
	}
	return result
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
	if match := collectionForLoopPattern.FindStringSubmatch(loopHeader); match != nil {
		return s.parseCollectionForLoop(index, match)
	}
	match := forLoopPattern.FindStringSubmatch(loopHeader)
	if match == nil {
		return nil, index, fmt.Errorf("pine line %d: for loop must use 'for i = start to end [by step]'", line.number)
	}
	loopVar := strings.TrimSpace(match[1])
	start, err := s.parseStaticIntExpression(line.number, match[2], "for start")
	if err != nil {
		return s.parseRuntimeForLoop(index, match)
	}
	end, err := s.parseStaticIntExpression(line.number, match[3], "for end")
	if err != nil {
		return s.parseRuntimeForLoop(index, match)
	}
	step := 1
	if strings.TrimSpace(match[4]) != "" {
		step, err = s.parseStaticIntExpression(line.number, match[4], "for step")
		if err != nil {
			return s.parseRuntimeForLoop(index, match)
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
	loopBodyEnd := s.staticForBodyEnd(index, line.indent)
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
		if errors.Is(err, errStaticForConditionalControl) {
			return s.parseRuntimeForLoop(index, match)
		}
		if errors.Is(err, errStaticForBreak) {
			statements = append(statements, bodyStatements...)
			nextIndex = loopBodyEnd
			break
		}
		if errors.Is(err, errStaticForContinue) {
			statements = append(statements, bodyStatements...)
			nextIndex = loopBodyEnd
			continue
		}
		if err != nil {
			return nil, index, err
		}
		nextIndex = afterBody
		statements = append(statements, bodyStatements...)
	}
	return statements, nextIndex, nil
}

func (s *parseState) parseRuntimeForLoop(index int, match []string) ([]strategyir.Statement, int, error) {
	line := s.lines[index]
	if s.runtimeLoopDepth >= maxRuntimeLoopDepth {
		return nil, index, fmt.Errorf("pine line %d: dynamic loop nesting exceeds %d", line.number, maxRuntimeLoopDepth)
	}
	loopVar := strings.TrimSpace(match[1])
	start := s.normalizeExpression(strings.TrimSpace(match[2]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, index, err
	}
	end := s.normalizeExpression(strings.TrimSpace(match[3]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, index, err
	}
	step := "1"
	if strings.TrimSpace(match[4]) != "" {
		step = s.normalizeExpression(strings.TrimSpace(match[4]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, index, err
		}
	}
	for label, expression := range map[string]string{"for start": start, "for end": end, "for step": step} {
		if err := validateExpression(line.number, label, expression); err != nil {
			return nil, index, err
		}
	}
	previousLoopVar := s.loopVariables[loopVar]
	s.loopVariables[loopVar] = true
	s.runtimeLoopDepth++
	body, nextIndex, err := s.parseBlock(index+1, line.indent)
	s.runtimeLoopDepth--
	if previousLoopVar {
		s.loopVariables[loopVar] = true
	} else {
		delete(s.loopVariables, loopVar)
	}
	if err != nil {
		return nil, index, err
	}
	endLine := line.number
	if len(body) > 0 {
		endLine = body[len(body)-1].SourceRange().EndLine
	}
	return []strategyir.Statement{&strategyir.LoopStmt{
		Range:           strategyir.SourceRange{StartLine: line.number, EndLine: endLine},
		Variable:        loopVar,
		StartExpression: start,
		EndExpression:   end,
		StepExpression:  step,
		Body:            body,
		MaxIterations:   maxRuntimeLoopIterations,
	}}, nextIndex, nil
}

func (s *parseState) parseCollectionForLoop(index int, match []string) ([]strategyir.Statement, int, error) {
	line := s.lines[index]
	if s.runtimeLoopDepth >= maxRuntimeLoopDepth {
		return nil, index, fmt.Errorf("pine line %d: dynamic loop nesting exceeds %d", line.number, maxRuntimeLoopDepth)
	}
	indexVar := strings.TrimSpace(match[1])
	valueVar := strings.TrimSpace(match[2])
	if valueVar == "" {
		valueVar = strings.TrimSpace(match[3])
	}
	collection := s.normalizeExpression(strings.TrimSpace(match[4]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, index, err
	}
	if err := validateExpression(line.number, "collection for source", collection); err != nil {
		return nil, index, err
	}
	previousIndexLoopVar := false
	if indexVar != "" && indexVar != "_" {
		previousIndexLoopVar = s.loopVariables[indexVar]
		s.loopVariables[indexVar] = true
	}
	previousValueLoopVar := false
	if valueVar != "" && valueVar != "_" {
		previousValueLoopVar = s.loopVariables[valueVar]
		s.loopVariables[valueVar] = true
	}
	s.runtimeLoopDepth++
	body, nextIndex, err := s.parseBlock(index+1, line.indent)
	s.runtimeLoopDepth--
	if indexVar != "" && indexVar != "_" {
		if previousIndexLoopVar {
			s.loopVariables[indexVar] = true
		} else {
			delete(s.loopVariables, indexVar)
		}
	}
	if valueVar != "" && valueVar != "_" {
		if previousValueLoopVar {
			s.loopVariables[valueVar] = true
		} else {
			delete(s.loopVariables, valueVar)
		}
	}
	if err != nil {
		return nil, index, err
	}
	endLine := line.number
	if len(body) > 0 {
		endLine = body[len(body)-1].SourceRange().EndLine
	}
	return []strategyir.Statement{&strategyir.LoopStmt{
		Range:         strategyir.SourceRange{StartLine: line.number, EndLine: endLine},
		Variable:      valueVar,
		IndexVariable: indexVar,
		Collection:    collection,
		Body:          body,
		MaxIterations: maxRuntimeLoopIterations,
	}}, nextIndex, nil
}

func (s *parseState) parseRuntimeWhileLoop(index int) (strategyir.Statement, int, error) {
	line := s.lines[index]
	if s.runtimeLoopDepth >= maxRuntimeLoopDepth {
		return nil, index, fmt.Errorf("pine line %d: dynamic loop nesting exceeds %d", line.number, maxRuntimeLoopDepth)
	}
	condition := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line.trimmed, "while "), ":"))
	condition = s.normalizeExpression(condition)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, index, err
	}
	if err := validateExpression(line.number, "while condition", condition); err != nil {
		return nil, index, err
	}
	s.runtimeLoopDepth++
	body, nextIndex, err := s.parseBlock(index+1, line.indent)
	s.runtimeLoopDepth--
	if err != nil {
		return nil, index, err
	}
	endLine := line.number
	if len(body) > 0 {
		endLine = body[len(body)-1].SourceRange().EndLine
	}
	return &strategyir.LoopStmt{
		Range:          strategyir.SourceRange{StartLine: line.number, EndLine: endLine},
		WhileCondition: condition,
		Body:           body,
		MaxIterations:  maxRuntimeLoopIterations,
	}, nextIndex, nil
}

func (s *parseState) staticForBodyEnd(index int, parentIndent int) int {
	next := index + 1
	for next < len(s.lines) && s.lines[next].indent > parentIndent {
		next++
	}
	return next
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
