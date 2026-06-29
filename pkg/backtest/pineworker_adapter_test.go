package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestCommandsFromOrderIntents(t *testing.T) {
	intents := []pineworker.OrderIntent{
		{Kind: "entry", ID: "long", Direction: "long", Quantity: 2, HasQuantity: true, BarIndex: 1, Time: 100},
		{Kind: "exit", ID: "exit", FromEntry: "long", Direction: "long", LimitPrice: 12, HasLimitPrice: true, QuantityPct: 50, HasQuantityPct: true},
		{Kind: "cancel_all", BarIndex: 2, Time: 200},
	}
	commands, err := CommandsFromOrderIntents(intents)
	if err != nil {
		t.Fatalf("CommandsFromOrderIntents error = %v", err)
	}
	if len(commands) != 3 {
		t.Fatalf("commands len = %d, want 3", len(commands))
	}
	if commands[0].Side != types.SideTypeBuy || commands[0].Quantity != 2 || commands[0].OrderType != types.OrderTypeMarket {
		t.Fatalf("entry command = %#v", commands[0])
	}
	if commands[1].Kind != "exit" || commands[1].Side != types.SideTypeSell || commands[1].OrderType != types.OrderTypeLimit || commands[1].QuantityPct != 50 {
		t.Fatalf("exit command = %#v", commands[1])
	}
	if commands[2].Kind != "cancel_all" || commands[2].Side != "" {
		t.Fatalf("cancel command = %#v", commands[2])
	}
}

func TestCommandFromOrderIntentPreservesShortDirection(t *testing.T) {
	command, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:        "entry",
		ID:          "short",
		Direction:   "short",
		Quantity:    2,
		HasQuantity: true,
	})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
	}
	if command.Direction != "short" || command.Side != types.SideTypeSell {
		t.Fatalf("command = %#v, want short sell", command)
	}
}

func TestCommandFromOrderIntentDefaultsEntryQuantity(t *testing.T) {
	command, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{Kind: "entry", Direction: "long"})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
	}
	if command.Quantity != 1 {
		t.Fatalf("Quantity = %f, want default 1", command.Quantity)
	}
}

func TestCommandFromOrderIntentRejectsUnsupportedIntent(t *testing.T) {
	_, _, err := CommandFromOrderIntent(pineworker.OrderIntent{Kind: "bracket"})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v, want unsupported", err)
	}
	_, _, err = CommandFromOrderIntent(pineworker.OrderIntent{Kind: "entry", Direction: ""})
	if err == nil || !strings.Contains(err.Error(), "requires long/short") {
		t.Fatalf("error = %v, want direction", err)
	}
}

func TestPineWorkerBacktestAdapterRun(t *testing.T) {
	runner := &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			JobID: "job-1",
			OrderIntents: []pineworker.OrderIntent{{
				Kind:        "entry",
				ID:          "long",
				Direction:   "long",
				Quantity:    1,
				HasQuantity: true,
			}},
			Metadata: pineworker.WorkerMetadata{WorkerID: "worker-1", Duration: time.Millisecond},
		},
	}
	adapter := PineWorkerBacktestAdapter{Runner: runner}
	commands, metadata, err := adapter.Run(context.Background(), validWorkerBacktestRequest())
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if runner.request.Mode != pineworker.ModeBacktest {
		t.Fatalf("Mode = %q, want backtest", runner.request.Mode)
	}
	if len(commands) != 1 || commands[0].ID != "long" {
		t.Fatalf("commands = %#v", commands)
	}
	if metadata.WorkerID != "worker-1" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestPineWorkerBacktestAdapterMapsErrors(t *testing.T) {
	_, _, err := (PineWorkerBacktestAdapter{}).Run(context.Background(), validWorkerBacktestRequest())
	if err == nil || !strings.Contains(err.Error(), "runner is required") {
		t.Fatalf("nil runner error = %v", err)
	}

	adapter := PineWorkerBacktestAdapter{Runner: &fakePineWorkerBacktestRunner{err: errors.New("timeout")}}
	_, _, err = adapter.Run(context.Background(), validWorkerBacktestRequest())
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("transport error = %v", err)
	}

	adapter = PineWorkerBacktestAdapter{Runner: &fakePineWorkerBacktestRunner{response: pineworker.RunScriptResponse{Error: "compile failed"}}}
	_, _, err = adapter.Run(context.Background(), validWorkerBacktestRequest())
	if err == nil || !strings.Contains(err.Error(), "compile failed") {
		t.Fatalf("worker error = %v", err)
	}
}

type fakePineWorkerBacktestRunner struct {
	request  pineworker.RunScriptRequest
	response pineworker.RunScriptResponse
	err      error
}

func (runner *fakePineWorkerBacktestRunner) RunScript(ctx context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	runner.request = request
	return runner.response, runner.err
}

func validWorkerBacktestRequest() pineworker.RunScriptRequest {
	return pineworker.RunScriptRequest{
		JobID:     "job-1",
		Source:    `//@version=6 strategy("x")`,
		Symbol:    "US.AAPL",
		Timeframe: "1",
		Candles: []pineworker.Candle{{
			OpenTime: 1,
			Open:     10,
			High:     12,
			Low:      9,
			Close:    11,
			Volume:   100,
		}},
	}
}
