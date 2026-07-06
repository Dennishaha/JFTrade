package ir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

func parseAdvancedIndicatorBinding(lineNumber int, alias, name string, args []string) (plannedBinding, bool, error) {
	parser := newAdvancedIndicatorParser(lineNumber, alias, name, args)
	switch name {
	case "cog", "cmo", "dev", "median", "percentrank":
		return parser.parseSourcePeriodIndicator(2)
	case "bbw":
		return parser.parseBBWBinding()
	case "tsi":
		return parser.parseTSIBinding()
	case "correlation":
		return parser.parseCorrelationBinding()
	case "percentile_linear_interpolation", "percentile_nearest_rank":
		return parser.parsePercentileBinding()
	case "swma":
		return parser.parseSingleSourceBinding(1)
	case "linreg":
		return parser.parseLinregBinding()
	case "obv":
		return parser.parseOBVBinding()
	case "pivothigh", "pivotlow":
		return parser.parsePivotBinding()
	case "kc", "kcw":
		return parser.parseKeltnerBinding()
	case "alma":
		return parser.parseALMABinding()
	default:
		return plannedBinding{}, false, nil
	}
}

type advancedIndicatorParser struct {
	lineNumber int
	alias      string
	name       string
	args       []string
	timeUnit   string
}

func newAdvancedIndicatorParser(lineNumber int, alias, name string, args []string) *advancedIndicatorParser {
	return &advancedIndicatorParser{lineNumber: lineNumber, alias: alias, name: name, args: args}
}

func (p *advancedIndicatorParser) parseTimeUnit(index int) error {
	if len(p.args) <= index {
		return nil
	}
	if len(p.args) != index+1 {
		return fmt.Errorf("pine line %d: %s() received an invalid argument count", p.lineNumber, p.name)
	}
	parsed, ok := indicatorbinding.ParseIndicatorTimeUnitValue(p.args[index])
	if !ok || parsed == "" {
		return fmt.Errorf("pine line %d: %s() timeframe %q is not supported", p.lineNumber, p.name, strings.TrimSpace(p.args[index]))
	}
	p.timeUnit = parsed
	p.args = p.args[:index]
	return nil
}

func (p *advancedIndicatorParser) sourceArg(value string) (string, error) {
	source, ok := indicatorbinding.ParsePriceSource(value)
	if !ok {
		return "", fmt.Errorf("pine line %d: %s() source %q is not supported", p.lineNumber, p.name, strings.TrimSpace(value))
	}
	return source, nil
}

func (p *advancedIndicatorParser) withTimeUnit(key string) string {
	if p.timeUnit == "" {
		return key
	}
	return key + ":" + p.timeUnit
}

func (p *advancedIndicatorParser) buildBinding(key string, args []string) (plannedBinding, bool, error) {
	return plannedBinding{Alias: p.alias, Kind: p.name, Key: key, Args: args}, true, nil
}

func (p *advancedIndicatorParser) parseSourcePeriodIndicator(timeUnitIndex int) (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(timeUnitIndex); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 2 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source and length", p.lineNumber, p.name)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", p.lineNumber, p.name)
	}
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("%s:%s:%d", p.name, source, period)), []string{source, strconv.Itoa(period)})
}

func (p *advancedIndicatorParser) parseBBWBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() requires source, length, and multiplier", p.lineNumber)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() length must be positive", p.lineNumber)
	}
	multiplier, err := indicatorbinding.ParsePositiveFloat(p.args[2])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: bbw() multiplier must be positive", p.lineNumber)
	}
	multiplierText := strconv.FormatFloat(multiplier, 'f', -1, 64)
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("bbw:%s:%d:%s", source, period, multiplierText)), []string{source, strconv.Itoa(period), multiplierText})
}

func (p *advancedIndicatorParser) parseTSIBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() requires source, short length, and long length", p.lineNumber)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	shortPeriod, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() short length must be positive", p.lineNumber)
	}
	longPeriod, err := indicatorbinding.ParsePositiveInt(p.args[2])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: tsi() long length must be positive", p.lineNumber)
	}
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("tsi:%s:%d:%d", source, shortPeriod, longPeriod)), []string{source, strconv.Itoa(shortPeriod), strconv.Itoa(longPeriod)})
}

func (p *advancedIndicatorParser) parseCorrelationBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: correlation() requires source, second source, and length", p.lineNumber)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	source2, err := p.sourceArg(p.args[1])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[2])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: correlation() length must be positive", p.lineNumber)
	}
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("correlation:%s:%s:%d", source, source2, period)), []string{source, source2, strconv.Itoa(period)})
}

func (p *advancedIndicatorParser) parsePercentileBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, and percentage", p.lineNumber, p.name)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", p.lineNumber, p.name)
	}
	percentage, err := strconv.ParseFloat(strings.TrimSpace(p.args[2]), 64)
	if err != nil || percentage < 0 || percentage > 100 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() percentage must be between 0 and 100", p.lineNumber, p.name)
	}
	percentageText := strconv.FormatFloat(percentage, 'f', -1, 64)
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("%s:%s:%d:%s", p.name, source, period, percentageText)), []string{source, strconv.Itoa(period), percentageText})
}

func (p *advancedIndicatorParser) parseSingleSourceBinding(timeUnitIndex int) (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(timeUnitIndex); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 1 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires one source", p.lineNumber, p.name)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	return p.buildBinding(p.withTimeUnit(p.name+":"+source), []string{source})
}

func (p *advancedIndicatorParser) parseLinregBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 3 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() requires source, length, and offset", p.lineNumber)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() length must be positive", p.lineNumber)
	}
	offset, err := strconv.Atoi(strings.TrimSpace(p.args[2]))
	if err != nil || offset < 0 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: linreg() offset must be a non-negative integer", p.lineNumber)
	}
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("linreg:%s:%d:%d", source, period, offset)), []string{source, strconv.Itoa(period), strconv.Itoa(offset)})
}

func (p *advancedIndicatorParser) parseOBVBinding() (plannedBinding, bool, error) {
	if len(p.args) == 0 {
		p.args = []string{"close"}
	}
	return p.parseSingleSourceBinding(1)
}

func (p *advancedIndicatorParser) parsePivotBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(3); err != nil {
		return plannedBinding{}, false, err
	}
	source := "high"
	if p.name == "pivotlow" {
		source = "low"
	}
	lengthArgs := p.args
	if len(p.args) == 3 {
		var err error
		source, err = p.sourceArg(p.args[0])
		if err != nil {
			return plannedBinding{}, false, err
		}
		lengthArgs = p.args[1:]
	}
	if len(lengthArgs) != 2 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires left and right bars with optional source", p.lineNumber, p.name)
	}
	left, err := indicatorbinding.ParsePositiveInt(lengthArgs[0])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() left bars must be positive", p.lineNumber, p.name)
	}
	right, err := indicatorbinding.ParsePositiveInt(lengthArgs[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() right bars must be positive", p.lineNumber, p.name)
	}
	return p.buildBinding(p.withTimeUnit(fmt.Sprintf("%s:%s:%d:%d", p.name, source, left, right)), []string{source, strconv.Itoa(left), strconv.Itoa(right)})
}

func (p *advancedIndicatorParser) parseKeltnerBinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(4); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) < 3 || len(p.args) > 4 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() requires source, length, multiplier, and optional useTrueRange", p.lineNumber, p.name)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() length must be positive", p.lineNumber, p.name)
	}
	multiplier, err := indicatorbinding.ParsePositiveFloat(p.args[2])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() multiplier must be positive", p.lineNumber, p.name)
	}
	useTR := true
	if len(p.args) == 4 {
		parsed, parseErr := strconv.ParseBool(strings.TrimSpace(p.args[3]))
		if parseErr != nil {
			return plannedBinding{}, false, fmt.Errorf("pine line %d: %s() useTrueRange must be boolean", p.lineNumber, p.name)
		}
		useTR = parsed
	}
	key := p.withTimeUnit(fmt.Sprintf("%s:%s:%d:%s:%t", p.name, source, period, strconv.FormatFloat(multiplier, 'f', -1, 64), useTR))
	return p.buildBinding(key, p.args)
}

func (p *advancedIndicatorParser) parseALMABinding() (plannedBinding, bool, error) {
	if err := p.parseTimeUnit(4); err != nil {
		return plannedBinding{}, false, err
	}
	if len(p.args) != 4 {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() requires source, length, offset, and sigma", p.lineNumber)
	}
	source, err := p.sourceArg(p.args[0])
	if err != nil {
		return plannedBinding{}, false, err
	}
	period, err := indicatorbinding.ParsePositiveInt(p.args[1])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() length must be positive", p.lineNumber)
	}
	offset, err := strconv.ParseFloat(strings.TrimSpace(p.args[2]), 64)
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() offset must be numeric", p.lineNumber)
	}
	sigma, err := indicatorbinding.ParsePositiveFloat(p.args[3])
	if err != nil {
		return plannedBinding{}, false, fmt.Errorf("pine line %d: alma() sigma must be positive", p.lineNumber)
	}
	key := p.withTimeUnit(fmt.Sprintf("alma:%s:%d:%s:%s", source, period, strconv.FormatFloat(offset, 'f', -1, 64), strconv.FormatFloat(sigma, 'f', -1, 64)))
	return p.buildBinding(key, p.args)
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
