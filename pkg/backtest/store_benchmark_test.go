package backtest

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func BenchmarkFutuKLineStoreInsertBatch(b *testing.B) {
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	baseKLines := buildBenchmarkKLines(baseStart, 2048)

	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), fmt.Sprintf("insert-%d.db", index))
		store, err := NewFutuKLineStore(dbPath)
		if err != nil {
			b.Fatalf("NewFutuKLineStore() error = %v", err)
		}
		batch := make([]types.KLine, len(baseKLines))
		copy(batch, baseKLines)
		symbol := fmt.Sprintf("US.BENCH%06d", index)
		for rowIndex := range batch {
			batch[rowIndex].Symbol = symbol
		}
		b.StartTimer()
		if err := store.InsertKLines(batch, "forward"); err != nil {
			b.Fatalf("InsertKLines() error = %v", err)
		}
		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatalf("store.Close() error = %v", err)
		}
		b.StartTimer()
	}
}

func BenchmarkFutuKLineStoreQueryBackward(b *testing.B) {
	dbPath, _, endTime := seedBenchmarkBacktestStore(b)
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Fatalf("store.Close() error = %v", err)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		rows, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, endTime.Add(time.Second), 512)
		if err != nil {
			b.Fatalf("QueryKLinesBackward() error = %v", err)
		}
		if len(rows) != 512 {
			b.Fatalf("QueryKLinesBackward() len = %d, want 512", len(rows))
		}
	}
}

func BenchmarkFutuKLineStoreQueryKLinesChSingleSeries(b *testing.B) {
	dbPath, startTime, endTime := seedBenchmarkBacktestStore(b)
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Fatalf("store.Close() error = %v", err)
		}
	})

	const expectedRows = 1536
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		ch, errCh := store.QueryKLinesCh(startTime, endTime, nil, []string{"US.AAPL"}, []types.Interval{types.Interval1m})
		rows, err := collectKLinesFromChannels(ch, errCh)
		if err != nil {
			b.Fatalf("QueryKLinesCh() error = %v", err)
		}
		if len(rows) != expectedRows {
			b.Fatalf("QueryKLinesCh() len = %d, want %d", len(rows), expectedRows)
		}
	}
}

func BenchmarkFutuKLineStoreQueryBackwardSessionAwareTwoHour(b *testing.B) {
	dbPath, endTime := seedSessionAwareIntradayBenchmarkStore(b)
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Fatalf("store.Close() error = %v", err)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		rows, err := store.QueryKLinesBackward(nil, "US.AAPL", types.Interval2h, endTime, 64)
		if err != nil {
			b.Fatalf("QueryKLinesBackward(2h) error = %v", err)
		}
		if len(rows) != 64 {
			b.Fatalf("QueryKLinesBackward(2h) len = %d, want 64", len(rows))
		}
	}
}

func seedSessionAwareIntradayBenchmarkStore(b *testing.B) (string, time.Time) {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "benchmark-session-aware.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	klines := buildBenchmarkSessionAwareHalfHourKLines(time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC), 40)
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeErr1 := store.Close()
		jftradeCheckTestError(b, jftradeErr1)
		b.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		b.Fatalf("store.Close() error = %v", err)
	}
	return dbPath, klines[len(klines)-1].EndTime.Time().Add(time.Second)
}

func buildBenchmarkSessionAwareHalfHourKLines(baseDay time.Time, tradingDays int) []types.KLine {
	klines := make([]types.KLine, 0, tradingDays*13)
	price := 100.0
	addedDays := 0
	for dayOffset := 0; addedDays < tradingDays; dayOffset++ {
		day := baseDay.AddDate(0, 0, dayOffset)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		sessionStart := time.Date(day.Year(), day.Month(), day.Day(), 13, 30, 0, 0, time.UTC)
		for slot := 0; slot < 13; slot++ {
			startAt := sessionStart.Add(time.Duration(slot) * 30 * time.Minute)
			cycle := float64((addedDays*13+slot)%17) * 0.15
			openValue := price
			closeValue := openValue + 0.2 + cycle/10
			highValue := closeValue + 0.35
			lowValue := openValue - 0.35
			klines = append(klines, types.KLine{
				StartTime: types.Time(startAt),
				EndTime:   types.Time(startAt.Add(30*time.Minute - time.Millisecond)),
				Interval:  types.Interval30m,
				Symbol:    "US.AAPL",
				Open:      fixedpoint.NewFromFloat(openValue),
				High:      fixedpoint.NewFromFloat(highValue),
				Low:       fixedpoint.NewFromFloat(lowValue),
				Close:     fixedpoint.NewFromFloat(closeValue),
				Volume:    fixedpoint.NewFromFloat(1000 + float64((slot*37)%400)),
			})
			price = closeValue
		}
		addedDays++
	}
	return klines
}
