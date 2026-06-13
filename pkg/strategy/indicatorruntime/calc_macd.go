package indicatorruntime

func calculateMACDSnapshot(values []float64, config macdConfig) map[string]any {
	return calculateMACDSnapshotFromSeries(calculateMACDSeries(values, config))
}

func calculateMACDSeries(values []float64, config macdConfig) macdSeries {
	return calculateMACDSeriesWithCache(values, config, nil)
}

func calculateMACDSeriesWithCache(values []float64, config macdConfig, cache *snapshotSeriesCache) macdSeries {
	minimum := max(config.fastPeriod, config.slowPeriod) + config.signalPeriod
	if minimum <= 0 || len(values) < minimum {
		return macdSeries{}
	}
	fastSequence := cache.getEMASequence(values, config.fastPeriod)
	slowSequence := cache.getEMASequence(values, config.slowPeriod)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return macdSeries{}
	}
	buffer := macdSeries{}
	if cache != nil {
		buffer = cache.macdBuffers[config]
	}
	diffSequence := reuseFloat64Slice(buffer.diff, len(values))
	signalSequence := reuseFloat64Slice(buffer.signal, len(values))
	signalMultiplier := 2 / float64(config.signalPeriod+1)
	for index := range values {
		diff := fastSequence[index] - slowSequence[index]
		diffSequence[index] = diff
		if index == 0 {
			signalSequence[index] = diff
			continue
		}
		signalSequence[index] = signalSequence[index-1] + (diff-signalSequence[index-1])*signalMultiplier
	}
	return macdSeries{diff: diffSequence, signal: signalSequence}
}

func calculateMACDSnapshotFromSeries(series macdSeries) map[string]any {
	if len(series.diff) == 0 || len(series.signal) == 0 {
		return nil
	}
	currentIndex := len(series.diff) - 1
	result := map[string]any{
		"diff":      series.diff[currentIndex],
		"signal":    series.signal[currentIndex],
		"histogram": (series.diff[currentIndex] - series.signal[currentIndex]) * 2,
	}
	if currentIndex > 0 {
		previousIndex := currentIndex - 1
		result["previousDiff"] = series.diff[previousIndex]
		result["previousSignal"] = series.signal[previousIndex]
		result["previousHistogram"] = (series.diff[previousIndex] - series.signal[previousIndex]) * 2
	}
	return result
}
