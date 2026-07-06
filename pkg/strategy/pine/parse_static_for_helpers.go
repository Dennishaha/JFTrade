package pine

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type staticForLoopSpec struct {
	variable string
	values   []int
}

func parseStaticForLoopHeader(loopHeader string) ([]string, bool) {
	match := forLoopPattern.FindStringSubmatch(loopHeader)
	return match, match != nil
}

func (s *parseState) parseStaticForLoopSpec(lineNumber int, match []string) (staticForLoopSpec, error) {
	loopVar := strings.TrimSpace(match[1])
	start, err := s.parseStaticIntExpression(lineNumber, match[2], "for start")
	if err != nil {
		return staticForLoopSpec{}, err
	}
	end, err := s.parseStaticIntExpression(lineNumber, match[3], "for end")
	if err != nil {
		return staticForLoopSpec{}, err
	}
	step, err := s.parseStaticForLoopStep(lineNumber, match[4])
	if err != nil {
		return staticForLoopSpec{}, err
	}
	values, err := expandStaticForLoopValues(lineNumber, start, end, step)
	if err != nil {
		return staticForLoopSpec{}, err
	}
	return staticForLoopSpec{variable: loopVar, values: values}, nil
}

func (s *parseState) parseStaticForLoopStep(lineNumber int, rawStep string) (int, error) {
	if strings.TrimSpace(rawStep) == "" {
		return 1, nil
	}
	return s.parseStaticIntExpression(lineNumber, rawStep, "for step")
}

func expandStaticForLoopValues(lineNumber int, start int, end int, step int) ([]int, error) {
	if step == 0 {
		return nil, fmt.Errorf("pine line %d: for loop step cannot be 0", lineNumber)
	}
	if (step > 0 && start > end) || (step < 0 && start < end) {
		return nil, fmt.Errorf("pine line %d: for loop step does not reach the end value", lineNumber)
	}

	values := make([]int, 0)
	for value := start; ; value += step {
		if (step > 0 && value > end) || (step < 0 && value < end) {
			break
		}
		values = append(values, value)
		if len(values) > maxStaticForIterations {
			return nil, fmt.Errorf("pine line %d: for loop expands to more than %d iterations", lineNumber, maxStaticForIterations)
		}
		if value == end {
			break
		}
	}
	if len(values) == 0 || values[len(values)-1] != end {
		return nil, fmt.Errorf("pine line %d: for loop step does not reach the end value", lineNumber)
	}
	return values, nil
}

func (s *parseState) expandStaticForLoop(index int, line parsedLine, match []string, spec staticForLoopSpec) ([]strategyir.Statement, int, error) {
	previousValue, hadValue := s.valueAliases[spec.variable]
	previousLoopVar := s.loopVariables[spec.variable]
	s.loopVariables[spec.variable] = true
	s.forDepth++
	defer func() {
		s.forDepth--
		if hadValue {
			s.valueAliases[spec.variable] = previousValue
		} else {
			delete(s.valueAliases, spec.variable)
		}
		if previousLoopVar {
			s.loopVariables[spec.variable] = true
		} else {
			delete(s.loopVariables, spec.variable)
		}
	}()

	return s.executeStaticForLoop(index, line, match, spec)
}

func (s *parseState) executeStaticForLoop(index int, line parsedLine, match []string, spec staticForLoopSpec) ([]strategyir.Statement, int, error) {
	statements := make([]strategyir.Statement, 0)
	nextIndex := index + 1
	loopBodyEnd := s.staticForBodyEnd(index, line.indent)
	for _, value := range spec.values {
		s.valueAliases[spec.variable] = strconv.Itoa(value)
		bodyStatements, afterBody, err := s.parseBlock(index+1, line.indent)
		switch {
		case errors.Is(err, errStaticForConditionalControl):
			return s.parseRuntimeForLoop(index, match)
		case errors.Is(err, errStaticForBreak):
			statements = append(statements, bodyStatements...)
			return statements, loopBodyEnd, nil
		case errors.Is(err, errStaticForContinue):
			statements = append(statements, bodyStatements...)
			nextIndex = loopBodyEnd
			continue
		case err != nil:
			return nil, index, err
		}
		nextIndex = afterBody
		statements = append(statements, bodyStatements...)
	}
	return statements, nextIndex, nil
}
