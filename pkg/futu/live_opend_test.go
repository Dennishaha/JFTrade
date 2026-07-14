package futu

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestLiveOpenDProto108Contract(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	if err := exchange.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	client := exchange.Client()
	state, err := client.GetGlobalState(ctx)
	if err != nil {
		t.Fatalf("GetGlobalState: %v", err)
	}
	if err := opend.ValidateMinimumVersion(state.ServerVer, &state.ServerBuildNo); err != nil {
		t.Fatal(err)
	}
	t.Logf("OpenD version=%s quoteLoggedIn=%v tradeLoggedIn=%v", opend.FormatVersion(state.ServerVer, state.ServerBuildNo), state.QotLogined, state.TrdLogined)

	beforeSearch, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo before search: %v", err)
	}
	aaplResults, err := client.GetSearchQuote(ctx, "AAPL", 100)
	if err != nil {
		t.Fatalf("GetSearchQuote AAPL: %v", err)
	}
	for index, candidate := range aaplResults {
		if index >= 5 {
			break
		}
		t.Logf("AAPL candidate[%d] market=%s(%d) code=%q name=%q type=%s",
			index,
			qotcommonpb.QotMarket(candidate.GetMarket()).String(),
			candidate.GetMarket(),
			candidate.GetCode(),
			candidate.GetName(),
			qotcommonpb.SecurityType(candidate.GetSecType()).String(),
		)
	}
	foundNamedAAPL := false
	for _, candidate := range aaplResults {
		if qotcommonpb.QotMarket(candidate.GetMarket()) == qotcommonpb.QotMarket_QotMarket_US_Security &&
			canonicalSearchQuoteSymbol("US", candidate.GetCode()) == "US.AAPL" && candidate.GetName() != "" {
			foundNamedAAPL = true
			break
		}
	}
	if !foundNamedAAPL {
		t.Fatalf("GetSearchQuote AAPL returned no named US.AAPL candidate: %#v", aaplResults)
	}
	chineseResults, err := client.GetSearchQuote(ctx, "腾讯", 100)
	if err != nil {
		t.Fatalf("GetSearchQuote Chinese name: %v", err)
	}
	foundNamedChineseResult := false
	for _, candidate := range chineseResults {
		if candidate.GetName() != "" {
			foundNamedChineseResult = true
			break
		}
	}
	if !foundNamedChineseResult {
		t.Fatalf("GetSearchQuote Chinese name returned no named candidates: %#v", chineseResults)
	}
	afterSearch, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo after search: %v", err)
	}
	if len(beforeSearch.GetConnSubInfoList()) != len(afterSearch.GetConnSubInfoList()) {
		t.Fatalf("search changed current-connection subscription count: before=%d after=%d",
			len(beforeSearch.GetConnSubInfoList()), len(afterSearch.GetConnSubInfoList()))
	}
	for index := range beforeSearch.GetConnSubInfoList() {
		if !proto.Equal(beforeSearch.GetConnSubInfoList()[index], afterSearch.GetConnSubInfoList()[index]) {
			t.Fatalf("search changed current-connection subscription state: before=%v after=%v",
				beforeSearch.GetConnSubInfoList(), afterSearch.GetConnSubInfoList())
		}
	}
	t.Logf("search AAPL=%d Chinese=%d currentConnectionSubscriptions=%d usedQuota=%d remainQuota=%d",
		len(aaplResults),
		len(chineseResults),
		liveCurrentConnectionSubscriptionCount(afterSearch),
		afterSearch.GetTotalUsedQuota(),
		afterSearch.GetRemainQuota(),
	)

	security := &qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   new("00700"),
	}
	snapshots, err := client.GetSecuritySnapshot(ctx, []*qotcommonpb.Security{security})
	if err != nil || len(snapshots) != 1 || snapshots[0].GetBasic().GetSecurity().GetCode() != "00700" {
		t.Fatalf("GetSecuritySnapshot = (%d, %v)", len(snapshots), err)
	}

	book, err := exchange.QueryOrderBook(ctx, "HK.00700", 5)
	if err != nil {
		t.Fatalf("QueryOrderBook: %v", err)
	}
	if book.Security == nil || book.Security.GetCode() != "00700" || len(book.BidList)+len(book.AskList) == 0 {
		t.Fatalf("QueryOrderBook returned no HK.00700 levels: %#v", book)
	}
	t.Logf("HK.00700 snapshotPrice=%v orderBookBids=%d asks=%d", snapshots[0].GetBasic().GetCurPrice(), len(book.BidList), len(book.AskList))
}

func liveCurrentConnectionSubscriptionCount(info interface {
	GetConnSubInfoList() []*qotcommonpb.ConnSubInfo
}) int {
	total := 0
	for _, connection := range info.GetConnSubInfoList() {
		if connection != nil && connection.GetIsOwnConnData() {
			total += len(connection.GetSubInfoList())
		}
	}
	return total
}
