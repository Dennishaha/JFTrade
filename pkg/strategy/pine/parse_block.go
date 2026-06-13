package pine

import (
	"fmt"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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
