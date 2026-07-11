package futu

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

func TestFutuWatchlistReadGateEnforcesTenCallsPerRollingThirtySeconds(t *testing.T) {
	gate := futuWatchlistReadGate{limit: 10, window: 30 * time.Second}
	start := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	for i := range 10 {
		if !gate.allow(start.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("call %d unexpectedly rejected", i+1)
		}
	}
	if gate.allow(start.Add(29 * time.Second)) {
		t.Fatal("11th call inside rolling window unexpectedly allowed")
	}
	if !gate.allow(start.Add(30 * time.Second)) {
		t.Fatal("call at expiry of first reservation should be allowed")
	}
}

func TestFutuWatchlistReaderCacheUsesTTLAndReturnsCopies(t *testing.T) {
	reader := newFutuWatchlistReader(nil)
	now := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	reader.groups = futuWatchlistGroupCacheEntry{
		expiresAt: now.Add(30 * time.Second),
		value:     []broker.WatchlistGroup{{Name: "Long Term", Type: "custom"}},
	}
	name := "Tencent"
	reader.members["long term"] = futuWatchlistMemberCacheEntry{
		expiresAt: now.Add(30 * time.Second),
		value: []broker.WatchlistSecurity{{
			InstrumentID: "HK.00700",
			Name:         &name,
		}},
	}

	groups, ok := reader.cachedGroups(now.Add(29 * time.Second))
	if !ok || len(groups) != 1 {
		t.Fatalf("cached groups = (%#v, %t)", groups, ok)
	}
	groups[0].Name = "mutated"
	again, ok := reader.cachedGroups(now.Add(29 * time.Second))
	if !ok || again[0].Name != "Long Term" {
		t.Fatalf("cached groups mutated through caller = %#v", again)
	}
	members, ok := reader.cachedMembers("long term", now.Add(29*time.Second))
	if !ok || members[0].Name == nil {
		t.Fatalf("cached members = (%#v, %t)", members, ok)
	}
	*members[0].Name = "mutated"
	againMembers, ok := reader.cachedMembers("long term", now.Add(29*time.Second))
	if !ok || againMembers[0].Name == nil || *againMembers[0].Name != "Tencent" {
		t.Fatalf("cached members mutated through caller = %#v", againMembers)
	}
	if _, ok := reader.cachedGroups(now.Add(30 * time.Second)); ok {
		t.Fatal("group cache should expire at TTL boundary")
	}
	if _, ok := reader.cachedMembers("long term", now.Add(30*time.Second)); ok {
		t.Fatal("member cache should expire at TTL boundary")
	}
}

func TestConvertFutuWatchlistGroupsMarksEveryNormalizedDuplicateAmbiguous(t *testing.T) {
	groups := convertFutuWatchlistGroups([]*qotgetusersecuritygrouppb.GroupData{
		{GroupName: new("Growth"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom))},
		{GroupName: new(" growth "), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom))},
		{GroupName: new("All"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_System))},
	})
	if len(groups) != 3 {
		t.Fatalf("groups = %#v", groups)
	}
	if !groups[0].Ambiguous || !groups[1].Ambiguous {
		t.Fatalf("duplicate groups should both be ambiguous: %#v", groups)
	}
	if groups[2].Ambiguous || groups[2].Type != "system" {
		t.Fatalf("system group = %#v", groups[2])
	}
}

func TestConvertFutuWatchlistSecuritiesPreservesCanonicalIDAndBrokerAlias(t *testing.T) {
	securities := convertFutuWatchlistSecurities([]*qotcommonpb.SecurityStaticInfo{{
		Basic: &qotcommonpb.SecurityStaticBasic{
			Security: &qotcommonpb.Security{
				Market: new(int32(qotcommonpb.QotMarket_QotMarket_JP_Security)),
				Code:   new("7203"),
			},
			Id:       new(int64(123456)),
			LotSize:  new(int32(100)),
			SecType:  new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
			Name:     new("Toyota"),
			ListTime: new("1949-05-16"),
		},
	}})
	if len(securities) != 1 {
		t.Fatalf("securities = %#v", securities)
	}
	got := securities[0]
	if got.InstrumentID != "JP.7203" || got.BrokerCode == nil || *got.BrokerCode != "7203" ||
		got.BrokerSecurityID == nil || *got.BrokerSecurityID != "123456" || *got.BrokerCode == *got.BrokerSecurityID {
		t.Fatalf("security = %#v", got)
	}
}

func TestFutuWatchlistFreshReadBypassesAndReplacesGroupAndMemberCaches(t *testing.T) {
	reader := newFutuWatchlistReader(nil)
	now := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	reader.now = func() time.Time { return now }
	groupCalls := 0
	memberCalls := 0
	memberName := "Tencent v1"
	reader.loadGroups = func(context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) {
		groupCalls++
		return []*qotgetusersecuritygrouppb.GroupData{{
			GroupName: new("Growth"),
			GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom)),
		}}, nil
	}
	reader.loadMembers = func(_ context.Context, groupName string) ([]*qotcommonpb.SecurityStaticInfo, error) {
		memberCalls++
		if groupName != "Growth" {
			t.Fatalf("groupName = %q", groupName)
		}
		return []*qotcommonpb.SecurityStaticInfo{futuWatchlistTestSecurity("00700", 700, memberName)}, nil
	}

	first, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth")
	if err != nil || len(first) != 1 || first[0].Name == nil || *first[0].Name != "Tencent v1" {
		t.Fatalf("initial members = (%#v, %v)", first, err)
	}
	memberName = "Tencent v2"
	cached, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth")
	if err != nil || cached[0].Name == nil || *cached[0].Name != "Tencent v1" {
		t.Fatalf("cached members = (%#v, %v)", cached, err)
	}
	if groupCalls != 1 || memberCalls != 1 {
		t.Fatalf("cached call counts = groups:%d members:%d", groupCalls, memberCalls)
	}

	fresh, err := reader.ListWatchlistGroupSecuritiesFresh(t.Context(), "Growth")
	if err != nil || len(fresh) != 1 || fresh[0].Name == nil || *fresh[0].Name != "Tencent v2" {
		t.Fatalf("fresh members = (%#v, %v)", fresh, err)
	}
	if groupCalls != 2 || memberCalls != 2 {
		t.Fatalf("fresh call counts = groups:%d members:%d", groupCalls, memberCalls)
	}
	afterFresh, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth")
	if err != nil || afterFresh[0].Name == nil || *afterFresh[0].Name != "Tencent v2" {
		t.Fatalf("cache after fresh read = (%#v, %v)", afterFresh, err)
	}
	if groupCalls != 2 || memberCalls != 2 {
		t.Fatalf("post-fresh cached call counts = groups:%d members:%d", groupCalls, memberCalls)
	}
}

func TestFutuWatchlistFreshMemberReadRechecksRemoteAmbiguity(t *testing.T) {
	reader := newFutuWatchlistReader(nil)
	reader.now = func() time.Time { return time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC) }
	groups := []*qotgetusersecuritygrouppb.GroupData{{
		GroupName: new("Growth"),
		GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom)),
	}}
	memberCalls := 0
	reader.loadGroups = func(context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) { return groups, nil }
	reader.loadMembers = func(context.Context, string) ([]*qotcommonpb.SecurityStaticInfo, error) {
		memberCalls++
		return []*qotcommonpb.SecurityStaticInfo{futuWatchlistTestSecurity("00700", 700, "Tencent")}, nil
	}
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth"); err != nil {
		t.Fatalf("initial member read: %v", err)
	}
	groups = append(groups, &qotgetusersecuritygrouppb.GroupData{
		GroupName: new(" growth "),
		GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom)),
	})
	_, err := reader.ListWatchlistGroupSecuritiesFresh(t.Context(), "Growth")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("fresh ambiguity error = %v", err)
	}
	if memberCalls != 1 {
		t.Fatalf("ambiguous fresh read should stop before 3213; member calls = %d", memberCalls)
	}
}

func futuWatchlistTestSecurity(code string, id int64, name string) *qotcommonpb.SecurityStaticInfo {
	return &qotcommonpb.SecurityStaticInfo{Basic: &qotcommonpb.SecurityStaticBasic{
		Security: &qotcommonpb.Security{
			Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   &code,
		},
		Id:       &id,
		LotSize:  new(int32(100)),
		SecType:  new(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
		Name:     &name,
		ListTime: new("2004-06-16"),
	}}
}
