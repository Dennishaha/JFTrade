package indicatorruntime

import "math"

func calculateRSI(values []float64, period int) any {
	series := calculateRSISeries(values, period)
	return calculateRSIFromSeries(series)
}

func calculateRSIFromSeries(series []float64) any {
	value, ok := calculateRSIValueFromSeries(series)
	if !ok {
		return nil
	}
	return value
}

func calculateRSIValueFromSeries(series []float64) (float64, bool) {
	if len(series) == 0 {
		return 0, false
	}
	return series[len(series)-1], true
}

func calculateRSISeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) <= period {
		return nil
	}
	gains := make([]float64, len(values)-1)
	losses := make([]float64, len(values)-1)
	for index := 1; index < len(values); index++ {
		delta := values[index] - values[index-1]
		if delta >= 0 {
			gains[index-1] = delta
			continue
		}
		losses[index-1] = math.Abs(delta)
	}
	result := make([]float64, 0, len(values)-period)
	rollingGains := 0.0
	rollingLosses := 0.0
	for index := range period {
		rollingGains += gains[index]
		rollingLosses += losses[index]
	}
	appendRSIValue := func(totalGains, totalLosses float64) {
		if totalLosses == 0 {
			result = append(result, 100.0)
			return
		}
		relativeStrength := totalGains / totalLosses
		result = append(result, 100-100/(1+relativeStrength))
	}
	appendRSIValue(rollingGains, rollingLosses)
	for index := period; index < len(gains); index++ {
		rollingGains += gains[index] - gains[index-period]
		rollingLosses += losses[index] - losses[index-period]
		appendRSIValue(rollingGains, rollingLosses)
	}
	return result
}
