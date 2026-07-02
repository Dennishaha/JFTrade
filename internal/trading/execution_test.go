package trading

import (
	"context"
	"errors"
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

func TestNormalizeExecutionOrderSupportsStopAndMarketOrders(t *testing.T) {
	service := NewService()
	stopPrice := 97.5

	stopCommand, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "SELL", OrderType: "STOP", Quantity: 4, StopPrice: &stopPrice,
	})
	if err != nil {
		t.Fatalf("normalize stop order: %v", err)
	}
	if stopCommand.OrderType != "STOP" || stopCommand.Query.StopPrice == nil || *stopCommand.Query.StopPrice != stopPrice {
		t.Fatalf("stop command = %#v", stopCommand)
	}

	marketCommand, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "HK", Symbol: "00700", Side: "BUY", OrderType: "MARKET", Quantity: 100,
	})
	if err != nil {
		t.Fatalf("normalize market order: %v", err)
	}
	if marketCommand.OrderType != "MARKET" || marketCommand.Query.Market != "HK" || marketCommand.Query.Session != nil {
		t.Fatalf("market command = %#v", marketCommand)
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

func TestExecutionOrderServiceFacadeUsesInjectedStoresAndBrokerCommands(t *testing.T) {
	ctx := context.Background()
	price := 101.25
	var listedFilter ExecutionOrderFilter
	var placedCommand ExecutionOrderCommand
	var canceledID string
	var eventsID string

	service := NewService(
		WithDefaultTradingEnvironment(func() string { return "real" }),
		WithListOrders(func(_ context.Context, filter ExecutionOrderFilter) (ExecutionOrders, error) {
			listedFilter = filter
			return ExecutionOrders{Orders: []ExecutionOrder{{
				InternalOrderID: "local-1",
				BrokerID:        filter.BrokerID,
				Status:          "SUBMITTED",
				UpdatedAt:       "2026-07-02T01:02:03Z",
				CreatedAt:       "2026-07-02T01:02:03Z",
			}}}, nil
		}),
		WithPlaceOrder(func(_ context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
			placedCommand = command
			return ExecutionOrder{
				InternalOrderID: "local-2",
				BrokerID:        command.BrokerID,
				BrokerOrderID:   new("broker-2"),
				Status:          "SUBMITTED",
			}, nil
		}),
		WithCancelOrder(func(_ context.Context, id string) (ExecutionOrder, error) {
			canceledID = id
			return ExecutionOrder{
				InternalOrderID: id,
				BrokerID:        "futu",
				BrokerOrderIDEx: new("broker-ex-3"),
				Status:          "CANCELING",
			}, nil
		}),
		WithGetOrderEvents(func(_ context.Context, id string) (ExecutionOrderEvents, error) {
			eventsID = id
			return ExecutionOrderEvents{
				InternalOrderID: id,
				Events: []ExecutionOrderEvent{{
					ID:              "evt-1",
					InternalOrderID: id,
					EventType:       "order_submitted",
					NextStatus:      "SUBMITTED",
				}},
			}, nil
		}),
	)

	filter := service.ExecutionFilter(" futu ", "", " acc-1 ", " us ")
	orders, err := service.ListExecutionOrders(ctx, filter, true)
	if err != nil {
		t.Fatalf("ListExecutionOrders: %v", err)
	}
	if len(orders.Orders) != 1 || listedFilter.TradingEnvironment != "REAL" || listedFilter.AccountID != "acc-1" || listedFilter.Market != "US" {
		t.Fatalf("listed orders/filter = %#v / %#v", orders, listedFilter)
	}

	snapshot, err := service.ExecutionOrdersSnapshot(ctx)
	if err != nil {
		t.Fatalf("ExecutionOrdersSnapshot: %v", err)
	}
	if len(snapshot.Orders) != 1 || listedFilter != (ExecutionOrderFilter{}) {
		t.Fatalf("snapshot/filter = %#v / %#v", snapshot, listedFilter)
	}

	preview, err := service.PreviewExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 3, Price: &price,
	})
	if err != nil {
		t.Fatalf("PreviewExecutionOrder: %v", err)
	}
	if !preview.PreviewValid || preview.BrokerID != "futu" || preview.Symbol != "US.AAPL" || preview.Price == nil || *preview.Price != price {
		t.Fatalf("preview = %#v", preview)
	}

	created, err := service.CreateExecutionOrder(ctx, ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 3, Price: &price, ClientOrderID: "client-123",
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder: %v", err)
	}
	if !created.Accepted || created.Operation != "PLACE" || created.InternalOrderID == nil || *created.InternalOrderID != "local-2" {
		t.Fatalf("created response = %#v", created)
	}
	if placedCommand.Symbol != "US.AAPL" || placedCommand.Query.ClientOrderID != "client-123" || placedCommand.Query.Remark == nil || *placedCommand.Query.Remark != "client-123" {
		t.Fatalf("placed command = %#v", placedCommand)
	}

	canceled, err := service.CancelExecutionOrder(ctx, " local-2 ")
	if err != nil {
		t.Fatalf("CancelExecutionOrder: %v", err)
	}
	if canceled.Operation != "CANCEL" || canceled.BrokerOrderIDEx == nil || *canceled.BrokerOrderIDEx != "broker-ex-3" || canceledID != "local-2" {
		t.Fatalf("cancel response/id = %#v / %q", canceled, canceledID)
	}

	events, err := service.ExecutionOrderEvents(ctx, " local-2 ")
	if err != nil {
		t.Fatalf("ExecutionOrderEvents: %v", err)
	}
	if eventsID != "local-2" || len(events.Events) != 1 || events.Events[0].EventType != "order_submitted" {
		t.Fatalf("events/id = %#v / %q", events, eventsID)
	}
}

func TestExecutionOrderServiceFacadeReturnsBusinessErrors(t *testing.T) {
	ctx := context.Background()
	price := 9.75
	upstream := errors.New("broker rejected order")
	service := NewService(
		WithListOrders(func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error) {
			return ExecutionOrders{}, upstream
		}),
		WithPlaceOrder(func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error) {
			return ExecutionOrder{}, upstream
		}),
		WithCancelOrder(func(context.Context, string) (ExecutionOrder, error) {
			return ExecutionOrder{}, upstream
		}),
		WithGetOrderEvents(func(context.Context, string) (ExecutionOrderEvents, error) {
			return ExecutionOrderEvents{}, upstream
		}),
	)

	if _, err := service.ListExecutionOrders(ctx, ExecutionOrderFilter{}, false); !errors.Is(err, upstream) {
		t.Fatalf("ListExecutionOrders error = %v, want upstream", err)
	}
	if _, err := service.CreateExecutionOrder(ctx, ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price,
	}); !errors.Is(err, upstream) {
		t.Fatalf("CreateExecutionOrder error = %v, want upstream", err)
	}
	if _, err := service.CancelExecutionOrder(ctx, "local-1"); !errors.Is(err, upstream) {
		t.Fatalf("CancelExecutionOrder error = %v, want upstream", err)
	}
	if _, err := service.ExecutionOrderEvents(ctx, "local-1"); !errors.Is(err, upstream) {
		t.Fatalf("ExecutionOrderEvents error = %v, want upstream", err)
	}

	if _, err := service.PreviewExecutionOrder(ExecutionPlaceRequest{Market: "US", Symbol: "AAPL", Side: "HOLD", Quantity: 1}); !IsRequestError(err) {
		t.Fatalf("PreviewExecutionOrder error = %v, want RequestError", err)
	}
	if IsRequestError(upstream) {
		t.Fatalf("plain upstream error classified as RequestError")
	}
}
