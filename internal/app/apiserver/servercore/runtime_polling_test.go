package servercore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyRuntimePollsClosedKLinesWhenTradePushStalls(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	stub.positions = []broker.PositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         3,
		SellableQuantity: 3,
	}}
	installStrategyRuntimeTestExchange(server, stub)

	originalInterval := strategyRuntimeClosedKLineSyncInterval
	strategyRuntimeClosedKLineSyncInterval = 10 * time.Millisecond
	defer func() {
		strategyRuntimeClosedKLineSyncInterval = originalInterval
	}()

	definition := strategyDesignDefinition{
		ID:           "runtime-poll-kline-test",
		Name:         "Runtime Poll KLine Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Poll KLine Test\", overlay=true)\nstrategy.close(\"Long\")",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.strategyStore.strategy(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instance.ID)

	stub.appendHistory(
		"US.AAPL",
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 100, strategyRuntimeTestTime(10, 0, 0)),
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 101, strategyRuntimeTestTime(10, 1, 0)),
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 102, strategyRuntimeTestTime(10, 2, 0)),
	)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.placedOrderCount() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := stub.placedOrderCount(); got != 3 {
		t.Fatalf("expected 3 broker orders from polled closed klines, got %d", got)
	}
	observation, ok := server.strategyRuntimeManager.runtimeObservation(instance.ID)
	if !ok {
		t.Fatalf("expected runtime observation for %s", instance.ID)
	}
	if observation.LastClosedKLineAt == nil || observation.LastOrderAt == nil {
		t.Fatalf("expected runtime observation to record polled kline/order timestamps, got %+v", observation)
	}
}
