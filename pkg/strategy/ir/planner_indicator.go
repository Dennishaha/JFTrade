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
var stochCallPattern = regexp.MustCompile(`\bstoch\s*\(\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var vwapCallPattern = regexp.MustCompile(`\bvwap\s*\(\s*([^)]+?)\s*\)`)
var mfiCallPattern = regexp.MustCompile(`\bmfi\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)
var dmiCallPattern = regexp.MustCompile(`\bdmi\s*\(\s*([0-9]+)\s*,\s*([0-9]+)\s*\)`)
var supertrendCallPattern = regexp.MustCompile(`\bsupertrend\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+)(?:\s*,\s*('[^']+'|"[^"]+"|[A-Za-z0-9_]+))?\s*\)`)
var sarCallPattern = regexp.MustCompile(`\bsar\s*\(\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*,\s*([0-9]+(?:\.[0-9]+)?)\s*\)`)
var maCallPattern = regexp.MustCompile(`\bma\s*\(([^()]*)\)`)
var securitySourceCallPattern = regexp.MustCompile(`\bsecurity_source\s*\(\s*([^,]+?)\s*,\s*([^,\)]+?)(?:\s*,\s*([0-9]+)\s*)?\)`)
var windowCallPattern = regexp.MustCompile(`\b(highest|lowest|highestbars|lowestbars|change|mom|roc|range|mode|rising|falling|sum)\s*\(\s*([^,]+?)\s*,\s*([0-9]+)\s*\)`)

func parseIndicatorBinding(statement *LetStmt) (plannedBinding, bool, error) {
	name, args, ok := indicatorbinding.ParseFunctionCall(statement.Expression)
	if !ok {
		return plannedBinding{}, false, nil
	}

	switch indicatorbinding.NormalizeFunctionName(name) {
	case "linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma",
		"cmo", "tsi", "correlation", "dev", "median", "percentile_linear_interpolation",
		"percentile_nearest_rank", "percentrank", "swma":
		return parseAdvancedIndicatorBinding(statement.Range.StartLine, statement.Name, indicatorbinding.NormalizeFunctionName(name), args)
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
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
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

func parseAdvancedIndicatorBinding(lineNumber int, alias, name string, args []string) (plannedBinding, bool, error) {
	sourceArg := func(value string) (string, error) {
		source, ok := indicatorbinding.ParsePriceSource(value)
		if !ok {
			return "", fmt.Errorf("pine line %d: %s() source %q is not supported", lineNumber, name, strings.TrimSpace(value))
		}
		return source, nil
	}
	timeUnit := ""
	parseTimeUnit := func(index int) error {
		if len(args) <= index {
			return nil
		}
		if len(args) != index+1 {
			return fmt.Errorf("pine line %d: %s() received an invalid argument count", lineNumber, name)
		}
		parsed, ok := indicatorbinding.ParseIndicatorTimeUnitValue(args[index])
		if !ok || parsed == "" {
			return fmt.Errorf("pine line %d: %s() timeframe %q is not supported", lineNumber, name, strings.TrimSpace(args[index]))
		}
		timeUnit = parsed
		args = args[:index]
		return nil
	}
	withTimeUnit := func(key string) string {
		if timeUnit == "" {
			return key
		}
		return key + ":" + timeUnit
	}
	switch name {
	case "cmo", "dev", "median", "percentrank":
		if err := parseTimeUnit(2); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 2 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source and length", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", lineNumber, name)
		}
		key := withTimeUnit(fmt.Sprintf("%s:%s:%d", name, source, period))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(period)}}, true, nil
	case "tsi":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() requires source, short length, and long length", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		shortPeriod, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() short length must be positive", lineNumber)
		}
		longPeriod, err := indicatorbinding.ParsePositiveInt(args[2])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() long length must be positive", lineNumber)
		}
		key := withTimeUnit(fmt.Sprintf("tsi:%s:%d:%d", source, shortPeriod, longPeriod))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(shortPeriod), strconv.Itoa(longPeriod)}}, true, nil
	case "correlation":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: correlation() requires source, second source, and length", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		source2, err := sourceArg(args[1])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[2])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: correlation() length must be positive", lineNumber)
		}
		key := withTimeUnit(fmt.Sprintf("correlation:%s:%s:%d", source, source2, period))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, source2, strconv.Itoa(period)}}, true, nil
	case "percentile_linear_interpolation", "percentile_nearest_rank":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, and percentage", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", lineNumber, name)
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if err != nil || percentage < 0 || percentage > 100 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() percentage must be between 0 and 100", lineNumber, name)
		}
		key := withTimeUnit(fmt.Sprintf("%s:%s:%d:%s", name, source, period, strconv.FormatFloat(percentage, 'f', -1, 64)))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(period), strconv.FormatFloat(percentage, 'f', -1, 64)}}, true, nil
	case "swma":
		if err := parseTimeUnit(1); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 1 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: swma() requires one source", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: alias, Kind: name, Key: withTimeUnit("swma:" + source), Args: []string{source}}, true, nil
	case "linreg":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() requires source, length, and offset", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() length must be positive", lineNumber)
		}
		offset, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || offset < 0 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() offset must be a non-negative integer", lineNumber)
		}
		key := withTimeUnit(fmt.Sprintf("linreg:%s:%d:%d", source, period, offset))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(period), strconv.Itoa(offset)}}, true, nil
	case "obv":
		if len(args) == 0 {
			args = []string{"close"}
		}
		if err := parseTimeUnit(1); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 1 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: obv() accepts one source", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		return plannedBinding{Alias: alias, Kind: name, Key: withTimeUnit("obv:" + source), Args: []string{source}}, true, nil
	case "pivothigh", "pivotlow":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		source := "high"
		if name == "pivotlow" {
			source = "low"
		}
		lengthArgs := args
		if len(args) == 3 {
			var err error
			source, err = sourceArg(args[0])
			if err != nil {
				return plannedBinding{}, false, err
			}
			lengthArgs = args[1:]
		}
		if len(lengthArgs) != 2 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires left and right bars with optional source", lineNumber, name)
		}
		left, err := indicatorbinding.ParsePositiveInt(lengthArgs[0])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() left bars must be positive", lineNumber, name)
		}
		right, err := indicatorbinding.ParsePositiveInt(lengthArgs[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() right bars must be positive", lineNumber, name)
		}
		key := withTimeUnit(fmt.Sprintf("%s:%s:%d:%d", name, source, left, right))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(left), strconv.Itoa(right)}}, true, nil
	case "kc", "kcw":
		if err := parseTimeUnit(4); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) < 3 || len(args) > 4 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, multiplier, and optional useTrueRange", lineNumber, name)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", lineNumber, name)
		}
		multiplier, err := indicatorbinding.ParsePositiveFloat(args[2])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() multiplier must be positive", lineNumber, name)
		}
		useTR := true
		if len(args) == 4 {
			parsed, parseErr := strconv.ParseBool(strings.TrimSpace(args[3]))
			if parseErr != nil {
				return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() useTrueRange must be boolean", lineNumber, name)
			}
			useTR = parsed
		}
		key := withTimeUnit(fmt.Sprintf("%s:%s:%d:%s:%t", name, source, period, strconv.FormatFloat(multiplier, 'f', -1, 64), useTR))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: args}, true, nil
	case "alma":
		if err := parseTimeUnit(4); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 4 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() requires source, length, offset, and sigma", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() length must be positive", lineNumber)
		}
		offset, err := strconv.ParseFloat(strings.TrimSpace(args[2]), 64)
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() offset must be numeric", lineNumber)
		}
		sigma, err := indicatorbinding.ParsePositiveFloat(args[3])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() sigma must be positive", lineNumber)
		}
		key := withTimeUnit(fmt.Sprintf("alma:%s:%d:%s:%s", source, period, strconv.FormatFloat(offset, 'f', -1, 64), strconv.FormatFloat(sigma, 'f', -1, 64)))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: args}, true, nil
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
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[3])
			if !timeUnitOK || timeUnit == "" {
				return nil, fmt.Errorf("pine line %d: rsi() timeframe %q is not supported", lineNumber, strings.TrimSpace(match[3]))
			}
			add("rsi", fmt.Sprintf("rsi:%s:%d:%s", source, period, timeUnit))
			continue
		}
		add("rsi", sourcePeriodKey("rsi", source, period, "close"))
	}
	for _, match := range macdCallPattern.FindAllStringSubmatch(expression, -1) {
		fast, fastErr := indicatorbinding.ParsePositiveInt(match[1])
		slow, slowErr := indicatorbinding.ParsePositiveInt(match[2])
		signal, signalErr := indicatorbinding.ParsePositiveInt(match[3])
		if fastErr != nil || slowErr != nil || signalErr != nil {
			continue
		}
		if len(match) > 5 && strings.TrimSpace(match[4]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[4])
			source, sourceOK := indicatorbinding.ParsePriceSource(match[5])
			if !timeUnitOK || timeUnit == "" || !sourceOK {
				return nil, fmt.Errorf("pine line %d: macd() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", lineNumber)
			}
			add("macd", fmt.Sprintf("macd:%s:%d:%d:%d:%s", source, fast, slow, signal, timeUnit))
			continue
		}
		add("macd", fmt.Sprintf("macd:%d:%d:%d", fast, slow, signal))
	}
	for _, match := range atrCallPattern.FindAllStringSubmatch(expression, -1) {
		period, err := indicatorbinding.ParsePositiveInt(match[1])
		if err != nil {
			continue
		}
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[2])
			if !timeUnitOK || timeUnit == "" {
				return nil, fmt.Errorf("pine line %d: atr() timeframe %q is not supported", lineNumber, strings.TrimSpace(match[2]))
			}
			add("atr", fmt.Sprintf("atr:%d:%s", period, timeUnit))
			continue
		}
		add("atr", "atr:"+strconv.Itoa(period))
	}
	for _, match := range bollingerCallPattern.FindAllStringSubmatch(expression, -1) {
		period, err := indicatorbinding.ParsePositiveInt(match[1])
		if err != nil {
			continue
		}
		multiplier, multiplierErr := strconv.ParseFloat(strings.TrimSpace(match[2]), 64)
		if multiplierErr != nil || multiplier <= 0 {
			continue
		}
		multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
		if len(match) > 4 && strings.TrimSpace(match[3]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[3])
			source, sourceOK := indicatorbinding.ParsePriceSource(match[4])
			if !timeUnitOK || timeUnit == "" || !sourceOK {
				return nil, fmt.Errorf("pine line %d: bollinger() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", lineNumber)
			}
			add("bollinger", fmt.Sprintf("bollinger:%s:%d:%s:%s", source, period, multiplierText, timeUnit))
			continue
		}
		add("bollinger", "bollinger:"+strconv.Itoa(period)+":"+multiplierText)
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
		period, periodErr := indicatorbinding.ParsePositiveInt(match[2])
		if periodErr != nil {
			continue
		}
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[3])
			if !timeUnitOK || timeUnit == "" {
				return nil, fmt.Errorf("pine line %d: supertrend() timeframe %q is not supported", lineNumber, strings.TrimSpace(match[3]))
			}
			add("supertrend", "supertrend:"+strconv.FormatFloat(factor, 'f', -1, 64)+":"+strconv.Itoa(period)+":"+timeUnit)
			continue
		}
		add("supertrend", "supertrend:"+strconv.FormatFloat(factor, 'f', -1, 64)+":"+strconv.Itoa(period))
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
	for _, match := range maCallPattern.FindAllStringSubmatch(expression, -1) {
		args := splitPlannerArguments(match[1])
		if len(args) < 2 || len(args) > 4 {
			return nil, fmt.Errorf("pine line %d: ma() requires type, period, optional time unit, and optional source", lineNumber)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return nil, fmt.Errorf("pine line %d: ma() type %q is not supported", lineNumber, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return nil, fmt.Errorf("pine line %d: ma() period must be a positive integer", lineNumber)
		}
		timeUnit, source, err := indicatorbinding.ParseMovingAverageOptionalArgs(args[2:])
		if err != nil {
			return nil, fmt.Errorf("pine line %d: %w", lineNumber, err)
		}
		add("ma", indicatorbinding.BuildMovingAverageKeyWithSource(averageType, period, timeUnit, source))
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
