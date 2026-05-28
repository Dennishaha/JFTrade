package backtest

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

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
