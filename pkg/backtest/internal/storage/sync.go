package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/internal/retry"
	"github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

var (
	syncBatchPause      = 2 * time.Second
	syncBatchSize       = 100000
	syncRetryBaseDelay  = 5 * time.Second
	syncRetryMaxDelay   = 20 * time.Second
	syncRetryMaxRetries = 3
)

func syncRetryConfig(progress *SyncProgress) retry.Config {
	return retry.Config{
		BaseDelay:   syncRetryBaseDelay,
		MaxDelay:    syncRetryMaxDelay,
		MaxRetries:  syncRetryMaxRetries,
		ShouldRetry: retry.FutuRateLimitShouldRetry,
		Notify: func(attempt int, err error, delay time.Duration) {
			if progress != nil {
				progress.IncrementRetries()
			}
		},
	}
}

// SyncKLines pulls historical K-lines from Futu OpenD and stores them in the
// SQLite backtest store. It syncs one symbol at a time, iterating through
// the requested intervals, and paginates via the OpenD RequestHistoryKL
// protocol (3103).
func (s *FutuKLineStore) SyncKLines(
	ctx context.Context,
	exchange *futu.Exchange,
	symbol string,
	intervals []bbgotypes.Interval,
	since, until time.Time,
	rehabType qotcommonpb.RehabType,
	sessionScope string,
	progress *SyncProgress,
) error {
	sessionScope = normalizeKLineSessionScopeName(sessionScope)
	if progress != nil {
		progress.SetRunning(len(intervals), time.Now().UTC())
	}

	for i, interval := range intervals {
		select {
		case <-ctx.Done():
			if progress != nil {
				progress.MarkCancelled(time.Now().UTC())
			}
			return ctx.Err()
		default:
		}
		if progress != nil {
			progress.BeginInterval(interval, i, time.Now().UTC())
		}
		if err := s.syncInterval(ctx, exchange, symbol, interval, since, until, rehabType, sessionScope, progress); err != nil {
			if progress != nil {
				if errors.Is(err, context.Canceled) {
					progress.MarkCancelled(time.Now().UTC())
				} else {
					progress.MarkFailed(err, time.Now().UTC())
				}
			}
			return fmt.Errorf("sync %s %s %s: %w", symbol, interval, since, err)
		}
		if progress != nil {
			progress.CompleteInterval(i + 1)
		}
	}
	if progress != nil {
		progress.MarkCompleted(len(intervals), time.Now().UTC())
	}
	return nil
}

func (s *FutuKLineStore) syncInterval(
	ctx context.Context,
	exchange *futu.Exchange,
	symbol string,
	interval bbgotypes.Interval,
	since, until time.Time,
	rehabType qotcommonpb.RehabType,
	sessionScope string,
	progress *SyncProgress,
) error {
	writeSessionScope := syncWriteSessionScope(symbol, interval, sessionScope)
	s.SetWriteSessionScope(writeSessionScope)

	// Always start from the requested start time.  Instead of jumping the
	// cursor to the "latest stored kline" (which would skip older gaps when
	// the DB already contains newer data), we check each batch against the
	// local store and only call OpenD for batches that are not yet covered.
	cursor := since

	// 新版opend不限制请求返回数量，一批可以返回超过1000条数据。
	batchSize := syncBatchSize
	for cursor.Before(until) {
		select {
		case <-ctx.Done():
			if progress != nil {
				progress.MarkCancelled(time.Now().UTC())
			}
			return ctx.Err()
		default:
		}
		batchEnd := cursor.Add(interval.Duration() * time.Duration(batchSize))
		if batchEnd.After(until) {
			batchEnd = until
		}

		// Skip batches that are already fully stored locally.
		covered, err := s.isBatchCovered(symbol, interval, cursor, batchEnd, RehabTypeName(int32(rehabType)))
		if err != nil {
			return fmt.Errorf("check batch coverage: %w", err)
		}
		if covered {
			cursor = batchEnd
			if progress != nil {
				progress.IncrementCompletedBatches(time.Now().UTC())
			}
			continue
		}

		var klines []bbgotypes.KLine
		queryEnd := syncHistoryRequestEndTime(interval, batchEnd)
		queryErr := retry.Do(func() error {
			var innerErr error
			// QueryAllKLines does not set MaxAckKLNum — OpenD returns all data in the range
			// using its internal page size. nextReqKey is followed until empty.
			klines, innerErr = exchange.QueryAllKLines(ctx, symbol, interval, cursor, queryEnd, rehabType)
			return innerErr
		}, syncRetryConfig(progress))
		if queryErr != nil {
			return fmt.Errorf("query klines %s %s [%s, %s]: %w", symbol, interval, cursor, queryEnd, queryErr)
		}

		if len(klines) == 0 {
			// Advance cursor to avoid infinite loop
			cursor = batchEnd
			continue
		}

		klines = filterSyncedKLinesBySessionScope(symbol, interval, writeSessionScope, klines)
		if len(klines) == 0 {
			cursor = batchEnd
			continue
		}

		if err := s.InsertKLines(klines, RehabTypeName(int32(rehabType))); err != nil {
			return fmt.Errorf("insert klines: %w", err)
		}

		// Advance cursor past the last kline's end time
		lastEnd := klines[len(klines)-1].EndTime.Time()
		if !lastEnd.After(cursor) {
			cursor = batchEnd
		} else {
			cursor = lastEnd
		}
		// Each QueryAllKLines call triggers RequestHistoryKL RPCs internally.
		// Sleep 2s to stay under OpenD rate limit (60req/30s).
		if progress != nil {
			progress.IncrementCompletedBatches(time.Now().UTC())
		}
		if syncBatchPause > 0 {
			time.Sleep(syncBatchPause)
		}
	}
	return nil
}

func syncHistoryRequestEndTime(interval bbgotypes.Interval, requestedEnd time.Time) time.Time {
	normalizedEnd := requestedEnd.UTC()
	if interval.Duration() <= 0 {
		return normalizedEnd
	}

	closedEnd := latestClosedKLineEndAtOrBefore(normalizedEnd, interval)
	if interval.Duration() < 24*time.Hour {
		return closedEnd.Add(time.Millisecond)
	}
	return closedEnd
}

func syncWriteSessionScope(symbol string, interval bbgotypes.Interval, requestedScope string) string {
	scope := normalizeKLineSessionScopeName(requestedScope)
	if scope != klineSessionScopeExtended {
		return scope
	}
	if market.IsUSSymbol(symbol) && interval.Duration() <= time.Hour {
		return klineSessionScopeExtended
	}
	return klineSessionScopeRegular
}

func filterSyncedKLinesBySessionScope(symbol string, interval bbgotypes.Interval, sessionScope string, klines []bbgotypes.KLine) []bbgotypes.KLine {
	if normalizeKLineSessionScopeName(sessionScope) != klineSessionScopeRegular || interval.Duration() > time.Hour {
		return klines
	}
	filtered := klines[:0]
	for _, kline := range klines {
		if market.IsRegularTradingTime(symbol, kline.StartTime.Time()) {
			filtered = append(filtered, kline)
		}
	}
	return filtered
}
