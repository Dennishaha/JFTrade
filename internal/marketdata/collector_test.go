package marketdata

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestCollectorCloseCancelsBlockingConnectAndPreventsRevival(t *testing.T) {
	stream := &collectorStream{connectStarted: make(chan struct{}), connectDone: make(chan struct{})}
	push := &collectorPushSource{streams: []*collectorStream{stream}}
	collector := NewCollector(NewCache(), nil, push, nil, CollectorOptions{
		DemandInterval: time.Hour,
		ConnectTimeout: time.Hour,
	})
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"HK.00700"} }))

	select {
	case <-stream.connectStarted:
	case <-time.After(time.Second):
		t.Fatal("Connect did not start")
	}
	if err := collector.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-stream.connectDone:
	default:
		t.Fatal("blocking Connect was not canceled")
	}
	if state := collector.State(); !state.Closed || state.Connected {
		t.Fatalf("state after Close = %#v", state)
	}
	collector.Wake()
	time.Sleep(10 * time.Millisecond)
	if got := push.newCalls.Load(); got != 1 {
		t.Fatalf("NewStream calls after Close = %d", got)
	}
}

func TestCollectorOldGenerationCannotCommitPushOrConnect(t *testing.T) {
	cache := NewCache()
	first := &collectorStream{connectStarted: make(chan struct{}), release: make(chan struct{})}
	second := &collectorStream{connectStarted: make(chan struct{}), release: make(chan struct{})}
	push := &collectorPushSource{streams: []*collectorStream{first, second}}
	var demand atomic.Value
	demand.Store([]string{"HK.00700"})
	collector := NewCollector(cache, nil, push, nil, CollectorOptions{
		DemandInterval: time.Hour,
		ConnectTimeout: time.Hour,
	})
	t.Cleanup(func() { jftradeErr1 := collector.Close(); jftradeCheckTestError(t, jftradeErr1) })
	collector.SetDemandSources(DemandSourceFunc(func() []string { return jftradeCheckedTypeAssertion[[]string](demand.Load()) }))
	<-first.connectStarted

	demand.Store([]string{"US.AAPL"})
	collector.Wake()
	<-second.connectStarted
	first.handler(testTick("HK.00700", "100", TickKindTrade))
	close(first.release)
	time.Sleep(10 * time.Millisecond)

	if got := cache.Count("HK.00700"); got != 0 {
		t.Fatalf("old generation committed %d ticks", got)
	}
	if state := collector.State(); state.Connected {
		t.Fatalf("old generation marked new stream connected: %#v", state)
	}
	close(second.release)
	waitFor(t, func() bool { return collector.State().Connected })
}

func TestCollectorPollingFallbackDoesNotCallPushHandler(t *testing.T) {
	cache := NewCache()
	quotes := &collectorQuoteSource{ticks: map[string]Tick{
		"HK.00700": testTick("HK.00700", "101", TickKindQuote),
	}}
	var pushes atomic.Int64
	collector := NewCollector(cache, quotes, nil, func(Tick) { pushes.Add(1) }, CollectorOptions{
		PollInterval: time.Hour, QueryTimeout: time.Second, DemandInterval: time.Hour,
	})
	t.Cleanup(func() { jftradeErr4 := collector.Close(); jftradeCheckTestError(t, jftradeErr4) })
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"HK.00700"} }))
	waitFor(t, func() bool { return collector.State().ActiveCount == 1 })
	collector.poll()
	waitFor(t, func() bool { return cache.Count("HK.00700") == 1 })
	if got := pushes.Load(); got != 0 {
		t.Fatalf("polling invoked PushTickHandler %d times", got)
	}
}

func TestCollectorResetInvalidatesBlockingQueryResult(t *testing.T) {
	cache := NewCache()
	quotes := &collectorQuoteSource{
		ticks:   map[string]Tick{"HK.00700": testTick("HK.00700", "102", TickKindQuote)},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	collector := NewCollector(cache, quotes, nil, nil, CollectorOptions{
		PollInterval: time.Hour, QueryTimeout: time.Hour, DemandInterval: time.Hour,
	})
	t.Cleanup(func() { jftradeErr5 := collector.Close(); jftradeCheckTestError(t, jftradeErr5) })
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"HK.00700"} }))
	collector.poll()
	<-quotes.started
	collector.Reset()
	close(quotes.release)
	time.Sleep(10 * time.Millisecond)
	if got := cache.Count("HK.00700"); got != 0 {
		t.Fatalf("old query generation committed %d ticks", got)
	}
}

func TestCollectorStreamFailureBacksOff(t *testing.T) {
	push := &collectorPushSource{}
	collector := NewCollector(NewCache(), nil, push, nil, CollectorOptions{
		DemandInterval: time.Hour,
	})
	t.Cleanup(func() { jftradeErr2 := collector.Close(); jftradeCheckTestError(t, jftradeErr2) })
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"HK.00700"} }))

	waitFor(t, func() bool { return collector.State().StreamFailures == 1 })
	state := collector.State()
	if state.StreamRetryAt.IsZero() || !state.StreamRetryAt.After(time.Now().UTC()) {
		t.Fatalf("StreamRetryAt = %s, want future retry", state.StreamRetryAt)
	}
	if state.StreamLastError == "" {
		t.Fatal("expected stream failure error to be recorded")
	}
	calls := push.newCalls.Load()

	collector.Wake()
	time.Sleep(20 * time.Millisecond)
	if got := push.newCalls.Load(); got != calls {
		t.Fatalf("stream retry was not deferred: calls %d -> %d", calls, got)
	}
}

func TestCollectorQuoteFailureBacksOff(t *testing.T) {
	cache := NewCache()
	quotes := &collectorQuoteSource{err: errors.New("quote unavailable")}
	collector := NewCollector(cache, quotes, nil, nil, CollectorOptions{
		PollInterval:   time.Hour,
		QueryTimeout:   time.Second,
		DemandInterval: time.Hour,
	})
	t.Cleanup(func() { jftradeErr3 := collector.Close(); jftradeCheckTestError(t, jftradeErr3) })
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"HK.00700"} }))
	waitFor(t, func() bool { return collector.State().ActiveCount == 1 })

	collector.poll()
	waitFor(t, func() bool { return collector.State().QuoteFailures == 1 })
	state := collector.State()
	if state.QuoteRetryAt.IsZero() || !state.QuoteRetryAt.After(time.Now().UTC()) {
		t.Fatalf("QuoteRetryAt = %s, want future retry", state.QuoteRetryAt)
	}
	if state.QuoteLastError == "" {
		t.Fatal("expected quote failure error to be recorded")
	}
	calls := quotes.calls.Load()

	collector.poll()
	time.Sleep(20 * time.Millisecond)
	if got := quotes.calls.Load(); got != calls {
		t.Fatalf("quote retry was not deferred: calls %d -> %d", calls, got)
	}
}

func TestRetryDelaySequence(t *testing.T) {
	wants := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second}
	for failures, want := range wants {
		if got := retryDelay(failures); got != want {
			t.Fatalf("retryDelay(%d) = %s, want %s", failures, got, want)
		}
	}
}

type collectorPushSource struct {
	mu       sync.Mutex
	streams  []*collectorStream
	newCalls atomic.Int64
}

func (s *collectorPushSource) NewStream(_ []string, handler PushTickHandler) (PushStream, error) {
	s.newCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) == 0 {
		return nil, errors.New("no stream")
	}
	stream := s.streams[0]
	s.streams = s.streams[1:]
	stream.handler = handler
	return stream, nil
}

type collectorStream struct {
	handler        PushTickHandler
	connectStarted chan struct{}
	connectDone    chan struct{}
	release        chan struct{}
	closeOnce      sync.Once
}

func (s *collectorStream) Connect(ctx context.Context) error {
	close(s.connectStarted)
	if s.release != nil {
		select {
		case <-ctx.Done():
			if s.connectDone != nil {
				close(s.connectDone)
			}
			return ctx.Err()
		case <-s.release:
			return nil
		}
	}
	<-ctx.Done()
	if s.connectDone != nil {
		close(s.connectDone)
	}
	return ctx.Err()
}

func (s *collectorStream) Close() error {
	s.closeOnce.Do(func() {})
	return nil
}

type collectorQuoteSource struct {
	ticks   map[string]Tick
	err     error
	started chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (s *collectorQuoteSource) QueryTickers(ctx context.Context, _ []string) (map[string]Tick, error) {
	s.calls.Add(1)
	if s.started != nil {
		close(s.started)
	}
	if s.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.release:
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.ticks, nil
}

func testTick(instrumentID, price string, kind TickKind) Tick {
	normalized, market, symbol, _ := NormalizeInstrumentID(instrumentID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value := decimal.RequireFromString(price)
	return Tick{
		InstrumentID: normalized, Market: market, Symbol: symbol,
		Price: value, Bid: value, Ask: value, QuoteAt: now, ObservedAt: now,
		Source: "test", Kind: kind,
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(time.Millisecond)
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
