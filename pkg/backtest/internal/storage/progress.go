package storage

import (
	"sync"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// SyncProgress tracks the progress of a K-line sync operation for frontend polling.
type SyncProgress struct {
	mu                 sync.RWMutex
	TaskID             string `json:"taskId"`
	Status             string `json:"status"`
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
