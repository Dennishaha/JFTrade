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

func TestPineWorkerReplayPumpConsumesThenExecutesCommands(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		testReplayKLine(start, 10, 11, 9, 10),
		testReplayKLine(start.Add(time.Minute), 10, 12, 9, 11),
	}
	plan, err := NewPineWorkerReplayPlan(
		pineworker.RunScriptRequest{Candles: CandlesFromKLines(klines)},
		[]WorkerOrderCommand{{Kind: "entry", ID: "long", Side: types.SideTypeBuy, OrderType: types.OrderTypeMarket, Quantity: 1, BarIndex: 0}},
		pineworker.WorkerMetadata{},
	)
	if err != nil {
		t.Fatalf("NewPineWorkerReplayPlan error = %v", err)
	}
	orderExecutor := &recordingWorkerOrderExecutor{}
	consumer := &recordingKLineConsumer{orderExecutor: orderExecutor}
	pump := &PineWorkerReplayPump{
		Plan: plan,
		CommandExecutor: &PineWorkerCommandExecutor{
			Symbol:         "US.AAPL",
			OrderExecutor:  orderExecutor,
			MarketResolver: fakeWorkerMarketResolver{"US.AAPL": {Symbol: "US.AAPL"}},
		},
		Consumer: consumer,
		Interval: types.Interval1m,
	}

	if err := pump.Consume(context.Background(), klines[0]); err != nil {
		t.Fatalf("Consume(0) error = %v", err)
	}
	if err := pump.Consume(context.Background(), klines[1]); err != nil {
		t.Fatalf("Consume(1) error = %v", err)
	}
	if err := pump.Finish(); err != nil {
		t.Fatalf("Finish error = %v", err)
	}
	if got := strings.Join(orderExecutor.events, ","); got != "consume:0,submit:long,consume:1" {
		t.Fatalf("events = %s", got)
	}
	if consumer.intervals[0] != types.Interval1m || pump.ConsumedBars() != 2 {
		t.Fatalf("intervals=%#v consumed=%d", consumer.intervals, pump.ConsumedBars())
	}
}

func TestPineWorkerReplayPumpValidatesReplayShape(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{testReplayKLine(start, 10, 11, 9, 10)}
	plan, err := NewPineWorkerReplayPlan(
		pineworker.RunScriptRequest{Candles: CandlesFromKLines(klines)},
		nil,
		pineworker.WorkerMetadata{},
	)
	if err != nil {
		t.Fatalf("NewPineWorkerReplayPlan error = %v", err)
	}
	pump := &PineWorkerReplayPump{
		Plan:            plan,
		CommandExecutor: validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{}),
		Consumer:        &recordingKLineConsumer{},
		Interval:        types.Interval1m,
	}
	err = pump.Consume(context.Background(), testReplayKLine(start.Add(time.Minute), 10, 11, 9, 10))
	if err == nil || !strings.Contains(err.Error(), "does not match planned candle") {
		t.Fatalf("open-time error = %v", err)
	}
	if err := pump.Consume(context.Background(), klines[0]); err != nil {
		t.Fatalf("Consume expected kline error = %v", err)
	}
	err = pump.Consume(context.Background(), klines[0])
	if err == nil || !strings.Contains(err.Error(), "extra kline") {
		t.Fatalf("extra kline error = %v", err)
	}
}

func TestPineWorkerReplayPumpFinishDetectsMissingBars(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		testReplayKLine(start, 10, 11, 9, 10),
		testReplayKLine(start.Add(time.Minute), 10, 12, 9, 11),
	}
	plan, err := NewPineWorkerReplayPlan(
		pineworker.RunScriptRequest{Candles: CandlesFromKLines(klines)},
		nil,
		pineworker.WorkerMetadata{},
	)
	if err != nil {
		t.Fatalf("NewPineWorkerReplayPlan error = %v", err)
	}
	pump := &PineWorkerReplayPump{
		Plan:            plan,
		CommandExecutor: validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{}),
		Consumer:        &recordingKLineConsumer{},
		Interval:        types.Interval1m,
	}
	if err := pump.Consume(context.Background(), klines[0]); err != nil {
		t.Fatalf("Consume error = %v", err)
	}
	err = pump.Finish()
	if err == nil || !strings.Contains(err.Error(), "expected 2") {
		t.Fatalf("Finish error = %v, want missing bars", err)
	}
}

func TestPineWorkerReplayPumpPropagatesCommandErrors(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{testReplayKLine(start, 10, 11, 9, 10)}
	plan, err := NewPineWorkerReplayPlan(
		pineworker.RunScriptRequest{Candles: CandlesFromKLines(klines)},
		[]WorkerOrderCommand{{Kind: "entry", ID: "long", Side: types.SideTypeBuy, OrderType: types.OrderTypeMarket, Quantity: 1, BarIndex: 0}},
		pineworker.WorkerMetadata{},
	)
	if err != nil {
		t.Fatalf("NewPineWorkerReplayPlan error = %v", err)
	}
	pump := &PineWorkerReplayPump{
		Plan: plan,
		CommandExecutor: &PineWorkerCommandExecutor{
			Symbol:         "US.AAPL",
			OrderExecutor:  &fakeWorkerOrderExecutor{submitErr: errors.New("submit failed")},
			MarketResolver: fakeWorkerMarketResolver{"US.AAPL": {Symbol: "US.AAPL"}},
		},
		Consumer: &recordingKLineConsumer{},
		Interval: types.Interval1m,
	}
	err = pump.Consume(context.Background(), klines[0])
	if err == nil || !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("Consume error = %v, want command error", err)
	}
}

type recordingKLineConsumer struct {
	orderExecutor *recordingWorkerOrderExecutor
	intervals     []types.Interval
}

func (consumer *recordingKLineConsumer) ConsumeKLine(_ types.KLine, interval types.Interval) {
	consumer.intervals = append(consumer.intervals, interval)
	if consumer.orderExecutor != nil {
		consumer.orderExecutor.events = append(consumer.orderExecutor.events, "consume:"+string(rune('0'+len(consumer.intervals)-1)))
	}
}

type recordingWorkerOrderExecutor struct {
	fakeWorkerOrderExecutor
	events []string
}

func (executor *recordingWorkerOrderExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	created, err := executor.fakeWorkerOrderExecutor.SubmitOrders(ctx, orders...)
	for _, order := range orders {
		executor.events = append(executor.events, "submit:"+order.ClientOrderID)
	}
	return created, err
}
