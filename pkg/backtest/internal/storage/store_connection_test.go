package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestNewFutuKLineStoreAllowsConcurrentReadConnections(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "backtest.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	if got := store.DB().Stats().MaxOpenConnections; got != 8 {
		t.Fatalf("MaxOpenConnections = %d, want 8", got)
	}
}

func TestFutuKLineUnifiedControllerSerializesConcurrentWriters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backtest.db")
	first, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore first: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, first.Close()) })
	second, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore second: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, second.Close()) })

	start := time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	writers := []struct {
		store  *FutuKLineStore
		offset int
	}{
		{first, 0},
		{second, 100},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(writers))
	for _, writer := range writers {
		wg.Go(func() {
			bars := make([]types.KLine, 0, 50)
			for index := range 50 {
				end := start.Add(time.Duration(writer.offset+index) * time.Minute)
				bars = append(bars, types.KLine{
					Symbol:    "US.TME",
					Interval:  types.Interval1m,
					StartTime: types.Time(end.Add(-time.Minute)),
					EndTime:   types.Time(end),
					Open:      fixedpoint.NewFromFloat(1),
					High:      fixedpoint.NewFromFloat(2),
					Low:       fixedpoint.NewFromFloat(1),
					Close:     fixedpoint.NewFromFloat(2),
					Volume:    fixedpoint.NewFromFloat(100),
				})
			}
			errs <- writer.store.InsertKLines(bars, "forward")
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("InsertKLines: %v", err)
		}
	}

	ch, errCh := first.QueryKLinesCh(start.Add(-time.Minute), start.Add(150*time.Minute), nil, []string{"US.TME"}, []types.Interval{types.Interval1m})
	stored := make([]types.KLine, 0, 100)
	for kline := range ch {
		stored = append(stored, kline)
	}
	for err := range errCh {
		if err != nil {
			t.Fatalf("QueryKLinesCh: %v", err)
		}
	}
	if len(stored) != 100 {
		t.Fatalf("stored bars = %d, want 100", len(stored))
	}
}

func TestFutuKLineReadersRunInParallelWithLaterWALWrite(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "backtest.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	start := time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
	initial := testConnectionKLine(start)
	if err := store.InsertKLine(initial, "forward"); err != nil {
		t.Fatalf("InsertKLine(initial): %v", err)
	}

	releaseReaders := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseReaders) }) }
	defer release()
	readerStarted := make(chan struct{}, 2)
	readerErrors := make(chan error, 2)
	for range 2 {
		go func() {
			signalled := false
			readerErrors <- store.StreamKLines(start.Add(-time.Minute), start.Add(time.Minute), nil, []string{"US.TME"}, []types.Interval{types.Interval1m}, func(types.KLine) {
				if !signalled {
					signalled = true
					readerStarted <- struct{}{}
					<-releaseReaders
				}
			})
		}()
	}
	for index := range 2 {
		select {
		case <-readerStarted:
		case <-time.After(time.Second):
			t.Fatalf("parallel reader %d did not start", index+1)
		}
	}

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- store.InsertKLine(testConnectionKLine(start.Add(time.Minute)), "forward")
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("InsertKLine with active readers: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WAL write was blocked by active readers")
	}

	release()
	for range 2 {
		if err := <-readerErrors; err != nil {
			t.Fatalf("StreamKLines: %v", err)
		}
	}
}

func TestFutuKLineReadWaitsForPreviouslyQueuedWrite(t *testing.T) {
	store, err := NewFutuKLineStore(filepath.Join(t.TempDir(), "backtest.db"))
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })
	end := time.Date(2026, 1, 3, 9, 31, 0, 0, time.UTC)
	if err := store.InsertKLine(testConnectionKLine(end), "forward"); err != nil {
		t.Fatalf("InsertKLine: %v", err)
	}

	tx, err := store.DB().BeginWrite(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginWrite: %v", err)
	}
	table := store.writeTableName("US.TME", types.Interval1m, "forward")
	if _, err := tx.ExecContext(context.Background(), `UPDATE `+quoteIdentifier(table)+` SET close = '9'`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("update: %v", err)
	}

	readDone := make(chan struct {
		kline *types.KLine
		err   error
	}, 1)
	go func() {
		kline, queryErr := store.QueryKLine(nil, "US.TME", types.Interval1m, "DESC", 1)
		readDone <- struct {
			kline *types.KLine
			err   error
		}{kline: kline, err: queryErr}
	}()
	select {
	case <-readDone:
		t.Fatal("read passed a previously queued write")
	case <-time.After(30 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	result := <-readDone
	if result.err != nil || result.kline == nil || result.kline.Close.String() != "9" {
		t.Fatalf("read after write = (%#v, %v)", result.kline, result.err)
	}
}

func testConnectionKLine(end time.Time) types.KLine {
	return types.KLine{
		Symbol:    "US.TME",
		Interval:  types.Interval1m,
		StartTime: types.Time(end.Add(-time.Minute)),
		EndTime:   types.Time(end),
		Open:      fixedpoint.NewFromFloat(1),
		High:      fixedpoint.NewFromFloat(2),
		Low:       fixedpoint.NewFromFloat(1),
		Close:     fixedpoint.NewFromFloat(2),
		Volume:    fixedpoint.NewFromFloat(100),
	}
}
