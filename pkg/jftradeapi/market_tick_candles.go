package jftradeapi

import "time"

func (s *Server) cachedTickCandles(instrumentID string, query marketCandlesQuery, limit int) []map[string]any {
	beginAt, endAt := tickCandleQueryRange(query)
	samples := s.tickCache.snapshot(instrumentID)

	candles := tickCandlesFromSamples(samples, beginAt, endAt)
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles
}

func tickCandleQueryRange(query marketCandlesQuery) (time.Time, time.Time) {
	endAt := time.Now()
	if !query.ToTime.Time.IsZero() {
		endAt = query.ToTime.Time
	}
	if !query.To.Time.IsZero() {
		endAt = query.To.Time
	}
	defaultBegin := endAt.Add(-15 * time.Minute)
	beginAt := defaultBegin
	if !query.FromTime.Time.IsZero() {
		beginAt = query.FromTime.Time
	}
	if !query.From.Time.IsZero() {
		beginAt = query.From.Time
	}
	return beginAt, endAt
}

func tickCandlesFromSamples(samples []marketTickSample, beginAt time.Time, endAt time.Time) []map[string]any {
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

		observedAt := parseQueryTime(sample.ObservedAt, time.Time{})
		if !observedAt.IsZero() && (observedAt.Before(beginAt) || observedAt.After(endAt)) {
			continue
		}
		candles = append(candles, map[string]any{
			"period":  "tick",
			"open":    priceString(sample.Price),
			"high":    priceString(sample.Price),
			"low":     priceString(sample.Price),
			"close":   priceString(sample.Price),
			"volume":  deltaVolume,
			"at":      sample.ObservedAt,
			"session": sample.Session,
		})
	}
	return candles
}
