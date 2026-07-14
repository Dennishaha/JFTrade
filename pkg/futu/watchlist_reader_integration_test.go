package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

func TestBrokerAdapterWatchlistReaderLoadsCachesAndRefreshesOpenDData(t *testing.T) {
	server := startQuoteOpenDServer(t)
	server.setWatchlistData(
		[]*qotgetusersecuritygrouppb.GroupData{
			{GroupName: new("Growth"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_Custom))},
			{GroupName: new("All"), GroupType: new(int32(qotgetusersecuritygrouppb.GroupType_GroupType_System))},
		},
		[]*qotcommonpb.SecurityStaticInfo{futuWatchlistTestSecurity("00700", 700, "Tencent")},
	)
	defer server.stop()

	adapter := newTestBrokerAdapter(t, server)
	reader, ok := adapter.(broker.WatchlistGroupReader)
	if !ok {
		t.Fatal("Futu adapter should expose WatchlistGroupReader")
	}
	freshReader, ok := adapter.(broker.FreshWatchlistGroupReader)
	if !ok {
		t.Fatal("Futu adapter should expose FreshWatchlistGroupReader")
	}

	groups, err := reader.ListWatchlistGroups(t.Context())
	if err != nil || len(groups) != 2 || groups[0].Name != "Growth" || groups[0].Type != "custom" || groups[1].Type != "system" {
		t.Fatalf("ListWatchlistGroups = (%#v, %v)", groups, err)
	}

	securities, err := reader.ListWatchlistGroupSecurities(t.Context(), " growth ")
	if err != nil || len(securities) != 1 || securities[0].InstrumentID != "HK.00700" || securities[0].Name == nil || *securities[0].Name != "Tencent" || securities[0].BrokerSecurityID == nil || *securities[0].BrokerSecurityID != "700" {
		t.Fatalf("ListWatchlistGroupSecurities = (%#v, %v)", securities, err)
	}
	if _, err := reader.ListWatchlistGroupSecurities(t.Context(), "Growth"); err != nil {
		t.Fatalf("cached ListWatchlistGroupSecurities: %v", err)
	}

	if _, err := freshReader.ListWatchlistGroupsFresh(t.Context()); err != nil {
		t.Fatalf("ListWatchlistGroupsFresh: %v", err)
	}
	if _, err := freshReader.ListWatchlistGroupSecuritiesFresh(t.Context(), "Growth"); err != nil {
		t.Fatalf("ListWatchlistGroupSecuritiesFresh: %v", err)
	}

	if got := server.userGroupCalls.Load(); got != 3 {
		t.Fatalf("Qot_GetUserSecurityGroup calls = %d, want 3 (cached reads must not consume quota)", got)
	}
	if got := server.userSecCalls.Load(); got != 2 {
		t.Fatalf("Qot_GetUserSecurity calls = %d, want 2 (one cached, one fresh)", got)
	}
	server.tradeMu.Lock()
	groupType := server.lastGroupType
	groupName := server.lastGroupName
	server.tradeMu.Unlock()
	if groupType != int32(qotgetusersecuritygrouppb.GroupType_GroupType_All) || groupName != "Growth" {
		t.Fatalf("OpenD watchlist requests = (groupType:%d groupName:%q)", groupType, groupName)
	}
}
