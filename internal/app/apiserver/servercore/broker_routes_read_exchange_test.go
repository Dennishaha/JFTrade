package servercore

import (
	"path/filepath"
	"strings"
	"testing"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdflowsummarypb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdflowsummary"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

func TestBrokerReadEndpointsReturnExchangeBackedData(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}, {
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(2001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setFunds(&trdcommonpb.Funds{
		Power:             new(float64(120000)),
		TotalAssets:       new(float64(100000)),
		Cash:              new(float64(40000)),
		MarketVal:         new(float64(60000)),
		FrozenCash:        new(float64(500)),
		DebtCash:          new(float64(0)),
		AvlWithdrawalCash: new(float64(39500)),
		Currency:          new(int32(trdcommonpb.Currency_Currency_HKD)),
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         new(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             new(float64(40000)),
			AvailableBalance: new(float64(39500)),
			NetCashPower:     new(float64(120000)),
		}},
		MarketInfoList: []*trdcommonpb.AccMarketInfo{{
			TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
			Assets:    new(float64(100000)),
		}},
	})
	opendServer.setPositions([]*trdcommonpb.Position{{
		PositionID:       new(uint64(1)),
		PositionSide:     new(int32(1)),
		Code:             new("HK.00700"),
		Name:             new("Tencent"),
		Qty:              new(float64(200)),
		CanSellQty:       new(float64(180)),
		Price:            new(320.5),
		CostPrice:        new(float64(300)),
		AverageCostPrice: new(float64(301)),
		Val:              new(float64(64100)),
		PlVal:            new(float64(3900)),
		PlRatio:          new(float64(13)),
		TrdMarket:        new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		Currency:         new(int32(trdcommonpb.Currency_Currency_HKD)),
	}})
	opendServer.setOrders([]*trdcommonpb.Order{{
		TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:      new(uint64(2001)),
		OrderIDEx:    new("EXT-2001"),
		Code:         new("HK.00700"),
		Name:         new("Tencent"),
		Qty:          new(float64(100)),
		Price:        new(319.8),
		CreateTime:   new("2026-05-20 09:30:00"),
		UpdateTime:   new("2026-05-20 09:31:00"),
		FillQty:      new(float64(20)),
		FillAvgPrice: new(319.5),
		TimeInForce:  new(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     new(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryOrders([]*trdcommonpb.Order{{
		TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
		OrderID:      new(uint64(2101)),
		OrderIDEx:    new("EXT-2101"),
		Code:         new("HK.00700"),
		Name:         new("Tencent"),
		Qty:          new(float64(50)),
		Price:        new(321.2),
		CreateTime:   new("2026-05-19 09:30:00"),
		UpdateTime:   new("2026-05-19 09:45:00"),
		FillQty:      new(float64(50)),
		FillAvgPrice: new(321.1),
		TimeInForce:  new(int32(trdcommonpb.TimeInForce_TimeInForce_GTC)),
		Currency:     new(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFills([]*trdcommonpb.OrderFill{{
		OrderID:    new(uint64(2001)),
		OrderIDEx:  new("EXT-2001"),
		FillID:     new(uint64(3001)),
		FillIDEx:   new("FILL-3001"),
		Code:       new("HK.00700"),
		Name:       new("Tencent"),
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        new(float64(20)),
		Price:      new(319.5),
		CreateTime: new("2026-05-20 09:31:30"),
		Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryFills([]*trdcommonpb.OrderFill{{
		OrderID:    new(uint64(2101)),
		OrderIDEx:  new("EXT-2101"),
		FillID:     new(uint64(3101)),
		FillIDEx:   new("FILL-3101"),
		Code:       new("HK.00700"),
		Name:       new("Tencent"),
		TrdSide:    new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		Qty:        new(float64(50)),
		Price:      new(321.1),
		CreateTime: new("2026-05-19 09:40:00"),
		Status:     new(int32(trdcommonpb.OrderFillStatus_OrderFillStatus_OK)),
		TrdMarket:  new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setOrderFees([]*trdcommonpb.OrderFee{{
		OrderIDEx: new("EXT-2001"),
		FeeAmount: new(12.5),
		FeeList:   []*trdcommonpb.OrderFeeItem{{Title: new("Commission"), Value: new(10.0)}},
	}})
	opendServer.setCashFlows([]*trdflowsummarypb.FlowSummaryInfo{{
		CashFlowID:        new(uint64(5001)),
		ClearingDate:      new("2026-05-20"),
		SettlementDate:    new("2026-05-21"),
		Currency:          new(int32(trdcommonpb.Currency_Currency_HKD)),
		CashFlowType:      new("DIVIDEND"),
		CashFlowDirection: new(int32(trdflowsummarypb.TrdCashFlowDirection_TrdCashFlowDirection_In)),
		CashFlowAmount:    new(88.8),
		CashFlowRemark:    new("cash-flow-test"),
	}})
	opendServer.setMarginRatios([]*trdgetmarginratiopb.MarginRatioInfo{{
		Security:       &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
		IsLongPermit:   new(true),
		IsShortPermit:  new(false),
		ShortFeeRate:   new(1.25),
		AlertLongRatio: new(0.3),
	}, {
		Security:      &qotcommonpb.Security{Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("07226")},
		IsLongPermit:  new(true),
		IsShortPermit: new(true),
	}})
	opendServer.setMaxTrdQtys(&trdcommonpb.MaxTrdQtys{
		MaxCashBuy:          new(float64(1000)),
		MaxCashAndMarginBuy: new(float64(2000)),
		MaxPositionSell:     new(float64(500)),
		MaxSellShort:        new(float64(300)),
		MaxBuyBack:          new(float64(150)),
		LongRequiredIM:      new(float64(10)),
		ShortRequiredIM:     new(float64(12)),
		Session:             new(int32(commonpb.Session_Session_RTH)),
	})
	defer opendServer.stop()

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          strings.Split(opendServer.addr, ":")[0],
		APIPort:       portFromAddr(t, opendServer.addr),
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}

	srv := newHTTPTestServer(t, store)

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
