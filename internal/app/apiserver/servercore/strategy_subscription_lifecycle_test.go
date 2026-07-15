package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestStrategyRuntimeHoldsExactKLineLeasesUntilStopAndClose(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return newStrategyRuntimeStubExchange() }
	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols: []string{"US.AAPL", "HK.00700"}, Interval: "5m", ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instance, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instance); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	snapshot, _ := server.marketdataSvc.GetSubscriptions(context.Background())
	entries := snapshot["entries"].([]map[string]any)
	if len(entries) != 2 || entries[0]["channel"] != "KLINE" || entries[0]["interval"] != "5m" || entries[1]["channel"] != "KLINE" || entries[1]["interval"] != "5m" {
		t.Fatalf("strategy exact subscriptions = %#v", entries)
	}
	if err := server.marketdataSvc.ClearSubscriptions(context.Background()); err != nil {
		t.Fatalf("web-only clear: %v", err)
	}
	if snapshot, _ = server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 2 {
		t.Fatalf("web cleanup removed running strategy leases: %#v", snapshot)
	}

	server.strategyRuntimeManager.stopStrategy(instanceID)
	server.strategyRuntimeManager.stopStrategy(instanceID)
	if snapshot, _ = server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("stop did not release leases: %#v", snapshot)
	}

	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instance); err != nil {
		t.Fatalf("restart strategy: %v", err)
	}
	server.strategyRuntimeManager.close()
	server.strategyRuntimeManager.close()
	if snapshot, _ = server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("manager close did not release leases: %#v", snapshot)
	}
}

func TestStrategyRuntimeLeaseFailureAndWarmupFailureRollBack(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }
	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instance, _ := server.strategyStore.strategy(instanceID)
	wantErr := errors.New("subscription quota exhausted")
	server.strategyRuntimeManager.deps.acquireMarketDataLease = func(context.Context, string, []mdsrv.InstrumentRef) (*mdsrv.ManagedSubscription, error) {
		return nil, wantErr
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instance); !errors.Is(err, wantErr) {
		t.Fatalf("lease failure = %v", err)
	}
	if len(server.strategyRuntimeManager.runtimes) != 0 {
		t.Fatal("failed lease created a runtime")
	}

	server.strategyRuntimeManager.deps.acquireMarketDataLease = func(ctx context.Context, consumerID string, refs []mdsrv.InstrumentRef) (*mdsrv.ManagedSubscription, error) {
		return server.marketdataSvc.AcquireManagedSubscription(ctx, consumerID, refs)
	}
	_, err = server.strategyRuntimeManager.buildManagedStrategyRuntime(
		context.Background(), stub, map[string]bbgotypes.Market{}, nil, nil, instance,
		instance.Params["script"].(string), bbgotypes.Interval1m,
	)
	if err == nil || !strings.Contains(err.Error(), "market metadata") {
		t.Fatalf("warmup failure = %v", err)
	}
	if snapshot, _ := server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("warmup failure retained lease: %#v", snapshot)
	}
}

func TestStrategyRuntimePanicReleasesSubscriptionLease(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return newStrategyRuntimeStubExchange() }
	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instance, _ := server.strategyStore.strategy(instanceID)
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instance); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	server.strategyRuntimeManager.handleRuntimePanic(instanceID, "US.AAPL", "boom")
	if snapshot, _ := server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("panic retained lease: %#v", snapshot)
	}
}

func TestStrategyKLineSubscriptionRefsSkipMalformedSymbols(t *testing.T) {
	refs := strategyKLineSubscriptionRefs([]string{" us.aapl ", "bad", ".missing", "HK."}, bbgotypes.Interval15m)
	if len(refs) != 1 || refs[0] != (mdsrv.InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "15m"}) {
		t.Fatalf("strategy refs = %#v", refs)
	}
}
