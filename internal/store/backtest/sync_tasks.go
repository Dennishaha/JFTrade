package backtest

import (
	"context"
	"sync"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// SyncTaskStore owns in-process K-line sync progress and cancellation state.
// It directly implements internal/backtest.SyncTaskStore.
type SyncTaskStore struct {
	mu      sync.RWMutex
	tasks   map[string]*bt.SyncProgress
	cancels map[string]context.CancelFunc
}

// NewSyncTaskStore returns the in-process task registry through the service
// and maintenance ports that consume it.
func NewSyncTaskStore() SyncTaskResource {
	return &SyncTaskStore{
		tasks:   make(map[string]*bt.SyncProgress),
		cancels: make(map[string]context.CancelFunc),
	}
}

func (s *SyncTaskStore) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = progress
	s.cancels[taskID] = cancel
}

func (s *SyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	progress, ok := s.tasks[taskID]
	if !ok || progress == nil {
		return nil, ok
	}
	return progress.Snapshot(), true
}

func (s *SyncTaskStore) Finish(taskID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, taskID)
}

func (s *SyncTaskStore) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	cancel, ok := s.cancels[taskID]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	delete(s.cancels, taskID)
	progress := s.tasks[taskID]
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if progress != nil {
		progress.MarkCancelled(cancelledAt)
		return progress.Snapshot(), true
	}
	return nil, true
}

// MaintenanceBusyReason reports active synchronization cancellation hooks.
func (s *SyncTaskStore) MaintenanceBusyReason(context.Context) string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cancels) > 0 {
		return "存在正在运行的行情同步"
	}
	return ""
}
