package jftradeapi

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestExecutionOrderRoutesPlaceListEventsAndCancel(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	opendServer.setPlacedOrderResponse(9001, "EXT-9001")
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

	payload, err := json.Marshal(map[string]any{
		"market":      "HK",
		"symbol":      "00700",
		"side":        "BUY",
		"orderType":   "LIMIT",
		"timeInForce": "DAY",
		"quantity":    100,
		"price":       320.5,
		"env":         "SIMULATE",
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution preview status = %d", resp.StatusCode)
	}

	var commandEnvelope struct {
		OK   bool                       `json:"ok"`
		Data brokerOrderCommandResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commandEnvelope); err != nil {
		t.Fatalf("decode execution command: %v", err)
	}
	if !commandEnvelope.OK {
		t.Fatal("expected command ok=true")
	}
	if !commandEnvelope.Data.Accepted {
		t.Fatal("expected accepted=true")
	}
	if commandEnvelope.Data.InternalOrderID == nil || *commandEnvelope.Data.InternalOrderID == "" {
		t.Fatal("expected internalOrderId in command response")
	}
	if commandEnvelope.Data.BrokerOrderID == nil || *commandEnvelope.Data.BrokerOrderID != "9001" {
		t.Fatalf("brokerOrderId = %#v, want 9001", commandEnvelope.Data.BrokerOrderID)
	}

	listResp, err := http.Get(srv.URL + "/api/v1/execution/orders")
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET execution orders status = %d", listResp.StatusCode)
	}

	var ordersEnvelope struct {
		OK   bool                    `json:"ok"`
		Data executionOrdersResponse `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&ordersEnvelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if !ordersEnvelope.OK {
		t.Fatal("expected orders ok=true")
	}
	if len(ordersEnvelope.Data.Orders) != 1 {
		t.Fatalf("expected one execution order, got %#v", ordersEnvelope.Data.Orders)
	}
	order := ordersEnvelope.Data.Orders[0]
	if got := order.InternalOrderID; got != *commandEnvelope.Data.InternalOrderID {
		t.Fatalf("internalOrderId = %q, want %q", got, *commandEnvelope.Data.InternalOrderID)
	}
	if order.Symbol == nil || *order.Symbol != "HK.00700" {
		t.Fatalf("symbol = %#v, want HK.00700", order.Symbol)
	}
	if got := order.Status; got != "SUBMITTED" {
		t.Fatalf("status = %q, want SUBMITTED", got)
	}
	if got := opendServer.placeOrderCallCount(); got != 1 {
		t.Fatalf("expected one place order call, got %d", got)
	}

	eventsResp, err := http.Get(srv.URL + "/api/v1/execution/orders/" + *commandEnvelope.Data.InternalOrderID + "/events")
	if err != nil {
		t.Fatalf("GET execution order events: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET execution order events status = %d", eventsResp.StatusCode)
	}

	var eventsEnvelope struct {
		OK   bool                         `json:"ok"`
		Data executionOrderEventsResponse `json:"data"`
	}
	if err := json.NewDecoder(eventsResp.Body).Decode(&eventsEnvelope); err != nil {
		t.Fatalf("decode execution order events: %v", err)
	}
	if !eventsEnvelope.OK {
		t.Fatal("expected events ok=true")
	}
	if len(eventsEnvelope.Data.Events) != 1 {
		t.Fatalf("expected one execution event, got %#v", eventsEnvelope.Data.Events)
	}
	if got := eventsEnvelope.Data.Events[0].EventType; got != "COMMAND_PLACE_ACCEPTED" {
		t.Fatalf("eventType = %q, want COMMAND_PLACE_ACCEPTED", got)
	}

	cancelReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/execution/orders/"+*commandEnvelope.Data.InternalOrderID+"/cancel", nil)
	if err != nil {
		t.Fatalf("NewRequest cancel: %v", err)
	}
	cancelResp, err := http.DefaultClient.Do(cancelReq)
	if err != nil {
		t.Fatalf("POST execution cancel: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution cancel status = %d", cancelResp.StatusCode)
	}

	var cancelEnvelope struct {
		OK   bool                       `json:"ok"`
		Data brokerOrderCommandResponse `json:"data"`
	}
	if err := json.NewDecoder(cancelResp.Body).Decode(&cancelEnvelope); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if !cancelEnvelope.OK || !cancelEnvelope.Data.Accepted {
		t.Fatalf("expected cancel accepted, got %#v", cancelEnvelope)
	}
	if cancelEnvelope.Data.OrderStatus == nil || *cancelEnvelope.Data.OrderStatus != "CANCEL_REQUESTED" {
		t.Fatalf("cancel order status = %#v, want CANCEL_REQUESTED", cancelEnvelope.Data.OrderStatus)
	}
	if got := opendServer.modifyOrderCallCount(); got != 1 {
		t.Fatalf("expected one modify order call, got %d", got)
	}

	updatedEventsResp, err := http.Get(srv.URL + "/api/v1/execution/orders/" + *commandEnvelope.Data.InternalOrderID + "/events")
	if err != nil {
		t.Fatalf("GET updated execution order events: %v", err)
	}
	defer updatedEventsResp.Body.Close()

	var updatedEventsEnvelope struct {
		OK   bool                         `json:"ok"`
		Data executionOrderEventsResponse `json:"data"`
	}
	if err := json.NewDecoder(updatedEventsResp.Body).Decode(&updatedEventsEnvelope); err != nil {
		t.Fatalf("decode updated execution events: %v", err)
	}
	if len(updatedEventsEnvelope.Data.Events) != 2 {
		t.Fatalf("expected two execution events after cancel, got %#v", updatedEventsEnvelope.Data.Events)
	}
	if got := updatedEventsEnvelope.Data.Events[1].EventType; got != "COMMAND_CANCEL_ACCEPTED" {
		t.Fatalf("second eventType = %q, want COMMAND_CANCEL_ACCEPTED", got)
	}
}

func TestExecutionOrderRoutesNormalizeUSPricePrecision(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setPlacedOrderResponse(9001, "EXT-9001")
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload, err := json.Marshal(map[string]any{
		"brokerId":           "futu",
		"market":             "US",
		"symbol":             "TME",
		"side":               "BUY",
		"orderType":          "LIMIT",
		"timeInForce":        "DAY",
		"quantity":           100,
		"price":              10.123,
		"accountId":          "1001",
		"tradingEnvironment": "SIMULATE",
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution preview status = %d", resp.StatusCode)
	}

	request := opendServer.lastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
	}
	if diff := math.Abs(request.GetPrice() - 10.12); diff > 1e-9 {
		t.Fatalf("price = %f, want 10.12", request.GetPrice())
	}
	if got := request.GetCode(); got != "TME" {
		t.Fatalf("Code = %q, want TME", got)
	}
	if got := request.GetSession(); got != int32(commonpb.Session_Session_RTH) {
		t.Fatalf("session = %d, want RTH", got)
	}
	if request.FillOutsideRTH == nil {
		t.Fatal("expected fillOutsideRTH to be set for US limit order")
	}
	if request.GetFillOutsideRTH() {
		t.Fatal("fillOutsideRTH = true, want false for default RTH session")
	}
}

func TestExecutionOrderRoutesPropagateUSSessionSelection(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setPlacedOrderResponse(9001, "EXT-9001")
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload, err := json.Marshal(map[string]any{
		"brokerId":           "futu",
		"market":             "US",
		"symbol":             "TME",
		"side":               "BUY",
		"orderType":          "LIMIT",
		"timeInForce":        "DAY",
		"session":            "ETH",
		"quantity":           100,
		"price":              10.12,
		"accountId":          "1001",
		"tradingEnvironment": "SIMULATE",
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution preview status = %d", resp.StatusCode)
	}

	request := opendServer.lastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
	}
	if got := request.GetSession(); got != int32(commonpb.Session_Session_ETH) {
		t.Fatalf("session = %d, want ETH", got)
	}
	if request.FillOutsideRTH == nil {
		t.Fatal("expected fillOutsideRTH to be set for extended-hours limit order")
	}
	if !request.GetFillOutsideRTH() {
		t.Fatal("fillOutsideRTH = false, want true for ETH session")
	}
}

func TestExecutionOrderRoutesAcceptExplicitCodeWithMarket(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setPlacedOrderResponse(9002, "EXT-9002")
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload, err := json.Marshal(map[string]any{
		"brokerId":           "futu",
		"market":             "US",
		"code":               "TME",
		"side":               "BUY",
		"orderType":          "LIMIT",
		"timeInForce":        "DAY",
		"quantity":           100,
		"price":              10.12,
		"accountId":          "1001",
		"tradingEnvironment": "SIMULATE",
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution preview status = %d", resp.StatusCode)
	}

	request := opendServer.lastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
	}
	if got := request.GetCode(); got != "TME" {
		t.Fatalf("Code = %q, want TME", got)
	}
}

func TestExecutionOrderRoutesRejectBareSymbolWithoutMarket(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload, err := json.Marshal(map[string]any{
		"brokerId":           "futu",
		"symbol":             "TME",
		"side":               "BUY",
		"orderType":          "LIMIT",
		"timeInForce":        "DAY",
		"quantity":           100,
		"price":              10.12,
		"accountId":          "1001",
		"tradingEnvironment": "SIMULATE",
	})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST execution preview status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if got := opendServer.placeOrderCallCount(); got != 0 {
		t.Fatalf("expected no place order call, got %d", got)
	}
}

func TestExecutionOrdersSyncBrokerOrdersAndTracksWorkerState(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             proto.Uint64(1001),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
		AccType:           proto.Int32(int32(trdcommonpb.TrdAccType_TrdAccType_Cash)),
	}})
	opendServer.setOrders([]*trdcommonpb.Order{{
		TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:     proto.Uint64(3001),
		OrderIDEx:   proto.String("EXT-3001"),
		Code:        proto.String("HK.00700"),
		Name:        proto.String("Tencent"),
		Qty:         proto.Float64(200),
		Price:       proto.Float64(321.1),
		FillQty:     proto.Float64(0),
		CreateTime:  proto.String("2026-05-20 09:30:00"),
		UpdateTime:  proto.String("2026-05-20 09:31:00"),
		TimeInForce: proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
		Currency:    proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
		TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}})
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
	server := NewServer(store)
	srv := httptest.NewServer(server)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/execution/orders")
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer resp.Body.Close()

	var ordersEnvelope struct {
		OK   bool                    `json:"ok"`
		Data executionOrdersResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ordersEnvelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if len(ordersEnvelope.Data.Orders) != 1 {
		t.Fatalf("expected one synced execution order, got %#v", ordersEnvelope.Data.Orders)
	}
	order := ordersEnvelope.Data.Orders[0]
	if order.BrokerOrderID == nil || *order.BrokerOrderID != "3001" {
		t.Fatalf("brokerOrderId = %#v, want 3001", order.BrokerOrderID)
	}
	if got := order.Status; got != "SUBMITTED" {
		t.Fatalf("status = %q, want SUBMITTED", got)
	}
	if got := opendServer.subAccPushCallCount(); got != 1 {
		t.Fatalf("expected one Trd_SubAccPush call, got %d", got)
	}

	workerResp, err := http.Get(srv.URL + "/api/v1/system/worker/broker-order-updates")
	if err != nil {
		t.Fatalf("GET worker status: %v", err)
	}
	defer workerResp.Body.Close()

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

func TestExecutionPushHandlersWriteBackAndNotify(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	price := 320.5
	placed := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    "EXT-9001",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		RequestedPrice:     &price,
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})

	server.handleFutuBrokerOrderFillPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.OrderFill{
		TrdSide:         proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		FillID:          proto.Uint64(1),
		OrderID:         proto.Uint64(9001),
		OrderIDEx:       proto.String("EXT-9001"),
		Code:            proto.String("HK.00700"),
		Name:            proto.String("Tencent"),
		Qty:             proto.Float64(100),
		Price:           proto.Float64(320.5),
		CreateTime:      proto.String("2026-05-20 09:32:00"),
		CreateTimestamp: proto.Float64(float64(time.Date(2026, time.May, 20, 9, 32, 0, 0, time.UTC).Unix())),
		TrdMarket:       proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	})

	filledOrder, ok := server.executionOrders.order(placed.InternalOrderID)
	if !ok {
		t.Fatal("expected placed order to remain in execution store")
	}
	if got := filledOrder.Status; got != "FILLED_ALL" {
		t.Fatalf("filled order status = %q, want FILLED_ALL", got)
	}
	events := server.executionOrders.orderEvents(placed.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("expected place and fill events, got %#v", events.Events)
	}
	fillNotificationFound := false
	for _, note := range server.liveNotificationsAfter(0) {
		if note.Title == "Futu 成交成功" {
			fillNotificationFound = true
			break
		}
	}
	if !fillNotificationFound {
		t.Fatalf("expected Futu 成交成功 notification, got %#v", server.liveNotificationsAfter(0))
	}

	placedCancel := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9002",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "SELL",
		OrderType:          "LIMIT",
		Status:             "CANCEL_REQUESTED",
		RequestedQuantity:  50,
		RequestedPrice:     &price,
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_CANCEL_ACCEPTED",
	})

	server.handleFutuBrokerOrderPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.Order{
		TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
		OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
		OrderID:     proto.Uint64(9002),
		Code:        proto.String("HK.00700"),
		Qty:         proto.Float64(50),
		Price:       proto.Float64(320.5),
		CreateTime:  proto.String("2026-05-20 09:33:00"),
		UpdateTime:  proto.String("2026-05-20 09:34:00"),
		TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		TimeInForce: proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
		Currency:    proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
	})

	cancelledOrder, ok := server.executionOrders.order(placedCancel.InternalOrderID)
	if !ok {
		t.Fatal("expected cancelled order to remain in execution store")
	}
	if got := cancelledOrder.Status; got != "CANCELLED_ALL" {
		t.Fatalf("cancelled order status = %q, want CANCELLED_ALL", got)
	}
	cancelNotificationFound := false
	for _, note := range server.liveNotificationsAfter(0) {
		if note.Title == "Futu 撤单成功" {
			cancelNotificationFound = true
			break
		}
	}
	if !cancelNotificationFound {
		t.Fatalf("expected Futu 撤单成功 notification, got %#v", server.liveNotificationsAfter(0))
	}
}

func TestRecordPlacedOrderReusesExistingBrokerDiscoveredOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)

	server.handleFutuBrokerOrderPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.Order{
		TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:     proto.Uint64(9001),
		OrderIDEx:   proto.String("EXT-9001"),
		Code:        proto.String("HK.00700"),
		Qty:         proto.Float64(100),
		Price:       proto.Float64(320.5),
		CreateTime:  proto.String("2026-05-20 09:33:00"),
		UpdateTime:  proto.String("2026-05-20 09:33:00"),
		TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	})

	ordersBefore := server.executionOrders.listOrders()
	if len(ordersBefore.Orders) != 1 {
		t.Fatalf("expected one discovered order, got %#v", ordersBefore.Orders)
	}
	discovered := ordersBefore.Orders[0]
	price := 320.5

	placed := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    "EXT-9001",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		RequestedPrice:     &price,
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})

	if placed.InternalOrderID != discovered.InternalOrderID {
		t.Fatalf("internalOrderId = %q, want %q", placed.InternalOrderID, discovered.InternalOrderID)
	}
	ordersAfter := server.executionOrders.listOrders()
	if len(ordersAfter.Orders) != 1 {
		t.Fatalf("expected one order after command acceptance, got %#v", ordersAfter.Orders)
	}
	events := server.executionOrders.orderEvents(placed.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("expected push + command events, got %#v", events.Events)
	}
	if got := events.Events[1].EventType; got != "COMMAND_PLACE_ACCEPTED" {
		t.Fatalf("second event type = %q, want COMMAND_PLACE_ACCEPTED", got)
	}
}
