package marketdata

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type Cache struct {
	mu        sync.Mutex
	samples   map[string][]Tick
	now       func() time.Time
	retention time.Duration
	max       int
}

func NewCache() *Cache {
	return &Cache{
		samples:   map[string][]Tick{},
		now:       time.Now,
		retention: CacheRetention,
		max:       MaxCacheSamples,
	}
}

func (c *Cache) Store(incoming Tick) *Tick {
	if incoming.InstrumentID == "" || incoming.Price.IsZero() {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now().UTC()
	cutoff := now.Add(-c.retention)
	samples := append([]Tick(nil), c.samples[incoming.InstrumentID]...)
	writeIndex := 0
	for _, existing := range samples {
		if observedAt := parseTime(existing.ObservedAt); !observedAt.IsZero() && observedAt.Before(cutoff) {
			continue
		}
		samples[writeIndex] = existing
		writeIndex++
	}
	samples = samples[:writeIndex]

	if len(samples) > 0 {
		latest := samples[len(samples)-1]
		inheritTickContext(&incoming, &latest)
		if ticksEquivalent(latest, incoming) {
			if shouldPromoteSource(latest.Source, incoming.Source) {
				latest.Source = incoming.Source
				latest.ObservedAt = incoming.ObservedAt
				samples[len(samples)-1] = latest
			}
			c.samples[incoming.InstrumentID] = samples
			return cloneTick(&latest)
		}
	}

	samples = append(samples, incoming)
	if len(samples) > c.max {
		samples = samples[len(samples)-c.max:]
	}
	c.samples[incoming.InstrumentID] = samples
	return cloneTick(&incoming)
}

func (c *Cache) Seed(sample Tick) {
	c.mu.Lock()
	c.samples[sample.InstrumentID] = []Tick{sample}
	c.mu.Unlock()
}

func (c *Cache) Count(instrumentID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.samples[instrumentID])
}

func (c *Cache) Snapshot(instrumentID string) []Tick {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Tick(nil), c.samples[instrumentID]...)
}

func (c *Cache) Latest(instrumentID string, maxAge time.Duration) *Tick {
	if maxAge <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	samples := c.samples[instrumentID]
	if len(samples) == 0 {
		return nil
	}
	latest := samples[len(samples)-1]
	observedAt := parseTime(latest.ObservedAt)
	if observedAt.IsZero() || c.now().UTC().Sub(observedAt.UTC()) > maxAge {
		return nil
	}
	return cloneTick(&latest)
}

func (c *Cache) LatestMany(instrumentIDs []string, maxAge time.Duration) []*Tick {
	if maxAge <= 0 || len(instrumentIDs) == 0 {
		return nil
	}
	cutoff := c.now().UTC().Add(-maxAge)
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]*Tick, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		samples := c.samples[instrumentID]
		if len(samples) == 0 {
			continue
		}
		latest := samples[len(samples)-1]
		observedAt := parseTime(latest.ObservedAt)
		if observedAt.IsZero() || observedAt.Before(cutoff) {
			continue
		}
		result = append(result, cloneTick(&latest))
	}
	return result
}

func (c *Cache) AllFresh(instrumentIDs []string, maxAge time.Duration) bool {
	for _, instrumentID := range instrumentIDs {
		if c.Latest(instrumentID, maxAge) == nil {
			return false
		}
	}
	return true
}

func inheritTickContext(incoming, latest *Tick) {
	if incoming == nil || latest == nil {
		return
	}
	if incoming.OpenPrice == nil {
		incoming.OpenPrice = latest.OpenPrice
	}
	if incoming.HighPrice == nil {
		incoming.HighPrice = latest.HighPrice
	}
	if incoming.LowPrice == nil {
		incoming.LowPrice = latest.LowPrice
	}
	if incoming.PreviousClosePrice == nil {
		incoming.PreviousClosePrice = latest.PreviousClosePrice
	}
	if incoming.LastClosePrice == nil {
		incoming.LastClosePrice = latest.LastClosePrice
	}
	if incoming.PreMarket == nil {
		incoming.PreMarket = latest.PreMarket
	}
	if incoming.AfterMarket == nil {
		incoming.AfterMarket = latest.AfterMarket
	}
	if incoming.Overnight == nil {
		incoming.Overnight = latest.Overnight
	}
	if incoming.Turnover.IsZero() {
		incoming.Turnover = latest.Turnover
	}
	if incoming.Session == "" || incoming.Session == "unknown" {
		incoming.Session = latest.Session
		incoming.ExtendedHours = latest.ExtendedHours
	}
	if incoming.Kind == TickKindTrade {
		incoming.Bid = latest.Bid
		incoming.Ask = latest.Ask
		if incoming.Volume == 0 {
			incoming.Volume = latest.Volume
		}
	}
}

func ticksEquivalent(left, right Tick) bool {
	return left.InstrumentID == right.InstrumentID &&
		left.Price.Equal(right.Price) &&
		left.Bid.Equal(right.Bid) &&
		left.Ask.Equal(right.Ask) &&
		left.Volume == right.Volume &&
		left.QuoteAt == right.QuoteAt &&
		left.Session == right.Session &&
		left.ExtendedHours == right.ExtendedHours &&
		optionalDecimalEqual(left.OpenPrice, right.OpenPrice) &&
		optionalDecimalEqual(left.HighPrice, right.HighPrice) &&
		optionalDecimalEqual(left.LowPrice, right.LowPrice) &&
		optionalDecimalEqual(left.PreviousClosePrice, right.PreviousClosePrice)
}

func optionalDecimalEqual(left, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func shouldPromoteSource(cached, incoming string) bool {
	return incoming == "bbgo:futu:stream" && cached != "bbgo:futu:stream"
}

func cloneTick(tick *Tick) *Tick {
	if tick == nil {
		return nil
	}
	return new(*tick)
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
