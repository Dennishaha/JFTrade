package indicatorruntime

func calculateKDJSnapshot(highs, lows, closes []float64, config kdjConfig) map[string]any {
	if config.period <= 0 || len(closes) == 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	return calculateKDJSnapshotFromSeries(kdjSeriesFromSlices(calculateKDJSeries(highs, lows, closes, config)))
}

func calculateKDJRSV(highestHigh, lowestLow, closeValue float64) float64 {
	if highestHigh == lowestLow {
		return 50
	}
	return ((closeValue - lowestLow) / (highestHigh - lowestLow)) * 100
}

func kdjSeriesFromSlices(kValues, dValues, jValues []float64) kdjSeries {
	return kdjSeries{k: kValues, d: dValues, j: jValues}
}

func calculateKDJSnapshotFromSeries(series kdjSeries) map[string]any {
	if len(series.k) == 0 || len(series.d) == 0 || len(series.j) == 0 {
		return nil
	}
	last := len(series.k) - 1
	result := map[string]any{
		"k": series.k[last],
		"d": series.d[last],
		"j": series.j[last],
	}
	if last > 0 {
		result["previousK"] = series.k[last-1]
		result["previousD"] = series.d[last-1]
		result["previousJ"] = series.j[last-1]
	}
	return result
}

func calculateKDJSeries(highs, lows, closes []float64, config kdjConfig) ([]float64, []float64, []float64) {
	series := calculateKDJSeriesWithBuffer(nil, highs, lows, closes, config)
	return series.k, series.d, series.j
}

func calculateKDJSeriesWithBuffer(buffer *reusableKDJSeriesBuffer, highs, lows, closes []float64, config kdjConfig) kdjSeries {
	if config.period <= 0 || len(closes) == 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return kdjSeries{}
	}
	if buffer == nil {
		buffer = &reusableKDJSeriesBuffer{}
	}
	buffer.series.k = reuseFloat64Slice(buffer.series.k, len(closes))
	buffer.series.d = reuseFloat64Slice(buffer.series.d, len(closes))
	buffer.series.j = reuseFloat64Slice(buffer.series.j, len(closes))
	dequeCapacity := min(len(closes), max(config.period, 1))
	buffer.highDeque.reset(dequeCapacity)
	buffer.lowDeque.reset(dequeCapacity)
	previousK := 50.0
	previousD := 50.0
	for index := range closes {
		windowStart := max(0, index-config.period+1)
		buffer.highDeque.popExpired(windowStart)
		buffer.lowDeque.popExpired(windowStart)
		buffer.highDeque.pushMax(highs, index)
		buffer.lowDeque.pushMin(lows, index)
		highestHigh := highs[buffer.highDeque.front()]
		lowestLow := lows[buffer.lowDeque.front()]
		rsv := calculateKDJRSV(highestHigh, lowestLow, closes[index])
		nextK := ((float64(config.m1)-1)*previousK + rsv) / float64(config.m1)
		nextD := ((float64(config.m2)-1)*previousD + nextK) / float64(config.m2)
		nextJ := 3*nextK - 2*nextD
		buffer.series.k[index] = nextK
		buffer.series.d[index] = nextD
		buffer.series.j[index] = nextJ
		previousK = nextK
		previousD = nextD
	}
	return buffer.series
}
