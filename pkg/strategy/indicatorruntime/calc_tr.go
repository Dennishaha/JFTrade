package indicatorruntime

import "math"

func trueRange(high, low, previousClose float64) float64 {
	return math.Max(high-low, math.Max(math.Abs(high-previousClose), math.Abs(low-previousClose)))
}

func calculateRollingHighLow(highs, lows []float64, period int) ([]float64, []float64) {
	if period <= 0 || len(highs) == 0 || len(highs) != len(lows) {
		return nil, nil
	}
	highestHighs := make([]float64, len(highs))
	lowestLows := make([]float64, len(lows))
	var highDeque, lowDeque indexDeque
	for index := range highs {
		windowStart := max(0, index-period+1)
		highDeque.popExpired(windowStart)
		lowDeque.popExpired(windowStart)
		highDeque.pushMax(highs, index)
		lowDeque.pushMin(lows, index)
		highestHighs[index] = highs[highDeque.front()]
		lowestLows[index] = lows[lowDeque.front()]
	}
	return highestHighs, lowestLows
}
