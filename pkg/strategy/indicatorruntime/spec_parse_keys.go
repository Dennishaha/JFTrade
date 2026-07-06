package indicatorruntime

import (
	"fmt"
	"strconv"
	"strings"
)

type indicatorRequirementSetBuilder struct {
	strict            bool
	maSet             map[movingAverageConfig]struct{}
	securitySourceSet map[securitySourceConfig]struct{}
	rsiSet            map[int]struct{}
	rsiSourceSet      map[sourcePeriodConfig]struct{}
	macdSet           map[macdConfig]struct{}
	bollingerSet      map[bollingerConfig]struct{}
	kdjSet            map[kdjConfig]struct{}
	atrSet            map[int]struct{}
	stdevSet          map[int]struct{}
	stdevSourceSet    map[sourcePeriodConfig]struct{}
	varianceSet       map[sourcePeriodConfig]struct{}
	windowSet         map[windowConfig]struct{}
	cumSet            map[sourceConfig]struct{}
	stochSet          map[sourcePeriodConfig]struct{}
	cciSet            map[int]struct{}
	cciSourceSet      map[sourcePeriodConfig]struct{}
	williamsRSet      map[int]struct{}
	vwapSet           map[sourceConfig]struct{}
	mfiSet            map[sourcePeriodConfig]struct{}
	dmiSet            map[dmiConfig]struct{}
	supertrendSet     map[supertrendConfig]struct{}
	sarSet            map[sarConfig]struct{}
	stopLossSet       map[stopLossConfig]struct{}
	rsiDivergenceSet  map[rsiDivergenceConfig]struct{}
	macdDivergenceSet map[macdDivergenceConfig]struct{}
	kdjDivergenceSet  map[kdjDivergenceConfig]struct{}
	advancedSet       map[advancedIndicatorConfig]struct{}
}

func newIndicatorRequirementSetBuilder(strict bool) *indicatorRequirementSetBuilder {
	return &indicatorRequirementSetBuilder{
		strict:            strict,
		maSet:             map[movingAverageConfig]struct{}{},
		securitySourceSet: map[securitySourceConfig]struct{}{},
		rsiSet:            map[int]struct{}{},
		rsiSourceSet:      map[sourcePeriodConfig]struct{}{},
		macdSet:           map[macdConfig]struct{}{},
		bollingerSet:      map[bollingerConfig]struct{}{},
		kdjSet:            map[kdjConfig]struct{}{},
		atrSet:            map[int]struct{}{},
		stdevSet:          map[int]struct{}{},
		stdevSourceSet:    map[sourcePeriodConfig]struct{}{},
		varianceSet:       map[sourcePeriodConfig]struct{}{},
		windowSet:         map[windowConfig]struct{}{},
		cumSet:            map[sourceConfig]struct{}{},
		stochSet:          map[sourcePeriodConfig]struct{}{},
		cciSet:            map[int]struct{}{},
		cciSourceSet:      map[sourcePeriodConfig]struct{}{},
		williamsRSet:      map[int]struct{}{},
		vwapSet:           map[sourceConfig]struct{}{},
		mfiSet:            map[sourcePeriodConfig]struct{}{},
		dmiSet:            map[dmiConfig]struct{}{},
		supertrendSet:     map[supertrendConfig]struct{}{},
		sarSet:            map[sarConfig]struct{}{},
		stopLossSet:       map[stopLossConfig]struct{}{},
		rsiDivergenceSet:  map[rsiDivergenceConfig]struct{}{},
		macdDivergenceSet: map[macdDivergenceConfig]struct{}{},
		kdjDivergenceSet:  map[kdjDivergenceConfig]struct{}{},
		advancedSet:       map[advancedIndicatorConfig]struct{}{},
	}
}

func (b *indicatorRequirementSetBuilder) parseKey(rawKey string) error {
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return nil
	}
	parts := strings.Split(key, ":")
	if len(parts) < 2 {
		return b.invalidKey("invalid indicator key: %s", key)
	}

	switch parts[0] {
	case "anchored_vwap", "cog", "cmo", "dev", "median", "percentrank", "bbw", "tsi",
		"correlation", "percentile_linear_interpolation", "percentile_nearest_rank",
		"swma", "linreg", "obv", "pivothigh", "pivotlow", "kc", "kcw", "alma":
		return b.parseAdvancedKey(key, parts)
	case "ma":
		return b.parseMovingAverageKey(key, parts)
	case "security_source":
		return b.parseSecuritySourceKey(key, parts)
	case "rsi":
		return b.parseRSIKey(key, parts)
	case "stdev":
		return b.parseStdevKey(key, parts)
	case "variance":
		return b.parseVarianceKey(key, parts)
	case "cum":
		return b.parseCumKey(key, parts)
	case "macd":
		return b.parseMACDKey(key, parts)
	case "bollinger":
		return b.parseBollingerKey(key, parts)
	case "kdj":
		return b.parseKDJKey(key, parts)
	case "atr":
		return b.parseATRKey(key, parts)
	case "cci":
		return b.parseCCIKey(key, parts)
	case "vwap":
		return b.parseVWAPKey(key, parts)
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
		return b.parseWindowKey(key, parts)
	case "stoch":
		return b.parseStochKey(key, parts)
	case "mfi":
		return b.parseMFIKey(key, parts)
	case "dmi":
		return b.parseDMIKey(key, parts)
	case "supertrend":
		return b.parseSupertrendKey(key, parts)
	case "sar":
		return b.parseSARKey(key, parts)
	case "williamsr":
		return b.parseWilliamsRKey(key, parts)
	case "sl", "risk":
		return b.parseStopLossKey(key, parts)
	case "divergence":
		return b.parseDivergenceKey(key, parts)
	default:
		return b.invalidKey("unsupported indicator key: %s", key)
	}
}

func (b *indicatorRequirementSetBuilder) build() indicatorRequirements {
	return indicatorRequirements{
		ma:             sortedMovingAverageConfigs(b.maSet),
		securitySource: sortedSecuritySourceConfigs(b.securitySourceSet),
		rsi:            sortedInts(b.rsiSet),
		rsiSource:      sortedSourcePeriodConfigs(b.rsiSourceSet),
		macd:           sortedMACDConfigs(b.macdSet),
		bollinger:      sortedBollingerConfigs(b.bollingerSet),
		kdj:            sortedKDJConfigs(b.kdjSet),
		atr:            sortedInts(b.atrSet),
		stdev:          sortedInts(b.stdevSet),
		stdevSource:    sortedSourcePeriodConfigs(b.stdevSourceSet),
		variance:       sortedSourcePeriodConfigs(b.varianceSet),
		windows:        sortedWindowConfigs(b.windowSet),
		cum:            sortedSourceConfigs(b.cumSet),
		stoch:          sortedSourcePeriodConfigs(b.stochSet),
		cci:            sortedInts(b.cciSet),
		cciSource:      sortedSourcePeriodConfigs(b.cciSourceSet),
		williamsR:      sortedInts(b.williamsRSet),
		vwap:           sortedSourceConfigs(b.vwapSet),
		mfi:            sortedSourcePeriodConfigs(b.mfiSet),
		dmi:            sortedDMIConfigs(b.dmiSet),
		supertrend:     sortedSupertrendConfigs(b.supertrendSet),
		sar:            sortedSARConfigs(b.sarSet),
		stopLoss:       sortedStopLossConfigs(b.stopLossSet),
		rsiDivergence:  sortedRSIDivergenceConfigs(b.rsiDivergenceSet),
		macdDivergence: sortedMACDDivergenceConfigs(b.macdDivergenceSet),
		kdjDivergence:  sortedKDJDivergenceConfigs(b.kdjDivergenceSet),
		advanced:       sortedAdvancedIndicatorConfigs(b.advancedSet),
	}
}

func (b *indicatorRequirementSetBuilder) parseAdvancedKey(key string, parts []string) error {
	switch parts[0] {
	case "anchored_vwap":
		return b.parseAnchoredVWAPKey(key, parts)
	case "cog", "cmo", "dev", "median", "percentrank":
		return b.parseAdvancedSourcePeriodKey(key, parts, parts[0], fmt.Sprintf("invalid %s key: %%s", parts[0]))
	case "bbw":
		return b.parseBBWKey(key, parts)
	case "tsi":
		return b.parseTSIKey(key, parts)
	case "correlation":
		return b.parseCorrelationKey(key, parts)
	case "percentile_linear_interpolation", "percentile_nearest_rank":
		return b.parsePercentileKey(key, parts)
	case "swma":
		return b.parseAdvancedSourceKey(key, parts, "swma", "invalid swma key: %s")
	case "linreg":
		return b.parseLinregKey(key, parts)
	case "obv":
		return b.parseAdvancedSourceKey(key, parts, "obv", "invalid obv key: %s")
	case "pivothigh", "pivotlow":
		return b.parsePivotKey(key, parts)
	case "kc", "kcw":
		return b.parseKeltnerKey(key, parts)
	case "alma":
		return b.parseALMAKey(key, parts)
	default:
		return b.invalidKey("unsupported indicator key: %s", key)
	}
}

func (b *indicatorRequirementSetBuilder) parseAnchoredVWAPKey(key string, parts []string) error {
	if len(parts) != 3 {
		return b.invalidKey("invalid anchored_vwap key: %s", key)
	}
	unit := strings.ToLower(strings.TrimSpace(parts[1]))
	source, ok := parseOHLCVSource(parts[2])
	if ok && (unit == "day" || unit == "week" || unit == "month") {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "anchored_vwap", source: source, timeUnit: unit}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid anchored_vwap key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseAdvancedSourcePeriodKey(key string, parts []string, kind string, message string) error {
	if len(parts) != 3 && len(parts) != 4 {
		return b.invalidKey(message, key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
	if sourceOK && periodOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: kind, source: source, timeUnit: timeUnit, period: period}] = struct{}{}
		return nil
	}
	return b.invalidKey(message, key)
}

func (b *indicatorRequirementSetBuilder) parseBBWKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid bbw key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "bbw", source: source, timeUnit: timeUnit, period: period, multiplier: multiplier}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid bbw key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseTSIKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid tsi key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	shortPeriod, shortOK := parsePositiveInt(parts[2])
	longPeriod, longOK := parsePositiveInt(parts[3])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && shortOK && longOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "tsi", source: source, timeUnit: timeUnit, period: shortPeriod, right: longPeriod}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid tsi key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseCorrelationKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid correlation key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	source2, source2OK := parseOHLCVSource(parts[2])
	period, periodOK := parsePositiveInt(parts[3])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && source2OK && periodOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "correlation", source: source, source2: source2, timeUnit: timeUnit, period: period}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid correlation key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parsePercentileKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid percentile key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	percentage, percentageErr := strconv.ParseFloat(parts[3], 64)
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && periodOK && percentageErr == nil && percentage >= 0 && percentage <= 100 && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, period: period, multiplier: percentage}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid percentile key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseAdvancedSourceKey(key string, parts []string, kind string, message string) error {
	if len(parts) != 2 && len(parts) != 3 {
		return b.invalidKey(message, key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 2)
	if sourceOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: kind, source: source, timeUnit: timeUnit}] = struct{}{}
		return nil
	}
	return b.invalidKey(message, key)
}

func (b *indicatorRequirementSetBuilder) parseLinregKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid linreg key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	offset, offsetOK := parseNonNegativeInt(parts[3])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && periodOK && offsetOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "linreg", source: source, timeUnit: timeUnit, period: period, offset: offset}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid linreg key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parsePivotKey(key string, parts []string) error {
	if len(parts) != 4 && len(parts) != 5 {
		return b.invalidKey("invalid pivot key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	left, leftOK := parsePositiveInt(parts[2])
	right, rightOK := parsePositiveInt(parts[3])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
	if sourceOK && leftOK && rightOK && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, left: left, right: right}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid pivot key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseKeltnerKey(key string, parts []string) error {
	if len(parts) != 5 && len(parts) != 6 {
		return b.invalidKey("invalid kc key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
	useTR, boolErr := strconv.ParseBool(parts[4])
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
	if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && boolErr == nil && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, period: period, multiplier: multiplier, useTR: useTR}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid kc key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseALMAKey(key string, parts []string) error {
	if len(parts) != 5 && len(parts) != 6 {
		return b.invalidKey("invalid alma key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	offset, offsetErr := strconv.ParseFloat(parts[3], 64)
	sigma, sigmaErr := strconv.ParseFloat(parts[4], 64)
	timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
	if sourceOK && periodOK && offsetErr == nil && sigmaErr == nil && sigma > 0 && timeUnitOK {
		b.advancedSet[advancedIndicatorConfig{key: key, kind: "alma", source: source, timeUnit: timeUnit, period: period, multiplier: offset, parameter: sigma}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid alma key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseMovingAverageKey(key string, parts []string) error {
	config, ok := parseMovingAverageConfig(parts)
	if ok {
		b.maSet[config] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid moving average key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseSecuritySourceKey(key string, parts []string) error {
	if len(parts) != 3 && len(parts) != 4 {
		return b.invalidKey("invalid security_source key: %s", key)
	}
	timeUnit := normalizeIndicatorTimeUnit(parts[1])
	source, sourceOK := parseOHLCVSource(parts[2])
	lookback := 0
	lookbackOK := true
	if len(parts) == 4 {
		lookback, lookbackOK = parseNonNegativeInt(parts[3])
	}
	if timeUnit != "" && sourceOK && lookbackOK {
		b.securitySourceSet[securitySourceConfig{source: source, timeUnit: timeUnit, lookback: lookback}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid security_source key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseRSIKey(key string, parts []string) error {
	if len(parts) == 4 {
		source, sourceOK := parseOHLCVSource(parts[1])
		period, periodOK := parsePositiveInt(parts[2])
		timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
		if sourceOK && periodOK && timeUnitOK && timeUnit != "" {
			b.advancedSet[advancedIndicatorConfig{key: key, kind: "rsi", source: source, period: period, timeUnit: timeUnit}] = struct{}{}
			return nil
		}
	}
	return b.parseLegacySourcePeriodKey(key, parts, "invalid rsi key: %s", "close", b.rsiSet, b.rsiSourceSet)
}

func (b *indicatorRequirementSetBuilder) parseStdevKey(key string, parts []string) error {
	return b.parseLegacySourcePeriodKey(key, parts, "invalid stdev key: %s", "close", b.stdevSet, b.stdevSourceSet)
}

func (b *indicatorRequirementSetBuilder) parseLegacySourcePeriodKey(
	key string,
	parts []string,
	message string,
	defaultSource string,
	defaultSet map[int]struct{},
	sourceSet map[sourcePeriodConfig]struct{},
) error {
	if len(parts) == 2 {
		period, ok := parsePositiveInt(parts[1])
		if ok {
			defaultSet[period] = struct{}{}
			return nil
		}
	}
	if len(parts) == 3 {
		source, sourceOK := parseOHLCVSource(parts[1])
		period, periodOK := parsePositiveInt(parts[2])
		if sourceOK && periodOK {
			if source == defaultSource {
				defaultSet[period] = struct{}{}
			} else {
				sourceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
			}
			return nil
		}
	}
	return b.invalidKey(message, key)
}

func (b *indicatorRequirementSetBuilder) parseVarianceKey(key string, parts []string) error {
	if len(parts) != 3 {
		return b.invalidKey("invalid variance key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	if sourceOK && periodOK {
		b.varianceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid variance key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseCumKey(key string, parts []string) error {
	if len(parts) != 2 {
		return b.invalidKey("invalid cum key: %s", key)
	}
	source, ok := parseOHLCVSource(parts[1])
	if ok {
		b.cumSet[sourceConfig{source: source}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid cum key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseMACDKey(key string, parts []string) error {
	if len(parts) == 6 {
		source, sourceOK := parseOHLCVSource(parts[1])
		fast, fastOK := parsePositiveInt(parts[2])
		slow, slowOK := parsePositiveInt(parts[3])
		signal, signalOK := parsePositiveInt(parts[4])
		timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
		if sourceOK && fastOK && slowOK && signalOK && timeUnitOK && timeUnit != "" {
			b.advancedSet[advancedIndicatorConfig{key: key, kind: "macd", source: source, period: fast, right: slow, offset: signal, timeUnit: timeUnit}] = struct{}{}
			return nil
		}
	}
	if len(parts) != 4 {
		return b.invalidKey("invalid macd key: %s", key)
	}
	fast, fastOK := parsePositiveInt(parts[1])
	slow, slowOK := parsePositiveInt(parts[2])
	signal, signalOK := parsePositiveInt(parts[3])
	if fastOK && slowOK && signalOK {
		b.macdSet[macdConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid macd key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseBollingerKey(key string, parts []string) error {
	if len(parts) == 5 {
		source, sourceOK := parseOHLCVSource(parts[1])
		period, periodOK := parsePositiveInt(parts[2])
		multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
		timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
		if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && timeUnitOK && timeUnit != "" {
			b.advancedSet[advancedIndicatorConfig{key: key, kind: "bollinger", source: source, period: period, multiplier: multiplier, timeUnit: timeUnit}] = struct{}{}
			return nil
		}
	}
	if len(parts) != 3 {
		return b.invalidKey("invalid bollinger key: %s", key)
	}
	period, periodOK := parsePositiveInt(parts[1])
	multiplier, multiplierErr := strconv.ParseFloat(parts[2], 64)
	if periodOK && multiplierErr == nil && multiplier > 0 {
		b.bollingerSet[bollingerConfig{period: period, multiplier: multiplier}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid bollinger key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseKDJKey(key string, parts []string) error {
	if len(parts) != 4 {
		return b.invalidKey("invalid kdj key: %s", key)
	}
	period, periodOK := parsePositiveInt(parts[1])
	m1, m1OK := parsePositiveInt(parts[2])
	m2, m2OK := parsePositiveInt(parts[3])
	if periodOK && m1OK && m2OK {
		b.kdjSet[kdjConfig{period: period, m1: m1, m2: m2}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid kdj key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseATRKey(key string, parts []string) error {
	if len(parts) == 3 {
		period, periodOK := parsePositiveInt(parts[1])
		timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 2)
		if periodOK && timeUnitOK && timeUnit != "" {
			b.advancedSet[advancedIndicatorConfig{key: key, kind: "atr", source: "close", period: period, timeUnit: timeUnit}] = struct{}{}
			return nil
		}
	}
	if len(parts) != 2 {
		return b.invalidKey("invalid atr key: %s", key)
	}
	period, ok := parsePositiveInt(parts[1])
	if ok {
		b.atrSet[period] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid atr key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseCCIKey(key string, parts []string) error {
	return b.parseLegacySourcePeriodKey(key, parts, "invalid cci key: %s", "hlc3", b.cciSet, b.cciSourceSet)
}

func (b *indicatorRequirementSetBuilder) parseVWAPKey(key string, parts []string) error {
	if len(parts) != 2 {
		return b.invalidKey("invalid vwap key: %s", key)
	}
	source, ok := parseOHLCVSource(parts[1])
	if ok {
		b.vwapSet[sourceConfig{source: source}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid vwap key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseWindowKey(key string, parts []string) error {
	if len(parts) != 3 {
		return b.invalidKey("invalid rolling window key: %s", key)
	}
	function := normalizeWindowFunction(parts[0])
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	if function != "" && sourceOK && periodOK {
		b.windowSet[windowConfig{function: function, source: source, period: period}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid rolling window key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseStochKey(key string, parts []string) error {
	if len(parts) != 3 && len(parts) != 4 {
		return b.invalidKey("invalid stoch key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	if !sourceOK || source == "volume" || !periodOK {
		return b.invalidKey("invalid stoch key: %s", key)
	}
	timeUnit := ""
	timeUnitOK := true
	if len(parts) == 4 {
		timeUnit, timeUnitOK = parseIndicatorTimeUnit(parts[3])
	}
	if timeUnitOK {
		b.stochSet[sourcePeriodConfig{source: source, period: period, timeUnit: timeUnit}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid stoch key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseMFIKey(key string, parts []string) error {
	if len(parts) != 3 {
		return b.invalidKey("invalid mfi key: %s", key)
	}
	source, sourceOK := parseOHLCVSource(parts[1])
	period, periodOK := parsePositiveInt(parts[2])
	if sourceOK && periodOK {
		b.mfiSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid mfi key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseDMIKey(key string, parts []string) error {
	if len(parts) != 3 {
		return b.invalidKey("invalid dmi key: %s", key)
	}
	diLength, diOK := parsePositiveInt(parts[1])
	adxSmoothing, adxOK := parsePositiveInt(parts[2])
	if diOK && adxOK {
		b.dmiSet[dmiConfig{diLength: diLength, adxSmoothing: adxSmoothing}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid dmi key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseSupertrendKey(key string, parts []string) error {
	if len(parts) == 4 {
		factor, factorErr := strconv.ParseFloat(parts[1], 64)
		atrPeriod, periodOK := parsePositiveInt(parts[2])
		timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
		if factorErr == nil && factor > 0 && periodOK && timeUnitOK && timeUnit != "" {
			b.advancedSet[advancedIndicatorConfig{key: key, kind: "supertrend", source: "close", period: atrPeriod, multiplier: factor, timeUnit: timeUnit}] = struct{}{}
			return nil
		}
	}
	if len(parts) != 3 {
		return b.invalidKey("invalid supertrend key: %s", key)
	}
	factor, factorErr := strconv.ParseFloat(parts[1], 64)
	atrPeriod, periodOK := parsePositiveInt(parts[2])
	if factorErr == nil && factor > 0 && periodOK {
		b.supertrendSet[supertrendConfig{factor: factor, atrPeriod: atrPeriod}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid supertrend key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseSARKey(key string, parts []string) error {
	if len(parts) != 4 {
		return b.invalidKey("invalid sar key: %s", key)
	}
	start, startErr := strconv.ParseFloat(parts[1], 64)
	increment, incrementErr := strconv.ParseFloat(parts[2], 64)
	maximum, maxErr := strconv.ParseFloat(parts[3], 64)
	if startErr == nil && incrementErr == nil && maxErr == nil && start > 0 && increment > 0 && maximum > 0 {
		b.sarSet[sarConfig{start: start, increment: increment, maximum: maximum}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid sar key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseWilliamsRKey(key string, parts []string) error {
	if len(parts) != 2 {
		return b.invalidKey("invalid williamsr key: %s", key)
	}
	period, ok := parsePositiveInt(parts[1])
	if ok {
		b.williamsRSet[period] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid williamsr key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseStopLossKey(key string, parts []string) error {
	config, ok := parseStopLossConfig(parts)
	if ok {
		b.stopLossSet[config] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid risk key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseDivergenceKey(key string, parts []string) error {
	if len(parts) < 5 {
		return b.invalidKey("invalid divergence key: %s", key)
	}
	direction := strings.TrimSpace(parts[len(parts)-2])
	lookback, lookbackOK := parsePositiveInt(parts[len(parts)-1])
	if !lookbackOK || (direction != "top" && direction != "bottom") {
		return b.invalidKey("invalid divergence key: %s", key)
	}
	switch parts[1] {
	case "rsi":
		return b.parseRSIDivergenceKey(key, parts, direction, lookback)
	case "macd":
		return b.parseMACDDivergenceKey(key, parts, direction, lookback)
	case "kdj":
		return b.parseKDJDivergenceKey(key, parts, direction, lookback)
	default:
		if b.strict {
			return fmt.Errorf("unsupported divergence key: %s", key)
		}
		return nil
	}
}

func (b *indicatorRequirementSetBuilder) parseRSIDivergenceKey(key string, parts []string, direction string, lookback int) error {
	if len(parts) != 5 {
		return b.invalidKey("invalid divergence key: %s", key)
	}
	period, ok := parsePositiveInt(parts[2])
	if ok {
		b.rsiDivergenceSet[rsiDivergenceConfig{period: period, direction: direction, lookback: lookback}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid divergence key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseMACDDivergenceKey(key string, parts []string, direction string, lookback int) error {
	if len(parts) != 7 {
		return b.invalidKey("invalid divergence key: %s", key)
	}
	fast, fastOK := parsePositiveInt(parts[2])
	slow, slowOK := parsePositiveInt(parts[3])
	signal, signalOK := parsePositiveInt(parts[4])
	if fastOK && slowOK && signalOK {
		b.macdDivergenceSet[macdDivergenceConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal, direction: direction, lookback: lookback}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid divergence key: %s", key)
}

func (b *indicatorRequirementSetBuilder) parseKDJDivergenceKey(key string, parts []string, direction string, lookback int) error {
	if len(parts) != 7 {
		return b.invalidKey("invalid divergence key: %s", key)
	}
	period, periodOK := parsePositiveInt(parts[2])
	m1, m1OK := parsePositiveInt(parts[3])
	m2, m2OK := parsePositiveInt(parts[4])
	if periodOK && m1OK && m2OK {
		b.kdjDivergenceSet[kdjDivergenceConfig{period: period, m1: m1, m2: m2, direction: direction, lookback: lookback}] = struct{}{}
		return nil
	}
	return b.invalidKey("invalid divergence key: %s", key)
}

func (b *indicatorRequirementSetBuilder) invalidKey(format string, args ...any) error {
	if !b.strict {
		return nil
	}
	return fmt.Errorf(format, args...)
}
