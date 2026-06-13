package indicatorruntime

import "math"

func calculateDMISnapshot(highs, lows, closes []float64, config dmiConfig) map[string]any {
	plusDI, minusDI, adx, ok := calculateDMIValues(highs, lows, closes, config)
	if !ok {
		return nil
	}
	return map[string]any{"plus": plusDI, "minus": minusDI, "adx": adx}
}

func calculateDMIValues(highs, lows, closes []float64, config dmiConfig) (float64, float64, float64, bool) {
	if config.diLength <= 0 || config.adxSmoothing <= 0 || len(closes) <= config.diLength+config.adxSmoothing || len(highs) != len(closes) || len(lows) != len(closes) {
		return 0, 0, 0, false
	}
	tr := make([]float64, 0, len(closes)-1)
	plusDM := make([]float64, 0, len(closes)-1)
	minusDM := make([]float64, 0, len(closes)-1)
	for index := 1; index < len(closes); index++ {
		upMove := highs[index] - highs[index-1]
		downMove := lows[index-1] - lows[index]
		if upMove > downMove && upMove > 0 {
			plusDM = append(plusDM, upMove)
		} else {
			plusDM = append(plusDM, 0)
		}
		if downMove > upMove && downMove > 0 {
			minusDM = append(minusDM, downMove)
		} else {
			minusDM = append(minusDM, 0)
		}
		tr = append(tr, trueRange(highs[index], lows[index], closes[index-1]))
	}
	smoothedTR := calculateRMASequence(tr, config.diLength)
	smoothedPlus := calculateRMASequence(plusDM, config.diLength)
	smoothedMinus := calculateRMASequence(minusDM, config.diLength)
	if len(smoothedTR) == 0 || len(smoothedPlus) != len(smoothedTR) || len(smoothedMinus) != len(smoothedTR) {
		return 0, 0, 0, false
	}
	plusDISeq := make([]float64, len(smoothedTR))
	minusDISeq := make([]float64, len(smoothedTR))
	dxSeq := make([]float64, len(smoothedTR))
	for index := range smoothedTR {
		if smoothedTR[index] == 0 {
			continue
		}
		plusDISeq[index] = 100 * smoothedPlus[index] / smoothedTR[index]
		minusDISeq[index] = 100 * smoothedMinus[index] / smoothedTR[index]
		sum := plusDISeq[index] + minusDISeq[index]
		if sum > 0 {
			dxSeq[index] = 100 * math.Abs(plusDISeq[index]-minusDISeq[index]) / sum
		}
	}
	adxSeq := calculateRMASequence(dxSeq, config.adxSmoothing)
	if len(adxSeq) == 0 {
		return 0, 0, 0, false
	}
	return plusDISeq[len(plusDISeq)-1], minusDISeq[len(minusDISeq)-1], adxSeq[len(adxSeq)-1], true
}
