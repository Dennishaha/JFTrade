package servercoretest

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func TestExecutionOrderRoutesNormalizeUSPricePrecision(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"US"}, Type: "MARGIN",
	}})
	opendServer.SetPlacedOrderResponse(9001, "EXT-9001")
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

	request := opendServer.LastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
		return
	}
	if diff := math.Abs(request.Price - 10.12); diff > 1e-9 {
		t.Fatalf("price = %f, want 10.12", request.Price)
	}
	if got := request.Code; got != "TME" {
		t.Fatalf("Code = %q, want TME", got)
	}
	if got := request.Session; got != "RTH" {
		t.Fatalf("session = %q, want RTH", got)
	}
	if request.FillOutsideRTH == nil {
		t.Fatal("expected fillOutsideRTH to be set for US limit order")
	}
	if *request.FillOutsideRTH {
		t.Fatal("fillOutsideRTH = true, want false for default RTH session")
	}
}

func TestExecutionOrderRoutesPropagateUSSessionSelection(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"US"}, Type: "MARGIN",
	}})
	opendServer.SetPlacedOrderResponse(9001, "EXT-9001")
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

	request := opendServer.LastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
		return
	}
	if got := request.Session; got != "ETH" {
		t.Fatalf("session = %q, want ETH", got)
	}
	if request.FillOutsideRTH == nil {
		t.Fatal("expected fillOutsideRTH to be set for extended-hours limit order")
	}
	if !*request.FillOutsideRTH {
		t.Fatal("fillOutsideRTH = false, want true for ETH session")
	}
}

func TestExecutionOrderRoutesAcceptExplicitCodeWithMarket(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"US"}, Type: "MARGIN",
	}})
	opendServer.SetPlacedOrderResponse(9002, "EXT-9002")
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

	request := opendServer.LastPlaceOrderRequest()
	if request == nil {
		t.Fatal("expected place order request to be captured")
		return
	}
	if got := request.Code; got != "TME" {
		t.Fatalf("Code = %q, want TME", got)
	}
}

func TestExecutionOrderRoutesRejectBareSymbolWithoutMarket(t *testing.T) {
	opendServer := fututestkit.StartBrokerServer(t)
	opendServer.SetAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"US"}, Type: "MARGIN",
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
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/execution/previews", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST execution preview: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST execution preview status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if got := opendServer.PlaceOrderCallCount(); got != 0 {
		t.Fatalf("expected no place order call, got %d", got)
	}
}
