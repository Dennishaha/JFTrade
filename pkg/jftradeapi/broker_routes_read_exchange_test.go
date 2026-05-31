package jftradeapi

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestBrokerReadEndpointsReturnExchangeBackedData(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}, {
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             proto.Uint64(2001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setFunds(&trdcommonpb.Funds{
		Power:             proto.Float64(120000),
		TotalAssets:       proto.Float64(100000),
		Cash:              proto.Float64(40000),
		MarketVal:         proto.Float64(60000),
		FrozenCash:        proto.Float64(500),
		DebtCash:          proto.Float64(0),
		AvlWithdrawalCash: proto.Float64(39500),
		Currency:          proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             proto.Float64(40000),
			AvailableBalance: proto.Float64(39500),
			NetCashPower:     proto.Float64(120000),
		}},
		MarketInfoList: []*trdcommonpb.AccMarketInfo{{
			TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Assets:    proto.Float64(100000),
		}},
	})
	opendServer.setPositions([]*trdcommonpb.Position{{
		PositionID:       proto.Uint64(1),
		PositionSide:     proto.Int32(1),
		Code:             proto.String("HK.00700"),
		Name:             proto.String("Tencent"),
		Qty:              proto.Float64(200),
		CanSellQty:       proto.Float64(180),
		Price:            proto.Float64(320.5),
		CostPrice:        proto.Float64(300),
		AverageCostPrice: proto.Float64(301),
		Val:              proto.Float64(64100),
		PlVal:            proto.Float64(3900),
		PlRatio:          proto.Float64(13),
		TrdMarket:        proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Currency:         proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
	}})
	opendServer.setOrders([]*trdcommonpb.Order{{
		TrdSide:      proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:      proto.Uint64(2001),
		OrderIDEx:    proto.String("EXT-2001"),
		Code:         proto.String("HK.00700"),
		Name:         proto.String("Tencent"),
		Qty:          proto.Float64(100),
		Price:        proto.Float64(319.8),
		CreateTime:   proto.String("2026-05-20 09:30:00"),
		UpdateTime:   proto.String("2026-05-20 09:31:00"),
		FillQty:      proto.Float64(20),
		FillAvgPrice: proto.Float64(319.5),
		TimeInForce:  proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryOrders([]*trdcommonpb.Order{{
		TrdSide:      proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
		OrderID:      proto.Uint64(2101),
		OrderIDEx:    proto.String("EXT-2101"),
		Code:         proto.String("HK.00700"),
		Name:         proto.String("Tencent"),
		Qty:          proto.Float64(50),
		Price:        proto.Float64(321.2),
		CreateTime:   proto.String("2026-05-19 09:30:00"),
		UpdateTime:   proto.String("2026-05-19 09:45:00"),
		FillQty:      proto.Float64(50),
		FillAvgPrice: proto.Float64(321.1),
		TimeInForce:  proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFills([]*trdcommonpb.OrderFill{{
		OrderID:    proto.Uint64(2001),
		OrderIDEx:  proto.String("EXT-2001"),
		FillID:     proto.Uint64(3001),
		FillIDEx:   proto.String("FILL-3001"),
		Code:       proto.String("HK.00700"),
		Name:       proto.String("Tencent"),
		TrdSide:    proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        proto.Float64(20),
		Price:      proto.Float64(319.5),
		CreateTime: proto.String("2026-05-20 09:31:30"),
		Status:     proto.Int32(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryFills([]*trdcommonpb.OrderFill{{
		OrderID:    proto.Uint64(2101),
		OrderIDEx:  proto.String("EXT-2101"),
		FillID:     proto.Uint64(3101),
		FillIDEx:   proto.String("FILL-3101"),
		Code:       proto.String("HK.00700"),
		Name:       proto.String("Tencent"),
		TrdSide:    proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        proto.Float64(50),
		Price:      proto.Float64(321.1),
		CreateTime: proto.String("2026-05-19 09:40:00"),
		Status:     proto.Int32(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFees([]*trdcommonpb.OrderFee{{
		OrderIDEx: proto.String("EXT-2001"),
		FeeAmount: proto.Float64(12.5),
		FeeList:   []*trdcommonpb.OrderFeeItem{{Title: proto.String("Commission"), Value: proto.Float64(10.0)}},
	}})
	opendServer.setCashFlows([]*trdflowsummarypb.FlowSummaryInfo{{
		CashFlowID:        proto.Uint64(5001),
		ClearingDate:      proto.String("2026-05-20"),
		SettlementDate:    proto.String("2026-05-21"),
		Currency:          proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		CashFlowType:      proto.String("DIVIDEND"),
		CashFlowDirection: proto.Int32(int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)),
		CashFlowAmount:    proto.Float64(88.8),
		CashFlowRemark:    proto.String("cash-flow-test"),
	}})
	opendServer.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:       &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("00700")},
		IsLongPermit:   proto.Bool(true),
		IsShortPermit:  proto.Bool(false),
		ShortFeeRate:   proto.Float64(1.25),
		AlertLongRatio: proto.Float64(0.3),
	}, {
		Security:      &qotcommonpb.Security{Market: proto.Int32(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: proto.String("07226")},
		IsLongPermit:  proto.Bool(true),
		IsShortPermit: proto.Bool(true),
	}})
	opendServer.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          proto.Float64(1000),
		MaxCashAndMarginBuy: proto.Float64(2000),
		MaxPositionSell:     proto.Float64(500),
		MaxSellShort:        proto.Float64(300),
		MaxBuyBack:          proto.Float64(150),
		LongRequiredIM:      proto.Float64(10),
		ShortRequiredIM:     proto.Float64(12),
		Session:             proto.Int32(int32(commonpb.Session_Session_RTH)),
	})
	defer opendServer.stop()

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          strings.Split(opendServer.addr, ":")[0],
		APIPort:       portFromAddr(t, opendServer.addr),
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}

	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	query := "?tradingEnvironment=SIMULATE&accountId=1001&market=HK"
	realQuery := "?tradingEnvironment=REAL&accountId=2001&market=HK"

	funds := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/funds"+query)
	if got := funds["connectivity"]; got != "connected" {
		t.Fatalf("funds connectivity = %v, want connected", got)
	}
	summary, ok := funds["summary"].(map[string]any)
	if !ok {
		t.Fatalf("funds summary = %#v", funds["summary"])
	}
	if got := summary["accountId"]; got != "1001" {
		t.Fatalf("funds summary accountId = %v, want 1001", got)
	}
	if got := summary["currency"]; got != "HKD" {
		t.Fatalf("funds summary currency = %v, want HKD", got)
	}

	positions := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/positions"+query)
	entries, ok := positions["positions"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("positions entries = %#v", positions["positions"])
	}
	position, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("position entry = %#v", entries[0])
	}
	if got := position["symbol"]; got != "HK.00700" {
		t.Fatalf("position symbol = %v, want HK.00700", got)
	}

	orders := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/orders"+query)
	orderEntries, ok := orders["orders"].([]any)
	if !ok || len(orderEntries) != 1 {
		t.Fatalf("orders entries = %#v", orders["orders"])
	}
	order, ok := orderEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("order entry = %#v", orderEntries[0])
	}
	if got := order["brokerOrderId"]; got != "2001" {
		t.Fatalf("brokerOrderId = %v, want 2001", got)
	}
	if got := order["status"]; got != "SUBMITTED" {
		t.Fatalf("order status = %v, want SUBMITTED", got)
	}

	historyOrders := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/orders"+query+"&scope=history")
	historyOrderEntries, ok := historyOrders["orders"].([]any)
	if !ok || len(historyOrderEntries) != 1 {
		t.Fatalf("history orders entries = %#v", historyOrders["orders"])
	}

	fills := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/fills"+query)
	fillEntries, ok := fills["fills"].([]any)
	if !ok || len(fillEntries) != 1 {
		t.Fatalf("fills entries = %#v", fills["fills"])
	}
	fill, ok := fillEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("fill entry = %#v", fillEntries[0])
	}
	if got := fill["brokerFillId"]; got != "3001" {
		t.Fatalf("brokerFillId = %v, want 3001", got)
	}

	historyFills := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/fills"+query+"&scope=history")
	historyFillEntries, ok := historyFills["fills"].([]any)
	if !ok || len(historyFillEntries) != 1 {
		t.Fatalf("history fills entries = %#v", historyFills["fills"])
	}

	fees := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/order-fees"+query+"&orderIdEx=EXT-2001")
	feeEntries, ok := fees["fees"].([]any)
	if !ok || len(feeEntries) != 1 {
		t.Fatalf("fees entries = %#v", fees["fees"])
	}
	fee, ok := feeEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("fee entry = %#v", feeEntries[0])
	}
	if got := fee["brokerOrderIdEx"]; got != "EXT-2001" {
		t.Fatalf("fee brokerOrderIdEx = %v, want EXT-2001", got)
	}

	cashFlows := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/cash-flows"+query+"&clearingDate=2026-05-20&direction=IN")
	flowEntries, ok := cashFlows["cashFlows"].([]any)
	if !ok || len(flowEntries) != 1 {
		t.Fatalf("cashFlows entries = %#v", cashFlows["cashFlows"])
	}
	flow, ok := flowEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("cashFlow entry = %#v", flowEntries[0])
	}
	if got := flow["cashFlowType"]; got != "DIVIDEND" {
		t.Fatalf("cashFlowType = %v, want DIVIDEND", got)
	}

	marginRatios := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/margin-ratios"+realQuery+"&symbol=HK.00700")
	ratioEntries, ok := marginRatios["marginRatios"].([]any)
	if !ok || len(ratioEntries) != 1 {
		t.Fatalf("marginRatios entries = %#v", marginRatios["marginRatios"])
	}
	ratio, ok := ratioEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("margin ratio entry = %#v", ratioEntries[0])
	}
	if got := ratio["symbol"]; got != "HK.00700" {
		t.Fatalf("margin ratio symbol = %v, want HK.00700", got)
	}

	bareCodeMarginRatios := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/margin-ratios"+realQuery+"&symbol=07226")
	bareCodeEntries, ok := bareCodeMarginRatios["marginRatios"].([]any)
	if !ok || len(bareCodeEntries) != 1 {
		t.Fatalf("bare-code marginRatios entries = %#v", bareCodeMarginRatios["marginRatios"])
	}
	bareCodeRatio, ok := bareCodeEntries[0].(map[string]any)
	if !ok {
		t.Fatalf("bare-code margin ratio entry = %#v", bareCodeEntries[0])
	}
	if got := bareCodeRatio["symbol"]; got != "HK.07226" {
		t.Fatalf("bare-code margin ratio symbol = %v, want HK.07226", got)
	}

	maxTradeQtys := decodeBrokerEnvelope(t, srv.URL+"/api/v1/brokers/futu/max-trade-qtys"+query+"&symbol=HK.00700&orderType=LIMIT&price=320.5")
	maxTradeQuantity, ok := maxTradeQtys["maxTradeQuantity"].(map[string]any)
	if !ok {
		t.Fatalf("maxTradeQuantity = %#v", maxTradeQtys["maxTradeQuantity"])
	}
	if got := maxTradeQuantity["maxCashBuy"]; got != 1000.0 {
		t.Fatalf("maxCashBuy = %v, want 1000", got)
	}
	if got := maxTradeQuantity["orderType"]; got != "LIMIT" {
		t.Fatalf("orderType = %v, want LIMIT", got)
	}
}
