package servercore

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestStrategyRuntimeAdapterRejectsUnsupportedLiveSemantics(t *testing.T) {
	adapter := &strategyRuntimeManagerAdapter{}
	err := adapter.Start(t.Context(), stratsrv.ManagedInstance{
		Binding: stratsrv.InstanceBinding{ExecutionMode: strategyExecutionModeLive},
		Params: map[string]any{
			"script": `//@version=6
strategy("Live", default_qty_type=strategy.percent_of_equity)
strategy.entry("Long", strategy.long)`,
		},
	})
	if !errors.Is(err, stratsrv.ErrBadRequest) || !strings.Contains(err.Error(), "percent_of_equity") {
		t.Fatalf("Start error = %v, want live semantic bad request", err)
	}
}

func TestStrategyRuntimeManagerUsesExplicitDependenciesInsteadOfServerOwnership(t *testing.T) {
	managerType := reflect.TypeFor[strategyRuntimeManager]()
	if _, exists := managerType.FieldByName("server"); exists {
		t.Fatalf("strategyRuntimeManager must not regain direct Server ownership")
	}
	if _, exists := reflect.TypeFor[strategyNotifyOnlyOrderExecutor]().FieldByName("server"); exists {
		t.Fatalf("notify-only order executor must not regain direct Server ownership")
	}
	if _, exists := reflect.TypeFor[strategyLiveOrderExecutor]().FieldByName("server"); exists {
		t.Fatalf("live order executor must not regain direct Server ownership")
	}

	depsType := reflect.TypeFor[strategyRuntimeManagerDeps]()
	transition, ok := depsType.FieldByName("transitionInstance")
	if !ok || transition.Type.NumOut() != 1 || !transition.Type.Out(0).Implements(reflect.TypeFor[error]()) {
		t.Fatalf("transitionInstance should expose only error, got %v", transition.Type)
	}
	countAudit, ok := depsType.FieldByName("countRuntimeAudit")
	if !ok || countAudit.Type.In(1) != reflect.TypeFor[runtimeactivity.AuditQuery]() {
		t.Fatalf("countRuntimeAudit should use runtimeactivity.AuditQuery, got %v", countAudit.Type)
	}
	upsertObservation, ok := depsType.FieldByName("upsertObservation")
	if !ok || upsertObservation.Type.In(1) != reflect.TypeFor[runtimeactivity.ObservationSnapshot]() {
		t.Fatalf("upsertObservation should use runtimeactivity.ObservationSnapshot, got %v", upsertObservation.Type)
	}
}

func TestStrategyRuntimeManagerStartValidationAndReservationBoundaries(t *testing.T) {
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{
			"running": {symbols: map[string]*strategySymbolRuntime{"US.AAPL": {}, "US.MSFT": {}}},
			"second":  {symbols: map[string]*strategySymbolRuntime{"US.AAPL": {}, "HK.00700": {}}},
		},
	}

	instruments := manager.activeInstrumentIDs()
	if got := strings.Join(instruments, ","); got != "HK.00700,US.AAPL,US.MSFT" {
		t.Fatalf("activeInstrumentIDs = %s", got)
	}

	err := manager.startStrategy(context.Background(), managedStrategyInstance{
		ID:      "bad-interval",
		Binding: strategyInstanceBinding{Interval: "bad", Symbols: []string{"US.AAPL"}},
		Params:  map[string]any{"script": "strategy.entry(\"Long\", strategy.long)"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid interval startStrategy error = %v", err)
	}

	err = manager.startStrategy(context.Background(), managedStrategyInstance{
		ID:      "no-symbols",
		Binding: strategyInstanceBinding{Interval: "1m"},
		Params:  map[string]any{"script": "strategy.entry(\"Long\", strategy.long)"},
	})
	if err == nil || !strings.Contains(err.Error(), "at least one symbol") {
		t.Fatalf("empty symbols startStrategy error = %v", err)
	}

	err = manager.startStrategy(context.Background(), managedStrategyInstance{
		ID:      "missing-script",
		Binding: strategyInstanceBinding{Interval: "1m", Symbols: []string{"US.AAPL"}},
		Params:  map[string]any{"script": "  "},
	})
	if err == nil || !strings.Contains(err.Error(), "missing script") {
		t.Fatalf("missing script startStrategy error = %v", err)
	}

	err = manager.startStrategy(context.Background(), managedStrategyInstance{
		ID:      "running",
		Binding: strategyInstanceBinding{Interval: "1m", Symbols: []string{"US.AAPL"}},
		Params:  map[string]any{"script": "strategy.entry(\"Long\", strategy.long)"},
	})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("duplicate startStrategy error = %v", err)
	}

	reservationManager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
	}
	release, err := reservationManager.reserveRuntimeStart("candidate")
	if err != nil {
		t.Fatalf("reserveRuntimeStart: %v", err)
	}
	if _, exists := reservationManager.starting["candidate"]; !exists {
		t.Fatalf("expected reservation to create starting entry")
	}
	if _, err := reservationManager.reserveRuntimeStart("candidate"); err == nil || !strings.Contains(err.Error(), "already starting") {
		t.Fatalf("duplicate reservation error = %v", err)
	}
	release()
	if _, exists := reservationManager.starting["candidate"]; exists {
		t.Fatalf("expected reservation release to clear starting entry")
	}
}

func TestStrategySymbolRuntimeTradeBucketsAndOrderSignals(t *testing.T) {
	stream := bbgotypes.NewStandardStream()
	var closedAt []time.Time
	runner := &strategySymbolRuntime{
		instanceID:    "inst-runtime",
		name:          "  Runner Name  ",
		symbol:        "US.AAPL",
		interval:      bbgotypes.Interval1m,
		exchange:      bbgotypes.ExchangeName("futu"),
		emitter:       &stream,
		onClosedKLine: func(at time.Time) { closedAt = append(closedAt, at) },
	}

	runner.handleRuntimeError(nil)
	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 0, strategyRuntimeTestTime(10, 0, 10)))
	if runner.currentBucket != nil {
		t.Fatalf("nonpositive trade should not open a runtime bucket")
	}

	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 10)))
	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 0, 40)))
	if got := runner.currentPrice(); got != 101 {
		t.Fatalf("currentPrice after same bucket merge = %v, want 101", got)
	}

	runner.handleTrade(strategyRuntimeTestTrade("US.AAPL", 102, strategyRuntimeTestTime(10, 1, 0)))
	if len(closedAt) != 1 {
		t.Fatalf("expected one closed kline after bucket rollover, got %d", len(closedAt))
	}
	if got := runner.lastClosedPrice; got != 101 {
		t.Fatalf("lastClosedPrice after rollover = %v, want 101", got)
	}

	runner.setCurrentBucket(&bbgotypes.KLine{Close: fixedpoint.NewFromFloat(103)})
	if got := runner.currentPrice(); got != 103 {
		t.Fatalf("currentPrice after setCurrentBucket = %v, want 103", got)
	}
	if got := (*strategySymbolRuntime)(nil).context(); got == nil {
		t.Fatalf("nil runtime context should fall back to background context")
	}

	notifyExecutor := &strategyNotifyOnlyOrderExecutor{
		instance: managedStrategyInstance{Definition: strategyDefinitionSummary{Name: "  Runtime Strategy  "}},
		runner:   runner,
	}
	message := notifyExecutor.describeOrderSignal(bbgotypes.SubmitOrder{
		Symbol:   "US.AAPL",
		Side:     bbgotypes.SideTypeSell,
		Quantity: fixedpoint.NewFromFloat(3),
		Price:    fixedpoint.NewFromFloat(104.25),
	})
	if !strings.Contains(message, "Runtime Strategy / US.AAPL: 卖出 3 股") ||
		!strings.Contains(message, "预备下单价格 104.25") ||
		!strings.Contains(message, "当时市价 103") {
		t.Fatalf("unexpected notify-only message: %s", message)
	}
	if err := notifyExecutor.CancelOrders(context.Background()); err != nil {
		t.Fatalf("notify-only CancelOrders: %v", err)
	}
	if err := (&strategyLiveOrderExecutor{}).CancelOrders(context.Background()); err != nil {
		t.Fatalf("live CancelOrders: %v", err)
	}
}

func TestStrategyRuntimeAccountAndFormattingBoundaries(t *testing.T) {
	if got := strategyRuntimeBrokerID(strategyInstanceBinding{}); got != "futu" {
		t.Fatalf("default broker id = %q, want futu", got)
	}
	if got := strategyRuntimeBrokerID(strategyInstanceBinding{BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "  Futu-Sim  "}}); got != "futu-sim" {
		t.Fatalf("normalized broker id = %q", got)
	}
	if got := strategyRuntimeDisplayName(managedStrategyInstance{
		ID:         "inst-id",
		Definition: strategyDefinitionSummary{StrategyID: "definition-id"},
	}, nil); got != "definition-id" {
		t.Fatalf("display name = %q, want definition-id", got)
	}
	if got := strategyRuntimeDisplayName(managedStrategyInstance{ID: "inst-id"}, &strategySymbolRuntime{name: " runner-name "}); got != "runner-name" {
		t.Fatalf("runner display name = %q, want runner-name", got)
	}
	if got := strategyRuntimeSideLabel(bbgotypes.SideTypeSell); got != "卖出" {
		t.Fatalf("sell label = %q", got)
	}
	if got := strategyRuntimeFormatPrice(0); got != "-" {
		t.Fatalf("zero price = %q, want -", got)
	}
	if got := strategyRuntimeFormatNumber(math.Copysign(0, -1)); got != "0" {
		t.Fatalf("negative zero format = %q, want 0", got)
	}

	if !strategyRuntimePositionMatchesSymbol(broker.PositionSnapshot{Symbol: "AAPL"}, "US.AAPL") {
		t.Fatalf("expected bare position symbol to match strategy market-qualified symbol")
	}
	if !strategyRuntimePositionMatchesSymbol(broker.PositionSnapshot{Market: "US", Symbol: "AAPL"}, "US:AAPL") {
		t.Fatalf("expected market+symbol position to match colon-qualified strategy symbol")
	}
	if strategyRuntimePositionMatchesSymbol(broker.PositionSnapshot{Market: "HK", Symbol: "00700"}, "US.AAPL") {
		t.Fatalf("unexpected cross-market position match")
	}

	availableFunds := 1000.0
	maxWithdrawal := 600.0
	totalAssets := 1500.0
	account := buildStrategyRuntimeAccount(&broker.FundsSnapshot{
		Currency:       new(string),
		TotalAssets:    &totalAssets,
		AvailableFunds: &availableFunds,
		MaxWithdrawal:  &maxWithdrawal,
	}, []broker.PositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         5,
		SellableQuantity: 3,
	}, {
		Market:           "HK",
		Symbol:           "00700",
		Quantity:         10,
		SellableQuantity: 10,
	}}, bbgotypes.Market{BaseCurrency: "AAPL", QuoteCurrency: "USD"}, "US.AAPL")

	usd, ok := account.Balance("USD")
	if !ok || usd.Available.Float64() != 1000 || usd.MaxWithdrawAmount.Float64() != 600 {
		t.Fatalf("unexpected USD balance: %+v ok=%v", usd, ok)
	}
	aapl, ok := account.Balance("AAPL")
	if !ok || aapl.Available.Float64() != 3 || aapl.Locked.Float64() != 2 || aapl.NetAsset.Float64() != 5 {
		t.Fatalf("unexpected AAPL balance: %+v ok=%v", aapl, ok)
	}

	cash := 250.0
	withBalances := buildStrategyRuntimeAccount(&broker.FundsSnapshot{
		CurrencyBalances: []broker.CurrencyBalanceSnapshot{{
			Currency: " hkd ",
			Cash:     &cash,
		}},
	}, nil, bbgotypes.Market{BaseCurrency: "00700", QuoteCurrency: "HKD"}, "HK.00700")
	hkd, ok := withBalances.Balance("HKD")
	if !ok || hkd.Available.Float64() != 250 {
		t.Fatalf("unexpected HKD balance: %+v ok=%v", hkd, ok)
	}
}

func TestStrategyRuntimeRefreshAndSyncErrorBoundaries(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	stream := bbgotypes.NewStandardStream()
	session := newStrategyRuntimeTestSession(stub)
	runner := &strategySymbolRuntime{
		symbol:          "US.AAPL",
		interval:        bbgotypes.Interval1m,
		ctx:             nil,
		runtimeExchange: stub,
		market:          bbgotypes.Market{Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		cachedFunds:     stub.funds,
		session:         session,
		emitter:         &stream,
	}

	stub.queryFundsErr = errors.New("funds permission denied")
	if err := runner.refreshBrokerAccount(); err == nil || !strings.Contains(err.Error(), "funds permission denied") {
		t.Fatalf("refreshBrokerAccount funds error = %v", err)
	}
	stub.queryFundsErr = nil
	stub.queryPositionsErr = errors.New("positions permission denied")
	if err := runner.refreshBrokerAccount(); err == nil || !strings.Contains(err.Error(), "positions permission denied") {
		t.Fatalf("refreshBrokerAccount positions error = %v", err)
	}

	var runtimeErrors []string
	stub.queryPositionsErr = nil
	runner.onError = func(message string) { runtimeErrors = append(runtimeErrors, message) }
	stub.history["US.AAPL"] = append(stub.history["US.AAPL"],
		strategyRuntimeHistoricalKLine("US.AAPL", bbgotypes.Interval1m, 101, strategyRuntimeTestTime(10, 0, 0)),
		bbgotypes.KLine{
			Symbol:    "US.AAPL",
			Interval:  bbgotypes.Interval1m,
			StartTime: bbgotypes.Time(strategyRuntimeTestTime(10, 1, 0)),
			EndTime:   bbgotypes.Time(strategyRuntimeTestTime(10, 2, 0).Add(-time.Millisecond)),
			Close:     fixedpoint.NewFromFloat(102),
			Closed:    false,
		},
	)
	strategyRuntimeClosedKLineSyncLimit = 0
	t.Cleanup(func() { strategyRuntimeClosedKLineSyncLimit = 8 })
	runner.syncClosedKLines()
	if len(runtimeErrors) != 0 {
		t.Fatalf("syncClosedKLines should not record errors for valid refresh, got %+v", runtimeErrors)
	}
	if got := runner.currentPrice(); got != 102 {
		t.Fatalf("currentPrice after open kline sync = %v, want 102", got)
	}

	stub.queryFundsErr = errors.New("funds refresh failed")
	stub.appendHistory("US.AAPL", strategyRuntimeHistoricalKLine("US.AAPL", bbgotypes.Interval1m, 103, strategyRuntimeTestTime(10, 2, 0)))
	runner.syncClosedKLines()
	if len(runtimeErrors) == 0 || !strings.Contains(runtimeErrors[len(runtimeErrors)-1], "funds refresh failed") {
		t.Fatalf("expected refresh error from closed kline emission, got %+v", runtimeErrors)
	}
}

func newStrategyRuntimeTestSession(exchange strategyRuntimeExchange) *bbgo.ExchangeSession {
	session := bbgo.NewExchangeSession("strategy-runtime-test", exchange)
	session.SetMarkets(bbgotypes.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})
	return session
}
