package backtest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgofixedpoint "github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// HistoricalCandleSource is the backtest consumer port for every historical
// provider. Supplier-specific payloads are converted before entering here.
type HistoricalCandleSource interface {
	FetchHistoricalCandles(context.Context, HistoricalCandleQuery) (HistoricalCandlePage, error)
}

type historicalCandleSourceValidator interface {
	ValidateHistoricalCandleQuery(HistoricalCandleQuery) error
}

type HistoricalCandleQuery struct {
	Market     string
	Symbol     string
	Interval   string
	Adjustment RehabType
	Sessions   []string
	Limit      int
	Before     time.Time
	Since      time.Time
}

type HistoricalCandle struct {
	At                     time.Time
	Open, High, Low, Close string
	Volume                 string
	Session                string
}

type HistoricalCandlePage struct {
	Candles    []HistoricalCandle
	HasMore    bool
	NextBefore time.Time
}

type HistoricalKLineSyncer struct {
	store    *bt.KLineStore
	source   HistoricalCandleSource
	closeFn  func() error
	once     sync.Once
	closeErr error
}

func NewHistoricalKLineSyncer(
	store *bt.KLineStore,
	source HistoricalCandleSource,
	closeFn func() error,
) *HistoricalKLineSyncer {
	return &HistoricalKLineSyncer{store: store, source: source, closeFn: closeFn}
}

func (s *HistoricalKLineSyncer) Validate(params KLineSyncParams) error {
	if s == nil || s.store == nil || s.source == nil {
		return fmt.Errorf("historical candle syncer is unavailable")
	}
	validator, ok := s.source.(historicalCandleSourceValidator)
	if !ok {
		return nil
	}
	for _, interval := range params.Intervals {
		query := historicalQuery(params, interval, params.Until.Add(time.Nanosecond))
		if err := validator.ValidateHistoricalCandleQuery(query); err != nil {
			return requestErrorf("%v", err)
		}
	}
	return nil
}

func (s *HistoricalKLineSyncer) Sync(
	ctx context.Context,
	params KLineSyncParams,
	progress *bt.SyncProgress,
) error {
	if s == nil || s.store == nil || s.source == nil {
		return fmt.Errorf("historical candle syncer is unavailable")
	}
	s.store.SetProviderID(params.MarketDataProvider)
	s.store.SetWriteSessionScope(params.SessionScope)
	if progress != nil {
		progress.SetRunning(len(params.Intervals), time.Now().UTC())
	}
	for index, interval := range params.Intervals {
		if err := ctx.Err(); err != nil {
			if progress != nil {
				progress.MarkCancelled(time.Now().UTC())
			}
			return err
		}
		if progress != nil {
			progress.BeginInterval(interval, index, time.Now().UTC())
		}
		if err := s.syncInterval(ctx, params, interval, progress); err != nil {
			if progress != nil {
				if errors.Is(err, context.Canceled) {
					progress.MarkCancelled(time.Now().UTC())
				} else {
					progress.MarkFailed(err, time.Now().UTC())
				}
			}
			return fmt.Errorf("sync %s %s from %s: %w", params.Symbol, interval, params.MarketDataProvider, err)
		}
		if progress != nil {
			progress.CompleteInterval(index + 1)
		}
	}
	if progress != nil {
		progress.MarkCompleted(len(params.Intervals), time.Now().UTC())
	}
	return nil
}

func (s *HistoricalKLineSyncer) syncInterval(
	ctx context.Context,
	params KLineSyncParams,
	interval bbgotypes.Interval,
	progress *bt.SyncProgress,
) error {
	before := params.Until.Add(time.Nanosecond)
	seenCursors := make(map[int64]struct{})
	insertedRows := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := fetchHistoricalPageWithRetry(
			ctx, s.source, historicalQuery(params, interval, before), progress,
		)
		if err != nil {
			return err
		}
		rows, err := historicalPageKLines(params.Symbol, interval, params.Since, params.Until, page.Candles)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := s.store.InsertKLines(rows, string(params.RehabType)); err != nil {
				return fmt.Errorf("insert klines: %w", err)
			}
			insertedRows += len(rows)
		}
		if progress != nil {
			progress.IncrementCompletedBatches(time.Now().UTC())
		}
		if !page.HasMore || len(page.Candles) == 0 {
			if insertedRows == 0 {
				return fmt.Errorf("historical provider returned no candles in the requested range")
			}
			return nil
		}
		next := page.NextBefore.UTC()
		if next.IsZero() {
			return fmt.Errorf("historical provider returned hasMore without next cursor")
		}
		if !next.Before(before) {
			return fmt.Errorf("historical provider cursor did not move backward")
		}
		if !next.After(params.Since) {
			if insertedRows == 0 {
				return fmt.Errorf("historical provider returned no candles in the requested range")
			}
			return nil
		}
		if _, duplicate := seenCursors[next.UnixNano()]; duplicate {
			return fmt.Errorf("historical provider repeated cursor")
		}
		seenCursors[next.UnixNano()] = struct{}{}
		before = next
	}
}

func historicalQuery(params KLineSyncParams, interval bbgotypes.Interval, before time.Time) HistoricalCandleQuery {
	return HistoricalCandleQuery{
		Market: params.Market, Symbol: params.Symbol, Interval: string(interval),
		Adjustment: params.RehabType, Sessions: syncSessions(params.SessionScope),
		Limit: 1000, Before: before, Since: params.Since,
	}
}

func fetchHistoricalPageWithRetry(
	ctx context.Context,
	source HistoricalCandleSource,
	query HistoricalCandleQuery,
	progress *bt.SyncProgress,
) (HistoricalCandlePage, error) {
	var page HistoricalCandlePage
	var err error
	for attempt := range 4 {
		page, err = source.FetchHistoricalCandles(ctx, query)
		if err == nil {
			return page, nil
		}
		if ctx.Err() != nil || IsRequestError(err) {
			return HistoricalCandlePage{}, err
		}
		if attempt == 3 {
			break
		}
		if progress != nil {
			progress.IncrementRetries()
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return HistoricalCandlePage{}, ctx.Err()
		case <-timer.C:
		}
	}
	return HistoricalCandlePage{}, err
}

func historicalPageKLines(
	symbol string,
	interval bbgotypes.Interval,
	since, until time.Time,
	candles []HistoricalCandle,
) ([]bbgotypes.KLine, error) {
	rows := make([]bbgotypes.KLine, 0, len(candles))
	for _, candle := range candles {
		if candle.At.Before(since) || !candle.At.Before(until) {
			continue
		}
		open, err := historicalFixedpoint("open", candle.Open)
		if err != nil {
			return nil, err
		}
		high, err := historicalFixedpoint("high", candle.High)
		if err != nil {
			return nil, err
		}
		low, err := historicalFixedpoint("low", candle.Low)
		if err != nil {
			return nil, err
		}
		closeValue, err := historicalFixedpoint("close", candle.Close)
		if err != nil {
			return nil, err
		}
		volumeValue := strings.TrimSpace(candle.Volume)
		if volumeValue == "" {
			volumeValue = "0"
		}
		volume, err := historicalFixedpoint("volume", volumeValue)
		if err != nil {
			return nil, err
		}
		end := candle.At.Add(interval.Duration()).Add(-time.Millisecond)
		rows = append(rows, bbgotypes.KLine{
			Exchange: bbgotypes.ExchangeBacktest, Symbol: symbol, Interval: interval,
			StartTime: bbgotypes.Time(candle.At.UTC()), EndTime: bbgotypes.Time(end.UTC()),
			Open: open, High: high, Low: low, Close: closeValue, Volume: volume, Closed: true,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].StartTime.Time().Before(rows[j].StartTime.Time()) })
	return rows, nil
}

func historicalFixedpoint(field, value string) (bbgofixedpoint.Value, error) {
	result, err := bbgofixedpoint.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("parse historical candle %s: %w", field, err)
	}
	return result, nil
}

func syncSessions(scope string) []string {
	if strings.EqualFold(strings.TrimSpace(scope), "extended") {
		return []string{"regular", "extended", "overnight"}
	}
	return []string{"regular"}
}

func (s *HistoricalKLineSyncer) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if s.store != nil {
			s.closeErr = s.store.Close()
		}
		if s.closeFn != nil {
			s.closeErr = errors.Join(s.closeErr, s.closeFn())
		}
	})
	return s.closeErr
}
