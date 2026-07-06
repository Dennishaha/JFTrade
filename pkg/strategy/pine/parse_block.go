package pine

import (
	"errors"
	"fmt"
	"maps"
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
	s.warnVaripUsage(line, lower)
	if nextIndex, handled, err := s.parseStatementDeclaration(index, line, lower); handled || err != nil {
		return nil, nextIndex, err
	}
	if err := rejectPublicDisabledHelperCalls(line.number, line.trimmed); err != nil {
		return nil, index, err
	}
	if statement, nextIndex, handled, err := s.parseStatementControlFlow(index, line, lower); handled || err != nil {
		return statement, nextIndex, err
	}
	if err := s.rejectUnsupported(line); err != nil {
		return nil, index, err
	}
	if statement, nextIndex, handled, err := s.parseStructuredStatement(index, line, lower); handled || err != nil {
		return statement, nextIndex, err
	}
	return s.parseSimpleStatement(index, line, lower)
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
		for _, bodyLine := range s.lines[nextIndex:endIndex] {
			if err := rejectPublicDisabledHelperCalls(bodyLine.number, bodyLine.trimmed); err != nil {
				return true, index, err
			}
		}
		compiledBody, compileErr := compileUDFBody(s.lines[nextIndex:endIndex])
		if compileErr != nil {
			return true, index, fmt.Errorf("pine line %d: user-defined function %q: %w", line.number, name, compileErr)
		}
		body = compiledBody
		nextIndex = endIndex
	} else if err := rejectPublicDisabledHelperCalls(line.number, body); err != nil {
		return true, index, err
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
	maps.Copy(branchLocals, locals)
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
	normalized := strings.ToLower(strings.TrimSpace(name))
	if _, disabled := publicDisabledHelperNames[normalized]; disabled {
		return true
	}
	switch normalized {
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
	match, ok := parseStaticForLoopHeader(loopHeader)
	if !ok {
		return nil, index, fmt.Errorf("pine line %d: for loop must use 'for i = start to end [by step]'", line.number)
	}
	spec, err := s.parseStaticForLoopSpec(line.number, match)
	if err != nil {
		if errors.Is(err, errStaticForRuntimeFallback) {
			return s.parseRuntimeForLoop(index, match)
		}
		return nil, index, err
	}
	return s.expandStaticForLoop(index, line, match, spec)
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
