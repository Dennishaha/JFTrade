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

func TestRuntimeSnapshotIdentityAndAccountHelpers(t *testing.T) {
	if got := strategyRuntimeBrokerPlaceOrderQuery(stratsrv.InstanceBinding{}, "US.AAPL"); got.Market != "US" {
		t.Fatalf("fallback order market = %#v", got)
	}
	if got := strategyRuntimeDisplayName(stratsrv.ManagedInstance{ID: "instance-only"}, nil); got != "instance-only" {
		t.Fatalf("instance display name = %q", got)
	}
	trade := bbgotypes.Trade{
		ID: 1, Side: bbgotypes.SideTypeBuy,
		Price: fixedpoint.NewFromFloat(10), Quantity: fixedpoint.NewFromFloat(2),
	}
	kline := strategyRuntimeTradeKLine(
		"test",
		"US.AAPL",
		bbgotypes.Interval1m,
		trade,
		time.Now(),
		time.Now().Add(time.Minute),
	)
	if kline.QuoteVolume.Float64() != 20 || kline.LastTradeID != 1 {
		t.Fatalf("trade kline = %#v", kline)
	}
	if cloneStrategyRuntimeFundsSnapshot(nil) != nil {
		t.Fatal("nil funds clone was non-nil")
	}
	currency := "USD"
	available := 100.0
	account := buildStrategyRuntimeAccount(
		&broker.FundsSnapshot{Currency: &currency, AvailableFunds: &available},
		nil,
		bbgotypes.Market{},
		"US.AAPL",
	)
	if _, ok := account.Balance("USD"); !ok {
		t.Fatalf("fallback currency balance missing: %#v", account)
	}
}

func TestRuntimeRejectsBeforeAnyBrokerSubmission(t *testing.T) {
	var kinds []string
	manager := &Manager{
		runtimes: map[string]*managedRuntime{},
		deps: Dependencies{
			AppendRuntimeEvent: func(_ string, _ string, kind string, _ string) error {
				kinds = append(kinds, kind)
				return nil
			},
			UpsertObservation: func(
				context.Context,
				runtimeactivity.ObservationSnapshot,
			) error {
				return errors.New("persistence degraded")
			},
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: stratsrv.ManagedInstance{
			ID: "risk-instance",
			Binding: stratsrv.InstanceBinding{RuntimeRisk: stratsrv.RuntimeRiskSettings{
				Mode: "enforce", CloseOnly: true,
			}},
		},
		runner: &symbolRuntime{lastClosedPrice: 10},
	}
	orders, err := executor.SubmitOrders(t.Context(), bbgotypes.SubmitOrder{
		Symbol: "US.AAPL", Side: bbgotypes.SideTypeBuy, Type: bbgotypes.OrderTypeLimit,
		Quantity: fixedpoint.NewFromFloat(1), Price: fixedpoint.NewFromFloat(10),
	})
	if err == nil || !strings.Contains(err.Error(), "runtime risk rejected") || len(orders) != 0 {
		t.Fatalf("risk-rejected submission = %#v, %v", orders, err)
	}
	if len(kinds) != 1 || kinds[0] != "risk_rejected" {
		t.Fatalf("risk lifecycle events = %#v", kinds)
	}
	manager.persistObservationSnapshot(runtimeactivity.ObservationSnapshot{InstanceID: "risk-instance"})
}

func TestRuntimePropagatesGatewayFailureAndSortsObservations(t *testing.T) {
	gatewayErr := errors.New("gateway failed")
	var kinds []string
	manager := &Manager{
		runtimes: map[string]*managedRuntime{
			"z-runtime": {
				instanceID: "z-runtime",
				symbols:    map[string]*symbolRuntime{},
			},
			"a-runtime": {
				instanceID: "a-runtime",
				symbols:    map[string]*symbolRuntime{},
			},
		},
		deps: Dependencies{
			TradeCommands: TradeCommandFuncs{
				Place: func(
					context.Context,
					trdsrv.ExecutionOrderCommand,
				) (trdsrv.ExecutionOrder, error) {
					return trdsrv.ExecutionOrder{}, gatewayErr
				},
			},
			AppendRuntimeEvent: func(_ string, _ string, kind string, _ string) error {
				kinds = append(kinds, kind)
				return nil
			},
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: stratsrv.ManagedInstance{
			ID:      "gateway-instance",
			Binding: stratsrv.InstanceBinding{RuntimeRisk: stratsrv.RuntimeRiskSettings{Mode: "off"}},
		},
		runner: &symbolRuntime{lastClosedPrice: 10},
	}
	orders, err := executor.SubmitOrders(t.Context(), bbgotypes.SubmitOrder{
		Symbol: "US.AAPL", Side: bbgotypes.SideTypeBuy, Type: bbgotypes.OrderTypeLimit,
		Quantity: fixedpoint.NewFromFloat(1), Price: fixedpoint.NewFromFloat(10),
	})
	if !errors.Is(err, gatewayErr) || len(orders) != 0 {
		t.Fatalf("gateway-failed submission = %#v, %v", orders, err)
	}
	if len(kinds) != 1 || kinds[0] != "order_submit_failed" {
		t.Fatalf("gateway lifecycle events = %#v", kinds)
	}
	summary := manager.RuntimeSummary()
	if len(summary.ActiveInstances) != 2 ||
		summary.ActiveInstances[0].InstanceID != "a-runtime" ||
		summary.ActiveInstances[1].InstanceID != "z-runtime" {
		t.Fatalf("sorted runtime summary = %#v", summary)
	}
}

func TestRuntimeAccountAcceptsBlankCurrencyBalance(t *testing.T) {
	account := buildStrategyRuntimeAccount(
		&broker.FundsSnapshot{CurrencyBalances: []broker.CurrencyBalanceSnapshot{
			{Currency: " "},
		}},
		nil,
		bbgotypes.Market{},
		"US.AAPL",
	)
	if account == nil {
		t.Fatal("account with blank currency was nil")
	}
}
