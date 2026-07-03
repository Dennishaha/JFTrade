package backtest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestBuildPineWorkerBacktestRequestFromKLines(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	params := map[string]string{"fast": "5"}
	request, err := BuildPineWorkerBacktestRequest(PineWorkerReplayPlanRequest{
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Params:    params,
		KLines:    []types.KLine{testReplayKLine(start, 10, 12, 9, 11)},
	})
	if err != nil {
		t.Fatalf("BuildPineWorkerBacktestRequest error = %v", err)
	}
	if request.JobID != "backtest:US.AAPL:1" || request.Mode != pineworker.ModeBacktest {
		t.Fatalf("request identity = %#v", request)
	}
	if request.Candles[0].OpenTime != start.UnixMilli() || request.Candles[0].Close != 11 {
		t.Fatalf("candle = %#v", request.Candles[0])
	}
	params["fast"] = "99"
	if request.Params["fast"] != "5" {
		t.Fatalf("params alias was not copied: %#v", request.Params)
	}
}

func TestBuildPineWorkerBacktestRequestValidatesInputs(t *testing.T) {
	_, err := BuildPineWorkerBacktestRequest(PineWorkerReplayPlanRequest{
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
	})
	if err == nil || !strings.Contains(err.Error(), "candles are required") {
		t.Fatalf("empty candles error = %v", err)
	}
	_, err = BuildPineWorkerBacktestRequest(PineWorkerReplayPlanRequest{
		Symbol:    "US.AAPL",
		Timeframe: "1",
		KLines:    []types.KLine{testReplayKLine(time.Now(), 10, 12, 9, 11)},
	})
	if err == nil || !strings.Contains(err.Error(), "source is required") {
		t.Fatalf("empty source error = %v", err)
	}
}

func TestPineWorkerReplayPlannerPlanGroupsCommands(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	runner := &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			OrderIntents: []pineworker.OrderIntent{
				{Kind: "entry", ID: "long", Direction: "long", BarIndex: 2},
				{Kind: "close", ID: "close", Direction: "long", BarIndex: 1},
			},
			Metadata: pineworker.WorkerMetadata{WorkerID: "worker-1"},
		},
	}
	planner := PineWorkerReplayPlanner{Adapter: PineWorkerBacktestAdapter{Runner: runner}}
	plan, err := planner.Plan(context.Background(), PineWorkerReplayPlanRequest{
		JobID:     "job-1",
		ScriptID:  "script-1",
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Params:    map[string]string{"length": "20"},
		KLines: []types.KLine{
			testReplayKLine(start, 10, 11, 9, 10),
			testReplayKLine(start.Add(time.Minute), 10, 12, 9, 11),
			testReplayKLine(start.Add(2*time.Minute), 11, 13, 10, 12),
		},
	})
	if err != nil {
		t.Fatalf("Plan error = %v", err)
	}
	if runner.request.JobID != "job-1" || runner.request.ScriptID != "script-1" || runner.request.Params["length"] != "20" {
		t.Fatalf("worker request = %#v", runner.request)
	}
	if got := commandIDs(plan.Commands); !reflect.DeepEqual(got, []string{"close", "long"}) {
		t.Fatalf("command order = %#v", got)
	}
	if len(plan.ByBarIndex[1]) != 1 || plan.ByBarIndex[1][0].Time != start.Add(time.Minute).UnixMilli() {
		t.Fatalf("bar 1 commands = %#v", plan.ByBarIndex[1])
	}
	if len(plan.ByOpenTime[start.Add(2*time.Minute).UnixMilli()]) != 1 {
		t.Fatalf("open time grouping = %#v", plan.ByOpenTime)
	}
	if plan.Metadata.WorkerID != "worker-1" || plan.CandleCount != 3 {
		t.Fatalf("plan metadata/count = %#v count=%d", plan.Metadata, plan.CandleCount)
	}
}

func TestPineWorkerReplayPlannerRejectsInvalidCommandBarIndex(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	planner := PineWorkerReplayPlanner{Adapter: PineWorkerBacktestAdapter{Runner: &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			OrderIntents: []pineworker.OrderIntent{{Kind: "entry", Direction: "long", BarIndex: 2}},
		},
	}}}
	_, err := planner.Plan(context.Background(), PineWorkerReplayPlanRequest{
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		KLines:    []types.KLine{testReplayKLine(start, 10, 12, 9, 11)},
	})
	if err == nil || !strings.Contains(err.Error(), "outside candle range") {
		t.Fatalf("error = %v, want range error", err)
	}
}

func TestPineWorkerReplayPlannerPropagatesWorkerError(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	planner := PineWorkerReplayPlanner{Adapter: PineWorkerBacktestAdapter{Runner: &fakePineWorkerBacktestRunner{
		err: errors.New("deadline exceeded"),
	}}}
	_, err := planner.Plan(context.Background(), PineWorkerReplayPlanRequest{
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		KLines:    []types.KLine{testReplayKLine(start, 10, 12, 9, 11)},
	})
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error = %v, want worker error", err)
	}
}

func commandIDs(commands []WorkerOrderCommand) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		result = append(result, command.ID)
	}
	return result
}

func testReplayKLine(start time.Time, open, high, low, close float64) types.KLine {
	return types.KLine{
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(open),
		High:      fixedpoint.NewFromFloat(high),
		Low:       fixedpoint.NewFromFloat(low),
		Close:     fixedpoint.NewFromFloat(close),
		Volume:    fixedpoint.NewFromFloat(100),
	}
}
