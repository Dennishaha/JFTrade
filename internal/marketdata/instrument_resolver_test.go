package marketdata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type resolverProviderStub struct {
	lookup func(context.Context, string, string) ([]InstrumentCandidate, error)
	search func(context.Context, string, int) ([]InstrumentCandidate, error)
}

func (p *resolverProviderStub) LookupInstrument(ctx context.Context, market, code string) ([]InstrumentCandidate, error) {
	if p.lookup == nil {
		return nil, nil
	}
	return p.lookup(ctx, market, code)
}

func (p *resolverProviderStub) SearchInstruments(ctx context.Context, query string, limit int) ([]InstrumentCandidate, error) {
	if p.search == nil {
		return nil, nil
	}
	return p.search(ctx, query, limit)
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

func TestMarketSubsetInstrumentResolverKeepsQualifiedExactLookup(t *testing.T) {
	var calls []string
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		lookup: func(_ context.Context, marketCode, code string) ([]InstrumentCandidate, error) {
			calls = append(calls, marketCode+"."+code)
			candidate := resolverCandidate(marketCode, code, "  Listing Name  ")
			return []InstrumentCandidate{candidate, candidate, {InstrumentID: marketCode + ".WRONG", Code: "WRONG"}}, nil
		},
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			t.Fatal("qualified input must not use cross-market search")
			return nil, nil
		},
	})

	result, err := resolver.Resolve(t.Context(), "CN", "sz:000001", 20)
	if err != nil {
		t.Fatalf("Resolve qualified: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionResolved || len(result.Entries) != 1 {
		t.Fatalf("qualified resolution = %+v", result)
	}
	entry := result.Entries[0]
	if entry.InstrumentID != "SZ.000001" || entry.ResolvedMarket != "CN" || entry.Name != "Listing Name" || !entry.Selectable {
		t.Fatalf("normalized exact candidate = %+v", entry)
	}
	if !slices.Equal(calls, []string{"SZ.000001"}) {
		t.Fatalf("qualified lookup calls = %#v", calls)
	}

	result, err = resolver.Resolve(t.Context(), "", "US.BRK.B", 20)
	if err != nil || result.ResolutionStatus != InstrumentResolutionResolved || result.Entries[0].InstrumentID != "US.BRK.B" {
		t.Fatalf("dotted code resolution = %+v, err=%v", result, err)
	}
	if !slices.Equal(calls, []string{"SZ.000001", "US.BRK.B"}) {
		t.Fatalf("qualified lookup calls = %#v", calls)
	}
}

func TestMarketSubsetInstrumentResolverMarksUnsupportedQualifiedMarketUnavailable(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		lookup: func(_ context.Context, marketCode, code string) ([]InstrumentCandidate, error) {
			return []InstrumentCandidate{resolverCandidate(marketCode, code, "Toyota")}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "", "JP.7203", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionUnavailable || len(result.Entries) != 1 {
		t.Fatalf("resolution = %+v", result)
	}
	if result.Entries[0].Selectable || result.Entries[0].UnavailableReason == "" {
		t.Fatalf("unsupported candidate = %+v", result.Entries[0])
	}
}

func TestMarketSubsetInstrumentResolverSearchesNamesAndPreservesRelevance(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		lookup: func(context.Context, string, string) ([]InstrumentCandidate, error) {
			t.Fatal("unqualified name must not use static lookup")
			return nil, nil
		},
		search: func(_ context.Context, query string, limit int) ([]InstrumentCandidate, error) {
			if query != "Apple" || limit != 100 {
				t.Fatalf("SearchInstruments(%q, %d), want Apple, 100", query, limit)
			}
			return []InstrumentCandidate{
				resolverCandidate("US", "AAPL", "Apple Inc."),
				resolverCandidate("HK", "04662", "南方东英苹果日报杠杆产品"),
				resolverCandidate("JP", "2788", "Apple International"),
			}, nil
		},
	})

	result, err := resolver.Resolve(t.Context(), "", "Apple", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantIDs := []string{"US.AAPL", "HK.04662", "JP.2788"}
	gotIDs := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		gotIDs = append(gotIDs, entry.InstrumentID)
	}
	if result.ResolutionStatus != InstrumentResolutionAmbiguous || !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("search resolution = %+v, want IDs %#v", result, wantIDs)
	}
	if result.Entries[2].Selectable || result.Entries[2].UnavailableReason == "" {
		t.Fatalf("JP candidate should be disabled: %+v", result.Entries[2])
	}
}

func TestMarketSubsetInstrumentResolverExactCodeWinsAndCrossMarketCodeStaysAmbiguous(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			return []InstrumentCandidate{
				resolverCandidate("US", "AAPL", "Apple Inc."),
				resolverCandidate("CA", "AAPL", "Apple CDR"),
				resolverCandidate("HK", "04662", "AAPL Daily Product"),
			}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "", "aapl", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionAmbiguous || len(result.Entries) != 2 {
		t.Fatalf("exact-code resolution = %+v", result)
	}
	if result.Entries[0].InstrumentID != "US.AAPL" || result.Entries[1].InstrumentID != "CA.AAPL" {
		t.Fatalf("exact code entries = %+v", result.Entries)
	}
}

func TestMarketSubsetInstrumentResolverFiltersCNAndDeduplicates(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			sh := resolverCandidate("SH", "000001", "SSE Composite")
			return []InstrumentCandidate{
				resolverCandidate("US", "000001", "US Product"),
				sh,
				sh,
				resolverCandidate("SZ", "000001", "Ping An Bank"),
				resolverCandidate("JP", "000001", "JP Product"),
			}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "cn", "000001", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.RequestedMarket != "CN" || result.ResolutionStatus != InstrumentResolutionAmbiguous || len(result.Entries) != 2 {
		t.Fatalf("CN resolution = %+v", result)
	}
	if result.Entries[0].InstrumentID != "SH.000001" || result.Entries[1].InstrumentID != "SZ.000001" {
		t.Fatalf("CN entries = %+v", result.Entries)
	}
}

func TestMarketSubsetInstrumentResolverNormalizesProviderPrefixedCodes(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			return []InstrumentCandidate{
				{
					Market:       "US",
					InstrumentID: "US.AAPL",
					Code:         "US.AAPL",
					Name:         "Apple",
				},
				{
					Market:       "US",
					InstrumentID: "US.BRK.B",
					Code:         "BRK.B",
					Name:         "Berkshire Hathaway",
				},
			}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "", "BRK.B", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionResolved || len(result.Entries) != 1 || result.Entries[0].InstrumentID != "US.BRK.B" || result.Entries[0].Code != "BRK.B" {
		t.Fatalf("prefixed-code resolution = %+v", result)
	}
}

func TestMarketSubsetInstrumentResolverUnavailableWhenAllMatchesAreUnsupported(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			return []InstrumentCandidate{
				resolverCandidate("JP", "7203", "Toyota"),
				resolverCandidate("SG", "C6L", "Singapore Airlines"),
			}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "", "airline", 20)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionUnavailable || len(result.Entries) != 2 {
		t.Fatalf("unavailable resolution = %+v", result)
	}
}

func TestMarketSubsetInstrumentResolverLimitsAfterRankingWithoutAutoResolvingHiddenMatches(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			return []InstrumentCandidate{
				resolverCandidate("US", "ONE", "One"),
				resolverCandidate("HK", "TWO", "Two"),
				resolverCandidate("SH", "THREE", "Three"),
			}, nil
		},
	})
	result, err := resolver.Resolve(t.Context(), "", "company", 1)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ResolutionStatus != InstrumentResolutionAmbiguous || result.TotalReturned != 1 || len(result.Entries) != 1 || result.Entries[0].InstrumentID != "US.ONE" {
		t.Fatalf("limited resolution = %+v", result)
	}
}

func TestMarketSubsetInstrumentResolverCachesAndCoalescesNormalizedKeyword(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &resolverProviderStub{
		search: func(_ context.Context, _ string, limit int) ([]InstrumentCandidate, error) {
			if limit != 100 {
				t.Errorf("limit = %d, want 100", limit)
			}
			if calls.Add(1) == 1 {
				close(started)
				<-release
			}
			return []InstrumentCandidate{resolverCandidate("US", "AAPL", "Apple")}, nil
		},
	}
	resolver := NewMarketSubsetInstrumentResolver(provider)
	now := time.Now()
	resolver.now = func() time.Time { return now }

	var wg sync.WaitGroup
	results := make(chan InstrumentResolution, 2)
	errorsCh := make(chan error, 2)
	for _, query := range []string{"Apple", " apple "} {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			result, err := resolver.Resolve(t.Context(), "", query, 20)
			results <- result
			errorsCh <- err
		}(query)
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	}
	for result := range results {
		if result.ResolutionStatus != InstrumentResolutionResolved {
			t.Fatalf("coalesced result = %+v", result)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent provider calls = %d, want 1", calls.Load())
	}

	if _, err := resolver.Resolve(t.Context(), "US", "APPLE", 20); err != nil {
		t.Fatalf("cached Resolve: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached provider calls = %d, want 1", calls.Load())
	}
	now = now.Add(instrumentSearchCacheTTL + time.Second)
	if _, err := resolver.Resolve(t.Context(), "", "apple", 20); err != nil {
		t.Fatalf("expired Resolve: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("post-expiry provider calls = %d, want 2", calls.Load())
	}
}

func TestMarketSubsetInstrumentResolverRechecksCacheInsideSingleflightWork(t *testing.T) {
	var providerCalls atomic.Int32
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			providerCalls.Add(1)
			return nil, errors.New("provider should not be called after cache refill")
		},
	})
	now := time.Now()
	resolver.now = func() time.Time { return now }
	entries := []InstrumentCandidate{resolverCandidate("US", "AAPL", "Apple")}
	resolver.cacheMu.Lock()
	resolver.searchCache["APPLE"] = cachedInstrumentSearch{
		expiresAt: now.Add(instrumentSearchCacheTTL),
		entries:   entries,
	}
	resolver.cacheMu.Unlock()

	got, err := resolver.loadAndCacheSearch(t.Context(), "APPLE", " apple ")
	if err != nil {
		t.Fatalf("loadAndCacheSearch() error = %v", err)
	}
	if providerCalls.Load() != 0 || len(got) != 1 || got[0].InstrumentID != entries[0].InstrumentID {
		t.Fatalf("loadAndCacheSearch() entries=%+v providerCalls=%d", got, providerCalls.Load())
	}
}

func TestMarketSubsetInstrumentResolverDoesNotCacheSearchErrors(t *testing.T) {
	var calls int
	wantErr := errors.New("OpenD search failed")
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			calls++
			return nil, wantErr
		},
	})
	for range 2 {
		_, err := resolver.Resolve(t.Context(), "", "Apple", 20)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Resolve error = %v, want %v", err, wantErr)
		}
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
}

func TestMarketSubsetInstrumentResolverValidatesInput(t *testing.T) {
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{})
	for _, test := range []struct {
		market string
		query  string
		limit  int
	}{
		{query: "   ", limit: 20},
		{query: "Apple", limit: -1},
		{query: "Apple", limit: 101},
		{market: "JP", query: "Toyota", limit: 20},
		{market: "CN", query: "US.AAPL", limit: 20},
	} {
		_, err := resolver.Resolve(t.Context(), test.market, test.query, test.limit)
		if !IsInstrumentSearchInputError(err) {
			t.Errorf("Resolve(%q, %q, %d) error = %v, want input error", test.market, test.query, test.limit, err)
		}
	}
}

func TestMarketSubsetInstrumentResolverPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	resolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{
		search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
			close(started)
			<-release
			return nil, nil
		},
	})
	errCh := make(chan error, 1)
	go func() {
		_, err := resolver.Resolve(ctx, "", "Apple", 20)
		errCh <- err
	}()
	<-started
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Resolve did not stop after context cancellation")
	}
	close(release)
}

func TestClassifyInstrumentResolutionKeepsMultiplePartialCandidatesAmbiguous(t *testing.T) {
	candidates := []InstrumentCandidate{
		{InstrumentID: "SH.000001", Selectable: true},
		{InstrumentID: "SZ.000001", Selectable: true},
	}
	if got := classifyInstrumentResolution(candidates, 1); got != InstrumentResolutionAmbiguous {
		t.Fatalf("classifyInstrumentResolution = %q, want %q", got, InstrumentResolutionAmbiguous)
	}
	if got := classifyInstrumentResolution([]InstrumentCandidate{{InstrumentID: "US.AAPL", Selectable: true}}, 1); got != InstrumentResolutionIncomplete {
		t.Fatalf("partial single resolution = %q, want %q", got, InstrumentResolutionIncomplete)
	}
}

func ExampleInstrumentSearchInputError() {
	err := instrumentSearchInputErrorf("%s", "query is required")
	fmt.Println(IsInstrumentSearchInputError(err), err)
	// Output: true query is required
}
