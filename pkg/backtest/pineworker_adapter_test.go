package backtest

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

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

func TestCommandFromOrderIntentMapsShortExitToBuy(t *testing.T) {
	command, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:         "exit",
		ID:           "short-stop",
		FromEntry:    "short",
		Direction:    "short",
		StopPrice:    110,
		HasStopPrice: true,
	})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
	}
	if command.Direction != "short" || command.Side != types.SideTypeBuy || command.OrderType != types.OrderTypeStopMarket {
		t.Fatalf("command = %#v, want short stop-market buy", command)
	}
}

func TestScopedExitQuantitySurvivesGoExecution(t *testing.T) {
	command, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:         "exit",
		ID:           "close-a-half",
		FromEntry:    "A",
		Direction:    "long",
		Quantity:     1.5,
		HasQuantity:  true,
		StopPrice:    95,
		HasStopPrice: true,
	})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
	}

	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(7),
		},
		Status: types.OrderStatusFilled, ExecutedQuantity: fixedpoint.NewFromFloat(7),
	})
	orders := &fakeWorkerOrderExecutor{}
	executor := validPineWorkerCommandExecutor(orders)
	executor.PositionSizer = sizer
	if err := executor.Execute(context.Background(), command); err != nil {
		t.Fatalf("Execute scoped exit: %v", err)
	}
	if len(orders.submitted) != 1 {
		t.Fatalf("submitted = %#v, want one scoped exit", orders.submitted)
	}
	order := orders.submitted[0]
	if order.Quantity.Float64() != 1.5 || order.Side != types.SideTypeSell || order.Type != types.OrderTypeStopMarket {
		t.Fatalf("submitted order = %#v, want quantity 1.5 stop-market sell despite net position 7", order)
	}
}

func TestCommandFromOrderIntentMapsConditionalOrderTypes(t *testing.T) {
	tests := []struct {
		name      string
		intent    pineworker.OrderIntent
		orderType types.OrderType
	}{
		{
			name: "entry stop market",
			intent: pineworker.OrderIntent{
				Kind: "entry", Direction: "long", StopPrice: 101, HasStopPrice: true,
			},
			orderType: types.OrderTypeStopMarket,
		},
		{
			name: "exit stop market",
			intent: pineworker.OrderIntent{
				Kind: "exit", Direction: "long", StopPrice: 95, HasStopPrice: true,
			},
			orderType: types.OrderTypeStopMarket,
		},
		{
			name: "entry stop limit",
			intent: pineworker.OrderIntent{
				Kind: "entry", Direction: "long", LimitPrice: 102, HasLimitPrice: true,
				StopPrice: 101, HasStopPrice: true,
			},
			orderType: types.OrderTypeStopLimit,
		},
		{
			name: "order stop limit",
			intent: pineworker.OrderIntent{
				Kind: "order", Direction: "short", LimitPrice: 98, HasLimitPrice: true,
				StopPrice: 99, HasStopPrice: true,
			},
			orderType: types.OrderTypeStopLimit,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, ok, err := CommandFromOrderIntent(test.intent)
			if err != nil || !ok {
				t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
			}
			if command.OrderType != test.orderType || command.LimitPrice != test.intent.LimitPrice || command.StopPrice != test.intent.StopPrice {
				t.Fatalf("command = %#v, want type %s with unchanged prices", command, test.orderType)
			}
		})
	}
}

func TestCommandFromOrderIntentRejectsUnsupportedExitBracket(t *testing.T) {
	_, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:          "exit",
		Direction:     "long",
		LimitPrice:    110,
		HasLimitPrice: true,
		StopPrice:     90,
		HasStopPrice:  true,
	})
	if err == nil || ok || !strings.Contains(err.Error(), "OCO bracket") {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v, want unsupported OCO bracket", err, ok)
	}
}

func TestCommandsFromOrderIntentsExpandsAtomicOCOExit(t *testing.T) {
	commands, err := CommandsFromOrderIntents([]pineworker.OrderIntent{{
		Kind: "exit", ID: "protect", FromEntry: "long", ParentID: "long", Direction: "long",
		LimitPrice: 110, HasLimitPrice: true, StopPrice: 95, HasStopPrice: true,
		Quantity: 1, HasQuantity: true, AtomicGroupID: "bracket-1", OCOGroupID: "protect-oco", ReduceOnly: true,
	}})
	if err != nil {
		t.Fatalf("CommandsFromOrderIntents: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v, want two OCO legs", commands)
	}
	if commands[0].ID != "protect:limit" || commands[0].IntentID != "protect" || commands[0].OrderType != types.OrderTypeLimit || commands[0].StopPrice != 0 {
		t.Fatalf("limit leg = %#v", commands[0])
	}
	if commands[1].ID != "protect:stop" || commands[1].IntentID != "protect" || commands[1].OrderType != types.OrderTypeStopMarket || commands[1].LimitPrice != 0 {
		t.Fatalf("stop leg = %#v", commands[1])
	}
	for _, command := range commands {
		if command.ParentID != "long" || command.AtomicGroupID != "bracket-1" || command.OCOGroupID != "protect-oco" || !command.ReduceOnly {
			t.Fatalf("unsafe OCO leg = %#v", command)
		}
	}

	_, err = CommandsFromOrderIntents([]pineworker.OrderIntent{{
		Kind: "exit", ID: "unsafe", Direction: "long",
		LimitPrice: 110, HasLimitPrice: true, StopPrice: 95, HasStopPrice: true,
	}})
	if err == nil || !strings.Contains(err.Error(), "requires oco and atomic group ids") {
		t.Fatalf("unsafe bracket error = %v", err)
	}
}

func TestCommandFromOrderIntentCanonicalizesSellEntryAsShort(t *testing.T) {
	command, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:        "entry",
		ID:          "sell-entry",
		Direction:   "sell",
		Quantity:    1,
		HasQuantity: true,
	})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent error = %v ok=%v", err, ok)
	}
	if command.Direction != "short" || command.Side != types.SideTypeSell {
		t.Fatalf("command = %#v, want canonical short sell", command)
	}

	closeCommand, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind:        "close",
		ID:          "cover",
		Direction:   "cover",
		Quantity:    1,
		HasQuantity: true,
	})
	if err != nil || !ok {
		t.Fatalf("CommandFromOrderIntent close error = %v ok=%v", err, ok)
	}
	if closeCommand.Direction != "short" || closeCommand.Side != types.SideTypeBuy {
		t.Fatalf("close command = %#v, want canonical short cover", closeCommand)
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

	invalidPrices := []pineworker.OrderIntent{
		{Kind: "entry", Direction: "long", LimitPrice: math.Inf(1), HasLimitPrice: true},
		{Kind: "entry", Direction: "long", StopPrice: math.NaN(), HasStopPrice: true},
	}
	for _, intent := range invalidPrices {
		if _, ok, priceErr := CommandFromOrderIntent(intent); priceErr == nil || ok {
			t.Fatalf("CommandFromOrderIntent(%#v) = ok %v, error %v; want invalid-price rejection", intent, ok, priceErr)
		}
	}

	invalidRelationships := []pineworker.OrderIntent{
		{Kind: "entry", ID: "reduce-entry", Direction: "long", ReduceOnly: true},
		{Kind: "entry", ID: "child-entry", Direction: "long", ParentID: "parent"},
		{Kind: "exit", ID: "unscoped-oco", Direction: "long", OCOGroupID: "oco"},
		{Kind: "cancel", ID: "grouped-cancel", AtomicGroupID: "atomic"},
	}
	for _, intent := range invalidRelationships {
		if _, ok, relationshipErr := CommandFromOrderIntent(intent); relationshipErr == nil || ok {
			t.Fatalf("CommandFromOrderIntent(%#v) = ok %v, error %v; want relationship rejection", intent, ok, relationshipErr)
		}
	}

	_, ok, err := CommandFromOrderIntent(pineworker.OrderIntent{
		Kind: "close", Direction: "long", LimitPrice: 101, HasLimitPrice: true,
		StopPrice: 99, HasStopPrice: true,
	})
	if err == nil || ok || !strings.Contains(err.Error(), "cannot combine") {
		t.Fatalf("close bracket error = %v ok=%v, want unsupported combination", err, ok)
	}

	if commands, commandsErr := CommandsFromOrderIntents([]pineworker.OrderIntent{{Kind: "unsupported"}}); commandsErr == nil || commands != nil {
		t.Fatalf("CommandsFromOrderIntents = (%#v, %v), want propagated conversion error", commands, commandsErr)
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
