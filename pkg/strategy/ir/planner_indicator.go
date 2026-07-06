package ir

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

var divergenceCallPattern = regexp.MustCompile(`divergence_(top|bottom)\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*,\s*([0-9]+)\s*\)`)
var rsiCallPattern = regexp.MustCompile(`\brsi\s*\(\s*([^,]+?)\s*,\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+))?\s*\)`)
var stdevCallPattern = regexp.MustCompile(`\bstdev\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var varianceCallPattern = regexp.MustCompile(`\bvariance\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var cciCallPattern = regexp.MustCompile(`\bcci\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var macdCallPattern = regexp.MustCompile(`\bmacd\s*\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+)\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?\s*\)`)
var atrCallPattern = regexp.MustCompile(`\batr\s*\(\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+))?\s*\)`)
var bollingerCallPattern = regexp.MustCompile(`\bbollinger\s*\(\s*([0-9]+)\s*,\s*([0-9]+(?:\.[0-9]+)?)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+)\s*,\s*([A-Za-z_][A-Za-z0-9_]*))?\s*\)`)
var cumCallPattern = regexp.MustCompile(`\bcum\s*\(\s*([^)]+?)\s*\)`)
var stochCallPattern = regexp.MustCompile(`\bstoch\s*\(\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+))?\s*\)`)
var vwapCallPattern = regexp.MustCompile(`\bvwap\s*\(\s*([^)]+?)\s*\)`)
var anchoredVWAPCallPattern = regexp.MustCompile(`\banchored_vwap\s*\(\s*([^,]+?)\s*,\s*([^,\)]+?)\s*\)`)
var mfiCallPattern = regexp.MustCompile(`\bmfi\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var dmiCallPattern = regexp.MustCompile(`\bdmi\s*\(\s*([0-9]+)\s*,\s*([0-9]+)\s*\)`)
var supertrendCallPattern = regexp.MustCompile(`\bsupertrend\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+))?\s*\)`)
var sarCallPattern = regexp.MustCompile(`\bsar\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*\)`)
var maCallPattern = regexp.MustCompile(`\bma\s*\(([^()]*)\)`)
var securitySourceCallPattern = regexp.MustCompile(`\bsecurity_source\s*\(\s*([^,]+?)\s*,\s*([^,\)]+?)(?:\s*,\s*(-?[0-9]+)\s*)?\)`)
var windowCallPattern = regexp.MustCompile(`\b(highest|lowest|highestbars|lowestbars|change|mom|roc|range|mode|rising|falling|sum)\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)

func parseIndicatorBinding(statement *LetStmt) (plannedBinding, bool, error) {
	name, args, ok := indicatorbinding.ParseFunctionCall(statement.Expression)
	if !ok {
		return plannedBinding{}, false, nil
	}

	normalizedName := indicatorbinding.NormalizeFunctionName(name)
	if isAdvancedIndicatorBinding(normalizedName) {
		return parseAdvancedIndicatorBinding(statement.Range.StartLine, statement.Name, normalizedName, args)
	}

	return parseBasicIndicatorBinding(statement, normalizedName, args)
}

func isAdvancedIndicatorBinding(name string) bool {
	switch name {
	case "linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma",
		"bbw", "cog", "cmo", "tsi", "correlation", "dev", "median", "percentile_linear_interpolation",
		"percentile_nearest_rank", "percentrank", "swma":
		return true
	default:
		return false
	}
}

func parseBasicIndicatorBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	switch name {
	case "ma":
		return parseMABinding(statement, args)
	case "rsi":
		return parseSourcePeriodBinding(statement, name, args, "close", "14")
	case "macd":
		binding, err := parseMACDBinding(statement.Range.StartLine, statement.Name, name, args)
		if err != nil {
			return plannedBinding{}, false, err
		}
		return binding, true, nil
	case "kdj":
		return parseKDJBinding(statement, name, args)
	case "atr":
		return parseATRBinding(statement, name, args)
	case "stdev":
		return parseSourcePeriodBinding(statement, name, args, "close", "20")
	case "variance":
		return parseVarianceBinding(statement, name, args)
	case "cum":
		return parseCumulativeBinding(statement, args)
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
		return parseWindowBinding(statement, name, args)
	case "stoch":
		return parseStochBinding(statement, args)
	case "cci":
		return parseSourcePeriodBinding(statement, name, args, "hlc3", "20")
	case "vwap":
		return parseSingleSourceBinding(statement, name, args, "vwap")
	case "anchored_vwap":
		return parseAnchoredVWAPBinding(statement, args)
	case "mfi":
		return parseMFIBinding(statement, name, args)
	case "dmi":
		return parseDMIBinding(statement, name, args)
	case "supertrend":
		return parseSupertrendBinding(statement, args)
	case "sar":
		return parseSARBinding(statement, args)
	case "security_source":
		return parseSecuritySourceBinding(statement, args)
	case "williams_r", "williamsr":
		return parseWilliamsRBinding(statement, name, args)
	case "bollinger":
		return parseBollingerBinding(statement, args)
	default:
		return plannedBinding{}, false, nil
	}
}

func parseMABinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
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
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "ma",
		Key:   key,
		Args:  []string{averageType, strconv.Itoa(period), timeUnit, source},
	}, true, nil
}

func parseSourcePeriodBinding(statement *LetStmt, name string, args []string, defaultSource string, defaultPeriod string) (plannedBinding, bool, error) {
	source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, defaultSource, defaultPeriod)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  name,
		Key:   sourcePeriodKey(name, source, period, defaultSource),
		Args:  sourcePeriodArgs(source, period, defaultSource),
	}, true, nil
}

func parseKDJBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 3)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "kdj",
		Key:   fmt.Sprintf("kdj:%d:%d:%d", values[0], values[1], values[2]),
		Args:  indicatorbinding.IntsToStrings(values),
	}, true, nil
}

func parseATRBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{Alias: statement.Name, Kind: "atr", Key: "atr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
}

func parseVarianceBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "close", "20")
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "variance",
		Key:   "variance:" + source + ":" + strconv.Itoa(period),
		Args:  []string{source, strconv.Itoa(period)},
	}, true, nil
}

func parseCumulativeBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
	source, err := parseSinglePriceSourceArg(statement.Range.StartLine, "cum", args)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{Alias: statement.Name, Kind: "cum", Key: "cum:" + source, Args: []string{source}}, true, nil
}

func parseWindowBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
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
	return plannedBinding{
		Alias: statement.Name,
		Kind:  name,
		Key:   name + ":" + source + ":" + strconv.Itoa(period),
		Args:  []string{source, strconv.Itoa(period)},
	}, true, nil
}

func parseStochBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
	if len(args) != 4 && len(args) != 5 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() requires source, high, low, length, and optional time unit arguments", statement.Range.StartLine)
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
	bindingArgs := []string{source, strconv.Itoa(period)}
	if len(args) == 5 {
		timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[4])
		if !ok {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: stoch() time unit %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[4]))
		}
		key += ":" + timeUnit
		bindingArgs = append(bindingArgs, timeUnit)
	}
	return plannedBinding{Alias: statement.Name, Kind: "stoch", Key: key, Args: bindingArgs}, true, nil
}

func parseSingleSourceBinding(statement *LetStmt, name string, args []string, kind string) (plannedBinding, bool, error) {
	source, err := parseSinglePriceSourceArg(statement.Range.StartLine, name, args)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{Alias: statement.Name, Kind: kind, Key: kind + ":" + source, Args: []string{source}}, true, nil
}

func parseAnchoredVWAPBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
	if len(args) != 2 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: anchored_vwap() requires source and anchor unit", statement.Range.StartLine)
	}
	source, ok := indicatorbinding.ParsePriceSource(args[0])
	unit := strings.ToLower(strings.TrimSpace(args[1]))
	if !ok || (unit != "day" && unit != "week" && unit != "month") {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: anchored_vwap() supports OHLCV/derived source and day/week/month anchors", statement.Range.StartLine)
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "anchored_vwap",
		Key:   "anchored_vwap:" + unit + ":" + source,
		Args:  []string{source, unit},
	}, true, nil
}

func parseMFIBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	source, period, err := parseSourcePeriodArgs(statement.Range.StartLine, name, args, "hlc3", "14")
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "mfi",
		Key:   "mfi:" + source + ":" + strconv.Itoa(period),
		Args:  []string{source, strconv.Itoa(period)},
	}, true, nil
}

func parseDMIBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	values, err := indicatorbinding.ExpectPositiveIntArgs(statement.Range.StartLine, name, args, 2)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "dmi",
		Key:   fmt.Sprintf("dmi:%d:%d", values[0], values[1]),
		Args:  indicatorbinding.IntsToStrings(values),
	}, true, nil
}

func parseSupertrendBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
	if len(args) != 2 && len(args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: supertrend() requires factor, atrPeriod, and optional time unit", statement.Range.StartLine)
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
	bindingArgs := []string{factorText, strconv.Itoa(period)}
	if len(args) == 3 {
		timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[2])
		if !ok || timeUnit == "" {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: supertrend() timeframe %q is not supported", statement.Range.StartLine, strings.TrimSpace(args[2]))
		}
		key += ":" + timeUnit
		bindingArgs = append(bindingArgs, timeUnit)
	}
	return plannedBinding{Alias: statement.Name, Kind: "supertrend", Key: key, Args: bindingArgs}, true, nil
}

func parseSARBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
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
	key := sarPlannerKey(sarPlannerConfig{start: start, increment: increment, maximum: maximum})
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "sar",
		Key:   key,
		Args:  strings.Split(strings.TrimPrefix(key, "sar:"), ":"),
	}, true, nil
}

func parseSecuritySourceBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
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
	bindingArgs := []string{source, timeUnit}
	if lookback > 0 {
		bindingArgs = append(bindingArgs, strconv.Itoa(lookback))
	}
	return plannedBinding{
		Alias: statement.Name,
		Kind:  "security_source",
		Key:   securitySourcePlannerKey(source, timeUnit, lookback),
		Args:  bindingArgs,
	}, true, nil
}

func parseWilliamsRBinding(statement *LetStmt, name string, args []string) (plannedBinding, bool, error) {
	period, err := indicatorbinding.ExpectOnePositiveIntArg(statement.Range.StartLine, name, args)
	if err != nil {
		return plannedBinding{}, false, err
	}
	return plannedBinding{Alias: statement.Name, Kind: "williamsr", Key: "williamsr:" + strconv.Itoa(period), Args: []string{strconv.Itoa(period)}}, true, nil
}

func parseBollingerBinding(statement *LetStmt, args []string) (plannedBinding, bool, error) {
	if len(args) != 2 && len(args) != 4 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() requires period, multiplier, optional time unit, and optional source", statement.Range.StartLine)
	}
	period, err := indicatorbinding.ParsePositiveInt(args[0])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() period must be a positive integer", statement.Range.StartLine)
	}
	multiplier, err := indicatorbinding.ParsePositiveFloat(args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() multiplier must be a positive number", statement.Range.StartLine)
	}
	multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
	key := "bollinger:" + strconv.Itoa(period) + ":" + multiplierText
	bindingArgs := []string{strconv.Itoa(period), multiplierText}
	if len(args) == 4 {
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(args[2])
		source, sourceOK := indicatorbinding.ParsePriceSource(args[3])
		if !timeUnitOK || timeUnit == "" || !sourceOK {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bollinger() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", statement.Range.StartLine)
		}
		key = fmt.Sprintf("bollinger:%s:%d:%s:%s", source, period, multiplierText, timeUnit)
		bindingArgs = append(bindingArgs, timeUnit, source)
	}
	return plannedBinding{Alias: statement.Name, Kind: "bollinger", Key: key, Args: bindingArgs}, true, nil
}

func parseSinglePriceSourceArg(lineNumber int, name string, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("pine line %d: %s() requires one source argument", lineNumber, name)
	}
	source, ok := indicatorbinding.ParsePriceSource(args[0])
	if !ok {
		return "", fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", lineNumber, name, strings.TrimSpace(args[0]))
	}
	return source, nil
}

func collectExpressionRequirements(lineNumber int, expression string) ([]IndicatorRequirement, error) {
	collector := newExpressionRequirementCollector(lineNumber)
	if err := collector.collect(expression); err != nil {
		return nil, err
	}
	return collector.requirements, nil
}

func splitPlannerArguments(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
