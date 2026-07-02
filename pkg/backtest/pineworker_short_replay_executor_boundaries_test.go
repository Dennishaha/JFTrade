package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestPineWorkerShortReplayExecutorDelegationBoundaries(t *testing.T) {
	missingDelegate := newPineWorkerShortReplayExecutor(nil, types.NewAccount(), nil)
	if _, err := missingDelegate.SubmitOrders(context.Background(), types.SubmitOrder{ClientOrderID: "real"}); err == nil || !strings.Contains(err.Error(), "delegate order executor") {
		t.Fatalf("missing delegate submit error = %v", err)
	}

	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)}})
	stream := types.NewStandardStream()
	delegateErr := errors.New("delegate unavailable")
	executor := newPineWorkerShortReplayExecutor(&fakeWorkerOrderExecutor{submitErr: delegateErr}, account, &stream)
	executor.onKLineClosed(testPineWorkerShortReplayKLine(time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC), 100))

	created, err := executor.SubmitOrders(
		context.Background(),
		types.SubmitOrder{
			ClientOrderID: "synthetic-short",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeSell,
			Type:          types.OrderTypeMarket,
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
			Tag:           pineWorkerShortReplayOrderTag,
		},
		types.SubmitOrder{ClientOrderID: "real-order"},
	)
	if err == nil || !errors.Is(err, delegateErr) {
		t.Fatalf("delegate error = %v, want %v", err, delegateErr)
	}
	if len(created) != 1 || created[0].ClientOrderID != "synthetic-short" || created[0].Status != types.OrderStatusFilled {
		t.Fatalf("partial synthetic fill before delegate error = %#v", created)
	}
}

func TestPineWorkerShortReplayExecutorSyntheticValidation(t *testing.T) {
	market := testPineWorkerShortReplayMarket()
	base := types.SubmitOrder{
		ClientOrderID: "short",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeSell,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(1),
		Price:         fixedpoint.NewFromFloat(100),
		Market:        market,
		Tag:           pineWorkerShortReplayOrderTag,
	}
	stream := types.NewStandardStream()

	if _, err := newPineWorkerShortReplayExecutor(nil, nil, &stream).SubmitOrders(context.Background(), base); err == nil || !strings.Contains(err.Error(), "account is required") {
		t.Fatalf("missing account error = %v", err)
	}
	if _, err := newPineWorkerShortReplayExecutor(nil, types.NewAccount(), nil).SubmitOrders(context.Background(), base); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing stream error = %v", err)
	}

	noPrice := base
	noPrice.Price = fixedpoint.Zero
	if _, err := newPineWorkerShortReplayExecutor(nil, types.NewAccount(), &stream).SubmitOrders(context.Background(), noPrice); err == nil || !strings.Contains(err.Error(), "price is unavailable") {
		t.Fatalf("missing price error = %v", err)
	}

	zeroQuantity := base
	zeroQuantity.Quantity = fixedpoint.Zero
	if _, err := newPineWorkerShortReplayExecutor(nil, types.NewAccount(), &stream).SubmitOrders(context.Background(), zeroQuantity); err == nil || !strings.Contains(err.Error(), "quantity must be positive") {
		t.Fatalf("zero quantity error = %v", err)
	}

	missingSide := base
	missingSide.Side = ""
	if _, err := newPineWorkerShortReplayExecutor(nil, types.NewAccount(), &stream).SubmitOrders(context.Background(), missingSide); err == nil || !strings.Contains(err.Error(), "side is required") {
		t.Fatalf("missing side error = %v", err)
	}
}

func TestPineWorkerShortReplayExecutorSyntheticPriceFallbacks(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)}})
	stream := types.NewStandardStream()
	executor := newPineWorkerShortReplayExecutor(nil, account, &stream)

	market := testPineWorkerShortReplayMarket()
	market.TickSize = fixedpoint.NewFromFloat(0.05)
	before := time.Now().UTC()
	created, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "avg-price",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeSell,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(1),
		AveragePrice:  fixedpoint.NewFromFloat(100.17),
		Market:        market,
		Tag:           pineWorkerShortReplayOrderTag,
	})
	if err != nil {
		t.Fatalf("SubmitOrders average price fallback error = %v", err)
	}
	if len(created) != 1 || created[0].Price.Float64() != 100.1 {
		t.Fatalf("average price fallback fill = %#v, want truncated price 100.1", created)
	}
	if created[0].CreationTime.Time().Before(before) {
		t.Fatalf("fallback event time = %s, want current time after %s", created[0].CreationTime.Time(), before)
	}

	eventTime := time.Date(2026, time.June, 29, 9, 31, 0, 0, time.UTC)
	executor.onKLineClosed(testPineWorkerShortReplayKLine(eventTime, 99.99))
	created, err = executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "last-price",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(1),
		Market:        market,
		Tag:           pineWorkerShortReplayOrderTag,
	})
	if err != nil {
		t.Fatalf("SubmitOrders last price fallback error = %v", err)
	}
	if created[0].Price.Float64() != 99.9 {
		t.Fatalf("last price fallback = %s, want 99.9", created[0].Price)
	}
	if !created[0].CreationTime.Time().Equal(eventTime.Add(time.Minute - time.Millisecond)) {
		t.Fatalf("event time = %s, want kline end time", created[0].CreationTime.Time())
	}
}
