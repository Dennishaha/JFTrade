package quickjs

func calculateRSIDivergence(closes []float64, config rsiDivergenceConfig) bool {
	series := calculateRSISeries(closes, config.period)
	alignedCloses, alignedSeries := alignSeries(closes, series)
	return detectDivergence(alignedCloses, alignedSeries, config.direction, config.lookback)
}

func calculateMACDDivergence(closes []float64, config macdDivergenceConfig) bool {
	minimum := max(config.fastPeriod, config.slowPeriod) + config.signalPeriod
	if minimum <= 0 || len(closes) < minimum {
		return false
	}
	fastSequence := calculateEMASequence(closes, config.fastPeriod)
	slowSequence := calculateEMASequence(closes, config.slowPeriod)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return false
	}
	diffSequence := make([]float64, len(closes))
	for index := range closes {
		diffSequence[index] = fastSequence[index] - slowSequence[index]
	}
	return detectDivergence(closes, diffSequence, config.direction, config.lookback)
}

func calculateKDJDivergence(highs, lows, closes []float64, config kdjDivergenceConfig) bool {
	_, _, jValues := calculateKDJSeries(highs, lows, closes, kdjConfig{period: config.period, m1: config.m1, m2: config.m2})
	return detectDivergence(closes, jValues, config.direction, config.lookback)
}

func alignSeries(prices, indicator []float64) ([]float64, []float64) {
	if len(prices) == 0 || len(indicator) == 0 {
		return nil, nil
	}
	if len(indicator) >= len(prices) {
		return prices, indicator[len(indicator)-len(prices):]
	}
	return prices[len(prices)-len(indicator):], indicator
}

func detectDivergence(prices, indicator []float64, direction string, lookback int) bool {
	if lookback <= 0 || len(prices) < lookback+1 || len(indicator) < lookback+1 {
		return false
	}
	start := len(prices) - lookback - 1
	currentPrice := prices[len(prices)-1]
	currentIndicator := indicator[len(indicator)-1]
	previousPrices := prices[start : len(prices)-1]
	previousIndicator := indicator[start : len(indicator)-1]
	if len(previousPrices) == 0 || len(previousIndicator) == 0 {
		return false
	}
	switch direction {
	case "top":
		return currentPrice > maxSlice(previousPrices) && currentIndicator < maxSlice(previousIndicator)
	case "bottom":
		return currentPrice < minSlice(previousPrices) && currentIndicator > minSlice(previousIndicator)
	default:
		return false
	}
}

func maxSlice(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = maxFloat(result, value)
	}
	return result
}

func minSlice(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = minFloat(result, value)
	}
	return result
}
