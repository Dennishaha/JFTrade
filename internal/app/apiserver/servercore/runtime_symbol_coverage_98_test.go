package servercore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98StrategyRuntimeSymbolKeepsPollingAndLateTradeFailuresVisible(t *testing.T) {
	// A runtime may be torn down while a subscription callback is still in
	// flight. Both nil receiver and missing exchange must be no-ops.
	var nilRuntime *strategySymbolRuntime
	nilRuntime.syncClosedKLines()
	(&strategySymbolRuntime{}).syncClosedKLines()

	stub := newStrategyRuntimeStubExchange()
	stub.queryKLinesErr = errors.New("market-data connection reset")
	var reported []string
	runner := &strategySymbolRuntime{
		symbol:          "US.AAPL",
		interval:        bbgotypes.Interval1m,
		runtimeExchange: stub,
		onError:         func(message string) { reported = append(reported, message) },
	}
	runner.syncClosedKLines()
	if len(reported) != 1 || !strings.Contains(reported[0], "market-data connection reset") {
		t.Fatalf("polling failure report = %#v", reported)
	}

	// A broker can omit an execution timestamp. The live runtime must use a
	// current bucket instead of dropping a valid priced trade.
	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 101, time.Time{}))
	if runner.currentBucket == nil || runner.currentPrice() != 101 {
		t.Fatalf("zero-time trade did not create a current bucket: %#v / %v", runner.currentBucket, runner.currentPrice())
	}

	// Out-of-order trades are possible after a reconnect. They still enrich the
	// existing bucket and must never roll the clock backwards or emit a closure.
	windowStart, windowEnd := strategyRuntimeBucketWindow(strategyRuntimeTestTime(10, 2, 10), bbgotypes.Interval1m)
	runner.currentBucket = &bbgotypes.KLine{
		Symbol: "US.AAPL", Interval: bbgotypes.Interval1m,
		StartTime: bbgotypes.Time(windowStart), EndTime: bbgotypes.Time(windowEnd),
		Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(102),
		Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(101), Closed: false,
	}
	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 103, strategyRuntimeTestTime(10, 1, 40)))
	if runner.currentBucket == nil || !runner.currentBucket.StartTime.Time().Equal(windowStart) || runner.currentPrice() != 103 {
		t.Fatalf("late trade changed bucket identity or price: %#v", runner.currentBucket)
	}
}

func TestCoverage98StrategyRuntimeSymbolFallbacksAvoidPanicsDuringShutdown(t *testing.T) {
	var nilRuntime *strategySymbolRuntime
	nilRuntime.syncClosedKLinesLoop()

	// A detached callback can report an error after the manager has discarded
	// its observer. It must retain a safe logging fallback rather than panic.
	(&strategySymbolRuntime{}).handleRuntimeError(errors.New("late runtime callback"))

	// The runtime uses the parent cancellation when it is present; this verifies
	// a regular cancellation remains observable to the polling loop's context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &strategySymbolRuntime{ctx: ctx}
	if runner.context().Err() == nil {
		t.Fatal("runtime context lost caller cancellation")
	}
}
