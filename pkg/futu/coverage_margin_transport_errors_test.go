package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestTradeReadMethodsPropagateTargetProtocolDisconnects(t *testing.T) {
	cases := []struct {
		name    string
		protoID uint32
		invoke  func(*Exchange) error
	}{
		{"accounts", opend.ProtoTrdGetAccList, func(exchange *Exchange) error { _, err := exchange.DiscoverAccounts(t.Context()); return err }},
		{"funds", opend.ProtoTrdGetFunds, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerFunds(t.Context(), BrokerReadQuery{})
			return err
		}},
		{"positions", opend.ProtoTrdGetPositionList, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerPositions(t.Context(), BrokerReadQuery{})
			return err
		}},
		{"orders", opend.ProtoTrdGetOrderList, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerOrders(t.Context(), BrokerReadQuery{}, "")
			return err
		}},
		{"history orders", opend.ProtoTrdGetHistoryOrderList, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerHistoryOrders(t.Context(), BrokerOrderHistoryQuery{})
			return err
		}},
		{"history fills", opend.ProtoTrdGetHistoryOrderFillList, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerHistoryOrderFills(t.Context(), BrokerOrderFillHistoryQuery{})
			return err
		}},
		{"fills", opend.ProtoTrdGetOrderFillList, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerOrderFills(t.Context(), BrokerOrderFillQuery{})
			return err
		}},
		{"fees", opend.ProtoTrdGetOrderFee, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerOrderFees(t.Context(), BrokerOrderFeeQuery{})
			return err
		}},
		{"cash flows", opend.ProtoTrdFlowSummary, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerCashFlows(t.Context(), BrokerCashFlowQuery{})
			return err
		}},
		{"max quantity", opend.ProtoTrdGetMaxTrdQtys, func(exchange *Exchange) error {
			_, err := exchange.QueryBrokerMaxTradeQuantity(t.Context(), BrokerMaxTradeQuantityQuery{Symbol: "HK.00700", OrderType: "LIMIT", Price: 300})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, exchange := coverageMarginExchange(t)
			server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
			server.setDropProto(tc.protoID)
			if err := tc.invoke(exchange); err == nil {
				t.Fatalf("protocol %d disconnect error = nil", tc.protoID)
			}
		})
	}
}

func TestQuoteKLineAndOrderBookPropagateTargetDisconnects(t *testing.T) {
	t.Run("basic quote", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		if err := exchange.SubscribeBasicQuote(t.Context(), "HK.00700", false); err != nil {
			t.Fatalf("SubscribeBasicQuote() error = %v", err)
		}
		server.setDropProto(opend.ProtoGetBasicQot)
		if _, err := exchange.QueryTicker(t.Context(), "HK.00700"); err == nil {
			t.Fatal("basic quote disconnect error = nil")
		}
	})
	t.Run("history kline", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
			t.Fatalf("SubscribeKLine() error = %v", err)
		}
		server.setDropProto(opend.ProtoRequestHistoryKL)
		if _, err := exchange.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{}); err == nil {
			t.Fatal("history K-line disconnect error = nil")
		}
	})
	t.Run("current kline", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
			t.Fatalf("SubscribeKLine() error = %v", err)
		}
		server.setDropProto(opend.ProtoGetKL)
		security, canonical, _ := futuSecurityFromSymbol("HK.00700")
		if _, err := exchange.queryCurrentKLines(t.Context(), security, canonical, types.Interval5m, qotcommonpb.KLType_KLType_5Min); err == nil {
			t.Fatal("current K-line disconnect error = nil")
		}
	})
	t.Run("order book subscribe", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setDropProto(opend.ProtoQotSub)
		if _, err := exchange.QueryOrderBook(t.Context(), "HK.00700", 10); err == nil {
			t.Fatal("order-book subscribe disconnect error = nil")
		}
	})
	t.Run("order book query", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		client, err := exchange.ensureClient(t.Context())
		if err != nil {
			t.Fatalf("ensureClient() error = %v", err)
		}
		security, canonical, _ := futuSecurityFromSymbol("HK.00700")
		if err := exchange.ensureOrderBookSubscriptions(t.Context(), client, []orderBookRequest{{canonical: canonical, security: security}}); err != nil {
			t.Fatalf("ensureOrderBookSubscriptions() error = %v", err)
		}
		server.setDropProto(opend.ProtoGetOrderBook)
		if _, err := exchange.QueryOrderBook(t.Context(), "HK.00700", 10); err == nil {
			t.Fatal("order-book query disconnect error = nil")
		}
	})
}

func TestTradeWriteMethodsPropagateAccountAndWriteDisconnects(t *testing.T) {
	order := types.SubmitOrder{Symbol: "HK.00700", Side: types.SideTypeBuy, Type: types.OrderTypeLimit, Price: fixedpoint.NewFromInt(300), Quantity: fixedpoint.NewFromInt(100)}
	t.Run("place account", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
		server.setDropProto(opend.ProtoTrdGetAccList)
		if _, err := exchange.PlaceBrokerOrder(t.Context(), BrokerPlaceOrderQuery{}, order); err == nil {
			t.Fatal("place account disconnect error = nil")
		}
	})
	t.Run("place write", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
		server.setDropProto(opend.ProtoTrdPlaceOrder)
		if _, err := exchange.submitOrder(t.Context(), order); err == nil {
			t.Fatal("place write disconnect error = nil")
		}
	})
	t.Run("cancel account", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
		server.setDropProto(opend.ProtoTrdGetAccList)
		if err := exchange.CancelBrokerOrders(t.Context(), BrokerReadQuery{}, types.Order{SubmitOrder: order, OrderID: 1}); err == nil {
			t.Fatal("cancel account disconnect error = nil")
		}
	})
	t.Run("cancel write", func(t *testing.T) {
		server, exchange := coverageMarginExchange(t)
		server.setAccounts([]*trdcommonpb.TrdAcc{testSimulateHKCashAccount()})
		server.setDropProto(opend.ProtoTrdModifyOrder)
		if err := exchange.CancelBrokerOrders(t.Context(), BrokerReadQuery{}, types.Order{SubmitOrder: order, OrderID: 1}); err == nil {
			t.Fatal("cancel write disconnect error = nil")
		}
	})
	marketOnly := order
	marketOnly.Market = types.Market{Symbol: "HK.00700"}
	if got := placedOrderFromSubmitOrder(marketOnly, 1); got.Market.Exchange != Name {
		t.Fatalf("placed order exchange fallback = %#v", got.Market)
	}
}

func TestDirectSubscriptionCallsPropagateClosedClientErrors(t *testing.T) {
	client := opend.New(opend.Config{})
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	security, canonical, _ := futuSecurityFromSymbol("HK.00700")
	exchange := NewExchange("")
	if err := setBasicQotSubscription(t.Context(), client, []*qotcommonpb.Security{security}, true, nil); err == nil {
		t.Fatal("closed-client basic subscription error = nil")
	}
	request := klineSubscriptionRequest{canonical: canonical, security: security, subType: qotcommonpb.SubType_SubType_KL_5Min}
	if err := setKLineSubscription(t.Context(), client, request, true); err == nil {
		t.Fatal("closed-client K-line subscription error = nil")
	}
	orderBookReq := orderBookRequest{canonical: canonical, security: security}
	if err := exchange.ensureOrderBookSubscriptions(t.Context(), client, []orderBookRequest{orderBookReq}); err == nil {
		t.Fatal("closed-client order-book subscription error = nil")
	}
	if err := exchange.ensureOrderBookPushSubscriptions(t.Context(), client, []orderBookRequest{orderBookReq}); err == nil {
		t.Fatal("closed-client order-book push subscription error = nil")
	}
	if err := exchange.ensureOrderBookSubscriptions(t.Context(), client, nil); err != nil {
		t.Fatalf("empty order-book subscription error = %v", err)
	}
	if err := exchange.ensureOrderBookPushSubscriptions(t.Context(), client, nil); err != nil {
		t.Fatalf("empty order-book push subscription error = %v", err)
	}
}
