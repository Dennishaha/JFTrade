package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestQueryAllKLinesReturnsCompleteSortedHistoricalSeries(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	start := time.Date(2026, time.June, 22, 13, 30, 0, 0, time.UTC)
	server.setHistorySeries([]*qotcommonpb.KLine{
		testHistoryKLine(start.Add(10*time.Minute), 102),
		testHistoryKLine(start, 100),
		testHistoryKLine(start.Add(5*time.Minute), 101),
	})

	klines, err := exchange.QueryAllKLines(
		t.Context(),
		"HK.00700",
		types.Interval5m,
		start,
		start.Add(15*time.Minute),
		qotcommonpb.RehabType_RehabType_Forward,
	)
	if err != nil {
		t.Fatalf("QueryAllKLines() error = %v", err)
	}
	if len(klines) != 3 {
		t.Fatalf("QueryAllKLines() returned %d klines, want 3", len(klines))
	}
	for index := 1; index < len(klines); index++ {
		if klines[index].StartTime.Time().Before(klines[index-1].StartTime.Time()) {
			t.Fatalf("klines are not sorted: %#v", klines)
		}
	}
	if got := server.currentKLCallCount(); got != 0 {
		t.Fatalf("QueryAllKLines current K-line calls = %d, want 0", got)
	}

	server.setHistorySessionError(0, 1, "history unavailable")
	if _, err := exchange.QueryAllKLines(t.Context(), "HK.00700", types.Interval5m, start, start.Add(time.Hour), qotcommonpb.RehabType_RehabType_None); err == nil {
		t.Fatal("QueryAllKLines(OpenD failure) error = nil")
	}
}

func TestExchangeReadAPIsPropagateOpenDQuoteAndSnapshotFailures(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	if err := exchange.SubscribeBasicQuote(t.Context(), "HK.00700", false); err != nil {
		t.Fatalf("SubscribeBasicQuote() error = %v", err)
	}
	server.setBasicQotError(1, 7, "quote entitlement denied")

	if _, err := exchange.QueryTicker(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QueryTicker(OpenD failure) error = nil")
	}
	if _, err := exchange.QueryTickers(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QueryTickers(OpenD failure) error = nil")
	}
	if _, err := exchange.QueryQuoteSnapshot(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QueryQuoteSnapshot(OpenD failure) error = nil")
	}

	server.setSecuritySnapshotError(1, 8, "snapshot entitlement denied")
	if _, err := exchange.QuerySecuritySnapshot(t.Context(), "HK.00700"); err == nil {
		t.Fatal("QuerySecuritySnapshot(OpenD failure) error = nil")
	}
}

func TestTradeAccountSelectionAndWriteBoundaryFailures(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	server.setAccounts([]*trdcommonpb.TrdAcc{testRealHKMarginAccount(), testSimulateHKCashAccount()})
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}

	resolved, err := exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{Market: "HK"})
	if err != nil || resolved.TradingEnvironment != "SIMULATE" || resolved.AccountID != "1001" {
		t.Fatalf("default account resolution = %#v, %v; want simulated 1001", resolved, err)
	}
	if _, err := exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{AccountID: "missing", Market: "HK"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing account resolution error = %v", err)
	}
	if _, err := exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{Market: "US"}); err == nil || !strings.Contains(err.Error(), "no trading account matched") {
		t.Fatalf("unavailable market resolution error = %v", err)
	}

	server.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:  new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:   new(uint64(3001)),
		AccType: new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	if _, err := exchange.resolveTradeAccountWithClient(t.Context(), client, BrokerReadQuery{Market: "EU"}); err == nil || !strings.Contains(err.Error(), "unsupported market") {
		t.Fatalf("unsupported market resolution error = %v", err)
	}

	server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
	server.setStaticInfos([]*qotcommonpb.SecurityStaticInfo{testTencentStaticInfo()})
	invalidQuantity := types.SubmitOrder{
		Symbol:   "HK.00700",
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeLimit,
		Price:    fixedpoint.NewFromInt(320),
		Quantity: fixedpoint.NewFromInt(10),
	}
	if _, err := exchange.PlaceBrokerOrder(t.Context(), BrokerPlaceOrderQuery{}, invalidQuantity); err == nil || !strings.Contains(err.Error(), "less than") {
		t.Fatalf("PlaceBrokerOrder(invalid quantity) error = %v", err)
	}

	invalidSide := invalidQuantity
	invalidSide.Quantity = fixedpoint.NewFromInt(100)
	invalidSide.Side = types.SideType("UNKNOWN")
	if _, err := exchange.PlaceBrokerOrder(t.Context(), BrokerPlaceOrderQuery{}, invalidSide); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("PlaceBrokerOrder(invalid side) error = %v", err)
	}
	if err := exchange.CancelBrokerOrders(t.Context(), BrokerReadQuery{}, types.Order{SubmitOrder: types.SubmitOrder{Symbol: "HK.00700"}}); err == nil || !strings.Contains(err.Error(), "requires broker order id") {
		t.Fatalf("CancelBrokerOrders(missing id) error = %v", err)
	}

	account := resolvedTradeAccount{Market: "HK"}
	for _, order := range []types.SubmitOrder{
		{Symbol: "BAD", Side: types.SideTypeBuy, Type: types.OrderTypeLimit, Quantity: fixedpoint.One},
		{Symbol: "HK.00700", Side: types.SideType("UNKNOWN"), Type: types.OrderTypeLimit, Quantity: fixedpoint.One},
		{Symbol: "HK.00700", Side: types.SideTypeBuy, Type: types.OrderType("UNKNOWN"), Quantity: fixedpoint.One},
	} {
		if _, err := placeOrderRequestFromSubmitOrder(account, order, BrokerPlaceOrderQuery{}); err == nil {
			t.Fatalf("placeOrderRequestFromSubmitOrder(%#v) error = nil", order)
		}
	}
}

func coverageMarginExchange(t *testing.T) (*quoteOpenDServer, *Exchange) {
	t.Helper()
	server := startQuoteOpenDServer(t)
	t.Cleanup(server.stop)
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })
	return server, exchange
}
