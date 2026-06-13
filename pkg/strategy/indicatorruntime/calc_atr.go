package indicatorruntime

import "math"

func calculateATR(highs, lows, closes []float64, period int) any {
	values := calculateATRSeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateATRSeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	trueRanges := make([]float64, len(closes))
	for index := range closes {
		if index == 0 {
			trueRanges[index] = highs[index] - lows[index]
			continue
		}
		trueRanges[index] = maxFloat(
			highs[index]-lows[index],
			maxFloat(math.Abs(highs[index]-closes[index-1]), math.Abs(lows[index]-closes[index-1])),
		)
	}
	result := make([]float64, 0, len(closes)-period+1)
	windowSum := 0.0
	for index, trueRange := range trueRanges {
		windowSum += trueRange
		if index >= period {
			windowSum -= trueRanges[index-period]
		}
		if index >= period-1 {
			result = append(result, windowSum/float64(period))
		}
	}
	return result
}
