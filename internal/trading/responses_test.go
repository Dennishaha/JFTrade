package trading

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestBrokerRuntimeResponseJSONShape(t *testing.T) {
	response := &BrokerRuntimeResponse{
		Descriptor: BrokerRuntimeDescriptor{
			ID: "futu", DisplayName: "Futu OpenAPI via OpenD",
			Environments: []string{"SIMULATE", "REAL"}, Capabilities: []BrokerMarketCapability{}, Notes: []string{},
		},
		Session: BrokerRuntimeSession{
			BrokerID: "futu", DisplayName: "Futu OpenAPI via OpenD",
			Connection: BrokerRuntimeConnection{
				Host: "127.0.0.1", APIPort: 11110, WebSocketPort: 11111, Port: 11110,
				MarketDataTransport: "bbgo-opend-tcp-api",
			},
			Connectivity: "disconnected", CheckedAt: "", AccountsDiscovered: 0,
			LiveWebSocketClients: BrokerRuntimeLiveClients{Limit: 20},
		},
		Accounts: []BrokerRuntimeAccount{},
	}

	keys := responseJSONKeys(t, response)
	for _, key := range []string{"descriptor", "session", "accounts"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("runtime response missing %q: %#v", key, keys)
		}
	}
	session, ok := keys["session"].(map[string]any)
	if !ok {
		t.Fatalf("runtime session = %#v", keys["session"])
	}
	for _, key := range []string{"connection", "connectivity", "globalState", "liveWebSocketClients"} {
		if _, ok := session[key]; !ok {
			t.Fatalf("runtime session missing %q: %#v", key, session)
		}
	}
	if session["globalState"] != nil || session["lastError"] != nil {
		t.Fatalf("runtime nullable fields = globalState:%#v lastError:%#v", session["globalState"], session["lastError"])
	}
	if accounts, ok := keys["accounts"].([]any); !ok || len(accounts) != 0 {
		t.Fatalf("runtime accounts = %#v, want empty array", keys["accounts"])
	}
}

// TestBrokerFundsResponseJSONShape 锁定 funds 响应的 JSON 键，
// 保证 typed DTO 与历史 map 响应形状一致。
func TestBrokerFundsResponseJSONShape(t *testing.T) {
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: &stubMarketDataReader{
			queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
				return &broker.FundsSnapshot{
					AccountID:          "acc-1",
					TradingEnvironment: "REAL",
					Market:             "US",
					Currency:           new("USD"),
				}, nil
			},
		}}
	}))

	success, err := service.Funds(t.Context(), broker.ReadQuery{BrokerID: "futu"})
	if err != nil {
		t.Fatalf("Funds: %v", err)
	}
	successKeys := responseJSONKeys(t, success)
	for _, key := range []string{"checkedAt", "connectivity", "lastError", "summary", "currencyBalances", "marketAssets"} {
		if _, ok := successKeys[key]; !ok {
			t.Fatalf("funds success response missing %q: %#v", key, success)
		}
	}
	if successKeys["lastError"] != nil {
		t.Fatalf("funds success lastError = %#v, want null", successKeys["lastError"])
	}
	if _, ok := successKeys["summary"].(map[string]any); !ok {
		t.Fatalf("funds success summary = %#v, want object", successKeys["summary"])
	}
	if balances, ok := successKeys["currencyBalances"].([]any); !ok || len(balances) != 0 {
		t.Fatalf("funds success currencyBalances = %#v, want empty array", successKeys["currencyBalances"])
	}

	failure := fundsReadError(errFundsShapeProbe)
	failureKeys := responseJSONKeys(t, failure)
	if failureKeys["summary"] != nil {
		t.Fatalf("funds failure summary = %#v, want null", failureKeys["summary"])
	}
	if failureKeys["lastError"] != errFundsShapeProbe.Error() {
		t.Fatalf("funds failure lastError = %#v", failureKeys["lastError"])
	}
	for _, key := range []string{"currencyBalances", "marketAssets"} {
		if values, ok := failureKeys[key].([]any); !ok || len(values) != 0 {
			t.Fatalf("funds failure %s = %#v, want empty array", key, failureKeys[key])
		}
	}
}

var errFundsShapeProbe = errors.New("broker market data not available")

// TestBrokerPositionsResponseJSONShape 锁定 positions 响应的 JSON 键。
func TestBrokerPositionsResponseJSONShape(t *testing.T) {
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: &stubMarketDataReader{
			queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
				return []broker.PositionSnapshot{{AccountID: "acc-1", Market: "US", Symbol: "US.AAPL", Quantity: 1}}, nil
			},
		}}
	}))

	success, err := service.Positions(t.Context(), broker.ReadQuery{BrokerID: "futu"})
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	keys := responseJSONKeys(t, success)
	for _, key := range []string{"checkedAt", "connectivity", "lastError", "positions"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("positions success response missing %q: %#v", key, success)
		}
	}
	positions, ok := keys["positions"].([]any)
	if !ok || len(positions) != 1 {
		t.Fatalf("positions = %#v, want one entry", keys["positions"])
	}
	entry, ok := positions[0].(map[string]any)
	if !ok || entry["symbol"] != "US.AAPL" || entry["accountId"] != "acc-1" {
		t.Fatalf("position entry = %#v", positions[0])
	}

	failureKeys := responseJSONKeys(t, positionsReadError(errFundsShapeProbe))
	if values, ok := failureKeys["positions"].([]any); !ok || len(values) != 0 {
		t.Fatalf("positions failure positions = %#v, want empty array", failureKeys["positions"])
	}
}

func TestBrokerReadStatusSerializesNullLastError(t *testing.T) {
	data, err := json.Marshal(connectedReadStatus())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	value, ok := decoded["lastError"]
	if !ok || value != nil {
		t.Fatalf("lastError = %#v (present=%v), want explicit null", value, ok)
	}
}
