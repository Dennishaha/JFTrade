package ir

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var divergenceCallPattern = regexp.MustCompile(`divergence_(top|bottom)\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*,\s*([0-9]+)\s*\)`)

type Requirements struct {
	Indicators                []IndicatorRequirement
	RequiresPosition          bool
	RequiresAvailableCash     bool
	RequiresMarginBuyingPower bool
	RequiresShortSellingPower bool
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
	sortStrings(keys)
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
		case *IfStmt:
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
			quantityMode, ok := parseQuantityMode(typed.QuantityMode)
			if !ok {
				return fmt.Errorf("dsl line %d: unsupported order quantity mode %q", typed.Range.StartLine, typed.QuantityMode)
			}
			result.RequiresPosition = true
			switch quantityMode {
			case "cash_percent":
				result.RequiresAvailableCash = true
			case "margin_buying_power_percent":
				result.RequiresMarginBuyingPower = true
			case "short_selling_power_percent":
				result.RequiresShortSellingPower = true
			case "account_position_percent":
				result.RequiresTotalAccountValue = true
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
	name, args, ok := parseFunctionCall(statement.Expression)
	if !ok {
		return plannedBinding{}, false, nil
	}

	switch normalizeFunctionName(name) {
	case "ma":
		if len(args) < 2 || len(args) > 3 {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: ma() requires type, period, and optional time unit", statement.Range.StartLine)
		}
		averageType, ok := parseMovingAverageType(args[0])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: ma() type %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[0]))
		}
		period, err := parsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: ma() period must be a positive integer", statement.Range.StartLine)
		}
		timeUnit := ""
		if len(args) == 3 {
			parsedTimeUnit, ok := parseIndicatorTimeUnitValue(args[2])
			if !ok {
				return plannedBinding{}, false, fmt.Errorf("dsl line %d: ma() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[2]))
			}
			timeUnit = parsedTimeUnit
		}
		key := buildMovingAverageKey(averageType, period, timeUnit)
		return plannedBinding{Alias: statement.Name, Kind: "ma", Key: key, Args: []string{averageType, strconv.Itoa(period), timeUnit}}, true, nil
	case "rsi":
		period, err := expectOnePositiveIntArg(statement, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "rsi", Key: "rsi:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "macd":
		values, err := expectPositiveIntArgs(statement, name, args, 3)
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := fmt.Sprintf("macd:%d:%d:%d", values[0], values[1], values[2])
		return plannedBinding{Alias: statement.Name, Kind: "macd", Key: key, Args: intsToStrings(values)}, true, nil
	case "kdj":
		values, err := expectPositiveIntArgs(statement, name, args, 3)
		if err != nil {
			return plannedBinding{}, false, err
		}
		key := fmt.Sprintf("kdj:%d:%d:%d", values[0], values[1], values[2])
		return plannedBinding{Alias: statement.Name, Kind: "kdj", Key: key, Args: intsToStrings(values)}, true, nil
	case "atr":
		period, err := expectOnePositiveIntArg(statement, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "atr", Key: "atr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "cci":
		period, err := expectOnePositiveIntArg(statement, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "cci", Key: "cci:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "williams_r", "williamsr":
		period, err := expectOnePositiveIntArg(statement, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: statement.Name, Kind: "williamsr", Key: "williamsr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
	case "bollinger":
		if len(args) != 2 {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: bollinger() requires period and multiplier", statement.Range.StartLine)
		}
		period, err := parsePositiveInt(args[0])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: bollinger() period must be a positive integer", statement.Range.StartLine)
		}
		multiplier, err := parsePositiveFloat(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("dsl line %d: bollinger() multiplier must be a positive number", statement.Range.StartLine)
		}
		key := "bollinger:" + strconv.Itoa(period) + ":" + strconv.FormatFloat(multiplier, 'f', -1, 64)
		return plannedBinding{Alias: statement.Name, Kind: "bollinger", Key: key, Args: []string{strconv.Itoa(period), strconv.FormatFloat(multiplier, 'f', -1, 64)}}, true, nil
	default:
		return plannedBinding{}, false, nil
	}
}

func collectConditionRequirements(condition string, bindings map[string]plannedBinding) []IndicatorRequirement {
	requirements := make([]IndicatorRequirement, 0)
	seen := map[string]struct{}{}

	for _, match := range divergenceCallPattern.FindAllStringSubmatch(condition, -1) {
		direction := strings.TrimSpace(match[1])
		alias := strings.TrimSpace(match[2])
		lookback, err := parsePositiveInt(match[3])
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
	mode, ok := parseProtectMode(statement.Mode)
	if !ok {
		return "", fmt.Errorf("dsl line %d: protect mode %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Mode))
	}
	direction, ok := parseProtectDirection(statement.Direction)
	if !ok {
		return "", fmt.Errorf("dsl line %d: protect direction %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.Direction))
	}
	timeValue, err := parsePositiveInt(statement.TimeValueExpression)
	if err != nil {
		return "", fmt.Errorf("dsl line %d: protect time value must be a positive integer", statement.Range.StartLine)
	}
	timeUnit, ok := parseIndicatorTimeUnitValue(statement.TimeUnit)
	if !ok {
		return "", fmt.Errorf("dsl line %d: protect time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.TimeUnit))
	}
	if timeUnit == "" {
		timeUnit = "bar"
	}
	percentage, err := parsePercentage(statement.PercentageExpression)
	if err != nil {
		return "", fmt.Errorf("dsl line %d: protect percentage must be a positive number", statement.Range.StartLine)
	}
	windowPolicy, ok := parseProtectWindowPolicy(statement.WindowPolicy)
	if !ok {
		return "", fmt.Errorf("dsl line %d: protect window policy %q is not supported", statement.Range.StartLine, strings.TrimSpace(statement.WindowPolicy))
	}
	if mode == "stopLoss" && windowPolicy == "continuous" {
		return fmt.Sprintf("sl:%s:%d:%s:%s", direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64)), nil
	}
	return fmt.Sprintf("risk:%s:%s:%d:%s:%s:%s", mode, direction, timeValue, timeUnit, strconv.FormatFloat(percentage, 'f', -1, 64), windowPolicy), nil
}

func parseFunctionCall(value string) (string, []string, bool) {
	trimmed := strings.TrimSpace(value)
	openIndex := strings.Index(trimmed, "(")
	closeIndex := strings.LastIndex(trimmed, ")")
	if openIndex <= 0 || closeIndex != len(trimmed)-1 || closeIndex <= openIndex {
		return "", nil, false
	}
	name := strings.TrimSpace(trimmed[:openIndex])
	args := splitArguments(trimmed[openIndex+1 : closeIndex])
	return name, args, true
}

func splitArguments(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	depth := 0
	quote := rune(0)
	for index, char := range trimmed {
		switch {
		case quote != 0:
			if char == quote {
				quote = 0
			}
		case char == '\'' || char == '"' || char == '`':
			quote = char
		case char == '(':
			depth++
		case char == ')':
			if depth > 0 {
				depth--
			}
		case char == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(trimmed[start:index]))
			start = index + 1
		}
	}
	parts = append(parts, strings.TrimSpace(trimmed[start:]))
	return parts
}

func normalizeFunctionName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseMovingAverageType(value string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "MA", "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func normalizeMovingAverageType(value string) string {
	parsed, ok := parseMovingAverageType(value)
	if !ok {
		return "MA"
	}
	return parsed
}

func parseIndicatorTimeUnitValue(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "bar", "bars":
		return "", true
	case "m", "min", "mins", "minute", "minutes":
		return "minute", true
	case "h", "hr", "hrs", "hour", "hours":
		return "hour", true
	case "d", "day", "days":
		return "day", true
	case "w", "week", "weeks":
		return "week", true
	case "mo", "mon", "month", "months":
		return "month", true
	default:
		return "", false
	}
}

func normalizeIndicatorTimeUnit(value string) string {
	parsed, _ := parseIndicatorTimeUnitValue(value)
	return parsed
}

func buildMovingAverageKey(averageType string, period int, timeUnit string) string {
	base := "ma:" + averageType + ":" + strconv.Itoa(period)
	if timeUnit == "" {
		return base
	}
	return base + ":" + timeUnit
}

func parseQuantityMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accountpositionpercent", "account_position_percent":
		return "account_position_percent", true
	case "symbolpositionpercent", "symbol_position_percent", "positionpercent", "position_percent":
		return "symbol_position_percent", true
	case "cashpercent", "cash_percent":
		return "cash_percent", true
	case "marginbuyingpowerpercent", "margin_buying_power_percent":
		return "margin_buying_power_percent", true
	case "shortsellingpowerpercent", "short_selling_power_percent":
		return "short_selling_power_percent", true
	case "amount":
		return "amount", true
	case "share", "shares":
		return "shares", true
	default:
		return "", false
	}
}

func normalizeQuantityMode(value string) string {
	parsed, ok := parseQuantityMode(value)
	if !ok {
		return "shares"
	}
	return parsed
}

func parseProtectMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stoploss", "stop_loss":
		return "stopLoss", true
	case "takeprofit", "take_profit":
		return "takeProfit", true
	case "trailingstop", "trailing_stop":
		return "trailingStop", true
	default:
		return "", false
	}
}

func normalizeProtectMode(value string) string {
	parsed, ok := parseProtectMode(value)
	if !ok {
		return "stopLoss"
	}
	return parsed
}

func parseProtectDirection(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long", true
	case "short":
		return "short", true
	case "auto":
		return "auto", true
	default:
		return "", false
	}
}

func normalizeProtectDirection(value string) string {
	parsed, ok := parseProtectDirection(value)
	if !ok {
		return "auto"
	}
	return parsed
}

func parseProtectWindowPolicy(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "continuous") {
		return "continuous", true
	}
	if strings.EqualFold(trimmed, "session") {
		return "session", true
	}
	return "", false
}

func normalizeProtectWindowPolicy(value string) string {
	parsed, ok := parseProtectWindowPolicy(value)
	if !ok {
		return "continuous"
	}
	return parsed
}

func parsePercentage(value string) (float64, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	return parsePositiveFloat(trimmed)
}

func parsePositiveFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("positive float is required")
	}
	return parsed, nil
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("positive integer is required")
	}
	return parsed, nil
}

func expectOnePositiveIntArg(statement *LetStmt, name string, args []string) (int, error) {
	values, err := expectPositiveIntArgs(statement, name, args, 1)
	if err != nil {
		return 0, err
	}
	return values[0], nil
}

func expectPositiveIntArgs(statement *LetStmt, name string, args []string, expected int) ([]int, error) {
	if len(args) != expected {
		return nil, fmt.Errorf("dsl line %d: %s() requires %d argument(s)", statement.Range.StartLine, name, expected)
	}
	values := make([]int, 0, expected)
	for _, arg := range args {
		parsed, err := parsePositiveInt(arg)
		if err != nil {
			return nil, fmt.Errorf("dsl line %d: %s() arguments must be positive integers", statement.Range.StartLine, name)
		}
		values = append(values, parsed)
	}
	return values, nil
}

func intsToStrings(values []int) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.Itoa(value))
	}
	return result
}

func cloneBindings(input map[string]plannedBinding) map[string]plannedBinding {
	cloned := make(map[string]plannedBinding, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
