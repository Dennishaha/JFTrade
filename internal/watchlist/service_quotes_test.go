package watchlist

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type blockingSnapshotSource struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	err     error
}

type quoteResultSource struct {
	quotes []Quote
	errors []QuoteError
	err    error
}

type blockingMetadataSnapshotSource struct {
	started chan struct{}
	release chan struct{}
	quote   Quote
}

func (source quoteResultSource) BatchSnapshots(context.Context, []string) ([]Quote, []QuoteError, error) {
	return source.quotes, source.errors, source.err
}

func (source *blockingMetadataSnapshotSource) BatchSnapshots(
	_ context.Context,
	_ []string,
) ([]Quote, []QuoteError, error) {
	close(source.started)
	<-source.release
	return []Quote{source.quote}, nil, nil
}

func (source *blockingSnapshotSource) BatchSnapshots(_ context.Context, instrumentIDs []string) ([]Quote, []QuoteError, error) {
	source.calls.Add(1)
	select {
	case source.started <- struct{}{}:
	default:
	}
	if source.release != nil {
		<-source.release
	}
	if source.err != nil {
		return nil, nil, source.err
	}
	quotes := make([]Quote, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		quotes = append(quotes, Quote{InstrumentID: instrumentID, Source: "test", ObservedAt: time.Now()})
	}
	return quotes, nil, nil
}

func TestBatchQuotesCachesAndSingleflightsOverlappingRequests(t *testing.T) {
	source := &blockingSnapshotSource{started: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewService(nil, WithBatchSnapshotSource(source), WithQuoteCacheTTL(time.Minute))
	firstDone := make(chan error, 1)
	go func() {
		result, err := service.BatchQuotes(context.Background(), []string{"us:aapl", "US.MSFT"})
		if err == nil && len(result.Quotes) != 2 {
			err = errors.New("first result did not include two quotes")
		}
		firstDone <- err
	}()
	<-source.started
	secondDone := make(chan error, 1)
	go func() {
		result, err := service.BatchQuotes(context.Background(), []string{"US.AAPL"})
		if err == nil && len(result.Quotes) != 1 {
			err = errors.New("second result did not include one quote")
		}
		secondDone <- err
	}()
	close(source.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if calls := source.calls.Load(); calls != 1 {
		t.Fatalf("source calls = %d, want 1", calls)
	}
	if _, err := service.BatchQuotes(t.Context(), []string{"US.AAPL", "US.MSFT"}); err != nil {
		t.Fatal(err)
	}
	if calls := source.calls.Load(); calls != 1 {
		t.Fatalf("cached source calls = %d, want 1", calls)
	}
}

func TestBatchQuotesTurnsBatchFailureIntoPerItemErrors(t *testing.T) {
	source := &blockingSnapshotSource{started: make(chan struct{}, 1), err: errors.New("provider down")}
	service := NewService(nil, WithBatchSnapshotSource(source))
	result, err := service.BatchQuotes(t.Context(), []string{"US.AAPL", "HK.00700"})
	if err != nil {
		t.Fatalf("BatchQuotes = %v", err)
	}
	if len(result.Quotes) != 0 || len(result.Errors) != 2 {
		t.Fatalf("result = %#v", result)
	}
	for _, itemError := range result.Errors {
		if itemError.Code != "SNAPSHOT_FAILED" || itemError.Message != "provider down" {
			t.Fatalf("item error = %#v", itemError)
		}
	}
}

func TestBatchQuotesHonorsProviderCachePolicy(t *testing.T) {
	now := time.Date(2026, time.July, 29, 9, 0, 0, 0, time.UTC)
	source := &policySnapshotSource{ttl: 15 * time.Second}
	service := NewService(
		nil,
		WithClock(func() time.Time { return now }),
		WithBatchSnapshotSource(source),
		WithQuoteCacheTTL(time.Second),
	)
	if _, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	if _, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"}); err != nil {
		t.Fatal(err)
	}
	if calls := source.calls.Load(); calls != 1 {
		t.Fatalf("source calls before provider TTL = %d, want 1", calls)
	}
	now = now.Add(6 * time.Second)
	if _, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"}); err != nil {
		t.Fatal(err)
	}
	if calls := source.calls.Load(); calls != 2 {
		t.Fatalf("source calls after provider TTL = %d, want 2", calls)
	}
}

func TestBatchQuotesPreservesPartialResultsAndUpdatesKnownMetadata(t *testing.T) {
	var received []InstrumentMetadata
	repository := &serviceTestRepository{updateMetadata: func(_ context.Context, metadata []InstrumentMetadata) error {
		received = append([]InstrumentMetadata(nil), metadata...)
		return nil
	}}
	service := NewService(repository, WithBatchSnapshotSource(quoteResultSource{
		quotes: []Quote{{InstrumentID: "US.AAPL", Name: " Apple Inc. ", Type: "equity"}},
		errors: []QuoteError{{InstrumentID: "HK.00700", Code: "DELAYED", Message: "delayed quote"}},
	}))

	result, err := service.BatchQuotes(t.Context(), []string{"US.AAPL", "HK.00700", "US.MSFT", "US.AAPL"})
	if err != nil {
		t.Fatalf("BatchQuotes: %v", err)
	}
	if len(result.Quotes) != 1 || result.Quotes[0].InstrumentID != "US.AAPL" {
		t.Fatalf("quotes = %#v", result.Quotes)
	}
	if len(result.Errors) != 2 || result.Errors[0].InstrumentID != "HK.00700" || result.Errors[1].Code != "NO_SNAPSHOT" || result.Errors[1].InstrumentID != "US.MSFT" {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if len(received) != 1 || received[0] != (InstrumentMetadata{InstrumentID: "US.AAPL", Name: "Apple Inc.", Type: "equity"}) {
		t.Fatalf("metadata = %#v", received)
	}
	if result.ObservedAt.IsZero() {
		t.Fatal("missing observation timestamp")
	}
}

func TestChangeQuoteProviderRejectsPreviousProviderInflightResults(t *testing.T) {
	oldSource := &blockingSnapshotSource{started: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewService(nil, WithBatchSnapshotSource(oldSource), WithQuoteCacheTTL(time.Minute))
	oldDone := make(chan error, 1)
	go func() {
		_, err := service.BatchQuotes(context.Background(), []string{"US.AAPL"})
		oldDone <- err
	}()
	<-oldSource.started

	err := service.ChangeQuoteProvider(func() error {
		service.RegisterBatchSnapshotSource(quoteResultSource{quotes: []Quote{{
			InstrumentID: "US.AAPL",
			Source:       "new-provider",
			ObservedAt:   time.Now(),
		}}})
		return nil
	})
	if err != nil {
		t.Fatalf("change quote provider: %v", err)
	}
	current, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"})
	if err != nil || len(current.Quotes) != 1 || current.Quotes[0].Source != "new-provider" {
		t.Fatalf("current provider quotes = %#v, err=%v", current, err)
	}

	close(oldSource.release)
	if err := <-oldDone; err != nil {
		t.Fatalf("old provider request: %v", err)
	}
	cached, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"})
	if err != nil || len(cached.Quotes) != 1 || cached.Quotes[0].Source != "new-provider" {
		t.Fatalf("stale provider repopulated cache: %#v, err=%v", cached, err)
	}
}

func TestChangeQuoteProviderFailurePreservesCurrentCache(t *testing.T) {
	source := &policySnapshotSource{ttl: time.Minute}
	service := NewService(nil, WithBatchSnapshotSource(source))
	first, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"})
	if err != nil || len(first.Quotes) != 1 {
		t.Fatalf("initial quotes = %#v, err=%v", first, err)
	}
	changeErr := errors.New("provider health check failed")
	if err := service.ChangeQuoteProvider(func() error { return changeErr }); !errors.Is(err, changeErr) {
		t.Fatalf("change provider error = %v, want %v", err, changeErr)
	}
	cached, err := service.BatchQuotes(t.Context(), []string{"US.AAPL"})
	if err != nil || len(cached.Quotes) != 1 {
		t.Fatalf("preserved quotes = %#v, err=%v", cached, err)
	}
	if calls := source.calls.Load(); calls != 1 {
		t.Fatalf("failed provider change invalidated cache; source calls = %d, want 1", calls)
	}
}

func TestResetQuoteCacheRejectsPreviousProviderInflightMetadata(t *testing.T) {
	metadataWrites := make(chan []InstrumentMetadata, 1)
	repository := &serviceTestRepository{updateMetadata: func(
		_ context.Context,
		metadata []InstrumentMetadata,
	) error {
		metadataWrites <- append([]InstrumentMetadata(nil), metadata...)
		return nil
	}}
	oldSource := &blockingMetadataSnapshotSource{
		started: make(chan struct{}),
		release: make(chan struct{}),
		quote: Quote{
			InstrumentID: "US.AAPL",
			Name:         "Old Provider Name",
			Type:         "old-provider-type",
			Source:       "old-provider",
			ObservedAt:   time.Now(),
		},
	}
	service := NewService(repository, WithBatchSnapshotSource(oldSource))
	oldDone := make(chan error, 1)
	go func() {
		_, err := service.BatchQuotes(context.Background(), []string{"US.AAPL"})
		oldDone <- err
	}()
	<-oldSource.started

	service.ResetQuoteCache()
	close(oldSource.release)
	if err := <-oldDone; err != nil {
		t.Fatalf("old provider request: %v", err)
	}
	select {
	case metadata := <-metadataWrites:
		t.Fatalf("stale provider updated instrument metadata: %#v", metadata)
	default:
	}
}

func TestQuoteResultHelpersReturnEmptySlicesForNilValues(t *testing.T) {
	if values := nonNilQuotes(nil); values == nil || len(values) != 0 {
		t.Fatalf("nonNilQuotes(nil) = %#v", values)
	}
	if values := nonNilQuoteErrors(nil); values == nil || len(values) != 0 {
		t.Fatalf("nonNilQuoteErrors(nil) = %#v", values)
	}
}

type policySnapshotSource struct {
	calls atomic.Int32
	ttl   time.Duration
}

func (s *policySnapshotSource) BatchSnapshots(
	_ context.Context,
	instrumentIDs []string,
) ([]Quote, []QuoteError, error) {
	s.calls.Add(1)
	quotes := make([]Quote, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		quotes = append(quotes, Quote{
			InstrumentID: instrumentID,
			Source:       "policy",
			ObservedAt:   time.Now(),
		})
	}
	return quotes, nil, nil
}

func (s *policySnapshotSource) QuoteCacheTTL() time.Duration {
	return s.ttl
}
