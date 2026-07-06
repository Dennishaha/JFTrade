package pine

import (
	"errors"
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) warnVaripUsage(line parsedLine, lower string) {
	if strings.HasPrefix(lower, "varip ") {
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: varip uses closed-bar var semantics in JFTrade", line.number))
	}
}

func (s *parseState) parseStatementDeclaration(index int, line parsedLine, lower string) (int, bool, error) {
	switch {
	case strings.HasPrefix(lower, "type "):
		nextIndex, err := s.parseExecutableTypeDefinition(index)
		return nextIndex, true, err
	case strings.HasPrefix(lower, "method "):
		nextIndex, err := s.parseExecutableMethodDefinition(index)
		return nextIndex, true, err
	case strings.HasPrefix(lower, "export "):
		return s.skipDeclarationBlock(index), true, nil
	default:
		if ok, nextIndex, err := s.parseUDFDefinition(index); ok || err != nil {
			return nextIndex, true, err
		}
		return index, false, nil
	}
}

func (s *parseState) parseStatementControlFlow(index int, line parsedLine, lower string) (strategyir.Statement, int, bool, error) {
	if lower == "break" || lower == "continue" {
		statement, nextIndex, err := s.parseLoopControlStatement(index, line, lower)
		return statement, nextIndex, true, err
	}
	if statement, ok, err := s.parseCollectionStatement(line); ok || err != nil {
		return statement, index + 1, true, err
	}
	if statement, ok, err := s.parseObjectStatement(line); ok || err != nil {
		return statement, index + 1, true, err
	}
	if strings.HasPrefix(lower, "while ") {
		statement, nextIndex, err := s.parseRuntimeWhileLoop(index)
		return statement, nextIndex, true, err
	}
	return nil, index, false, nil
}

func (s *parseState) parseLoopControlStatement(index int, line parsedLine, lower string) (strategyir.Statement, int, error) {
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

func (s *parseState) parseStructuredStatement(index int, line parsedLine, lower string) (strategyir.Statement, int, bool, error) {
	if statement, nextIndex, ok, err := s.parseSwitch(index); ok || err != nil {
		return statement, nextIndex, true, err
	}
	if strings.HasPrefix(lower, "if ") {
		statement, nextIndex, err := s.parseIfStatement(index, line)
		return statement, nextIndex, true, err
	}
	if strings.EqualFold(line.trimmed, "else") {
		return nil, index, true, fmt.Errorf("pine line %d: else must follow an if block", line.number)
	}
	return nil, index, false, nil
}

func (s *parseState) parseIfStatement(index int, line parsedLine) (strategyir.Statement, int, error) {
	condition, err := s.normalizeIfCondition(line)
	if err != nil {
		return nil, index, err
	}
	thenStatements, nextIndex, err := s.parseConditionalBlock(index+1, line)
	if err != nil {
		return nil, index, err
	}

	statement := &strategyir.IfStmt{
		Range:     strategyir.SourceRange{StartLine: line.number, EndLine: conditionalEndLine(line.number, thenStatements)},
		Condition: condition,
		Then:      thenStatements,
	}
	if !s.hasMatchingElse(line, nextIndex) {
		if len(statement.Then) == 0 {
			return nil, nextIndex, nil
		}
		return statement, nextIndex, nil
	}

	elseStatements, afterElse, err := s.parseConditionalBlock(nextIndex+1, line)
	if err != nil {
		return nil, index, err
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

func (s *parseState) normalizeIfCondition(line parsedLine) (string, error) {
	condition := strings.TrimSpace(strings.TrimPrefix(line.trimmed, "if "))
	condition = strings.TrimSuffix(condition, ":")
	condition = s.normalizeExpression(condition)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return "", err
	}
	if err := validateExpression(line.number, "if condition", condition); err != nil {
		return "", err
	}
	return condition, nil
}

func (s *parseState) parseConditionalBlock(startIndex int, line parsedLine) ([]strategyir.Statement, int, error) {
	statements, nextIndex, err := s.parseBlock(startIndex, line.indent)
	if err != nil {
		if errors.Is(err, errStaticForBreak) || errors.Is(err, errStaticForContinue) {
			return nil, nextIndex, errStaticForConditionalControl
		}
		return nil, nextIndex, err
	}
	return statements, nextIndex, nil
}

func conditionalEndLine(lineNumber int, statements []strategyir.Statement) int {
	if len(statements) == 0 {
		return lineNumber
	}
	return statements[len(statements)-1].SourceRange().EndLine
}

func (s *parseState) hasMatchingElse(line parsedLine, nextIndex int) bool {
	return nextIndex < len(s.lines) &&
		s.lines[nextIndex].indent == line.indent &&
		strings.EqualFold(s.lines[nextIndex].trimmed, "else")
}

func (s *parseState) parseSimpleStatement(index int, line parsedLine, lower string) (strategyir.Statement, int, error) {
	if isVisualOnlyCall(lower) {
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: visual-only call %q is ignored by JFTrade", line.number, callName(line.trimmed)))
		return nil, index + 1, nil
	}
	parsers := []func(parsedLine) (strategyir.Statement, bool, error){
		s.parseGeneralTupleAssignment,
		s.parseTupleAssignment,
		s.parseAssignment,
		s.parseStrategyCall,
	}
	for _, parser := range parsers {
		if statement, ok, err := parser(line); ok || err != nil {
			return statement, index + 1, err
		}
	}
	if statement, ok := parseLogOrAlert(line); ok {
		return statement, index + 1, nil
	}
	return nil, index, fmt.Errorf("pine line %d: unsupported executable statement %q", line.number, line.trimmed)
}
