package watchlist

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCoverage98BatchQuotesRejectsUnsafeInputAndHonorsCanceledFlights(t *testing.T) {
	service := NewService(nil, WithBatchSnapshotSource(quoteResultSource{}))
	for _, instrumentIDs := range [][]string{nil, {"AAPL"}} {
		if _, err := service.BatchQuotes(t.Context(), instrumentIDs); !errors.Is(err, ErrValidation) {
			t.Fatalf("BatchQuotes(%v) error = %v, want validation failure", instrumentIDs, err)
		}
	}

	owned, waiting := service.reserveQuoteFlights([]string{"US.AAPL"})
	if !slices.Equal(owned, []string{"US.AAPL"}) || len(waiting) != 0 {
		t.Fatalf("initial quote reservation = owned:%#v waiting:%#v", owned, waiting)
	}
	duplicateOwned, duplicateWaiting := service.reserveQuoteFlights([]string{"US.AAPL", "US.AAPL"})
	if len(duplicateOwned) != 0 || len(duplicateWaiting) != 1 {
		t.Fatalf("single-flight duplicate reservation = owned:%#v waiting:%#v", duplicateOwned, duplicateWaiting)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.BatchQuotes(ctx, []string{"US.AAPL"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled quote waiter error = %v", err)
	}
	service.completeQuoteFlights(owned, nil, nil, nil)
}

func TestCoverage98QuoteCacheAndImportHelpersKeepAbsentDataExplicit(t *testing.T) {
	now := time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)
	service := NewService(nil, WithClock(func() time.Time { return now }))
	service.quoteCache["US.AAPL"] = quoteCacheEntry{expiresAt: now}
	quotes, itemErrors := service.collectQuoteCache([]string{"US.AAPL", "US.MSFT"})
	if len(quotes) != 0 || len(itemErrors) != 2 || itemErrors[0].Code != "NO_SNAPSHOT" || itemErrors[1].InstrumentID != "US.MSFT" {
		t.Fatalf("expired and absent quote cache = quotes:%#v errors:%#v", quotes, itemErrors)
	}

	updates := 0
	metadataService := NewService(&serviceTestRepository{updateMetadata: func(context.Context, []InstrumentMetadata) error {
		updates++
		return nil
	}})
	metadataService.updateInstrumentMetadata(t.Context(), []Quote{{InstrumentID: "US.AAPL"}})
	if updates != 0 {
		t.Fatalf("empty quote metadata caused %d repository writes", updates)
	}

	var nilService *Service
	if _, err := nilService.sourceReader("futu:default"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil source reader error = %v", err)
	}
	if normalizeLimit(17) != 17 {
		t.Fatalf("in-range page limit was rewritten")
	}
	if got := normalizedUniqueFold([]string{" ", "Growth", " growth ", "Value"}); !slices.Equal(got, []string{"Growth", "Value"}) {
		t.Fatalf("folded group names = %#v", got)
	}
	_, _, localOnly := diffMembers(nil, []string{"US.TSLA", "US.AAPL"})
	if !slices.EqualFunc(localOnly, []ImportDiffItem{{InstrumentID: "US.AAPL", Selected: false}, {InstrumentID: "US.TSLA", Selected: false}}, func(left, right ImportDiffItem) bool {
		return left == right
	}) {
		t.Fatalf("local-only diff sort = %#v", localOnly)
	}
}

func TestCoverage98PreviewImportRejectsInvalidDerivedGroupName(t *testing.T) {
	reader := &serviceTestReader{
		groups:  []RemoteGroup{{RemoteGroupID: "remote-1", Name: "Remote"}},
		members: []RemoteMember{{InstrumentID: "US.AAPL"}},
	}
	service := NewService(&serviceTestRepository{
		replaceRemoteGroups: func(context.Context, string, []RemoteGroup) error { return nil },
	}, WithSourceReader("source", reader))
	_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{
		SourceID: "source", RemoteGroupID: "remote-1", NewGroupName: strings.Repeat("界", 65),
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("oversized derived group error = %v", err)
	}
}
