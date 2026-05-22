package jftradeapi

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type tickSampleCacheManager struct {
	mu      sync.Mutex
	samples map[string][]marketTickSample
}

func newTickSampleCacheManager() tickSampleCacheManager {
	return tickSampleCacheManager{samples: map[string][]marketTickSample{}}
}

func (cache *tickSampleCacheManager) snapshot(instrumentID string) []marketTickSample {
	cache.mu.Lock()
	samples := append([]marketTickSample(nil), cache.samples[instrumentID]...)
	cache.mu.Unlock()
	return samples
}

func (cache *tickSampleCacheManager) seed(sample marketTickSample) {
	cache.mu.Lock()
	cache.samples[sample.InstrumentID] = []marketTickSample{sample}
	cache.mu.Unlock()
}

func (cache *tickSampleCacheManager) count(instrumentID string) int {
	cache.mu.Lock()
	count := len(cache.samples[instrumentID])
	cache.mu.Unlock()
	return count
}

func (cache *tickSampleCacheManager) store(sample marketTickSample) *marketTickSample {
	if sample.InstrumentID == "" || sample.Price.IsZero() {
		return nil
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	retentionCutoff := time.Now().UTC().Add(-tickCacheRetention)
	samples := append([]marketTickSample(nil), cache.samples[sample.InstrumentID]...)
	writeIndex := 0
	for _, existing := range samples {
		observedAt := parseQueryTime(existing.ObservedAt, retentionCutoff)
		if observedAt.Before(retentionCutoff) {
			continue
		}
		samples[writeIndex] = existing
		writeIndex++
	}
	samples = samples[:writeIndex]
	if len(samples) > 0 && marketTickSamplesEquivalent(samples[len(samples)-1], sample) {
		latest := samples[len(samples)-1]
		if shouldPromoteTickSampleSource(latest.Source, sample.Source) {
			latest.Source = sample.Source
			latest.ObservedAt = sample.ObservedAt
			samples[len(samples)-1] = latest
		}
		cache.samples[sample.InstrumentID] = samples
		copyOfLatest := latest
		return &copyOfLatest
	}
	samples = append(samples, sample)
	if len(samples) > maxTickCacheSamples {
		samples = samples[len(samples)-maxTickCacheSamples:]
	}
	cache.samples[sample.InstrumentID] = samples
	return &sample
}

func (cache *tickSampleCacheManager) latest(instrumentID string, maxAge time.Duration) *marketTickSample {
	if maxAge <= 0 {
		return nil
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	samples := cache.samples[instrumentID]
	if len(samples) == 0 {
		return nil
	}
	latest := samples[len(samples)-1]
	observedAt := parseQueryTime(latest.ObservedAt, time.Time{})
	if observedAt.IsZero() || time.Since(observedAt.UTC()) > maxAge {
		return nil
	}
	copyOfLatest := latest
	return &copyOfLatest
}

func (cache *tickSampleCacheManager) latestMany(instrumentIDs []string, maxAge time.Duration) []*marketTickSample {
	if maxAge <= 0 || len(instrumentIDs) == 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	cache.mu.Lock()
	defer cache.mu.Unlock()

	results := make([]*marketTickSample, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		samples := cache.samples[instrumentID]
		if len(samples) == 0 {
			continue
		}
		latest := samples[len(samples)-1]
		observedAt := parseQueryTime(latest.ObservedAt, time.Time{})
		if observedAt.IsZero() || observedAt.Before(cutoff) {
			continue
		}
		copyOfLatest := latest
		results = append(results, &copyOfLatest)
	}
	return results
}

func (s *Server) storeTickerSample(sample marketTickSample) *marketTickSample {
	return s.tickCache.store(sample)
}

func (s *Server) latestTickerSample(instrumentID string, maxAge time.Duration) *marketTickSample {
	return s.tickCache.latest(instrumentID, maxAge)
}

func (s *Server) latestTickerSamples(instrumentIDs []string, maxAge time.Duration) []*marketTickSample {
	return s.tickCache.latestMany(instrumentIDs, maxAge)
}

func (s *Server) allHaveFreshTickerSamples(instrumentIDs []string, maxAge time.Duration) bool {
	if len(instrumentIDs) == 0 {
		return true
	}
	for _, instrumentID := range instrumentIDs {
		if s.latestTickerSample(instrumentID, maxAge) == nil {
			return false
		}
	}
	return true
}

func marketTickSamplesEquivalent(left marketTickSample, right marketTickSample) bool {
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

func optionalDecimalEqual(left *decimal.Decimal, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func shouldPromoteTickSampleSource(cachedSource string, incomingSource string) bool {
	return incomingSource == "bbgo:futu:stream" && cachedSource != "bbgo:futu:stream"
}
