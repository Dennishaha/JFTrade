package backtest

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected cleanup error: %v", err)
	}
}

func collectKLinesFromChannels(ch chan types.KLine, errCh chan error) ([]types.KLine, error) {
	rows := make([]types.KLine, 0)
	for ch != nil || errCh != nil {
		select {
		case kline, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			rows = append(rows, kline)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return rows, nil
}

func seedBenchmarkBacktestStore(b *testing.B) (string, time.Time, time.Time) {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "benchmark-backtest.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := buildBenchmarkKLines(baseStart, 2048)
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeCheckTestError(b, store.Close())
		b.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		b.Fatalf("store.Close() error = %v", err)
	}
	startIndex := 512
	return dbPath, klines[startIndex].StartTime.Time(), klines[len(klines)-1].EndTime.Time()
}

func buildBenchmarkKLines(baseStart time.Time, count int) []types.KLine {
	klines := make([]types.KLine, 0, count)
	previousClose := 100.0
	for index := range count {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		cycle := math.Sin(float64(index)/18.0)*4 + math.Cos(float64(index)/7.0)*1.5
		drift := float64(index%97) / 97.0 * 0.4
		closeValue := 100 + cycle + drift
		openValue := previousClose
		highValue := math.Max(openValue, closeValue) + 0.75 + float64(index%5)*0.03
		lowValue := math.Min(openValue, closeValue) - 0.75 - float64(index%7)*0.02
		klines = append(klines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(highValue),
			Low:       fixedpoint.NewFromFloat(lowValue),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64((index*37)%400)),
		})
		previousClose = closeValue
	}
	return klines
}
