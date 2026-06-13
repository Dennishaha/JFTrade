package ir

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

var divergenceCallPattern = regexp.MustCompile(`divergence_(top|bottom)\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*,\s*([0-9]+)\s*\)`)
var rsiCallPattern = regexp.MustCompile(`\brsi\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var stdevCallPattern = regexp.MustCompile(`\bstdev\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var varianceCallPattern = regexp.MustCompile(`\bvariance\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var cciCallPattern = regexp.MustCompile(`\bcci\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var cumCallPattern = regexp.MustCompile(`\bcum\s*\(\s*([^)]+?)\s*\)`)
var stochCallPattern = regexp.MustCompile(`\bstoch\s*\(\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var vwapCallPattern = regexp.MustCompile(`\bvwap\s*\(\s*([^)]+?)\s*\)`)
var mfiCallPattern = regexp.MustCompile(`\bmfi\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var dmiCallPattern = regexp.MustCompile(`\bdmi\s*\(\s*([0-9]+)\s*,\s*([0-9]+)\s*\)`)
var supertrendCallPattern = regexp.MustCompile(`\bsupertrend\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+)\s*\)`)
var sarCallPattern = regexp.MustCompile(`\bsar\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*\)`)
var securitySourceCallPattern = regexp.MustCompile(`\bsecurity_source\s*\(\s*([^,]+?)\s*,\s*([^,\)]+?)(?:\s*,\s*([0-9]+)\s*)?\)`)
var windowCallPattern = regexp.MustCompile(`\b(highest|lowest|highestbars|lowestbars|change|mom|roc|rising|falling|sum)\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var positionVariablePattern = regexp.MustCompile(`\b(position_size|position_avg_price)\b`)
var equityVariablePattern = regexp.MustCompile(`\bequity\b`)

type Requirements struct {
	Indicators                []IndicatorRequirement
	RequiresPosition          bool
	RequiresTotalAccountValue bool
}

type IndicatorRequirement struct {
	Alias string
	Kind  string
	Key   string
}

type plannedBinding struct {
	Alias string
	Kind  string
	Key   string
	Args  []string
}

func PlanRequirements(program *Program) (Requirements, error) {
	if program == nil {
		return Requirements{}, fmt.Errorf("strategy program is required")
	}

	result := Requirements{}
	indicatorByKey := map[string]IndicatorRequirement{}

	for _, hook := range program.Hooks {
		bindings := map[string]plannedBinding{}
		if err := planStatements(hook.Statements, bindings, indicatorByKey, &result); err != nil {
			return Requirements{}, err
		}
	}

	keys := make([]string, 0, len(indicatorByKey))
	for key := range indicatorByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result.Indicators = make([]IndicatorRequirement, 0, len(keys))
	for _, key := range keys {
		result.Indicators = append(result.Indicators, indicatorByKey[key])
	}

	return result, nil
}

func planStatements(
	statements []Statement,
	bindings map[string]plannedBinding,
	indicatorByKey map[string]IndicatorRequirement,
	result *Requirements,
) error {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *LetStmt:
			if expressionRequiresPosition(typed.Expression) {
				result.RequiresPosition = true
			}
			if expressionRequiresTotalAccountValue(typed.Expression) {
				result.RequiresTotalAccountValue = true
			}
			binding, recognized, err := parseIndicatorBinding(typed)
			if err != nil {
				return err
			}
			if recognized {
				bindings[typed.Name] = binding
				indicatorByKey[binding.Key] = IndicatorRequirement{
					Alias: binding.Alias,
					Kind:  binding.Kind,
					Key:   binding.Key,
				}
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.Expression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
		case *IfStmt:
			if expressionRequiresPosition(typed.Condition) {
				result.RequiresPosition = true
			}
			if expressionRequiresTotalAccountValue(typed.Condition) {
				result.RequiresTotalAccountValue = true
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.Condition)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			for _, requirement := range collectConditionRequirements(typed.Condition, bindings) {
				indicatorByKey[requirement.Key] = requirement
			}

			thenBindings := cloneBindings(bindings)
			if err := planStatements(typed.Then, thenBindings, indicatorByKey, result); err != nil {
				return err
			}

			elseBindings := cloneBindings(bindings)
			if err := planStatements(typed.Else, elseBindings, indicatorByKey, result); err != nil {
				return err
			}
		case *OrderStmt:
			quantityMode, ok := indicatorbinding.ParseQuantityMode(typed.QuantityMode)
			if !ok {
				return fmt.Errorf("pine line %d: unsupported order quantity mode %q", typed.Range.StartLine, typed.QuantityMode)
			}
			result.RequiresPosition = true
			switch quantityMode {
			case "account_position_percent":
				result.RequiresTotalAccountValue = true
			}
			requirements, err := collectExpressionRequirements(typed.Range.StartLine, typed.QuantityExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			requirements, err = collectExpressionRequirements(typed.Range.StartLine, typed.LimitExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
			requirements, err = collectExpressionRequirements(typed.Range.StartLine, typed.StopExpression)
			if err != nil {
				return err
			}
			for _, requirement := range requirements {
				indicatorByKey[requirement.Key] = requirement
			}
		case *ExitStmt:
			if _, ok := indicatorbinding.ParseQuantityMode(typed.QuantityMode); !ok {
				return fmt.Errorf("pine line %d: unsupported exit quantity mode %q", typed.Range.StartLine, typed.QuantityMode)
			}
			result.RequiresPosition = true
			for _, expression := range []string{typed.QuantityExpression, typed.StopExpression, typed.LimitExpression, typed.TrailPoints, typed.TrailOffset} {
				if expressionRequiresPosition(expression) {
					result.RequiresPosition = true
				}
				if expressionRequiresTotalAccountValue(expression) {
					result.RequiresTotalAccountValue = true
				}
				requirements, err := collectExpressionRequirements(typed.Range.StartLine, expression)
				if err != nil {
					return err
				}
				for _, requirement := range requirements {
					indicatorByKey[requirement.Key] = requirement
				}
			}
		case *CancelStmt:
			continue
		case *ProtectStmt:
			result.RequiresPosition = true
			key, err := buildProtectRequirementKey(typed)
			if err != nil {
				return err
			}
			indicatorByKey[key] = IndicatorRequirement{Kind: "protect", Key: key}
		case *LogStmt, *NotifyStmt:
			continue
		default:
			return fmt.Errorf("unsupported IR statement type %T", statement)
		}
	}

	return nil
}

func parseIndicatorBinding(statement *LetStmt) (plannedBinding, bool, error) {
	name, args, ok := indicatorbinding.ParseFunctionCall(statement.Expression)
	if !ok {
		return plannedBinding{}, false, nil
	}

	switch indicatorbinding.NormalizeFunctionName(name) {
	case "ma":
		if len(args) < 2 || len(args) > 4 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() requires type, period, optional time unit, and optional source", statement.Range.StartLine)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() type %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() period must be a positive integer", statement.Range.StartLine)
		}
		timeUnit, source, err := indicatorbinding.ParseMovingAverageOptionalArgs(args[2:])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		key := indicatorbinding.BuildMovingAverageKeyWithSource(averageType, period, timeUnit, source)
		return plannedBinding{Alias: statement.Name, Kind: "ma", Key: key, Args: []string{averageType, strconv.Itoa(period), timeUnit, source}}, true, nil
	case "rsi":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "14")
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := sourcePeriodKey("rsi", source, period, "close")
		return plannedBinding{Alias: statement.Name, Kind: "rsi", Key: key, Args: sourcePeriodArgs(source, period, "close")}, true, nil
	case "macd":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 3)
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := fmt.Sprintf("macd:%d:%d:%d", values[0], values[1], values[2])
		return plannedBinding{Alias: statement.Name, Kind: "macd", Key: key, Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "kdj":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 3)
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := fmt.Sprintf("kdj:%d:%d:%d", values[0], values[1], values[2])
		return plannedBinding{Alias: statement.Name, Kind: "kdj", Key: key, Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "atr":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "atr", Key: "atr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "stdev":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "20")
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := sourcePeriodKey("stdev", source, period, "close")
		return plannedBinding{Alias: statement.Name, Kind: "stdev", Key: key, Args: sourcePeriodArgs(source, period, "close")}, true, nil
	case "variance":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "20")
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := "variance:" + source + ":" + strconv.Itoa(period)
		return plannedBinding{Alias: statement.Name, Kind: "variance", Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "cum":
		if len(args) != 1 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: cum() requires one source argument", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: cum() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		key := "cum:" + source
		return plannedBinding{Alias: statement.Name, Kind: "cum", Key: key, Args: []string{source}}, true, nil
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "rising", "falling", "sum":
		if len(args) != 2 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source and length arguments", statement.Range.StartLine, name)
		}
		source, ok := indicatorbinding.ParseOHLCVSource(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume", statement.Range.StartLine, name, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be a positive integer", statement.Range.StartLine, name)
		}
		function := indicatorbinding.NormalizeFunctionName(name)
		key := function + ":" + source + ":" + strconv.Itoa(period)
		return plannedBinding{Alias: statement.Name, Kind: function, Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "stoch":
		if len(args) != 4 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() requires source, high, low, and length arguments", statement.Range.StartLine)
		}
		source, ok := parseStochSource(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() source %q is not supported; use open/high/low/close/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		if !strings.EqualFold(strings.TrimSpace(args[1]), "high") || !strings.EqualFold(strings.TrimSpace(args[2]), "low") {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() currently supports literal high and low arguments only", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[3])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() length must be a positive integer", statement.Range.StartLine)
		}
		key := "stoch:" + source + ":" + strconv.Itoa(period)
		return plannedBinding{Alias: statement.Name, Kind: "stoch", Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "cci":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "hlc3", "20")
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := sourcePeriodKey("cci", source, period, "hlc3")
		return plannedBinding{Alias: statement.Name, Kind: "cci", Key: key, Args: sourcePeriodArgs(source, period, "hlc3")}, true, nil
	case "vwap":
		if len(args) != 1 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: vwap() requires one source argument", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: vwap() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		return plannedBinding{Alias: statement.Name, Kind: "vwap", Key: "vwap:" + source, Args: []string{source}}, true, nil
	case "mfi":
		source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "hlc3", "14")
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := "mfi:" + source + ":" + strconv.Itoa(period)
		return plannedBinding{Alias: statement.Name, Kind: "mfi", Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "dmi":
		values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 2)
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := fmt.Sprintf("dmi:%d:%d", values[0], values[1])
		return plannedBinding{Alias: statement.Name, Kind: "dmi", Key: key, Args: indicatorbinding.IntsToStrings(values)}, true, nil
	case "supertrend":
		if len(args) != 2 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: supertrend() requires factor and atrPeriod", statement.Range.StartLine)
		}
		factor, err := indicatorbinding.ParsePositiveFloat(args[0])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: supertrend() factor must be a positive number", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: supertrend() atrPeriod must be a positive integer", statement.Range.StartLine)
		}
		factorText := strconv.FormatFloat(factor, 'f', -1, 64)
		key := "supertrend:" + factorText + ":" + strconv.Itoa(period)
		return plannedBinding{Alias: statement.Name, Kind: "supertrend", Key: key, Args: []string{factorText, strconv.Itoa(period)}}, true, nil
	case "sar":
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: sar() requires start, increment, and max", statement.Range.StartLine)
		}
		start, err := indicatorbinding.ParsePositiveFloat(args[0])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: sar() start must be a positive number", statement.Range.StartLine)
		}
		increment, err := indicatorbinding.ParsePositiveFloat(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: sar() increment must be a positive number", statement.Range.StartLine)
		}
		maximum, err := indicatorbinding.ParsePositiveFloat(args[2])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: sar() max must be a positive number", statement.Range.StartLine)
		}
		config := sarPlannerConfig{start: start, increment: increment, maximum: maximum}
		key := sarPlannerKey(config)
		return plannedBinding{Alias: statement.Name, Kind: "sar", Key: key, Args: strings.Split(strings.TrimPrefix(key, "sar:"), ":")}, true, nil
	case "security_source":
		if len(args) < 2 || len(args) > 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: security_source() requires source, time unit, and optional lookback", statement.Range.StartLine)
		}
		source, ok := indicatorbinding.ParsePriceSource(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: security_source() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[1])
		if !ok || timeUnit == "" {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: security_source() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[1]))
		}
		lookback := 0
		if len(args) == 3 {
			parsed, err := strconv.Atoi(strings.TrimSpace(args[2]))
			if err != nil || parsed < 0 {
				return plannedBinding{}, false, fmt.Errorf("pine line %d: security_source() lookback must be a non-negative integer", statement.Range.StartLine)
			}
			lookback = parsed
		}
		key := securitySourcePlannerKey(source, timeUnit, lookback)
		bindingArgs := []string{source, timeUnit}
		if lookback > 0 {
			bindingArgs = append(bindingArgs, strconv.Itoa(lookback))
		}
		return plannedBinding{Alias: statement.Name, Kind: "security_source", Key: key, Args: bindingArgs}, true, nil
	case "williams_r", "williamsr":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "williamsr", Key: "williamsr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "bollinger":
		if len(args) != 2 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() requires period and multiplier", statement.Range.StartLine)
		}
		period, err := indicatorbinding.ParsePositiveInt(args[0])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() period must be a positive integer", statement.Range.StartLine)
		}
		multiplier, err := indicatorbinding.ParsePositiveFloat(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() multiplier must be a positive number", statement.Range.StartLine)
		}
		key := "bollinger:" + strconv.Itoa(period) + ":" + strconv.FormatFloat(multiplier, 'f', -1, 64)
		return plannedBinding{Alias: statement.Name, Kind: "bollinger", Key: key, Args: []string{strconv.Itoa(period), strconv.FormatFloat(multiplier, 'f', -1, 64)}}, true, nil
	default:
		return plannedBinding{}, false, nil
	}
}

func collectExpressionRequirements(lineNumber int, expression string) ([]IndicatorRequirement, error) {
	requirements := make([]IndicatorRequirement, 0)
	seen := map[string]struct{}{}
	add := func(kind, key string) {
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		requirements = append(requirements, IndicatorRequirement{Kind: kind, Key: key})
	}
	for _, match := range stdevCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: stdev() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		add("stdev", sourcePeriodKey("stdev", source, period, "close"))
	}
	for _, match := range rsiCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: rsi() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		add("rsi", sourcePeriodKey("rsi", source, period, "close"))
	}
	for _, match := range cciCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: cci() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		add("cci", sourcePeriodKey("cci", source, period, "hlc3"))
	}
	for _, match := range varianceCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: variance() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		add("variance", "variance:"+source+":"+strconv.Itoa(period))
	}
	for _, match := range cumCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: cum() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		add("cum", "cum:"+source)
	}
	for _, match := range windowCallPattern.FindAllStringSubmatch(expression, -1) {
		function := strings.ToLower(strings.TrimSpace(match[1]))
		source, ok := indicatorbinding.ParsePriceSource(match[2])
		if !ok {
			return nil, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, function, strings.TrimSpace(match[2]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[3])
		if err != nil {
			return nil, fmt.Errorf("pine line %d: %s() length must be a positive integer", lineNumber, function)
		}
		add(function, function+":"+source+":"+strconv.Itoa(period))
	}
	for _, match := range stochCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := parseStochSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: stoch() source %q is not supported; use open/high/low/close/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		if !strings.EqualFold(strings.TrimSpace(match[2]), "high") || !strings.EqualFold(strings.TrimSpace(match[3]), "low") {
			return nil, fmt.Errorf("pine line %d: stoch() currently supports literal high and low arguments only", lineNumber)
		}
		period, err := indicatorbinding.ParsePositiveInt(match[4])
		if err != nil {
			return nil, fmt.Errorf("pine line %d: stoch() length must be a positive integer", lineNumber)
		}
		add("stoch", "stoch:"+source+":"+strconv.Itoa(period))
	}
	for _, match := range vwapCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: vwap() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		add("vwap", "vwap:"+source)
	}
	for _, match := range mfiCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return nil, fmt.Errorf("pine line %d: mfi() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			return nil, fmt.Errorf("pine line %d: mfi() length must be a positive integer", lineNumber)
		}
		add("mfi", "mfi:"+source+":"+strconv.Itoa(period))
	}
	for _, match := range dmiCallPattern.FindAllStringSubmatch(expression, -1) {
		add("dmi", "dmi:"+strings.TrimSpace(match[1])+":"+strings.TrimSpace(match[2]))
	}
	for _, match := range supertrendCallPattern.FindAllStringSubmatch(expression, -1) {
		factor, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err != nil || factor <= 0 {
			continue
		}
		add("supertrend", "supertrend:"+strconv.FormatFloat(factor, 'f', -1, 64)+":"+strings.TrimSpace(match[2]))
	}
	for _, match := range sarCallPattern.FindAllStringSubmatch(expression, -1) {
		start, startErr := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		increment, incrementErr := strconv.ParseFloat(strings.TrimSpace(match[2]), 64)
		maximum, maxErr := strconv.ParseFloat(strings.TrimSpace(match[3]), 64)
		if startErr != nil || incrementErr != nil || maxErr != nil || start <= 0 || increment <= 0 || maximum <= 0 {
			continue
		}
		add("sar", sarPlannerKey(sarPlannerConfig{start: start, increment: increment, maximum: maximum}))
	}
	for _, match := range securitySourceCallPattern.FindAllStringSubmatch(expression, -1) {
		source, sourceOK := indicatorbinding.ParsePriceSource(match[1])
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[2])
		if !sourceOK || !timeUnitOK || timeUnit == "" {
			return nil, fmt.Errorf("pine line %d: security_source() supports open/high/low/close/volume/hl2/hlc3/ohlc4 and supported higher timeframes", lineNumber)
		}
		lookback := 0
		if strings.TrimSpace(match[3]) != "" {
			parsed, err := strconv.Atoi(strings.TrimSpace(match[3]))
			if err != nil || parsed < 0 {
				return nil, fmt.Errorf("pine line %d: security_source() lookback must be a non-negative integer", lineNumber)
			}
			lookback = parsed
		}
		add("security_source", securitySourcePlannerKey(source, timeUnit, lookback))
	}
	return requirements, nil
}

type sarPlannerConfig struct {
	start     float64
	increment float64
	maximum   float64
}

func sarPlannerKey(config sarPlannerConfig) string {
	return "sar:" +
		strconv.FormatFloat(config.start, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.increment, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.maximum, 'f', -1, 64)
}

func securitySourcePlannerKey(source string, timeUnit string, lookback int) string {
	key := "security_source:" + timeUnit + ":" + source
	if lookback > 0 {
		key += ":" + strconv.Itoa(lookback)
	}
	return key
}

func parseSourcePeriodArgs(lineNumber int, name string, args []string, defaultSource string, defaultPeriod string) (string, int, error) {
	sourceText, periodText := defaultSource, defaultPeriod
	if len(args) == 1 {
		periodText = strings.TrimSpace(args[0])
	} else if len(args) >= 2 {
		sourceText = strings.TrimSpace(args[0])
		periodText = strings.TrimSpace(args[1])
	}
	source, ok := indicatorbinding.ParsePriceSource(sourceText)
	if !ok {
		return "", 0, fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, name, sourceText)
	}
	period, err := indicatorbinding.ParsePositiveInt(periodText)
	if err != nil {
		return "", 0, fmt.Errorf("pine line %d: %s() length must be a positive integer", lineNumber, name)
	}
	return source, period, nil
}

func sourcePeriodKey(prefix string, source string, period int, legacySource string) string {
	if strings.TrimSpace(source) == "" || source == legacySource {
		return prefix + ":" + strconv.Itoa(period)
	}
	return prefix + ":" + source + ":" + strconv.Itoa(period)
}

func sourcePeriodArgs(source string, period int, legacySource string) []string {
	periodText := strconv.Itoa(period)
	if strings.TrimSpace(source) == "" || source == legacySource {
		return []string{periodText}
	}
	return []string{source, periodText}
}

func parseStochSource(value string) (string, bool) {
	source, ok := indicatorbinding.ParsePriceSource(value)
	if !ok || source == "volume" {
		return "", false
	}
	return source, true
}

func expressionRequiresPosition(expression string) bool {
	return positionVariablePattern.MatchString(expression)
}

func expressionRequiresTotalAccountValue(expression string) bool {
	return equityVariablePattern.MatchString(expression)
}

func collectConditionRequirements(condition string, bindings map[string]plannedBinding) []IndicatorRequirement {
	requirements := make([]IndicatorRequirement, 0)
	seen := map[string]struct{}{}

	for _, match := range divergenceCallPattern.FindAllStringSubmatch(condition, -1) {
		direction := strings.TrimSpace(match[1])
		alias := strings.TrimSpace(match[2])
		lookback, err := indicatorbinding.ParsePositiveInt(match[3])
		if err != nil {
			continue
		}
		binding, ok := bindings[alias]
		if !ok {
			continue
		}

		key, ok := buildDivergenceRequirementKey(binding, direction, lookback)
		if !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		requirements = append(requirements, IndicatorRequirement{
			Alias: alias,
			Kind:  "divergence",
			Key:   key,
		})
	}

	return requirements
}

func buildDivergenceRequirementKey(binding plannedBinding, direction string, lookback int) (string, bool) {
	switch binding.Kind {
	case "rsi":
		return fmt.Sprintf("divergence:rsi:%s:%s:%d", binding.Args[0], direction, lookback), true
	case "macd":
		return fmt.Sprintf("divergence:macd:%s:%s:%s:%s:%d", binding.Args[0], binding.Args[1], binding.Args[2], direction, lookback), true
	case "kdj":
		return fmt.Sprintf("divergence:kdj:%s:%s:%s:%s:%d", binding.Args[0], binding.Args[1], binding.Args[2], direction, lookback), true
	default:
		return "", false
	}
}

func buildProtectRequirementKey(statement *ProtectStmt) (string, error) {
	mode, ok := indicatorbinding.ParseProtectMode(statement.Mode)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect mode %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Mode))
	}
	direction, ok := indicatorbinding.ParseProtectDirection(statement.Direction)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect direction %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Direction))
	}
	timeValue, err := indicatorbinding.ParsePositiveInt(statement.TimeValueExpression)
	if err != nil {
		return "", fmt.Errorf("pine line %d: protect time value must be a positive integer", statement.Range.StartLine)
	}
	timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(statement.TimeUnit)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.TimeUnit))
	}
	if timeUnit == "" {
		timeUnit = "bar"
	}
	percentage, err := indicatorbinding.ParsePercentage(statement.PercentageExpression)
	if err != nil {
		return "", fmt.Errorf("pine line %d: protect percentage must be a positive number", statement.Range.StartLine)
	}
	windowPolicy, ok := indicatorbinding.ParseProtectWindowPolicy(statement.WindowPolicy)
	if !ok {
		return "", fmt.Errorf("pine line %d: protect window policy %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.WindowPolicy))
	}
	if mode == "stopLoss" && windowPolicy == "continuous" {
		return fmt.Sprintf("sl:%s:%d:%s:%s", direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64)), nil
	}
	return fmt.Sprintf("risk:%s:%s:%d:%s:%s:%s", mode, direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64), windowPolicy), nil
}

func cloneBindings(input map[string]plannedBinding) map[string]plannedBinding {
	cloned := make(map[string]plannedBinding, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
