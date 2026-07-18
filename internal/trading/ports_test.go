package trading

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type testOrderStorePort struct {
	listed bool
	events bool
}

func (p *testOrderStorePort) ListOrders(context.Context, ExecutionOrderFilter) (ExecutionOrders, error) {
	p.listed = true
	return ExecutionOrders{Orders: []ExecutionOrder{{InternalOrderID: "exec-port", Status: OrderStatusBrokerAccepted}}}, nil
}

func (p *testOrderStorePort) OrderEvents(context.Context, string) (ExecutionOrderEvents, error) {
	p.events = true
	return ExecutionOrderEvents{InternalOrderID: "exec-port"}, nil
}

type testOrderGatewayPort struct {
	placed    bool
	cancelled bool
}

func (p *testOrderGatewayPort) PlaceOrder(context.Context, ExecutionOrderCommand) (ExecutionOrder, error) {
	p.placed = true
	return ExecutionOrder{InternalOrderID: "exec-port", Status: OrderStatusBrokerAccepted}, nil
}

func (p *testOrderGatewayPort) CancelOrder(context.Context, string) (ExecutionOrder, error) {
	p.cancelled = true
	return ExecutionOrder{InternalOrderID: "exec-port", Status: OrderStatusCancelRequested}, nil
}

type testBrokerRuntimePort struct {
	active broker.Broker
}

func (p testBrokerRuntimePort) ActiveBroker() broker.Broker { return p.active }
func (p testBrokerRuntimePort) Runtime(context.Context) map[string]any {
	return map[string]any{"connectivity": "connected"}
}

func TestServiceUsesExplicitTradingPorts(t *testing.T) {
	store := &testOrderStorePort{}
	gateway := &testOrderGatewayPort{}
	active := &stubBroker{id: "futu"}
	service := NewService(
		WithOrderStore(store),
		WithOrderGateway(gateway),
		WithBrokerRuntimeProvider(testBrokerRuntimePort{active: active}),
	)
	price := 100.0
	placed, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		BrokerID: "futu", Market: "US", Symbol: "AAPL", Side: "BUY",
		OrderType: "LIMIT", Quantity: 1, Price: &price,
	})
	if err != nil || !gateway.placed || placed.InternalOrderID == nil || *placed.InternalOrderID != "exec-port" {
		t.Fatalf("placed=%#v gateway=%#v err=%v", placed, gateway, err)
	}
	if _, err := service.ListExecutionOrders(t.Context(), ExecutionOrderFilter{}, false); err != nil || !store.listed {
		t.Fatalf("ListExecutionOrders listed=%v err=%v", store.listed, err)
	}
	if _, err := service.ExecutionOrderEvents(t.Context(), "exec-port"); err != nil || !store.events {
		t.Fatalf("ExecutionOrderEvents events=%v err=%v", store.events, err)
	}
	if _, err := service.CancelExecutionOrder(t.Context(), "exec-port"); err != nil || !gateway.cancelled {
		t.Fatalf("CancelExecutionOrder cancelled=%v err=%v", gateway.cancelled, err)
	}
	runtime, err := service.Runtime(t.Context(), "futu")
	if err != nil || runtime["connectivity"] != "connected" {
		t.Fatalf("Runtime=%#v err=%v", runtime, err)
	}
}

func TestServiceDefaultTradingPortsFailExplicitly(t *testing.T) {
	service := newExecutionTestService()
	if _, err := service.ListExecutionOrders(t.Context(), ExecutionOrderFilter{}, false); !errors.Is(err, ErrOrderStoreUnavailable) {
		t.Fatalf("ListExecutionOrders error = %v", err)
	}
	price := 100.0
	_, err := service.CreateExecutionOrder(t.Context(), ExecutionPlaceRequest{
		Market: "US", Symbol: "AAPL", Side: "BUY", OrderType: "LIMIT", Quantity: 1, Price: &price,
	})
	if !errors.Is(err, ErrOrderGatewayUnavailable) {
		t.Fatalf("CreateExecutionOrder error = %v", err)
	}
}
