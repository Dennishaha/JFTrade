package trading

import (
	"strings"
	"testing"
)

func TestNormalizeExecutionOrderDefaultsUSLimitOrder(t *testing.T) {
	service := NewService(WithDefaultTradingEnvironment(func() string { return "simulate" }))
	price := 123.45

	command, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market:   "us",
		Symbol:   "aapl",
		Side:     "buy",
		Quantity: 10,
		Price:    &price,
	})
	if err != nil {
		t.Fatalf("normalizeExecutionOrder: %v", err)
	}
	if command.BrokerID != "futu" || command.Symbol != "US.AAPL" || command.Side != "BUY" || command.OrderType != "LIMIT" {
		t.Fatalf("command = %#v", command)
	}
	if command.Query.TradingEnvironment != "SIMULATE" || command.Query.Market != "US" {
		t.Fatalf("query = %#v", command.Query)
	}
	if command.Query.TimeInForce == nil || *command.Query.TimeInForce != "DAY" {
		t.Fatalf("timeInForce = %#v", command.Query.TimeInForce)
	}
	if command.Query.Session == nil || *command.Query.Session != "RTH" {
		t.Fatalf("session = %#v", command.Query.Session)
	}
	if command.Query.FillOutsideRTH == nil || *command.Query.FillOutsideRTH {
		t.Fatalf("fillOutsideRTH = %#v, want false", command.Query.FillOutsideRTH)
	}
}

func TestNormalizeExecutionOrderSupportsExtendedUSLimitSessions(t *testing.T) {
	service := NewService()
	price := 88.0

	command, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market:    "US",
		Symbol:    "AAPL",
		Side:      "SELL",
		OrderType: "LIMIT",
		Session:   "ETH",
		Quantity:  5,
		Price:     &price,
	})
	if err != nil {
		t.Fatalf("normalizeExecutionOrder: %v", err)
	}
	if command.Query.Session == nil || *command.Query.Session != "ETH" {
		t.Fatalf("session = %#v", command.Query.Session)
	}
	if command.Query.FillOutsideRTH == nil || !*command.Query.FillOutsideRTH {
		t.Fatalf("fillOutsideRTH = %#v, want true", command.Query.FillOutsideRTH)
	}
}

func TestNormalizeExecutionOrderRejectsBusinessRuleViolations(t *testing.T) {
	service := NewService()
	price := 10.0
	stopPrice := 9.0

	cases := []struct {
		name    string
		payload ExecutionPlaceRequest
		want    string
	}{
		{
			name: "unsupported broker",
			payload: ExecutionPlaceRequest{
				BrokerID: "ib", Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price,
			},
			want: "brokerId=futu only",
		},
		{
			name: "missing price for limit order",
			payload: ExecutionPlaceRequest{
				Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1,
			},
			want: "requires price",
		},
		{
			name: "missing stop price",
			payload: ExecutionPlaceRequest{
				Market: "US", Symbol: "AAPL", Side: "BUY", OrderType: "STOP_LIMIT", Quantity: 1, Price: &price,
			},
			want: "requires stopPrice",
		},
		{
			name: "session limited to US market",
			payload: ExecutionPlaceRequest{
				Market: "HK", Symbol: "00700", Side: "BUY", Quantity: 1, OrderType: "MARKET", Session: "ETH",
			},
			want: "supported for US market orders only",
		},
		{
			name: "fok unsupported",
			payload: ExecutionPlaceRequest{
				Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price, TimeInForce: "FOK",
			},
			want: "does not support timeInForce FOK",
		},
		{
			name: "stop limit requires stop price even with price",
			payload: ExecutionPlaceRequest{
				Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, OrderType: "STOP_LIMIT", Price: &price, StopPrice: &stopPrice,
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.normalizeExecutionOrder(tc.payload)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("normalizeExecutionOrder unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
