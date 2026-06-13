package indicatorruntime

import "math"

func calculateStdDev(values []float64, period int) (float64, bool) {
	return calculateStdDevValue(values, period)
}

func calculateStdDevValue(values []float64, period int) (float64, bool) {
	variance, ok := calculateVarianceValue(values, period)
	if !ok {
		return 0, false
	}
	return math.Sqrt(variance), true
}

func calculateVarianceValue(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	windowValues := values[len(values)-period:]
	middle, ok := simpleMovingAverage(windowValues, period)
	if !ok {
		return 0, false
	}
	variance := 0.0
	for _, value := range windowValues {
		delta := value - middle
		variance += delta * delta
	}
	return variance / float64(period), true
}
