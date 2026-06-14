package marketdata

import "time"

func TickCandles(samples []Tick, from, to time.Time, limit int) []map[string]any {
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}

	candles := make([]map[string]any, 0, len(samples))
	previousCumulativeVolume := 0.0
	hasPreviousCumulativeVolume := false
	for _, sample := range samples {
		deltaVolume := 0.0
		if hasPreviousCumulativeVolume {
			deltaVolume = sample.Volume - previousCumulativeVolume
			if deltaVolume < 0 {
				deltaVolume = 0
			}
		}
		previousCumulativeVolume = sample.Volume
		hasPreviousCumulativeVolume = true

		observedAt := parseTime(sample.ObservedAt)
		if !observedAt.IsZero() && (observedAt.Before(from) || observedAt.After(to)) {
			continue
		}
		candles = append(candles, map[string]any{
			"period":  "tick",
			"open":    sample.Price.String(),
			"high":    sample.Price.String(),
			"low":     sample.Price.String(),
			"close":   sample.Price.String(),
			"volume":  deltaVolume,
			"at":      sample.ObservedAt,
			"session": sample.Session,
		})
	}
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles
}
