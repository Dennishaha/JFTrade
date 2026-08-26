package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	apibacktest "github.com/jftrade/jftrade-main/internal/api/backtest"
	srvbacktest "github.com/jftrade/jftrade-main/internal/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const (
	stage9BacktestsWriteFixtureVersion = "stage9.backtests-write.v1"
	stage9BacktestsWriteTimestamp      = "2026-08-23T04:00:00Z"
)

type stage9BacktestsWriteFixture struct {
	Version   string                            `json:"version"`
	Timestamp string                            `json:"timestamp"`
	Cases     []stage9BacktestsWriteFixtureCase `json:"cases"`
}

type stage9BacktestsWriteFixtureCase struct {
	Name              string                               `json:"name"`
	Requests          []stage9BacktestsWriteFixtureRequest `json:"requests"`
	PortMode          string                               `json:"portMode"`
	RestartAfterFirst bool                                 `json:"restartAfterFirst,omitempty"`
	Expected          []stage9BacktestsWriteExpected       `json:"expected"`
	Calls             []map[string]any                     `json:"calls,omitempty"`
	Effects           stage9BacktestsWriteEffects          `json:"effects"`
}

type stage9BacktestsWriteFixtureRequest struct {
	Method      string  `json:"method"`
	RequestPath string  `json:"requestPath"`
	Body        *string `json:"body,omitempty"`
	Context     string  `json:"context,omitempty"`
}

type stage9BacktestsWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	PortCall bool              `json:"portCall"`
	Envelope map[string]any    `json:"envelope"`
}

type stage9BacktestsWriteEffects struct {
	StrategyLookups  int `json:"strategyLookups"`
	RunAdds          int `json:"runAdds"`
	SyncAdapterOpens int `json:"syncAdapterOpens"`
	SyncTaskAdds     int `json:"syncTaskAdds"`
	SyncCancels      int `json:"syncCancels"`
	RunStatusReads   int `json:"runStatusReads"`
	RunDeletes       int `json:"runDeletes"`
}

// TestStage9BacktestsWriteFixtureMatchesCurrentGoOwner freezes all four
// backtest mutation routes. The fake collaborators record the Go service's
// side-effect boundary without starting PineTS, acquiring market data, or
// opening the production run database.
func TestStage9BacktestsWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 backtests-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/backtests-write.json",
	)
	want := stage9BacktestsWriteFixture{
		Version:   stage9BacktestsWriteFixtureVersion,
		Timestamp: stage9BacktestsWriteTimestamp,
		Cases:     make([]stage9BacktestsWriteFixtureCase, 0, len(stage9BacktestsWriteCases())),
	}
	for _, spec := range stage9BacktestsWriteCases() {
		state := newStage9BacktestsWriteState(spec.PortMode)
		router, service := stage9BacktestsWriteRouter(state)
		expected := make([]stage9BacktestsWriteExpected, 0, len(spec.Requests))
		calls := make([]map[string]any, 0, len(spec.Requests))
		for index, request := range spec.Requests {
			response := stage9BacktestsWriteRequest(t, router, request)
			if response.Code == 0 {
				t.Fatalf("case %s request %d did not produce a response", spec.Name, index)
			}
			var envelope map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("case %s decode response: %v", spec.Name, err)
			}
			stage9NormalizeBacktestsWriteEnvelope(t, spec.Name, envelope)
			expected = append(expected, stage9BacktestsWriteExpected{
				Status:   response.Code,
				Headers:  stage9BacktestsWriteHeaders(response),
				PortCall: stage9BacktestsWritePortCall(request),
				Envelope: envelope,
			})
			if call := stage9BacktestsWriteDelegation(request); call != nil {
				calls = append(calls, call)
			}
			if spec.RestartAfterFirst && index == 0 {
				if err := service.Close(); err != nil {
					t.Fatalf("case %s close before restart: %v", spec.Name, err)
				}
				router, service = stage9BacktestsWriteRouter(state)
			}
		}
		if err := service.Close(); err != nil {
			t.Fatalf("case %s close service: %v", spec.Name, err)
		}
		if len(calls) == 0 {
			calls = nil
		}
		want.Cases = append(want.Cases, stage9BacktestsWriteFixtureCase{
			Name:              spec.Name,
			Requests:          spec.Requests,
			PortMode:          spec.PortMode,
			RestartAfterFirst: spec.RestartAfterFirst,
			Expected:          expected,
			Calls:             calls,
			Effects:           state.trace.effects(),
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode backtests-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write backtests-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read backtests-write fixture: %v", err)
	}
	var got stage9BacktestsWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode backtests-write fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 backtests-write fixture drifted from the Go owner")
	}
}

type stage9BacktestsWriteCaseSpec struct {
	Name              string
	Requests          []stage9BacktestsWriteFixtureRequest
	PortMode          string
	RestartAfterFirst bool
}

func stage9BacktestsWriteCases() []stage9BacktestsWriteCaseSpec {
	body := func(value string) *string { return &value }
	post := func(path, value string) stage9BacktestsWriteFixtureRequest {
		return stage9BacktestsWriteFixtureRequest{
			Method: http.MethodPost, RequestPath: path, Body: body(value),
		}
	}
	deleteRequest := func(path string) stage9BacktestsWriteFixtureRequest {
		return stage9BacktestsWriteFixtureRequest{Method: http.MethodDelete, RequestPath: path}
	}
	validStart := stage9BacktestsWriteValidStartBody()
	validSync := stage9BacktestsWriteValidSyncBody()
	return []stage9BacktestsWriteCaseSpec{
		{Name: "start-success", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart)}},
		{Name: "start-trailing-json-is-ignored", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart+` {"ignored":true}`)}},
		{Name: "start-null-body", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", "null")}},
		{Name: "start-empty-body", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{{Method: http.MethodPost, RequestPath: "/api/v1/backtests"}}},
		{Name: "start-malformed-body", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", "{")}},
		{Name: "start-array-body", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", "[]")}},
		{Name: "start-invalid-instrument", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", `{"definitionId":"def-1","symbol":"bad symbol","startTime":"2024-01-02T00:00:00Z","endTime":"2024-01-03T00:00:00Z"}`)}},
		{Name: "start-strategy-missing", PortMode: "start-strategy-missing", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart)}},
		{Name: "start-strategy-provider-failure", PortMode: "start-strategy-error", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart)}},
		{Name: "start-run-store-failure", PortMode: "start-run-store-error", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart)}},
		{Name: "start-canceled-context-still-queues", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{{Method: http.MethodPost, RequestPath: "/api/v1/backtests", Body: body(validStart), Context: "canceled"}}},
		{Name: "start-repeated-write-is-not-idempotent", PortMode: "start-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests", validStart), post("/api/v1/backtests", validStart)}},
		{Name: "sync-success", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", validSync)}},
		{Name: "sync-default-intervals", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", `{"market":"US","code":"AAPL","since":"2024-01-02T00:00:00Z","until":"2024-01-03T00:00:00Z"}`)}},
		{Name: "sync-null-body", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", "null")}},
		{Name: "sync-empty-body", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{{Method: http.MethodPost, RequestPath: "/api/v1/backtests/sync"}}},
		{Name: "sync-malformed-body", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", "{")}},
		{Name: "sync-invalid-session-scope", PortMode: "sync-success", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", `{"market":"US","code":"AAPL","intervals":["1d"],"since":"2024-01-02T00:00:00Z","until":"2024-01-03T00:00:00Z","sessionScope":"legacy"}`)}},
		{Name: "sync-adapter-failure", PortMode: "sync-adapter-error", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", validSync)}},
		{Name: "sync-adapter-canceled", PortMode: "sync-adapter-canceled", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", validSync)}},
		{Name: "sync-adapter-deadline", PortMode: "sync-adapter-deadline", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", validSync)}},
		{Name: "sync-task-store-failure", PortMode: "sync-task-store-error", Requests: []stage9BacktestsWriteFixtureRequest{post("/api/v1/backtests/sync", validSync)}},
		{Name: "cancel-success", PortMode: "cancel-success", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/sync/fixture-task")}},
		{Name: "cancel-missing", PortMode: "cancel-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/sync/missing-task")}},
		{Name: "cancel-repeat-is-not-idempotent", PortMode: "cancel-repeat", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/sync/fixture-task"), deleteRequest("/api/v1/backtests/sync/fixture-task")}},
		{Name: "cancel-blank-id", PortMode: "cancel-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/sync/%20")}},
		{Name: "cancel-invalid-escape", PortMode: "cancel-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/sync/%zz")}},
		{Name: "delete-completed-success", PortMode: "delete-completed", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-failed-success", PortMode: "delete-failed", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-cancelled-success", PortMode: "delete-cancelled", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-running-rejected", PortMode: "delete-running", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-missing", PortMode: "delete-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/missing-run")}},
		{Name: "delete-store-failure", PortMode: "delete-store-error", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-status-delete-race", PortMode: "delete-race", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-failure-recovers-after-restart", PortMode: "delete-recovery", RestartAfterFirst: true, Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run"), deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-repeated-write-is-not-idempotent", PortMode: "delete-completed", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/fixture-run"), deleteRequest("/api/v1/backtests/fixture-run")}},
		{Name: "delete-blank-id", PortMode: "delete-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/%20")}},
		{Name: "delete-invalid-escape", PortMode: "delete-missing", Requests: []stage9BacktestsWriteFixtureRequest{deleteRequest("/api/v1/backtests/%zz")}},
	}
}

func stage9BacktestsWriteValidStartBody() string {
	return `{"definitionId":"def-1","market":"US","code":"AAPL","interval":"1d","startTime":"2024-01-02T00:00:00Z","endTime":"2024-01-03T00:00:00Z"}`
}

func stage9BacktestsWriteValidSyncBody() string {
	return `{"market":"US","code":"AAPL","intervals":["1d"],"since":"2024-01-02T00:00:00Z","until":"2024-01-03T00:00:00Z"}`
}

type stage9BacktestsWriteState struct {
	mode      string
	runs      *stage9BacktestsWriteRunStore
	syncTasks *stage9BacktestsWriteSyncTaskStore
	trace     *stage9BacktestsWriteTrace
}

func newStage9BacktestsWriteState(mode string) *stage9BacktestsWriteState {
	state := &stage9BacktestsWriteState{
		mode:      mode,
		runs:      newStage9BacktestsWriteRunStore(),
		syncTasks: newStage9BacktestsWriteSyncTaskStore(),
		trace:     &stage9BacktestsWriteTrace{},
	}
	state.runs.trace = state.trace
	state.syncTasks.trace = state.trace
	switch mode {
	case "start-run-store-error":
		state.runs.addErr = errors.New("run store unavailable")
	case "cancel-success", "cancel-repeat":
		state.syncTasks.seedCancelable("fixture-task")
	case "cancel-finished":
		state.syncTasks.seedFinished("fixture-task")
	case "delete-completed", "delete-race", "delete-store-error":
		state.runs.seed("fixture-run", "completed")
	case "delete-failed":
		state.runs.seed("fixture-run", "failed")
	case "delete-cancelled":
		state.runs.seed("fixture-run", "cancelled")
	case "delete-running":
		state.runs.seed("fixture-run", "running")
	case "delete-recovery":
		state.runs.seed("fixture-run", "completed")
		state.runs.deleteErrs = []error{errors.New("delete transaction failed"), nil}
	}
	if mode == "delete-race" {
		state.runs.deleteRace = true
	}
	if mode == "delete-store-error" {
		state.runs.deleteErrs = []error{errors.New("delete transaction failed")}
	}
	return state
}

func stage9BacktestsWriteRouter(state *stage9BacktestsWriteState) (*gin.Engine, *srvbacktest.Service) {
	opts := []srvbacktest.Option{
		srvbacktest.WithRunStore(state.runs),
		srvbacktest.WithStrategyProvider(stage9BacktestsWriteStrategyProvider{mode: state.mode, trace: state.trace}),
		srvbacktest.WithRunBacktestFn(func(context.Context, bt.RunConfig) *bt.RunResult {
			return &bt.RunResult{}
		}),
	}
	if state.mode != "sync-task-store-error" {
		opts = append(opts, srvbacktest.WithSyncTaskStore(state.syncTasks))
	}
	opts = append(opts, srvbacktest.WithNewKLineSyncerFn(func(string) (srvbacktest.KLineSyncer, error) {
		state.trace.syncAdapterOpens++
		switch state.mode {
		case "sync-adapter-error":
			return nil, errors.New("sqlite unavailable")
		case "sync-adapter-canceled":
			return nil, context.Canceled
		case "sync-adapter-deadline":
			return nil, context.DeadlineExceeded
		default:
			return stage9BacktestsWriteBlockingSyncer{}, nil
		}
	}))
	service := srvbacktest.NewService(opts...)
	router := gin.New()
	apibacktest.RegisterRoutes(router.Group("/api/v1"), service)
	return router, service
}

type stage9BacktestsWriteStrategyProvider struct {
	mode  string
	trace *stage9BacktestsWriteTrace
}

func (p stage9BacktestsWriteStrategyProvider) Definition(string) (srvbacktest.StrategyDef, bool, error) {
	p.trace.strategyLookups++
	switch p.mode {
	case "start-strategy-missing":
		return srvbacktest.StrategyDef{}, false, nil
	case "start-strategy-error":
		return srvbacktest.StrategyDef{}, false, errors.New("strategy database unavailable")
	default:
		return srvbacktest.StrategyDef{
			ID:           "def-1",
			Version:      "v1",
			SourceFormat: strategydefinition.SourceFormatPineV6,
			Script: `//@version=6
strategy("Stage 9 fixture", overlay=true)
strategy.entry("Long", strategy.long, qty=1)`,
		}, true, nil
	}
}

type stage9BacktestsWriteBlockingSyncer struct{}

func (stage9BacktestsWriteBlockingSyncer) Sync(ctx context.Context, _ srvbacktest.KLineSyncParams, _ *bt.SyncProgress) error {
	<-ctx.Done()
	return ctx.Err()
}

func (stage9BacktestsWriteBlockingSyncer) Close() error { return nil }

type stage9BacktestsWriteRunStore struct {
	mu         sync.Mutex
	runs       map[string]*srvbacktest.RunState
	trace      *stage9BacktestsWriteTrace
	addErr     error
	deleteErrs []error
	deleteRace bool
}

func newStage9BacktestsWriteRunStore() *stage9BacktestsWriteRunStore {
	return &stage9BacktestsWriteRunStore{runs: make(map[string]*srvbacktest.RunState)}
}

func (s *stage9BacktestsWriteRunStore) seed(id, status string) {
	s.runs[id] = &srvbacktest.RunState{ID: id, Status: status}
}

func (s *stage9BacktestsWriteRunStore) Add(run *srvbacktest.RunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		s.trace.runAdds++
	}
	if s.addErr != nil {
		return s.addErr
	}
	s.runs[run.ID] = cloneStage9BacktestsWriteRun(run)
	return nil
}

func (s *stage9BacktestsWriteRunStore) Get(runID string) (*srvbacktest.RunState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		s.trace.runStatusReads++
	}
	return cloneStage9BacktestsWriteRun(s.runs[runID]), s.runs[runID] != nil
}

func (s *stage9BacktestsWriteRunStore) GetFull(runID string) (*srvbacktest.RunState, bool, error) {
	run, ok := s.Get(runID)
	return run, ok, nil
}

func (s *stage9BacktestsWriteRunStore) List() []*srvbacktest.RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]*srvbacktest.RunState, 0, len(s.runs))
	for _, run := range s.runs {
		runs = append(runs, cloneStage9BacktestsWriteRun(run))
	}
	return runs
}

func (s *stage9BacktestsWriteRunStore) ListLightweight() []*srvbacktest.RunState {
	runs := s.List()
	for _, run := range runs {
		run.Result = nil
	}
	return runs
}

func (s *stage9BacktestsWriteRunStore) Update(string, func(*srvbacktest.RunState)) (bool, error) {
	return false, nil
}

func (s *stage9BacktestsWriteRunStore) UpdateMemoryOnly(string, func(*srvbacktest.RunState)) bool {
	return false
}

func (s *stage9BacktestsWriteRunStore) Delete(runID string) (*srvbacktest.RunState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		s.trace.runDeletes++
	}
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	if len(s.deleteErrs) > 0 {
		err := s.deleteErrs[0]
		s.deleteErrs = s.deleteErrs[1:]
		if err != nil {
			return nil, true, err
		}
	}
	if s.deleteRace {
		return nil, false, nil
	}
	delete(s.runs, runID)
	return cloneStage9BacktestsWriteRun(run), true, nil
}

func (s *stage9BacktestsWriteRunStore) SetCancel(string, context.CancelFunc) {}
func (s *stage9BacktestsWriteRunStore) Cancel(string) bool                   { return false }
func (s *stage9BacktestsWriteRunStore) Close() error                         { return nil }

func cloneStage9BacktestsWriteRun(run *srvbacktest.RunState) *srvbacktest.RunState {
	if run == nil {
		return nil
	}
	clone := *run
	return &clone
}

type stage9BacktestsWriteSyncTaskStore struct {
	mu       sync.Mutex
	progress map[string]*bt.SyncProgress
	cancels  map[string]context.CancelFunc
	trace    *stage9BacktestsWriteTrace
}

func newStage9BacktestsWriteSyncTaskStore() *stage9BacktestsWriteSyncTaskStore {
	return &stage9BacktestsWriteSyncTaskStore{
		progress: make(map[string]*bt.SyncProgress),
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (s *stage9BacktestsWriteSyncTaskStore) seedCancelable(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress[taskID] = bt.NewSyncProgress(taskID, "US.AAPL", time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC))
	s.cancels[taskID] = func() {}
}

func (s *stage9BacktestsWriteSyncTaskStore) seedFinished(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress[taskID] = bt.NewSyncProgress(taskID, "US.AAPL", time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC))
}

func (s *stage9BacktestsWriteSyncTaskStore) Add(taskID string, progress *bt.SyncProgress, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		s.trace.syncTaskAdds++
	}
	s.progress[taskID] = progress
	s.cancels[taskID] = cancel
}

func (s *stage9BacktestsWriteSyncTaskStore) Get(taskID string) (*bt.SyncProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	progress, ok := s.progress[taskID]
	if !ok {
		return nil, false
	}
	return progress.Snapshot(), true
}

func (s *stage9BacktestsWriteSyncTaskStore) Finish(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, taskID)
}

func (s *stage9BacktestsWriteSyncTaskStore) Cancel(taskID string, cancelledAt time.Time) (*bt.SyncProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trace != nil {
		s.trace.syncCancels++
	}
	cancel, ok := s.cancels[taskID]
	if !ok {
		return nil, false
	}
	delete(s.cancels, taskID)
	if cancel != nil {
		cancel()
	}
	progress := s.progress[taskID]
	if progress == nil {
		return nil, true
	}
	progress.MarkCancelled(cancelledAt)
	return progress.Snapshot(), true
}

type stage9BacktestsWriteTrace struct {
	strategyLookups  int
	runAdds          int
	syncAdapterOpens int
	syncTaskAdds     int
	syncCancels      int
	runStatusReads   int
	runDeletes       int
}

func (s *stage9BacktestsWriteTrace) effects() stage9BacktestsWriteEffects {
	return stage9BacktestsWriteEffects{
		StrategyLookups:  s.strategyLookups,
		RunAdds:          s.runAdds,
		SyncAdapterOpens: s.syncAdapterOpens,
		SyncTaskAdds:     s.syncTaskAdds,
		SyncCancels:      s.syncCancels,
		RunStatusReads:   s.runStatusReads,
		RunDeletes:       s.runDeletes,
	}
}

func stage9BacktestsWriteRequest(
	t *testing.T,
	router http.Handler,
	request stage9BacktestsWriteFixtureRequest,
) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if request.Body != nil {
		body = strings.NewReader(*request.Body)
	}
	ctx := context.Background()
	if request.Context == "canceled" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	if request.Context == "deadline" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Unix(1, 0))
		cancel()
	}
	var requestBody interface{ Read([]byte) (int, error) }
	if body != nil {
		requestBody = body
	}
	target := strings.Replace(request.RequestPath, "%zz", "placeholder", 1)
	httpRequest := httptest.NewRequestWithContext(ctx, request.Method, target, requestBody)
	if target != request.RequestPath {
		httpRequest.URL.Path = request.RequestPath
		httpRequest.URL.RawPath = request.RequestPath
		httpRequest.RequestURI = request.RequestPath
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httpRequest)
	return response
}

func stage9BacktestsWriteHeaders(response *httptest.ResponseRecorder) map[string]string {
	return map[string]string{"Content-Type": response.Header().Get("Content-Type")}
}

func stage9NormalizeBacktestsWriteEnvelope(t *testing.T, name string, envelope map[string]any) {
	t.Helper()
	rawTimestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("case %s response has no timestamp", name)
	}
	if _, err := time.Parse(time.RFC3339Nano, rawTimestamp); err != nil {
		t.Fatalf("case %s timestamp %q is not RFC3339Nano: %v", name, rawTimestamp, err)
	}
	envelope["timestamp"] = stage9BacktestsWriteTimestamp
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return
	}
	if id, ok := data["id"].(string); ok && strings.HasPrefix(id, "bt-") {
		data["id"] = "fixture-run"
	}
	if taskID, ok := data["taskId"].(string); ok && strings.HasPrefix(taskID, "sync-") {
		data["taskId"] = "fixture-task"
	}
	if name == "sync-null-body" {
		for _, field := range []string{"since", "until"} {
			raw, ok := data[field].(string)
			if !ok {
				t.Fatalf("case %s missing dynamic %s", name, field)
			}
			if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
				t.Fatalf("case %s %s=%q is not RFC3339Nano: %v", name, field, raw, err)
			}
		}
		data["since"] = "2026-07-23T04:00:00Z"
		data["until"] = "2026-08-22T04:00:00Z"
	}
}

func stage9BacktestsWritePortCall(request stage9BacktestsWriteFixtureRequest) bool {
	path := strings.SplitN(request.RequestPath, "?", 2)[0]
	if request.Method == http.MethodPost {
		if path != "/api/v1/backtests" && path != "/api/v1/backtests/sync" {
			return false
		}
		value, ok := stage9BacktestsWriteFirstJSON(request.Body)
		return ok && (value == nil || isJSONMap(value))
	}
	if request.Method != http.MethodDelete {
		return false
	}
	raw := ""
	switch {
	case strings.HasPrefix(path, "/api/v1/backtests/sync/"):
		raw = strings.TrimPrefix(path, "/api/v1/backtests/sync/")
	case strings.HasPrefix(path, "/api/v1/backtests/"):
		raw = strings.TrimPrefix(path, "/api/v1/backtests/")
	default:
		return false
	}
	if raw == "" || strings.Contains(raw, "/") {
		return false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.Contains(decoded, "/") {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/backtests/sync/") {
		return true
	}
	return strings.TrimSpace(decoded) != ""
}

func stage9BacktestsWriteDelegation(request stage9BacktestsWriteFixtureRequest) map[string]any {
	if !stage9BacktestsWritePortCall(request) {
		return nil
	}
	path := strings.SplitN(request.RequestPath, "?", 2)[0]
	if request.Method == http.MethodPost {
		payload, _ := stage9BacktestsWriteFirstJSON(request.Body)
		operation := "start"
		if path == "/api/v1/backtests/sync" {
			operation = "sync"
		}
		return map[string]any{"operation": operation, "payload": payload}
	}
	if strings.HasPrefix(path, "/api/v1/backtests/sync/") {
		id := stage9BacktestsWriteDecodedID(strings.TrimPrefix(path, "/api/v1/backtests/sync/"))
		return map[string]any{"operation": "cancel-sync", "taskId": strings.TrimSpace(id)}
	}
	id := stage9BacktestsWriteDecodedID(strings.TrimPrefix(path, "/api/v1/backtests/"))
	return map[string]any{"operation": "delete", "runId": strings.TrimSpace(id)}
}

func stage9BacktestsWriteFirstJSON(body *string) (any, bool) {
	if body == nil || strings.TrimSpace(*body) == "" {
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(*body))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func isJSONMap(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

func stage9BacktestsWriteDecodedID(raw string) string {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}
