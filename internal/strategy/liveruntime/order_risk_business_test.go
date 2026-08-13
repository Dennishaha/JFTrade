package liveruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestLiveOrderPassesStopPriceToExecutionGateway(t *testing.T) {
	var captured trdsrv.ExecutionOrderCommand
	manager := NewManager(Dependencies{
		TradeCommands: TradeCommandFuncs{
			Place: func(_ context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
				captured = command
				return trdsrv.ExecutionOrder{InternalOrderID: "internal-stop"}, nil
			},
		},
		AppendRuntimeEvent: func(string, string, string, string) error { return nil },
	})
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: stratsrv.ManagedInstance{
			ID: "stop-instance",
			Binding: stratsrv.InstanceBinding{
				RuntimeRisk: stratsrv.RuntimeRiskSettings{Mode: "off"},
			},
		},
		runner: &symbolRuntime{lastClosedPrice: 100},
	}
	stopPrice := fixedpoint.NewFromFloat(95.25)
	orders, err := executor.SubmitOrders(t.Context(), bbgotypes.SubmitOrder{
		ClientOrderID: "stop-order",
		Symbol:        "US.AAPL",
		Side:          bbgotypes.SideTypeSell,
		Type:          bbgotypes.OrderTypeStopMarket,
		Quantity:      fixedpoint.NewFromFloat(1),
		StopPrice:     stopPrice,
		ReduceOnly:    true,
	})
	if err != nil || len(orders) != 1 {
		t.Fatalf("SubmitOrders = %#v, %v", orders, err)
	}
	if captured.OrderType != string(bbgotypes.OrderTypeStopMarket) ||
		captured.Query.OrderType != string(bbgotypes.OrderTypeStopMarket) {
		t.Fatalf("execution order types = %q/%q", captured.OrderType, captured.Query.OrderType)
	}
	if captured.Query.StopPrice == nil || *captured.Query.StopPrice != stopPrice.Float64() {
		t.Fatalf("execution stop price = %#v, want %v", captured.Query.StopPrice, stopPrice.Float64())
	}
	if captured.Query.Price != nil {
		t.Fatalf("stop-market limit price = %#v, want nil", captured.Query.Price)
	}
	if !captured.Query.ReduceOnly {
		t.Fatal("execution reduce-only flag = false, want true")
	}
}

func TestRuntimeRiskEvaluatesOrderLimits(t *testing.T) {
	executor := &strategyLiveOrderExecutor{
		instance: stratsrv.ManagedInstance{
			Binding: stratsrv.InstanceBinding{
				RuntimeRisk: stratsrv.RuntimeRiskSettings{
					Mode:             "enforce",
					CloseOnly:        true,
					MaxOrderQuantity: new(5.0),
					MaxOrderNotional: new(500.0),
				},
			},
		},
		runner: &symbolRuntime{
			lastClosedPrice: 100,
			cachedPositions: []broker.PositionSnapshot{{
				Market:           "US",
				Symbol:           "AAPL",
				Quantity:         4,
				SellableQuantity: 4,
			}},
		},
	}

	tests := []struct {
		name     string
		side     string
		quantity float64
		price    *float64
		want     string
	}{
		{name: "buy blocked by close only", side: "BUY", quantity: 1, want: "close_only"},
		{name: "sell exceeds position", side: "SELL", quantity: 5, want: "close_only_insufficient_position"},
		{name: "sell exceeds quantity", side: "SELL", quantity: 6, want: "close_only_insufficient_position"},
		{name: "sell exceeds notional", side: "SELL", quantity: 4, price: new(float64(130)), want: "max_order_notional"},
		{name: "sell allowed", side: "SELL", quantity: 4, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := executor.evaluateRuntimeRisk(trdsrv.ExecutionOrderCommand{
				Symbol: "US.AAPL",
				Side:   test.side,
				Query: broker.PlaceOrderQuery{
					Symbol:   "US.AAPL",
					Side:     test.side,
					Quantity: test.quantity,
					Price:    test.price,
				},
			})
			if decision.Reason != test.want {
				t.Fatalf("reason = %q, want %q", decision.Reason, test.want)
			}
		})
	}
	executor.instance.Binding.RuntimeRisk.CloseOnly = false
	quantityDecision := executor.evaluateRuntimeRisk(trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Side:   "BUY",
		Query: broker.PlaceOrderQuery{
			Symbol:   "US.AAPL",
			Side:     "BUY",
			Quantity: 6,
		},
	})
	if quantityDecision.Reason != "max_order_quantity" {
		t.Fatalf("quantity decision reason = %q, want max_order_quantity", quantityDecision.Reason)
	}
}

func TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders(t *testing.T) {
	t.Run("tracked orders", func(t *testing.T) {
		cancelled := []string{}
		manager := NewManager(Dependencies{
			TradeCommands: TradeCommandFuncs{
				Cancel: func(_ context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
					cancelled = append(cancelled, internalOrderID)
					return trdsrv.ExecutionOrder{InternalOrderID: internalOrderID}, nil
				},
			},
			AppendRuntimeEvent: func(string, string, string, string) error { return nil },
		})
		executor := &strategyLiveOrderExecutor{manager: manager, instance: stratsrv.ManagedInstance{ID: "instance-a"}}
		executor.trackOrder("owned-1", "internal-1")
		executor.trackOrder("owned-2", "internal-2")

		err := executor.CancelOrders(context.Background(),
			orderForCancel("owned-1"),
			orderForCancel("untracked"),
			orderForCancel("owned-2"),
		)
		if err != nil {
			t.Fatalf("CancelOrders: %v", err)
		}
		if strings.Join(cancelled, ",") != "internal-1,internal-2" {
			t.Fatalf("cancelled = %#v, want only tracked orders", cancelled)
		}
		if _, ok := executor.trackedInternalOrderID("owned-1"); ok {
			t.Fatal("owned-1 remained tracked after successful cancel")
		}
		if _, ok := executor.trackedInternalOrderID("owned-2"); ok {
			t.Fatal("owned-2 remained tracked after successful cancel")
		}
	})

	t.Run("gateway failure preserves tracking", func(t *testing.T) {
		cancelErr := errors.New("cancel failed")
		manager := NewManager(Dependencies{
			TradeCommands: TradeCommandFuncs{
				Cancel: func(context.Context, string) (trdsrv.ExecutionOrder, error) {
					return trdsrv.ExecutionOrder{}, cancelErr
				},
			},
			AppendRuntimeEvent: func(string, string, string, string) error { return nil },
		})
		executor := &strategyLiveOrderExecutor{manager: manager, instance: stratsrv.ManagedInstance{ID: "instance-a"}}
		executor.trackOrder("owned", "internal-1")

		if err := executor.CancelOrders(context.Background(), orderForCancel("owned")); !errors.Is(err, cancelErr) {
			t.Fatalf("CancelOrders error = %v, want %v", err, cancelErr)
		}
		if got, ok := executor.trackedInternalOrderID("owned"); !ok || got != "internal-1" {
			t.Fatalf("tracked order after failed cancel = %q/%v, want preserved", got, ok)
		}
	})

	t.Run("untracked order is ignored", func(t *testing.T) {
		cancelled := false
		manager := NewManager(Dependencies{
			TradeCommands: TradeCommandFuncs{
				Cancel: func(context.Context, string) (trdsrv.ExecutionOrder, error) {
					cancelled = true
					return trdsrv.ExecutionOrder{}, nil
				},
			},
			AppendRuntimeEvent: func(string, string, string, string) error { return nil },
		})
		executor := &strategyLiveOrderExecutor{manager: manager, instance: stratsrv.ManagedInstance{ID: "instance-a"}}
		if err := executor.CancelOrders(context.Background(), orderForCancel("missing")); err != nil {
			t.Fatalf("CancelOrders missing: %v", err)
		}
		if cancelled {
			t.Fatal("missing cancel reached execution gateway")
		}
	})
}

func orderForCancel(clientOrderID string) bbgotypes.Order {
	return bbgotypes.Order{SubmitOrder: bbgotypes.SubmitOrder{ClientOrderID: clientOrderID}}
}

func TestMarketDayStartUsesOrderSymbolTimezone(t *testing.T) {
	now := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got, want := marketDayStartUTC("US.AAPL", now), time.Date(2025, time.December, 31, 5, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US day start = %s, want %s", got, want)
	}
	if got, want := marketDayStartUTC("HK.00700", now), time.Date(2025, time.December, 31, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("HK day start = %s, want %s", got, want)
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	overnight := time.Date(2026, time.June, 14, 20, 30, 0, 0, ny)
	if got, want := marketDayStartUTC("US.AAPL", overnight), time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US overnight day start = %s, want %s", got, want)
	}
}

func TestSubmittedOrderCountKeepsInstanceScopeWithinMarketDay(t *testing.T) {
	instanceID := "multi-market-instance"
	events := []time.Time{
		time.Date(2025, time.December, 31, 6, 0, 0, 0, time.UTC),
		time.Date(2025, time.December, 31, 17, 0, 0, 0, time.UTC),
	}
	manager := NewManager(Dependencies{
		CountRuntimeAudit: func(_ context.Context, query runtimeactivity.AuditQuery) (int, error) {
			if query.InstanceID != instanceID || query.Kind != "order_submitted" || query.FromAt == nil {
				t.Fatalf("unexpected audit query: %+v", query)
			}
			count := 0
			for _, at := range events {
				if !at.Before(*query.FromAt) {
					count++
				}
			}
			return count, nil
		},
	})
	now := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got := manager.todaySubmittedOrderCount(instanceID, "US.AAPL", now); got != 2 {
		t.Fatalf("US market-day instance order count = %d, want 2", got)
	}
	if got := manager.todaySubmittedOrderCount(instanceID, "HK.00700", now); got != 1 {
		t.Fatalf("HK market-day instance order count = %d, want 1", got)
	}
}
