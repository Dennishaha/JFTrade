package indicatorruntime

func calculateSupertrendSnapshot(highs, lows, closes []float64, config supertrendConfig) map[string]any {
	line, direction, ok := calculateSupertrendValues(highs, lows, closes, config)
	if !ok {
		return nil
	}
	return map[string]any{"line": line, "direction": direction}
}

func calculateSupertrendValues(highs, lows, closes []float64, config supertrendConfig) (float64, float64, bool) {
	if config.factor <= 0 || config.atrPeriod <= 0 || len(closes) <= config.atrPeriod || len(highs) != len(closes) || len(lows) != len(closes) {
		return 0, 0, false
	}
	atr := calculateATRSeries(highs, lows, closes, config.atrPeriod)
	if len(atr) == 0 {
		return 0, 0, false
	}
	offset := len(closes) - len(atr)
	finalUpper := 0.0
	finalLower := 0.0
	supertrend := 0.0
	direction := 1.0
	for atrIndex, atrValue := range atr {
		index := atrIndex + offset
		hl2 := (highs[index] + lows[index]) / 2
		basicUpper := hl2 + config.factor*atrValue
		basicLower := hl2 - config.factor*atrValue
		if atrIndex == 0 {
			finalUpper = basicUpper
			finalLower = basicLower
			supertrend = finalLower
			direction = 1
			continue
		}
		previousClose := closes[index-1]
		if basicUpper < finalUpper || previousClose > finalUpper {
			finalUpper = basicUpper
		}
		if basicLower > finalLower || previousClose < finalLower {
			finalLower = basicLower
		}
		if supertrend == finalUpper {
			if closes[index] <= finalUpper {
				supertrend = finalUpper
				direction = -1
			} else {
				supertrend = finalLower
				direction = 1
			}
		} else if closes[index] >= finalLower {
			supertrend = finalLower
			direction = 1
		} else {
			supertrend = finalUpper
			direction = -1
		}
	}
	return supertrend, direction, true
}
