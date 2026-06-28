package backtest

import (
	"context"
	"fmt"

	"github.com/c9s/bbgo/pkg/types"
)

type PineWorkerKLineConsumer interface {
	ConsumeKLine(types.KLine, types.Interval)
}

type PineWorkerReplayPump struct {
	Plan            PineWorkerReplayPlan
	CommandExecutor *PineWorkerCommandExecutor
	Consumer        PineWorkerKLineConsumer
	Interval        types.Interval

	nextBarIndex int
}

func (pump *PineWorkerReplayPump) Consume(ctx context.Context, kline types.KLine) error {
	if pump.Consumer == nil {
		return fmt.Errorf("pine worker replay kline consumer is required")
	}
	if pump.CommandExecutor == nil {
		return fmt.Errorf("pine worker replay command executor is required")
	}
	if pump.nextBarIndex >= pump.Plan.CandleCount {
		return fmt.Errorf("pine worker replay received extra kline at index %d", pump.nextBarIndex)
	}
	if err := pump.validateKLine(kline); err != nil {
		return err
	}

	barIndex := pump.nextBarIndex
	pump.Consumer.ConsumeKLine(kline, pump.Interval)
	pump.nextBarIndex++

	if commands := pump.Plan.ByBarIndex[barIndex]; len(commands) > 0 {
		if err := pump.CommandExecutor.ExecuteBarCommands(ctx, commands); err != nil {
			return fmt.Errorf("execute pine worker commands for bar %d: %w", barIndex, err)
		}
	}
	return nil
}

func (pump *PineWorkerReplayPump) Finish() error {
	if pump.nextBarIndex != pump.Plan.CandleCount {
		return fmt.Errorf("pine worker replay consumed %d bars, expected %d", pump.nextBarIndex, pump.Plan.CandleCount)
	}
	return nil
}

func (pump *PineWorkerReplayPump) ConsumedBars() int {
	return pump.nextBarIndex
}

func (pump *PineWorkerReplayPump) validateKLine(kline types.KLine) error {
	if len(pump.Plan.Request.Candles) == 0 {
		return fmt.Errorf("pine worker replay plan has no candles")
	}
	expected := pump.Plan.Request.Candles[pump.nextBarIndex]
	openTime := kline.StartTime.Time().UnixMilli()
	if expected.OpenTime > 0 && openTime != expected.OpenTime {
		return fmt.Errorf("pine worker replay kline %d open time %d does not match planned candle %d", pump.nextBarIndex, openTime, expected.OpenTime)
	}
	return nil
}
