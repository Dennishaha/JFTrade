package servercore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

// ---------------------------------------------------------------------------
// Depth endpoint routing & HTTP-level behaviour
// ---------------------------------------------------------------------------

func TestMarketDepthEndpointRouting(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/US/NVDA?num=5")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	// Without a configured OpenD the handler should return 502, NOT 404.
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("depth endpoint returned 404 — route not registered")
	}
	// 502 means the route matched but OpenD is unreachable.
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("depth endpoint returned %d, want 502 without OpenD", resp.StatusCode)
	}
}

func TestMarketDepthEndpointMethodNotAllowed(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/market-data/depth/US/NVDA", "application/json", nil)
	if err != nil {
		t.Fatalf("POST depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST to depth endpoint should return 404, got %d", resp.StatusCode)
	}
}

func TestMarketDepthEndpointPutNotAllowed(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	req, jftradeErr1 := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/market-data/depth/US/NVDA", nil)
	jftradeCheckTestError(t, jftradeErr1)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT to depth endpoint should return 404, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Depth response shape (mock OpenD)
// ---------------------------------------------------------------------------

func TestMarketDepthResponseWithMockOpenD(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBook(
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(155.0, 1000, 5),
			marketDataDepthOrderBookFixture(154.5, 500, 3),
		},
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(155.5, 800, 4),
			marketDataDepthOrderBookFixture(156.0, 1200, 6),
		},
	)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0) // placeholder — actual value irrelevant for mock
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/US/NVDA?num=10")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("depth endpoint returned %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Request struct {
				Market       string `json:"market"`
				Symbol       string `json:"symbol"`
				InstrumentID string `json:"instrumentId"`
				Num          int    `json:"num"`
			} `json:"request"`
			Depth struct {
				Symbol     string `json:"symbol"`
				SymbolName string `json:"symbolName"`
				Bids       []struct {
					Price      float64 `json:"price"`
					Volume     float64 `json:"volume"`
					OrderCount int32   `json:"orderCount"`
				} `json:"bids"`
				Asks []struct {
					Price      float64 `json:"price"`
					Volume     float64 `json:"volume"`
					OrderCount int32   `json:"orderCount"`
				} `json:"asks"`
			} `json:"depth"`
			Meta struct {
				InstrumentID string `json:"instrumentId"`
				Source       string `json:"source"`
				FromCache    bool   `json:"fromCache"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode depth response: %v", err)
	}

	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if envelope.Data.Request.Market != "US" {
		t.Errorf("request market = %q, want US", envelope.Data.Request.Market)
	}
	if envelope.Data.Request.Symbol != "NVDA" {
		t.Errorf("request symbol = %q, want NVDA", envelope.Data.Request.Symbol)
	}
	if envelope.Data.Request.InstrumentID != "US.NVDA" {
		t.Errorf("request instrumentId = %q, want US.NVDA", envelope.Data.Request.InstrumentID)
	}
	if envelope.Data.Request.Num != 10 {
		t.Errorf("request num = %d, want 10", envelope.Data.Request.Num)
	}

	if envelope.Data.Depth.Symbol != "US.NVDA" {
		t.Errorf("depth symbol = %q, want US.NVDA", envelope.Data.Depth.Symbol)
	}
	if len(envelope.Data.Depth.Bids) != 2 {
		t.Fatalf("bids len = %d, want 2", len(envelope.Data.Depth.Bids))
	}
	if envelope.Data.Depth.Bids[0].Price != 155.0 {
		t.Errorf("bids[0].price = %f, want 155.0", envelope.Data.Depth.Bids[0].Price)
	}
	if envelope.Data.Depth.Bids[0].Volume != 1000 {
		t.Errorf("bids[0].volume = %f, want 1000", envelope.Data.Depth.Bids[0].Volume)
	}
	if envelope.Data.Depth.Bids[0].OrderCount != 5 {
		t.Errorf("bids[0].orderCount = %d, want 5", envelope.Data.Depth.Bids[0].OrderCount)
	}
	if len(envelope.Data.Depth.Asks) != 2 {
		t.Fatalf("asks len = %d, want 2", len(envelope.Data.Depth.Asks))
	}
	if envelope.Data.Depth.Asks[0].Price != 155.5 {
		t.Errorf("asks[0].price = %f, want 155.5", envelope.Data.Depth.Asks[0].Price)
	}
	if envelope.Data.Meta.InstrumentID != "US.NVDA" {
		t.Errorf("meta instrumentId = %q, want US.NVDA", envelope.Data.Meta.InstrumentID)
	}
	if envelope.Data.Meta.Source != "bbgo:futu" {
		t.Errorf("meta source = %q, want bbgo:futu", envelope.Data.Meta.Source)
	}
	if envelope.Data.Meta.FromCache {
		t.Error("meta fromCache should be false for direct depth query")
	}

	if got := quoteServer.orderBookCallCount(); got != 1 {
		t.Errorf("orderBook OpenD calls = %d, want 1", got)
	}
}

func TestMarketDepthWebSocketSendsInitialPayload(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBook(
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(154.9, 900, 4),
		},
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(155.1, 850, 5),
		},
	)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: liveWebSocketSubscriptions{
			Depth: []liveWebSocketDepthSubscription{{
				Market:       "US",
				Symbol:       "TME",
				InstrumentID: "US.TME",
				Num:          10,
			}},
		},
	}); err != nil {
		t.Fatalf("subscribe depth websocket: %v", err)
	}

	_ = readLiveWebSocketEvent(t, conn)
	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "market.depth" {
		t.Fatalf("unexpected websocket event: %+v", event)
	}
	request := jftradeCheckedTypeAssertion[map[string]any](event["request"])
	if request == nil || request["instrumentId"] != "US.TME" {
		t.Fatalf("unexpected request payload: %+v", event["request"])
	}
	depth := jftradeCheckedTypeAssertion[map[string]any](event["depth"])
	if depth == nil {
		t.Fatalf("missing depth payload: %+v", event)
	}
	bids := jftradeCheckedTypeAssertion[[]any](depth["bids"])
	if len(bids) != 1 {
		t.Fatalf("bids len = %d, want 1", len(bids))
	}
}

// ---------------------------------------------------------------------------
// Num parameter clamping
// ---------------------------------------------------------------------------

func TestMarketDepthNumClamping(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBook(
		[]*qotcommonpb.OrderBook{marketDataDepthOrderBookFixture(100, 10, 1)},
		[]*qotcommonpb.OrderBook{marketDataDepthOrderBookFixture(101, 10, 1)},
	)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		queryNum   string
		expectCode int
		expectNum  int
	}{
		{"num=0 clamps to 1", "0", http.StatusOK, 1},
		{"num=-5 clamps to 1", "-5", http.StatusOK, 1},
		{"num=100 clamps to 50", "100", http.StatusOK, 50},
		{"num=50 is max valid", "50", http.StatusOK, 50},
		{"no num defaults to 10", "", http.StatusOK, 10},
		{"num=5 is valid", "5", http.StatusOK, 5},
		{"non-numeric num defaults to 10", "abc", http.StatusOK, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := srv.URL + "/api/v1/market-data/depth/HK/00700"
			if tt.queryNum != "" {
				url += "?num=" + tt.queryNum
			}
			resp, err := jftradeTestHTTPGet(t, url)
			if err != nil {
				t.Fatalf("GET depth: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

			if resp.StatusCode != tt.expectCode {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.expectCode)
			}
			var envelope struct {
				OK   bool `json:"ok"`
				Data struct {
					Request struct {
						Num int `json:"num"`
					} `json:"request"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode depth response: %v", err)
			}
			if !envelope.OK {
				t.Fatal("expected ok=true")
			}
			if envelope.Data.Request.Num != tt.expectNum {
				t.Errorf("response request.num = %d, want %d", envelope.Data.Request.Num, tt.expectNum)
			}
			if got := quoteServer.orderBookLastNum(); got != int32(tt.expectNum) {
				t.Errorf("OpenD order book num = %d, want %d", got, tt.expectNum)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Market / symbol casing normalisation
// ---------------------------------------------------------------------------

func TestMarketDepthSymbolCasing(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBook(
		[]*qotcommonpb.OrderBook{marketDataDepthOrderBookFixture(100, 10, 1)},
		[]*qotcommonpb.OrderBook{marketDataDepthOrderBookFixture(101, 10, 1)},
	)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/us/nvda?num=5")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lowercase depth endpoint returned %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Request struct {
				Market       string `json:"market"`
				Symbol       string `json:"symbol"`
				InstrumentID string `json:"instrumentId"`
			} `json:"request"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if envelope.Data.Request.Market != "US" {
		t.Errorf("market = %q, want US (upper-cased)", envelope.Data.Request.Market)
	}
	if envelope.Data.Request.Symbol != "NVDA" {
		t.Errorf("symbol = %q, want NVDA (upper-cased)", envelope.Data.Request.Symbol)
	}
	if envelope.Data.Request.InstrumentID != "US.NVDA" {
		t.Errorf("instrumentId = %q, want US.NVDA", envelope.Data.Request.InstrumentID)
	}
}

// ---------------------------------------------------------------------------
// HK market depth
// ---------------------------------------------------------------------------

func TestMarketDepthHKMarket(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBook(
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(320.0, 5000, 10),
			marketDataDepthOrderBookFixture(319.8, 3000, 8),
		},
		[]*qotcommonpb.OrderBook{
			marketDataDepthOrderBookFixture(320.2, 4000, 6),
			marketDataDepthOrderBookFixture(320.4, 2000, 3),
		},
	)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/HK/00700?num=5")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("depth endpoint returned %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Request struct {
				Market       string `json:"market"`
				Symbol       string `json:"symbol"`
				InstrumentID string `json:"instrumentId"`
			} `json:"request"`
			Depth struct {
				Symbol string `json:"symbol"`
				Bids   []struct {
					Price      float64 `json:"price"`
					Volume     float64 `json:"volume"`
					OrderCount int32   `json:"orderCount"`
				} `json:"bids"`
				Asks []struct {
					Price      float64 `json:"price"`
					Volume     float64 `json:"volume"`
					OrderCount int32   `json:"orderCount"`
				} `json:"asks"`
			} `json:"depth"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if envelope.Data.Request.InstrumentID != "HK.00700" {
		t.Errorf("instrumentId = %q, want HK.00700", envelope.Data.Request.InstrumentID)
	}
	if envelope.Data.Depth.Symbol != "HK.00700" {
		t.Errorf("depth symbol = %q, want HK.00700", envelope.Data.Depth.Symbol)
	}
}

// ---------------------------------------------------------------------------
// Empty order book (no bids/asks)
// ---------------------------------------------------------------------------

func TestMarketDepthEmptyOrderBook(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	// Empty order book
	quoteServer.setOrderBook(nil, nil)

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/US/AAPL?num=10")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty order book endpoint returned %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Depth struct {
				Bids []any `json:"bids"`
				Asks []any `json:"asks"`
			} `json:"depth"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !envelope.OK {
		t.Fatal("expected ok=true for empty order book")
	}
	if len(envelope.Data.Depth.Bids) != 0 {
		t.Errorf("expected 0 bids, got %d", len(envelope.Data.Depth.Bids))
	}
	if len(envelope.Data.Depth.Asks) != 0 {
		t.Errorf("expected 0 asks, got %d", len(envelope.Data.Depth.Asks))
	}
}

// ---------------------------------------------------------------------------
// OpenD error propagation
// ---------------------------------------------------------------------------

func TestMarketDepthOpenDError(t *testing.T) {
	quoteServer := startMarketDataQuoteOpenDServer(t)
	defer quoteServer.stop()

	quoteServer.setOrderBookErr(fmt.Errorf("opend simulated error"))

	host, port := splitHostPort(t, quoteServer.addr)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := fmt.Sprintf("%d", 0)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
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

// ---------------------------------------------------------------------------
// Route collision safety: depth prefix does not catch other market-data routes
// ---------------------------------------------------------------------------

func TestMarketDepthRouteDoesNotCollide(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	// /api/v1/market-data/depths should NOT match the depth route (different prefix)
	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depths")
	if err != nil {
		t.Fatalf("GET depths: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/api/v1/market-data/depths returned %d, want 404 (should not collide with depth route)", resp.StatusCode)
	}
}
