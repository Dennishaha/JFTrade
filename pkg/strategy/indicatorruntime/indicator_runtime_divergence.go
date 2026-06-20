package indicatorruntime

func detectRSIDivergence(closes, series []float64, direction string, lookback int) bool {
	alignedCloses, alignedSeries := alignSeries(closes, series)
	return detectDivergence(alignedCloses, alignedSeries, direction, lookback)
}

func detectMACDDivergence(closes, diffSequence []float64, direction string, lookback int) bool {
	return detectDivergence(closes, diffSequence, direction, lookback)
}

func detectKDJDivergence(closes, jValues []float64, direction string, lookback int) bool {
	alignedCloses, alignedSeries := alignSeries(closes, jValues)
	return detectDivergence(alignedCloses, alignedSeries, direction, lookback)
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
