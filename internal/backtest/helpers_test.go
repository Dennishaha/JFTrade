package backtest

import (
	"context"
	"sync"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const testPineScript = `//@version=6
strategy("Service Test", overlay=true, initial_capital=25000)
strategy.entry("Long", strategy.long, qty=1)`

func validStartRequest() StartRequest {
	return StartRequest{
		DefinitionID: "def-1",
		Market:       "US",
		Code:         "AAPL",
		StartTime:    "2024-01-02T00:00:00Z",
		EndTime:      "2024-01-03T00:00:00Z",
	}
}

func withStartTime(req StartRequest, start string) StartRequest {
	req.StartTime = start
	return req
}

func withEndTime(req StartRequest, end string) StartRequest {
	req.EndTime = end
	return req
}

func newTestBacktestService(runs RunStore, runner func(context.Context, bt.RunConfig) *bt.RunResult) *Service {
	return NewService(
		WithRunStore(runs),
		WithStrategyProvider(fakeStrategyProvider{defs: map[string]StrategyDef{
			"def-1": {
				ID:           "def-1",
				Version:      "v1",
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       testPineScript,
			},
		}}),
		WithRunBacktestFn(runner),
	)
}

func waitForRunStatus(t *testing.T, runs *memoryRunStore, runID string, want string) *RunState {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			run, _, jftradeErr1 := runs.GetFull(runID)
			jftradeCheckTestError(t, jftradeErr1)
			t.Fatalf("timed out waiting for run %s status %q; latest = %#v", runID, want, run)
		case <-ticker.C:
			run, ok, err := runs.GetFull(runID)
			if err != nil {
				t.Fatalf("GetFull() error = %v", err)
			}
			if ok && run.Status == want {
				return run
			}
		}
	}
}

func waitForSyncFinished(t *testing.T, tasks *memorySyncTaskStore, taskID string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sync task %s to finish", taskID)
		case <-ticker.C:
			if tasks.isFinished(taskID) {
				return
			}
		}
	}
}

type fakeStrategyProvider struct {
	defs map[string]StrategyDef
	err  error
}

func (p fakeStrategyProvider) Definition(id string) (StrategyDef, bool, error) {
	if p.err != nil {
		return StrategyDef{}, false, p.err
	}
	def, ok := p.defs[id]
	return def, ok, nil
}

type memoryRunStore struct {
	mu        sync.Mutex
	runs      map[string]*RunState
	cancels   map[string]context.CancelFunc
	updateErr error
}

func newMemoryRunStore() *memoryRunStore {
	return &memoryRunStore{
		runs:    map[string]*RunState{},
		cancels: map[string]context.CancelFunc{},
	}
}

func (s *memoryRunStore) Add(run *RunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *memoryRunStore) Get(runID string) (*RunState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false
	}
	clone := cloneRun(run)
	clone.Result = nil
	return clone, true
}

func (s *memoryRunStore) GetFull(runID string) (*RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	return cloneRun(run), true, nil
}

func (s *memoryRunStore) List() []*RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]*RunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, cloneRun(run))
	}
	return runs
}

func (s *memoryRunStore) ListLightweight() []*RunState {
	runs := s.List()
	for _, run := range runs {
		run.Result = nil
	}
	return runs
}

func (s *memoryRunStore) Update(runID string, mutate func(*RunState)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false, nil
	}
	if s.updateErr != nil {
		return true, s.updateErr
	}
	mutate(run)
	return true, nil
}

func (s *memoryRunStore) UpdateMemoryOnly(runID string, mutate func(*RunState)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return false
	}
	mutate(run)
	return true
}

func (s *memoryRunStore) Delete(runID string) (*RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	delete(s.runs, runID)
	delete(s.cancels, runID)
	return cloneRun(run), true, nil
}

func (s *memoryRunStore) SetCancel(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel == nil {
		delete(s.cancels, runID)
		return
	}
	s.cancels[runID] = cancel
}

func (s *memoryRunStore) Cancel(runID string) bool {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if !ok || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *memoryRunStore) Close() error {
	return nil
}

func cloneRun(run *RunState) *RunState {
	if run == nil {
		return nil
	}
	clone := *run
	if run.Result != nil {
		clone.Result = run.Result.Snapshot()
	}
	return &clone
}

type memorySyncTaskStore struct {
	mu       sync.Mutex
	tasks    map[string]*bt.SyncProgress
	cancels  map[string]context.CancelFunc
	finished map[string]bool
}

func (s *memorySyncTaskStore) isFinished(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished[taskID]
}

func newMemorySyncTaskStore() *memorySyncTaskStore {
	return &memorySyncTaskStore{
		tasks:    map[string]*bt.SyncProgress{},
		cancels:  map[string]context.CancelFunc{},
		finished: map[string]bool{},
	}
}

func (s *memorySyncTaskStore) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = progress
	s.cancels[taskID] = cancel
}

func (s *memorySyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	progress, ok := s.tasks[taskID]
	if !ok {
		return nil, false
	}
	return progress.Snapshot(), true
}

func (s *memorySyncTaskStore) Finish(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished[taskID] = true
	delete(s.cancels, taskID)
}

func (s *memorySyncTaskStore) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	s.mu.Lock()
	progress, ok := s.tasks[taskID]
	cancel := s.cancels[taskID]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	if cancel != nil {
		cancel()
	}
	progress.MarkCancelled(cancelledAt)
	return progress.Snapshot(), true
}

type fakeKLineSyncer struct {
	mu            sync.Mutex
	params        KLineSyncParams
	err           error
	closed        bool
	done          chan struct{}
	started       chan struct{}
	waitForCancel bool
	fields        observability.Fields
}

func (s *fakeKLineSyncer) Sync(ctx context.Context, params KLineSyncParams, progress *bt.SyncProgress) error {
	s.mu.Lock()
	s.params = params
	s.fields = observability.FieldsFromContext(ctx)
	err := s.err
	done := s.done
	started := s.started
	waitForCancel := s.waitForCancel
	s.mu.Unlock()
	if started != nil {
		close(started)
	}
	if waitForCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	if err == nil {
		progress.MarkCompleted(len(params.Intervals), time.Now().UTC())
	}
	if done != nil {
		close(done)
	}
	return err
}

func (s *fakeKLineSyncer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
