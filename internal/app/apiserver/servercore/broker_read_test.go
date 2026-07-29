package servercore

import (
	"path/filepath"
	"strings"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func seedBrokerRouteReadEndpointData(server *fututestkit.BrokerServer) {
	server.SetAccounts([]fututestkit.Account{
		{Environment: "SIMULATE", ID: 1001, Markets: []string{"HK"}, Type: "CASH"},
		{Environment: "REAL", ID: 2001, Markets: []string{"HK"}, Type: "MARGIN"},
	})
	server.SetFunds(fututestkit.Funds{
		Power: 120000, TotalAssets: 100000, Cash: 40000, MarketValue: 60000,
		FrozenCash: 500, AvailableToWithdraw: 39500, Currency: "HKD",
		CashEntries: []fututestkit.CashEntry{{
			Currency: "HKD", Cash: 40000, AvailableBalance: 39500, NetCashPower: 120000,
		}},
		MarketEntries: []fututestkit.MarketEntry{{Market: "HK", Assets: 100000}},
	})
	server.SetPositions([]fututestkit.Position{{
		ID: 1, Side: 1, Code: "HK.00700", Name: "Tencent", Quantity: 200,
		SellableQuantity: 180, Price: 320.5, CostPrice: 300, AverageCostPrice: 301,
		Value: 64100, ProfitLoss: 3900, ProfitLossRatio: 13, Market: "HK", Currency: "HKD",
	}})
	server.SetOrders([]fututestkit.Order{{
		Side: "BUY", Type: "NORMAL", Status: "SUBMITTED", ID: 2001, ExternalID: "EXT-2001",
		Code: "HK.00700", Name: "Tencent", Quantity: 100, Price: 319.8,
		CreatedAt: "2026-05-20 09:30:00", UpdatedAt: "2026-05-20 09:31:00",
		FilledQuantity: 20, AverageFillPrice: 319.5, TimeInForce: "GTC", Currency: "HKD", Market: "HK",
	}})
	server.SetHistoryOrders([]fututestkit.Order{{
		Side: "BUY", Type: "NORMAL", Status: "FILLED", ID: 2101, ExternalID: "EXT-2101",
		Code: "HK.00700", Name: "Tencent", Quantity: 50, Price: 321.2,
		CreatedAt: "2026-05-19 09:30:00", UpdatedAt: "2026-05-19 09:45:00",
		FilledQuantity: 50, AverageFillPrice: 321.1, TimeInForce: "GTC", Currency: "HKD", Market: "HK",
	}})
	server.SetOrderFills([]fututestkit.OrderFill{{
		OrderID: 2001, OrderIDEx: "EXT-2001", FillID: 3001, FillIDEx: "FILL-3001",
		Code: "HK.00700", Name: "Tencent", Side: "BUY", Quantity: 20, Price: 319.5,
		CreatedAt: "2026-05-20 09:31:30", Status: "OK", Market: "HK",
	}})
	server.SetHistoryFills([]fututestkit.OrderFill{{
		OrderID: 2101, OrderIDEx: "EXT-2101", FillID: 3101, FillIDEx: "FILL-3101",
		Code: "HK.00700", Name: "Tencent", Side: "BUY", Quantity: 50, Price: 321.1,
		CreatedAt: "2026-05-19 09:40:00", Status: "OK", Market: "HK",
	}})
	server.SetOrderFees([]fututestkit.OrderFee{{
		OrderIDEx: "EXT-2001", Amount: 12.5,
		Items: []fututestkit.FeeItem{{Title: "Commission", Value: 10}},
	}})
	server.SetCashFlows([]fututestkit.CashFlow{{
		ID: 5001, ClearingDate: "2026-05-20", SettlementDate: "2026-05-21", Currency: "HKD",
		Type: "DIVIDEND", Direction: "IN", Amount: 88.8, Remark: "cash-flow-test",
	}})
	server.SetMarginRatios([]fututestkit.MarginRatio{
		{Market: "HK", Code: "00700", LongPermitted: true, ShortFeeRate: 1.25, AlertLongRatio: 0.3},
		{Market: "HK", Code: "07226", LongPermitted: true, ShortPermitted: true},
	})
	server.SetMaxTradeQuantities(fututestkit.MaxTradeQuantities{
		CashBuy: 1000, CashAndMarginBuy: 2000, PositionSell: 500, SellShort: 300,
		BuyBack: 150, LongRequiredIM: 10, ShortRequiredIM: 12, Session: "RTH",
	})
}

func TestBrokerReadEndpointsReturnExchangeBackedData(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	seedBrokerRouteReadEndpointData(opendServer)
	defer opendServer.Close()

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type:          "futu",
		Host:          strings.Split(opendServer.Addr(), ":")[0],
		APIPort:       portFromAddr(t, opendServer.Addr()),
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
