package indicatorruntime

import "math"

func calculateCCI(highs, lows, closes []float64, period int) any {
	values := calculateCCISeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateCCISeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	typicalPrices := make([]float64, len(closes))
	for index := range closes {
		typicalPrices[index] = (highs[index] + lows[index] + closes[index]) / 3
	}
	result := make([]float64, 0, len(closes)-period+1)
	rollingSum := 0.0
	for index := period - 1; index < len(typicalPrices); index++ {
		if index == period-1 {
			for cursor := 0; cursor < period; cursor++ {
				rollingSum += typicalPrices[cursor]
			}
		} else {
			rollingSum += typicalPrices[index] - typicalPrices[index-period]
		}
		average := rollingSum / float64(period)
		meanDeviation := 0.0
		for cursor := index - period + 1; cursor <= index; cursor++ {
			meanDeviation += math.Abs(typicalPrices[cursor] - average)
		}
		meanDeviation /= float64(period)
		if meanDeviation == 0 {
			result = append(result, 0)
			continue
		}
		result = append(result, (typicalPrices[index]-average)/(0.015*meanDeviation))
	}
	return result
}

func calculateCCIFromValues(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	mean, ok := simpleMovingAverage(window, period)
	if !ok {
		return 0, false
	}
	meanDeviation := 0.0
	for _, value := range window {
		meanDeviation += math.Abs(value - mean)
	}
	meanDeviation /= float64(period)
	if meanDeviation == 0 {
		return 0, true
	}
	return (values[len(values)-1] - mean) / (0.015 * meanDeviation), true
}
