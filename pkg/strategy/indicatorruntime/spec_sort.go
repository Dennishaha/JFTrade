package indicatorruntime

import "sort"

func sortedAdvancedIndicatorConfigs(values map[advancedIndicatorConfig]struct{}) []advancedIndicatorConfig {
	result := make([]advancedIndicatorConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].key < result[right].key
	})
	return result
}

func sortedWindowConfigs(values map[windowConfig]struct{}) []windowConfig {
	result := make([]windowConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].function != result[right].function {
			return result[left].function < result[right].function
		}
		if result[left].source != result[right].source {
			return result[left].source < result[right].source
		}
		return result[left].period < result[right].period
	})
	return result
}

func sortedSourceConfigs(values map[sourceConfig]struct{}) []sourceConfig {
	result := make([]sourceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		return normalizeSourceOrClose(result[left].source) < normalizeSourceOrClose(result[right].source)
	})
	return result
}

func sortedSourcePeriodConfigs(values map[sourcePeriodConfig]struct{}) []sourcePeriodConfig {
	result := make([]sourcePeriodConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].timeUnit != result[right].timeUnit {
			return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
		}
		return normalizeSourceOrClose(result[left].source) < normalizeSourceOrClose(result[right].source)
	})
	return result
}

func sortedDMIConfigs(values map[dmiConfig]struct{}) []dmiConfig {
	result := make([]dmiConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].diLength != result[right].diLength {
			return result[left].diLength < result[right].diLength
		}
		return result[left].adxSmoothing < result[right].adxSmoothing
	})
	return result
}

func sortedSupertrendConfigs(values map[supertrendConfig]struct{}) []supertrendConfig {
	result := make([]supertrendConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].atrPeriod != result[right].atrPeriod {
			return result[left].atrPeriod < result[right].atrPeriod
		}
		return result[left].factor < result[right].factor
	})
	return result
}

func sortedSARConfigs(values map[sarConfig]struct{}) []sarConfig {
	result := make([]sarConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].start != result[right].start {
			return result[left].start < result[right].start
		}
		if result[left].increment != result[right].increment {
			return result[left].increment < result[right].increment
		}
		return result[left].maximum < result[right].maximum
	})
	return result
}

func sortedMovingAverageConfigs(values map[movingAverageConfig]struct{}) []movingAverageConfig {
	result := make([]movingAverageConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].averageType != result[right].averageType {
			return result[left].averageType < result[right].averageType
		}
		if normalizeIndicatorTimeUnit(result[left].timeUnit) != normalizeIndicatorTimeUnit(result[right].timeUnit) {
			return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
		}
		return normalizeSourceOrClose(result[left].source) < normalizeSourceOrClose(result[right].source)
	})
	return result
}

func sortedSecuritySourceConfigs(values map[securitySourceConfig]struct{}) []securitySourceConfig {
	result := make([]securitySourceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if normalizeIndicatorTimeUnit(result[left].timeUnit) != normalizeIndicatorTimeUnit(result[right].timeUnit) {
			return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
		}
		return normalizeSourceOrClose(result[left].source) < normalizeSourceOrClose(result[right].source)
	})
	return result
}

func sortedStopLossConfigs(values map[stopLossConfig]struct{}) []stopLossConfig {
	result := make([]stopLossConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if normalizeStopLossMode(result[left].mode) != normalizeStopLossMode(result[right].mode) {
			return normalizeStopLossMode(result[left].mode) < normalizeStopLossMode(result[right].mode)
		}
		if result[left].timeValue != result[right].timeValue {
			return result[left].timeValue < result[right].timeValue
		}
		if normalizeIndicatorTimeUnit(result[left].timeUnit) != normalizeIndicatorTimeUnit(result[right].timeUnit) {
			return normalizeIndicatorTimeUnit(result[left].timeUnit) < normalizeIndicatorTimeUnit(result[right].timeUnit)
		}
		if result[left].percentage != result[right].percentage {
			return result[left].percentage < result[right].percentage
		}
		if normalizeStopLossWindowPolicy(result[left].windowPolicy) != normalizeStopLossWindowPolicy(result[right].windowPolicy) {
			return normalizeStopLossWindowPolicy(result[left].windowPolicy) < normalizeStopLossWindowPolicy(result[right].windowPolicy)
		}
		return normalizeStopLossDirection(result[left].direction) < normalizeStopLossDirection(result[right].direction)
	})
	return result
}

func sortedInts(values map[int]struct{}) []int {
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func sortedMACDConfigs(values map[macdConfig]struct{}) []macdConfig {
	result := make([]macdConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].fastPeriod != result[right].fastPeriod {
			return result[left].fastPeriod < result[right].fastPeriod
		}
		if result[left].slowPeriod != result[right].slowPeriod {
			return result[left].slowPeriod < result[right].slowPeriod
		}
		return result[left].signalPeriod < result[right].signalPeriod
	})
	return result
}

func sortedBollingerConfigs(values map[bollingerConfig]struct{}) []bollingerConfig {
	result := make([]bollingerConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		return result[left].multiplier < result[right].multiplier
	})
	return result
}

func sortedKDJConfigs(values map[kdjConfig]struct{}) []kdjConfig {
	result := make([]kdjConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].m1 != result[right].m1 {
			return result[left].m1 < result[right].m1
		}
		return result[left].m2 < result[right].m2
	})
	return result
}

func sortedRSIDivergenceConfigs(values map[rsiDivergenceConfig]struct{}) []rsiDivergenceConfig {
	result := make([]rsiDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}

func sortedMACDDivergenceConfigs(values map[macdDivergenceConfig]struct{}) []macdDivergenceConfig {
	result := make([]macdDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].fastPeriod != result[right].fastPeriod {
			return result[left].fastPeriod < result[right].fastPeriod
		}
		if result[left].slowPeriod != result[right].slowPeriod {
			return result[left].slowPeriod < result[right].slowPeriod
		}
		if result[left].signalPeriod != result[right].signalPeriod {
			return result[left].signalPeriod < result[right].signalPeriod
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}

func sortedKDJDivergenceConfigs(values map[kdjDivergenceConfig]struct{}) []kdjDivergenceConfig {
	result := make([]kdjDivergenceConfig, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].period != result[right].period {
			return result[left].period < result[right].period
		}
		if result[left].m1 != result[right].m1 {
			return result[left].m1 < result[right].m1
		}
		if result[left].m2 != result[right].m2 {
			return result[left].m2 < result[right].m2
		}
		if result[left].direction != result[right].direction {
			return result[left].direction < result[right].direction
		}
		return result[left].lookback < result[right].lookback
	})
	return result
}
