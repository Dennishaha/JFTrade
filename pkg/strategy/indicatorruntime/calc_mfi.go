package indicatorruntime

func calculateMFIValue(values, volumes []float64, period int) (float64, bool) {
	if period <= 0 || len(values) <= period || len(volumes) != len(values) {
		return 0, false
	}
	start := len(values) - period
	positiveFlow := 0.0
	negativeFlow := 0.0
	for index := start; index < len(values); index++ {
		flow := values[index] * volumes[index]
		switch {
		case values[index] > values[index-1]:
			positiveFlow += flow
		case values[index] < values[index-1]:
			negativeFlow += flow
		}
	}
	if negativeFlow == 0 {
		return 100, true
	}
	ratio := positiveFlow / negativeFlow
	return 100 - 100/(1+ratio), true
}
