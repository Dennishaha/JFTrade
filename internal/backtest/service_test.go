package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/observability"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

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
	var gotFields observability.Fields
	svc := NewService(
		WithRunStore(runs),
		WithStrategyProvider(provider),
		WithDBPathFn(func() string { return "/tmp/backtest.db" }),
		WithRunBacktestFn(func(ctx context.Context, config bt.RunConfig) *bt.RunResult {
			gotConfig = config
			gotFields = observability.FieldsFromContext(ctx)
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

	requestContext := observability.WithFields(context.Background(), observability.Fields{RequestID: "request-backtest-1"})
	started, err := svc.Start(requestContext, StartRequest{
		DefinitionID: "def-1",
		Market:       "US",
		Code:         "AAPL",
		StartDate:    "2024-01-02",
		EndDate:      "2024-01-03",
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
	if started.Request.StartDate != "2024-01-02" || started.Request.EndDate != "2024-01-03" {
		t.Fatalf("market date labels = %q..%q", started.Request.StartDate, started.Request.EndDate)
	}
	if started.Request.StartTime != "2024-01-02T05:00:00Z" || started.Request.EndTime != "2024-01-04T04:59:59.999999999Z" {
		t.Fatalf("normalized UTC range = %q..%q", started.Request.StartTime, started.Request.EndTime)
	}
	if started.Request.MarketTimezone != "America/New_York" {
		t.Fatalf("market timezone = %q, want America/New_York", started.Request.MarketTimezone)
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
	if gotConfig.ExecutionModel != bt.ExecutionModelConservativeBarV1 || started.Request.ExecutionModel != bt.ExecutionModelConservativeBarV1 {
		t.Fatalf("execution model config/request = %q/%q, want %q", gotConfig.ExecutionModel, started.Request.ExecutionModel, bt.ExecutionModelConservativeBarV1)
	}
	if gotFields.RequestID != "request-backtest-1" || gotFields.RunID != started.ID || gotFields.InstrumentID != "US.AAPL" || gotFields.Source != "backtest" {
		t.Fatalf("backtest observability fields = %#v", gotFields)
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

func TestEnsureScriptDataReturnsReadyAndIncludesDerivedWarmup(t *testing.T) {
	var checkedSince time.Time
	svc := NewService(
		WithDBPathFn(func() string { return "/tmp/coverage.db" }),
		WithKLineCoverageCheckFn(func(_ string, symbol, interval string, since, until time.Time, rehabType, sessionScope string) error {
			checkedSince = since
			if symbol != "US.AAPL" || interval != "1m" || rehabType != "forward" || sessionScope != "auto" {
				t.Fatalf("coverage args = %s %s %s %s", symbol, interval, rehabType, sessionScope)
			}
			return nil
		}),
	)
	readiness, err := svc.EnsureScriptData(context.Background(), ScriptStartRequest{
		Script: `//@version=6
strategy("Warmup", overlay=true)
ma = ta.sma(close, 20)
if close > ma
    strategy.entry("Long", strategy.long)`,
		Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: "2024-01-02T15:00:00Z", EndTime: "2024-01-03T15:00:00Z",
	})
	if err != nil {
		t.Fatalf("EnsureScriptData() error = %v", err)
	}
	if !readiness.Ready || readiness.Status != DataStatusReady {
		t.Fatalf("readiness = %#v, want ready", readiness)
	}
	if !checkedSince.Before(time.Date(2024, 1, 2, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("coverage since = %s, want warmup before backtest start", checkedSince)
	}
}

func TestEnsureScriptDataStartsAndDeduplicatesKLineSync(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{waitForCancel: true, started: make(chan struct{})}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
			return errors.New("missing K-line coverage for US.AAPL 1m")
		}),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) { return syncer, nil }),
	)
	request := ScriptStartRequest{
		Script: testPineScript, Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: "2024-01-02T15:00:00Z", EndTime: "2024-01-03T15:00:00Z",
		RehabType: "backward", UseExtendedHours: new(true),
	}
	first, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil {
		t.Fatalf("first EnsureScriptData() error = %v", err)
	}
	select {
	case <-syncer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not start")
	}
	second, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil {
		t.Fatalf("second EnsureScriptData() error = %v", err)
	}
	if first.Sync == nil || second.Sync == nil || first.Sync.TaskID != second.Sync.TaskID {
		t.Fatalf("sync tasks = %#v / %#v, want reused task", first.Sync, second.Sync)
	}
	syncer.mu.Lock()
	params := syncer.params
	syncer.mu.Unlock()
	if params.RehabType != RehabTypeBackward || params.SessionScope != "extended" {
		t.Fatalf("sync params = %#v, want backward extended", params)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestEnsureScriptDataSurfacesFailedSyncWithoutRestarting(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{err: errors.New("OpenD unavailable"), done: make(chan struct{})}
	factoryCalls := 0
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
			return errors.New("missing K-line coverage for US.AAPL 1m")
		}),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) {
			factoryCalls++
			return syncer, nil
		}),
	)
	request := ScriptStartRequest{
		Script: testPineScript, Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: "2024-01-02T15:00:00Z", EndTime: "2024-01-03T15:00:00Z",
	}
	first, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil || first.Sync == nil {
		t.Fatalf("first EnsureScriptData() = %#v, %v", first, err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not finish")
	}
	waitForSyncFinished(t, tasks, first.Sync.TaskID)
	second, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil {
		t.Fatalf("second EnsureScriptData() error = %v", err)
	}
	if second.Status != DataStatusSyncFailed || second.Error != "OpenD unavailable" || factoryCalls != 1 {
		t.Fatalf("readiness=%#v factoryCalls=%d, want one failed sync", second, factoryCalls)
	}
}

func TestEnsureScriptDataStopsAfterCompletedSyncStillLacksCoverage(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
			return errors.New("missing K-line coverage for US.AAPL 1m")
		}),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) { return syncer, nil }),
	)
	request := ScriptStartRequest{
		Script: testPineScript, Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: "2024-01-02T15:00:00Z", EndTime: "2024-01-03T15:00:00Z",
	}
	first, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil || first.Sync == nil {
		t.Fatalf("first EnsureScriptData() = %#v, %v", first, err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not finish")
	}
	waitForSyncFinished(t, tasks, first.Sync.TaskID)
	second, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil {
		t.Fatalf("second EnsureScriptData() error = %v", err)
	}
	if second.Status != DataStatusInsufficientAfterSync {
		t.Fatalf("status = %q, want %q", second.Status, DataStatusInsufficientAfterSync)
	}
}

func TestEnsureScriptDataBecomesReadyAfterCompletedSyncFillsCoverage(t *testing.T) {
	tasks := newMemorySyncTaskStore()
	syncer := &fakeKLineSyncer{done: make(chan struct{})}
	covered := false
	svc := NewService(
		WithSyncTaskStore(tasks),
		WithKLineCoverageCheckFn(func(string, string, string, time.Time, time.Time, string, string) error {
			if covered {
				return nil
			}
			return errors.New("missing K-line coverage for US.AAPL 1m")
		}),
		WithNewKLineSyncerFn(func(string) (KLineSyncer, error) { return syncer, nil }),
	)
	request := ScriptStartRequest{
		Script: testPineScript, Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: "2024-01-02T15:00:00Z", EndTime: "2024-01-03T15:00:00Z",
	}
	first, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil || first.Sync == nil {
		t.Fatalf("first EnsureScriptData() = %#v, %v", first, err)
	}
	select {
	case <-syncer.done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not finish")
	}
	waitForSyncFinished(t, tasks, first.Sync.TaskID)
	covered = true
	second, err := svc.EnsureScriptData(context.Background(), request)
	if err != nil {
		t.Fatalf("second EnsureScriptData() error = %v", err)
	}
	if !second.Ready || second.Status != DataStatusReady {
		t.Fatalf("readiness = %#v, want ready after sync", second)
	}
}

func TestEnsureDefinitionsDataUsesMaximumCandidateWarmup(t *testing.T) {
	provider := fakeStrategyProvider{defs: map[string]StrategyDef{
		"fast": {ID: "fast", Version: "1", SourceFormat: strategydefinition.SourceFormatPineV6, Script: testPineScript},
		"slow": {ID: "slow", Version: "1", SourceFormat: strategydefinition.SourceFormatPineV6, Script: `//@version=6
strategy("Slow", overlay=true)
ma = ta.sma(close, 50)
if close > ma
    strategy.entry("Long", strategy.long)`},
	}}
	var checkedSince time.Time
	svc := NewService(
		WithStrategyProvider(provider),
		WithKLineCoverageCheckFn(func(_ string, _, _ string, since, _ time.Time, _, _ string) error {
			checkedSince = since
			return nil
		}),
	)
	start := time.Date(2024, 1, 2, 15, 0, 0, 0, time.UTC)
	readiness, err := svc.EnsureDefinitionsData(context.Background(), StartRequest{
		Market: "US", Code: "AAPL", Interval: "1m",
		StartTime: start.Format(time.RFC3339), EndTime: start.Add(24 * time.Hour).Format(time.RFC3339),
	}, []string{"fast", "slow"})
	if err != nil {
		t.Fatalf("EnsureDefinitionsData() error = %v", err)
	}
	if !readiness.Ready {
		t.Fatalf("readiness = %#v, want ready", readiness)
	}
	if !checkedSince.Before(start.Add(-40 * time.Minute)) {
		t.Fatalf("coverage since = %s, want maximum candidate warmup", checkedSince)
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
	window := jftradeCheckedTypeAssertion[map[string]any](payload["window"])
	if window["resolution"] != "2m" || window["truncated"] != true || window["nextCursor"] != "1" {
		t.Fatalf("window = %#v, want 2m truncated next cursor", window)
	}
	series := jftradeCheckedTypeAssertion[map[string]any](payload["series"])
	candles := jftradeCheckedTypeAssertion[[]bt.Candle](series["candles"])
	if len(candles) != 1 {
		t.Fatalf("candles len = %d, want 1", len(candles))
	}
	if candles[0].Open != "10" || candles[0].High != "12" || candles[0].Low != "9" || candles[0].Close != "11.5" || candles[0].Volume != "300" {
		t.Fatalf("aggregated candle = %#v", candles[0])
	}
	trades := jftradeCheckedTypeAssertion[[]bt.TradeEvent](series["trades"])
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
			name:      "unsupported execution model",
			req:       StartRequest{DefinitionID: "def-1", Market: "US", Code: "AAPL", StartTime: validReq.StartTime, EndTime: validReq.EndTime, ExecutionModel: "optimistic"},
			runs:      newMemoryRunStore(),
			provider:  validProvider,
			wantError: "unsupported backtest executionModel",
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
