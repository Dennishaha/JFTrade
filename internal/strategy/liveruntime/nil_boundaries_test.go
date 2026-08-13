package liveruntime

import (
	"context"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestNilRuntimeBoundariesReturnEmptyState(t *testing.T) {
	var manager *Manager
	manager.SetPineWorkerRunner(nil)
	if manager.currentPineWorkerRunner() != nil {
		t.Fatal("nil manager returned a Pine worker")
	}
	var runtime *symbolRuntime
	if runtime.brokerPositionsSnapshot() != nil || runtime.brokerAccountSnapshot() != nil {
		t.Fatal("nil strategy runtime returned account state")
	}
	if _, ok := manager.currentInstance("missing"); ok {
		t.Fatal("nil manager returned an instance")
	}
	if err := manager.appendRuntimeEvent("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.transitionInstance("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.reconcileRuntimeFailure("", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil manager placement error = nil")
	}
	if _, err := manager.cancelExecutionOrder(t.Context(), ""); err == nil {
		t.Fatal("nil manager cancellation error = nil")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("nil manager close error = %v", err)
	}
}

func TestTradeCommandFuncsRequireCallbacksAndDelegateCommands(t *testing.T) {
	empty := TradeCommandFuncs{}
	if _, err := empty.PlaceExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("empty place callback error = nil")
	}
	if _, err := empty.CancelExecutionOrder(t.Context(), "order-1"); err == nil {
		t.Fatal("empty cancel callback error = nil")
	}
	commands := TradeCommandFuncs{
		Place: func(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
			return trdsrv.ExecutionOrder{InternalOrderID: "order-1"}, nil
		},
		Cancel: func(context.Context, string) (trdsrv.ExecutionOrder, error) {
			return trdsrv.ExecutionOrder{InternalOrderID: "order-1"}, nil
		},
	}
	if order, err := commands.PlaceExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err != nil || order.InternalOrderID != "order-1" {
		t.Fatalf("delegated place = %+v, %v", order, err)
	}
	if order, err := commands.CancelExecutionOrder(t.Context(), "order-1"); err != nil || order.InternalOrderID != "order-1" {
		t.Fatalf("delegated cancel = %+v, %v", order, err)
	}
}
