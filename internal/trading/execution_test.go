package trading

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestNormalizeExecutionOrderDefaultsUSLimitOrder(t *testing.T) {
	service := newExecutionTestService(WithDefaultTradingEnvironment(func() string { return "simulate" }))
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
	if command.BrokerID != "test-broker" || command.Symbol != "US.AAPL" || command.Side != "BUY" || command.OrderType != "LIMIT" {
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
	service := newExecutionTestService()
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
	service := newExecutionTestService()
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
	service := newExecutionTestService()
	price := 10.0
	stopPrice := 9.0

	cases := []struct {
		name    string
		payload ExecutionPlaceRequest
		want    string
	}{
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

func TestNormalizeExecutionOrderPreservesBrokerAbstraction(t *testing.T) {
	service := NewService(WithActiveBroker(func() broker.Broker { return &stubBroker{id: "ib"} }))
	price := 10.0
	command, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		BrokerID: "ib", Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 1, Price: &price,
	})
	if err != nil {
		t.Fatalf("normalizeExecutionOrder() error = %v", err)
	}
	if command.BrokerID != "ib" || command.Query.BrokerID != "ib" {
		t.Fatalf("broker selection = %#v", command)
	}
}

func TestExecutionOrderServiceFacadeUsesInjectedStoresAndBrokerCommands(t *testing.T) {
	ctx := context.Background()
	price := 101.25
	var listedFilter ExecutionOrderFilter
	var placedCommand ExecutionOrderCommand
	var canceledID string
	var eventsID string

	service := newExecutionTestService(
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
	if !preview.PreviewValid || preview.BrokerID != "test-broker" || preview.Symbol != "US.AAPL" || preview.Price == nil || *preview.Price != price {
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
	service := newExecutionTestService(
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

func TestCreateExecutionOrderRejectsInvalidPayloadBeforeBrokerCall(t *testing.T) {
	price := 9.75
	service := newExecutionTestService(WithPlaceOrder(func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error) {
		t.Fatal("placeOrder should not be called for invalid payloads")
		return ExecutionOrder{}, nil
	}))

	_, err := service.CreateExecutionOrder(context.Background(), ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", Quantity: 0, Price: &price,
	})
	if !IsRequestError(err) || !strings.Contains(err.Error(), "quantity must be greater than 0") {
		t.Fatalf("CreateExecutionOrder error = %v", err)
	}
}

func TestCreateExecutionOrderRunsPreTradeRiskBeforeBrokerCall(t *testing.T) {
	price := 9.75
	service := newExecutionTestService(
		WithPreTradeRiskGateway(NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
			return PreTradeRiskConfig{}
		})),
		WithPlaceOrder(func(context.Context, ExecutionOrderCommand) (ExecutionOrder, error) {
			t.Fatal("placeOrder should not be called when REAL trading is disabled")
			return ExecutionOrder{}, nil
		}),
	)

	_, err := service.CreateExecutionOrder(context.Background(), ExecutionPlaceRequest{
		TradingEnvironment: "REAL",
		Market:             "US",
		Symbol:             "AAPL",
		Side:               "BUY",
		Quantity:           1,
		Price:              &price,
	})
	if !IsRiskRejected(err) || !strings.Contains(err.Error(), "real trading is disabled") {
		t.Fatalf("CreateExecutionOrder error = %v, want real-trading risk rejection", err)
	}
}

func TestCreateExecutionOrderAllowsRuntimeEnabledRealTradeBeforeBrokerCall(t *testing.T) {
	price := 100.0
	maxNotional := 500.0
	placed := false
	service := newExecutionTestService(
		WithPreTradeRiskGateway(NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
			return PreTradeRiskConfig{RealTradingEnabled: true, RuntimeMaxOrderNotional: &maxNotional}
		})),
		WithPlaceOrder(func(_ context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
			placed = true
			return ExecutionOrder{InternalOrderID: "local-risk-real", BrokerID: command.BrokerID, Status: "SUBMITTED"}, nil
		}),
	)

	response, err := service.CreateExecutionOrder(context.Background(), ExecutionPlaceRequest{
		TradingEnvironment: "REAL",
		Market:             "US",
		Symbol:             "AAPL",
		Side:               "BUY",
		Quantity:           1,
		Price:              &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder: %v", err)
	}
	if !placed || !response.Accepted || response.InternalOrderID == nil || *response.InternalOrderID != "local-risk-real" {
		t.Fatalf("response=%#v placed=%v", response, placed)
	}
}

func TestCreateExecutionOrderAllowsSimulateWhenRealTradingIsDisabled(t *testing.T) {
	price := 9.75
	placed := false
	service := newExecutionTestService(
		WithPreTradeRiskGateway(NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
			return PreTradeRiskConfig{}
		})),
		WithPlaceOrder(func(_ context.Context, command ExecutionOrderCommand) (ExecutionOrder, error) {
			placed = true
			return ExecutionOrder{InternalOrderID: "local-risk-sim", BrokerID: command.BrokerID, Status: "SUBMITTED"}, nil
		}),
	)

	response, err := service.CreateExecutionOrder(context.Background(), ExecutionPlaceRequest{
		TradingEnvironment: "SIMULATE",
		Market:             "US",
		Symbol:             "AAPL",
		Side:               "BUY",
		Quantity:           1,
		Price:              &price,
	})
	if err != nil {
		t.Fatalf("CreateExecutionOrder: %v", err)
	}
	if !placed || !response.Accepted || response.InternalOrderID == nil || *response.InternalOrderID != "local-risk-sim" {
		t.Fatalf("response=%#v placed=%v", response, placed)
	}
}

func TestPreTradeRiskRejectsKillSwitchAndLimits(t *testing.T) {
	price := 10.0
	maxQty := 5.0
	maxNotional := 40.0
	command := ExecutionOrderCommand{Query: broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL"},
		Quantity:  6,
		Price:     &price,
	}}

	killSwitch := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{RealTradingEnabled: true, RuntimeKillSwitch: true}
	})
	if decision := killSwitch.EvaluatePlaceOrder(context.Background(), command); decision.Allows() || decision.ReasonCode != "REAL_TRADE_KILL_SWITCH_ACTIVE" {
		t.Fatalf("kill switch decision = %#v", decision)
	}

	quantityLimit := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{RealTradingEnabled: true, RuntimeMaxOrderQty: &maxQty}
	})
	if decision := quantityLimit.EvaluatePlaceOrder(context.Background(), command); decision.Allows() || decision.ReasonCode != "MAX_ORDER_QUANTITY_EXCEEDED" {
		t.Fatalf("quantity decision = %#v", decision)
	}

	notionalLimit := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{RealTradingEnabled: true, RuntimeMaxOrderNotional: &maxNotional}
	})
	if decision := notionalLimit.EvaluatePlaceOrder(context.Background(), command); decision.Allows() || decision.ReasonCode != "MAX_ORDER_NOTIONAL_EXCEEDED" {
		t.Fatalf("notional decision = %#v", decision)
	}

	stopCommand := command
	stopCommand.Query.Price = nil
	stopCommand.Query.StopPrice = &price
	if decision := notionalLimit.EvaluatePlaceOrder(context.Background(), stopCommand); decision.Allows() || decision.ReasonCode != "MAX_ORDER_NOTIONAL_EXCEEDED" {
		t.Fatalf("stop notional decision = %#v", decision)
	}

}

func TestPreTradeRiskSnapshotUsesNonNilEmptySlices(t *testing.T) {
	snapshot := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{}
	}).Snapshot()

	if snapshot.HardStopEntries == nil || len(snapshot.HardStopEntries) != 0 {
		t.Fatalf("hardStopEntries = %#v, want non-nil empty slice", snapshot.HardStopEntries)
	}
	if snapshot.HardStopEvents == nil || len(snapshot.HardStopEvents) != 0 {
		t.Fatalf("hardStopEvents = %#v, want non-nil empty slice", snapshot.HardStopEvents)
	}
	if snapshot.KillSwitchEvents == nil || len(snapshot.KillSwitchEvents) != 0 {
		t.Fatalf("killSwitchEvents = %#v, want non-nil empty slice", snapshot.KillSwitchEvents)
	}
}

func TestPreTradeRiskDoesNotLetUnknownNotionalBypassRealTradeControls(t *testing.T) {
	maxNotional := 1000.0
	command := ExecutionOrderCommand{Query: broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL"},
		Quantity:  2,
	}}

	limitGateway := NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
		return PreTradeRiskConfig{RealTradingEnabled: true, RuntimeMaxOrderNotional: &maxNotional}
	})
	if decision := limitGateway.EvaluatePlaceOrder(t.Context(), command); decision.ReasonCode != "RISK_PRICE_UNAVAILABLE" || decision.Allows() {
		t.Fatalf("notional-limit decision = %#v", decision)
	}
}

func TestRealTradeEnvVariablesDoNotConfigurePreTradeRisk(t *testing.T) {
	t.Setenv("JFTRADE_ALLOW_REAL_TRADING", "true")
	t.Setenv("JFTRADE_REAL_TRADE_KILL_SWITCH", "on")
	t.Setenv("JFTRADE_REAL_TRADE_MAX_ORDER_QUANTITY", "12.5")
	t.Setenv("JFTRADE_REAL_TRADE_MAX_ORDER_NOTIONAL", "2500")
	t.Setenv("JFTRADE_REAL_TRADE_APPROVAL_NOTIONAL", "1000")

	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	snapshot := plane.Snapshot()
	if snapshot.RealTradingEnabled || snapshot.KillSwitchActive {
		t.Fatalf("env should not configure runtime gates: %#v", snapshot)
	}
	if snapshot.EffectiveMaxOrderQuantity != nil || snapshot.EffectiveMaxOrderNotional != nil {
		t.Fatalf("env should not configure runtime limits: %#v", snapshot)
	}
}

func TestRealTradeControlPlanePersistsKillSwitchAndHardStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real-trade-control.json")
	plane, err := NewRealTradeControlPlane(path)
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}

	if _, err := plane.ActivateKillSwitch(context.Background(), RealTradeKillSwitchCommand{
		OperatorID: "tester",
		Reason:     "incident",
	}); err != nil {
		t.Fatalf("ActivateKillSwitch: %v", err)
	}
	snapshot := plane.Snapshot()
	if !snapshot.KillSwitchActive || snapshot.KillSwitchSource == nil || *snapshot.KillSwitchSource != "RUNTIME" {
		t.Fatalf("kill switch snapshot = %#v", snapshot)
	}

	reloaded, err := NewRealTradeControlPlane(path)
	if err != nil {
		t.Fatalf("reload control plane: %v", err)
	}
	if !reloaded.Snapshot().KillSwitchActive {
		t.Fatalf("reloaded snapshot = %#v", reloaded.Snapshot())
	}
	if _, err := reloaded.ReleaseKillSwitch(context.Background(), RealTradeKillSwitchCommand{OperatorID: "tester"}); err != nil {
		t.Fatalf("ReleaseKillSwitch: %v", err)
	}
	if reloaded.Snapshot().KillSwitchActive {
		t.Fatalf("released snapshot = %#v", reloaded.Snapshot())
	}
	maxQty := 10.0
	if _, err := reloaded.UpdateRuntimeRiskConfig(context.Background(), RealTradeRuntimeRiskCommand{
		RealTradingEnabled: true,
		MaxOrderQuantity:   &maxQty,
		OperatorID:         "tester",
		Reason:             "enable real trading",
	}); err != nil {
		t.Fatalf("UpdateRuntimeRiskConfig: %v", err)
	}

	if _, err := reloaded.ActivateHardStop(context.Background(), RealTradeHardStopCommand{
		BrokerID:           "futu",
		TradingEnvironment: "REAL",
		AccountID:          "ACC-1",
		Market:             "US",
		Symbol:             "AAPL",
		OperatorID:         "tester",
		Reason:             "symbol halt",
	}); err != nil {
		t.Fatalf("ActivateHardStop: %v", err)
	}
	price := 10.0
	decision := reloaded.EvaluatePlaceOrder(context.Background(), ExecutionOrderCommand{
		BrokerID: "futu",
		Symbol:   "AAPL",
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{
				TradingEnvironment: "REAL",
				AccountID:          "ACC-1",
				Market:             "US",
			},
			Quantity: 1,
			Price:    &price,
		},
	})
	if decision.Allows() || decision.ReasonCode != "REAL_TRADE_HARD_STOP_ACTIVE" {
		t.Fatalf("hard stop decision = %#v", decision)
	}
	events := reloaded.Snapshot().HardStopEvents
	if len(events) == 0 || events[0].EventType != "rejected" {
		t.Fatalf("hard stop events = %#v", events)
	}
}

func TestRealTradeControlPlaneRuntimeRiskConfigValidationAndDisableEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real-trade-control.json")
	plane, err := NewRealTradeControlPlane(path)
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}

	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{RealTradingEnabled: true}); err == nil {
		t.Fatal("UpdateRuntimeRiskConfig enabled without limits should fail")
	}
	invalidLimit := -1.0
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{MaxOrderQuantity: &invalidLimit}); err == nil {
		t.Fatal("UpdateRuntimeRiskConfig with non-positive quantity should fail")
	}

	maxQty := 25.0
	maxNotional := 5000.0
	snapshot, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{
		TradingEnvironment: "real",
		RealTradingEnabled: true,
		MaxOrderQuantity:   &maxQty,
		MaxOrderNotional:   &maxNotional,
		OperatorID:         "tester",
		Reason:             "session open",
	})
	if err != nil {
		t.Fatalf("UpdateRuntimeRiskConfig: %v", err)
	}
	if !snapshot.RealTradingEnabled || !snapshot.RuntimeRiskConfigured {
		t.Fatalf("runtime risk snapshot = %#v", snapshot)
	}
	entry := snapshot.RiskEntry
	if entry == nil || entry.OperatorID != "tester" || entry.TradingEnvironment != "REAL" {
		t.Fatalf("runtime risk entry = %#v", snapshot.RiskEntry)
	}
	events := snapshot.RiskEvents
	if len(events) != 1 || events[0].Action != "RISK_CONFIG_UPDATED" || events[0].RealTradingEnabled == nil || !*events[0].RealTradingEnabled {
		t.Fatalf("risk update events = %#v", events)
	}

	disabled, err := plane.DisableRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{OperatorID: "tester", Reason: "session closed"})
	if err != nil {
		t.Fatalf("DisableRuntimeRiskConfig: %v", err)
	}
	if disabled.RealTradingEnabled || disabled.RuntimeRiskConfigured || disabled.RiskEntry != nil {
		t.Fatalf("disabled runtime risk snapshot = %#v", disabled)
	}
	events = disabled.RiskEvents
	if len(events) < 2 || events[0].Action != "RISK_CONFIG_DISABLED" || events[0].RealTradingEnabled == nil || *events[0].RealTradingEnabled {
		t.Fatalf("risk disable events = %#v", events)
	}

	reloaded, err := NewRealTradeControlPlane(path)
	if err != nil {
		t.Fatalf("reload control plane: %v", err)
	}
	if got := reloaded.Snapshot(); got.RuntimeRiskConfigured || len(got.RiskEvents) != len(events) {
		t.Fatalf("reloaded disabled runtime risk snapshot = %#v", got)
	}
}

func TestRealTradeControlPlaneRollsBackFailedPersistence(t *testing.T) {
	plane, err := NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("NewRealTradeControlPlane: %v", err)
	}
	if _, err := plane.ActivateKillSwitch(t.Context(), RealTradeKillSwitchCommand{Reason: "incident"}); err != nil {
		t.Fatalf("ActivateKillSwitch: %v", err)
	}
	before := plane.Snapshot()

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plane.path = filepath.Join(parentFile, "real-trade-control.json")
	if _, err := plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{}); err == nil {
		t.Fatal("ReleaseKillSwitch should fail when state cannot be persisted")
	}
	after := plane.Snapshot()
	if !after.KillSwitchActive {
		t.Fatalf("failed release opened kill switch: %#v", after)
	}
	if len(after.KillSwitchEvents) != len(before.KillSwitchEvents) {
		t.Fatalf("failed release changed audit events: before=%#v after=%#v", before, after)
	}

	plane.path = ""
	if _, err := plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{}); err != nil {
		t.Fatalf("ReleaseKillSwitch after restoring persistence: %v", err)
	}
	result, err := plane.ActivateHardStop(t.Context(), RealTradeHardStopCommand{AccountID: "ACC-1"})
	if err != nil {
		t.Fatalf("ActivateHardStop: %v", err)
	}
	entries := result.HardStopEntries
	plane.path = filepath.Join(parentFile, "real-trade-control.json")
	if _, err := plane.ReleaseHardStop(t.Context(), entries[0].ID, RealTradeHardStopCommand{}); err == nil {
		t.Fatal("ReleaseHardStop should fail when state cannot be persisted")
	}
	if got := plane.Snapshot().HardStopEntries; len(got) != 1 || got[0].ID != entries[0].ID {
		t.Fatalf("failed hard-stop release changed active entries: %#v", got)
	}

	maxQty := 10.0
	plane.path = ""
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{RealTradingEnabled: true, MaxOrderQuantity: &maxQty}); err != nil {
		t.Fatalf("UpdateRuntimeRiskConfig: %v", err)
	}
	beforeRuntimeRisk := plane.Snapshot()
	updatedMaxQty := 5.0
	plane.path = filepath.Join(parentFile, "real-trade-control.json")
	if _, err := plane.UpdateRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{RealTradingEnabled: true, MaxOrderQuantity: &updatedMaxQty}); err == nil {
		t.Fatal("UpdateRuntimeRiskConfig should fail when state cannot be persisted")
	}
	if got := plane.Snapshot(); got.EffectiveMaxOrderQuantity == nil || *got.EffectiveMaxOrderQuantity != *beforeRuntimeRisk.EffectiveMaxOrderQuantity {
		t.Fatalf("failed runtime-risk update changed active limit: before=%#v after=%#v", beforeRuntimeRisk, got)
	}
	if _, err := plane.DisableRuntimeRiskConfig(t.Context(), RealTradeRuntimeRiskCommand{}); err == nil {
		t.Fatal("DisableRuntimeRiskConfig should fail when state cannot be persisted")
	}
	if got := plane.Snapshot(); !got.RuntimeRiskConfigured {
		t.Fatalf("failed runtime-risk disable removed active config: %#v", got)
	}
}

func TestRealTradeControlPlaneFailsClosedWhenPersistedStateCannotLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "real-trade-control.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plane, err := NewRealTradeControlPlane(path)
	if err == nil || plane == nil {
		t.Fatalf("NewRealTradeControlPlane = (%#v, %v), want usable fail-closed plane and error", plane, err)
	}
	price := 10.0
	decision := plane.EvaluatePlaceOrder(t.Context(), ExecutionOrderCommand{Query: broker.PlaceOrderQuery{
		ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL"},
		Quantity:  1,
		Price:     &price,
	}})
	if decision.ReasonCode != "REAL_TRADE_KILL_SWITCH_ACTIVE" || decision.Allows() {
		t.Fatalf("fail-closed decision = %#v", decision)
	}
	if snapshot := plane.Snapshot(); snapshot.ControlPlaneAvailable || snapshot.ControlPlaneError == nil {
		t.Fatalf("fail-closed snapshot = %#v", snapshot)
	}
	if _, err := plane.ReleaseKillSwitch(t.Context(), RealTradeKillSwitchCommand{}); err == nil {
		t.Fatal("unavailable control plane should reject mutations")
	}
}

func TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions(t *testing.T) {
	service := newExecutionTestService()

	command, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Env:       "real",
		Market:    "US",
		Symbol:    "AAPL",
		Side:      "BUY",
		OrderType: "MARKET",
		Session:   "OVERNIGHT",
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("normalizeExecutionOrder: %v", err)
	}
	if command.Query.TradingEnvironment != "REAL" || command.Query.Session == nil || *command.Query.Session != "OVERNIGHT" {
		t.Fatalf("query = %#v", command.Query)
	}
	if command.Query.FillOutsideRTH != nil {
		t.Fatalf("FillOutsideRTH = %#v, want nil for non-limit order", command.Query.FillOutsideRTH)
	}
}

func TestExecutionNormalizationHelpersRejectUnsupportedInputs(t *testing.T) {
	if _, err := normalizeExecutionOrderType("iceberg"); !IsRequestError(err) {
		t.Fatalf("normalizeExecutionOrderType error = %v, want RequestError", err)
	}
	if _, _, err := normalizeExecutionSession("US", "LIMIT", "pre-open"); !IsRequestError(err) {
		t.Fatalf("normalizeExecutionSession error = %v, want RequestError", err)
	}
}

func TestNormalizeExecutionOrderRejectsInvalidInstrumentAndUnsupportedOrderType(t *testing.T) {
	service := newExecutionTestService()
	price := 10.0

	if _, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Side: "BUY", Quantity: 1, Price: &price,
	}); !IsRequestError(err) || !strings.Contains(err.Error(), "symbol or code is required") {
		t.Fatalf("invalid instrument error = %v", err)
	}

	if _, err := service.normalizeExecutionOrder(ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", OrderType: "ICEBERG", Quantity: 1, Price: &price,
	}); !IsRequestError(err) || !strings.Contains(err.Error(), "unsupported orderType") {
		t.Fatalf("unsupported orderType error = %v", err)
	}
}

func TestExecutionOrderDetailsReturnsOrderAndBoundedRecentEvents(t *testing.T) {
	events := make([]ExecutionOrderEvent, 12)
	for index := range events {
		events[index] = ExecutionOrderEvent{ID: fmt.Sprintf("evt-%02d", index+1), InternalOrderID: "exec-1"}
	}
	service := NewService(
		WithListOrders(func(context.Context, ExecutionOrderFilter) (ExecutionOrders, error) {
			return ExecutionOrders{Orders: []ExecutionOrder{{InternalOrderID: "exec-1", Status: OrderStatusBrokerAccepted}}}, nil
		}),
		WithGetOrderEvents(func(context.Context, string) (ExecutionOrderEvents, error) {
			return ExecutionOrderEvents{InternalOrderID: "exec-1", Events: events}, nil
		}),
	)
	details, err := service.ExecutionOrderDetails(t.Context(), " exec-1 ")
	if err != nil {
		t.Fatalf("ExecutionOrderDetails: %v", err)
	}
	if details.Order.InternalOrderID != "exec-1" || details.Order.Status != OrderStatusBrokerAccepted {
		t.Fatalf("details order = %#v", details.Order)
	}
	if len(details.RecentEvents) != 10 || details.RecentEvents[0].ID != "evt-03" || details.RecentEvents[9].ID != "evt-12" {
		t.Fatalf("recent events = %#v", details.RecentEvents)
	}
	if details.CheckedAt == "" {
		t.Fatal("checkedAt is empty")
	}

	_, err = service.ExecutionOrderDetails(t.Context(), "missing")
	if !errors.Is(err, ErrExecutionOrderNotFound) {
		t.Fatalf("missing order error = %v", err)
	}
}

func TestExecutionOrderDetailsRefreshesTargetHistoryBeforeReturning(t *testing.T) {
	store := &executionDetailsRefreshStore{
		order: ExecutionOrder{
			InternalOrderID:    "exec-refresh",
			BrokerID:           "futu",
			BrokerOrderID:      new("broker-refresh"),
			TradingEnvironment: "SIMULATE",
			AccountID:          "ACC-1",
			Market:             "US",
			Status:             OrderStatusBrokerAccepted,
		},
	}
	source := &fakeOrderUpdateSource{
		history: []Order{{
			AccountID:          "ACC-1",
			TradingEnvironment: "SIMULATE",
			Market:             "US",
			BrokerOrderID:      "broker-refresh",
			Status:             "FILLED_ALL",
			UpdatedAt:          "2026-07-04T01:02:03Z",
		}},
	}
	worker := NewOrderUpdatesWorker(source, store, OrderUpdatesConfig{
		Now: func() time.Time { return time.Date(2026, 7, 4, 1, 2, 3, 0, time.UTC) },
	})
	service := NewService(WithOrderStore(store), WithOrderUpdates(worker))

	details, err := service.ExecutionOrderDetails(t.Context(), "exec-refresh")
	if err != nil {
		t.Fatalf("ExecutionOrderDetails: %v", err)
	}
	if source.historyCalls != 1 {
		t.Fatalf("history calls = %d, want 1", source.historyCalls)
	}
	if len(source.historyQueries) != 1 {
		t.Fatalf("history queries = %#v", source.historyQueries)
	}
	if got := source.historyQueries[0]; got.BrokerID != "futu" || got.TradingEnvironment != "SIMULATE" || got.AccountID != "ACC-1" || got.Market != "US" {
		t.Fatalf("history query = %#v", got)
	}
	if details.Order.Status != OrderStatusFilled || details.Order.RawBrokerStatus == nil || *details.Order.RawBrokerStatus != "FILLED_ALL" {
		t.Fatalf("details order = %#v", details.Order)
	}
}

type executionDetailsRefreshStore struct {
	order  ExecutionOrder
	events []ExecutionOrderEvent
}

func (s *executionDetailsRefreshStore) ListOrders(context.Context, ExecutionOrderFilter) (ExecutionOrders, error) {
	return ExecutionOrders{Orders: []ExecutionOrder{s.order}}, nil
}

func (s *executionDetailsRefreshStore) OrderEvents(context.Context, string) (ExecutionOrderEvents, error) {
	return ExecutionOrderEvents{InternalOrderID: s.order.InternalOrderID, Events: append([]ExecutionOrderEvent(nil), s.events...)}, nil
}

func (s *executionDetailsRefreshStore) ApplyOrder(_ context.Context, _ string, order Order, _ OrderWriteMetadata) {
	if s.order.BrokerOrderID == nil || strings.TrimSpace(*s.order.BrokerOrderID) != strings.TrimSpace(order.BrokerOrderID) {
		return
	}
	rawStatus := strings.TrimSpace(order.Status)
	incomingStatus := CanonicalBrokerOrderStatus(rawStatus)
	if reconciled, accepted := ReconcileCanonicalOrderStatus(s.order.Status, incomingStatus); accepted {
		s.order.Status = reconciled
		s.order.RawBrokerStatus = executionStringPointer(rawStatus)
	}
	if strings.TrimSpace(order.UpdatedAt) != "" {
		s.order.UpdatedAt = order.UpdatedAt
	}
}

func (s *executionDetailsRefreshStore) ApplyFill(context.Context, string, Fill) {}
