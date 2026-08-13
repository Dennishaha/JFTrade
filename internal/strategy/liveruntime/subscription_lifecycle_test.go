package liveruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestSubscriptionLeaseAndWarmupFailuresRollBackRuntime(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	instance := subscriptionTestInstance()
	wantErr := errors.New("subscription quota exhausted")
	manager := NewManager(Dependencies{
		ExchangeProvider: func() Exchange { return stub },
		PineWorker:       newFakeStrategyRuntimePineWorker(),
		AcquireMarketDataLease: func(context.Context, string, []mdsrv.InstrumentRef) (SubscriptionLease, error) {
			return nil, wantErr
		},
	})
	if err := manager.Start(t.Context(), instance); err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("lease failure = %v", err)
	}
	if len(manager.runtimes) != 0 {
		t.Fatal("failed lease created a runtime")
	}

	lease := &closeTestLease{}
	manager = NewManager(Dependencies{
		PineWorker: newFakeStrategyRuntimePineWorker(),
		AcquireMarketDataLease: func(context.Context, string, []mdsrv.InstrumentRef) (SubscriptionLease, error) {
			return lease, nil
		},
	})
	_, err := manager.buildManagedStrategyRuntime(
		t.Context(),
		stub,
		stub,
		map[string]bbgotypes.Market{},
		nil,
		nil,
		instance,
		instance.Params["script"].(string),
		bbgotypes.Interval1m,
	)
	if err == nil || !strings.Contains(err.Error(), "market metadata") {
		t.Fatalf("warmup failure = %v", err)
	}
	if got := lease.count.Load(); got != 1 {
		t.Fatalf("warmup failure released lease %d times, want 1", got)
	}
}

func TestRuntimePanicReleasesSubscriptionLease(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	lease := &closeTestLease{}
	manager := NewManager(Dependencies{
		ExchangeProvider: func() Exchange { return stub },
		PineWorker:       newFakeStrategyRuntimePineWorker(),
		AcquireMarketDataLease: func(context.Context, string, []mdsrv.InstrumentRef) (SubscriptionLease, error) {
			return lease, nil
		},
	})
	instance := subscriptionTestInstance()
	if err := manager.Start(t.Context(), instance); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manager.handleRuntimePanic(instance.ID, "US.AAPL", "boom")
	if got := lease.count.Load(); got != 1 {
		t.Fatalf("panic released lease %d times, want 1", got)
	}
	if len(manager.ActiveInstrumentIDs()) != 0 {
		t.Fatal("panic left runtime instruments active")
	}
}

func TestKLineSubscriptionRefsSkipMalformedSymbols(t *testing.T) {
	refs := strategyKLineSubscriptionRefs(
		[]string{" us.aapl ", "bad", ".missing", "HK."},
		bbgotypes.Interval15m,
	)
	if len(refs) != 1 || refs[0] != (mdsrv.InstrumentRef{
		Channel:  "KLINE",
		Market:   "US",
		Symbol:   "AAPL",
		Interval: "15m",
	}) {
		t.Fatalf("strategy refs = %#v", refs)
	}
}

func subscriptionTestInstance() stratsrv.ManagedInstance {
	return stratsrv.ManagedInstance{
		ID: "subscription-instance",
		Definition: stratsrv.DefinitionSummary{
			Name: "Subscription Test",
		},
		Binding: stratsrv.InstanceBinding{
			Symbols:       []string{"US.AAPL"},
			Interval:      "1m",
			ExecutionMode: "notify_only",
			BrokerAccount: &stratsrv.BrokerAccountBinding{
				BrokerID:           "futu",
				AccountID:          "123456",
				TradingEnvironment: "SIMULATE",
				Market:             "US",
			},
		},
		Params: map[string]any{
			"script": "//@version=6\nstrategy(\"Subscription Test\")",
		},
	}
}
