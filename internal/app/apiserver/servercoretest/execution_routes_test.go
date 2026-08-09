package servercoretest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	tradingstore "github.com/jftrade/jftrade-main/internal/store/trading"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func seedExecutionOrders(t *testing.T, store *servercore.SettingsStore, records ...trdsrv.ExecutionPlacedOrderRecord) {
	t.Helper()
	execStore, err := tradingstore.New(tradingstore.DerivePath(store.Path()))
	if err != nil {
		t.Fatalf("open execution order store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, execStore.Close()) }()
	for _, record := range records {
		execStore.RecordPlacedOrder(record)
	}
}

func getExecutionOrdersForTest(t *testing.T, url string) trdsrv.ExecutionOrders {
	t.Helper()
	resp, err := jftradeTestHTTPGet(t, url)
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET execution orders status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data trdsrv.ExecutionOrders `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected execution orders ok=true")
	}
	return envelope.Data
}

func TestExecutionOrdersEndpointFiltersByTradingEnvironmentAndScope(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	seedExecutionOrders(t, store,
		trdsrv.ExecutionPlacedOrderRecord{
			BrokerID:           "futu",
			BrokerOrderID:      "1001",
			TradingEnvironment: "SIMULATE",
			AccountID:          "SIM-001",
			Market:             "HK",
			Symbol:             "HK.00700",
			Side:               "BUY",
			OrderType:          "LIMIT",
			Status:             "SUBMITTED",
			RequestedQuantity:  100,
			EventType:          "COMMAND_PLACE_ACCEPTED",
		},
		trdsrv.ExecutionPlacedOrderRecord{
			BrokerID:           "futu",
			BrokerOrderID:      "2001",
			TradingEnvironment: "REAL",
			AccountID:          "REAL-001",
			Market:             "US",
			Symbol:             "US.AAPL",
			Side:               "BUY",
			OrderType:          "LIMIT",
			Status:             "SUBMITTED",
			RequestedQuantity:  1,
			EventType:          "COMMAND_PLACE_ACCEPTED",
		},
	)

	srv := newHTTPTestServer(t, store)

	defaultOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders")
	if len(defaultOrders.Orders) != 1 || defaultOrders.Orders[0].TradingEnvironment != "SIMULATE" {
		t.Fatalf("default orders = %#v, want only SIMULATE", defaultOrders.Orders)
	}

	realOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders?tradingEnvironment=REAL")
	if len(realOrders.Orders) != 1 || realOrders.Orders[0].TradingEnvironment != "REAL" {
		t.Fatalf("REAL orders = %#v, want only REAL", realOrders.Orders)
	}

	scopedOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders?brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US")
	if len(scopedOrders.Orders) != 1 || scopedOrders.Orders[0].AccountID != "REAL-001" || scopedOrders.Orders[0].Market != "US" {
		t.Fatalf("scoped orders = %#v, want REAL-001 US", scopedOrders.Orders)
	}

	mismatchedOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders?brokerId=futu&tradingEnvironment=REAL&accountId=SIM-001&market=HK")
	if len(mismatchedOrders.Orders) != 0 {
		t.Fatalf("mismatched orders = %#v, want empty", mismatchedOrders.Orders)
	}
}

func TestExecutionOrdersEndpointDefaultTradingEnvironmentFromSettings(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{DefaultTradingEnvironment: "REAL"}); err != nil {
		t.Fatalf("saveExecutionSettings: %v", err)
	}
	seedExecutionOrders(t, store,
		trdsrv.ExecutionPlacedOrderRecord{
			BrokerID:           "futu",
			BrokerOrderID:      "1001",
			TradingEnvironment: "SIMULATE",
			AccountID:          "SIM-001",
			Market:             "HK",
			Symbol:             "HK.00700",
			Side:               "BUY",
			OrderType:          "LIMIT",
			Status:             "SUBMITTED",
			RequestedQuantity:  100,
			EventType:          "COMMAND_PLACE_ACCEPTED",
		},
		trdsrv.ExecutionPlacedOrderRecord{
			BrokerID:           "futu",
			BrokerOrderID:      "2001",
			TradingEnvironment: "REAL",
			AccountID:          "REAL-001",
			Market:             "HK",
			Symbol:             "HK.00700",
			Side:               "BUY",
			OrderType:          "LIMIT",
			Status:             "SUBMITTED",
			RequestedQuantity:  100,
			EventType:          "COMMAND_PLACE_ACCEPTED",
		},
	)

	srv := newHTTPTestServer(t, store)

	defaultOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders")
	if len(defaultOrders.Orders) != 1 || defaultOrders.Orders[0].TradingEnvironment != "REAL" {
		t.Fatalf("default orders = %#v, want only REAL", defaultOrders.Orders)
	}
}

func TestExecutionOrdersSyncBrokerOrdersAndTracksWorkerState(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"HK"}, Type: "CASH",
	}})
	opendServer.SetOrders([]fututestkit.Order{{
		Side: "BUY", Type: "NORMAL", Status: "SUBMITTED", ID: 3001, ExternalID: "EXT-3001",
		Code: "HK.00700", Name: "Tencent", Quantity: 200, Price: 321.1,
		CreatedAt: "2026-05-20 09:30:00", UpdatedAt: "2026-05-20 09:31:00",
		TimeInForce: "DAY", Currency: "HKD", Market: "HK",
	}})
	opendServer.SetHistoryOrders([]fututestkit.Order{{
		Side: "SELL", Type: "NORMAL", Status: "FILLED", ID: 3002, ExternalID: "EXT-3002",
		Code: "HK.00700", Name: "Tencent", Quantity: 100, Price: 322.2,
		FilledQuantity: 100, AverageFillPrice: 322.2,
		CreatedAt: "2026-05-19 09:30:00", UpdatedAt: "2026-05-19 09:31:00",
		TimeInForce: "DAY", Currency: "HKD", Market: "HK",
	}})
	defer opendServer.Close()

	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: settingsfile.NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
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

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/execution/orders")
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	var ordersEnvelope struct {
		OK   bool                   `json:"ok"`
		Data trdsrv.ExecutionOrders `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ordersEnvelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if len(ordersEnvelope.Data.Orders) != 2 {
		t.Fatalf("expected two synced execution orders, got %#v", ordersEnvelope.Data.Orders)
	}
	var order trdsrv.ExecutionOrder
	var historyOrder trdsrv.ExecutionOrder
	for _, candidate := range ordersEnvelope.Data.Orders {
		if candidate.BrokerOrderID != nil && *candidate.BrokerOrderID == "3001" {
			order = candidate
		}
		if candidate.BrokerOrderID != nil && *candidate.BrokerOrderID == "3002" {
			historyOrder = candidate
		}
	}
	if order.BrokerOrderID == nil || *order.BrokerOrderID != "3001" {
		t.Fatalf("brokerOrderId = %#v, want 3001", order.BrokerOrderID)
	}
	if got := order.Status; got != "BROKER_ACCEPTED" {
		t.Fatalf("status = %q, want BROKER_ACCEPTED", got)
	}
	if order.Source != "broker" || order.SourceDetail != "broker.current" {
		t.Fatalf("current source = %s/%s, want broker/broker.current", order.Source, order.SourceDetail)
	}
	if historyOrder.BrokerOrderID == nil || *historyOrder.BrokerOrderID != "3002" {
		t.Fatalf("history brokerOrderId = %#v, want 3002", historyOrder.BrokerOrderID)
	}
	if historyOrder.Source != "broker" || historyOrder.SourceDetail != "broker.history" {
		t.Fatalf("history source = %s/%s, want broker/broker.history", historyOrder.Source, historyOrder.SourceDetail)
	}
	if got := opendServer.SubAccountPushCallCount(); got != 1 {
		t.Fatalf("expected one Trd_SubAccPush call, got %d", got)
	}

	workerResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/worker/broker-order-updates")
	if err != nil {
		t.Fatalf("GET worker status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, workerResp.Body.Close()) }()

	var workerEnvelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(workerResp.Body).Decode(&workerEnvelope); err != nil {
		t.Fatalf("decode worker status: %v", err)
	}
	subscriptions, ok := workerEnvelope.Data["subscriptions"].([]any)
	if !ok || len(subscriptions) == 0 {
		t.Fatalf("expected active subscriptions, got %#v", workerEnvelope.Data["subscriptions"])
	}
	brokers, ok := workerEnvelope.Data["brokers"].([]any)
	if !ok || len(brokers) != 1 {
		t.Fatalf("expected one broker worker summary, got %#v", workerEnvelope.Data["brokers"])
	}
}
