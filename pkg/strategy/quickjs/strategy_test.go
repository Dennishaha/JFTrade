package quickjs

import (
	"context"
	"strings"
	"testing"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	qjs "modernc.org/quickjs"
)

type stubOrderExecutor struct {
	createdOrders  types.OrderSlice
	lastSubmitted  []types.SubmitOrder
	cancelledOrder []types.Order
}

func (s *stubOrderExecutor) SubmitOrders(_ context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	s.lastSubmitted = append([]types.SubmitOrder(nil), orders...)
	return s.createdOrders, nil
}

func (s *stubOrderExecutor) CancelOrders(_ context.Context, orders ...types.Order) error {
	s.cancelledOrder = append([]types.Order(nil), orders...)
	return nil
}

func TestRuntimeBridgePlaceOrderAndCancelOrder(t *testing.T) {
	session := &bbgo2.ExchangeSession{Account: types.NewAccount()}
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true
	session.SetMarkets(types.MarketMap{
		"BTCUSDT": {
			Symbol:        "BTCUSDT",
			BaseCurrency:  "BTC",
			QuoteCurrency: "USDT",
		},
	})

	executor := &stubOrderExecutor{
		createdOrders: types.OrderSlice{
			{
				SubmitOrder: types.SubmitOrder{
					ClientOrderID: "quickjs-test-order",
					Symbol:        "BTCUSDT",
					Side:          types.SideTypeBuy,
					Type:          types.OrderTypeMarket,
					Quantity:      fixedpoint.NewFromFloat(0.25),
				},
				OrderID: 42,
				Status:  types.OrderStatusNew,
			},
		},
	}

	bridge, err := newRuntimeBridge(context.Background(), &Strategy{
		Name:   "quickjs-test",
		Symbol: "BTCUSDT",
		Script: "function onInit(ctx) { console.log(ctx.symbol); }",
	}, executor, session)
	if err != nil {
		t.Fatalf("newRuntimeBridge() error = %v", err)
	}
	defer bridge.close()

	ackJSON, err := bridge.vm.Eval(`(() => {
		const ack = placeOrder({ clientOrderId: "quickjs-test-order", side: "BUY", quantity: 0.25, orderType: "MARKET" });
		return JSON.stringify([ack.accepted, ack.requestId, ack.orderId, ack.status]);
	})()`, qjs.EvalGlobal)
	if err != nil {
		t.Fatalf("placeOrder() error = %v", err)
	}

	ackText, ok := ackJSON.(string)
	if !ok {
		t.Fatalf("placeOrder() returned %T", ackJSON)
	}
	if ackText != `[true,"quickjs-test-order","42","NEW"]` {
		t.Fatalf("placeOrder() ack = %s", ackText)
	}
	if len(executor.lastSubmitted) != 1 {
		t.Fatalf("SubmitOrders() count = %d", len(executor.lastSubmitted))
	}
	if executor.lastSubmitted[0].Symbol != "BTCUSDT" {
		t.Fatalf("submitted symbol = %s", executor.lastSubmitted[0].Symbol)
	}

	riskJSON, err := bridge.vm.Eval(`JSON.stringify([getRiskState().realTradingEnabled, isOperationBlocked("PLACE"), isOperationBlocked("CANCEL")])`, qjs.EvalGlobal)
	if err != nil {
		t.Fatalf("getRiskState() error = %v", err)
	}
	riskText, ok := riskJSON.(string)
	if !ok {
		t.Fatalf("getRiskState() returned %T", riskJSON)
	}
	if riskText != `[true,false,false]` {
		t.Fatalf("risk state = %s", riskText)
	}

	if _, err := bridge.vm.Eval(`cancelOrder("42")`, qjs.EvalGlobal); err != nil {
		t.Fatalf("cancelOrder() error = %v", err)
	}
	if len(executor.cancelledOrder) != 1 {
		t.Fatalf("CancelOrders() count = %d", len(executor.cancelledOrder))
	}
	if executor.cancelledOrder[0].OrderID != 42 {
		t.Fatalf("cancelled order id = %d", executor.cancelledOrder[0].OrderID)
	}
}

func TestBuildRuntimeRiskSnapshot(t *testing.T) {
	account := types.NewAccount()
	account.AccountType = types.AccountTypeSpot
	account.CanTrade = true
	account.CanDeposit = true
	account.CanWithdraw = true

	snapshot := buildRuntimeRiskSnapshot(false, account, true)
	if !snapshot.RealTradingEnabled {
		t.Fatalf("realTradingEnabled = %v", snapshot.RealTradingEnabled)
	}
	if !snapshot.AllowsCancel {
		t.Fatalf("allowsCancel = %v", snapshot.AllowsCancel)
	}
	if len(snapshot.BlockedOperations) != 0 {
		t.Fatalf("blockedOperations = %v", snapshot.BlockedOperations)
	}

	blockedSnapshot := buildRuntimeRiskSnapshot(true, account, false)
	if len(blockedSnapshot.BlockedOperations) != 3 {
		t.Fatalf("blockedOperations len = %d", len(blockedSnapshot.BlockedOperations))
	}
	if blockedSnapshot.AllowsCancel {
		t.Fatalf("allowsCancel = %v", blockedSnapshot.AllowsCancel)
	}
}

func TestBuildPositionSnapshot(t *testing.T) {
	account := types.NewAccount()
	account.SetBalance("BTC", types.Balance{
		Currency:  "BTC",
		Available: fixedpoint.NewFromFloat(0.75),
		Locked:    fixedpoint.NewFromFloat(0.05),
	})

	position := &types.Position{
		Symbol:        "BTCUSDT",
		BaseCurrency:  "BTC",
		QuoteCurrency: "USDT",
		Base:          fixedpoint.NewFromFloat(0.8),
		AverageCost:   fixedpoint.NewFromFloat(100000),
	}

	snapshot := buildPositionSnapshot(
		"BTCUSDT",
		types.Market{Symbol: "BTCUSDT", BaseCurrency: "BTC", QuoteCurrency: "USDT"},
		position,
		fixedpoint.NewFromFloat(110000),
		account,
	)

	if snapshot.Direction != "LONG" {
		t.Fatalf("direction = %v", snapshot.Direction)
	}
	if snapshot.Quantity != 0.8 {
		t.Fatalf("quantity = %v", snapshot.Quantity)
	}
	if snapshot.AvailableQuantity != 0.75 {
		t.Fatalf("availableQuantity = %v", snapshot.AvailableQuantity)
	}
	if snapshot.MarketValue != float64(88000) {
		t.Fatalf("marketValue = %v", snapshot.MarketValue)
	}
	if snapshot.UnrealizedPnL != float64(8000) {
		t.Fatalf("unrealizedPnL = %v", snapshot.UnrealizedPnL)
	}
}

func TestRuntimeBridgeGetAvailableCashUsesQuoteCurrencyBalance(t *testing.T) {
	account := types.NewAccount()
	account.TotalAccountValue = fixedpoint.NewFromFloat(88000)
	account.SetBalance("USDT", types.Balance{
		Currency:  "USDT",
		Available: fixedpoint.NewFromFloat(1200),
		NetAsset:  fixedpoint.NewFromFloat(1500),
	})
	account.SetBalance("BTC", types.Balance{
		Currency:  "BTC",
		Available: fixedpoint.NewFromFloat(0.75),
		NetAsset:  fixedpoint.NewFromFloat(82500),
	})

	session := &bbgo2.ExchangeSession{Account: account}
	session.SetMarkets(types.MarketMap{
		"BTCUSDT": {
			Symbol:        "BTCUSDT",
			BaseCurrency:  "BTC",
			QuoteCurrency: "USDT",
		},
	})

	bridge := &runtimeBridge{
		strategy: &Strategy{Symbol: "BTCUSDT"},
		session:  session,
	}

	if cash := bridge.getAvailableCash(); cash != 1200 {
		t.Fatalf("getAvailableCash() = %v", cash)
	}
}

func TestRuntimeBridgeGetTotalAccountValuePrefersNormalizedValue(t *testing.T) {
	account := types.NewAccount()
	account.TotalAccountValue = fixedpoint.NewFromFloat(88000)
	account.SetBalance("USDT", types.Balance{
		Currency:  "USDT",
		Available: fixedpoint.NewFromFloat(1200),
		NetAsset:  fixedpoint.NewFromFloat(1500),
	})

	bridge := &runtimeBridge{session: &bbgo2.ExchangeSession{Account: account}}

	if total := bridge.getTotalAccountValue(); total != 88000 {
		t.Fatalf("getTotalAccountValue() = %v", total)
	}
}

func TestRuntimeBridgeHookContextBlocksWarmupOrders(t *testing.T) {
	session := &bbgo2.ExchangeSession{Account: types.NewAccount()}
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true
	session.SetMarkets(types.MarketMap{
		"BTCUSDT": {
			Symbol:        "BTCUSDT",
			BaseCurrency:  "BTC",
			QuoteCurrency: "USDT",
		},
	})

	executor := &stubOrderExecutor{}
	baseTime := time.Date(2026, time.May, 25, 9, 0, 0, 0, time.UTC)
	strategy := &Strategy{
		Name:        "quickjs-warmup-test",
		Symbol:      "BTCUSDT",
		WarmupUntil: baseTime.Add(2 * time.Minute),
		Script: `function onKLineClosed(ctx) {
			placeOrder({ clientOrderId: "warmup-test-order", side: "BUY", quantity: 0.25, orderType: "MARKET" });
		}`,
	}

	bridge, err := newRuntimeBridge(context.Background(), strategy, executor, session)
	if err != nil {
		t.Fatalf("newRuntimeBridge() error = %v", err)
	}
	defer bridge.close()

	err = bridge.invokeHook("onKLineClosed", map[string]any{"symbol": "BTCUSDT"}, &HookContext{
		CurrentKlineTime: baseTime.Add(time.Minute),
		WarmupUntil:      strategy.WarmupUntil,
	})
	if err != nil {
		t.Fatalf("invokeHook() during warmup error = %v", err)
	}
	if len(executor.lastSubmitted) != 0 {
		t.Fatalf("SubmitOrders() count during warmup = %d", len(executor.lastSubmitted))
	}

	err = bridge.invokeHook("onKLineClosed", map[string]any{"symbol": "BTCUSDT"}, &HookContext{
		CurrentKlineTime: baseTime.Add(3 * time.Minute),
		WarmupUntil:      strategy.WarmupUntil,
	})
	if err != nil {
		t.Fatalf("invokeHook() after warmup error = %v", err)
	}
	if len(executor.lastSubmitted) != 1 {
		t.Fatalf("SubmitOrders() count after warmup = %d", len(executor.lastSubmitted))
	}
}

func TestRuntimeBridgePlaceOrderStillErrorsWhenRiskStateBlocksPlace(t *testing.T) {
	session := &bbgo2.ExchangeSession{Account: types.NewAccount()}
	session.SetMarkets(types.MarketMap{
		"BTCUSDT": {
			Symbol:        "BTCUSDT",
			BaseCurrency:  "BTC",
			QuoteCurrency: "USDT",
		},
	})

	executor := &stubOrderExecutor{}
	strategy := &Strategy{
		Name:   "quickjs-blocked-test",
		Symbol: "BTCUSDT",
		Script: `function onKLineClosed(ctx) {
			placeOrder({ clientOrderId: "blocked-test-order", side: "BUY", quantity: 0.25, orderType: "MARKET" });
		}`,
	}

	bridge, err := newRuntimeBridge(context.Background(), strategy, executor, session)
	if err != nil {
		t.Fatalf("newRuntimeBridge() error = %v", err)
	}
	defer bridge.close()

	err = bridge.invokeHook("onKLineClosed", map[string]any{"symbol": "BTCUSDT"}, &HookContext{})
	if err == nil {
		t.Fatal("expected runtime risk state to block placeOrder")
	}
	if !strings.Contains(err.Error(), "place operation is blocked by current runtime state") {
		t.Fatalf("unexpected error = %v", err)
	}
	if len(executor.lastSubmitted) != 0 {
		t.Fatalf("SubmitOrders() count = %d", len(executor.lastSubmitted))
	}
}
