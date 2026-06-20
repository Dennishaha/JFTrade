package indicatorruntime

import (
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var indicatorKeyPattern = regexp.MustCompile(`ctx\.indicators\[(?:"([^"]+)"|'([^']+)')\]`)

func parseIndicatorRequirements(script string) indicatorRequirements {
	keys := make([]string, 0)
	for _, match := range indicatorKeyPattern.FindAllStringSubmatch(script, -1) {
		key := strings.TrimSpace(firstNonEmpty(match[1], match[2]))
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	requirements, jftradeErr1 := parseIndicatorRequirementKeys(keys, false)
	jftradeLogError(jftradeErr1)
	return requirements
}

func indicatorRequirementsFromPlan(plan strategyir.Requirements) (indicatorRequirements, error) {
	keys := make([]string, 0, len(plan.Indicators))
	for _, requirement := range plan.Indicators {
		key := strings.TrimSpace(requirement.Key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	return parseIndicatorRequirementKeys(keys, true)
}

func parseIndicatorRequirementKeys(keys []string, strict bool) (indicatorRequirements, error) {
	maSet := map[movingAverageConfig]struct{}{}
	securitySourceSet := map[securitySourceConfig]struct{}{}
	rsiSet := map[int]struct{}{}
	rsiSourceSet := map[sourcePeriodConfig]struct{}{}
	macdSet := map[macdConfig]struct{}{}
	bollingerSet := map[bollingerConfig]struct{}{}
	kdjSet := map[kdjConfig]struct{}{}
	atrSet := map[int]struct{}{}
	stdevSet := map[int]struct{}{}
	stdevSourceSet := map[sourcePeriodConfig]struct{}{}
	varianceSet := map[sourcePeriodConfig]struct{}{}
	windowSet := map[windowConfig]struct{}{}
	cumSet := map[sourceConfig]struct{}{}
	stochSet := map[sourcePeriodConfig]struct{}{}
	cciSet := map[int]struct{}{}
	cciSourceSet := map[sourcePeriodConfig]struct{}{}
	williamsRSet := map[int]struct{}{}
	vwapSet := map[sourceConfig]struct{}{}
	mfiSet := map[sourcePeriodConfig]struct{}{}
	dmiSet := map[dmiConfig]struct{}{}
	supertrendSet := map[supertrendConfig]struct{}{}
	sarSet := map[sarConfig]struct{}{}
	stopLossSet := map[stopLossConfig]struct{}{}
	rsiDivergenceSet := map[rsiDivergenceConfig]struct{}{}
	macdDivergenceSet := map[macdDivergenceConfig]struct{}{}
	kdjDivergenceSet := map[kdjDivergenceConfig]struct{}{}
	advancedSet := map[advancedIndicatorConfig]struct{}{}

	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid indicator key: %s", key)
			}
			continue
		}

		switch parts[0] {
		case "anchored_vwap":
			if len(parts) == 3 {
				unit := strings.ToLower(strings.TrimSpace(parts[1]))
				source, sourceOK := parseOHLCVSource(parts[2])
				if sourceOK && (unit == "day" || unit == "week" || unit == "month") {
					advancedSet[advancedIndicatorConfig{key: key, kind: "anchored_vwap", source: source, timeUnit: unit}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid anchored_vwap key: %s", key)
			}
		case "cog", "cmo", "dev", "median", "percentrank":
			if len(parts) == 3 || len(parts) == 4 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
				if sourceOK && periodOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, period: period}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid %s key: %s", parts[0], key)
			}
		case "bbw":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "bbw", source: source, timeUnit: timeUnit, period: period, multiplier: multiplier}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid bbw key: %s", key)
			}
		case "tsi":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				shortPeriod, shortOK := parsePositiveInt(parts[2])
				longPeriod, longOK := parsePositiveInt(parts[3])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && shortOK && longOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "tsi", source: source, timeUnit: timeUnit, period: shortPeriod, right: longPeriod}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid tsi key: %s", key)
			}
		case "correlation":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				source2, source2OK := parseOHLCVSource(parts[2])
				period, periodOK := parsePositiveInt(parts[3])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && source2OK && periodOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "correlation", source: source, source2: source2, timeUnit: timeUnit, period: period}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid correlation key: %s", key)
			}
		case "percentile_linear_interpolation", "percentile_nearest_rank":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				percentage, percentageErr := strconv.ParseFloat(parts[3], 64)
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && periodOK && percentageErr == nil && percentage >= 0 && percentage <= 100 && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, period: period, multiplier: percentage}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid percentile key: %s", key)
			}
		case "swma":
			if len(parts) == 2 || len(parts) == 3 {
				source, sourceOK := parseOHLCVSource(parts[1])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 2)
				if sourceOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "swma", source: source, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid swma key: %s", key)
			}
		case "linreg":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				offset, offsetOK := parseNonNegativeInt(parts[3])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && periodOK && offsetOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "linreg", source: source, timeUnit: timeUnit, period: period, offset: offset}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid linreg key: %s", key)
			}
		case "obv":
			if len(parts) == 2 || len(parts) == 3 {
				source, sourceOK := parseOHLCVSource(parts[1])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 2)
				if sourceOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "obv", source: source, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid obv key: %s", key)
			}
		case "pivothigh", "pivotlow":
			if len(parts) == 4 || len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				left, leftOK := parsePositiveInt(parts[2])
				right, rightOK := parsePositiveInt(parts[3])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && leftOK && rightOK && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, left: left, right: right}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid pivot key: %s", key)
			}
		case "kc", "kcw":
			if len(parts) == 5 || len(parts) == 6 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
				useTR, boolErr := strconv.ParseBool(parts[4])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
				if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && boolErr == nil && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: parts[0], source: source, timeUnit: timeUnit, period: period, multiplier: multiplier, useTR: useTR}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid kc key: %s", key)
			}
		case "alma":
			if len(parts) == 5 || len(parts) == 6 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				offset, offsetErr := strconv.ParseFloat(parts[3], 64)
				sigma, sigmaErr := strconv.ParseFloat(parts[4], 64)
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
				if sourceOK && periodOK && offsetErr == nil && sigmaErr == nil && sigma > 0 && timeUnitOK {
					advancedSet[advancedIndicatorConfig{key: key, kind: "alma", source: source, timeUnit: timeUnit, period: period, multiplier: offset, parameter: sigma}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid alma key: %s", key)
			}
		case "ma":
			config, ok := parseMovingAverageConfig(parts)
			if ok {
				maSet[config] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid moving average key: %s", key)
			}
		case "security_source":
			if len(parts) != 3 && len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid security_source key: %s", key)
				}
				continue
			}
			timeUnit := normalizeIndicatorTimeUnit(parts[1])
			source, sourceOK := parseOHLCVSource(parts[2])
			lookback := 0
			lookbackOK := true
			if len(parts) == 4 {
				lookback, lookbackOK = parseNonNegativeInt(parts[3])
			}
			if timeUnit != "" && sourceOK {
				if lookbackOK {
					securitySourceSet[securitySourceConfig{source: source, timeUnit: timeUnit, lookback: lookback}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid security_source key: %s", key)
			}
		case "rsi":
			if len(parts) == 4 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
				if sourceOK && periodOK && timeUnitOK && timeUnit != "" {
					advancedSet[advancedIndicatorConfig{key: key, kind: "rsi", source: source, period: period, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if len(parts) == 2 {
				period, ok := parsePositiveInt(parts[1])
				if ok {
					rsiSet[period] = struct{}{}
					continue
				}
			}
			if len(parts) == 3 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				if sourceOK && periodOK {
					if source == "close" {
						rsiSet[period] = struct{}{}
					} else {
						rsiSourceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
					}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid rsi key: %s", key)
			}
		case "stdev":
			if len(parts) == 2 {
				period, ok := parsePositiveInt(parts[1])
				if ok {
					stdevSet[period] = struct{}{}
					continue
				}
			}
			if len(parts) == 3 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				if sourceOK && periodOK {
					if source == "close" {
						stdevSet[period] = struct{}{}
					} else {
						stdevSourceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
					}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid stdev key: %s", key)
			}
		case "variance":
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid variance key: %s", key)
				}
				continue
			}
			source, sourceOK := parseOHLCVSource(parts[1])
			period, periodOK := parsePositiveInt(parts[2])
			if sourceOK && periodOK {
				varianceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid variance key: %s", key)
			}
		case "cum":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid cum key: %s", key)
				}
				continue
			}
			source, ok := parseOHLCVSource(parts[1])
			if ok {
				cumSet[sourceConfig{source: source}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid cum key: %s", key)
			}
		case "macd":
			if len(parts) == 6 {
				source, sourceOK := parseOHLCVSource(parts[1])
				fast, fastOK := parsePositiveInt(parts[2])
				slow, slowOK := parsePositiveInt(parts[3])
				signal, signalOK := parsePositiveInt(parts[4])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 5)
				if sourceOK && fastOK && slowOK && signalOK && timeUnitOK && timeUnit != "" {
					advancedSet[advancedIndicatorConfig{key: key, kind: "macd", source: source, period: fast, right: slow, offset: signal, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid macd key: %s", key)
				}
				continue
			}
			fast, fastOK := parsePositiveInt(parts[1])
			slow, slowOK := parsePositiveInt(parts[2])
			signal, signalOK := parsePositiveInt(parts[3])
			if fastOK && slowOK && signalOK {
				macdSet[macdConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid macd key: %s", key)
			}
		case "bollinger":
			if len(parts) == 5 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				multiplier, multiplierErr := strconv.ParseFloat(parts[3], 64)
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 4)
				if sourceOK && periodOK && multiplierErr == nil && multiplier > 0 && timeUnitOK && timeUnit != "" {
					advancedSet[advancedIndicatorConfig{key: key, kind: "bollinger", source: source, period: period, multiplier: multiplier, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid bollinger key: %s", key)
				}
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			multiplier, multiplierErr := strconv.ParseFloat(parts[2], 64)
			if periodOK && multiplierErr == nil && multiplier > 0 {
				bollingerSet[bollingerConfig{period: period, multiplier: multiplier}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid bollinger key: %s", key)
			}
		case "kdj":
			if len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid kdj key: %s", key)
				}
				continue
			}
			period, periodOK := parsePositiveInt(parts[1])
			m1, m1OK := parsePositiveInt(parts[2])
			m2, m2OK := parsePositiveInt(parts[3])
			if periodOK && m1OK && m2OK {
				kdjSet[kdjConfig{period: period, m1: m1, m2: m2}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid kdj key: %s", key)
			}
		case "atr":
			if len(parts) == 3 {
				period, periodOK := parsePositiveInt(parts[1])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 2)
				if periodOK && timeUnitOK && timeUnit != "" {
					advancedSet[advancedIndicatorConfig{key: key, kind: "atr", source: "close", period: period, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid atr key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				atrSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid atr key: %s", key)
			}
		case "cci":
			if len(parts) == 2 {
				period, ok := parsePositiveInt(parts[1])
				if ok {
					cciSet[period] = struct{}{}
					continue
				}
			}
			if len(parts) == 3 {
				source, sourceOK := parseOHLCVSource(parts[1])
				period, periodOK := parsePositiveInt(parts[2])
				if sourceOK && periodOK {
					if source == "hlc3" {
						cciSet[period] = struct{}{}
					} else {
						cciSourceSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
					}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid cci key: %s", key)
			}
		case "vwap":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid vwap key: %s", key)
				}
				continue
			}
			source, ok := parseOHLCVSource(parts[1])
			if ok {
				vwapSet[sourceConfig{source: source}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid vwap key: %s", key)
			}
		case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid rolling window key: %s", key)
				}
				continue
			}
			function := normalizeWindowFunction(parts[0])
			source, sourceOK := parseOHLCVSource(parts[1])
			period, periodOK := parsePositiveInt(parts[2])
			if function != "" && sourceOK && periodOK {
				windowSet[windowConfig{function: function, source: source, period: period}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid rolling window key: %s", key)
			}
		case "stoch":
			if len(parts) != 3 && len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid stoch key: %s", key)
				}
				continue
			}
			source, sourceOK := parseOHLCVSource(parts[1])
			period, periodOK := parsePositiveInt(parts[2])
			if sourceOK && source != "volume" && periodOK {
				timeUnit := ""
				timeUnitOK := true
				if len(parts) == 4 {
					timeUnit, timeUnitOK = parseIndicatorTimeUnit(parts[3])
				}
				if timeUnitOK {
					stochSet[sourcePeriodConfig{source: source, period: period, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid stoch key: %s", key)
			}
		case "mfi":
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid mfi key: %s", key)
				}
				continue
			}
			source, sourceOK := parseOHLCVSource(parts[1])
			period, periodOK := parsePositiveInt(parts[2])
			if sourceOK && periodOK {
				mfiSet[sourcePeriodConfig{source: source, period: period}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid mfi key: %s", key)
			}
		case "dmi":
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid dmi key: %s", key)
				}
				continue
			}
			diLength, diOK := parsePositiveInt(parts[1])
			adxSmoothing, adxOK := parsePositiveInt(parts[2])
			if diOK && adxOK {
				dmiSet[dmiConfig{diLength: diLength, adxSmoothing: adxSmoothing}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid dmi key: %s", key)
			}
		case "supertrend":
			if len(parts) == 4 {
				factor, factorErr := strconv.ParseFloat(parts[1], 64)
				atrPeriod, periodOK := parsePositiveInt(parts[2])
				timeUnit, timeUnitOK := parseOptionalAdvancedTimeUnit(parts, 3)
				if factorErr == nil && factor > 0 && periodOK && timeUnitOK && timeUnit != "" {
					advancedSet[advancedIndicatorConfig{key: key, kind: "supertrend", source: "close", period: atrPeriod, multiplier: factor, timeUnit: timeUnit}] = struct{}{}
					continue
				}
			}
			if len(parts) != 3 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid supertrend key: %s", key)
				}
				continue
			}
			factor, factorErr := strconv.ParseFloat(parts[1], 64)
			atrPeriod, periodOK := parsePositiveInt(parts[2])
			if factorErr == nil && factor > 0 && periodOK {
				supertrendSet[supertrendConfig{factor: factor, atrPeriod: atrPeriod}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid supertrend key: %s", key)
			}
		case "sar":
			if len(parts) != 4 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid sar key: %s", key)
				}
				continue
			}
			start, startErr := strconv.ParseFloat(parts[1], 64)
			increment, incrementErr := strconv.ParseFloat(parts[2], 64)
			maximum, maxErr := strconv.ParseFloat(parts[3], 64)
			if startErr == nil && incrementErr == nil && maxErr == nil && start > 0 && increment > 0 && maximum > 0 {
				sarSet[sarConfig{start: start, increment: increment, maximum: maximum}] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid sar key: %s", key)
			}
		case "williamsr":
			if len(parts) != 2 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid williamsr key: %s", key)
				}
				continue
			}
			period, ok := parsePositiveInt(parts[1])
			if ok {
				williamsRSet[period] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid williamsr key: %s", key)
			}
		case "sl", "risk":
			config, ok := parseStopLossConfig(parts)
			if ok {
				stopLossSet[config] = struct{}{}
				continue
			}
			if strict {
				return indicatorRequirements{}, fmt.Errorf("invalid risk key: %s", key)
			}
		case "divergence":
			if len(parts) < 5 {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
				continue
			}
			direction := strings.TrimSpace(parts[len(parts)-2])
			lookback, lookbackOK := parsePositiveInt(parts[len(parts)-1])
			if !lookbackOK || (direction != "top" && direction != "bottom") {
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
				continue
			}
			switch parts[1] {
			case "rsi":
				if len(parts) != 5 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				period, ok := parsePositiveInt(parts[2])
				if ok {
					rsiDivergenceSet[rsiDivergenceConfig{period: period, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			case "macd":
				if len(parts) != 7 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				fast, fastOK := parsePositiveInt(parts[2])
				slow, slowOK := parsePositiveInt(parts[3])
				signal, signalOK := parsePositiveInt(parts[4])
				if fastOK && slowOK && signalOK {
					macdDivergenceSet[macdDivergenceConfig{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			case "kdj":
				if len(parts) != 7 {
					if strict {
						return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
					}
					continue
				}
				period, periodOK := parsePositiveInt(parts[2])
				m1, m1OK := parsePositiveInt(parts[3])
				m2, m2OK := parsePositiveInt(parts[4])
				if periodOK && m1OK && m2OK {
					kdjDivergenceSet[kdjDivergenceConfig{period: period, m1: m1, m2: m2, direction: direction, lookback: lookback}] = struct{}{}
					continue
				}
				if strict {
					return indicatorRequirements{}, fmt.Errorf("invalid divergence key: %s", key)
				}
			default:
				if strict {
					return indicatorRequirements{}, fmt.Errorf("unsupported divergence key: %s", key)
				}
			}
		default:
			if strict {
				return indicatorRequirements{}, fmt.Errorf("unsupported indicator key: %s", key)
			}
		}
	}

	return indicatorRequirements{
		ma:             sortedMovingAverageConfigs(maSet),
		securitySource: sortedSecuritySourceConfigs(securitySourceSet),
		rsi:            sortedInts(rsiSet),
		rsiSource:      sortedSourcePeriodConfigs(rsiSourceSet),
		macd:           sortedMACDConfigs(macdSet),
		bollinger:      sortedBollingerConfigs(bollingerSet),
		kdj:            sortedKDJConfigs(kdjSet),
		atr:            sortedInts(atrSet),
		stdev:          sortedInts(stdevSet),
		stdevSource:    sortedSourcePeriodConfigs(stdevSourceSet),
		variance:       sortedSourcePeriodConfigs(varianceSet),
		windows:        sortedWindowConfigs(windowSet),
		cum:            sortedSourceConfigs(cumSet),
		stoch:          sortedSourcePeriodConfigs(stochSet),
		cci:            sortedInts(cciSet),
		cciSource:      sortedSourcePeriodConfigs(cciSourceSet),
		williamsR:      sortedInts(williamsRSet),
		vwap:           sortedSourceConfigs(vwapSet),
		mfi:            sortedSourcePeriodConfigs(mfiSet),
		dmi:            sortedDMIConfigs(dmiSet),
		supertrend:     sortedSupertrendConfigs(supertrendSet),
		sar:            sortedSARConfigs(sarSet),
		stopLoss:       sortedStopLossConfigs(stopLossSet),
		rsiDivergence:  sortedRSIDivergenceConfigs(rsiDivergenceSet),
		macdDivergence: sortedMACDDivergenceConfigs(macdDivergenceSet),
		kdjDivergence:  sortedKDJDivergenceConfigs(kdjDivergenceSet),
		advanced:       sortedAdvancedIndicatorConfigs(advancedSet),
	}, nil
}

func parseOptionalAdvancedTimeUnit(parts []string, index int) (string, bool) {
	if len(parts) == index {
		return "", true
	}
	if len(parts) != index+1 {
		return "", false
	}
	timeUnit := normalizeIndicatorTimeUnit(parts[index])
	return timeUnit, timeUnit != ""
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed > 0
}

func parseNonNegativeInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed >= 0
}

func parseOHLCVSource(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open":
		return "open", true
	case "high":
		return "high", true
	case "low":
		return "low", true
	case "close":
		return "close", true
	case "volume":
		return "volume", true
	case "hl2":
		return "hl2", true
	case "hlc3":
		return "hlc3", true
	case "ohlc4":
		return "ohlc4", true
	default:
		return "", false
	}
}

func normalizeWindowFunction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "rising", "falling", "sum":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func parseMovingAverageConfig(parts []string) (movingAverageConfig, bool) {
	if len(parts) == 2 {
		period, ok := parsePositiveInt(parts[1])
		if !ok {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{averageType: "MA", period: period}, true
	}
	if len(parts) == 3 {
		if period, ok := parsePositiveInt(parts[1]); ok {
			source, sourceOK := parseOHLCVSource(parts[2])
			if sourceOK {
				return movingAverageConfig{
					averageType: "MA",
					period:      period,
					source:      source,
				}, true
			}
			return movingAverageConfig{
				averageType: "MA",
				period:      period,
				timeUnit:    normalizeIndicatorTimeUnit(parts[2]),
			}, true
		}
		period, ok := parsePositiveInt(parts[2])
		if !ok {
			return movingAverageConfig{}, false
		}
		return movingAverageConfig{
			averageType: normalizeMovingAverageType(parts[1]),
			period:      period,
		}, true
	}
	if len(parts) == 4 {
		period, ok := parsePositiveInt(parts[2])
		if !ok {
			return movingAverageConfig{}, false
		}
		source, sourceOK := parseOHLCVSource(parts[3])
		if sourceOK {
			return movingAverageConfig{
				averageType: normalizeMovingAverageType(parts[1]),
				period:      period,
				source:      source,
			}, true
		}
		return movingAverageConfig{
			averageType: normalizeMovingAverageType(parts[1]),
			period:      period,
			timeUnit:    normalizeIndicatorTimeUnit(parts[3]),
		}, true
	}
	if len(parts) != 5 {
		return movingAverageConfig{}, false
	}
	period, ok := parsePositiveInt(parts[2])
	if !ok {
		return movingAverageConfig{}, false
	}
	source, sourceOK := parseOHLCVSource(parts[4])
	if !sourceOK {
		return movingAverageConfig{}, false
	}
	return movingAverageConfig{
		averageType: normalizeMovingAverageType(parts[1]),
		period:      period,
		timeUnit:    normalizeIndicatorTimeUnit(parts[3]),
		source:      source,
	}, true
}

func normalizeSourceOrClose(value string) string {
	source, ok := parseOHLCVSource(value)
	if !ok || source == "" {
		return "close"
	}
	return source
}

func parseStopLossConfig(parts []string) (stopLossConfig, bool) {
	switch firstNonEmpty(parts[0]) {
	case "sl":
		if len(parts) != 5 {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[2])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         "stopLoss",
			direction:    normalizeStopLossDirection(parts[1]),
			timeValue:    timeValue,
			timeUnit:     normalizeIndicatorTimeUnit(parts[3]),
			percentage:   percentage,
			windowPolicy: "continuous",
		}, true
	case "risk":
		if len(parts) != 7 {
			return stopLossConfig{}, false
		}
		mode, ok := parseStopLossMode(parts[1])
		if !ok {
			return stopLossConfig{}, false
		}
		timeValue, ok := parsePositiveInt(parts[3])
		if !ok {
			return stopLossConfig{}, false
		}
		percentage, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
		if err != nil || percentage <= 0 {
			return stopLossConfig{}, false
		}
		windowPolicy, ok := parseStopLossWindowPolicy(parts[6])
		if !ok {
			return stopLossConfig{}, false
		}
		return stopLossConfig{
			mode:         mode,
			direction:    normalizeStopLossDirection(parts[2]),
			timeValue:    timeValue,
			timeUnit:     normalizeIndicatorTimeUnit(parts[4]),
			percentage:   percentage,
			windowPolicy: windowPolicy,
		}, true
	default:
		return stopLossConfig{}, false
	}
}

func resolveBarCount(period int, timeUnit string, intervalMinutes int) int {
	if period <= 0 {
		return 0
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	if minutes, ok := indicatorTimeUnitMinutes(timeUnit); ok {
		return max(1, int(math.Ceil(float64(period*minutes)/float64(intervalMinutes))))
	}
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "":
		return period
	case "day":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerDay)/float64(intervalMinutes))))
	case "week":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerWeek)/float64(intervalMinutes))))
	case "month":
		return max(1, int(math.Ceil(float64(period*tradingSessionMinutesPerMonth)/float64(intervalMinutes))))
	default:
		return period
	}
}

func indicatorTimeUnitMinutes(timeUnit string) (int, bool) {
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "minute":
		return 1, true
	case "hour":
		return 60, true
	default:
		normalized := normalizeIndicatorTimeUnit(timeUnit)
		if strings.HasSuffix(normalized, "m") {
			minutes, err := strconv.Atoi(strings.TrimSuffix(normalized, "m"))
			if err == nil && minutes > 0 {
				return minutes, true
			}
		}
		return 0, false
	}
}

func normalizeMovingAverageType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "MA"
	}
}

func parseIndicatorTimeUnit(value string) (string, bool) {
	normalized := normalizeIndicatorTimeUnit(value)
	if normalized == "" {
		return "", false
	}
	if _, ok := indicatorTimeUnitMinutes(normalized); ok {
		return normalized, true
	}
	switch normalized {
	case "day", "week", "month":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeIndicatorTimeUnit(value string) string {
	trimmed := strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	normalized := strings.ToLower(strings.TrimSpace(trimmed))
	switch normalized {
	case "", "bar", "bars":
		return ""
	case "m", "min", "mins", "minute", "minutes":
		return "minute"
	case "h", "hr", "hrs", "hour", "hours":
		return "hour"
	case "d", "day", "days":
		return "day"
	case "w", "week", "weeks":
		return "week"
	case "mo", "mon", "month", "months":
		return "month"
	default:
		if strings.HasSuffix(normalized, "m") {
			minutes, err := strconv.Atoi(strings.TrimSuffix(normalized, "m"))
			if err == nil && minutes > 0 {
				switch minutes {
				case 1:
					return "minute"
				case 60:
					return "hour"
				default:
					return strconv.Itoa(minutes) + "m"
				}
			}
		}
		return ""
	}
}

func normalizeStopLossDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "long":
		return "long"
	case "short":
		return "short"
	default:
		return "auto"
	}
}

func normalizeStopLossMode(value string) string {
	switch strings.TrimSpace(value) {
	case "takeProfit":
		return "takeProfit"
	case "trailingStop":
		return "trailingStop"
	default:
		return "stopLoss"
	}
}

func parseStopLossMode(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "stopLoss", "takeProfit", "trailingStop":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}

func normalizeStopLossWindowPolicy(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "session") {
		return "session"
	}
	return "continuous"
}

func parseStopLossWindowPolicy(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "continuous", "session":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
