package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

// addFailingRunStore simulates a real persistence outage before a queued run
// can be recorded. All other operations retain the in-memory store's normal
// production-facing behavior.
type addFailingRunStore struct {
	*memoryRunStore
	err error
}

func (s addFailingRunStore) Add(*RunState) error {
	return s.err
}

func TestCoverage98BacktestStartDoesNotLeakLifecycleTaskWhenQueuePersistenceFails(t *testing.T) {
	persistErr := errors.New("run database is unavailable")
	runs := addFailingRunStore{memoryRunStore: newMemoryRunStore(), err: persistErr}
	service := newTestBacktestService(&runs, func(context.Context, bt.RunConfig) *bt.RunResult {
		t.Fatal("runner must not start before its queued state is durable")
		return nil
	})

	if _, err := service.Start(context.Background(), validStartRequest()); !errors.Is(err, persistErr) {
		t.Fatalf("Start() error = %v, want persistence error", err)
	}

	// Start owns the lifecycle task until persistence succeeds. Closing after a
	// failed enqueue must therefore return immediately rather than wait for a
	// task that can never run.
	done := make(chan error, 1)
	go func() { done <- service.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() waited for a leaked backtest lifecycle task")
	}
}

func TestCoverage98BacktestPreparationRejectsInvalidInstrumentAndWarmupPlan(t *testing.T) {
	validDefinition := StrategyDef{
		ID:           "def-prepare",
		Version:      "v1",
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       testPineScript,
	}

	invalidInstrument := validStartRequest()
	invalidInstrument.Market = "MARS"
	invalidInstrument.Symbol = ""
	if _, err := prepareResolvedBacktest(invalidInstrument, validDefinition); err == nil || !IsRequestError(err) || !strings.Contains(err.Error(), "unsupported market") {
		t.Fatalf("prepareResolvedBacktest invalid instrument error = %v", err)
	}

	lowerTimeframeDefinition := validDefinition
	lowerTimeframeDefinition.Script = `//@version=6
strategy("MTF warmup validation", overlay=true)
fast = request.security(syminfo.tickerid, "15", ta.ema(close, 20))
if close > fast
    strategy.entry("Long", strategy.long, qty=1)`
	invalidWarmup := validStartRequest()
	invalidWarmup.Interval = "1h"
	if _, err := prepareResolvedBacktest(invalidWarmup, lowerTimeframeDefinition); err == nil || !IsRequestError(err) || !strings.Contains(err.Error(), "derive strategy warmup") {
		t.Fatalf("prepareResolvedBacktest lower-timeframe warmup error = %v", err)
	}
}

func TestCoverage98BacktestKeepsTerminalStateWhenRunningTransitionCannotPersist(t *testing.T) {
	runs := newMemoryRunStore()
	runs.updateErr = errors.New("transient run-store write failure")
	service := newTestBacktestService(runs, func(_ context.Context, config bt.RunConfig) *bt.RunResult {
		return &bt.RunResult{Symbol: config.Symbol, Interval: config.Interval, FinalBalance: config.InitialBalance}
	})

	started, err := service.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	completed := waitForRunStatus(t, runs, started.ID, "completed")
	if completed.Result == nil || completed.Result.Symbol != "US.AAPL" {
		t.Fatalf("terminal run after running-transition persistence failure = %#v", completed)
	}
}
