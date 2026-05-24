package jftradeapi

import (
	"context"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/backtest"
)

type backtestRunStore struct {
	mu   sync.RWMutex
	runs map[string]*backtestRunState
}

func cloneBacktestRunState(run *backtestRunState) *backtestRunState {
	if run == nil {
		return nil
	}

	snapshot := *run
	snapshot.Result = run.Result.Snapshot()
	return &snapshot
}

func newBacktestRunStore() *backtestRunStore {
	return &backtestRunStore{runs: make(map[string]*backtestRunState)}
}

func (s *backtestRunStore) add(run *backtestRunState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
}

func (s *backtestRunStore) list() []*backtestRunState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]*backtestRunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, cloneBacktestRunState(run))
	}
	return runs
}

func (s *backtestRunStore) get(runID string) (*backtestRunState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[runID]
	if !ok {
		return nil, false
	}
	return cloneBacktestRunState(run), true
}

func (s *backtestRunStore) update(runID string, mutate func(*backtestRunState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return false
	}
	mutate(run)
	return true
}

type backtestSyncTaskStore struct {
	mu      sync.RWMutex
	tasks   map[string]*backtest.SyncProgress
	cancels map[string]context.CancelFunc
}

func newBacktestSyncTaskStore() *backtestSyncTaskStore {
	return &backtestSyncTaskStore{
		tasks:   make(map[string]*backtest.SyncProgress),
		cancels: make(map[string]context.CancelFunc),
	}
}

func (s *backtestSyncTaskStore) add(taskID string, progress *backtest.SyncProgress, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = progress
	s.cancels[taskID] = cancel
}

func (s *backtestSyncTaskStore) get(taskID string) (*backtest.SyncProgress, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	progress, ok := s.tasks[taskID]
	if !ok {
		return nil, false
	}
	return progress.Snapshot(), true
}

func (s *backtestSyncTaskStore) finish(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, taskID)
}

func (s *backtestSyncTaskStore) cancel(taskID string, cancelledAt time.Time) (*backtest.SyncProgress, bool) {
	s.mu.Lock()
	cancel, ok := s.cancels[taskID]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	delete(s.cancels, taskID)
	progress := s.tasks[taskID]
	s.mu.Unlock()

	cancel()
	if progress != nil {
		progress.MarkCancelled(cancelledAt)
		return progress.Snapshot(), true
	}
	return nil, true
}
