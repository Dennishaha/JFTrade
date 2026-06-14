package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

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
	resp, err := http.Post(srv.URL+"/api/v1/execution/orders", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution orders: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution orders status = %d", resp.StatusCode)
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
