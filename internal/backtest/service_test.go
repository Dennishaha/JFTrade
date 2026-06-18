package backtest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const testPineScript = `//@version=6
strategy("Service Test", overlay=true, initial_capital=25000)
strategy.entry("Long", strategy.long, qty=1)`

func TestStartQueuesRunAndExecutesWithInjectedRunner(t *testing.T) {
	runs := newMemoryRunStore()
	provider := fakeStrategyProvider{
		defs: map[string]StrategyDef{
			"def-1": {
				ID:           "def-1",
				Version:      "v2",
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       testPineScript,
			},
		},
	}

	done := make(chan struct{})
	var gotConfig bt.RunConfig
	svc := NewService(
		WithRunStore(runs),
		WithStrategyProvider(provider),
		WithDBPathFn(func() string { return "/tmp/backtest.db" }),
		WithRunBacktestFn(func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
			gotConfig = config
			close(done)
			return &bt.RunResult{
				Symbol:       config.Symbol,
				Interval:     config.Interval,
				StartTime:    config.StartTime.Format(time.RFC3339),
				EndTime:      config.EndTime.Format(time.RFC3339),
				FinalBalance: config.InitialBalance + 100,
			}
		}),
	)

	started, err := svc.Start(context.Background(), StartRequest{
		DefinitionID: "def-1",
		Market:       "US",
		Code:         "AAPL",
		StartTime:    "2024-01-02T00:00:00Z",
		EndTime:      "2024-01-03T00:00:00Z",
		RehabType:    "forward",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != "queued" {
		t.Fatalf("initial status = %q, want queued", started.Status)
	}
	if started.Request.Symbol != "US.AAPL" {
		t.Fatalf("symbol = %q, want US.AAPL", started.Request.Symbol)
	}
	if started.Request.Interval != "1m" {
		t.Fatalf("interval = %q, want 1m", started.Request.Interval)
	}
	if started.Request.DefinitionVersion != "v2" {
		t.Fatalf("definition version = %q, want v2", started.Request.DefinitionVersion)
	}
	if started.Request.InitialBalance != 25000 {
		t.Fatalf("initial balance = %v, want script metadata 25000", started.Request.InitialBalance)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not called")
	}

	completed := waitForRunStatus(t, runs, started.ID, "completed")
	if completed.Result == nil {
		t.Fatal("completed run has nil result")
	}
	if completed.Result.FinalBalance != 25100 {
		t.Fatalf("final balance = %v, want 25100", completed.Result.FinalBalance)
	}
	if gotConfig.DBPath != "/tmp/backtest.db" {
		t.Fatalf("DBPath = %q, want /tmp/backtest.db", gotConfig.DBPath)
	}
	if gotConfig.Symbol != "US.AAPL" || gotConfig.Interval != "1m" {
		t.Fatalf("config symbol/interval = %q/%q, want US.AAPL/1m", gotConfig.Symbol, gotConfig.Interval)
	}
	if gotConfig.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("source format = %q, want pine v6", gotConfig.SourceFormat)
	}
	if gotConfig.InitialBalance != 25000 {
		t.Fatalf("config initial balance = %v, want 25000", gotConfig.InitialBalance)
	}
}

func TestStartScriptQueuesResearchRunWithoutStrategyProvider(t *testing.T) {
	runs := newMemoryRunStore()
	done := make(chan struct{})
	var gotConfig bt.RunConfig
	svc := NewService(
		WithRunStore(runs),
		WithDBPathFn(func() string { return "/tmp/research.db" }),
		WithRunBacktestFn(func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
			gotConfig = config
			close(done)
			return &bt.RunResult{
				Symbol:       config.Symbol,
				Interval:     config.Interval,
				StartTime:    config.StartTime.Format(time.RFC3339),
				EndTime:      config.EndTime.Format(time.RFC3339),
				FinalBalance: config.InitialBalance + 250,
			}
		}),
	)

	started, err := svc.StartScript(context.Background(), ScriptStartRequest{
		Script:    testPineScript,
		Market:    "US",
		Code:      "AAPL",
		Interval:  "1m",
		StartTime: "2024-01-02T00:00:00Z",
		EndTime:   "2024-01-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("StartScript() error = %v", err)
	}
	if !strings.HasPrefix(started.Request.DefinitionID, "adk-research-") {
		t.Fatalf("definition id = %q, want adk research id", started.Request.DefinitionID)
	}
	if strings.Contains(started.Request.DefinitionID, "Service Test") {
		t.Fatalf("definition id leaked script content: %q", started.Request.DefinitionID)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not called")
	}
	if gotConfig.StrategyScript != testPineScript {
		t.Fatalf("strategy script = %q, want inline script", gotConfig.StrategyScript)
	}
	if gotConfig.DBPath != "/tmp/research.db" {
		t.Fatalf("DBPath = %q, want /tmp/research.db", gotConfig.DBPath)
	}
	completed := waitForRunStatus(t, runs, started.ID, "completed")
	if completed.Result == nil || completed.Result.FinalBalance != 25250 {
		t.Fatalf("completed result = %#v, want final balance 25250", completed.Result)
	}
}

func TestResultViewReturnsWindowedAggregatedChartData(t *testing.T) {
	runs := newMemoryRunStore()
	run := &RunState{
		ID:     "bt-view",
		Status: "completed",
		Request: StartRequest{
			DefinitionID:   "def-1",
			Market:         "US",
			Code:           "AAPL",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			StartTime:      "2024-01-02T00:00:00Z",
			EndTime:        "2024-01-02T00:10:00Z",
			InitialBalance: 1000,
			RehabType:      "forward",
		},
		Result: &bt.RunResult{
			Symbol:       "US.AAPL",
			Interval:     "1m",
			StartTime:    "2024-01-02T00:00:00Z",
			EndTime:      "2024-01-02T00:10:00Z",
			FinalBalance: 1100,
			PnL:          100,
			Candles: []bt.Candle{
				{Time: "2024-01-02T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100"},
				{Time: "2024-01-02T00:01:00Z", Open: "10.5", High: "12", Low: "10", Close: "11.5", Volume: "200"},
				{Time: "2024-01-02T00:02:00Z", Open: "11.5", High: "13", Low: "11", Close: "12.5", Volume: "300"},
			},
			Trades:        []bt.TradeEvent{{Time: "2024-01-02T00:01:00Z", Side: "BUY", Price: "11", Qty: "1"}},
			PnLCurve:      []bt.PnLPoint{{Time: "2024-01-02T00:00:00Z", Equity: 1000}, {Time: "2024-01-02T00:02:00Z", Equity: 1100}},
			DrawdownCurve: []bt.DrawdownPoint{{Time: "2024-01-02T00:02:00Z", Drawdown: 0.01}},
		},
		CreatedAt: "2024-01-02T00:00:00Z",
		UpdatedAt: "2024-01-02T00:10:00Z",
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("runs.Add: %v", err)
	}
	svc := NewService(WithRunStore(runs))

	payload, err := svc.ResultView(ResultViewRequest{
		RunID:      "bt-view",
		View:       "chart",
		Resolution: "2m",
		Include:    []string{"candles", "trades"},
		Limit:      1,
	})
	if err != nil {
		t.Fatalf("ResultView() error = %v", err)
	}
	window := payload["window"].(map[string]any)
	if window["resolution"] != "2m" || window["truncated"] != true || window["nextCursor"] != "1" {
		t.Fatalf("window = %#v, want 2m truncated next cursor", window)
	}
	series := payload["series"].(map[string]any)
	candles := series["candles"].([]bt.Candle)
	if len(candles) != 1 {
		t.Fatalf("candles len = %d, want 1", len(candles))
	}
	if candles[0].Open != "10" || candles[0].High != "12" || candles[0].Low != "9" || candles[0].Close != "11.5" || candles[0].Volume != "300" {
		t.Fatalf("aggregated candle = %#v", candles[0])
	}
	trades := series["trades"].([]bt.TradeEvent)
	if len(trades) != 1 || trades[0].Side != "BUY" {
		t.Fatalf("trades = %#v, want one BUY", trades)
	}

	_, err = svc.ResultView(ResultViewRequest{RunID: "bt-view", View: "chart", Resolution: "30s"})
	if err == nil || !strings.Contains(err.Error(), "finer than native interval") {
		t.Fatalf("fine resolution error = %v, want finer-than-native rejection", err)
	}
}

func TestStartValidationErrors(t *testing.T) {
	validProvider := fakeStrategyProvider{
		defs: map[string]StrategyDef{
			"def-1": {
				ID:           "def-1",
				Version:      "v1",
				SourceFormat: strategydefinition.SourceFormatPineV6,
				Script:       testPineScript,
			},
		},
	}
	validReq := StartRequest{
		DefinitionID: "def-1",
		Market:       "US",
		Code:         "AAPL",
		StartTime:    "2024-01-02T00:00:00Z",
		EndTime:      "2024-01-03T00:00:00Z",
	}

	tests := []struct {
		name      string
		req       StartRequest
		runs      RunStore
		provider  StrategyProvider
		wantError string
	}{
		{
			name:      "missing definition id",
			req:       StartRequest{Market: "US", Code: "AAPL", StartTime: validReq.StartTime, EndTime: validReq.EndTime},
			runs:      newMemoryRunStore(),
			provider:  validProvider,
			wantError: "definitionId is required",
		},
		{
			name:      "missing strategy provider",
			req:       validReq,
			runs:      newMemoryRunStore(),
			wantError: "strategy provider not configured",
		},
		{
			name:      "definition not found",
			req:       validReq,
			runs:      newMemoryRunStore(),
			provider:  fakeStrategyProvider{defs: map[string]StrategyDef{}},
			wantError: "strategy definition not found",
		},
		{
			name:      "provider error",
			req:       validReq,
			runs:      newMemoryRunStore(),
			provider:  fakeStrategyProvider{err: errors.New("provider failed")},
			wantError: "provider failed",
		},
		{
			name: "unsupported source format",
			req:  validReq,
			runs: newMemoryRunStore(),
			provider: fakeStrategyProvider{defs: map[string]StrategyDef{
				"def-1": {ID: "def-1", SourceFormat: "javascript", Script: testPineScript},
			}},
			wantError: "unsupported strategy source format",
		},
		{
			name:      "invalid start time",
			req:       withStartTime(validReq, "not-time"),
			runs:      newMemoryRunStore(),
			provider:  validProvider,
			wantError: "invalid startTime",
		},
		{
			name:      "invalid end time",
			req:       withEndTime(validReq, "not-time"),
			runs:      newMemoryRunStore(),
			provider:  validProvider,
			wantError: "invalid endTime",
		},
		{
			name:      "end before start",
			req:       withEndTime(validReq, "2024-01-01T00:00:00Z"),
			runs:      newMemoryRunStore(),
			provider:  validProvider,
			wantError: "endTime must be after startTime",
		},
		{
			name:      "missing run store",
			req:       validReq,
			provider:  validProvider,
			wantError: "run store not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(
				WithRunStore(tt.runs),
				WithStrategyProvider(tt.provider),
				WithRunBacktestFn(func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
					t.Fatalf("runner should not be called")
					return nil
				}),
			)
			_, err := svc.Start(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Start() error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func TestStartMarksFailedWhenRunnerReturnsError(t *testing.T) {
	runs := newMemoryRunStore()
	svc := newTestBacktestService(runs, func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
		return &bt.RunResult{
			Symbol:       config.Symbol,
			Interval:     config.Interval,
			StartTime:    config.StartTime.Format(time.RFC3339),
			EndTime:      config.EndTime.Format(time.RFC3339),
			FinalBalance: config.InitialBalance,
			Error:        "boom",
		}
	})

	started, err := svc.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	failed := waitForRunStatus(t, runs, started.ID, "failed")
	if failed.Result == nil || failed.Result.Error != "boom" {
		t.Fatalf("failed result error = %#v, want boom", failed.Result)
	}
}

func TestCloseCancelsAndWaitsForActiveBacktest(t *testing.T) {
	runs := newMemoryRunStore()
	startedRunner := make(chan struct{})
	svc := newTestBacktestService(runs, func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
		close(startedRunner)
		<-ctx.Done()
		return &bt.RunResult{Symbol: config.Symbol}
	})

	started, err := svc.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-startedRunner:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not called")
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForRunStatus(t, runs, started.ID, "cancelled")
	if _, err := svc.Start(context.Background(), validStartRequest()); !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("Start() after Close error = %v, want ErrServiceClosed", err)
	}
}

func TestRunStoreDelegation(t *testing.T) {
	runs := newMemoryRunStore()
	svc := NewService(WithRunStore(runs))
	result := &bt.RunResult{Symbol: "US.AAPL", FinalBalance: 123}
	run := &RunState{
		ID:        "run-1",
		Status:    "completed",
		Request:   validStartRequest(),
		Result:    result,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	listed := svc.List()
	if len(listed) != 1 || listed[0].ID != "run-1" {
		t.Fatalf("List() = %#v, want run-1", listed)
	}
	if listed[0].Result != nil {
		t.Fatalf("List() returned result details, want lightweight state")
	}

	status, ok := svc.GetStatus("run-1")
	if !ok || status.ID != "run-1" || status.Result != nil {
		t.Fatalf("GetStatus() = %#v, %v; want lightweight run", status, ok)
	}

	full, ok, err := svc.GetResult("run-1")
	if err != nil || !ok || full.Result == nil || full.Result.FinalBalance != 123 {
		t.Fatalf("GetResult() = %#v, %v, %v; want full result", full, ok, err)
	}

	cancelled := false
	runs.SetCancel("run-1", func() { cancelled = true })
	if !svc.Cancel("run-1") || !cancelled {
		t.Fatal("Cancel() did not delegate to run store cancel")
	}
	if svc.Cancel("missing") {
		t.Fatal("Cancel(missing) = true, want false")
	}

	deleted, ok, err := svc.Delete("run-1")
	if err != nil || !ok || deleted.ID != "run-1" {
		t.Fatalf("Delete() = %#v, %v, %v; want deleted run", deleted, ok, err)
	}
	if _, ok := svc.GetStatus("run-1"); ok {
		t.Fatal("run still exists after Delete()")
	}
}

func TestFinishRunFallsBackToMemoryOnlyWhenPersistentUpdateFails(t *testing.T) {
	runs := newMemoryRunStore()
	svc := NewService(WithRunStore(runs))
	run := &RunState{
		ID:        "run-1",
		Status:    "running",
		Request:   validStartRequest(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	runs.updateErr = errors.New("persist failed")

	result := &bt.RunResult{Symbol: "US.AAPL", FinalBalance: 101}
	svc.finishRun("run-1", "completed", result)

	got, ok, err := runs.GetFull("run-1")
	if err != nil || !ok {
		t.Fatalf("GetFull() = %#v, %v, %v; want run", got, ok, err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if got.Result == nil || got.Result.FinalBalance != 101 {
		t.Fatalf("result = %#v, want final balance 101", got.Result)
	}
}

func TestSyncProgressAndCancelDelegateToSyncTaskStore(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	svc := NewService(WithSyncTaskStore(tasks))
	cancelled := false
	progress := bt.NewSyncProgress("task-1", "HK.00700", time.Now())
	tasks.Add("task-1", progress, func() { cancelled = true })

	got, ok := svc.GetSyncProgress("task-1")
	if !ok || got.TaskID != "task-1" || got.Status != "queued" {
		t.Fatalf("GetSyncProgress() = %#v, %v; want queued task", got, ok)
	}

	got, ok = svc.CancelSync("task-1")
	if !ok || got.Status != "cancelled" || !cancelled {
		t.Fatalf("CancelSync() = %#v, %v cancelled=%v; want cancelled task", got, ok, cancelled)
	}
	if _, ok := svc.GetSyncProgress("missing"); ok {
		t.Fatal("GetSyncProgress(missing) = true, want false")
	}
}

func TestSyncConvertsParamsAndClosesAdapter(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	var gotDBPath string
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithDBPathFn(func() string { return "/tmp/sync.db" }),
		WithNewKLineSyncerFn(func(dbPath string) (KLineSyncer, error) {
			gotDBPath = dbPath
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market:       "US",
		Code:         "AAPL",
		Intervals:    []string{"1d", "1w"},
		Since:        "2024-01-02T00:00:00Z",
		Until:        "2024-01-03T00:00:00Z",
		RehabType:    "backward",
		SessionScope: "extended",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if gotDBPath != "/tmp/sync.db" {
		t.Fatalf("db path = %q, want /tmp/sync.db", gotDBPath)
	}
	if started.Symbol != "US.AAPL" {
		t.Fatalf("symbol = %q, want US.AAPL", started.Symbol)
	}

	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	syncer.mu.Lock()
	params := syncer.params
	closed := syncer.closed
	syncer.mu.Unlock()
	if params.Symbol != "US.AAPL" {
		t.Fatalf("params symbol = %q, want US.AAPL", params.Symbol)
	}
	if !reflect.DeepEqual(params.Intervals, []bbgotypes.Interval{"1h"}) {
		t.Fatalf("params intervals = %#v, want [1h]", params.Intervals)
	}
	if params.RehabType != RehabTypeBackward {
		t.Fatalf("rehab type = %q, want backward", params.RehabType)
	}
	if params.SessionScope != "extended" {
		t.Fatalf("session scope = %q, want extended", params.SessionScope)
	}
	if !closed {
		t.Fatal("sync adapter was not closed")
	}
	if !tasks.isFinished(started.TaskID) {
		t.Fatal("sync task was not marked finished")
	}
}

func TestSyncFailureMarksProgressFailedAndClosesAdapter(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{
		err:  errors.New("OpenD unavailable"),
		done: make(chan struct{}),
	}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	progress, ok := tasks.Get(started.TaskID)
	if !ok || progress.Status != "failed" || progress.Error != "OpenD unavailable" {
		t.Fatalf("progress = %#v, %v; want failed OpenD error", progress, ok)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("failed sync adapter was not closed")
	}
}

func TestCloseCancelsAndWaitsForActiveSync(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{
		waitForCancel: true,
		started:       make(chan struct{}),
	}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForSyncFinished(t, tasks, started.TaskID)
	progress, ok := tasks.Get(started.TaskID)
	if !ok || progress.Status != "cancelled" {
		t.Fatalf("progress = %#v, %v; want cancelled", progress, ok)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("sync adapter was not closed before Close returned")
	}
}

func TestSyncTaskIDsAreUnique(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return &fakeKLineSyncer{}, nil
		}),
	)
	request := SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	}
	first, err := svc.Sync(context.Background(), request)
	if err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	second, err := svc.Sync(context.Background(), request)
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if first.TaskID == second.TaskID {
		t.Fatalf("task IDs collided: %q", first.TaskID)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestSyncClosesAdapterWhenTaskStoreMissing(t *testing.T) {
	syncer := &fakeKLineSyncer{}
	svc := NewService(WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
		return syncer, nil
	}))

	_, err := svc.Sync(context.Background(), SyncRequest{
		Market: "HK",
		Code:   "00700",
		Since:  "2024-01-02T00:00:00Z",
		Until:  "2024-01-03T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "sync task store not configured") {
		t.Fatalf("Sync() error = %v, want missing task store", err)
	}
	syncer.mu.Lock()
	closed := syncer.closed
	syncer.mu.Unlock()
	if !closed {
		t.Fatal("adapter was not closed after setup failure")
	}
}

func TestSyncRequestErrors(t *testing.T) {
	svc := NewService()
	tests := []struct {
		name string
		req  SyncRequest
	}{
		{
			name: "invalid symbol",
			req:  SyncRequest{Symbol: "bad symbol"},
		},
		{
			name: "invalid since",
			req:  SyncRequest{Market: "HK", Code: "00700", Since: "bad"},
		},
		{
			name: "invalid until",
			req:  SyncRequest{Market: "HK", Code: "00700", Until: "bad"},
		},
		{
			name: "reversed range",
			req:  SyncRequest{Market: "HK", Code: "00700", Since: "2024-01-03T00:00:00Z", Until: "2024-01-02T00:00:00Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Sync(context.Background(), tt.req)
			if err == nil || !IsRequestError(err) {
				t.Fatalf("Sync() error = %v, want RequestError", err)
			}
		})
	}
}

func TestSyncUnknownRehabTypeFallsBackToForward(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			return syncer, nil
		}),
	)

	started, err := svc.Sync(context.Background(), SyncRequest{
		Market:    "HK",
		Code:      "00700",
		Since:     "2024-01-02T00:00:00Z",
		Until:     "2024-01-03T00:00:00Z",
		RehabType: "sideways",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync adapter was not called")
	}
	waitForSyncFinished(t, tasks, started.TaskID)

	syncer.mu.Lock()
	rehabType := syncer.params.RehabType
	syncer.mu.Unlock()
	if rehabType != RehabTypeForward {
		t.Fatalf("rehabType = %v, want forward fallback", rehabType)
	}
}

func TestPlanSyncIntervals(t *testing.T) {
	tests := []struct {
		name         string
		symbol       string
		requested    []bbgotypes.Interval
		sessionScope string
		want         []bbgotypes.Interval
	}{
		{
			name:      "deduplicates planned intervals",
			symbol:    "HK.00700",
			requested: []bbgotypes.Interval{"1m", "1m", "3d"},
			want:      []bbgotypes.Interval{"1m", "1d"},
		},
		{
			name:      "downgrades unsupported multi day and sub daily intervals",
			symbol:    "HK.00700",
			requested: []bbgotypes.Interval{"3d", "2w", "2h"},
			want:      []bbgotypes.Interval{"1d", "1h"},
		},
		{
			name:         "uses hourly data for us extended daily sessions",
			symbol:       "US.AAPL",
			requested:    []bbgotypes.Interval{"1d", "1w"},
			sessionScope: "extended",
			want:         []bbgotypes.Interval{"1h"},
		},
		{
			name:         "keeps us regular daily sessions unchanged",
			symbol:       "US.AAPL",
			requested:    []bbgotypes.Interval{"1d"},
			sessionScope: "regular",
			want:         []bbgotypes.Interval{"1d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planSyncIntervals(tt.symbol, tt.requested, tt.sessionScope); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planSyncIntervals() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeSessionScope(t *testing.T) {
	for input, want := range map[string]string{
		"regular":    "regular",
		" extended ": "extended",
		"":           "legacy",
		"unknown":    "legacy",
	} {
		if got := normalizeSessionScope(input); got != want {
			t.Fatalf("normalizeSessionScope(%q) = %q, want %q", input, got, want)
		}
	}
}

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
			run, _, _ := runs.GetFull(runID)
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
}

func (s *fakeKLineSyncer) Sync(ctx context.Context, params KLineSyncParams, progress *bt.SyncProgress) error {
	s.mu.Lock()
	s.params = params
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
