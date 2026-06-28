package backtest

import (
	"fmt"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func CollectPineWorkerReplayKLines(
	streamer klineRangeStreamer,
	since time.Time,
	until time.Time,
	exchange types.Exchange,
	symbol string,
	interval types.Interval,
) ([]types.KLine, error) {
	if streamer == nil {
		return nil, fmt.Errorf("pine worker replay streamer is required")
	}
	if symbol == "" {
		return nil, fmt.Errorf("pine worker replay symbol is required")
	}
	if interval == "" {
		return nil, fmt.Errorf("pine worker replay interval is required")
	}
	klines := make([]types.KLine, 0, estimateReplayBarCapacity(since, until, interval))
	err := streamer.StreamKLines(since, until, exchange, []string{symbol}, []types.Interval{interval}, func(kline types.KLine) {
		if kline.Symbol == symbol && kline.Interval == interval {
			klines = append(klines, kline)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("collect pine worker replay klines: %w", err)
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("pine worker replay has no klines for %s %s", symbol, interval)
	}
	return klines, nil
}
