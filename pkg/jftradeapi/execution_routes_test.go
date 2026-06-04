package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

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
	_, err = store.saveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
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
