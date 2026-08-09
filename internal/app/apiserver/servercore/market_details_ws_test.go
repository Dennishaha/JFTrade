package servercore

import (
	"net/http/httptest"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	livecore "github.com/jftrade/jftrade-main/internal/live"
)

func TestMarketSecurityDetailsWebSocketSendsInitialPayload(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	server := newMarketDataTestServerWithQuoteRuntime(t, quoteServer.Addr())
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: livecore.Subscriptions{
			ProviderBrokerID: "futu",
			SecurityDetails: []livecore.SecurityDetailsSubscription{{
				Market:       "HK",
				Symbol:       "00700",
				InstrumentID: "HK.00700",
			}},
		},
	}); err != nil {
		t.Fatalf("subscribe security details websocket: %v", err)
	}

	event := readLiveWebSocketEventOfType(t, conn, "market.security-details")
	if event["type"] != "market.security-details" {
		t.Fatalf("unexpected websocket event: %+v", event)
	}
	if event["source"] != "market-data" {
		t.Fatalf("unexpected websocket source: %+v", event)
	}
	payload := liveWebSocketPayload(t, event, "market.security-details")
	request, ok := payload["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", payload["request"])
	}
	if got := request["instrumentId"]; got != "HK.00700" {
		t.Fatalf("instrumentId = %v", got)
	}
	security, ok := payload["security"].(map[string]any)
	if !ok {
		t.Fatalf("security payload type = %T", payload["security"])
	}
	if got := security["name"]; got != "Tencent Holdings" {
		t.Fatalf("security name = %v", got)
	}
}
