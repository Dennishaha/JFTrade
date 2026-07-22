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
	averageGains := calculateRMASequence(gains, period)
	averageLosses := calculateRMASequence(losses, period)
	result := make([]float64, 0, len(averageGains))
	for index, averageGain := range averageGains {
		result = append(result, rsiFromWilderAverages(averageGain, averageLosses[index]))
	}
	return result
}

func rsiFromWilderAverages(averageGain, averageLoss float64) float64 {
	if averageLoss == 0 {
		return 100
	}
	relativeStrength := averageGain / averageLoss
	return 100 - 100/(1+relativeStrength)
}
