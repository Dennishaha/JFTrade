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
var stdevCallPattern = regexp.MustCompile(`\bstdev\s*\(\s*([0-9]+)\s*\)`)
var positionVariablePattern = regexp.MustCompile(`\b(position_size|position_avg_price)\b`)

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
			for _, requirement := range collectExpressionRequirements(typed.Expression) {
				indicatorByKey[requirement.Key] = requirement
			}
		case *IfStmt:
			if expressionRequiresPosition(typed.Condition) {
				result.RequiresPosition = true
			}
			for _, requirement := range collectExpressionRequirements(typed.Condition) {
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
			for _, requirement := range collectExpressionRequirements(typed.QuantityExpression) {
				indicatorByKey[requirement.Key] = requirement
			}
			for _, requirement := range collectExpressionRequirements(typed.LimitExpression) {
				indicatorByKey[requirement.Key] = requirement
			}
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
		if len(args) < 2 || len(args) > 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() requires type, period, and optional time unit", statement.Range.StartLine)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() type %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() period must be a positive integer", statement.Range.StartLine)
		}
		timeUnit := ""
		if len(args) == 3 {
			parsedTimeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[2])
			if !ok {
				return plannedBinding{}, false, fmt.Errorf("pine line %d: ma() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[2]))
			}
			timeUnit = parsedTimeUnit
		}
		key := indicatorbinding.BuildMovingAverageKey(averageType, period, timeUnit)
		return plannedBinding{Alias: statement.Name, Kind: "ma", Key: key, Args: []string{averageType, strconv.Itoa(period), timeUnit}}, true, nil
	case "rsi":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "rsi", Key: "rsi:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
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
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "stdev", Key: "stdev:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "cci":
		period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "cci", Key: "cci:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
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

func collectExpressionRequirements(expression string) []IndicatorRequirement {
	requirements := make([]IndicatorRequirement, 0)
	seen := map[string]struct{}{}
	for _, match := range stdevCallPattern.FindAllStringSubmatch(expression, -1) {
		period, err := indicatorbinding.ParsePositiveInt(match[1])
		if err != nil {
			continue
		}
		key := "stdev:" + strconv.Itoa(period)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		requirements = append(requirements, IndicatorRequirement{Kind: "stdev", Key: key})
	}
	return requirements
}

func expressionRequiresPosition(expression string) bool {
	return positionVariablePattern.MatchString(expression)
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
