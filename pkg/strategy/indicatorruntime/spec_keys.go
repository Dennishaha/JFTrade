package indicatorruntime

import (
	"strconv"
	"strings"
)

func maIndicatorKey(config movingAverageConfig) string {
	base := "ma:" + normalizeMovingAverageType(config.averageType) + ":" + strconv.Itoa(config.period)
	timeUnit := normalizeIndicatorTimeUnit(config.timeUnit)
	source, _ := parseOHLCVSource(config.source)
	if source == "close" {
		source = ""
	}
	if timeUnit == "" && source == "" {
		return base
	}
	if timeUnit == "" {
		return base + ":" + source
	}
	if source == "" {
		return base + ":" + timeUnit
	}
	return base + ":" + timeUnit + ":" + source
}

func securitySourceIndicatorKey(config securitySourceConfig) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = "close"
	}
	key := "security_source:" + normalizeIndicatorTimeUnit(config.timeUnit) + ":" + source
	if config.lookback > 0 {
		key += ":" + strconv.Itoa(config.lookback)
	}
	return key
}

func legacyMAIndicatorKey(period int) string {
	return "ma:" + strconv.Itoa(period)
}

func rsiIndicatorKey(period int) string {
	return "rsi:" + strconv.Itoa(period)
}

func sourcePeriodIndicatorKey(prefix string, config sourcePeriodConfig, legacySource string) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = legacySource
	}
	if source == legacySource {
		return prefix + ":" + strconv.Itoa(config.period)
	}
	return prefix + ":" + source + ":" + strconv.Itoa(config.period)
}

func sourceIndicatorKey(prefix string, config sourceConfig) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(config.source))
	}
	return prefix + ":" + source
}

func macdIndicatorKey(fastPeriod, slowPeriod, signalPeriod int) string {
	return "macd:" + strconv.Itoa(fastPeriod) + ":" + strconv.Itoa(slowPeriod) + ":" + strconv.Itoa(signalPeriod)
}

func bollingerIndicatorKey(period int, multiplier float64) string {
	return "bollinger:" + strconv.Itoa(period) + ":" + strconv.FormatFloat(multiplier, 'f', -1, 64)
}

func kdjIndicatorKey(period, m1, m2 int) string {
	return "kdj:" + strconv.Itoa(period) + ":" + strconv.Itoa(m1) + ":" + strconv.Itoa(m2)
}

func atrIndicatorKey(period int) string {
	return "atr:" + strconv.Itoa(period)
}

func stdevIndicatorKey(period int) string {
	return "stdev:" + strconv.Itoa(period)
}

func varianceIndicatorKey(config sourcePeriodConfig) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = "close"
	}
	return "variance:" + source + ":" + strconv.Itoa(config.period)
}

func windowIndicatorKey(config windowConfig) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(config.source))
	}
	return normalizeWindowFunction(config.function) + ":" + source + ":" + strconv.Itoa(config.period)
}

func stochIndicatorKey(config sourcePeriodConfig) string {
	source, _ := parseOHLCVSource(config.source)
	if source == "" {
		source = "close"
	}
	return "stoch:" + source + ":" + strconv.Itoa(config.period)
}

func cciIndicatorKey(period int) string {
	return "cci:" + strconv.Itoa(period)
}

func dmiIndicatorKey(config dmiConfig) string {
	return "dmi:" + strconv.Itoa(config.diLength) + ":" + strconv.Itoa(config.adxSmoothing)
}

func supertrendIndicatorKey(config supertrendConfig) string {
	return "supertrend:" + strconv.FormatFloat(config.factor, 'f', -1, 64) + ":" + strconv.Itoa(config.atrPeriod)
}

func sarIndicatorKey(config sarConfig) string {
	return "sar:" +
		strconv.FormatFloat(config.start, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.increment, 'f', -1, 64) + ":" +
		strconv.FormatFloat(config.maximum, 'f', -1, 64)
}

func williamsRIndicatorKey(period int) string {
	return "williamsr:" + strconv.Itoa(period)
}

func stopLossIndicatorKey(config stopLossConfig) string {
	timeUnit := normalizeIndicatorTimeUnit(config.timeUnit)
	if timeUnit == "" {
		timeUnit = "bar"
	}
	mode := normalizeStopLossMode(config.mode)
	windowPolicy := normalizeStopLossWindowPolicy(config.windowPolicy)
	if mode == "stopLoss" && windowPolicy == "continuous" {
		return "sl:" + normalizeStopLossDirection(config.direction) + ":" + strconv.Itoa(config.timeValue) + ":" + timeUnit + ":" + strconv.FormatFloat(config.percentage, 'f', -1, 64)
	}
	return "risk:" + mode + ":" + normalizeStopLossDirection(config.direction) + ":" + strconv.Itoa(config.timeValue) + ":" + timeUnit + ":" + strconv.FormatFloat(config.percentage, 'f', -1, 64) + ":" + windowPolicy
}

func rsiDivergenceIndicatorKey(period int, direction string, lookback int) string {
	return "divergence:rsi:" + strconv.Itoa(period) + ":" + direction + ":" + strconv.Itoa(lookback)
}

func macdDivergenceIndicatorKey(fastPeriod, slowPeriod, signalPeriod int, direction string, lookback int) string {
	return "divergence:macd:" + strconv.Itoa(fastPeriod) + ":" + strconv.Itoa(slowPeriod) + ":" + strconv.Itoa(signalPeriod) + ":" + direction + ":" + strconv.Itoa(lookback)
}

func kdjDivergenceIndicatorKey(period, m1, m2 int, direction string, lookback int) string {
	return "divergence:kdj:" + strconv.Itoa(period) + ":" + strconv.Itoa(m1) + ":" + strconv.Itoa(m2) + ":" + direction + ":" + strconv.Itoa(lookback)
}
