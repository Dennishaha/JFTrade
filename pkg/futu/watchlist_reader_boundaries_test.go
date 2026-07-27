package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

func TestWatchlistGroupReaderCoversSecondCacheAndFailures(t *testing.T) {
	base := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	reader := newFutuWatchlistReader(nil)
	reader.groups = futuWatchlistGroupCacheEntry{expiresAt: base, value: []broker.WatchlistGroup{{Name: "cached"}}}
	times := []time.Time{base, base.Add(-time.Second)}
	reader.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	groups, err := reader.ListWatchlistGroups(t.Context())
	if err != nil || len(groups) != 1 || groups[0].Name != "cached" {
		t.Fatalf("second cached group lookup = %#v, %v", groups, err)
	}

	reader = newFutuWatchlistReader(nil)
	reader.now = func() time.Time { return base }
	reader.gate.limit = 0
	if _, err := reader.ListWatchlistGroupsFresh(t.Context()); !errors.Is(err, ErrWatchlistReadRateLimited) {
		t.Fatalf("rate-limited groups error = %v", err)
	}
	reader.gate = futuWatchlistReadGate{limit: 1, window: time.Minute}
	reader.loadGroups = nil
	if _, err := reader.ListWatchlistGroupsFresh(t.Context()); err == nil {
		t.Fatal("missing group loader error = nil")
	}
	reader.gate = futuWatchlistReadGate{limit: 1, window: time.Minute}
	reader.loadGroups = func(_ context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) {
		return nil, errors.New("group failure")
	}
	if _, err := reader.ListWatchlistGroupsFresh(t.Context()); err == nil {
		t.Fatal("group loader error = nil")
	}
}

func TestWatchlistMemberReaderCoversSecondCacheAndFailures(t *testing.T) {
	base := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	newReader := func() *futuWatchlistReader {
		reader := newFutuWatchlistReader(nil)
		reader.now = func() time.Time { return base }
		reader.loadGroups = func(_ context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) {
			return []*qotgetusersecuritygrouppb.GroupData{{GroupName: new("Growth")}}, nil
		}
		return reader
	}

	reader := newReader()
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), " "); err == nil {
		t.Fatal("blank group name error = nil")
	}
	reader.loadGroups = func(_ context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) {
		return nil, errors.New("group failure")
	}
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth"); err == nil {
		t.Fatal("member group-list failure = nil")
	}
	reader = newReader()
	reader.loadGroups = func(_ context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) { return nil, nil }
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth"); err == nil {
		t.Fatal("missing group error = nil")
	}

	reader = newReader()
	reader.groups = futuWatchlistGroupCacheEntry{expiresAt: base.Add(time.Hour), value: []broker.WatchlistGroup{{Name: "Growth"}}}
	reader.members["growth"] = futuWatchlistMemberCacheEntry{expiresAt: base, value: []broker.WatchlistSecurity{{InstrumentID: "HK.00700"}}}
	times := []time.Time{base.Add(-2 * time.Minute), base, base.Add(-time.Second)}
	reader.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	members, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth")
	if err != nil || len(members) != 1 {
		t.Fatalf("second cached member lookup = %#v, %v", members, err)
	}

	reader = newReader()
	reader.groups = futuWatchlistGroupCacheEntry{expiresAt: base.Add(time.Minute), value: []broker.WatchlistGroup{{Name: "Growth"}}}
	reader.gate.limit = 1
	reader.gate.window = time.Minute
	reader.gate.calls = []time.Time{base}
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth"); !errors.Is(err, ErrWatchlistReadRateLimited) {
		t.Fatalf("rate-limited members error = %v", err)
	}
	reader = newReader()
	reader.loadMembers = nil
	if _, err := reader.ListWatchlistGroupSecuritiesFresh(t.Context(), "Growth"); err == nil {
		t.Fatal("missing member loader error = nil")
	}
	reader = newReader()
	reader.loadMembers = func(_ context.Context, _ string) ([]*qotcommonpb.SecurityStaticInfo, error) {
		return nil, errors.New("member failure")
	}
	if _, err := reader.ListWatchlistGroupSecuritiesFresh(t.Context(), "Growth"); err == nil {
		t.Fatal("member loader error = nil")
	}
}

func TestWatchlistConversionAndGateRejectInvalidInputs(t *testing.T) {
	if (&futuWatchlistReadGate{}).allow(time.Now()) {
		t.Fatal("zero watchlist gate unexpectedly allowed a call")
	}
	groups := convertFutuWatchlistGroups([]*qotgetusersecuritygrouppb.GroupData{
		nil,
		{GroupName: new(" ")},
		{GroupName: new("Unknown"), GroupType: new(int32(-1))},
	})
	if len(groups) != 1 || groups[0].Type != "unknown" {
		t.Fatalf("converted boundary groups = %#v", groups)
	}
	securities := convertFutuWatchlistSecurities([]*qotcommonpb.SecurityStaticInfo{
		nil,
		{},
		{Basic: &qotcommonpb.SecurityStaticBasic{Security: &qotcommonpb.Security{Market: new(int32(-1)), Code: new("BAD")}}},
	})
	if len(securities) != 0 {
		t.Fatalf("converted invalid securities = %#v", securities)
	}
}
