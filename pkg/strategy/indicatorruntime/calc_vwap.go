package indicatorruntime

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func calculateSessionVWAP(values, volumes []float64, endTimes []time.Time, symbol string, includeExtendedHours bool) (float64, bool) {
	if len(values) == 0 || len(volumes) != len(values) || len(endTimes) != len(values) {
		return 0, false
	}
	currentPeriod, ok := vwapPeriodKey(symbol, endTimes[len(endTimes)-1], "day", includeExtendedHours)
	if !ok {
		return 0, false
	}
	totalPV := 0.0
	totalVolume := 0.0
	for index := len(values) - 1; index >= 0; index-- {
		period, periodOK := vwapPeriodKey(symbol, endTimes[index], "day", includeExtendedHours)
		if !periodOK || period != currentPeriod {
			break
		}
		totalPV += values[index] * volumes[index]
		totalVolume += volumes[index]
	}
	if totalVolume <= 0 {
		return 0, false
	}
	return totalPV / totalVolume, true
}

func calculateAnchoredVWAP(values, volumes []float64, endTimes []time.Time, unit string, symbol string, includeExtendedHours bool) (float64, bool) {
	if len(values) == 0 || len(volumes) != len(values) || len(endTimes) != len(values) {
		return 0, false
	}
	currentPeriod, ok := vwapPeriodKey(symbol, endTimes[len(endTimes)-1], unit, includeExtendedHours)
	if !ok {
		return 0, false
	}
	totalPV, totalVolume := 0.0, 0.0
	for index := len(values) - 1; index >= 0; index-- {
		period, periodOK := vwapPeriodKey(symbol, endTimes[index], unit, includeExtendedHours)
		if !periodOK || period != currentPeriod {
			break
		}
		totalPV += values[index] * volumes[index]
		totalVolume += volumes[index]
	}
	if totalVolume <= 0 {
		return 0, false
	}
	return totalPV / totalVolume, true
}

func vwapPeriodKey(symbol string, at time.Time, unit string, includeExtendedHours bool) (string, bool) {
	for _, candidate := range []time.Time{at, at.Add(-time.Nanosecond)} {
		if key, ok := market.TradingPeriodKey(symbol, candidate, unit, includeExtendedHours); ok {
			return key, true
		}
	}
	return market.CalendarPeriodKey(symbol, at, unit)
}
