package indicatorruntime

import "math"

func calculateSARSnapshotValues(highs, lows, closes []float64, config sarConfig) (float64, float64, bool, bool) {
	values := calculateSARSeries(highs, lows, closes, config)
	if len(values) == 0 {
		return 0, 0, false, false
	}
	current := values[len(values)-1]
	if len(values) == 1 {
		return current, 0, true, false
	}
	return current, values[len(values)-2], true, true
}

func calculateSARSeries(highs, lows, closes []float64, config sarConfig) []float64 {
	if config.start <= 0 || config.increment <= 0 || config.maximum <= 0 || len(highs) < 2 || len(lows) != len(highs) || len(closes) != len(highs) {
		return nil
	}
	result := make([]float64, 0, len(highs)-1)
	longTrend := closes[1] >= closes[0]
	if closes[1] == closes[0] {
		longTrend = highs[1]+lows[1] >= highs[0]+lows[0]
	}
	sar := lows[0]
	extremePoint := highs[1]
	if !longTrend {
		sar = highs[0]
		extremePoint = lows[1]
	}
	acceleration := config.start
	for index := 1; index < len(highs); index++ {
		sar = sar + acceleration*(extremePoint-sar)
		if longTrend {
			sar = math.Min(sar, lows[index-1])
			if index >= 2 {
				sar = math.Min(sar, lows[index-2])
			}
			if lows[index] < sar {
				longTrend = false
				sar = extremePoint
				extremePoint = lows[index]
				acceleration = config.start
			} else if highs[index] > extremePoint {
				extremePoint = highs[index]
				acceleration = math.Min(acceleration+config.increment, config.maximum)
			}
		} else {
			sar = math.Max(sar, highs[index-1])
			if index >= 2 {
				sar = math.Max(sar, highs[index-2])
			}
			if highs[index] > sar {
				longTrend = true
				sar = extremePoint
				extremePoint = highs[index]
				acceleration = config.start
			} else if lows[index] < extremePoint {
				extremePoint = lows[index]
				acceleration = math.Min(acceleration+config.increment, config.maximum)
			}
		}
		result = append(result, sar)
	}
	return result
}
