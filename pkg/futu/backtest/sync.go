package backtest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// SyncProgress tracks the progress of a K-line sync operation for frontend polling.
type SyncProgress struct {
	mu                 sync.RWMutex
	TaskID             string `json:"taskId"`
	Status             string `json:"status"` // "running", "completed", "failed"
	Symbol             string `json:"symbol"`
	CurrentInterval    string `json:"currentInterval"`
	TotalIntervals     int    `json:"totalIntervals"`
	CompletedIntervals int    `json:"completedIntervals"`
	TotalBatches       int    `json:"totalBatches"`
	CompletedBatches   int    `json:"completedBatches"`
	Retries            int    `json:"retries"`
	Error              string `json:"error,omitempty"`
	StartedAt          string `json:"startedAt"`
	UpdatedAt          string `json:"updatedAt"`
}

func NewSyncProgress(taskID string, symbol string, queuedAt time.Time) *SyncProgress {
	return &SyncProgress{
		TaskID:    taskID,
		Status:    "queued",
		Symbol:    symbol,
		StartedAt: queuedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (p *SyncProgress) Snapshot() *SyncProgress {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	return &SyncProgress{
		TaskID:             p.TaskID,
		Status:             p.Status,
		Symbol:             p.Symbol,
		CurrentInterval:    p.CurrentInterval,
		TotalIntervals:     p.TotalIntervals,
		CompletedIntervals: p.CompletedIntervals,
		TotalBatches:       p.TotalBatches,
		CompletedBatches:   p.CompletedBatches,
		Retries:            p.Retries,
		Error:              p.Error,
		StartedAt:          p.StartedAt,
		UpdatedAt:          p.UpdatedAt,
	}
}

func (p *SyncProgress) SetRunning(totalIntervals int, startedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.TotalIntervals = totalIntervals
	p.Status = "running"
	p.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
}

func (p *SyncProgress) BeginInterval(interval bbgotypes.Interval, completedIntervals int, updatedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.CurrentInterval = string(interval)
	p.CompletedIntervals = completedIntervals
	p.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
}

func (p *SyncProgress) CompleteInterval(completedIntervals int) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.CompletedIntervals = completedIntervals
}

func (p *SyncProgress) IncrementCompletedBatches(updatedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.CompletedBatches++
	p.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
}

func (p *SyncProgress) IncrementRetries() {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Retries++
}

func (p *SyncProgress) MarkFailed(err error, updatedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = "failed"
	if err != nil {
		p.Error = err.Error()
	} else {
		p.Error = ""
	}
	p.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
}

func (p *SyncProgress) MarkCancelled(updatedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = "cancelled"
	p.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
}

func (p *SyncProgress) MarkCompleted(totalIntervals int, updatedAt time.Time) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = "completed"
	p.CompletedIntervals = totalIntervals
	p.CurrentInterval = ""
	p.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
}

// rateLimitRetry checks if an error is an OpenD rate-limit error and
// retries the operation with exponential backoff (up to maxRetries times).
func rateLimitRetry(operation func() error, progress *SyncProgress) error {
	const maxRetries = 3
	const baseWait = 5 * time.Second
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			progress.IncrementRetries()
			wait := baseWait * time.Duration(1<<(attempt-1)) // 5s, 10s, 20s
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
	return fmt.Errorf("rate-limit retry exhausted after %d attempts: %w", maxRetries, lastErr)
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
	market, _ := splitMarketSymbol(symbol)
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
		if err := s.syncInterval(ctx, exchange, symbol, market, interval, since, until, rehabType, progress); err != nil {
			if progress != nil {
				progress.MarkFailed(err, time.Now().UTC())
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
	symbol, market string,
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
	batchSize := 100000
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

		if err := s.InsertKLines(klines, market, RehabTypeName(int32(rehabType))); err != nil {
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
		time.Sleep(2 * time.Second)
	}
	return nil
}

// splitMarketSymbol splits "HK.00700" into ("HK", "00700").
func splitMarketSymbol(instrumentID string) (market, symbol string) {
	parts := strings.SplitN(strings.ToUpper(strings.TrimSpace(instrumentID)), ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", instrumentID
}
