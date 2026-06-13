package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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
