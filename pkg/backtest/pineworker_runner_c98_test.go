package backtest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestCoverage98PineWorkerRunnerRejectsInvalidConfigurationBeforeReplay(t *testing.T) {
	t.Run("unsupported execution model does not open a replay store", func(t *testing.T) {
		result := RunWithPineWorker(t.Context(), RunConfig{
			ExecutionModel: "optimistic-fill-v1",
		}, &fakePineWorkerBacktestRunner{})
		if result == nil || !strings.Contains(result.Error, "unsupported backtest executionModel") {
			t.Fatalf("invalid execution model result = %#v", result)
		}
	})

	t.Run("lower timeframe request is rejected before data coverage lookup", func(t *testing.T) {
		store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "pine-worker-warmup.db"))
		if err != nil {
			t.Fatalf("NewFutuKLineStore: %v", err)
		}
		t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

		now := time.Date(2026, time.July, 1, 13, 30, 0, 0, time.UTC)
		result := newRunResult(RunConfig{})
		_, _, _, _, _, ok := preparePineWorkerStrategy(t.Context(), RunConfig{
			Symbol:       "US.AAPL",
			Interval:     string(types.Interval5m),
			SourceFormat: strategydefinition.SourceFormatPineV6,
			StartTime:    now,
			EndTime:      now.Add(time.Hour),
			StrategyScript: `//@version=6
strategy("lower timeframe is unsafe")
probe = request.security(syminfo.tickerid, "1", close)`,
		}, result, store)
		if ok || !strings.Contains(result.Error, "lower than strategy interval") {
			t.Fatalf("lower-timeframe warmup result = %#v, ok=%v", result, ok)
		}
	})

	start := time.Date(2026, time.July, 1, 13, 30, 0, 0, time.UTC)
	warmupUntil, queryStart := pineWorkerWarmupRange(RunConfig{StartTime: start, WarmupCandles: 2}, types.Interval5m, 4)
	if !warmupUntil.Equal(start) || !queryStart.Equal(start.Add(-20*time.Minute)) {
		t.Fatalf("warmup range = %s/%s, want %s/%s", warmupUntil, queryStart, start, start.Add(-20*time.Minute))
	}
}

func TestCoverage98PineWorkerRunnerReplayStopsOnIntegrityFailures(t *testing.T) {
	first := testPineWorkerRunnerKLine(time.Date(2026, time.July, 1, 13, 30, 0, 0, time.UTC), 100)
	batch := &pineWorkerReplayKLineBatch{}
	batch.append(first)

	t.Run("extra and time-shifted bars fail closed", func(t *testing.T) {
		state := newPineWorkerBacktestReplayState(batch, nil, &PineWorkerCommandExecutor{})
		state.nextBarIndex = batch.Len()
		if err := state.onKLineClosed(t.Context(), first); err == nil || !strings.Contains(err.Error(), "extra closed kline") {
			t.Fatalf("extra kline error = %v", err)
		}

		state = newPineWorkerBacktestReplayState(batch, nil, &PineWorkerCommandExecutor{})
		shifted := first
		shifted.StartTime = types.Time(first.StartTime.Time().Add(time.Minute))
		if err := state.onKLineClosed(t.Context(), shifted); err == nil || !strings.Contains(err.Error(), "does not match planned candle") {
			t.Fatalf("shifted kline error = %v", err)
		}
	})

	t.Run("command executor failure stops the current bar", func(t *testing.T) {
		state := newPineWorkerBacktestReplayState(batch, []WorkerOrderCommand{{Kind: "entry", BarIndex: 0}}, &PineWorkerCommandExecutor{})
		if err := state.onKLineClosed(t.Context(), first); err == nil || !strings.Contains(err.Error(), "execute commands for bar 0") {
			t.Fatalf("command failure = %v", err)
		}
	})

	t.Run("existing replay errors prevent consumption and shutdown work", func(t *testing.T) {
		prep := &pineWorkerBacktestPreparation{result: &RunResult{Error: "earlier replay failure"}}
		execution := &pineWorkerBacktestExecution{replayKLines: batch}
		consumePineWorkerReplay(prep, execution)
		if prep.result.Error != "earlier replay failure" {
			t.Fatalf("replay error was changed: %q", prep.result.Error)
		}
	})
}

func TestCoverage98PineWorkerRunnerFailsClosedForInvalidWorkerCommandsDuringReplay(t *testing.T) {
	isolateBacktestHome(t)

	start := time.Date(2026, time.July, 2, 13, 30, 0, 0, time.UTC)
	newConfiguredReplay := func(t *testing.T) (RunConfig, *fakePineWorkerBacktestRunner) {
		t.Helper()
		dbPath := filepath.Join(t.TempDir(), "pine-worker-invalid-command.db")
		store, err := NewFutuKLineStore(dbPath)
		if err != nil {
			t.Fatalf("NewFutuKLineStore: %v", err)
		}
		klines := []types.KLine{
			testPineWorkerRunnerKLine(start, 100),
			testPineWorkerRunnerKLine(start.Add(time.Minute), 101),
		}
		if err := store.InsertKLines(klines, "forward"); err != nil {
			jftradeCheckTestError(t, store.Close())
			t.Fatalf("InsertKLines: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close K-line store: %v", err)
		}
		return RunConfig{
			DBPath:       dbPath,
			Symbol:       "US.AAPL",
			Interval:     string(types.Interval1m),
			SourceFormat: strategydefinition.SourceFormatPineV6,
			StartTime:    klines[0].StartTime.Time(),
			EndTime:      klines[len(klines)-1].EndTime.Time(),
			StrategyScript: `//@version=6
strategy("invalid worker command")`,
			InitialBalance: 10_000,
		}, &fakePineWorkerBacktestRunner{}
	}

	t.Run("explicit zero quantity is reported instead of silently replaying an invalid entry", func(t *testing.T) {
		cfg, runner := newConfiguredReplay(t)
		runner.response.OrderIntents = []pineworker.OrderIntent{{
			Kind: "entry", ID: "invalid-zero", Direction: "long", Quantity: 0, HasQuantity: true, BarIndex: 0,
		}}

		result := RunWithPineWorker(context.Background(), cfg, runner)
		if result == nil || !strings.Contains(result.Error, "pine worker replay command") || !strings.Contains(result.Error, "quantity must be positive") {
			t.Fatalf("zero quantity replay result = %#v", result)
			return
		}
		if len(result.OrderBook) != 0 {
			t.Fatalf("invalid command created orders: %#v", result.OrderBook)
		}
	})

	t.Run("cancel without an order id is reported as invalid worker output", func(t *testing.T) {
		cfg, runner := newConfiguredReplay(t)
		runner.response.OrderIntents = []pineworker.OrderIntent{{Kind: "cancel", BarIndex: 0}}

		result := RunWithPineWorker(context.Background(), cfg, runner)
		if result == nil || !strings.Contains(result.Error, "pine worker replay command") || !strings.Contains(result.Error, "cancel command id is required") {
			t.Fatalf("missing cancel id replay result = %#v", result)
		}
	})

	t.Run("worker transport failure prevents any replay from starting", func(t *testing.T) {
		cfg, runner := newConfiguredReplay(t)
		runner.err = errors.New("worker process connection lost")

		result := RunWithPineWorker(context.Background(), cfg, runner)
		if result == nil || !strings.Contains(result.Error, "plan pine worker replay") || !strings.Contains(result.Error, "connection lost") {
			t.Fatalf("worker transport failure result = %#v", result)
		}
		if len(result.OrderBook) != 0 || len(result.Candles) != 0 {
			t.Fatalf("worker planning failure must not replay orders or candles: %#v", result)
		}
	})
}
