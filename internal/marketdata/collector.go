package marketdata

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

const (
	FallbackPollInterval  = time.Second
	FallbackQueryTimeout  = 900 * time.Millisecond
	StreamConnectTimeout  = 8 * time.Second
	DemandRefreshInterval = 250 * time.Millisecond
)

var retryDelays = [...]time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	30 * time.Second,
}

// DemandSource reports instruments currently needed by one consumer class.
type DemandSource interface {
	ActiveInstruments() []string
}

// DemandSourceFunc adapts a function to DemandSource.
type DemandSourceFunc func() []string

func (f DemandSourceFunc) ActiveInstruments() []string {
	if f == nil {
		return nil
	}
	return f()
}

// QuoteSource supplies polling fallback ticks.
type QuoteSource interface {
	QueryTickers(context.Context, []string) (map[string]Tick, error)
}

// PushSource creates a broker push stream for one immutable demand generation.
type PushSource interface {
	NewStream([]string, PushTickHandler) (PushStream, error)
}

// PushStream is the lifecycle surface required by the collector.
type PushStream interface {
	Connect(context.Context) error
	Close() error
}

// PushTickHandler receives push ticks only. Polling fallback never calls it.
type PushTickHandler func(Tick)

type RuntimeState struct {
	Connected       bool
	Generation      uint64
	ActiveCount     int
	LastRefreshAt   time.Time
	QuoteRetryAt    time.Time
	QuoteFailures   int
	QuoteLastError  string
	StreamRetryAt   time.Time
	StreamFailures  int
	StreamLastError string
	Closed          bool
}

type CollectorOptions struct {
	PollInterval   time.Duration
	QueryTimeout   time.Duration
	ConnectTimeout time.Duration
	DemandInterval time.Duration
	Now            func() time.Time
}

type Collector struct {
	cache       *Cache
	quotes      QuoteSource
	push        PushSource
	pushHandler PushTickHandler

	mu            sync.Mutex
	demandSources []DemandSource
	state         RuntimeState
	key           string
	stream        PushStream
	streamCancel  context.CancelFunc
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	closeOnce     sync.Once
	wake          chan struct{}
	paused        bool

	pollInterval   time.Duration
	queryTimeout   time.Duration
	connectTimeout time.Duration
	demandInterval time.Duration
	now            func() time.Time
}

func NewCollector(cache *Cache, quotes QuoteSource, push PushSource, handler PushTickHandler, options CollectorOptions) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Collector{
		cache:          cache,
		quotes:         quotes,
		push:           push,
		pushHandler:    handler,
		ctx:            ctx,
		cancel:         cancel,
		wake:           make(chan struct{}, 1),
		pollInterval:   durationOr(options.PollInterval, FallbackPollInterval),
		queryTimeout:   durationOr(options.QueryTimeout, FallbackQueryTimeout),
		connectTimeout: durationOr(options.ConnectTimeout, StreamConnectTimeout),
		demandInterval: durationOr(options.DemandInterval, DemandRefreshInterval),
		now:            options.Now,
	}
	if c.now == nil {
		c.now = time.Now
	}
	c.wg.Add(1)
	go c.run()
	return c
}

func (c *Collector) SetDemandSources(sources ...DemandSource) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.state.Closed {
		c.demandSources = append([]DemandSource(nil), sources...)
	}
	c.mu.Unlock()
	c.Wake()
}

func (c *Collector) Wake() {
	if c == nil {
		return
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Collector) State() RuntimeState {
	if c == nil {
		return RuntimeState{Closed: true}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state
	state.ActiveCount = demandCount(c.key)
	return state
}

func (c *Collector) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.state.Closed {
		c.mu.Unlock()
		return
	}
	stream := c.detachStreamLocked()
	c.key = ""
	c.state.Generation++
	c.state.Connected = false
	c.state.LastRefreshAt = time.Time{}
	c.state.QuoteRetryAt = time.Time{}
	c.state.QuoteFailures = 0
	c.state.QuoteLastError = ""
	c.state.StreamRetryAt = time.Time{}
	c.state.StreamFailures = 0
	c.state.StreamLastError = ""
	c.paused = true
	c.mu.Unlock()
	closeStream(stream)
}

func (c *Collector) Resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.state.Closed {
		c.paused = false
	}
	c.mu.Unlock()
	c.Wake()
}

func (c *Collector) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.state.Closed = true
		c.state.Generation++
		stream := c.detachStreamLocked()
		c.mu.Unlock()
		c.cancel()
		if stream != nil {
			closeErr = stream.Close()
		}
		c.wg.Wait()
	})
	return closeErr
}

func (c *Collector) run() {
	defer c.wg.Done()
	demandTicker := time.NewTicker(c.demandInterval)
	pollTicker := time.NewTicker(c.pollInterval)
	defer demandTicker.Stop()
	defer pollTicker.Stop()

	c.reconcile()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.wake:
			c.reconcile()
		case <-demandTicker.C:
			c.reconcile()
		case <-pollTicker.C:
			c.poll()
		}
	}
}

func (c *Collector) reconcile() {
	instruments := c.activeInstruments()
	key := strings.Join(instruments, ",")

	c.mu.Lock()
	if c.state.Closed {
		c.mu.Unlock()
		return
	}
	if key == c.key {
		generation := c.state.Generation
		needsStream := key != "" && c.push != nil && c.stream == nil && !c.now().UTC().Before(c.state.StreamRetryAt)
		c.mu.Unlock()
		if needsStream {
			c.startStream(generation, instruments)
		}
		return
	}
	old := c.detachStreamLocked()
	c.key = key
	c.state.Generation++
	generation := c.state.Generation
	c.state.Connected = false
	c.state.StreamRetryAt = time.Time{}
	c.state.StreamFailures = 0
	c.state.StreamLastError = ""
	c.mu.Unlock()
	closeStream(old)

	if len(instruments) == 0 || c.push == nil {
		return
	}
	c.startStream(generation, instruments)
}

func (c *Collector) startStream(generation uint64, instruments []string) {
	c.mu.Lock()
	if c.state.Closed || c.state.Generation != generation || c.key == "" {
		c.mu.Unlock()
		return
	}
	now := c.now().UTC()
	if now.Before(c.state.StreamRetryAt) {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	stream, err := c.push.NewStream(instruments, func(tick Tick) {
		c.commitPush(generation, tick)
	})
	if err != nil {
		c.commitStreamFailure(generation, err)
		return
	}

	connectCtx, cancel := context.WithTimeout(c.ctx, c.connectTimeout)
	c.mu.Lock()
	if c.state.Closed || c.state.Generation != generation {
		c.mu.Unlock()
		cancel()
		closeStream(stream)
		return
	}
	c.stream = stream
	c.streamCancel = cancel
	c.wg.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.wg.Done()
		err := stream.Connect(connectCtx)
		cancel()
		if err != nil {
			closeStream(stream)
			c.commitStreamFailure(generation, err)
			return
		}
		c.mu.Lock()
		if !c.state.Closed && c.state.Generation == generation && c.stream == stream {
			c.state.Connected = true
			c.state.StreamFailures = 0
			c.state.StreamRetryAt = time.Time{}
			c.state.StreamLastError = ""
		}
		c.mu.Unlock()
	}()
}

func (c *Collector) poll() {
	instruments := c.activeInstruments()
	if len(instruments) == 0 || c.quotes == nil || c.cache.AllFresh(instruments, TickFreshness) {
		return
	}

	c.mu.Lock()
	if c.state.Closed {
		c.mu.Unlock()
		return
	}
	now := c.now().UTC()
	if now.Before(c.state.QuoteRetryAt) || (!c.state.LastRefreshAt.IsZero() && now.Sub(c.state.LastRefreshAt) < c.pollInterval) {
		c.mu.Unlock()
		return
	}
	c.state.LastRefreshAt = now
	generation := c.state.Generation
	c.wg.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithTimeout(c.ctx, c.queryTimeout)
		defer cancel()
		ticks, err := c.quotes.QueryTickers(ctx, instruments)
		if err != nil {
			c.commitQuoteFailure(generation, err)
			return
		}
		c.mu.Lock()
		valid := !c.state.Closed && c.state.Generation == generation
		if valid {
			c.state.QuoteFailures = 0
			c.state.QuoteRetryAt = time.Time{}
			c.state.QuoteLastError = ""
		}
		c.mu.Unlock()
		if !valid {
			return
		}
		for _, instrumentID := range instruments {
			if tick, ok := ticks[instrumentID]; ok {
				c.cache.Store(tick)
			}
		}
	}()
}

func (c *Collector) commitPush(generation uint64, tick Tick) {
	c.mu.Lock()
	valid := !c.state.Closed && c.state.Generation == generation
	c.mu.Unlock()
	if !valid {
		return
	}
	stored := c.cache.Store(tick)
	if stored != nil && c.pushHandler != nil {
		c.pushHandler(*stored)
	}
}

func (c *Collector) commitQuoteFailure(generation uint64, err error) {
	if err == nil || errors.Is(err, context.Canceled) && c.ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Closed || c.state.Generation != generation {
		return
	}
	delay := retryDelay(c.state.QuoteFailures)
	c.state.QuoteFailures++
	c.state.QuoteRetryAt = c.now().UTC().Add(delay)
	c.state.QuoteLastError = err.Error()
	observability.ErrorWithImportance(observability.WithFields(c.ctx, observability.Fields{Source: "market-data"}), observability.ImportanceHigh, "marketdata fallback query failed", err, "retry_in_ms", delay.Milliseconds())
}

func (c *Collector) commitStreamFailure(generation uint64, err error) {
	if err == nil || errors.Is(err, context.Canceled) && c.ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	if c.state.Closed || c.state.Generation != generation {
		c.mu.Unlock()
		return
	}
	if c.streamCancel != nil {
		c.streamCancel()
		c.streamCancel = nil
	}
	c.stream = nil
	c.state.Connected = false
	delay := retryDelay(c.state.StreamFailures)
	c.state.StreamFailures++
	c.state.StreamRetryAt = c.now().UTC().Add(delay)
	c.state.StreamLastError = err.Error()
	c.mu.Unlock()
	observability.ErrorWithImportance(observability.WithFields(c.ctx, observability.Fields{Source: "market-data"}), observability.ImportanceHigh, "marketdata push stream connect failed", err, "retry_in_ms", delay.Milliseconds())
}

func (c *Collector) activeInstruments() []string {
	c.mu.Lock()
	sources := append([]DemandSource(nil), c.demandSources...)
	closed := c.state.Closed
	paused := c.paused
	c.mu.Unlock()
	if closed || paused {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, source := range sources {
		if source == nil {
			continue
		}
		for _, raw := range source.ActiveInstruments() {
			instrumentID := strings.ToUpper(strings.TrimSpace(raw))
			if instrumentID == "" {
				continue
			}
			if _, exists := seen[instrumentID]; exists {
				continue
			}
			seen[instrumentID] = struct{}{}
			result = append(result, instrumentID)
		}
	}
	sort.Strings(result)
	return result
}

func (c *Collector) detachStreamLocked() PushStream {
	if c.streamCancel != nil {
		c.streamCancel()
		c.streamCancel = nil
	}
	stream := c.stream
	c.stream = nil
	c.state.Connected = false
	return stream
}

func retryDelay(failures int) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures >= len(retryDelays) {
		return retryDelays[len(retryDelays)-1]
	}
	return retryDelays[failures]
}

func closeStream(stream PushStream) {
	if stream != nil {
		jftradeErr1 := stream.Close()
		jftradeLogError(jftradeErr1)
	}
}

func demandCount(key string) int {
	if key == "" {
		return 0
	}
	return strings.Count(key, ",") + 1
}

func durationOr(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
