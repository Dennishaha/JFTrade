package servercore

import (
	"path/filepath"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyRuntimeAdapterAllowsBrokerExecutedLiveSemantics(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.runtimes.StrategyRuntime().SetExchangeProvider(func() liveruntime.Exchange { return stub })
	definition := stratsrv.Definition{
		ID:           "runtime-live-semantics-test",
		Name:         "Runtime Live Semantics Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script: `//@version=6
strategy("Live", default_qty_type=strategy.percent_of_equity)
strategy.entry("Long", strategy.long, qty_percent=10)
strategy.cancel_all()`,
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(t.Context(), instanceRecord); err != nil {
		t.Fatalf("Start error = %v, want supported live semantics", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)
}
