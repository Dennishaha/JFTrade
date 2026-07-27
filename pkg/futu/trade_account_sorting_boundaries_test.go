package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestTradeAccountResolverUsesStableTieBreakersAndSurfacesClientClose(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	simulatedSecond := testSimulateHKCashAccount()
	simulatedSecond.AccID = new(uint64(1002))
	server.setAccounts([]*trdcommonpb.TrdAcc{simulatedSecond, testSimulateHKCashAccount()})
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	resolved, err := exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{})
	if err != nil || resolved.AccountID != "1001" {
		t.Fatalf("account-id tie break = %#v, %v; want 1001", resolved, err)
	}

	us := testSimulateHKCashAccount()
	us.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	hk := testSimulateHKCashAccount()
	server.setAccounts([]*trdcommonpb.TrdAcc{us, hk})
	resolved, err = exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{})
	if err != nil || resolved.Market != "HK" {
		t.Fatalf("market tie break = %#v, %v; want HK", resolved, err)
	}

	closedClient := opend.New(opend.Config{})
	jftradeCheckTestError(t, closedClient.Close())
	if _, err := exchange.resolveTradeAccountWithClient(t.Context(), closedClient, BrokerReadQuery{}); err == nil {
		t.Fatal("closed OpenD client account lookup error = nil")
	}
}
