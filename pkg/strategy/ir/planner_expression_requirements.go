package ir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorbinding"
)

type expressionRequirementCollector struct {
	lineNumber   int
	requirements []IndicatorRequirement
	seen         map[string]struct{}
}

func newExpressionRequirementCollector(lineNumber int) *expressionRequirementCollector {
	return &expressionRequirementCollector{
		lineNumber:   lineNumber,
		requirements: make([]IndicatorRequirement, 0),
		seen:         map[string]struct{}{},
	}
}

func (c *expressionRequirementCollector) collect(expression string) error {
	parsers := []func(string) error{
		c.collectStdevRequirements,
		c.collectRSIRequirements,
		c.collectMACDRequirements,
		c.collectATRRequirements,
		c.collectBollingerRequirements,
		c.collectCCIRequirements,
		c.collectVarianceRequirements,
		c.collectCumRequirements,
		c.collectWindowRequirements,
		c.collectStochRequirements,
		c.collectVWAPRequirements,
		c.collectAnchoredVWAPRequirements,
		c.collectMFIRequirements,
		c.collectDMIRequirements,
		c.collectSupertrendRequirements,
		c.collectSARRequirements,
		c.collectMARequirements,
		c.collectSecuritySourceRequirements,
	}
	for _, parser := range parsers {
		if err := parser(expression); err != nil {
			return err
		}
	}
	return nil
}

func (c *expressionRequirementCollector) add(kind string, key string) {
	if _, exists := c.seen[key]; exists {
		return
	}
	c.seen[key] = struct{}{}
	c.requirements = append(c.requirements, IndicatorRequirement{Kind: kind, Key: key})
}

func (c *expressionRequirementCollector) collectStdevRequirements(expression string) error {
	for _, match := range stdevCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: stdev() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		c.add("stdev", sourcePeriodKey("stdev", source, period, "close"))
	}
	return nil
}

func (c *expressionRequirementCollector) collectRSIRequirements(expression string) error {
	for _, match := range rsiCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: rsi() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[3])
			if !timeUnitOK || timeUnit == "" {
				return fmt.Errorf("pine line %d: rsi() timeframe %q is not supported", c.lineNumber, strings.TrimSpace(match[3]))
			}
			c.add("rsi", fmt.Sprintf("rsi:%s:%d:%s", source, period, timeUnit))
			continue
		}
		c.add("rsi", sourcePeriodKey("rsi", source, period, "close"))
	}
	return nil
}

func (c *expressionRequirementCollector) collectMACDRequirements(expression string) error {
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
				return fmt.Errorf("pine line %d: macd() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", c.lineNumber)
			}
			c.add("macd", fmt.Sprintf("macd:%s:%d:%d:%d:%s", source, fast, slow, signal, timeUnit))
			continue
		}
		c.add("macd", fmt.Sprintf("macd:%d:%d:%d", fast, slow, signal))
	}
	return nil
}

func (c *expressionRequirementCollector) collectATRRequirements(expression string) error {
	for _, match := range atrCallPattern.FindAllStringSubmatch(expression, -1) {
		period, err := indicatorbinding.ParsePositiveInt(match[1])
		if err != nil {
			continue
		}
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[2])
			if !timeUnitOK || timeUnit == "" {
				return fmt.Errorf("pine line %d: atr() timeframe %q is not supported", c.lineNumber, strings.TrimSpace(match[2]))
			}
			c.add("atr", fmt.Sprintf("atr:%d:%s", period, timeUnit))
			continue
		}
		c.add("atr", "atr:"+strconv.Itoa(period))
	}
	return nil
}

func (c *expressionRequirementCollector) collectBollingerRequirements(expression string) error {
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
				return fmt.Errorf("pine line %d: bollinger() supports OHLCV/hl2/hlc3/ohlc4 source and supported timeframe", c.lineNumber)
			}
			c.add("bollinger", fmt.Sprintf("bollinger:%s:%d:%s:%s", source, period, multiplierText, timeUnit))
			continue
		}
		c.add("bollinger", "bollinger:"+strconv.Itoa(period)+":"+multiplierText)
	}
	return nil
}

func (c *expressionRequirementCollector) collectCCIRequirements(expression string) error {
	for _, match := range cciCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: cci() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		c.add("cci", sourcePeriodKey("cci", source, period, "hlc3"))
	}
	return nil
}

func (c *expressionRequirementCollector) collectVarianceRequirements(expression string) error {
	for _, match := range varianceCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: variance() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			continue
		}
		c.add("variance", "variance:"+source+":"+strconv.Itoa(period))
	}
	return nil
}

func (c *expressionRequirementCollector) collectCumRequirements(expression string) error {
	for _, match := range cumCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: cum() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		c.add("cum", "cum:"+source)
	}
	return nil
}

func (c *expressionRequirementCollector) collectWindowRequirements(expression string) error {
	for _, match := range windowCallPattern.FindAllStringSubmatch(expression, -1) {
		function := strings.ToLower(strings.TrimSpace(match[1]))
		source, ok := indicatorbinding.ParsePriceSource(match[2])
		if !ok {
			return fmt.Errorf("pine line %d: %s() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, function, strings.TrimSpace(match[2]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[3])
		if err != nil {
			return fmt.Errorf("pine line %d: %s() length must be a positive integer", c.lineNumber, function)
		}
		c.add(function, function+":"+source+":"+strconv.Itoa(period))
	}
	return nil
}

func (c *expressionRequirementCollector) collectStochRequirements(expression string) error {
	for _, match := range stochCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := parseStochSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: stoch() source %q is not supported; use open/high/low/close/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		if !strings.EqualFold(strings.TrimSpace(match[2]), "high") || !strings.EqualFold(strings.TrimSpace(match[3]), "low") {
			return fmt.Errorf("pine line %d: stoch() currently supports literal high and low arguments only", c.lineNumber)
		}
		period, err := indicatorbinding.ParsePositiveInt(match[4])
		if err != nil {
			return fmt.Errorf("pine line %d: stoch() length must be a positive integer", c.lineNumber)
		}
		key := "stoch:" + source + ":" + strconv.Itoa(period)
		if len(match) > 5 && strings.TrimSpace(match[5]) != "" {
			timeUnit, ok := indicatorbinding.ParseIndicatorTimeUnitValue(match[5])
			if !ok {
				return fmt.Errorf("pine line %d: stoch() time unit %q is not supported", c.lineNumber, strings.TrimSpace(match[5]))
			}
			key += ":" + timeUnit
		}
		c.add("stoch", key)
	}
	return nil
}

func (c *expressionRequirementCollector) collectVWAPRequirements(expression string) error {
	for _, match := range vwapCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: vwap() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		c.add("vwap", "vwap:"+source)
	}
	return nil
}

func (c *expressionRequirementCollector) collectAnchoredVWAPRequirements(expression string) error {
	for _, match := range anchoredVWAPCallPattern.FindAllStringSubmatch(expression, -1) {
		source, sourceOK := indicatorbinding.ParsePriceSource(match[1])
		unit := strings.ToLower(strings.TrimSpace(match[2]))
		if !sourceOK || (unit != "day" && unit != "week" && unit != "month") {
			return fmt.Errorf("pine line %d: anchored_vwap() supports OHLCV/derived source and day/week/month anchors", c.lineNumber)
		}
		c.add("anchored_vwap", "anchored_vwap:"+unit+":"+source)
	}
	return nil
}

func (c *expressionRequirementCollector) collectMFIRequirements(expression string) error {
	for _, match := range mfiCallPattern.FindAllStringSubmatch(expression, -1) {
		source, ok := indicatorbinding.ParsePriceSource(match[1])
		if !ok {
			return fmt.Errorf("pine line %d: mfi() source %q is not supported; use open/high/low/close/volume/hl2/hlc3/ohlc4", c.lineNumber, strings.TrimSpace(match[1]))
		}
		period, err := indicatorbinding.ParsePositiveInt(match[2])
		if err != nil {
			return fmt.Errorf("pine line %d: mfi() length must be a positive integer", c.lineNumber)
		}
		c.add("mfi", "mfi:"+source+":"+strconv.Itoa(period))
	}
	return nil
}

func (c *expressionRequirementCollector) collectDMIRequirements(expression string) error {
	for _, match := range dmiCallPattern.FindAllStringSubmatch(expression, -1) {
		c.add("dmi", "dmi:"+strings.TrimSpace(match[1])+":"+strings.TrimSpace(match[2]))
	}
	return nil
}

func (c *expressionRequirementCollector) collectSupertrendRequirements(expression string) error {
	for _, match := range supertrendCallPattern.FindAllStringSubmatch(expression, -1) {
		factor, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err != nil || factor <= 0 {
			continue
		}
		period, periodErr := indicatorbinding.ParsePositiveInt(match[2])
		if periodErr != nil {
			continue
		}
		factorText := strconv.FormatFloat(factor, 'f', -1, 64)
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[3])
			if !timeUnitOK || timeUnit == "" {
				return fmt.Errorf("pine line %d: supertrend() timeframe %q is not supported", c.lineNumber, strings.TrimSpace(match[3]))
			}
			c.add("supertrend", "supertrend:"+factorText+":"+strconv.Itoa(period)+":"+timeUnit)
			continue
		}
		c.add("supertrend", "supertrend:"+factorText+":"+strconv.Itoa(period))
	}
	return nil
}

func (c *expressionRequirementCollector) collectSARRequirements(expression string) error {
	for _, match := range sarCallPattern.FindAllStringSubmatch(expression, -1) {
		start, startErr := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		increment, incrementErr := strconv.ParseFloat(strings.TrimSpace(match[2]), 64)
		maximum, maxErr := strconv.ParseFloat(strings.TrimSpace(match[3]), 64)
		if startErr != nil || incrementErr != nil || maxErr != nil || start <= 0 || increment <= 0 || maximum <= 0 {
			continue
		}
		c.add("sar", sarPlannerKey(sarPlannerConfig{start: start, increment: increment, maximum: maximum}))
	}
	return nil
}

func (c *expressionRequirementCollector) collectMARequirements(expression string) error {
	for _, match := range maCallPattern.FindAllStringSubmatch(expression, -1) {
		args := splitPlannerArguments(match[1])
		if len(args) < 2 || len(args) > 4 {
			return fmt.Errorf("pine line %d: ma() requires type, period, optional time unit, and optional source", c.lineNumber)
		}
		averageType, ok := indicatorbinding.ParseMovingAverageType(args[0])
		if !ok {
			return fmt.Errorf("pine line %d: ma() type %q is not supported", c.lineNumber, strings.TrimSpace(args[0]))
		}
		period, err := indicatorbinding.ParsePositiveInt(args[1])
		if err != nil {
			return fmt.Errorf("pine line %d: ma() period must be a positive integer", c.lineNumber)
		}
		timeUnit, source, err := indicatorbinding.ParseMovingAverageOptionalArgs(args[2:])
		if err != nil {
			return fmt.Errorf("pine line %d: %w", c.lineNumber, err)
		}
		c.add("ma", indicatorbinding.BuildMovingAverageKeyWithSource(averageType, period, timeUnit, source))
	}
	return nil
}

func (c *expressionRequirementCollector) collectSecuritySourceRequirements(expression string) error {
	for _, match := range securitySourceCallPattern.FindAllStringSubmatch(expression, -1) {
		source, sourceOK := indicatorbinding.ParsePriceSource(match[1])
		timeUnit, timeUnitOK := indicatorbinding.ParseIndicatorTimeUnitValue(match[2])
		if !sourceOK || !timeUnitOK || timeUnit == "" {
			return fmt.Errorf("pine line %d: security_source() supports open/high/low/close/volume/hl2/hlc3/ohlc4 and supported higher timeframes", c.lineNumber)
		}
		lookback := 0
		if strings.TrimSpace(match[3]) != "" {
			parsed, err := strconv.Atoi(strings.TrimSpace(match[3]))
			if err != nil || parsed < 0 {
				return fmt.Errorf("pine line %d: security_source() lookback must be a non-negative integer", c.lineNumber)
			}
			lookback = parsed
		}
		c.add("security_source", securitySourcePlannerKey(source, timeUnit, lookback))
	}
	return nil
}
