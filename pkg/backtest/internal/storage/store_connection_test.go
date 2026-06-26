package storage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
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

func TestFutuKLineStoresWithSamePathShareAccessQueue(t *testing.T) {
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

	if first.accessQueue == nil || first.accessQueue != second.accessQueue {
		t.Fatal("stores for the same sqlite path do not share one access queue")
	}
}

func TestFutuKLineSharedAccessQueueSerializesConcurrentWriters(t *testing.T) {
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

func TestKLineAccessQueueAllowsConsecutiveReadsBeforeWrites(t *testing.T) {
	queue := newKLineAccessQueue()
	readRelease := make(chan struct{})
	writeRelease := make(chan struct{})
	readOneStarted := make(chan struct{})
	readTwoStarted := make(chan struct{})
	writeStarted := make(chan struct{})
	readAfterWriteStarted := make(chan struct{})
	errs := make(chan error, 4)

	waitForStart := func(name string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("%s did not start", name)
		}
	}
	assertNotStarted := func(name string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
			t.Fatalf("%s started too early", name)
		case <-time.After(50 * time.Millisecond):
		}
	}

	go func() {
		errs <- queue.enqueueRead(func() error {
			close(readOneStarted)
			<-readRelease
			return nil
		})
	}()
	waitForStart("first read", readOneStarted)

	go func() {
		errs <- queue.enqueueRead(func() error {
			close(readTwoStarted)
			<-readRelease
			return nil
		})
	}()
	waitForStart("second read", readTwoStarted)

	go func() {
		errs <- queue.enqueueWrite(func() error {
			close(writeStarted)
			<-writeRelease
			return nil
		})
	}()
	assertNotStarted("write", writeStarted)

	go func() {
		errs <- queue.enqueueRead(func() error {
			close(readAfterWriteStarted)
			return nil
		})
	}()
	assertNotStarted("read after write", readAfterWriteStarted)

	close(readRelease)
	waitForStart("write", writeStarted)
	assertNotStarted("read after write", readAfterWriteStarted)

	close(writeRelease)
	waitForStart("read after write", readAfterWriteStarted)

	for index := 0; index < cap(errs); index++ {
		if err := <-errs; err != nil {
			t.Fatalf("queued operation %d: %v", index, err)
		}
	}
}
