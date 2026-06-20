package storage

import (
	"fmt"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func (s *FutuKLineStore) QueryTradingPeriodKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryTradingPeriodKLinesInRangeLocked(symbol, interval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) QuerySessionAwareIntradayKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) QueryDailyKLinesInRange(symbol string, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryDailyKLinesInRangeLocked(symbol, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) queryAggregatedKLinesInRange(symbol string, interval, baseInterval types.Interval, since, until time.Time) ([]types.KLine, error) {
	if interval == types.Interval1d {
		return s.queryDailyKLinesInRangeLocked(symbol, since, until, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesInRangeLocked(symbol, interval, baseInterval, since, until, false)
	}
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol, interval, baseInterval, since, until, false)
	}
	if interval == baseInterval || interval.Duration() <= baseInterval.Duration() {
		return nil, nil
	}
	baseSince := alignTimeToIntervalStart(since, interval)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, until)
	if err != nil {
		return nil, err
	}
	return aggregateKLinesFromBase(symbol, interval, baseInterval, baseRows, since, until), nil
}

func (s *FutuKLineStore) queryDailyKLinesInRangeLocked(symbol string, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	dailySince := alignTimeToIntervalStart(since, types.Interval1d)
	dailyUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if dailyUntil.Before(dailySince) {
		return nil, nil
	}

	if !includeExtendedHours {
		stored, err := s.queryStoredKLinesInRange(symbol, types.Interval1d, s.rehabType, dailySince, dailyUntil)
		if err != nil {
			return nil, err
		}
		if len(stored) > 0 {
			return stored, nil
		}
	}

	baseInterval, err := s.resolveDailyAggregationBaseInterval(symbol, dailySince, dailyUntil, includeExtendedHours)
	if err != nil {
		if includeExtendedHours {
			stored, storedErr := s.queryStoredKLinesInRange(symbol, types.Interval1d, s.rehabType, dailySince, dailyUntil)
			if storedErr != nil {
				return nil, storedErr
			}
			if len(stored) > 0 {
				return stored, nil
			}
		}
		return nil, err
	}

	return s.queryAggregatedDailyKLinesInRangeLocked(symbol, baseInterval, dailySince, dailyUntil, includeExtendedHours)
}

func (s *FutuKLineStore) queryTradingPeriodKLinesInRangeLocked(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	if interval == types.Interval1d {
		return s.queryDailyKLinesInRangeLocked(symbol, since, until, includeExtendedHours)
	}
	if !isTradingPeriodAggregationInterval(interval) {
		return nil, nil
	}

	baseInterval, err := s.resolveTradingPeriodAggregationBaseInterval(symbol, interval, since, until, includeExtendedHours)
	if err != nil {
		return nil, err
	}
	return s.queryAggregatedTradingPeriodKLinesInRangeLocked(symbol, interval, baseInterval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) querySessionAwareIntradayKLinesInRangeLocked(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	if !shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return nil, nil
	}

	baseInterval, err := s.resolveSessionAwareIntradayAggregationBaseInterval(symbol, interval, since, until, includeExtendedHours)
	if err != nil {
		return nil, err
	}
	return s.queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol, interval, baseInterval, since, until, includeExtendedHours)
}

func (s *FutuKLineStore) resolveSessionAwareIntradayAggregationBaseInterval(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := aggregationBaseIntervals(interval)
	if includeExtendedHours {
		filtered := make([]types.Interval, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download lower-interval data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := sessionAwareIntradayAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s-or-lower data covering the full range for extended-hours intraday aggregation", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesInRangeLocked(symbol string, interval, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := sessionAwareIntradayAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateSessionAwareIntradayKLinesFromBase(symbol, interval, baseRows, since, until, includeExtendedHours), nil
}

func (s *FutuKLineStore) resolveTradingPeriodAggregationBaseInterval(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := aggregationBaseIntervals(interval)
	if includeExtendedHours {
		filtered := make([]types.Interval, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download lower-interval data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := tradingPeriodAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s-or-lower data covering the full range for extended-hours trading-period aggregation", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s %s [%s, %s]; download %s data covering the full range", symbol, interval, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesInRangeLocked(symbol string, interval, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := tradingPeriodAggregationBaseRange(symbol, interval, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateTradingPeriodKLinesFromBase(symbol, interval, baseRows, since, until, includeExtendedHours), nil
}

func (s *FutuKLineStore) resolveDailyAggregationBaseInterval(symbol string, since, until time.Time, includeExtendedHours bool) (types.Interval, error) {
	candidates := prioritizeDailyAggregationBaseIntervals(aggregationBaseIntervals(types.Interval1d), includeExtendedHours)
	if len(candidates) == 0 {
		return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download 1d data covering the full range", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}

	baseSince, baseUntil := dailyAggregationBaseRange(symbol, since, until, includeExtendedHours)
	for _, baseInterval := range candidates {
		baseMissing, err := s.findMissingRangesInTable(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
		if err != nil {
			return "", err
		}
		if len(baseMissing) == 0 {
			return baseInterval, nil
		}
	}

	preferred := candidates[0]
	if includeExtendedHours {
		return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download %s data covering the full range for extended-hours daily aggregation", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
	}
	return "", fmt.Errorf("missing K-line coverage for %s 1d [%s, %s]; download %s data covering the full range", symbol, since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339), preferred)
}

func (s *FutuKLineStore) queryAggregatedDailyKLinesInRangeLocked(symbol string, baseInterval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error) {
	baseSince, baseUntil := dailyAggregationBaseRange(symbol, since, until, includeExtendedHours)
	baseRows, err := s.queryStoredKLinesInRange(symbol, baseInterval, s.rehabType, baseSince, baseUntil)
	if err != nil {
		return nil, err
	}
	return aggregateDailyKLinesFromBase(symbol, baseRows, since, until, includeExtendedHours), nil
}

func prioritizeDailyAggregationBaseIntervals(candidates []types.Interval, includeExtendedHours bool) []types.Interval {
	if !includeExtendedHours || len(candidates) == 0 {
		return candidates
	}
	preferred := make([]types.Interval, 0, len(candidates))
	fallback := make([]types.Interval, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Duration() > 0 && candidate.Duration() <= time.Hour {
			preferred = append(preferred, candidate)
			continue
		}
		fallback = append(fallback, candidate)
	}
	return append(preferred, fallback...)
}

func dailyAggregationBaseRange(symbol string, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := alignTimeToIntervalStart(since, types.Interval1d)
	baseUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if includeExtendedHours && market.IsUSSymbol(symbol) {
		baseSince = baseSince.Add(-6 * time.Hour)
		baseUntil = baseUntil.Add(6 * time.Hour)
	}
	return baseSince, baseUntil
}

func tradingPeriodAggregationBaseRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := alignTimeToIntervalStart(since, types.Interval1d)
	if interval != types.Interval1d {
		if labelStart, ok := market.TradingPeriodLabelStartForDate(symbol, since, tradingPeriodUnit(interval)); ok {
			baseSince = labelStart
		}
	}
	baseUntil := latestClosedKLineEndAtOrBefore(until, types.Interval1d)
	if includeExtendedHours && market.IsUSSymbol(symbol) {
		baseSince = baseSince.Add(-6 * time.Hour)
		baseUntil = baseUntil.Add(6 * time.Hour)
	}
	return baseSince, baseUntil
}

func sessionAwareIntradayAggregationBaseRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) (time.Time, time.Time) {
	baseSince := since
	if bucketStart, _, ok := market.SessionAwareIntradayBucketBounds(symbol, since, interval.Duration(), includeExtendedHours); ok {
		baseSince = bucketStart
	}
	return baseSince, until
}

func aggregateDailyKLinesFromBase(symbol string, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	labelSince := alignTimeToIntervalStart(since, types.Interval1d)
	labelUntil := alignTimeToIntervalStart(until, types.Interval1d)
	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentLabel string
	currentVolume := fixedpoint.Zero

	flush := func() {
		if currentLabel == "" {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		labelAt, ok := market.TradingDayLabelStart(symbol, dailyAggregationObservedAt(base), includeExtendedHours)
		if !ok {
			continue
		}
		if labelAt.Before(labelSince) || labelAt.After(labelUntil) {
			continue
		}

		labelKey := labelAt.Format("2006-01-02")
		if currentLabel == "" || currentLabel != labelKey {
			flush()
			currentLabel = labelKey
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(labelAt),
				EndTime:   types.Time(labelAt.Add(24 * time.Hour).Add(-time.Millisecond)),
				Interval:  types.Interval1d,
				Symbol:    symbol,
				Open:      base.Open,
				High:      base.High,
				Low:       base.Low,
				Close:     base.Close,
				Closed:    true,
			}
		} else {
			if base.High.Compare(current.High) > 0 {
				current.High = base.High
			}
			if base.Low.Compare(current.Low) < 0 {
				current.Low = base.Low
			}
			current.Close = base.Close
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func dailyAggregationObservedAt(kline types.KLine) time.Time {
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		return kline.EndTime.Time().UTC()
	}
	return observedAt
}

func aggregateTradingPeriodKLinesFromBase(symbol string, interval types.Interval, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	unit := tradingPeriodUnit(interval)
	if unit == "" {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentLabel string
	currentVolume := fixedpoint.Zero
	flush := func() {
		if currentLabel == "" {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		labelAt, ok := tradingPeriodLabelForBaseKLine(symbol, base, unit, includeExtendedHours)
		if !ok {
			continue
		}

		labelKey := labelAt.Format("2006-01-02")
		if currentLabel == "" || currentLabel != labelKey {
			flush()
			currentLabel = labelKey
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(labelAt),
				EndTime:   types.Time(tradingPeriodLabelEnd(labelAt, interval)),
				Interval:  interval,
				Symbol:    symbol,
				Open:      base.Open,
				High:      base.High,
				Low:       base.Low,
				Close:     base.Close,
				Closed:    true,
			}
		} else {
			if base.High.Compare(current.High) > 0 {
				current.High = base.High
			}
			if base.Low.Compare(current.Low) < 0 {
				current.Low = base.Low
			}
			current.Close = base.Close
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func aggregateSessionAwareIntradayKLinesFromBase(symbol string, interval types.Interval, baseRows []types.KLine, since, until time.Time, includeExtendedHours bool) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows))
	var current types.KLine
	var currentBucketStart time.Time
	var currentBucketEnd time.Time
	currentVolume := fixedpoint.Zero
	bucketDuration := interval.Duration()

	flush := func() {
		if currentBucketStart.IsZero() {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		observedAt := dailyAggregationObservedAt(base)
		bucketStart, bucketEnd, ok := currentBucketStart, currentBucketEnd, !currentBucketStart.IsZero() && !observedAt.Before(currentBucketStart) && !observedAt.After(currentBucketEnd)
		if !ok {
			bucketStart, bucketEnd, ok = market.SessionAwareIntradayBucketBounds(symbol, observedAt, bucketDuration, includeExtendedHours)
			if !ok {
				continue
			}
		}

		if currentBucketStart.IsZero() || !bucketStart.Equal(currentBucketStart) {
			flush()
			currentBucketStart = bucketStart
			currentBucketEnd = bucketEnd
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(bucketStart),
				EndTime:   types.Time(bucketEnd),
				Interval:  interval,
				Symbol:    symbol,
				Open:      base.Open,
				High:      base.High,
				Low:       base.Low,
				Close:     base.Close,
				Closed:    true,
			}
		} else {
			if base.High.Compare(current.High) > 0 {
				current.High = base.High
			}
			if base.Low.Compare(current.Low) < 0 {
				current.Low = base.Low
			}
			current.Close = base.Close
		}
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}

func tradingPeriodLabelForBaseKLine(symbol string, kline types.KLine, unit string, includeExtendedHours bool) (time.Time, bool) {
	if kline.Interval == types.Interval1d {
		return market.TradingPeriodLabelStartForDate(symbol, kline.StartTime.Time(), unit)
	}
	return market.TradingPeriodLabelStart(symbol, dailyAggregationObservedAt(kline), unit, includeExtendedHours)
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesForwardLocked(symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	labelStart, ok := market.TradingPeriodLabelStartForDate(symbol, startTime, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelStart, interval).Before(startTime) {
		labelStart = shiftTradingPeriodLabel(labelStart, interval, 1)
	}
	effectiveUntil := tradingPeriodLabelEnd(shiftTradingPeriodLabel(labelStart, interval, normalizedLimit-1), interval)
	source, err := s.resolveReadSource(symbol, interval, labelStart, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, labelStart, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > normalizedLimit {
		aggregated = aggregated[:normalizedLimit]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) queryAggregatedTradingPeriodKLinesBackwardLocked(symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	anchor := endTime.Add(-time.Millisecond)
	labelStart, ok := market.TradingPeriodLabelStartForDate(symbol, anchor, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelStart, interval).After(anchor) {
		labelStart = shiftTradingPeriodLabel(labelStart, interval, -1)
	}
	since := shiftTradingPeriodLabel(labelStart, interval, -(normalizedLimit - 1))
	effectiveUntil := tradingPeriodLabelEnd(labelStart, interval)
	source, err := s.resolveReadSource(symbol, interval, since, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > normalizedLimit {
		aggregated = aggregated[len(aggregated)-normalizedLimit:]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesForwardLocked(symbol string, interval types.Interval, startTime time.Time, limit int, includeExtendedHours bool) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := startTime
	lastStart := time.Time{}
	for len(rows) < normalizedLimit {
		batch, err := s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, cursor, cursor.Add(14*24*time.Hour), includeExtendedHours)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		appended := 0
		for _, row := range batch {
			if row.EndTime.Time().Before(startTime) {
				continue
			}
			if !lastStart.IsZero() && row.StartTime.Time().Equal(lastStart) {
				continue
			}
			rows = append(rows, row)
			lastStart = row.StartTime.Time()
			appended++
			if len(rows) == normalizedLimit {
				return rows, nil
			}
		}

		nextCursor := batch[len(batch)-1].EndTime.Time().Add(time.Millisecond)
		if appended == 0 || !nextCursor.After(cursor) {
			break
		}
		cursor = nextCursor
	}
	return rows, nil
}

func (s *FutuKLineStore) queryAggregatedSessionAwareIntradayKLinesBackwardLocked(symbol string, interval types.Interval, endTime time.Time, limit int, includeExtendedHours bool) ([]types.KLine, error) {
	normalizedLimit := limit
	if normalizedLimit <= 0 {
		normalizedLimit = 1
	}

	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := endTime.Add(-time.Millisecond)
	for len(rows) < normalizedLimit {
		batch, err := s.querySessionAwareIntradayKLinesInRangeLocked(symbol, interval, cursor.Add(-14*24*time.Hour), cursor, includeExtendedHours)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		appended := 0
		prepend := make([]types.KLine, 0, normalizedLimit-len(rows))
		for index := len(batch) - 1; index >= 0 && len(rows)+len(prepend) < normalizedLimit; index-- {
			row := batch[index]
			if row.EndTime.Time().After(cursor) {
				continue
			}
			if len(rows) > 0 && row.StartTime.Time().Equal(rows[0].StartTime.Time()) {
				continue
			}
			prepend = append(prepend, row)
			appended++
		}
		if len(prepend) > 0 {
			reverseKLines(prepend)
			rows = append(prepend, rows...)
		}

		nextCursor := batch[0].StartTime.Time().Add(-time.Millisecond)
		if appended == 0 || !nextCursor.Before(cursor) {
			break
		}
		cursor = nextCursor
	}
	if len(rows) > normalizedLimit {
		rows = rows[len(rows)-normalizedLimit:]
	}
	return rows, nil
}

func isTradingPeriodAggregationInterval(interval types.Interval) bool {
	switch interval {
	case types.Interval1d, types.Interval1w, types.Interval1mo:
		return true
	default:
		return false
	}
}

func tradingPeriodUnit(interval types.Interval) string {
	switch interval {
	case types.Interval1d:
		return "day"
	case types.Interval1w:
		return "week"
	case types.Interval1mo:
		return "month"
	default:
		return ""
	}
}

func tradingPeriodLabelEnd(labelAt time.Time, interval types.Interval) time.Time {
	switch interval {
	case types.Interval1d:
		return labelAt.AddDate(0, 0, 1).Add(-time.Millisecond)
	case types.Interval1w:
		return labelAt.AddDate(0, 0, 7).Add(-time.Millisecond)
	case types.Interval1mo:
		return labelAt.AddDate(0, 1, 0).Add(-time.Millisecond)
	default:
		return labelAt
	}
}

func shiftTradingPeriodLabel(labelAt time.Time, interval types.Interval, steps int) time.Time {
	switch interval {
	case types.Interval1d:
		return labelAt.AddDate(0, 0, steps)
	case types.Interval1w:
		return labelAt.AddDate(0, 0, 7*steps)
	case types.Interval1mo:
		return labelAt.AddDate(0, steps, 0)
	default:
		return labelAt
	}
}

func shouldUseSessionAwareIntradayAggregation(symbol string, interval types.Interval) bool {
	if !isSessionAwareIntradayAggregationInterval(interval) {
		return false
	}
	_, ok := market.ProfileForSymbol(symbol)
	return ok
}

func isSessionAwareIntradayAggregationInterval(interval types.Interval) bool {
	duration := interval.Duration()
	return duration > time.Hour && duration < 24*time.Hour
}

func aggregateKLinesFromBase(symbol string, interval, baseInterval types.Interval, baseRows []types.KLine, since, until time.Time) []types.KLine {
	if len(baseRows) == 0 {
		return nil
	}

	factor := int(interval.Duration() / baseInterval.Duration())
	if factor <= 1 {
		return nil
	}

	aggregated := make([]types.KLine, 0, len(baseRows)/factor)
	var current types.KLine
	var currentBucketStart time.Time
	currentCount := 0
	currentVolume := fixedpoint.Zero

	flush := func() {
		if currentCount != factor {
			return
		}
		endAt := current.EndTime.Time()
		if endAt.Before(since) || endAt.After(until) {
			return
		}
		current.Volume = currentVolume
		aggregated = append(aggregated, current)
	}

	for _, base := range baseRows {
		bucketStart := alignTimeToIntervalStart(base.StartTime.Time(), interval)
		if currentCount == 0 || !bucketStart.Equal(currentBucketStart) {
			flush()
			currentBucketStart = bucketStart
			currentCount = 0
			currentVolume = fixedpoint.Zero
			current = types.KLine{
				StartTime: types.Time(bucketStart),
				EndTime:   base.EndTime,
				Interval:  interval,
				Symbol:    symbol,
				Open:      base.Open,
				High:      base.High,
				Low:       base.Low,
				Close:     base.Close,
				Closed:    true,
			}
		} else {
			if base.High.Compare(current.High) > 0 {
				current.High = base.High
			}
			if base.Low.Compare(current.Low) < 0 {
				current.Low = base.Low
			}
			current.Close = base.Close
			current.EndTime = base.EndTime
		}
		currentCount++
		currentVolume = currentVolume.Add(base.Volume)
	}
	flush()
	return aggregated
}
