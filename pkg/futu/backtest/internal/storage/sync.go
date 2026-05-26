package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

var (
	syncBatchPause      = 2 * time.Second
	syncBatchSize       = 100000
	syncRetryMaxRetries = 3
	syncRetryBaseWait   = 5 * time.Second
)

// rateLimitRetry checks if an error is an OpenD rate-limit error and
// retries the operation with exponential backoff (up to maxRetries times).
func rateLimitRetry(operation func() error, progress *SyncProgress) error {
	var lastErr error
	for attempt := 0; attempt <= syncRetryMaxRetries; attempt++ {
		if attempt > 0 {
			if progress != nil {
				progress.IncrementRetries()
			}
			wait := syncRetryBaseWait * time.Duration(1<<(attempt-1)) // 5s, 10s, 20s
			// Sleep in small increments so cancellation is responsive.
			deadline := time.Now().Add(wait)
			for time.Now().Before(deadline) {
				time.Sleep(500 * time.Millisecond)
			}
		}
		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		// Only retry on rate-limit errors (retType=-1) or timeout-like messages.
		errStr := lastErr.Error()
		if !strings.Contains(errStr, "频率太高") && !strings.Contains(errStr, "retType=-1") {
			return lastErr
		}
	}
	return fmt.Errorf("rate-limit retry exhausted after %d attempts: %w", syncRetryMaxRetries, lastErr)
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
	progress *SyncProgress,
) error {
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
		if err := s.syncInterval(ctx, exchange, symbol, interval, since, until, rehabType, progress); err != nil {
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
	progress *SyncProgress,
) error {
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
		if covered, _ := s.isBatchCovered(symbol, interval, cursor, batchEnd, RehabTypeName(int32(rehabType))); covered {
			cursor = batchEnd
			if progress != nil {
				progress.IncrementCompletedBatches(time.Now().UTC())
			}
			continue
		}

		var klines []bbgotypes.KLine
		queryErr := rateLimitRetry(func() error {
			var innerErr error
			// QueryAllKLines does not set MaxAckKLNum — OpenD returns all data in the range
			// using its internal page size. nextReqKey is followed until empty.
			klines, innerErr = exchange.QueryAllKLines(ctx, symbol, interval, cursor, batchEnd, rehabType)
			return innerErr
		}, progress)
		if queryErr != nil {
			return fmt.Errorf("query klines %s %s [%s, %s]: %w", symbol, interval, cursor, batchEnd, queryErr)
		}

		if len(klines) == 0 {
			// Advance cursor to avoid infinite loop
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
