package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type switchArm struct {
	condition string
	value     string
	line      parsedLine
}

func (s *parseState) parseSwitch(index int) (strategyir.Statement, int, bool, error) {
	line := s.lines[index]
	if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
		raw := strings.TrimSpace(match[4])
		if strings.EqualFold(raw, "switch") || strings.HasPrefix(strings.ToLower(raw), "switch ") {
			selector := strings.TrimSpace(raw[len("switch"):])
			arms, nextIndex, err := s.readSwitchArms(index+1, line.indent, selector)
			if err != nil {
				return nil, index, true, err
			}
			expression, err := s.lowerSwitchExpression(line.number, arms)
			if err != nil {
				return nil, index, true, err
			}
			return &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: line.number, EndLine: arms[len(arms)-1].line.number},
				Name:       strings.TrimSpace(match[2]),
				Expression: expression,
				Mode:       assignmentMode(strings.TrimSpace(match[1]), strings.TrimSpace(match[3])),
			}, nextIndex, true, nil
		}
	}
	lower := strings.ToLower(strings.TrimSpace(line.trimmed))
	if lower != "switch" && !strings.HasPrefix(lower, "switch ") {
		return nil, index, false, nil
	}
	selector := strings.TrimSpace(line.trimmed[len("switch"):])
	arms, nextIndex, err := s.readSwitchArms(index+1, line.indent, selector)
	if err != nil {
		return nil, index, true, err
	}
	statement, err := s.lowerSwitchStatement(line.number, arms)
	return statement, nextIndex, true, err
}

func (s *parseState) readSwitchArms(start, parentIndent int, selector string) ([]switchArm, int, error) {
	arms := make([]switchArm, 0)
	index := start
	for index < len(s.lines) && s.lines[index].indent > parentIndent {
		line := s.lines[index]
		parts := strings.SplitN(line.trimmed, "=>", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, index, fmt.Errorf("pine line %d: switch arms must use condition => expression/statement", line.number)
		}
		condition := strings.TrimSpace(parts[0])
		if selector != "" && condition != "" {
			condition = "(" + selector + ") == (" + condition + ")"
		}
		arms = append(arms, switchArm{condition: condition, value: strings.TrimSpace(parts[1]), line: line})
		index++
	}
	if len(arms) == 0 {
		return nil, index, fmt.Errorf("pine switch requires at least one arm")
	}
	return arms, index, nil
}

func (s *parseState) lowerSwitchExpression(lineNumber int, arms []switchArm) (string, error) {
	fallback := "na"
	for index := len(arms) - 1; index >= 0; index-- {
		arm := arms[index]
		value := s.normalizeExpression(arm.value)
		if err := s.takeNormalizationErr(arm.line.number); err != nil {
			return "", err
		}
		if arm.condition == "" {
			fallback = value
			continue
		}
		condition := s.normalizeExpression(arm.condition)
		if err := s.takeNormalizationErr(arm.line.number); err != nil {
			return "", err
		}
		fallback = fmt.Sprintf("ifelse(%s, %s, %s)", condition, value, fallback)
	}
	if err := validateExpression(lineNumber, "switch expression", fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

func (s *parseState) lowerSwitchStatement(lineNumber int, arms []switchArm) (strategyir.Statement, error) {
	var fallback []strategyir.Statement
	for index := len(arms) - 1; index >= 0; index-- {
		arm := arms[index]
		statement, err := s.parseInlineSwitchStatement(arm)
		if err != nil {
			return nil, err
		}
		if arm.condition == "" {
			fallback = []strategyir.Statement{statement}
			continue
		}
		condition := s.normalizeExpression(arm.condition)
		if err := s.takeNormalizationErr(arm.line.number); err != nil {
			return nil, err
		}
		if err := validateExpression(arm.line.number, "switch condition", condition); err != nil {
			return nil, err
		}
		fallback = []strategyir.Statement{&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: lineNumber, EndLine: arm.line.number},
			Condition: condition,
			Then:      []strategyir.Statement{statement},
			Else:      fallback,
		}}
	}
	if len(fallback) != 1 {
		return nil, fmt.Errorf("pine line %d: switch statement requires a conditional arm", lineNumber)
	}
	return fallback[0], nil
}

func (s *parseState) parseInlineSwitchStatement(arm switchArm) (strategyir.Statement, error) {
	line := arm.line
	line.trimmed = arm.value
	if statement, ok, err := s.parseAssignment(line); ok || err != nil {
		return statement, err
	}
	if statement, ok, err := s.parseStrategyCall(line); ok || err != nil {
		return statement, err
	}
	if statement, ok := parseLogOrAlert(line); ok {
		return statement, nil
	}
	return nil, fmt.Errorf("pine line %d: unsupported switch statement %q", line.number, arm.value)
}
