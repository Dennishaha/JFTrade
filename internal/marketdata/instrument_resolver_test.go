package marketdata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

type exactLookupFunc func(context.Context, string, string) ([]InstrumentCandidate, error)

func (lookup exactLookupFunc) LookupInstrument(ctx context.Context, market, code string) ([]InstrumentCandidate, error) {
	return lookup(ctx, market, code)
}

func resolverCandidate(market, code, name string) InstrumentCandidate {
	return InstrumentCandidate{
		Market:       market,
		InstrumentID: market + "." + code,
		Code:         code,
		Name:         name,
		SecurityType: "Eqty",
		LotSize:      100,
		Source:       "test",
	}
}

func TestMarketSubsetInstrumentResolverClassifiesExactLookups(t *testing.T) {
	lookupErr := errors.New("leaf unavailable")
	tests := []struct {
		name         string
		responses    map[string][]InstrumentCandidate
		errors       map[string]error
		wantStatus   InstrumentResolutionStatus
		wantIDs      []string
		wantFailures []string
	}{
		{
			name:       "only Shanghai matches",
			responses:  map[string][]InstrumentCandidate{"SH": {resolverCandidate("SH", "600519", "Kweichow Moutai")}},
			wantStatus: InstrumentResolutionResolved,
			wantIDs:    []string{"SH.600519"},
		},
		{
			name:       "only Shenzhen matches",
			responses:  map[string][]InstrumentCandidate{"SZ": {resolverCandidate("SZ", "600519", "Shenzhen Listing")}},
			wantStatus: InstrumentResolutionResolved,
			wantIDs:    []string{"SZ.600519"},
		},
		{
			name: "both exchanges match",
			responses: map[string][]InstrumentCandidate{
				"SH": {resolverCandidate("SH", "600519", "Shanghai Listing")},
				"SZ": {resolverCandidate("SZ", "600519", "Shenzhen Listing")},
			},
			wantStatus: InstrumentResolutionAmbiguous,
			wantIDs:    []string{"SH.600519", "SZ.600519"},
		},
		{
			name:       "neither exchange matches",
			responses:  map[string][]InstrumentCandidate{},
			wantStatus: InstrumentResolutionNotFound,
		},
		{
			name:         "one match and one failure is incomplete",
			responses:    map[string][]InstrumentCandidate{"SH": {resolverCandidate("SH", "600519", "Shanghai Listing")}},
			errors:       map[string]error{"SZ": lookupErr},
			wantStatus:   InstrumentResolutionIncomplete,
			wantIDs:      []string{"SH.600519"},
			wantFailures: []string{"SZ"},
		},
		{
			name:         "all failures are incomplete in stable order",
			errors:       map[string]error{"SH": lookupErr, "SZ": errors.New("second leaf unavailable")},
			wantStatus:   InstrumentResolutionIncomplete,
			wantFailures: []string{"SH", "SZ"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := NewMarketSubsetInstrumentResolver(exactLookupFunc(func(_ context.Context, market, code string) ([]InstrumentCandidate, error) {
				if code != "600519" {
					return nil, fmt.Errorf("lookup code = %q, want 600519", code)
				}
				return test.responses[market], test.errors[market]
			}))
			result, err := resolver.Resolve(t.Context(), " cn ", "600519")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if result.RequestedMarket != "CN" || result.Query != "600519" || result.ResolutionStatus != test.wantStatus {
				t.Fatalf("resolution metadata = %+v, want status %s", result, test.wantStatus)
			}
			ids := make([]string, 0, len(result.Entries))
			for _, entry := range result.Entries {
				ids = append(ids, entry.InstrumentID)
				if entry.Symbol != entry.Code || entry.ResolvedMarket != "CN" {
					t.Fatalf("candidate was not normalized: %+v", entry)
				}
			}
			if !slices.Equal(ids, test.wantIDs) {
				t.Fatalf("entry IDs = %#v, want %#v", ids, test.wantIDs)
			}
			failureMarkets := make([]string, 0, len(result.Failures))
			for _, failure := range result.Failures {
				failureMarkets = append(failureMarkets, failure.Market)
				if failure.Code != "600519" || failure.Message == "" {
					t.Fatalf("failure = %+v", failure)
				}
			}
			if !slices.Equal(failureMarkets, test.wantFailures) {
				t.Fatalf("failure markets = %#v, want %#v", failureMarkets, test.wantFailures)
			}
			if result.TotalReturned != len(test.wantIDs) || result.Entries == nil || result.Failures == nil {
				t.Fatalf("response collection contract = %+v", result)
			}
		})
	}
}

func TestMarketSubsetInstrumentResolverRunsLeavesConcurrentlyAndKeepsConfiguredOrder(t *testing.T) {
	started := make(chan string, 2)
	releaseSH := make(chan struct{})
	resolver := NewMarketSubsetInstrumentResolver(exactLookupFunc(func(ctx context.Context, market, code string) ([]InstrumentCandidate, error) {
		started <- market
		if market == "SH" {
			select {
			case <-releaseSH:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []InstrumentCandidate{resolverCandidate(market, code, market+" Listing")}, nil
	}))

	resultCh := make(chan InstrumentResolution, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := resolver.Resolve(t.Context(), "CN", "000001")
		resultCh <- result
		errCh <- err
	}()

	seen := map[string]bool{}
	for range 2 {
		select {
		case market := <-started:
			seen[market] = true
		case <-time.After(time.Second):
			t.Fatal("leaf lookups did not start concurrently")
		}
	}
	if !seen["SH"] || !seen["SZ"] {
		t.Fatalf("started lookups = %#v", seen)
	}
	close(releaseSH)
	if err := <-errCh; err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	result := <-resultCh
	if got := []string{result.Entries[0].Market, result.Entries[1].Market}; !slices.Equal(got, []string{"SH", "SZ"}) {
		t.Fatalf("candidate order = %#v, want [SH SZ]", got)
	}
}

func TestMarketSubsetInstrumentResolverQualifiedLeafAndDedupBoundaries(t *testing.T) {
	var mu sync.Mutex
	calls := make([]string, 0, 2)
	resolver := NewMarketSubsetInstrumentResolver(exactLookupFunc(func(_ context.Context, market, code string) ([]InstrumentCandidate, error) {
		mu.Lock()
		calls = append(calls, market+"."+code)
		mu.Unlock()
		candidate := resolverCandidate(market, code, "  Listing Name  ")
		return []InstrumentCandidate{candidate, candidate, {InstrumentID: market + ".WRONG", Code: "WRONG"}}, nil
	}))

	result, err := resolver.Resolve(t.Context(), "CN", "sz:000001")
	if err != nil {
		t.Fatalf("Resolve qualified: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionResolved || len(result.Entries) != 1 || result.Entries[0].InstrumentID != "SZ.000001" || result.Entries[0].Name != "Listing Name" {
		t.Fatalf("qualified resolution = %+v", result)
	}
	if !slices.Equal(calls, []string{"SZ.000001"}) {
		t.Fatalf("qualified lookup calls = %#v", calls)
	}
	result, err = resolver.Resolve(t.Context(), "US", "BRK.B")
	if err != nil || result.ResolutionStatus != InstrumentResolutionResolved || result.Entries[0].InstrumentID != "US.BRK.B" {
		t.Fatalf("dotted leaf code resolution = %+v, err=%v", result, err)
	}
	if !slices.Equal(calls, []string{"SZ.000001", "US.BRK.B"}) {
		t.Fatalf("dotted leaf lookup calls = %#v", calls)
	}
	result, err = resolver.Resolve(t.Context(), "SH", "600519")
	if err != nil || result.ResolutionStatus != InstrumentResolutionResolved || result.Entries[0].InstrumentID != "SH.600519" {
		t.Fatalf("direct leaf resolution = %+v, err=%v", result, err)
	}
	if !slices.Equal(calls, []string{"SZ.000001", "US.BRK.B", "SH.600519"}) {
		t.Fatalf("direct leaf lookup calls = %#v", calls)
	}

	result, err = resolver.Resolve(t.Context(), "", "SH.600519")
	if err != nil || result.ResolutionStatus != InstrumentResolutionResolved || len(result.Entries) != 1 || result.Entries[0].InstrumentID != "SH.600519" {
		t.Fatalf("market-less qualified resolution = %+v, err=%v, calls=%#v", result, err, calls)
	}
	if !slices.Equal(calls, []string{"SZ.000001", "US.BRK.B", "SH.600519", "SH.600519"}) {
		t.Fatalf("market-less qualified lookup calls = %#v", calls)
	}

	if _, err := resolver.Resolve(t.Context(), "", "unqualified"); err == nil {
		t.Fatal("market-less bare query should require a market")
	}
	if _, err := resolver.Resolve(t.Context(), "CN", "US.AAPL"); err == nil {
		t.Fatal("CN request should reject a US-qualified instrument")
	}
}

func TestMarketSubsetInstrumentResolverPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 2)
	resolver := NewMarketSubsetInstrumentResolver(exactLookupFunc(func(ctx context.Context, _, _ string) ([]InstrumentCandidate, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	errCh := make(chan error, 1)
	go func() {
		_, err := resolver.Resolve(ctx, "CN", "000001")
		errCh <- err
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("lookup did not observe resolver context")
		}
	}
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Resolve did not stop after context cancellation")
	}
}

func TestMarketSubsetInstrumentResolverRejectsExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	resolver := NewMarketSubsetInstrumentResolver(exactLookupFunc(func(context.Context, string, string) ([]InstrumentCandidate, error) {
		t.Fatal("expired context should stop before provider lookup")
		return nil, nil
	}))
	_, err := resolver.Resolve(ctx, "CN", "000001")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Resolve error = %v, want context.DeadlineExceeded", err)
	}
}

func TestClassifyInstrumentResolutionKeepsMultiplePartialCandidatesAmbiguous(t *testing.T) {
	if got := classifyInstrumentResolution(2, 1); got != InstrumentResolutionAmbiguous {
		t.Fatalf("classifyInstrumentResolution(2, 1) = %q, want %q", got, InstrumentResolutionAmbiguous)
	}
}
