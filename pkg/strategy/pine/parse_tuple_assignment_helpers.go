package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) parseRequestSecurityTupleAssignment(line parsedLine, aliases []string, expression string) (strategyir.Statement, bool, error) {
	args, ok := requestSecurityCallArgs(expression)
	if !ok {
		return nil, false, nil
	}
	lowered, tupleOK := lowerSupportedRequestSecurityTuple(args)
	if !tupleOK {
		return nil, false, nil
	}
	if len(lowered) != len(aliases) {
		return nil, true, fmt.Errorf("pine line %d: request.security tuple returns %d values but assignment has %d aliases", line.number, len(lowered), len(aliases))
	}
	normalized, err := s.normalizeTupleExpressions(line.number, lowered)
	if err != nil {
		return nil, true, err
	}
	for index := 1; index < len(aliases); index++ {
		s.expressionAliases[aliases[index]] = normalized[index]
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       aliases[0],
		Expression: normalized[0],
	}, true, nil
}

func tupleAssignmentExpressionParts(expression string) (string, []string) {
	tupleExpression := expression
	if lowered := replaceSupportedRequestSecurity(expression); lowered != expression {
		tupleExpression = stripWrappingParens(lowered)
	}
	return tupleExpression, splitArguments(callArgs(tupleExpression))
}

func (s *parseState) parseBollingerTupleAssignment(line parsedLine, aliases []string, args []string, tupleExpression string) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(tupleExpression)
	if strings.HasPrefix(lower, "ta.bb(") {
		if len(args) < 3 {
			return nil, true, fmt.Errorf("pine line %d: ta.bb(source, length, mult) requires three arguments", line.number)
		}
		expr := fmt.Sprintf("bollinger(%s, %s)", s.normalizeExpression(args[1]), s.normalizeExpression(args[2]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		return s.buildBandTupleAssignment(line, aliases, expr)
	}
	if !strings.HasPrefix(lower, "bollinger(") {
		return nil, false, nil
	}
	if len(args) != 4 {
		return nil, true, fmt.Errorf("pine line %d: MTF ta.bb requires length, multiplier, static timeframe, and source", line.number)
	}
	return s.buildBandTupleAssignment(line, aliases, fmt.Sprintf("bollinger(%s, %s, %s, %s)", args[0], args[1], args[2], args[3]))
}

func (s *parseState) parseDMITupleAssignment(line parsedLine, aliases []string, args []string, tupleExpression string) (strategyir.Statement, bool, error) {
	if !strings.HasPrefix(strings.ToLower(tupleExpression), "ta.dmi(") {
		return nil, false, nil
	}
	if len(args) < 2 {
		return nil, true, fmt.Errorf("pine line %d: ta.dmi(diLength, adxSmoothing) requires two arguments", line.number)
	}
	expr := fmt.Sprintf("dmi(%s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, true, err
	}
	return s.buildDirectionalTupleAssignment(line, aliases, expr, ".minus", ".adx")
}

func (s *parseState) parseSupertrendTupleAssignment(line parsedLine, aliases []string, args []string, tupleExpression string) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(tupleExpression)
	if strings.HasPrefix(lower, "ta.supertrend(") {
		if len(args) < 2 {
			return nil, true, fmt.Errorf("pine line %d: ta.supertrend(factor, atrPeriod) requires two arguments", line.number)
		}
		expr := fmt.Sprintf("supertrend(%s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		return s.buildDirectionalTupleAssignment(line, aliases, expr, ".direction")
	}
	if !strings.HasPrefix(lower, "supertrend(") {
		return nil, false, nil
	}
	if len(args) != 3 {
		return nil, true, fmt.Errorf("pine line %d: MTF ta.supertrend requires factor, ATR period, and static timeframe", line.number)
	}
	return s.buildDirectionalTupleAssignment(line, aliases, fmt.Sprintf("supertrend(%s, %s, %s)", args[0], args[1], args[2]), ".direction")
}

func (s *parseState) parseKeltnerTupleAssignment(line parsedLine, aliases []string, args []string, tupleExpression string) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(tupleExpression)
	if strings.HasPrefix(lower, "ta.kc(") {
		if len(args) < 3 || len(args) > 4 {
			return nil, true, fmt.Errorf("pine line %d: ta.kc(source, length, mult, useTrueRange?) requires three or four arguments", line.number)
		}
		useTR := "true"
		if len(args) == 4 {
			useTR = s.normalizeExpression(args[3])
		}
		expr := fmt.Sprintf("kc(%s, %s, %s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]), s.normalizeExpression(args[2]), useTR)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		return s.buildBandTupleAssignment(line, aliases, expr)
	}
	if !strings.HasPrefix(lower, "kc(") {
		return nil, false, nil
	}
	if len(args) != 5 {
		return nil, true, fmt.Errorf("pine line %d: MTF ta.kc requires source, length, mult, useTrueRange, and static timeframe", line.number)
	}
	return s.buildBandTupleAssignment(line, aliases, fmt.Sprintf("kc(%s, %s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4]))
}

func (s *parseState) parseMACDTupleAssignment(line parsedLine, aliases []string, args []string, tupleExpression string) (strategyir.Statement, bool, error) {
	lower := strings.ToLower(tupleExpression)
	if strings.HasPrefix(lower, "macd(") {
		if len(args) != 5 {
			return nil, true, fmt.Errorf("pine line %d: MTF ta.macd requires fast, slow, signal, static timeframe, and source", line.number)
		}
		return s.buildDirectionalTupleAssignment(line, aliases, fmt.Sprintf("macd(%s, %s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4]), ".signal", ".histogram")
	}
	if !strings.HasPrefix(lower, "ta.macd(") {
		return nil, false, nil
	}
	if len(args) < 4 {
		return nil, true, fmt.Errorf("pine line %d: ta.macd(source, fast, slow, signal) requires four arguments", line.number)
	}
	expr := fmt.Sprintf("macd(%s, %s, %s)", s.normalizeExpression(args[1]), s.normalizeExpression(args[2]), s.normalizeExpression(args[3]))
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, true, err
	}
	return s.buildDirectionalTupleAssignment(line, aliases, expr, ".signal", ".histogram")
}

func (s *parseState) normalizeTupleExpressions(lineNumber int, expressions []string) ([]string, error) {
	normalized := make([]string, len(expressions))
	for index, expression := range expressions {
		normalized[index] = s.normalizeExpression(expression)
		if err := s.takeNormalizationErr(lineNumber); err != nil {
			return nil, err
		}
		if err := validateExpression(lineNumber, "assignment expression", normalized[index]); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func (s *parseState) buildBandTupleAssignment(line parsedLine, aliases []string, expr string) (strategyir.Statement, bool, error) {
	if len(aliases) > 1 && aliases[1] != "" {
		s.expressionAliases[aliases[1]] = aliases[0] + ".upper"
	}
	if len(aliases) > 2 && aliases[2] != "" {
		s.expressionAliases[aliases[2]] = aliases[0] + ".lower"
	}
	return buildTupleLetStatement(line, aliases[0], expr)
}

func (s *parseState) buildDirectionalTupleAssignment(line parsedLine, aliases []string, expr string, suffixes ...string) (strategyir.Statement, bool, error) {
	for index, suffix := range suffixes {
		aliasIndex := index + 1
		if aliasIndex < len(aliases) && aliases[aliasIndex] != "" {
			s.expressionAliases[aliases[aliasIndex]] = aliases[0] + suffix
		}
	}
	return buildTupleLetStatement(line, aliases[0], expr)
}

func buildTupleLetStatement(line parsedLine, alias string, expr string) (strategyir.Statement, bool, error) {
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       alias,
		Expression: expr,
	}, true, validateExpression(line.number, "assignment expression", expr)
}
