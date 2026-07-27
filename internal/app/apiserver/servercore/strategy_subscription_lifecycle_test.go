package servercore

import (
	"context"
	"path/filepath"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
)

func TestStrategyRuntimeHoldsExactKLineLeasesUntilStopAndClose(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.runtimes.StrategyRuntime().SetExchangeProvider(func() liveruntime.Exchange { return newStrategyRuntimeStubExchange() })
	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols: []string{"US.AAPL", "HK.00700"}, Interval: "5m", ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instance, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instance); err != nil {
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

	server.runtimes.StrategyRuntime().Stop(instanceID)
	server.runtimes.StrategyRuntime().Stop(instanceID)
	if snapshot, _ = server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("stop did not release leases: %#v", snapshot)
	}

	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instance); err != nil {
		t.Fatalf("restart strategy: %v", err)
	}
	if err := server.runtimes.StrategyRuntime().Close(); err != nil {
		t.Fatalf("manager close: %v", err)
	}
	if err := server.runtimes.StrategyRuntime().Close(); err != nil {
		t.Fatalf("manager repeated close: %v", err)
	}
	if snapshot, _ = server.marketdataSvc.GetSubscriptions(context.Background()); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("manager close did not release leases: %#v", snapshot)
	}
}
