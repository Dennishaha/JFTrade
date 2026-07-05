package ir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

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
	case "cog", "cmo", "dev", "median", "percentrank":
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
	case "bbw":
		if err := parseTimeUnit(3); err != nil {
			return plannedBinding{}, false, err
		}
		if len(args) != 3 {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() requires source, length, and multiplier", lineNumber)
		}
		source, err := sourceArg(args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() length must be positive", lineNumber)
		}
		multiplier, err := indicatorbinding.ParsePositiveFloat(args[2])
		if err != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() multiplier must be positive", lineNumber)
		}
		multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
		key := withTimeUnit(fmt.Sprintf("bbw:%s:%d:%s", source, period, multiplierText))
		return plannedBinding{Alias: alias, Kind: name, Key: key, Args: []string{source, strconv.Itoa(period), multiplierText}}, true, nil
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

func parseMACDBinding(lineNumber int, alias string, name string, args []string) (plannedBinding, error) {
	if len(args) != 3 && len(args) != 5 {
		return plannedBinding{}, fmt.Errorf("pine line %d: %s() requires fast, slow, signal, optional time unit, and optional source", lineNumber, name)
	}
	values, err := indicatorbinding.ExpectPositiveIntArgs(lineNumber, name, args[:3], 3)
	if err != nil {
		return plannedBinding{}, err
	}
	bindingArgs := indicatorbinding.IntsToStrings(values)
	key := fmt.Sprintf("macd:%d:%d:%d", values[0], values[1], values[2])
	if len(args) == 5 {
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(args[3])
		source, sourceOK := indicatorbinding.ParsePriceSource(args[4])
		if !timeUnitOK || timeUnit == "" || !sourceOK {
			return plannedBinding{}, fmt.Errorf("pine line %d: macd() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", lineNumber)
		}
		key = fmt.Sprintf("macd:%s:%d:%d:%d:%s", source, values[0], values[1], values[2], timeUnit)
		bindingArgs = append(bindingArgs, timeUnit, source)
	}
	return plannedBinding{Alias: alias, Kind: "macd", Key: key, Args: bindingArgs}, nil
}
