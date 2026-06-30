package backtest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func TestCollectPineWorkerReplayKLines(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	want := []types.KLine{
		testReplayKLine(start, 10, 11, 9, 10),
		testReplayKLine(start.Add(time.Minute), 10, 12, 9, 11),
	}
	streamer := &fakePineWorkerReplayStreamer{
		klines: append(want, types.KLine{
			Symbol:   "US.MSFT",
			Interval: types.Interval1m,
		}),
	}
	got, err := CollectPineWorkerReplayKLines(streamer, start, start.Add(time.Minute), nil, "US.AAPL", types.Interval1m)
	if err != nil {
		t.Fatalf("CollectPineWorkerReplayKLines error = %v", err)
	}
	if len(got) != len(want) || got[0].Symbol != "US.AAPL" || got[1].StartTime.Time() != want[1].StartTime.Time() {
		t.Fatalf("klines = %#v", got)
	}
	if streamer.symbols[0] != "US.AAPL" || streamer.intervals[0] != types.Interval1m {
		t.Fatalf("stream request symbols=%#v intervals=%#v", streamer.symbols, streamer.intervals)
	}
}

func TestCollectPineWorkerReplayKLinesMapsErrors(t *testing.T) {
	_, err := CollectPineWorkerReplayKLines(nil, time.Time{}, time.Time{}, nil, "US.AAPL", types.Interval1m)
	if err == nil || !strings.Contains(err.Error(), "streamer is required") {
		t.Fatalf("nil streamer error = %v", err)
	}
	_, err = CollectPineWorkerReplayKLines(&fakePineWorkerReplayStreamer{err: errors.New("stream failed")}, time.Time{}, time.Time{}, nil, "US.AAPL", types.Interval1m)
	if err == nil || !strings.Contains(err.Error(), "stream failed") {
		t.Fatalf("stream error = %v", err)
	}
	_, err = CollectPineWorkerReplayKLines(&fakePineWorkerReplayStreamer{}, time.Time{}, time.Time{}, nil, "US.AAPL", types.Interval1m)
	if err == nil || !strings.Contains(err.Error(), "no klines") {
		t.Fatalf("empty error = %v", err)
	}
}

func TestPineWorkerReplayKLineBatchUsesFixedChunksAndExactResultCapacity(t *testing.T) {
	start := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	streamer := &fakePineWorkerReplayStreamer{klines: make([]types.KLine, pineWorkerReplayKLineChunkSize+1)}
	for index := range streamer.klines {
		streamer.klines[index] = testReplayKLine(start.Add(time.Duration(index)*time.Minute), 10, 12, 9, 11)
	}
	batch, err := collectPineWorkerReplayKLineBatch(
		streamer,
		start,
		start.Add(time.Duration(len(streamer.klines))*time.Minute),
		nil,
		"US.AAPL",
		types.Interval1m,
	)
	if err != nil {
		t.Fatalf("collectPineWorkerReplayKLineBatch error = %v", err)
	}
	if batch.Len() != pineWorkerReplayKLineChunkSize+1 || len(batch.chunks) != 2 {
		t.Fatalf("batch length/chunks = %d/%d", batch.Len(), len(batch.chunks))
	}
	if len(batch.chunks[0]) != pineWorkerReplayKLineChunkSize || len(batch.chunks[1]) != 1 {
		t.Fatalf("chunk lengths = %d/%d", len(batch.chunks[0]), len(batch.chunks[1]))
	}
	last, ok := batch.At(batch.Len() - 1)
	if !ok || !last.StartTime.Time().Equal(streamer.klines[len(streamer.klines)-1].StartTime.Time()) {
		t.Fatalf("last kline = %#v, ok=%v", last, ok)
	}
	warmupUntil := streamer.klines[100].EndTime.Time()
	if got, want := batch.resultCapacity(warmupUntil), batch.Len()-101; got != want {
		t.Fatalf("result capacity = %d, want %d", got, want)
	}
	flat := batch.flatten()
	if len(flat) != batch.Len() || !flat[len(flat)-1].StartTime.Time().Equal(last.StartTime.Time()) {
		t.Fatalf("flattened batch length/last = %d/%#v", len(flat), flat[len(flat)-1])
	}
}

type fakePineWorkerReplayStreamer struct {
	klines    []types.KLine
	err       error
	symbols   []string
	intervals []types.Interval
}

func (streamer *fakePineWorkerReplayStreamer) StreamKLines(
	_ time.Time,
	_ time.Time,
	_ types.Exchange,
	symbols []string,
	intervals []types.Interval,
	emit func(types.KLine),
) error {
	streamer.symbols = symbols
	streamer.intervals = intervals
	if streamer.err != nil {
		return streamer.err
	}
	for _, kline := range streamer.klines {
		emit(kline)
	}
	return nil
}
