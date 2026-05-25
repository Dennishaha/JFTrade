package quickjs

import (
	"math"

	"github.com/c9s/bbgo/pkg/types"
)

const indicatorSeriesLimit = 256

type indicatorRuntime struct {
	requirements indicatorRequirements
	highs        []float64
	lows         []float64
	closes       []float64
}

func newIndicatorRuntime(script string) *indicatorRuntime {
	requirements := parseIndicatorRequirements(script)
	if requirements.isEmpty() {
		return nil
	}
	return &indicatorRuntime{requirements: requirements}
}

func (r *indicatorRuntime) push(kline types.KLine) {
	if r == nil {
		return
	}
	r.highs = append(r.highs, kline.High.Float64())
	r.lows = append(r.lows, kline.Low.Float64())
	r.closes = append(r.closes, kline.Close.Float64())
	if len(r.closes) > indicatorSeriesLimit {
		start := len(r.closes) - indicatorSeriesLimit
		r.highs = append([]float64(nil), r.highs[start:]...)
		r.lows = append([]float64(nil), r.lows[start:]...)
		r.closes = append([]float64(nil), r.closes[start:]...)
	}
}

func (r *indicatorRuntime) snapshot() map[string]any {
	if r == nil {
		return nil
	}
	result := map[string]any{}
	for _, period := range r.requirements.ma {
		result[maIndicatorKey(period)] = buildMovingAverageSnapshot(r.closes, period)
	}
	for _, period := range r.requirements.rsi {
		result[rsiIndicatorKey(period)] = calculateRSI(r.closes, period)
	}
	for _, config := range r.requirements.macd {
		result[macdIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod)] = calculateMACDSnapshot(r.closes, config)
	}
	for _, config := range r.requirements.bollinger {
		result[bollingerIndicatorKey(config.period, config.multiplier)] = calculateBollingerSnapshot(r.closes, config)
	}
	for _, config := range r.requirements.kdj {
		result[kdjIndicatorKey(config.period, config.m1, config.m2)] = calculateKDJSnapshot(r.highs, r.lows, r.closes, config)
	}
	for _, period := range r.requirements.atr {
		result[atrIndicatorKey(period)] = calculateATR(r.highs, r.lows, r.closes, period)
	}
	for _, period := range r.requirements.cci {
		result[cciIndicatorKey(period)] = calculateCCI(r.highs, r.lows, r.closes, period)
	}
	for _, period := range r.requirements.williamsR {
		result[williamsRIndicatorKey(period)] = calculateWilliamsR(r.highs, r.lows, r.closes, period)
	}
	for _, config := range r.requirements.rsiDivergence {
		result[rsiDivergenceIndicatorKey(config.period, config.direction, config.lookback)] = calculateRSIDivergence(r.closes, config)
	}
	for _, config := range r.requirements.macdDivergence {
		result[macdDivergenceIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod, config.direction, config.lookback)] = calculateMACDDivergence(r.closes, config)
	}
	for _, config := range r.requirements.kdjDivergence {
		result[kdjDivergenceIndicatorKey(config.period, config.m1, config.m2, config.direction, config.lookback)] = calculateKDJDivergence(r.highs, r.lows, r.closes, config)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func buildMovingAverageSnapshot(values []float64, period int) map[string]any {
	current, currentOK := simpleMovingAverage(values, period)
	previous, previousOK := simpleMovingAverage(values[:max(len(values)-1, 0)], period)
	if !currentOK && !previousOK {
		return nil
	}
	result := map[string]any{"value": nil, "previous": nil}
	if currentOK {
		result["value"] = current
	}
	if previousOK {
		result["previous"] = previous
	}
	return result
}

func simpleMovingAverage(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	sum := 0.0
	for _, value := range values[len(values)-period:] {
		sum += value
	}
	return sum / float64(period), true
}

func calculateRSI(values []float64, period int) any {
	series := calculateRSISeries(values, period)
	if len(series) == 0 {
		return nil
	}
	return series[len(series)-1]
}

func calculateRSISeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) <= period {
		return nil
	}
	result := make([]float64, 0, len(values)-period)
	for end := period; end < len(values); end++ {
		gains := 0.0
		losses := 0.0
		for index := end - period + 1; index <= end; index++ {
			delta := values[index] - values[index-1]
			if delta >= 0 {
				gains += delta
			} else {
				losses += math.Abs(delta)
			}
		}
		if losses == 0 {
			result = append(result, 100.0)
			continue
		}
		relativeStrength := gains / losses
		result = append(result, 100-100/(1+relativeStrength))
	}
	return result
}

func calculateMACDSnapshot(values []float64, config macdConfig) map[string]any {
	minimum := max(config.fastPeriod, config.slowPeriod) + config.signalPeriod
	if minimum <= 0 || len(values) < minimum {
		return nil
	}
	fastSequence := calculateEMASequence(values, config.fastPeriod)
	slowSequence := calculateEMASequence(values, config.slowPeriod)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return nil
	}
	diffSequence := make([]float64, 0, len(values))
	for index := range values {
		diffSequence = append(diffSequence, fastSequence[index]-slowSequence[index])
	}
	signalSequence := calculateEMASequence(diffSequence, config.signalPeriod)
	if len(signalSequence) == 0 {
		return nil
	}
	currentIndex := len(diffSequence) - 1
	result := map[string]any{
		"diff":      diffSequence[currentIndex],
		"signal":    signalSequence[currentIndex],
		"histogram": (diffSequence[currentIndex] - signalSequence[currentIndex]) * 2,
	}
	if currentIndex > 0 {
		previousIndex := currentIndex - 1
		result["previousDiff"] = diffSequence[previousIndex]
		result["previousSignal"] = signalSequence[previousIndex]
		result["previousHistogram"] = (diffSequence[previousIndex] - signalSequence[previousIndex]) * 2
	}
	return result
}

func calculateEMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	multiplier := 2 / float64(period+1)
	sequence := make([]float64, 0, len(values))
	previous := values[0]
	for index, value := range values {
		if index == 0 {
			sequence = append(sequence, value)
			continue
		}
		previous = previous + (value-previous)*multiplier
		sequence = append(sequence, previous)
	}
	return sequence
}

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

func calculateKDJSnapshot(highs, lows, closes []float64, config kdjConfig) map[string]any {
	if config.period <= 0 || len(closes) == 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	kValues, dValues, jValues := calculateKDJSeries(highs, lows, closes, config)
	if len(kValues) == 0 {
		return nil
	}
	last := len(kValues) - 1
	result := map[string]any{
		"k": kValues[last],
		"d": dValues[last],
		"j": jValues[last],
	}
	if last > 0 {
		result["previousK"] = kValues[last-1]
		result["previousD"] = dValues[last-1]
		result["previousJ"] = jValues[last-1]
	}
	return result
}

func calculateKDJSeries(highs, lows, closes []float64, config kdjConfig) ([]float64, []float64, []float64) {
	kValues := make([]float64, 0, len(closes))
	dValues := make([]float64, 0, len(closes))
	jValues := make([]float64, 0, len(closes))
	previousK := 50.0
	previousD := 50.0
	for index := range closes {
		start := max(0, index-config.period+1)
		highestHigh := highs[start]
		lowestLow := lows[start]
		for cursor := start + 1; cursor <= index; cursor++ {
			highestHigh = maxFloat(highestHigh, highs[cursor])
			lowestLow = minFloat(lowestLow, lows[cursor])
		}
		rsv := 50.0
		if highestHigh != lowestLow {
			rsv = ((closes[index] - lowestLow) / (highestHigh - lowestLow)) * 100
		}
		nextK := ((float64(config.m1)-1)*previousK + rsv) / float64(config.m1)
		nextD := ((float64(config.m2)-1)*previousD + nextK) / float64(config.m2)
		nextJ := 3*nextK - 2*nextD
		kValues = append(kValues, nextK)
		dValues = append(dValues, nextD)
		jValues = append(jValues, nextJ)
		previousK = nextK
		previousD = nextD
	}
	return kValues, dValues, jValues
}

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
	for index := period - 1; index < len(trueRanges); index++ {
		sum := 0.0
		for cursor := index - period + 1; cursor <= index; cursor++ {
			sum += trueRanges[cursor]
		}
		result = append(result, sum/float64(period))
	}
	return result
}

func calculateCCI(highs, lows, closes []float64, period int) any {
	values := calculateCCISeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateCCISeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	typicalPrices := make([]float64, len(closes))
	for index := range closes {
		typicalPrices[index] = (highs[index] + lows[index] + closes[index]) / 3
	}
	result := make([]float64, 0, len(closes)-period+1)
	for index := period - 1; index < len(typicalPrices); index++ {
		window := typicalPrices[index-period+1 : index+1]
		sum := 0.0
		for _, value := range window {
			sum += value
		}
		average := sum / float64(period)
		meanDeviation := 0.0
		for _, value := range window {
			meanDeviation += math.Abs(value - average)
		}
		meanDeviation /= float64(period)
		if meanDeviation == 0 {
			result = append(result, 0)
			continue
		}
		result = append(result, (typicalPrices[index]-average)/(0.015*meanDeviation))
	}
	return result
}

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
	result := make([]float64, 0, len(closes)-period+1)
	for index := period - 1; index < len(closes); index++ {
		start := index - period + 1
		highestHigh := highs[start]
		lowestLow := lows[start]
		for cursor := start + 1; cursor <= index; cursor++ {
			highestHigh = maxFloat(highestHigh, highs[cursor])
			lowestLow = minFloat(lowestLow, lows[cursor])
		}
		if highestHigh == lowestLow {
			result = append(result, -50)
			continue
		}
		result = append(result, -100*(highestHigh-closes[index])/(highestHigh-lowestLow))
	}
	return result
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
