package indicatorruntime

import "time"

func calculateSessionVWAP(values, volumes []float64, endTimes []time.Time) (float64, bool) {
	if len(values) == 0 || len(volumes) != len(values) || len(endTimes) != len(values) {
		return 0, false
	}
	currentDay := endTimes[len(endTimes)-1].UTC().YearDay()
	currentYear := endTimes[len(endTimes)-1].UTC().Year()
	totalPV := 0.0
	totalVolume := 0.0
	for index := len(values) - 1; index >= 0; index-- {
		at := endTimes[index].UTC()
		if at.Year() != currentYear || at.YearDay() != currentDay {
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

func calculateAnchoredVWAP(values, volumes []float64, endTimes []time.Time, unit string) (float64, bool) {
	if len(values) == 0 || len(volumes) != len(values) || len(endTimes) != len(values) {
		return 0, false
	}
	last := endTimes[len(endTimes)-1].UTC()
	samePeriod := func(at time.Time) bool {
		at = at.UTC()
		switch unit {
		case "day":
			return at.Year() == last.Year() && at.YearDay() == last.YearDay()
		case "week":
			atYear, atWeek := at.ISOWeek()
			lastYear, lastWeek := last.ISOWeek()
			return atYear == lastYear && atWeek == lastWeek
		case "month":
			return at.Year() == last.Year() && at.Month() == last.Month()
		default:
			return false
		}
	}
	totalPV, totalVolume := 0.0, 0.0
	for index := len(values) - 1; index >= 0; index-- {
		if !samePeriod(endTimes[index]) {
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
