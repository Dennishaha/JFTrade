package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestExecutionOrdersEndpointFiltersByTradingEnvironmentAndScope(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
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
	})
	server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
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
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

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
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SaveExecutionSettings(ExecutionSettings{DefaultTradingEnvironment: "REAL"}); err != nil {
		t.Fatalf("saveExecutionSettings: %v", err)
	}
	server := newTestServer(t, store)
	server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
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
	})
	server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
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
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	defaultOrders := getExecutionOrdersForTest(t, srv.URL+"/api/v1/execution/orders")
	if len(defaultOrders.Orders) != 1 || defaultOrders.Orders[0].TradingEnvironment != "REAL" {
		t.Fatalf("default orders = %#v, want only REAL", defaultOrders.Orders)
	}
}

func TestExecutionOrderStorePromotesBrokerSourceToSystemOnPlacedMerge(t *testing.T) {
	store := newExecutionOrderStore()
	brokerOrderIDEx := "EXT-7001"
	store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "7001",
		BrokerOrderIDEx:    &brokerOrderIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		Quantity:           100,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")

	order := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "7001",
		BrokerOrderIDEx:    brokerOrderIDEx,
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	if order.Source != "system" || order.SourceDetail != "command.place" {
		t.Fatalf("source = %s/%s, want system/command.place", order.Source, order.SourceDetail)
	}
	if got := len(store.listOrders().Orders); got != 1 {
		t.Fatalf("orders = %d, want merged single order", got)
	}
}

func TestExecutionOrderStorePersistsOrdersEventsAndFillKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "execution-orders.db")
	store, err := newExecutionOrderStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("newExecutionOrderStoreWithDB: %v", err)
	}
	order := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "7001",
		BrokerOrderIDEx:    "EXT-7001",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	fillIDEx := "FILL-7001"
	store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "7001",
		BrokerOrderIDEx:    stringPointerOrNil("EXT-7001"),
		BrokerFillID:       "90001",
		BrokerFillIDEx:     &fillIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		FilledQuantity:     10,
		FilledAt:           "2026-05-20T10:00:00Z",
	})
	if err := store.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	reloaded, err := newExecutionOrderStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("reload execution store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, reloaded.Close()) }()

	reloadedOrder, ok := reloaded.order(order.InternalOrderID)
	if !ok {
		t.Fatalf("expected persisted order %s", order.InternalOrderID)
	}
	if reloadedOrder.Source != "system" || reloadedOrder.SourceDetail != "command.place" {
		t.Fatalf("source = %s/%s, want system/command.place", reloadedOrder.Source, reloadedOrder.SourceDetail)
	}
	events := reloaded.orderEvents(order.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("persisted events = %#v, want 2 events", events.Events)
	}
	fillKey := executionFillLookupKey("futu", "SIM-001", "SIMULATE", "HK", "90001", &fillIDEx)
	if _, ok := reloaded.seenFillKeys[fillKey]; !ok {
		t.Fatalf("expected persisted fill key %s", fillKey)
	}
}

func getExecutionOrdersForTest(t *testing.T, url string) executionOrdersResponse {
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
		OK   bool                    `json:"ok"`
		Data executionOrdersResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected execution orders ok=true")
	}
	return envelope.Data
}

func TestExecutionOrdersSyncBrokerOrdersAndTracksWorkerState(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	opendServer.setOrders([]*trdcommonpb.Order{{
		TrdSide:     new(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:     new(uint64(3001)),
		OrderIDEx:   new("EXT-3001"),
		Code:        new("HK.00700"),
		Name:        new("Tencent"),
		Qty:         new(float64(200)),
		Price:       new(321.1),
		FillQty:     new(float64(0)),
		CreateTime:  new("2026-05-20 09:30:00"),
		UpdateTime:  new("2026-05-20 09:31:00"),
		TimeInForce: new(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
		Currency:    new(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
	opendServer.setHistoryOrders([]*trdcommonpb.Order{{
		TrdSide:      new(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
		OrderType:    new(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus:  new(int32(trdcommonpb.OrderStatus_OrderStatus_Filled_All)),
		OrderID:      new(uint64(3002)),
		OrderIDEx:    new("EXT-3002"),
		Code:         new("HK.00700"),
		Name:         new("Tencent"),
		Qty:          new(float64(100)),
		Price:        new(322.2),
		FillQty:      new(float64(100)),
		FillAvgPrice: new(322.2),
		CreateTime:   new("2026-05-19 09:30:00"),
		UpdateTime:   new("2026-05-19 09:31:00"),
		TimeInForce:  new(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
		Currency:     new(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:    new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
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
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/execution/orders")
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	var ordersEnvelope struct {
		OK   bool                    `json:"ok"`
		Data executionOrdersResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ordersEnvelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if len(ordersEnvelope.Data.Orders) != 2 {
		t.Fatalf("expected two synced execution orders, got %#v", ordersEnvelope.Data.Orders)
	}
	var order executionOrderSummaryResponse
	var historyOrder executionOrderSummaryResponse
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
	if got := order.Status; got != "SUBMITTED" {
		t.Fatalf("status = %q, want SUBMITTED", got)
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
	if got := opendServer.subAccPushCallCount(); got != 1 {
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
	notifications := server.liveNotificationsAfter(0)
	if len(notifications) == 0 {
		t.Fatal("expected synced broker order to emit a live notification")
	}
	found := false
	for _, note := range notifications {
		if note.Title == "Futu 订单已提交" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Futu 订单已提交 notification, got %#v", notifications)
	}
}

func TestExecutionOrderStoreBrokerSyncUpdatesAndFillDeduplication(t *testing.T) {
	store := newExecutionOrderStore()
	orderIDEx := "EXT-9001"
	initialPrice := 100.0
	initialFilled := 10.0
	initialAverage := 100.0
	remark := "submitted by broker"
	summary, event, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		Quantity:           100,
		FilledQuantity:     &initialFilled,
		Price:              &initialPrice,
		FilledAveragePrice: &initialAverage,
		SubmittedAt:        "2026-05-20T01:30:00Z",
		UpdatedAt:          "2026-05-20T01:31:00Z",
		Remark:             &remark,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")
	if !changed || event == nil || event.EventType != "BROKER_SYNC_DISCOVERED" {
		t.Fatalf("initial sync summary=%+v event=%+v changed=%v", summary, event, changed)
	}
	if summary.BrokerID != "futu" || summary.Source != "broker" || summary.SourceDetail != "broker.current" {
		t.Fatalf("initial sync source = %+v", summary)
	}

	_, noEvent, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		Quantity:           100,
		FilledQuantity:     &initialFilled,
		Price:              &initialPrice,
		FilledAveragePrice: &initialAverage,
		SubmittedAt:        "2026-05-20T01:30:00Z",
		UpdatedAt:          "2026-05-20T01:31:00Z",
		Remark:             &remark,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")
	if changed || noEvent != nil {
		t.Fatalf("identical broker sync changed=%v event=%+v, want no-op", changed, noEvent)
	}

	updatedPrice := 101.0
	updatedFilled := 20.0
	updatedAverage := 100.5
	lastError := "partial fill warning"
	updated, updateEvent, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "FILLED_PART",
		Quantity:           100,
		FilledQuantity:     &updatedFilled,
		Price:              &updatedPrice,
		FilledAveragePrice: &updatedAverage,
		SubmittedAt:        "2026-05-20T01:30:00Z",
		UpdatedAt:          "2026-05-20T01:35:00Z",
		LastError:          &lastError,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")
	if !changed || updateEvent == nil || updateEvent.EventType != "BROKER_SYNC_UPDATED" {
		t.Fatalf("updated sync summary=%+v event=%+v changed=%v", updated, updateEvent, changed)
	}
	if updateEvent.PreviousStatus == nil || *updateEvent.PreviousStatus != "SUBMITTED" || updateEvent.NextStatus != "FILLED_PART" {
		t.Fatalf("update event status = %+v", updateEvent)
	}
	if updated.LastError == nil || *updated.LastError != lastError || updated.LastErrorSource == nil || *updated.LastErrorSource != "broker.sync" {
		t.Fatalf("updated last error = %+v", updated)
	}

	fillPrice := 101.0
	fillIDEx := "FILL-1"
	filled, fillEvent, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		BrokerFillID:       "FILL-1",
		BrokerFillIDEx:     &fillIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		FilledQuantity:     30,
		FillPrice:          &fillPrice,
		FilledAt:           "2026-05-20T01:36:00Z",
	})
	if !changed || fillEvent == nil || fillEvent.EventType != "BROKER_FILL_RECEIVED" {
		t.Fatalf("first fill summary=%+v event=%+v changed=%v", filled, fillEvent, changed)
	}
	if filled.FilledQuantity == nil || *filled.FilledQuantity != 50 {
		t.Fatalf("filled quantity after first fill = %#v, want 50", filled.FilledQuantity)
	}
	if filled.FilledAveragePrice == nil || *filled.FilledAveragePrice != 100.8 {
		t.Fatalf("filled average after first fill = %#v, want 100.8", filled.FilledAveragePrice)
	}
	if _, duplicateEvent, duplicateChanged := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		BrokerFillID:       "FILL-1",
		BrokerFillIDEx:     &fillIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		FilledQuantity:     30,
		FillPrice:          &fillPrice,
		FilledAt:           "2026-05-20T01:36:00Z",
	}); duplicateChanged || duplicateEvent != nil {
		t.Fatalf("duplicate fill changed=%v event=%+v, want no-op", duplicateChanged, duplicateEvent)
	}

	finalFillPrice := 102.0
	finalStatus, finalFillIDEx := "FILLED_ALL", "FILL-2"
	completed, finalEvent, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID:          "SIM-001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    &orderIDEx,
		BrokerFillID:       "FILL-2",
		BrokerFillIDEx:     &finalFillIDEx,
		Symbol:             "HK.00700",
		Side:               "BUY",
		FilledQuantity:     50,
		FillPrice:          &finalFillPrice,
		FilledAt:           "2026-05-20T01:40:00Z",
		Status:             &finalStatus,
	})
	if !changed || finalEvent == nil || completed.Status != "FILLED_ALL" {
		t.Fatalf("final fill summary=%+v event=%+v changed=%v", completed, finalEvent, changed)
	}
	if completed.FilledQuantity == nil || *completed.FilledQuantity != 100 {
		t.Fatalf("final filled quantity = %#v, want 100", completed.FilledQuantity)
	}
	if completed.LastError != nil || completed.LastErrorSource != nil {
		t.Fatalf("fills should clear broker sync errors, got %+v", completed)
	}
	events := store.orderEvents(completed.InternalOrderID)
	if len(events.Events) != 4 {
		t.Fatalf("events len = %d, want discovery/update/two fills: %+v", len(events.Events), events.Events)
	}
}

func TestExecutionOrderStorePlacedMergeCancelFilteringAndRetention(t *testing.T) {
	store := newExecutionOrderStore()
	price := 88.5
	first := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           " futu ",
		BrokerOrderIDEx:    "EXT-MERGE",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "",
		RequestedQuantity:  100,
		RequestedPrice:     &price,
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	if first.Status != "SUBMITTED" || first.BrokerID != "futu" || first.BrokerOrderIDEx == nil || *first.BrokerOrderIDEx != "EXT-MERGE" {
		t.Fatalf("initial placed order = %+v", first)
	}

	merged := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "MERGED-ORDER",
		BrokerOrderIDEx:    "EXT-MERGE",
		TradingEnvironment: "SIMULATE",
		AccountID:          "SIM-001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "SELL",
		OrderType:          "MARKET",
		RequestedQuantity:  50,
		Remark:             "resubmitted with broker id",
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})
	if merged.InternalOrderID != first.InternalOrderID {
		t.Fatalf("placed order with same brokerOrderIDEx created %s, want merge into %s", merged.InternalOrderID, first.InternalOrderID)
	}
	if merged.BrokerOrderID == nil || *merged.BrokerOrderID != "MERGED-ORDER" || merged.Symbol == nil || *merged.Symbol != "HK.00700" {
		t.Fatalf("merged identity fields = %+v", merged)
	}
	if merged.RequestedQuantity == nil || *merged.RequestedQuantity != 50 || merged.Remark == nil || *merged.Remark != "resubmitted with broker id" {
		t.Fatalf("merged order economics = %+v", merged)
	}

	if _, ok := store.markCancelRequested("missing-order", map[string]string{"reason": "user"}); ok {
		t.Fatal("cancel missing order returned ok")
	}
	cancelled, ok := store.markCancelRequested(first.InternalOrderID, map[string]string{"reason": "user"})
	if !ok || cancelled.Status != "CANCEL_REQUESTED" {
		t.Fatalf("cancelled order = %+v ok=%v", cancelled, ok)
	}
	events := store.orderEvents(first.InternalOrderID)
	if len(events.Events) != 3 || events.Events[2].EventType != "COMMAND_CANCEL_ACCEPTED" {
		t.Fatalf("events after cancel = %+v", events.Events)
	}

	filtered := store.listOrdersFiltered(executionOrderListFilter{
		BrokerID: "FUTU", TradingEnvironment: "simulate", AccountID: "SIM-001", Market: "hk",
	})
	if len(filtered.Orders) != 1 || filtered.Orders[0].InternalOrderID != first.InternalOrderID {
		t.Fatalf("case-insensitive filtered orders = %+v", filtered.Orders)
	}
	if mismatch := store.listOrdersFiltered(executionOrderListFilter{AccountID: "REAL-001"}); len(mismatch.Orders) != 0 {
		t.Fatalf("mismatched account filter returned %+v", mismatch.Orders)
	}

	cloned, ok := store.order(first.InternalOrderID)
	if !ok || cloned.Symbol == nil {
		t.Fatalf("order clone missing: %+v ok=%v", cloned, ok)
	}
	*cloned.Symbol = "MUTATED"
	reloaded, _ := store.order(first.InternalOrderID)
	if reloaded.Symbol == nil || *reloaded.Symbol == "MUTATED" {
		t.Fatalf("order() leaked mutable pointer, got %+v", reloaded)
	}

	old := time.Now().UTC().Add(-120 * 24 * time.Hour).Format(time.RFC3339Nano)
	recent := time.Now().UTC().Format(time.RFC3339Nano)
	store.seenFillKeys["old-fill"] = old
	store.seenFillKeys["recent-fill"] = recent
	store.seenFillKeys["invalid-fill"] = "not-a-time"
	store.configureSeenFillRetention(0)
	if store.seenFillRetentionDays != 90 {
		t.Fatalf("default retention days = %d, want 90", store.seenFillRetentionDays)
	}
	if _, ok := store.seenFillKeys["old-fill"]; ok {
		t.Fatal("old fill key was not pruned")
	}
	if _, ok := store.seenFillKeys["recent-fill"]; !ok {
		t.Fatal("recent fill key was unexpectedly pruned")
	}
	if _, ok := store.seenFillKeys["invalid-fill"]; !ok {
		t.Fatal("invalid fill timestamp should be preserved for manual inspection")
	}
	store.configureSeenFillRetention(5000)
	if store.seenFillRetentionDays != 3650 {
		t.Fatalf("max retention days = %d, want 3650", store.seenFillRetentionDays)
	}
}
