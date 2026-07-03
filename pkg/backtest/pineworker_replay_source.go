package backtest

import (
	"fmt"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

const pineWorkerReplayKLineChunkSize = 4096

type pineWorkerReplayKLineBatch struct {
	chunks [][]types.KLine
	length int
}

func (batch *pineWorkerReplayKLineBatch) append(kline types.KLine) {
	if len(batch.chunks) == 0 || len(batch.chunks[len(batch.chunks)-1]) == pineWorkerReplayKLineChunkSize {
		batch.chunks = append(batch.chunks, make([]types.KLine, 0, pineWorkerReplayKLineChunkSize))
	}
	last := len(batch.chunks) - 1
	batch.chunks[last] = append(batch.chunks[last], kline)
	batch.length++
}

func (batch *pineWorkerReplayKLineBatch) Len() int {
	if batch == nil {
		return 0
	}
	return batch.length
}

func (batch *pineWorkerReplayKLineBatch) At(index int) (types.KLine, bool) {
	if batch == nil || index < 0 || index >= batch.length {
		return types.KLine{}, false
	}
	return batch.chunks[index/pineWorkerReplayKLineChunkSize][index%pineWorkerReplayKLineChunkSize], true
}

func (batch *pineWorkerReplayKLineBatch) forEach(visit func(types.KLine) bool) {
	if batch == nil || visit == nil {
		return
	}
	for _, chunk := range batch.chunks {
		for _, kline := range chunk {
			if !visit(kline) {
				return
			}
		}
	}
}

func (batch *pineWorkerReplayKLineBatch) flatten() []types.KLine {
	if batch.Len() == 0 {
		return nil
	}
	klines := make([]types.KLine, 0, batch.length)
	batch.forEach(func(kline types.KLine) bool {
		klines = append(klines, kline)
		return true
	})
	return klines
}

func (batch *pineWorkerReplayKLineBatch) resultCapacity(warmupUntil time.Time) int {
	count := 0
	index := 0
	batch.forEach(func(kline types.KLine) bool {
		if index == batch.length-1 {
			return false
		}
		index++
		if !kline.EndTime.Time().Before(warmupUntil) {
			count++
		}
		return true
	})
	return count
}

func CollectPineWorkerReplayKLines(
	streamer klineRangeStreamer,
	since time.Time,
	until time.Time,
	exchange types.Exchange,
	symbol string,
	interval types.Interval,
) ([]types.KLine, error) {
	batch, err := collectPineWorkerReplayKLineBatch(streamer, since, until, exchange, symbol, interval)
	if err != nil {
		return nil, err
	}
	return batch.flatten(), nil
}

func collectPineWorkerReplayKLineBatch(
	streamer klineRangeStreamer,
	since time.Time,
	until time.Time,
	exchange types.Exchange,
	symbol string,
	interval types.Interval,
) (*pineWorkerReplayKLineBatch, error) {
	if streamer == nil {
		return nil, fmt.Errorf("pine worker replay streamer is required")
	}
	if symbol == "" {
		return nil, fmt.Errorf("pine worker replay symbol is required")
	}
	if interval == "" {
		return nil, fmt.Errorf("pine worker replay interval is required")
	}
	batch := &pineWorkerReplayKLineBatch{}
	err := streamer.StreamKLines(since, until, exchange, []string{symbol}, []types.Interval{interval}, func(kline types.KLine) {
		if kline.Symbol == symbol && kline.Interval == interval {
			batch.append(kline)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("collect pine worker replay klines: %w", err)
	}
	if batch.Len() == 0 {
		return nil, fmt.Errorf("pine worker replay has no klines for %s %s", symbol, interval)
	}
	return batch, nil
}
