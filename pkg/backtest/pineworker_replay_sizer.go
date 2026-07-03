package backtest

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type pineWorkerReplaySizer struct {
	symbol        string
	quoteCurrency string
	account       *types.Account

	mu          sync.RWMutex
	netPosition fixedpoint.Value
	lastPrice   fixedpoint.Value
}

func newPineWorkerReplaySizer(symbol string, quoteCurrency string, account *types.Account) *pineWorkerReplaySizer {
	return &pineWorkerReplaySizer{
		symbol:        strings.TrimSpace(symbol),
		quoteCurrency: strings.TrimSpace(quoteCurrency),
		account:       account,
	}
}

func (sizer *pineWorkerReplaySizer) onKLineClosed(kline types.KLine) {
	if sizer == nil {
		return
	}
	if sizer.symbol != "" && kline.Symbol != sizer.symbol {
		return
	}
	sizer.mu.Lock()
	sizer.lastPrice = kline.Close
	sizer.mu.Unlock()
}

func (sizer *pineWorkerReplaySizer) onOrderUpdate(order types.Order) {
	if sizer == nil || order.Status != types.OrderStatusFilled {
		return
	}
	if sizer.symbol != "" && order.Symbol != sizer.symbol {
		return
	}
	quantity := order.ExecutedQuantity
	if quantity.IsZero() {
		quantity = order.Quantity
	}
	if quantity.Sign() <= 0 {
		return
	}

	sizer.mu.Lock()
	defer sizer.mu.Unlock()
	switch order.Side {
	case types.SideTypeBuy:
		sizer.netPosition = sizer.netPosition.Add(quantity)
	case types.SideTypeSell:
		sizer.netPosition = sizer.netPosition.Sub(quantity)
	}
}

func (sizer *pineWorkerReplaySizer) NetPosition() fixedpoint.Value {
	if sizer == nil {
		return fixedpoint.Zero
	}
	sizer.mu.RLock()
	defer sizer.mu.RUnlock()
	return sizer.netPosition
}

func (sizer *pineWorkerReplaySizer) QuantityForCommand(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error) {
	if sizer == nil {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires position sizing", command.ID)
	}
	if math.IsNaN(command.QuantityPct) || math.IsInf(command.QuantityPct, 0) || command.QuantityPct <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct must be positive", command.ID)
	}

	percent := fixedpoint.NewFromFloat(command.QuantityPct / 100)
	switch normalizeWorkerIntentKind(command.Kind) {
	case "entry", "order":
		return sizer.entryQuantity(command, market, percent)
	case "exit", "close", "close_all":
		return sizer.closeQuantity(command, market, percent)
	default:
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s does not support quantity pct", command.ID)
	}
}

func (sizer *pineWorkerReplaySizer) entryQuantity(command WorkerOrderCommand, market types.Market, percent fixedpoint.Value) (fixedpoint.Value, error) {
	price := sizer.priceForCommand(command, market)
	if price.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires a positive price", command.ID)
	}
	equity, err := sizer.equity(market)
	if err != nil {
		return fixedpoint.Zero, err
	}
	if equity.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires positive equity", command.ID)
	}
	return truncatePineWorkerSizedQuantity(market, equity.Mul(percent).Div(price)), nil
}

func (sizer *pineWorkerReplaySizer) closeQuantity(command WorkerOrderCommand, market types.Market, percent fixedpoint.Value) (fixedpoint.Value, error) {
	sizer.mu.RLock()
	position := sizer.netPosition.Abs()
	sizer.mu.RUnlock()
	if position.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires an open position", command.ID)
	}
	quantity := position.Mul(percent)
	if quantity.Compare(position) > 0 {
		quantity = position
	}
	return truncatePineWorkerSizedQuantity(market, quantity), nil
}

func (sizer *pineWorkerReplaySizer) equity(market types.Market) (fixedpoint.Value, error) {
	if sizer.account == nil {
		return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct account is required")
	}
	quoteCurrency := strings.TrimSpace(market.QuoteCurrency)
	if quoteCurrency == "" {
		quoteCurrency = sizer.quoteCurrency
	}
	if quoteCurrency == "" {
		return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct quote currency is required")
	}
	balance, _ := sizer.account.Balance(quoteCurrency)
	equity := balance.Total()

	sizer.mu.RLock()
	netPosition := sizer.netPosition
	lastPrice := sizer.lastPrice
	sizer.mu.RUnlock()
	if !netPosition.IsZero() {
		if lastPrice.Sign() <= 0 {
			return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct requires a positive mark price")
		}
		equity = equity.Add(netPosition.Mul(lastPrice))
	}
	return equity, nil
}

func (sizer *pineWorkerReplaySizer) priceForCommand(command WorkerOrderCommand, market types.Market) fixedpoint.Value {
	if command.LimitPrice > 0 {
		return fixedpoint.NewFromFloat(command.LimitPrice)
	}
	if command.StopPrice > 0 {
		return fixedpoint.NewFromFloat(command.StopPrice)
	}
	sizer.mu.RLock()
	price := sizer.lastPrice
	sizer.mu.RUnlock()
	if !market.TickSize.IsZero() && price.Sign() > 0 {
		return market.TruncatePrice(price)
	}
	return price
}

func truncatePineWorkerSizedQuantity(market types.Market, quantity fixedpoint.Value) fixedpoint.Value {
	if quantity.Sign() <= 0 {
		return fixedpoint.Zero
	}
	if !market.StepSize.IsZero() {
		return market.TruncateQuantity(quantity)
	}
	if market.VolumePrecision > 0 {
		return market.RoundDownQuantityByPrecision(quantity)
	}
	return quantity
}
