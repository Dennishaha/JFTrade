package storage

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func (s *FutuKLineStore) QueryKLine(
	ex types.Exchange, symbol string, interval types.Interval,
	orderBy string, limit int,
) (*types.KLine, error) {
	if limit <= 0 {
		limit = 1
	}
	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderBy))
	kline, err := s.queryKLine(symbol, interval, normalizedOrder, limit)
	if err != nil || kline != nil {
		return kline, err
	}
	if normalizedOrder == "ASC" {
		return nil, nil
	}
	return nil, nil
}

func (s *FutuKLineStore) queryKLine(
	symbol string, interval types.Interval, orderBy string, limit int,
) (*types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderBy))
	if normalizedOrder != "ASC" && normalizedOrder != "DESC" {
		normalizedOrder = "DESC"
	}
	tableName, err := s.selectReadTableName(symbol, interval, s.rehabType, time.Time{}, time.Time{})
	if err != nil || tableName == "" {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s ORDER BY end_time %s LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName), normalizedOrder,
	)

	row := s.db.QueryRow(query, limit)
	return scanKLine(row, symbol, interval)
}

// queryLatestKLineInRange returns the most recent K-line for the given
// symbol+interval whose end_time falls within [since, until].
func (s *FutuKLineStore) queryLatestKLineInRange(
	symbol string, interval types.Interval,
	since, until time.Time,
) (*types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, err := s.resolveReadSource(symbol, interval, since, until)
	if err != nil {
		return nil, err
	}
	if source.synthesize {
		klines, synthErr := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
		if synthErr != nil || len(klines) == 0 {
			return nil, synthErr
		}
		return new(klines[len(klines)-1]), nil
	}
	klines, queryErr := s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
	if queryErr != nil || len(klines) == 0 {
		return nil, queryErr
	}
	return new(klines[len(klines)-1]), nil
}

func (s *FutuKLineStore) QueryKLinesForward(
	exchange types.Exchange, symbol string, interval types.Interval,
	startTime time.Time, limit int,
) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, err := s.queryStoredKLinesForward(symbol, interval, s.rehabType, startTime, limit)
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 || !canAggregateFromLowerInterval(interval) {
		return stored, nil
	}
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesForwardLocked(symbol, interval, startTime, limit, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesForwardLocked(symbol, interval, startTime, limit)
	}
	effectiveUntil := firstClosedKLineEndAtOrAfter(startTime, interval).Add(interval.Duration() * time.Duration(limit-1))
	source, err := s.resolveReadSource(symbol, interval, startTime, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if !source.synthesize {
		return nil, nil
	}
	aggregated, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, startTime, effectiveUntil)
	if err != nil {
		return nil, err
	}
	if len(aggregated) > limit {
		aggregated = aggregated[:limit]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) QueryKLinesBackward(
	exchange types.Exchange, symbol string, interval types.Interval,
	endTime time.Time, limit int,
) ([]types.KLine, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, err := s.queryStoredKLinesBackward(symbol, interval, s.rehabType, endTime, limit)
	if err != nil {
		return nil, err
	}
	if len(stored) > 0 || !canAggregateFromLowerInterval(interval) {
		return stored, nil
	}
	if shouldUseSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryAggregatedSessionAwareIntradayKLinesBackwardLocked(symbol, interval, endTime, limit, false)
	}
	if isTradingPeriodAggregationInterval(interval) {
		return s.queryAggregatedTradingPeriodKLinesBackwardLocked(symbol, interval, endTime, limit)
	}
	effectiveUntil := endTime.Add(-time.Millisecond)
	effectiveUntil = latestClosedKLineEndAtOrBefore(effectiveUntil, interval)
	since := alignTimeToIntervalStart(effectiveUntil.Add(-interval.Duration()*time.Duration(limit-1)), interval)
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
	if len(aggregated) > limit {
		aggregated = aggregated[len(aggregated)-limit:]
	}
	return aggregated, nil
}

func (s *FutuKLineStore) QueryKLinesCh(
	since, until time.Time, exchange types.Exchange,
	symbols []string, intervals []types.Interval,
) (chan types.KLine, chan error) {
	ch := make(chan types.KLine, klineQueryChannelBufferSize(symbols, intervals))
	errCh := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(errCh)

		if len(symbols) == 0 || len(intervals) == 0 {
			return
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		if len(symbols) == 1 && len(intervals) == 1 {
			symbol := symbols[0]
			interval := intervals[0]
			source, err := s.resolveReadSource(symbol, interval, since, until)
			if err != nil {
				errCh <- err
				return
			}

			if source.synthesize {
				series, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
				if err != nil {
					errCh <- err
					return
				}
				for _, k := range series {
					ch <- k
				}
			} else {
				if err := s.streamStoredKLinesInRangeToChannel(symbol, interval, s.rehabType, since, until, ch); err != nil {
					errCh <- err
					return
				}
			}
			return
		}

		klines := make([]types.KLine, 0, len(symbols)*len(intervals))
		for _, symbol := range symbols {
			for _, interval := range intervals {
				source, err := s.resolveReadSource(symbol, interval, since, until)
				if err != nil {
					errCh <- err
					return
				}

				var series []types.KLine
				if source.synthesize {
					series, err = s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
				} else {
					series, err = s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
				}
				if err != nil {
					errCh <- err
					return
				}
				klines = append(klines, series...)
			}
		}

		sort.Slice(klines, func(i, j int) bool {
			leftEnd := klines[i].EndTime.Time()
			rightEnd := klines[j].EndTime.Time()
			if !leftEnd.Equal(rightEnd) {
				return leftEnd.Before(rightEnd)
			}
			leftInterval := intervalStorageValue(klines[i].Interval)
			rightInterval := intervalStorageValue(klines[j].Interval)
			if leftInterval != rightInterval {
				return leftInterval < rightInterval
			}
			return klines[i].Symbol < klines[j].Symbol
		})
		for _, k := range klines {
			ch <- k
		}
	}()

	return ch, errCh
}

func (s *FutuKLineStore) StreamKLines(
	since, until time.Time, exchange types.Exchange,
	symbols []string, intervals []types.Interval,
	emit func(types.KLine),
) error {
	if emit == nil || len(symbols) == 0 || len(intervals) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(symbols) == 1 && len(intervals) == 1 {
		symbol := symbols[0]
		interval := intervals[0]
		source, err := s.resolveReadSource(symbol, interval, since, until)
		if err != nil {
			return err
		}

		if source.synthesize {
			series, err := s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
			if err != nil {
				return err
			}
			for _, kline := range series {
				emit(kline)
			}
			return nil
		}

		return s.streamStoredKLinesInRange(symbol, interval, s.rehabType, since, until, emit)
	}

	klines := make([]types.KLine, 0, len(symbols)*len(intervals))
	for _, symbol := range symbols {
		for _, interval := range intervals {
			source, err := s.resolveReadSource(symbol, interval, since, until)
			if err != nil {
				return err
			}

			var series []types.KLine
			if source.synthesize {
				series, err = s.queryAggregatedKLinesInRange(symbol, interval, source.baseInterval, since, until)
			} else {
				series, err = s.queryStoredKLinesInRange(symbol, interval, s.rehabType, since, until)
			}
			if err != nil {
				return err
			}
			klines = append(klines, series...)
		}
	}

	sort.Slice(klines, func(i, j int) bool {
		leftEnd := klines[i].EndTime.Time()
		rightEnd := klines[j].EndTime.Time()
		if !leftEnd.Equal(rightEnd) {
			return leftEnd.Before(rightEnd)
		}
		leftInterval := intervalStorageValue(klines[i].Interval)
		rightInterval := intervalStorageValue(klines[j].Interval)
		if leftInterval != rightInterval {
			return leftInterval < rightInterval
		}
		return klines[i].Symbol < klines[j].Symbol
	})
	for _, kline := range klines {
		emit(kline)
	}
	return nil
}

func (s *FutuKLineStore) queryStoredKLinesForward(symbol string, interval types.Interval, rehabType string, startTime time.Time, limit int) ([]types.KLine, error) {
	var firstNonEmpty []types.KLine
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := s.queryStoredKLinesForwardFromTable(tableName, symbol, interval, startTime, limit)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) >= limit {
			return rows, nil
		}
		if len(firstNonEmpty) == 0 {
			firstNonEmpty = rows
		}
	}
	return firstNonEmpty, nil
}

func (s *FutuKLineStore) queryStoredKLinesForwardFromTable(tableName string, symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? ORDER BY end_time ASC LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(startTime), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKLinesWithCapacity(rows, symbol, interval, limit)
}

func (s *FutuKLineStore) queryStoredKLinesBackward(symbol string, interval types.Interval, rehabType string, endTime time.Time, limit int) ([]types.KLine, error) {
	var firstNonEmpty []types.KLine
	tableNames, tableCount := s.readTableNames(symbol, interval, rehabType)
	for index := 0; index < tableCount; index++ {
		tableName := tableNames[index]
		exists, err := s.klineTableExists(tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		rows, err := s.queryStoredKLinesBackwardFromTable(tableName, symbol, interval, endTime, limit)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		if len(rows) >= limit {
			return rows, nil
		}
		if len(firstNonEmpty) == 0 {
			firstNonEmpty = rows
		}
	}
	return firstNonEmpty, nil
}

func (s *FutuKLineStore) queryStoredKLinesBackwardFromTable(tableName string, symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time <= ? ORDER BY end_time DESC LIMIT ?`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(endTime), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	klines, err := scanKLinesWithCapacity(rows, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	reverseKLines(klines)
	return klines, nil
}

func (s *FutuKLineStore) queryStoredKLinesInRange(symbol string, interval types.Interval, rehabType string, since, until time.Time) ([]types.KLine, error) {
	tableName, err := s.selectReadTableName(symbol, interval, rehabType, since, until)
	if err != nil || tableName == "" {
		return nil, err
	}
	return s.queryStoredKLinesInRangeFromTable(tableName, symbol, interval, since, until)
}

func (s *FutuKLineStore) streamStoredKLinesInRange(symbol string, interval types.Interval, rehabType string, since, until time.Time, emit func(types.KLine)) error {
	tableName, err := s.selectReadTableName(symbol, interval, rehabType, since, until)
	if err != nil || tableName == "" {
		return err
	}
	return s.streamStoredKLinesInRangeFromTable(tableName, symbol, interval, since, until, emit)
}

func (s *FutuKLineStore) streamStoredKLinesInRangeToChannel(symbol string, interval types.Interval, rehabType string, since, until time.Time, ch chan<- types.KLine) error {
	return s.streamStoredKLinesInRange(symbol, interval, rehabType, since, until, func(kline types.KLine) {
		ch <- kline
	})
}

func (s *FutuKLineStore) queryStoredKLinesInRangeFromTable(tableName string, symbol string, interval types.Interval, since, until time.Time) ([]types.KLine, error) {

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(since), timeToUnixMillis(until))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKLines(rows, symbol, interval)
}

func (s *FutuKLineStore) streamStoredKLinesInRangeFromTable(tableName string, symbol string, interval types.Interval, since, until time.Time, emit func(types.KLine)) error {
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE end_time >= ? AND end_time <= ? ORDER BY end_time ASC`,
		selectKLineColumns,
		quoteIdentifier(tableName),
	)
	rows, err := s.db.Query(query, timeToUnixMillis(since), timeToUnixMillis(until))
	if err != nil {
		return err
	}
	defer rows.Close()
	return streamKLines(rows, symbol, interval, emit)
}

func klineQueryChannelBufferSize(symbols []string, intervals []types.Interval) int {
	if len(symbols) == 1 && len(intervals) == 1 {
		return 256
	}
	return 512
}
