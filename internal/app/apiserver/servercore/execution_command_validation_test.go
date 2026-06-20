package servercore

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestExecutionOrderRoutesNormalizeUSPricePrecision(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]*trdcommonpb.TrdAcc{{
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newHTTPTestServer(t, store)

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
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/execution/orders", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution orders status = %d", resp.StatusCode)
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
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newHTTPTestServer(t, store)

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
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/execution/orders", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution orders status = %d", resp.StatusCode)
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
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}})
	opendServer.setPlacedOrderResponse(9002, "EXT-9002")
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newHTTPTestServer(t, store)

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
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/execution/orders", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution orders: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST execution orders status = %d", resp.StatusCode)
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
		TrdEnv:            new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:             new(uint64(1001)),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)},
		AccType:           new(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
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
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newHTTPTestServer(t, store)

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
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/execution/orders/preview", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST execution preview status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if got := opendServer.placeOrderCallCount(); got != 0 {
		t.Fatalf("expected no place order call, got %d", got)
	}
}
