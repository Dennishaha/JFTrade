package watchlist

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Service) BatchQuotes(ctx context.Context, instrumentIDs []string) (BatchQuotes, error) {
	if s == nil {
		return BatchQuotes{}, ErrUnavailable
	}
	if len(instrumentIDs) == 0 || len(instrumentIDs) > MaxPageLimit {
		return BatchQuotes{}, fmt.Errorf("%w: instrumentIds must contain 1-%d items", ErrValidation, MaxPageLimit)
	}
	normalized := make([]string, 0, len(instrumentIDs))
	seen := make(map[string]struct{}, len(instrumentIDs))
	for _, value := range instrumentIDs {
		instrumentID, err := NormalizeInstrumentID(value)
		if err != nil {
			return BatchQuotes{}, err
		}
		if _, ok := seen[instrumentID]; ok {
			continue
		}
		seen[instrumentID] = struct{}{}
		normalized = append(normalized, instrumentID)
	}
	source, owned, waitFor, generation := s.reserveQuoteFlightsWithSource(normalized)
	if source == nil {
		return BatchQuotes{}, ErrUnavailable
	}
	if len(owned) > 0 {
		quotes, itemErrors, err := source.BatchSnapshots(ctx, owned)
		s.completeQuoteFlights(ctx, generation, owned, quotes, itemErrors, err, s.quoteCacheTTL(source))
	}
	for _, flight := range waitFor {
		select {
		case <-ctx.Done():
			return BatchQuotes{}, ctx.Err()
		case <-flight.done:
		}
	}
	quotes, itemErrors := s.collectQuoteCache(normalized)
	return BatchQuotes{Quotes: nonNilQuotes(quotes), Errors: nonNilQuoteErrors(itemErrors), ObservedAt: s.now()}, nil
}

func (s *Service) quoteCacheTTL(source BatchSnapshotSource) time.Duration {
	if policy, ok := source.(QuoteCachePolicySource); ok {
		if ttl := policy.QuoteCacheTTL(); ttl > 0 {
			return ttl
		}
	}
	return s.quoteTTL
}

func (s *Service) updateInstrumentMetadata(ctx context.Context, quotes []Quote) {
	writer, ok := s.repository.(InstrumentMetadataWriter)
	if !ok || len(quotes) == 0 {
		return
	}
	metadata := make([]InstrumentMetadata, 0, len(quotes))
	for _, quote := range quotes {
		if strings.TrimSpace(quote.Name) == "" && strings.TrimSpace(quote.Type) == "" {
			continue
		}
		metadata = append(metadata, InstrumentMetadata{
			InstrumentID: quote.InstrumentID,
			Name:         strings.TrimSpace(quote.Name),
			Type:         strings.TrimSpace(quote.Type),
		})
	}
	if len(metadata) > 0 {
		_ = writer.UpdateInstrumentMetadata(ctx, metadata)
	}
}

func (s *Service) reserveQuoteFlights(instrumentIDs []string) ([]string, []*quoteFlight, uint64) {
	_, owned, waitFor, generation := s.reserveQuoteFlightsWithSource(instrumentIDs)
	return owned, waitFor, generation
}

func (s *Service) reserveQuoteFlightsWithSource(instrumentIDs []string) (BatchSnapshotSource, []string, []*quoteFlight, uint64) {
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	s.mu.RLock()
	source := s.quoteSource
	s.mu.RUnlock()
	now := s.now()
	owned := make([]string, 0, len(instrumentIDs))
	waitFor := make([]*quoteFlight, 0)
	seenFlights := make(map[*quoteFlight]struct{})
	for _, instrumentID := range instrumentIDs {
		if entry, ok := s.quoteCache[instrumentID]; ok && now.Before(entry.expiresAt) {
			continue
		}
		delete(s.quoteCache, instrumentID)
		if flight := s.quoteFlight[instrumentID]; flight != nil {
			if _, seen := seenFlights[flight]; !seen {
				waitFor = append(waitFor, flight)
				seenFlights[flight] = struct{}{}
			}
			continue
		}
		flight := &quoteFlight{done: make(chan struct{})}
		s.quoteFlight[instrumentID] = flight
		owned = append(owned, instrumentID)
	}
	return source, owned, waitFor, s.quoteGeneration
}

func (s *Service) completeQuoteFlights(
	ctx context.Context,
	generation uint64,
	instrumentIDs []string,
	quotes []Quote,
	itemErrors []QuoteError,
	batchErr error,
	cacheTTL time.Duration,
) {
	quoteByID := make(map[string]Quote, len(quotes))
	for _, quote := range quotes {
		quoteByID[quote.InstrumentID] = quote
	}
	errorByID := make(map[string]QuoteError, len(itemErrors))
	for _, itemError := range itemErrors {
		errorByID[itemError.InstrumentID] = itemError
	}
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	if generation != s.quoteGeneration {
		return
	}
	s.updateInstrumentMetadata(ctx, quotes)
	if cacheTTL <= 0 {
		cacheTTL = s.quoteTTL
	}
	expiresAt := s.now().Add(cacheTTL)
	for _, instrumentID := range instrumentIDs {
		entry := quoteCacheEntry{expiresAt: expiresAt}
		if quote, ok := quoteByID[instrumentID]; ok {
			copy := quote
			entry.quote = &copy
		} else if itemError, ok := errorByID[instrumentID]; ok {
			copy := itemError
			entry.itemError = &copy
		} else {
			message := "snapshot source returned no result"
			code := "NO_SNAPSHOT"
			if batchErr != nil {
				message, code = batchErr.Error(), "SNAPSHOT_FAILED"
			}
			entry.itemError = &QuoteError{InstrumentID: instrumentID, Code: code, Message: message}
		}
		s.quoteCache[instrumentID] = entry
		if flight := s.quoteFlight[instrumentID]; flight != nil {
			delete(s.quoteFlight, instrumentID)
			close(flight.done)
		}
	}
}

// ResetQuoteCache prevents snapshots from the previous active provider from
// being served or repopulating the cache after a provider switch.
func (s *Service) ResetQuoteCache() {
	if s == nil {
		return
	}
	s.quoteMu.Lock()
	s.resetQuoteCacheLocked()
	s.quoteMu.Unlock()
}

// ChangeQuoteProvider serializes an active-provider mutation with quote flight
// reservation and completion. A successful change invalidates existing
// provider results before new requests can select a route. A failed change
// preserves the current provider's cache and in-flight work.
func (s *Service) ChangeQuoteProvider(change func() error) error {
	if s == nil {
		if change == nil {
			return nil
		}
		return change()
	}
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	if change != nil {
		if err := change(); err != nil {
			return err
		}
	}
	s.resetQuoteCacheLocked()
	return nil
}

func (s *Service) resetQuoteCacheLocked() {
	s.quoteGeneration++
	s.quoteCache = make(map[string]quoteCacheEntry)
	for _, flight := range s.quoteFlight {
		close(flight.done)
	}
	s.quoteFlight = make(map[string]*quoteFlight)
}

func (s *Service) collectQuoteCache(instrumentIDs []string) ([]Quote, []QuoteError) {
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	now := s.now()
	quotes := make([]Quote, 0, len(instrumentIDs))
	itemErrors := make([]QuoteError, 0)
	for _, instrumentID := range instrumentIDs {
		entry, ok := s.quoteCache[instrumentID]
		if !ok || !now.Before(entry.expiresAt) {
			itemErrors = append(itemErrors, QuoteError{InstrumentID: instrumentID, Code: "NO_SNAPSHOT", Message: "snapshot result is unavailable"})
			continue
		}
		if entry.quote != nil {
			quotes = append(quotes, *entry.quote)
		}
		if entry.itemError != nil {
			itemErrors = append(itemErrors, *entry.itemError)
		}
	}
	return quotes, itemErrors
}
