package marketdata

import (
	"time"

	"github.com/shopspring/decimal"
)

func TickCandles(samples []Tick, from, to time.Time, limit int) []map[string]any {
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}

	candles := make([]map[string]any, 0, len(samples))
	for _, sample := range samples {
		observedAt := parseTime(sample.ObservedAt)
		if !observedAt.IsZero() && (observedAt.Before(from) || observedAt.After(to)) {
			continue
		}
		deltaVolume := sample.VolumeDelta
		if deltaVolume.IsNegative() {
			deltaVolume = decimal.Zero
		}
		candles = append(candles, map[string]any{
			"period":  "tick",
			"open":    sample.Price.String(),
			"high":    sample.Price.String(),
			"low":     sample.Price.String(),
			"close":   sample.Price.String(),
			"volume":  deltaVolume.String(),
			"at":      sample.ObservedAt,
			"session": sample.Session,
		})
	}
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles
}
