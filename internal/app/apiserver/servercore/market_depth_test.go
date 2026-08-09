package servercore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	livecore "github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func marketDataDepthOrderBookFixture(price float64, volume int64, orderCount int32) fututestkit.OrderBookEntry {
	return fututestkit.OrderBookEntry{Price: price, Volume: volume, OrderCount: orderCount}
}

func acquireTestDepthSubscription(t *testing.T, server *Server, market, symbol string) {
	t.Helper()
	server.marketdataSvc.SetSubscriptionReconciler(server.runtimes.MarketData())
	if _, err := server.marketdataSvc.AcquireSubscription(t.Context(), "test-depth", []mdsrv.InstrumentRef{{
		Channel: "ORDER_BOOK",
		Market:  market,
		Symbol:  symbol,
	}}); err != nil {
		t.Fatalf("acquire depth subscription: %v", err)
	}
}

func TestMarketDepthWebSocketSendsInitialPayload(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(154.9, 900, 4),
		},
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(155.1, 850, 5),
		},
	)

	host, port := splitHostPort(t, quoteServer.Addr())
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "US",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := newTestServer(t, store)
	acquireTestDepthSubscription(t, server, "US", "TME")
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: livecore.Subscriptions{
			ProviderBrokerID: "futu",
			Depth: []livecore.DepthSubscription{{
				Market:       "US",
				Symbol:       "TME",
				InstrumentID: "US.TME",
				Num:          10,
			}},
		},
	}); err != nil {
		t.Fatalf("subscribe depth websocket: %v", err)
	}

	event := readLiveWebSocketEventOfType(t, conn, "market.depth")
	if event["type"] != "market.depth" {
		t.Fatalf("unexpected websocket event: %+v", event)
	}
	if event["source"] != "market-data" {
		t.Fatalf("unexpected websocket source: %+v", event)
	}
	payload := liveWebSocketPayload(t, event, "market.depth")
	request := jftradeCheckedTypeAssertion[map[string]any](payload["request"])
	if request == nil || request["instrumentId"] != "US.TME" {
		t.Fatalf("unexpected request payload: %+v", payload["request"])
	}
	depth := jftradeCheckedTypeAssertion[map[string]any](payload["depth"])
	if depth == nil {
		t.Fatalf("missing depth payload: %+v", event)
	}
	bids := jftradeCheckedTypeAssertion[[]any](depth["bids"])
	if len(bids) != 1 {
		t.Fatalf("bids len = %d, want 1", len(bids))
	}
}

// ---------------------------------------------------------------------------
// OpenD error propagation
// ---------------------------------------------------------------------------

func TestMarketDepthOpenDError(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBookError(fmt.Errorf("opend simulated error"))

	host, port := splitHostPort(t, quoteServer.Addr())
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := newTestServer(t, store)
	acquireTestDepthSubscription(t, server, "US", "NVDA")
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/US/NVDA?num=5")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	// Should get 502 when OpenD returns error
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 for OpenD error, got %d", resp.StatusCode)
	}

	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.OK {
		t.Fatal("expected ok=false")
	}
	if envelope.Error.Code != "OPEND_DEPTH_FAILED" {
		t.Errorf("error code = %q, want OPEND_DEPTH_FAILED", envelope.Error.Code)
	}
}
