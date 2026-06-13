package indicatorruntime

func calculateWilliamsR(highs, lows, closes []float64, period int) any {
	values := calculateWilliamsRSeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateWilliamsRSeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	highestHighs, lowestLows := calculateRollingHighLow(highs, lows, period)
	if len(highestHighs) == 0 || len(lowestLows) == 0 {
		return nil
	}
	result := make([]float64, 0, len(closes)-period+1)
	for index := period - 1; index < len(closes); index++ {
		highestHigh := highestHighs[index]
		lowestLow := lowestLows[index]
		if highestHigh == lowestLow {
			result = append(result, -50)
			continue
		}
		result = append(result, -100*(highestHigh-closes[index])/(highestHigh-lowestLow))
	}
	return result
}
