package backtest

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/service"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

var maxBacktestQueryTime = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)

type sessionFilteredBacktestStore struct {
	base                 service.BackTestable
	includeExtendedHours bool
}

func newBacktestReplayStore(base service.BackTestable, useExtendedHours *bool) service.BackTestable {
	if useExtendedHours == nil {
		return base
	}
	return &sessionFilteredBacktestStore{base: base, includeExtendedHours: *useExtendedHours}
}

type tradingPeriodKLineRangeQuerier interface {
	QueryTradingPeriodKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error)
}

type sessionAwareIntradayKLineRangeQuerier interface {
	QuerySessionAwareIntradayKLinesInRange(symbol string, interval types.Interval, since, until time.Time, includeExtendedHours bool) ([]types.KLine, error)
}

type klineRangeStreamer interface {
	StreamKLines(since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval, emit func(types.KLine)) error
}

func (s *sessionFilteredBacktestStore) Verify(
	sourceExchange types.Exchange, symbols []string, startTime time.Time, endTime time.Time,
) error {
	return s.base.Verify(sourceExchange, symbols, startTime, endTime)
}

func (s *sessionFilteredBacktestStore) Sync(
	ctx context.Context, ex types.Exchange, symbol string, intervals []types.Interval, since, until time.Time,
) error {
	return s.base.Sync(ctx, ex, symbol, intervals, since, until)
}

func (s *sessionFilteredBacktestStore) QueryKLine(
	ex types.Exchange, symbol string, interval types.Interval, orderBy string, limit int,
) (*types.KLine, error) {
	if !s.shouldUseCustomTradingPeriodAggregation(symbol, interval) && !s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) && (s.includeExtendedHours || !shouldFilterExtendedHours(symbol, interval)) {
		return s.base.QueryKLine(ex, symbol, interval, orderBy, limit)
	}

	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderBy))
	if normalizedOrder == "ASC" {
		rows, err := s.QueryKLinesForward(ex, symbol, interval, time.Unix(0, 0).UTC(), 1)
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		first := rows[0]
		return &first, nil
	}

	rows, err := s.QueryKLinesBackward(ex, symbol, interval, maxBacktestQueryTime, 1)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	last := rows[len(rows)-1]
	return &last, nil
}

func (s *sessionFilteredBacktestStore) QueryKLinesForward(
	exchange types.Exchange, symbol string, interval types.Interval, startTime time.Time, limit int,
) ([]types.KLine, error) {
	if s.shouldUseCustomTradingPeriodAggregation(symbol, interval) {
		return s.queryCustomTradingPeriodForward(symbol, interval, startTime, limit)
	}
	if s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryCustomSessionAwareIntradayForward(symbol, interval, startTime, limit)
	}
	if s.includeExtendedHours || !shouldFilterExtendedHours(symbol, interval) {
		return s.base.QueryKLinesForward(exchange, symbol, interval, startTime, limit)
	}

	normalizedLimit := normalizeKLineLimit(limit)
	pageSize := filteredPageSize(normalizedLimit)
	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := startTime

	for len(rows) < normalizedLimit {
		batch, err := s.base.QueryKLinesForward(exchange, symbol, interval, cursor, pageSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		for _, kline := range batch {
			if keepRegularSessionKLine(kline) {
				rows = append(rows, kline)
				if len(rows) == normalizedLimit {
					return rows, nil
				}
			}
		}

		nextCursor := batch[len(batch)-1].EndTime.Time().Add(time.Millisecond)
		if !nextCursor.After(cursor) {
			break
		}
		cursor = nextCursor
		if len(batch) < pageSize {
			break
		}
	}

	return rows, nil
}

func (s *sessionFilteredBacktestStore) QueryKLinesBackward(
	exchange types.Exchange, symbol string, interval types.Interval, endTime time.Time, limit int,
) ([]types.KLine, error) {
	if s.shouldUseCustomTradingPeriodAggregation(symbol, interval) {
		return s.queryCustomTradingPeriodBackward(symbol, interval, endTime, limit)
	}
	if s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) {
		return s.queryCustomSessionAwareIntradayBackward(symbol, interval, endTime, limit)
	}
	if s.includeExtendedHours || !shouldFilterExtendedHours(symbol, interval) {
		return s.base.QueryKLinesBackward(exchange, symbol, interval, endTime, limit)
	}

	normalizedLimit := normalizeKLineLimit(limit)
	pageSize := filteredPageSize(normalizedLimit)
	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := endTime

	for len(rows) < normalizedLimit {
		batch, err := s.base.QueryKLinesBackward(exchange, symbol, interval, cursor, pageSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		filtered := make([]types.KLine, 0, len(batch))
		for _, kline := range batch {
			if keepRegularSessionKLine(kline) {
				filtered = append(filtered, kline)
			}
		}
		if len(filtered) > 0 {
			rows = append(filtered, rows...)
			if len(rows) > normalizedLimit {
				rows = rows[len(rows)-normalizedLimit:]
			}
		}

		nextCursor := batch[0].EndTime.Time().Add(-time.Millisecond)
		if !nextCursor.Before(cursor) {
			break
		}
		cursor = nextCursor
		if len(batch) < pageSize {
			break
		}
	}

	return rows, nil
}

func (s *sessionFilteredBacktestStore) QueryKLinesCh(
	since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval,
) (chan types.KLine, chan error) {
	if !s.needsCustomHandling(symbols, intervals) {
		return s.base.QueryKLinesCh(since, until, exchange, symbols, intervals)
	}

	baseCh, baseErrCh := s.base.QueryKLinesCh(since, until, exchange, symbols, intervals)
	ch := make(chan types.KLine, filteredKLineChannelBufferSize(symbols, intervals))
	errCh := make(chan error, 1)

	go func() {
		defer close(ch)
		defer close(errCh)

		if !s.includeExtendedHours {
			for baseCh != nil || baseErrCh != nil {
				select {
				case kline, ok := <-baseCh:
					if !ok {
						baseCh = nil
						continue
					}
					if shouldFilterExtendedHours(kline.Symbol, kline.Interval) && !keepRegularSessionKLine(kline) {
						continue
					}
					ch <- kline
				case err, ok := <-baseErrCh:
					if !ok {
						baseErrCh = nil
						continue
					}
					if err != nil {
						errCh <- err
						return
					}
				}
			}
			return
		}

		baseRows, err := collectKLinesFromStoreChannels(baseCh, baseErrCh)
		if err != nil {
			errCh <- err
			return
		}

		rows := make([]types.KLine, 0, len(baseRows))
		for _, kline := range baseRows {
			if s.shouldUseCustomTradingPeriodAggregation(kline.Symbol, kline.Interval) {
				continue
			}
			if s.shouldUseCustomSessionAwareIntradayAggregation(kline.Symbol, kline.Interval) {
				continue
			}
			if !s.includeExtendedHours && shouldFilterExtendedHours(kline.Symbol, kline.Interval) && !keepRegularSessionKLine(kline) {
				continue
			}
			rows = append(rows, kline)
		}

		if s.includeExtendedHours {
			for _, symbol := range symbols {
				for _, interval := range intervals {
					if !s.shouldUseCustomTradingPeriodAggregation(symbol, interval) {
						if !s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) {
							continue
						}
						periodRows, periodErr := s.queryCustomSessionAwareIntradayRange(symbol, interval, since, until)
						if periodErr != nil {
							errCh <- periodErr
							return
						}
						rows = append(rows, periodRows...)
						continue
					}
					periodRows, periodErr := s.queryCustomTradingPeriodRange(symbol, interval, since, until)
					if periodErr != nil {
						errCh <- periodErr
						return
					}
					rows = append(rows, periodRows...)
				}
			}
		}

		sort.Slice(rows, func(i, j int) bool {
			leftEnd := rows[i].EndTime.Time()
			rightEnd := rows[j].EndTime.Time()
			if !leftEnd.Equal(rightEnd) {
				return leftEnd.Before(rightEnd)
			}
			leftInterval := rows[i].Interval.Duration()
			rightInterval := rows[j].Interval.Duration()
			if leftInterval != rightInterval {
				return leftInterval < rightInterval
			}
			return rows[i].Symbol < rows[j].Symbol
		})
		for _, kline := range rows {
			ch <- kline
		}
	}()

	return ch, errCh
}

func (s *sessionFilteredBacktestStore) StreamKLines(
	since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval, emit func(types.KLine),
) error {
	if emit == nil {
		return nil
	}
	if !s.needsCustomHandling(symbols, intervals) {
		if streamer, ok := s.base.(klineRangeStreamer); ok {
			return streamer.StreamKLines(since, until, exchange, symbols, intervals, emit)
		}
		baseCh, baseErrCh := s.base.QueryKLinesCh(since, until, exchange, symbols, intervals)
		return emitKLinesFromStoreChannels(baseCh, baseErrCh, emit)
	}

	if !s.includeExtendedHours {
		if streamer, ok := s.base.(klineRangeStreamer); ok {
			return streamer.StreamKLines(since, until, exchange, symbols, intervals, func(kline types.KLine) {
				if shouldFilterExtendedHours(kline.Symbol, kline.Interval) && !keepRegularSessionKLine(kline) {
					return
				}
				emit(kline)
			})
		}
		baseCh, baseErrCh := s.base.QueryKLinesCh(since, until, exchange, symbols, intervals)
		return emitKLinesFromStoreChannels(baseCh, baseErrCh, func(kline types.KLine) {
			if shouldFilterExtendedHours(kline.Symbol, kline.Interval) && !keepRegularSessionKLine(kline) {
				return
			}
			emit(kline)
		})
	}

	baseRows, err := collectBaseKLinesForStream(s.base, since, until, exchange, symbols, intervals)
	if err != nil {
		return err
	}

	rows := make([]types.KLine, 0, len(baseRows))
	for _, kline := range baseRows {
		if s.shouldUseCustomTradingPeriodAggregation(kline.Symbol, kline.Interval) {
			continue
		}
		if s.shouldUseCustomSessionAwareIntradayAggregation(kline.Symbol, kline.Interval) {
			continue
		}
		if !s.includeExtendedHours && shouldFilterExtendedHours(kline.Symbol, kline.Interval) && !keepRegularSessionKLine(kline) {
			continue
		}
		rows = append(rows, kline)
	}

	for _, symbol := range symbols {
		for _, interval := range intervals {
			if !s.shouldUseCustomTradingPeriodAggregation(symbol, interval) {
				if !s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) {
					continue
				}
				periodRows, periodErr := s.queryCustomSessionAwareIntradayRange(symbol, interval, since, until)
				if periodErr != nil {
					return periodErr
				}
				rows = append(rows, periodRows...)
				continue
			}
			periodRows, periodErr := s.queryCustomTradingPeriodRange(symbol, interval, since, until)
			if periodErr != nil {
				return periodErr
			}
			rows = append(rows, periodRows...)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		leftEnd := rows[i].EndTime.Time()
		rightEnd := rows[j].EndTime.Time()
		if !leftEnd.Equal(rightEnd) {
			return leftEnd.Before(rightEnd)
		}
		leftInterval := rows[i].Interval.Duration()
		rightInterval := rows[j].Interval.Duration()
		if leftInterval != rightInterval {
			return leftInterval < rightInterval
		}
		return rows[i].Symbol < rows[j].Symbol
	})
	for _, kline := range rows {
		emit(kline)
	}
	return nil
}

func filteredKLineChannelBufferSize(symbols []string, intervals []types.Interval) int {
	if len(symbols) == 1 && len(intervals) == 1 {
		return 256
	}
	return 512
}

func (s *sessionFilteredBacktestStore) shouldUseCustomTradingPeriodAggregation(symbol string, interval types.Interval) bool {
	return s.includeExtendedHours && isCustomTradingPeriodInterval(interval) && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.")
}

func (s *sessionFilteredBacktestStore) shouldUseCustomSessionAwareIntradayAggregation(symbol string, interval types.Interval) bool {
	return s.includeExtendedHours && isCustomSessionAwareIntradayInterval(interval) && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.")
}

func (s *sessionFilteredBacktestStore) needsCustomHandling(symbols []string, intervals []types.Interval) bool {
	if !s.includeExtendedHours {
		return needsExtendedHoursFilter(symbols, intervals)
	}
	for _, symbol := range symbols {
		for _, interval := range intervals {
			if s.shouldUseCustomTradingPeriodAggregation(symbol, interval) || s.shouldUseCustomSessionAwareIntradayAggregation(symbol, interval) {
				return true
			}
		}
	}
	return false
}

func (s *sessionFilteredBacktestStore) queryCustomTradingPeriodForward(symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := normalizeKLineLimit(limit)
	labelSince, ok := futu.TradingPeriodLabelStartForDate(symbol, startTime, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelSince, interval).Before(startTime) {
		labelSince = shiftTradingPeriodLabel(labelSince, interval, 1)
	}
	labelUntil := tradingPeriodLabelEnd(shiftTradingPeriodLabel(labelSince, interval, normalizedLimit-1), interval)
	rows, err := s.queryCustomTradingPeriodRange(symbol, interval, labelSince, labelUntil)
	if err != nil {
		return nil, err
	}
	if len(rows) > normalizedLimit {
		rows = rows[:normalizedLimit]
	}
	return rows, nil
}

func (s *sessionFilteredBacktestStore) queryCustomTradingPeriodBackward(symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := normalizeKLineLimit(limit)
	effectiveUntil := endTime.Add(-time.Millisecond)
	labelStart, ok := futu.TradingPeriodLabelStartForDate(symbol, effectiveUntil, tradingPeriodUnit(interval))
	if !ok {
		return nil, nil
	}
	if tradingPeriodLabelEnd(labelStart, interval).After(effectiveUntil) {
		labelStart = shiftTradingPeriodLabel(labelStart, interval, -1)
	}
	labelSince := shiftTradingPeriodLabel(labelStart, interval, -(normalizedLimit - 1))
	labelUntil := tradingPeriodLabelEnd(labelStart, interval)
	rows, err := s.queryCustomTradingPeriodRange(symbol, interval, labelSince, labelUntil)
	if err != nil {
		return nil, err
	}
	if len(rows) > normalizedLimit {
		rows = rows[len(rows)-normalizedLimit:]
	}
	return rows, nil
}

func (s *sessionFilteredBacktestStore) queryCustomTradingPeriodRange(symbol string, interval types.Interval, since, until time.Time) ([]types.KLine, error) {
	querier, ok := s.base.(tradingPeriodKLineRangeQuerier)
	if !ok {
		return s.base.QueryKLinesForward(nil, symbol, interval, since, normalizeKLineLimit(int(until.Sub(since)/(24*time.Hour))+1))
	}
	return querier.QueryTradingPeriodKLinesInRange(symbol, interval, since, until, true)
}

func (s *sessionFilteredBacktestStore) queryCustomSessionAwareIntradayForward(symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := normalizeKLineLimit(limit)
	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := startTime
	lastStart := time.Time{}
	for len(rows) < normalizedLimit {
		batch, err := s.queryCustomSessionAwareIntradayRange(symbol, interval, cursor, cursor.Add(14*24*time.Hour))
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

func (s *sessionFilteredBacktestStore) queryCustomSessionAwareIntradayBackward(symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {
	normalizedLimit := normalizeKLineLimit(limit)
	rows := make([]types.KLine, 0, normalizedLimit)
	cursor := endTime.Add(-time.Millisecond)
	for len(rows) < normalizedLimit {
		batch, err := s.queryCustomSessionAwareIntradayRange(symbol, interval, cursor.Add(-14*24*time.Hour), cursor)
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
		for index := 0; index < len(prepend); index++ {
			rows = append([]types.KLine{prepend[index]}, rows...)
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

func (s *sessionFilteredBacktestStore) queryCustomSessionAwareIntradayRange(symbol string, interval types.Interval, since, until time.Time) ([]types.KLine, error) {
	querier, ok := s.base.(sessionAwareIntradayKLineRangeQuerier)
	if !ok {
		return s.base.QueryKLinesForward(nil, symbol, interval, since, normalizeKLineLimit(int(until.Sub(since)/interval.Duration())+4))
	}
	return querier.QuerySessionAwareIntradayKLinesInRange(symbol, interval, since, until, true)
}

func collectKLinesFromStoreChannels(ch chan types.KLine, errCh chan error) ([]types.KLine, error) {
	rows := make([]types.KLine, 0)
	for ch != nil || errCh != nil {
		select {
		case kline, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			rows = append(rows, kline)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return rows, nil
}

func collectBaseKLinesForStream(base service.BackTestable, since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval) ([]types.KLine, error) {
	if streamer, ok := base.(klineRangeStreamer); ok {
		rows := make([]types.KLine, 0)
		err := streamer.StreamKLines(since, until, exchange, symbols, intervals, func(kline types.KLine) {
			rows = append(rows, kline)
		})
		if err != nil {
			return nil, err
		}
		return rows, nil
	}
	baseCh, baseErrCh := base.QueryKLinesCh(since, until, exchange, symbols, intervals)
	return collectKLinesFromStoreChannels(baseCh, baseErrCh)
}

func emitKLinesFromStoreChannels(ch chan types.KLine, errCh chan error, emit func(types.KLine)) error {
	rows, err := collectKLinesFromStoreChannels(ch, errCh)
	if err != nil {
		return err
	}
	for _, kline := range rows {
		emit(kline)
	}
	return nil
}

func shouldFilterExtendedHours(symbol string, interval types.Interval) bool {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") {
		return false
	}
	duration := interval.Duration()
	return duration > 0 && duration <= time.Hour
}

func keepRegularSessionKLine(kline types.KLine) bool {
	if !shouldFilterExtendedHours(kline.Symbol, kline.Interval) {
		return true
	}
	return futu.IsRegularTradingTime(kline.Symbol, kline.StartTime.Time()) ||
		futu.IsRegularTradingTime(kline.Symbol, kline.EndTime.Time())
}

func needsExtendedHoursFilter(symbols []string, intervals []types.Interval) bool {
	for _, symbol := range symbols {
		for _, interval := range intervals {
			if shouldFilterExtendedHours(symbol, interval) {
				return true
			}
		}
	}
	return false
}

func normalizeKLineLimit(limit int) int {
	if limit <= 0 {
		return 1
	}
	return limit
}

func filteredPageSize(limit int) int {
	switch {
	case limit <= 16:
		return 64
	case limit <= 64:
		return limit * 4
	case limit <= 256:
		return limit * 2
	default:
		return limit
	}
}

func isCustomTradingPeriodInterval(interval types.Interval) bool {
	switch interval {
	case types.Interval1d, types.Interval1w, types.Interval1mo:
		return true
	default:
		return false
	}
}

func isCustomSessionAwareIntradayInterval(interval types.Interval) bool {
	duration := interval.Duration()
	return duration > time.Hour && duration < 24*time.Hour
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
