package backtest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type historicalSourceStub struct {
	mu          sync.Mutex
	queries     []HistoricalCandleQuery
	pages       []HistoricalCandlePage
	errors      []error
	validateErr error
}

type historicalSourceFunc func(context.Context, HistoricalCandleQuery) (HistoricalCandlePage, error)

func (fn historicalSourceFunc) FetchHistoricalCandles(
	ctx context.Context,
	query HistoricalCandleQuery,
) (HistoricalCandlePage, error) {
	return fn(ctx, query)
}

func (s *historicalSourceStub) ValidateHistoricalCandleQuery(HistoricalCandleQuery) error {
	return s.validateErr
}

func (s *historicalSourceStub) FetchHistoricalCandles(
	_ context.Context,
	query HistoricalCandleQuery,
) (HistoricalCandlePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = append(s.queries, query)
	if len(s.errors) > 0 {
		err := s.errors[0]
		s.errors = s.errors[1:]
		if err != nil {
			return HistoricalCandlePage{}, err
		}
	}
	if len(s.pages) == 0 {
		return HistoricalCandlePage{}, nil
	}
	page := s.pages[0]
	s.pages = s.pages[1:]
	return page, nil
}

func TestHistoricalKLineSyncerPaginatesBackwardAndIsolatesProvider(t *testing.T) {
	since := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	second := since.Add(time.Minute)
	until := since.Add(2 * time.Minute)
	source := &historicalSourceStub{pages: []HistoricalCandlePage{
		{
			Candles: []HistoricalCandle{
				historicalTestCandle(until, "999"),
				historicalTestCandle(second, "102"),
			},
			HasMore: true, NextBefore: second,
		},
		{Candles: []HistoricalCandle{
			historicalTestCandle(second, "102"),
			historicalTestCandle(since, "101"),
			historicalTestCandle(since.Add(-time.Minute), "998"),
		}},
	}}
	store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "yfinance")
	if err != nil {
		t.Fatalf("NewKLineStore: %v", err)
	}
	syncer := NewHistoricalKLineSyncer(store, source, nil)
	params := KLineSyncParams{
		Market: "US", MarketDataProvider: "yfinance", Symbol: "US.AAPL",
		Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
		Since:     since, Until: until, RehabType: RehabTypeNone, SessionScope: "regular",
	}
	if err := syncer.Sync(t.Context(), params, nil); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	store.SetRehabType("none")
	rows, err := store.QueryKLinesForward(nil, "US.AAPL", bbgotypes.Interval1m, since, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("yfinance rows = %d, err=%v", len(rows), err)
	}
	store.SetProviderID("akshare")
	rows, err = store.QueryKLinesForward(nil, "US.AAPL", bbgotypes.Interval1m, since, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("akshare rows reused yfinance cache = %d, err=%v", len(rows), err)
	}
	source.mu.Lock()
	queries := append([]HistoricalCandleQuery(nil), source.queries...)
	source.mu.Unlock()
	if len(queries) != 2 || !queries[1].Before.Equal(second) {
		t.Fatalf("historical cursors = %#v", queries)
	}
	if err := syncer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestHistoricalKLineSyncerCancelsInFlightProviderPage(t *testing.T) {
	store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "akshare")
	if err != nil {
		t.Fatalf("NewKLineStore: %v", err)
	}
	started := make(chan struct{})
	source := historicalSourceFunc(func(ctx context.Context, _ HistoricalCandleQuery) (HistoricalCandlePage, error) {
		close(started)
		<-ctx.Done()
		return HistoricalCandlePage{}, ctx.Err()
	})
	syncer := NewHistoricalKLineSyncer(store, source, nil)
	ctx, cancel := context.WithCancel(t.Context())
	progress := bt.NewSyncProgress("cancel", "US.AAPL", time.Now().UTC())
	done := make(chan error, 1)
	go func() {
		done <- syncer.Sync(ctx, KLineSyncParams{
			Market: "US", MarketDataProvider: "akshare", Symbol: "US.AAPL",
			Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
			Since:     time.Now().UTC().Add(-time.Hour), Until: time.Now().UTC(),
			RehabType: RehabTypeNone, SessionScope: "regular",
		}, progress)
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Sync cancellation error = %v", err)
	}
	if snapshot := progress.Snapshot(); snapshot.Status != "cancelled" {
		t.Fatalf("cancelled progress = %+v", snapshot)
	}
	if err := syncer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestHistoricalKLineSyncerRetriesTransientPageAndRejectsCapabilitiesDuringPreflight(t *testing.T) {
	since := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "yfinance")
	if err != nil {
		t.Fatalf("NewKLineStore: %v", err)
	}
	source := &historicalSourceStub{
		errors: []error{errors.New("temporary provider failure")},
		pages:  []HistoricalCandlePage{{Candles: []HistoricalCandle{historicalTestCandle(since, "101")}}},
	}
	syncer := NewHistoricalKLineSyncer(store, source, nil)
	params := KLineSyncParams{
		Market: "US", MarketDataProvider: "yfinance", Symbol: "US.AAPL",
		Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
		Since:     since, Until: since.Add(time.Minute), RehabType: RehabTypeNone, SessionScope: "regular",
	}
	progress := bt.NewSyncProgress("retry", params.Symbol, time.Now().UTC())
	if err := syncer.Sync(t.Context(), params, progress); err != nil {
		t.Fatalf("Sync after transient error: %v", err)
	}
	if snapshot := progress.Snapshot(); snapshot.Retries != 1 {
		t.Fatalf("retries = %d, want 1", snapshot.Retries)
	}

	source.validateErr = errors.New("provider yfinance does not support forward price adjustment")
	params.RehabType = RehabTypeForward
	if err := syncer.Validate(params); !IsRequestError(err) {
		t.Fatalf("Validate error = %v, want request error", err)
	}
	if err := syncer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestHistoricalKLineSyncerRejectsEmptyProviderResult(t *testing.T) {
	store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "akshare")
	if err != nil {
		t.Fatalf("NewKLineStore: %v", err)
	}
	syncer := NewHistoricalKLineSyncer(store, &historicalSourceStub{}, nil)
	now := time.Now().UTC()
	err = syncer.Sync(t.Context(), KLineSyncParams{
		Market: "US", MarketDataProvider: "akshare", Symbol: "US.AAPL",
		Intervals: []bbgotypes.Interval{bbgotypes.Interval5m},
		Since:     now.Add(-time.Hour), Until: now, RehabType: RehabTypeNone, SessionScope: "regular",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "returned no candles") {
		t.Fatalf("empty provider result error = %v", err)
	}
}

func TestHistoricalKLineSyncerValidatesLifecycleAndTerminalProviderFailures(t *testing.T) {
	if err := (*HistoricalKLineSyncer)(nil).Validate(KLineSyncParams{}); err == nil {
		t.Fatal("nil syncer validation error = nil")
	}
	if err := (*HistoricalKLineSyncer)(nil).Sync(t.Context(), KLineSyncParams{}, nil); err == nil {
		t.Fatal("nil syncer sync error = nil")
	}
	if err := (*HistoricalKLineSyncer)(nil).Close(); err != nil {
		t.Fatalf("nil syncer close = %v", err)
	}

	store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "yfinance")
	if err != nil {
		t.Fatalf("NewKLineStore: %v", err)
	}
	plainSource := historicalSourceFunc(func(context.Context, HistoricalCandleQuery) (HistoricalCandlePage, error) {
		return HistoricalCandlePage{}, requestErrorf("invalid provider request")
	})
	closeErr := errors.New("lease release failed")
	syncer := NewHistoricalKLineSyncer(store, plainSource, func() error { return closeErr })
	params := KLineSyncParams{
		Market: "US", MarketDataProvider: "yfinance", Symbol: "US.AAPL", Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
		Since: time.Now().UTC().Add(-time.Hour), Until: time.Now().UTC(), RehabType: RehabTypeNone, SessionScope: "extended",
	}
	if err := syncer.Validate(params); err != nil {
		t.Fatalf("source without validator rejected: %v", err)
	}
	progress := bt.NewSyncProgress("request-failure", params.Symbol, time.Now().UTC())
	if err := syncer.Sync(t.Context(), params, progress); !IsRequestError(err) {
		t.Fatalf("request failure = %v", err)
	}
	if snapshot := progress.Snapshot(); snapshot.Status != "failed" {
		t.Fatalf("failed progress = %+v", snapshot)
	}
	if err := syncer.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v", err)
	}
	if err := syncer.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second Close error = %v", err)
	}

	cancelStore, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "cancel.db"), "yfinance")
	if err != nil {
		t.Fatalf("NewKLineStore(cancel): %v", err)
	}
	validated := &historicalSourceStub{}
	cancelSyncer := NewHistoricalKLineSyncer(cancelStore, validated, nil)
	if err := cancelSyncer.Validate(params); err != nil {
		t.Fatalf("successful provider validation: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(t.Context())
	cancel()
	cancelProgress := bt.NewSyncProgress("pre-cancelled", params.Symbol, time.Now().UTC())
	if err := cancelSyncer.Sync(cancelCtx, params, cancelProgress); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled sync error = %v", err)
	}
	if snapshot := cancelProgress.Snapshot(); snapshot.Status != "cancelled" {
		t.Fatalf("pre-cancelled progress = %+v", snapshot)
	}
	if err := cancelSyncer.Close(); err != nil {
		t.Fatalf("cancel syncer Close: %v", err)
	}
}

func TestHistoricalKLineSyncerRejectsBrokenPagination(t *testing.T) {
	since := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		page HistoricalCandlePage
	}{
		{name: "missing cursor", page: HistoricalCandlePage{Candles: []HistoricalCandle{historicalTestCandle(since.Add(time.Minute), "1")}, HasMore: true}},
		{name: "forward cursor", page: HistoricalCandlePage{
			Candles: []HistoricalCandle{historicalTestCandle(since.Add(time.Minute), "1")},
			HasMore: true, NextBefore: since.Add(3 * time.Minute),
		}},
		{name: "cursor reaches boundary", page: HistoricalCandlePage{
			Candles: []HistoricalCandle{historicalTestCandle(since.Add(time.Minute), "1")},
			HasMore: true, NextBefore: since,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := bt.NewKLineStore(filepath.Join(t.TempDir(), "backtest.db"), "futu")
			if err != nil {
				t.Fatalf("NewKLineStore: %v", err)
			}
			syncer := NewHistoricalKLineSyncer(store, &historicalSourceStub{pages: []HistoricalCandlePage{test.page}}, nil)
			err = syncer.Sync(t.Context(), KLineSyncParams{
				Market: "US", MarketDataProvider: "futu", Symbol: "US.AAPL", Intervals: []bbgotypes.Interval{bbgotypes.Interval1m},
				Since: since, Until: since.Add(2 * time.Minute), RehabType: RehabTypeNone, SessionScope: "regular",
			}, nil)
			if test.name == "cursor reaches boundary" {
				if err != nil {
					t.Fatalf("boundary cursor error = %v", err)
				}
			} else if err == nil {
				t.Fatalf("broken pagination error = nil")
			}
			if closeErr := syncer.Close(); closeErr != nil {
				t.Fatalf("Close: %v", closeErr)
			}
		})
	}
}

func TestHistoricalCandleConversionRejectsInvalidFieldsAndDefaultsVolume(t *testing.T) {
	at := time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)
	base := HistoricalCandle{At: at, Open: "1", High: "2", Low: "1", Close: "2", Session: "regular"}
	rows, err := historicalPageKLines("US.AAPL", bbgotypes.Interval1m, at, at.Add(time.Minute), []HistoricalCandle{
		{At: at.Add(-time.Minute)}, base, {At: at.Add(time.Minute)},
	})
	if err != nil || len(rows) != 1 || rows[0].Volume != 0 {
		t.Fatalf("converted rows = %+v, %v", rows, err)
	}
	for _, field := range []string{"open", "high", "low", "close", "volume"} {
		broken := base
		switch field {
		case "open":
			broken.Open = "bad"
		case "high":
			broken.High = "bad"
		case "low":
			broken.Low = "bad"
		case "close":
			broken.Close = "bad"
		case "volume":
			broken.Volume = "bad"
		}
		if _, err := historicalPageKLines("US.AAPL", bbgotypes.Interval1m, at, at.Add(time.Minute), []HistoricalCandle{broken}); err == nil {
			t.Fatalf("invalid %s error = nil", field)
		}
	}
	if got := syncSessions(" EXTENDED "); len(got) != 3 || got[2] != "overnight" {
		t.Fatalf("extended sync sessions = %#v", got)
	}
}

func TestHistoricalProviderRetryExhaustionAndTimerCancellation(t *testing.T) {
	calls := 0
	source := historicalSourceFunc(func(context.Context, HistoricalCandleQuery) (HistoricalCandlePage, error) {
		calls++
		return HistoricalCandlePage{}, errors.New("temporary failure")
	})
	if _, err := fetchHistoricalPageWithRetry(t.Context(), source, HistoricalCandleQuery{}, nil); err == nil || calls != 4 {
		t.Fatalf("retry exhaustion = calls %d, error %v", calls, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if _, err := fetchHistoricalPageWithRetry(ctx, source, HistoricalCandleQuery{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry timer cancellation = %v", err)
	}
}

func historicalTestCandle(at time.Time, closeValue string) HistoricalCandle {
	return HistoricalCandle{
		At: at, Open: "100", High: "103", Low: "99", Close: closeValue, Volume: "1000",
		Session: "regular",
	}
}
