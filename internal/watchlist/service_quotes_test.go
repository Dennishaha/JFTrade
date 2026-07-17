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

func (source quoteResultSource) BatchSnapshots(context.Context, []string) ([]Quote, []QuoteError, error) {
	return source.quotes, source.errors, source.err
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

func TestQuoteResultHelpersReturnEmptySlicesForNilValues(t *testing.T) {
	if values := nonNilQuotes(nil); values == nil || len(values) != 0 {
		t.Fatalf("nonNilQuotes(nil) = %#v", values)
	}
	if values := nonNilQuoteErrors(nil); values == nil || len(values) != 0 {
		t.Fatalf("nonNilQuoteErrors(nil) = %#v", values)
	}
}
