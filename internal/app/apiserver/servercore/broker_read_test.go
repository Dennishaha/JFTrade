package servercore

import (
	"path/filepath"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestBrokerReadEndpointsReturnExchangeBackedData(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.seedReadEndpointData()
	defer opendServer.stop()

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
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
