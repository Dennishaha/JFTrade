package indicatorruntime

import "math"

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

func calculateTMASequence(values []float64, period int) []float64 {
	return calculateTMASequenceWithCache(values, period, nil)
}

func calculateTMASequenceWithCache(values []float64, period int, cache *snapshotSeriesCache) []float64 {
	sequence := cache.getSMASequence(values, period)
	if len(sequence) < period {
		return nil
	}
	return calculateSMASequence(sequence, period)
}

func calculateHMASequence(values []float64, period int) []float64 {
	return calculateHMASequenceWithCache(values, period, nil)
}

func calculateHMASequenceWithCache(values []float64, period int, cache *snapshotSeriesCache) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	halfPeriod := max(1, period/2)
	sqrtPeriod := max(1, int(math.Round(math.Sqrt(float64(period)))))
	fastSequence := cache.getWMASequence(values, halfPeriod)
	slowSequence := cache.getWMASequence(values, period)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return nil
	}
	combined := make([]float64, 0, len(values)-period+1)
	for end := period; end <= len(values); end++ {
		fastIndex := end - halfPeriod
		slowIndex := end - period
		if fastIndex < 0 || fastIndex >= len(fastSequence) || slowIndex < 0 || slowIndex >= len(slowSequence) {
			continue
		}
		combined = append(combined, 2*fastSequence[fastIndex]-slowSequence[slowIndex])
	}
	return calculateWMASequence(combined, sqrtPeriod)
}

func volumeWeightedMovingAverage(values, volumes []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period || len(volumes) < period {
		return 0, false
	}
	windowValues := values[len(values)-period:]
	windowVolumes := volumes[len(volumes)-period:]
	volumeSum := 0.0
	weightedSum := 0.0
	for index, value := range windowValues {
		volume := windowVolumes[index]
		volumeSum += volume
		weightedSum += value * volume
	}
	if volumeSum == 0 {
		return 0, false
	}
	return weightedSum / volumeSum, true
}

func calculateSMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	windowSum := 0.0
	for index, value := range values {
		windowSum += value
		if index >= period {
			windowSum -= values[index-period]
		}
		if index >= period-1 {
			result = append(result, windowSum/float64(period))
		}
	}
	return result
}

func calculateSMMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	first, ok := simpleMovingAverage(values[:period], period)
	if !ok {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	result = append(result, first)
	previous := first
	for index := period; index < len(values); index++ {
		previous = (previous*float64(period-1) + values[index]) / float64(period)
		result = append(result, previous)
	}
	return result
}

func calculateRMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	first, ok := simpleMovingAverage(values[:period], period)
	if !ok {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	result = append(result, first)
	previous := first
	for index := period; index < len(values); index++ {
		previous = (previous*float64(period-1) + values[index]) / float64(period)
		result = append(result, previous)
	}
	return result
}

func calculateWMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	weightSum := float64(period * (period + 1) / 2)
	windowSum := 0.0
	weightedSum := 0.0
	for index := range period {
		weight := float64(index + 1)
		windowSum += values[index]
		weightedSum += values[index] * weight
	}
	result = append(result, weightedSum/weightSum)
	for index := period; index < len(values); index++ {
		outgoing := values[index-period]
		previousWindowSum := windowSum
		windowSum += values[index] - outgoing
		weightedSum = weightedSum - previousWindowSum + values[index]*float64(period)
		result = append(result, weightedSum/weightSum)
	}
	return result
}

func calculateEMASequence(values []float64, period int) []float64 {
	return fillEMASequence(nil, values, period)
}
