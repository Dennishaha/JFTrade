package servercore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type errorWatchlistReader struct {
	groupErr  error
	memberErr error
	freshErr  error
}

func (r errorWatchlistReader) ListWatchlistGroups(context.Context) ([]broker.WatchlistGroup, error) {
	return nil, r.groupErr
}

func (r errorWatchlistReader) ListWatchlistGroupSecurities(context.Context, string) ([]broker.WatchlistSecurity, error) {
	return nil, r.memberErr
}

func (r errorWatchlistReader) ListWatchlistGroupsFresh(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return r.ListWatchlistGroups(ctx)
}

func (r errorWatchlistReader) ListWatchlistGroupSecuritiesFresh(context.Context, string) ([]broker.WatchlistSecurity, error) {
	return nil, r.freshErr
}

type basicWatchlistReader struct{}

func (basicWatchlistReader) ListWatchlistGroups(context.Context) ([]broker.WatchlistGroup, error) {
	return nil, nil
}

func (basicWatchlistReader) ListWatchlistGroupSecurities(context.Context, string) ([]broker.WatchlistSecurity, error) {
	return nil, nil
}

type nilSnapshotSource struct{}

func (nilSnapshotSource) QuerySecuritySnapshot(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return nil, nil
}

func TestFutuWatchlistReaderRemainingSourceAndReadErrors(t *testing.T) {
	reader := newFutuWatchlistReader(nil, func() bool { return true })
	if source, err := reader.Source(t.Context()); err != nil || source.Status != "unavailable" {
		t.Fatalf("nil source = %#v, %v", source, err)
	}
	if _, err := (*futuWatchlistReader)(nil).ListGroups(t.Context()); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("nil ListGroups error = %v", err)
	}

	providerErr := errors.New("reader provider failed")
	reader = newFutuWatchlistReader(func() (broker.WatchlistGroupReader, error) { return nil, providerErr }, func() bool { return true })
	if _, err := reader.ListGroups(t.Context()); !errors.Is(err, providerErr) {
		t.Fatalf("provider ListGroups error = %v", err)
	}

	groupErr := errors.New("groups failed")
	reader = newFutuWatchlistReader(func() (broker.WatchlistGroupReader, error) { return errorWatchlistReader{groupErr: groupErr}, nil }, func() bool { return true })
	if _, err := reader.ListGroups(t.Context()); !errors.Is(err, groupErr) {
		t.Fatalf("remote ListGroups error = %v", err)
	}
	if _, err := reader.ListGroupMembers(t.Context(), " "); !errors.Is(err, watchlist.ErrValidation) {
		t.Fatalf("blank group error = %v", err)
	}
	if _, err := reader.ListGroupMembers(t.Context(), "futu-group:missing"); !errors.Is(err, groupErr) {
		t.Fatalf("member group-read error = %v", err)
	}

	fake := &fakeBrokerWatchlistReader{groups: []broker.WatchlistGroup{{Name: "Group", Type: "custom"}}, members: map[string][]broker.WatchlistSecurity{}}
	reader = newFutuWatchlistReader(func() (broker.WatchlistGroupReader, error) { return fake, nil }, func() bool { return true })
	if _, err := reader.ListGroupMembers(t.Context(), "futu-group:missing"); !errors.Is(err, watchlist.ErrNotFound) {
		t.Fatalf("missing group error = %v", err)
	}
	groups, err := reader.ListGroups(t.Context())
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %#v, %v", groups, err)
	}
	reader.reader = func() (broker.WatchlistGroupReader, error) { return nil, providerErr }
	if _, err := reader.ListGroupMembers(t.Context(), groups[0].RemoteGroupID); !errors.Is(err, providerErr) {
		t.Fatalf("member provider error = %v", err)
	}

	memberErr := errors.New("members failed")
	reader.reader = func() (broker.WatchlistGroupReader, error) { return errorWatchlistReader{memberErr: memberErr}, nil }
	if _, err := reader.ListGroupMembers(t.Context(), groups[0].RemoteGroupID); !errors.Is(err, memberErr) {
		t.Fatalf("member read error = %v", err)
	}
}

func TestFutuWatchlistFreshAndRemoteIDRemainingBoundaries(t *testing.T) {
	for _, id := range []string{"", "wrong-prefix", "futu-group:%%%", "futu-group:" + "YQ", futuRemoteGroupID("custom", "", 0, 1)} {
		if _, err := futuRemoteGroupName(id); !errors.Is(err, watchlist.ErrValidation) {
			t.Fatalf("remote group id %q error = %v", id, err)
		}
	}
	if _, found := remoteGroupByID(nil, "missing"); found {
		t.Fatal("missing remote group found")
	}

	validID := futuRemoteGroupID("custom", "Group", 0, 1)
	providerErr := errors.New("provider failed")
	reader := newFutuWatchlistReader(func() (broker.WatchlistGroupReader, error) { return nil, providerErr }, func() bool { return true })
	if _, err := reader.ListGroupMembersFresh(t.Context(), validID); !errors.Is(err, providerErr) {
		t.Fatalf("fresh provider error = %v", err)
	}
	reader.reader = func() (broker.WatchlistGroupReader, error) { return basicWatchlistReader{}, nil }
	if _, err := reader.ListGroupMembersFresh(t.Context(), validID); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("unsupported fresh reader error = %v", err)
	}
	freshErr := errors.New("fresh members failed")
	reader.reader = func() (broker.WatchlistGroupReader, error) { return errorWatchlistReader{freshErr: freshErr}, nil }
	if _, err := reader.ListGroupMembersFresh(t.Context(), validID); !errors.Is(err, freshErr) {
		t.Fatalf("fresh member error = %v", err)
	}

	name, securityType, code, securityID := " Name ", " Equity ", " 001 ", " 1 "
	members := remoteMembersFromBroker([]broker.WatchlistSecurity{{
		InstrumentID: "HK.00001", Name: &name, SecurityType: &securityType, BrokerCode: &code, BrokerSecurityID: &securityID,
	}})
	if len(members) != 1 || members[0].Name != "Name" || members[0].Type != "Equity" || members[0].BrokerCode != "001" || members[0].SecurityID != "1" {
		t.Fatalf("normalized members = %#v", members)
	}
	if got := (*futuWatchlistReader)(nil).clock(); got.IsZero() {
		t.Fatal("nil reader clock returned zero")
	}
}

func TestFutuSnapshotRemainingProviderRateLimitAndSplitPaths(t *testing.T) {
	if _, _, err := (*futuWatchlistSnapshotSource)(nil).BatchSnapshots(t.Context(), nil); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("nil snapshot source error = %v", err)
	}
	providerErr := errors.New("snapshot provider failed")
	source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return nil, providerErr })
	if _, _, err := source.BatchSnapshots(t.Context(), []string{"US.AAPL"}); !errors.Is(err, providerErr) {
		t.Fatalf("snapshot provider error = %v", err)
	}

	rateLimited := brokerSnapshotSourceFunc(func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
		return nil, broker.NewSnapshotRateLimitError(12*time.Second, nil)
	})
	items, itemErrors := queryFutuSnapshotBatch(t.Context(), rateLimited, []string{"US.AAPL", "US.MSFT"})
	if items != nil || len(itemErrors) != 2 || itemErrors[0].Code != "SNAPSHOT_RATE_LIMITED" {
		t.Fatalf("rate limited snapshot = %#v, %#v", items, itemErrors)
	}
	items, itemErrors = queryFutuSnapshotBatch(t.Context(), nilSnapshotSource{}, []string{"US.AAPL"})
	if len(items) != 0 || len(itemErrors) != 1 || itemErrors[0].Code != "SNAPSHOT_NOT_RETURNED" {
		t.Fatalf("nil result snapshot = %#v, %#v", items, itemErrors)
	}

	fake := &recordingBatchSnapshotSource{failID: "US.BAD"}
	items, itemErrors = queryFutuSnapshotBatch(t.Context(), fake, []string{"US.BAD", "US.GOOD"})
	if _, ok := items["US.GOOD"]; !ok || len(itemErrors) != 1 || itemErrors[0].InstrumentID != "US.BAD" {
		t.Fatalf("split snapshot = %#v, %#v", items, itemErrors)
	}

	invalidResult := brokerSnapshotSourceFunc(func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
		return &broker.SecuritySnapshotResult{Snapshots: []broker.SecuritySnapshotItem{{Symbol: "invalid"}}}, nil
	})
	items, itemErrors = queryFutuSnapshotBatch(t.Context(), invalidResult, []string{"US.AAPL"})
	if len(items) != 0 || len(itemErrors) != 1 {
		t.Fatalf("invalid symbol snapshot = %#v, %#v", items, itemErrors)
	}

}

type brokerSnapshotSourceFunc func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error)

func (f brokerSnapshotSourceFunc) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return f(ctx, query)
}

func TestWatchlistQuoteRemainingFormattingAndTimeBoundaries(t *testing.T) {
	if watchlistStringValue(nil) != "" {
		t.Fatal("nil watchlist string was non-empty")
	}
	blank := " "
	if parseWatchlistSnapshotTime("US.AAPL", nil) != nil || parseWatchlistSnapshotTime("US.AAPL", &blank) != nil {
		t.Fatal("blank snapshot time parsed")
	}
	invalid := "not-a-time"
	if parseWatchlistSnapshotTime("US.AAPL", &invalid) != nil {
		t.Fatal("invalid known-market time parsed")
	}
	rfc3339 := "2026-07-15T12:30:00+08:00"
	if parsed := parseWatchlistSnapshotTime("US.AAPL", &rfc3339); parsed == nil || parsed.Location() != time.UTC {
		t.Fatalf("RFC3339 snapshot time = %v", parsed)
	}
	legacy := "2026-07-15 09:30:00.000"
	if parsed := parseWatchlistSnapshotTime("US.AAPL", &legacy); parsed == nil {
		t.Fatal("legacy snapshot time did not parse")
	}

	fallback := 10.0
	if preferredExtendedPrice(nil, &fallback) != &fallback {
		t.Fatal("preferred extended fallback changed")
	}
	if extendedQuoteBlock(nil, time.Now(), nil, nil) != nil {
		t.Fatal("nil extended block was non-nil")
	}
	previous, price := 0.0, 1.0
	block := extendedQuoteBlock(&broker.ExtendedSessionSnapshot{Price: &price}, time.Now(), nil, &previous)
	if block == nil || block.Change == nil || block.ChangePercent != nil {
		t.Fatalf("zero-close extended block = %#v", block)
	}

	session := "regular"
	quote := watchlistQuoteFromBrokerSnapshot("US.AAPL", broker.SecuritySnapshotItem{
		Session: &session, LastPrice: &price, PreviousClose: &previous,
	}, time.Now().UTC())
	if quote.Extended != nil || quote.Change == nil || quote.ChangePercent != nil {
		t.Fatalf("minimal quote = %#v", quote)
	}
	if cloneFloat(nil) != nil {
		t.Fatal("nil float clone was non-nil")
	}
}
