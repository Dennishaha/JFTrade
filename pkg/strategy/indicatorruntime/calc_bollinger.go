package indicatorruntime

import "math"

func calculateBollingerSnapshot(values []float64, config bollingerConfig) map[string]any {
	if config.period <= 0 || len(values) < config.period {
		return nil
	}
	windowValues := values[len(values)-config.period:]
	middle, ok := simpleMovingAverage(windowValues, config.period)
	if !ok {
		return nil
	}
	variance := 0.0
	for _, value := range windowValues {
		delta := value - middle
		variance += delta * delta
	}
	standardDeviation := math.Sqrt(variance / float64(len(windowValues)))
	return map[string]any{
		"middle": middle,
		"upper":  middle + standardDeviation*config.multiplier,
		"lower":  middle - standardDeviation*config.multiplier,
	}
}
