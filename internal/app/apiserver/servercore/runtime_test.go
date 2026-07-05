package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestStrategyRuntimeNotifyOnlyEmitsSignalNotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	notifications := server.liveNotificationsAfter(0)
	if len(notifications) == 0 {
		t.Fatalf("expected strategy signal notification")
	}
	found := false
	for _, note := range notifications {
		if note.Title != "策略下单信号" {
			continue
		}
		found = true
		if !strings.Contains(note.Message, "买入 10 股") || !strings.Contains(note.Message, "仅通知模式") {
			t.Fatalf("unexpected signal message: %+v", note)
		}
	}
	if !found {
		t.Fatalf("expected 策略下单信号 notification, got %+v", notifications)
	}
	if got := len(server.executionOrders.listOrders().Orders); got != 0 {
		t.Fatalf("notify_only should not place execution orders, got %d", got)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	signalAudited := false
	for _, entry := range audit.Entries {
		if entry.Kind == "signal_notified" {
			signalAudited = true
			break
		}
	}
	if !signalAudited {
		t.Fatalf("expected signal_notified audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeStartEnsuresMissingMarketMetadata(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}

	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy should inject missing market metadata: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	observation, ok := server.strategyRuntimeManager.runtimeObservation(instanceID)
	if !ok {
		t.Fatalf("expected runtime observation after start")
	}
	if len(observation.ActiveSymbols) != 1 || observation.ActiveSymbols[0] != "US.TME" {
		t.Fatalf("unexpected active symbols: %+v", observation.ActiveSymbols)
	}
	if _, ok := stub.markets["US.TME"]; !ok {
		t.Fatalf("expected EnsureMarket to inject US.TME into market map")
	}
}

func TestStrategyRuntimeStartRejectsWhenInstanceWorkerLimitReached(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := store.SavePineWorkerSettings(jfsettings.PineWorkerSettings{BacktestWorkerLimit: 2, InstanceWorkerLimit: 1}); err != nil {
		t.Fatalf("SavePineWorkerSettings: %v", err)
	}
	server := newTestServer(t, store)
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return newStrategyRuntimeStubExchange() }
	useFakeStrategyRuntimePineWorker(server, newFakeStrategyRuntimePineWorker())

	firstID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	firstRecord, ok := server.strategyStore.strategy(firstID)
	if !ok {
		t.Fatalf("strategy(%s) not found", firstID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), firstRecord); err != nil {
		t.Fatalf("first startStrategy: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(firstID)

	secondID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.MSFT"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	secondRecord, ok := server.strategyStore.strategy(secondID)
	if !ok {
		t.Fatalf("strategy(%s) not found", secondID)
	}
	err = server.strategyRuntimeManager.startStrategy(context.Background(), secondRecord)
	if !errors.Is(err, pineworker.ErrCapacityExceeded) {
		t.Fatalf("second startStrategy error = %v, want ErrCapacityExceeded", err)
	}
	var capacityErr pineworker.CapacityExceededError
	if !errors.As(err, &capacityErr) || capacityErr.Workers != 1 {
		t.Fatalf("second startStrategy capacity error = %#v, want workers=1", err)
	}
}

func TestStrategyRuntimeLiveWorkerRequestIncludesModeCandlesAndParams(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }
	worker := newFakeStrategyRuntimePineWorker()
	useFakeStrategyRuntimePineWorker(server, worker)

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	instanceRecord.Params["threshold"] = "100"
	instanceRecord.Params["enabled"] = true
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	request, ok := worker.lastRequest()
	if !ok {
		t.Fatal("expected live worker request")
	}
	if request.Mode != "live" || request.Symbol != "US.AAPL" || request.Timeframe != "1m" {
		t.Fatalf("unexpected live worker request routing fields: %+v", request)
	}
	if len(request.Candles) != 2 {
		t.Fatalf("expected warmup + current closed candle, got %d candles", len(request.Candles))
	}
	if request.Params["threshold"] != "100" || request.Params["enabled"] != "true" {
		t.Fatalf("unexpected worker params: %+v", request.Params)
	}
}

func TestStrategyRuntimeLiveWorkerErrorRecordsRuntimeError(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }
	worker := newFakeStrategyRuntimePineWorker()
	worker.err = errors.New("worker crashed")
	useFakeStrategyRuntimePineWorker(server, worker)

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	observation, ok := server.strategyRuntimeManager.runtimeObservation(instanceID)
	if !ok {
		t.Fatalf("expected runtime observation for %s", instanceID)
	}
	if observation.LastError == nil || !strings.Contains(*observation.LastError, "worker crashed") {
		t.Fatalf("expected worker error observation, got %+v", observation)
	}
}
