package futu

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestLiveOpenDProto108Contract(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
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
