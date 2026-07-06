package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) parseGeneralTupleAssignment(line parsedLine) (strategyir.Statement, bool, error) {
	match := generalTuplePattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	rawNames := splitArguments(match[1])
	if len(rawNames) < 2 || len(rawNames) > 8 {
		return nil, true, fmt.Errorf("pine line %d: tuple assignment supports 2 to 8 aliases", line.number)
	}
	names := make([]string, 0, len(rawNames))
	for _, raw := range rawNames {
		name := strings.TrimSpace(raw)
		if name != "_" && !identifierPattern.MatchString(name) {
			return nil, true, fmt.Errorf("pine line %d: invalid tuple alias %q", line.number, name)
		}
		names = append(names, name)
	}
	rawExpression := strings.TrimSpace(match[3])
	var expressions []string
	if len(rawExpression) >= 2 && rawExpression[0] == '[' && rawExpression[len(rawExpression)-1] == ']' {
		expressions = splitArguments(rawExpression[1 : len(rawExpression)-1])
	} else if args, ok := requestSecurityCallArgs(rawExpression); ok {
		lowered, tupleOK := lowerSupportedRequestSecurityTupleGeneral(args)
		if !tupleOK || len(names) <= 3 {
			return nil, false, nil
		}
		expressions = lowered
	} else {
		return nil, false, nil
	}
	if len(expressions) != len(names) {
		return nil, true, fmt.Errorf("pine line %d: tuple returns %d values but assignment has %d aliases", line.number, len(expressions), len(names))
	}
	normalized := make([]string, len(expressions))
	for index, expression := range expressions {
		normalized[index] = s.normalizeExpression(expression)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "tuple expression", normalized[index]); err != nil {
			return nil, true, err
		}
	}
	mode := strategyir.AssignmentModeLet
	if strings.TrimSpace(match[2]) == ":=" {
		mode = strategyir.AssignmentModeReassign
	}
	return &strategyir.TupleStmt{
		Range:       strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Names:       names,
		Expressions: normalized,
		Mode:        mode,
	}, true, nil
}

//nolint:funlen
func (s *parseState) parseTupleAssignment(line parsedLine) (strategyir.Statement, bool, error) {
	match := tupleAssignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	aliases := tupleAssignmentAliases(match)
	expression := strings.TrimSpace(match[5])
	if args, ok := requestSecurityCallArgs(expression); ok {
		if lowered, tupleOK := lowerSupportedRequestSecurityTuple(args); tupleOK {
			if len(lowered) != len(aliases) {
				return nil, true, fmt.Errorf("pine line %d: request.security tuple returns %d values but assignment has %d aliases", line.number, len(lowered), len(aliases))
			}
			for index := range lowered {
				lowered[index] = s.normalizeExpression(lowered[index])
				if err := s.takeNormalizationErr(line.number); err != nil {
					return nil, true, err
				}
				if err := validateExpression(line.number, "assignment expression", lowered[index]); err != nil {
					return nil, true, err
				}
			}
			for index := 1; index < len(aliases); index++ {
				s.expressionAliases[aliases[index]] = lowered[index]
			}
			return &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
				Name:       aliases[0],
				Expression: lowered[0],
			}, true, nil
		}
	}
	tupleExpression := expression
	if lowered := replaceSupportedRequestSecurity(expression); lowered != expression {
		tupleExpression = stripWrappingParens(lowered)
	}
	lower := strings.ToLower(tupleExpression)
	args := splitArguments(callArgs(tupleExpression))
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
	if strings.HasPrefix(lower, "bollinger(") {
		if len(args) != 4 {
			return nil, true, fmt.Errorf("pine line %d: MTF ta.bb requires length, multiplier, static timeframe, and source", line.number)
		}
		basisAlias := strings.TrimSpace(match[1])
		upperAlias := strings.TrimSpace(match[2])
		lowerAlias := strings.TrimSpace(match[3])
		expr := fmt.Sprintf("bollinger(%s, %s, %s, %s)", args[0], args[1], args[2], args[3])
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
	if strings.HasPrefix(lower, "supertrend(") {
		if len(args) != 3 {
			return nil, true, fmt.Errorf("pine line %d: MTF ta.supertrend requires factor, ATR period, and static timeframe", line.number)
		}
		alias := strings.TrimSpace(match[1])
		directionAlias := strings.TrimSpace(match[2])
		expr := fmt.Sprintf("supertrend(%s, %s, %s)", args[0], args[1], args[2])
		if directionAlias != "" {
			s.expressionAliases[directionAlias] = alias + ".direction"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       alias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if strings.HasPrefix(lower, "ta.kc(") {
		if len(args) < 3 || len(args) > 4 {
			return nil, true, fmt.Errorf("pine line %d: ta.kc(source, length, mult, useTrueRange?) requires three or four arguments", line.number)
		}
		alias := strings.TrimSpace(match[1])
		upperAlias := strings.TrimSpace(match[2])
		lowerAlias := strings.TrimSpace(match[3])
		useTR := "true"
		if len(args) == 4 {
			useTR = s.normalizeExpression(args[3])
		}
		expr := fmt.Sprintf("kc(%s, %s, %s, %s)", s.normalizeExpression(args[0]), s.normalizeExpression(args[1]), s.normalizeExpression(args[2]), useTR)
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if upperAlias != "" {
			s.expressionAliases[upperAlias] = alias + ".upper"
		}
		if lowerAlias != "" {
			s.expressionAliases[lowerAlias] = alias + ".lower"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       alias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if strings.HasPrefix(lower, "kc(") {
		if len(args) != 5 {
			return nil, true, fmt.Errorf("pine line %d: MTF ta.kc requires source, length, mult, useTrueRange, and static timeframe", line.number)
		}
		alias := strings.TrimSpace(match[1])
		upperAlias := strings.TrimSpace(match[2])
		lowerAlias := strings.TrimSpace(match[3])
		expr := fmt.Sprintf("kc(%s, %s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
		if upperAlias != "" {
			s.expressionAliases[upperAlias] = alias + ".upper"
		}
		if lowerAlias != "" {
			s.expressionAliases[lowerAlias] = alias + ".lower"
		}
		return &strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Name:       alias,
			Expression: expr,
		}, true, validateExpression(line.number, "assignment expression", expr)
	}
	if strings.HasPrefix(lower, "macd(") {
		if len(args) != 5 {
			return nil, true, fmt.Errorf("pine line %d: MTF ta.macd requires fast, slow, signal, static timeframe, and source", line.number)
		}
		alias := strings.TrimSpace(match[1])
		signalAlias := strings.TrimSpace(match[2])
		histAlias := strings.TrimSpace(match[3])
		expr := fmt.Sprintf("macd(%s, %s, %s, %s, %s)", args[0], args[1], args[2], args[3], args[4])
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
	if !strings.HasPrefix(lower, "ta.macd(") {
		return nil, true, fmt.Errorf("pine line %d: tuple assignment is supported only for ta.macd(...), ta.bb(...), ta.dmi(...), ta.supertrend(...), ta.kc(...), or whitelisted request.security tuples", line.number)
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

func tupleAssignmentAliases(match []string) []string {
	aliases := make([]string, 0, 3)
	for _, raw := range match[1:4] {
		alias := strings.TrimSpace(raw)
		if alias != "" {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

func requestSecurityCallArgs(expression string) ([]string, bool) {
	trimmed := strings.TrimSpace(expression)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "request.security(") {
		return nil, false
	}
	open := strings.Index(trimmed, "(")
	if open < 0 {
		return nil, false
	}
	close := matchingParen(trimmed, open)
	if close != len(trimmed)-1 {
		return nil, false
	}
	return splitArguments(trimmed[open+1 : close]), true
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
	rawExpression := strings.TrimSpace(match[4])
	mode := assignmentMode(strings.TrimSpace(match[1]), operator)
	if mode == strategyir.AssignmentModeReassign {
		delete(s.sourceAliases, name)
		delete(s.valueAliases, name)
		delete(s.expressionAliases, name)
	}
	if call, _, ok := parseFunctionCallText(rawExpression); ok && isVisualCallName(call) {
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: visual-only assignment %q is ignored by JFTrade", line.number, call))
		return nil, true, nil
	}
	expression := s.normalizeExpression(rawExpression)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return nil, true, err
	}
	if expression == "" {
		return nil, true, fmt.Errorf("pine line %d: assignment expression is required", line.number)
	}
	if err := validateExpression(line.number, "assignment expression", expression); err != nil {
		return nil, true, err
	}
	if mode != strategyir.AssignmentModeReassign {
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
	}
	if namespace, ok := s.collectionNamespaces[strings.ToLower(strings.TrimSpace(rawExpression))]; ok {
		s.collectionNamespaces[strings.ToLower(name)] = namespace
	} else {
		delete(s.collectionNamespaces, strings.ToLower(name))
	}
	return &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:       name,
		Expression: expression,
		Mode:       mode,
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
