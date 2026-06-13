package pineruntime

import (
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
)

func (r *strategyRuntime) getPosition(symbol string, barTime time.Time) *positionSnapshot {
	if cached, ok := r.cachedPosition(symbol, barTime); ok {
		return cached
	}
	if r.session == nil {
		return nil
	}
	market, ok := r.session.Market(symbol)
	if !ok {
		return nil
	}
	var baseQuantity fixedpoint.Value
	position := runtimePositionForSymbol(r.session, symbol)
	if position != nil {
		baseQuantity = position.Base
	}
	availableQuantity := fixedpoint.Zero
	if account := runtimeAccount(r.session); account != nil && market.BaseCurrency != "" {
		if balance, ok := account.Balance(market.BaseCurrency); ok {
			availableQuantity = balance.Available
			if baseQuantity.IsZero() {
				baseQuantity = balance.Total()
			}
		}
	}
	lastPrice, _ := r.session.LastPrice(symbol)
	marketPrice := lastPrice
	if marketPrice.IsZero() && position != nil {
		marketPrice = position.AverageCost
	}
	averagePrice := 0.0
	if position != nil {
		averagePrice = position.AverageCost.Float64()
	}
	direction := "FLAT"
	if baseQuantity.Sign() > 0 {
		direction = "LONG"
	} else if baseQuantity.Sign() < 0 {
		direction = "SHORT"
	}
	snapshot := &positionSnapshot{
		Symbol:            symbol,
		Quantity:          baseQuantity.Float64(),
		AvailableQuantity: availableQuantity.Float64(),
		MarketValue:       marketPrice.Mul(baseQuantity).Float64(),
		AveragePrice:      averagePrice,
		Direction:         direction,
	}
	r.storeCachedPosition(symbol, barTime, snapshot)
	return snapshot
}
