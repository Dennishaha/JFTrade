package jftradeapi

import "time"

func (s *Server) cachedTickCandles(instrumentID string, query map[string][]string, limit int) []map[string]any {
	beginAt, endAt := tickCandleQueryRange(query)
	samples := s.tickCache.snapshot(instrumentID)

	candles := tickCandlesFromSamples(samples, beginAt, endAt)
	if limit > 0 && len(candles) > limit {
		candles = candles[len(candles)-limit:]
	}
	return candles
}

func tickCandleQueryRange(query map[string][]string) (time.Time, time.Time) {
	endAt := parseQueryTime(firstQuery(query, "toTime", ""), time.Now())
	if queryEnd := firstQuery(query, "to", ""); queryEnd != "" {
		endAt = parseQueryTime(queryEnd, endAt)
	}
	defaultBegin := endAt.Add(-15 * time.Minute)
	beginAt := parseQueryTime(firstQuery(query, "fromTime", ""), defaultBegin)
	if queryBegin := firstQuery(query, "from", ""); queryBegin != "" {
		beginAt = parseQueryTime(queryBegin, beginAt)
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
			"open":    priceJSON(sample.Price),
			"high":    priceJSON(sample.Price),
			"low":     priceJSON(sample.Price),
			"close":   priceJSON(sample.Price),
			"volume":  deltaVolume,
			"at":      sample.ObservedAt,
			"session": sample.Session,
		})
	}
	return candles
}
