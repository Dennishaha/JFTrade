package backtest

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type orderBookEntryState struct {
	entry         OrderBookEntry
	submittedTime time.Time
	filledTime    time.Time
}

const closedTradeStatsVersion = 2

type incrementalOrderFill struct {
	quantity fixedpoint.Value
	price    fixedpoint.Value
}

type closingOrderStats struct {
	quantity    fixedpoint.Value
	realizedPnL fixedpoint.Value
	unknownPnL  bool
}

type accountQuerier interface {
	QueryAccount(context.Context) (*types.Account, error)
}

type resultCollector struct {
	symbol           string
	strategyInterval types.Interval
	quoteCurrency    string
	warmupUntil      time.Time
	result           *RunResult

	totalOrders       int
	totalFilledOrders int
	closedTrades      int
	winningTrades     int
	orderBook         []orderBookEntryState
	orderBookIndex    map[string]int

	netPosition       fixedpoint.Value
	averageEntryPrice fixedpoint.Value
	positionCostKnown bool
	closingOrders     map[string]*closingOrderStats

	pnlCurve            []PnLPoint
	candles             []Candle
	warnedBadClose      bool
	lastCashTotal       fixedpoint.Value
	hasLastCashTotal    bool
	orderFees           map[uint64]appliedTradeFees
	tradeIndexByOrderID map[uint64]int
	orderExecuted       map[string]fixedpoint.Value
	orderNotional       map[string]fixedpoint.Value
}

func newResultCollector(symbol string, strategyInterval types.Interval, quoteCurrency string, warmupUntil time.Time, result *RunResult) *resultCollector {
	return &resultCollector{
		symbol:              symbol,
		strategyInterval:    strategyInterval,
		quoteCurrency:       quoteCurrency,
		warmupUntil:         warmupUntil,
		result:              result,
		orderBookIndex:      make(map[string]int),
		orderFees:           make(map[uint64]appliedTradeFees),
		tradeIndexByOrderID: make(map[uint64]int),
		orderExecuted:       make(map[string]fixedpoint.Value),
		orderNotional:       make(map[string]fixedpoint.Value),
		closingOrders:       make(map[string]*closingOrderStats),
	}
}

func (c *resultCollector) onOrderUpdate(order types.Order) {
	c.recordOrderBookEntry(order)
	c.totalOrders++
	if order.Status != types.OrderStatusFilled && order.Status != types.OrderStatusPartiallyFilled {
		if order.Status.Closed() {
			c.finalizeClosingOrder(orderBookEntryKey(order))
		}
		log.Printf("backtest: ORDER id=%d status=%s %s %s", order.OrderID, order.Status, order.Symbol, order.Side)
		return
	}

	fill := c.incrementalFill(order)
	if fill.quantity.Sign() <= 0 {
		if order.Status.Closed() {
			c.finalizeClosingOrder(orderBookEntryKey(order))
		}
		log.Printf("backtest: ORDER id=%d status=%s has no incremental fill", order.OrderID, order.Status)
		return
	}
	c.totalFilledOrders++
	closedQuantity, realizedPnL, pnlKnown := c.applyPositionFill(order.Side, fill.quantity, fill.price)
	log.Printf("backtest: FILLED id=%d %s %s qty=%s price=%s", order.OrderID, order.Symbol, order.Side, fill.quantity.String(), fill.price.String())
	eventTime := order.UpdateTime.Time()
	fees := c.feesForOrder(order.OrderID)
	tradeEvent := TradeEvent{
		Time:        eventTime.UTC().Format(time.RFC3339Nano),
		Side:        string(order.Side),
		Price:       fill.price.String(),
		Qty:         fill.quantity.String(),
		Warmup:      !eventTime.IsZero() && eventTime.Before(c.warmupUntil),
		BrokerFee:   fees.BrokerFee,
		MarketFee:   fees.MarketFee,
		TotalFee:    fees.TotalFee,
		FeeCurrency: fees.FeeCurrency,
	}
	if closedQuantity.Sign() > 0 && pnlKnown {
		tradeEvent.PnL = realizedPnL.Float64()
	}
	c.result.Trades = append(c.result.Trades, tradeEvent)
	c.tradeIndexByOrderID[order.OrderID] = len(c.result.Trades) - 1
	if closedQuantity.Sign() > 0 {
		c.recordClosingFill(orderBookEntryKey(order), closedQuantity, realizedPnL, pnlKnown)
	}
	if order.Status.Closed() {
		c.finalizeClosingOrder(orderBookEntryKey(order))
	}
	c.hasLastCashTotal = false
}

func (c *resultCollector) incrementalFill(order types.Order) incrementalOrderFill {
	if c.orderExecuted == nil {
		c.orderExecuted = make(map[string]fixedpoint.Value)
	}
	if c.orderNotional == nil {
		c.orderNotional = make(map[string]fixedpoint.Value)
	}
	key := orderBookEntryKey(order)
	executed := order.ExecutedQuantity
	if executed.IsZero() && order.Status == types.OrderStatusFilled {
		executed = order.Quantity
	}
	if executed.Sign() <= 0 {
		return incrementalOrderFill{}
	}
	previous := c.orderExecuted[key]
	fillQuantity := executed.Sub(previous)
	if fillQuantity.Sign() <= 0 {
		return incrementalOrderFill{}
	}

	averagePrice := order.AveragePrice
	if averagePrice.IsZero() {
		averagePrice = order.Price
	}
	fillPrice := averagePrice
	if averagePrice.Sign() > 0 {
		cumulativeNotional := averagePrice.Mul(executed)
		incrementalNotional := cumulativeNotional.Sub(c.orderNotional[key])
		if incrementalNotional.Sign() > 0 {
			fillPrice = incrementalNotional.Div(fillQuantity)
		}
		c.orderNotional[key] = cumulativeNotional
	}
	c.orderExecuted[key] = executed
	return incrementalOrderFill{quantity: fillQuantity, price: fillPrice}
}

// applyPositionFill maintains weighted-average position cost and returns the
// portion of this fill that closed an existing position.
func (c *resultCollector) applyPositionFill(
	side types.SideType,
	quantity fixedpoint.Value,
	price fixedpoint.Value,
) (fixedpoint.Value, fixedpoint.Value, bool) {
	if quantity.Sign() <= 0 {
		return fixedpoint.Zero, fixedpoint.Zero, false
	}
	delta := quantity
	if side == types.SideTypeSell {
		delta = quantity.Neg()
	} else if side != types.SideTypeBuy {
		return fixedpoint.Zero, fixedpoint.Zero, false
	}

	current := c.netPosition
	if current.IsZero() || current.Sign() == delta.Sign() {
		newPosition := current.Add(delta)
		if current.IsZero() {
			c.averageEntryPrice = price
			c.positionCostKnown = price.Sign() > 0
		} else if c.positionCostKnown && price.Sign() > 0 {
			currentCost := c.averageEntryPrice.Mul(current.Abs())
			fillCost := price.Mul(quantity)
			c.averageEntryPrice = currentCost.Add(fillCost).Div(newPosition.Abs())
		} else {
			c.averageEntryPrice = fixedpoint.Zero
			c.positionCostKnown = false
		}
		c.netPosition = newPosition
		return fixedpoint.Zero, fixedpoint.Zero, false
	}

	closedQuantity := quantity
	if current.Abs().Compare(closedQuantity) < 0 {
		closedQuantity = current.Abs()
	}
	pnlKnown := c.positionCostKnown && price.Sign() > 0
	realizedPnL := fixedpoint.Zero
	if pnlKnown {
		if current.Sign() > 0 {
			realizedPnL = price.Sub(c.averageEntryPrice).Mul(closedQuantity)
		} else {
			realizedPnL = c.averageEntryPrice.Sub(price).Mul(closedQuantity)
		}
	}

	newPosition := current.Add(delta)
	c.netPosition = newPosition
	switch {
	case newPosition.IsZero():
		c.averageEntryPrice = fixedpoint.Zero
		c.positionCostKnown = false
	case newPosition.Sign() != current.Sign():
		c.averageEntryPrice = price
		c.positionCostKnown = price.Sign() > 0
	}
	return closedQuantity, realizedPnL, pnlKnown
}

func (c *resultCollector) recordClosingFill(
	key string,
	quantity fixedpoint.Value,
	realizedPnL fixedpoint.Value,
	pnlKnown bool,
) {
	if quantity.Sign() <= 0 {
		return
	}
	if c.closingOrders == nil {
		c.closingOrders = make(map[string]*closingOrderStats)
	}
	stats := c.closingOrders[key]
	if stats == nil {
		stats = &closingOrderStats{}
		c.closingOrders[key] = stats
	}
	stats.quantity = stats.quantity.Add(quantity)
	stats.realizedPnL = stats.realizedPnL.Add(realizedPnL)
	stats.unknownPnL = stats.unknownPnL || !pnlKnown
}

func (c *resultCollector) finalizeClosingOrder(key string) {
	stats := c.closingOrders[key]
	if stats == nil {
		return
	}
	delete(c.closingOrders, key)
	if stats.quantity.Sign() <= 0 {
		return
	}
	c.closedTrades++
	if !stats.unknownPnL && stats.realizedPnL.Sign() > 0 {
		c.winningTrades++
	}
}

func (c *resultCollector) finalizePendingClosingOrders() {
	for key := range c.closingOrders {
		c.finalizeClosingOrder(key)
	}
}

func (c *resultCollector) recordOrderBookEntry(order types.Order) {
	key := orderBookEntryKey(order)
	index, ok := c.orderBookIndex[key]
	if !ok {
		index = len(c.orderBook)
		c.orderBookIndex[key] = index
		c.orderBook = append(c.orderBook, orderBookEntryState{
			entry: OrderBookEntry{
				OrderID: orderBookDisplayID(order),
			},
		})
	}

	state := &c.orderBook[index]
	state.entry.Symbol = order.Symbol
	state.entry.Side = string(order.Side)
	state.entry.Quantity = order.Quantity.String()
	state.entry.OrderType = string(order.Type)
	state.entry.Status = string(order.Status)
	if clientOrderID := strings.TrimSpace(order.ClientOrderID); clientOrderID != "" {
		state.entry.ClientOrderID = clientOrderID
	}
	if !order.Price.IsZero() {
		state.entry.OrderPrice = order.Price.String()
	}

	eventTime := order.UpdateTime.Time().UTC()
	if !eventTime.IsZero() && (state.submittedTime.IsZero() || eventTime.Before(state.submittedTime)) {
		state.submittedTime = eventTime
		state.entry.SubmittedAt = eventTime.Format(time.RFC3339Nano)
	}

	if order.Status == types.OrderStatusFilled || order.Status == types.OrderStatusPartiallyFilled {
		if !eventTime.IsZero() && (state.filledTime.IsZero() || state.filledTime.Before(eventTime)) {
			state.filledTime = eventTime
			state.entry.FilledAt = eventTime.Format(time.RFC3339Nano)
		}
		filledQuantity := order.ExecutedQuantity
		if filledQuantity.IsZero() && order.Status == types.OrderStatusFilled {
			filledQuantity = order.Quantity
		}
		if filledQuantity.Sign() > 0 {
			state.entry.FilledQuantity = filledQuantity.String()
		}
		price := order.AveragePrice
		if price.IsZero() {
			price = order.Price
		}
		if !price.IsZero() {
			state.entry.FilledPrice = price.String()
		}
		c.applyOrderFeesToEntry(order.OrderID, &state.entry)
	}
	state.entry.Warmup = c.isWarmupOrder(*state)
}

func (c *resultCollector) isWarmupOrder(state orderBookEntryState) bool {
	if c.warmupUntil.IsZero() {
		return false
	}
	if !state.submittedTime.IsZero() {
		return state.submittedTime.Before(c.warmupUntil)
	}
	return !state.filledTime.IsZero() && state.filledTime.Before(c.warmupUntil)
}

func (c *resultCollector) recordTradeFees(trade types.Trade, fees appliedTradeFees) {
	if trade.OrderID == 0 || fees.TotalFee <= 0 {
		return
	}
	existing := c.orderFees[trade.OrderID]
	existing.BrokerFee += fees.BrokerFee
	existing.MarketFee += fees.MarketFee
	existing.TotalFee += fees.TotalFee
	if existing.FeeCurrency == "" {
		existing.FeeCurrency = fees.FeeCurrency
	}
	c.orderFees[trade.OrderID] = existing

	if index, ok := c.tradeIndexByOrderID[trade.OrderID]; ok && index >= 0 && index < len(c.result.Trades) {
		c.result.Trades[index].BrokerFee = existing.BrokerFee
		c.result.Trades[index].MarketFee = existing.MarketFee
		c.result.Trades[index].TotalFee = existing.TotalFee
		c.result.Trades[index].FeeCurrency = existing.FeeCurrency
	}
	if index, ok := c.orderBookIndex["id:"+fmt.Sprint(trade.OrderID)]; ok && index >= 0 && index < len(c.orderBook) {
		c.applyOrderFeesToEntry(trade.OrderID, &c.orderBook[index].entry)
	}
	c.hasLastCashTotal = false
}

func (c *resultCollector) feesForOrder(orderID uint64) appliedTradeFees {
	if orderID == 0 {
		return appliedTradeFees{}
	}
	return c.orderFees[orderID]
}

func (c *resultCollector) applyOrderFeesToEntry(orderID uint64, entry *OrderBookEntry) {
	if entry == nil {
		return
	}
	fees := c.feesForOrder(orderID)
	if fees.TotalFee <= 0 {
		return
	}
	entry.BrokerFee = fees.BrokerFee
	entry.MarketFee = fees.MarketFee
	entry.TotalFee = fees.TotalFee
	entry.FeeCurrency = fees.FeeCurrency
}

func orderBookEntryKey(order types.Order) string {
	if order.OrderID != 0 {
		return fmt.Sprintf("id:%v", order.OrderID)
	}
	if clientOrderID := strings.TrimSpace(order.ClientOrderID); clientOrderID != "" {
		return "client:" + clientOrderID
	}
	eventTime := order.CreationTime.Time().UTC()
	if eventTime.IsZero() {
		eventTime = order.UpdateTime.Time().UTC()
	}
	return fmt.Sprintf(
		"fallback:%s:%s:%s",
		order.Symbol,
		order.Side,
		eventTime.Format(time.RFC3339Nano),
	)
}

func orderBookDisplayID(order types.Order) string {
	if order.OrderID != 0 {
		return fmt.Sprint(order.OrderID)
	}
	if clientOrderID := strings.TrimSpace(order.ClientOrderID); clientOrderID != "" {
		return clientOrderID
	}
	return "pending"
}

func (c *resultCollector) onKLineClosed(ctx context.Context, exchange accountQuerier, kline types.KLine) {
	if kline.Symbol != c.symbol {
		return
	}
	endAt := kline.EndTime.Time()
	if endAt.Before(c.warmupUntil) {
		return
	}
	timestamp := endAt.UTC().Format(time.RFC3339Nano)
	if kline.Interval == c.strategyInterval {
		c.candles = append(c.candles, Candle{
			Time:   timestamp,
			Open:   kline.Open.String(),
			High:   kline.High.String(),
			Low:    kline.Low.String(),
			Close:  kline.Close.String(),
			Volume: kline.Volume.String(),
		})
	}

	total := fixedpoint.Zero
	if c.hasLastCashTotal {
		total = c.lastCashTotal
	} else {
		account, err := exchange.QueryAccount(ctx)
		if err != nil {
			return
		}
		if balance, ok := account.Balance(c.quoteCurrency); ok {
			total = balance.Total()
			c.lastCashTotal = total
			c.hasLastCashTotal = true
		}
	}
	if !c.netPosition.IsZero() && kline.Close.Sign() > 0 {
		total = total.Add(c.netPosition.Mul(kline.Close))
	} else if !c.netPosition.IsZero() && !c.warnedBadClose {
		c.warnedBadClose = true
		msg := fmt.Sprintf("回测期间发现非正收盘价 (%.4f)，持仓市值无法计算，权益曲线可能不准确。请检查K线数据或复权方式。", kline.Close.Float64())
		log.Printf("backtest: %s", msg)
		c.result.AddRuntimeError(msg)
	}

	c.pnlCurve = append(c.pnlCurve, PnLPoint{
		Time:   timestamp,
		Equity: total.Float64(),
	})
}

func buildDrawdownMetrics(pnlCurve []PnLPoint) (float64, float64, []DrawdownPoint) {
	if len(pnlCurve) == 0 {
		return 0, 0, nil
	}

	drawdownCurve := make([]DrawdownPoint, len(pnlCurve))
	peak := pnlCurve[0].Equity
	maxDrawdown := 0.0
	currentDrawdown := 0.0

	for index := range pnlCurve {
		point := pnlCurve[index]
		if point.Equity > peak {
			peak = point.Equity
		}

		drawdown := 0.0
		if peak > 0 && point.Equity < peak {
			drawdown = (peak - point.Equity) / peak
		}

		drawdownCurve[index] = DrawdownPoint{
			Time:     point.Time,
			Drawdown: drawdown,
		}
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
		currentDrawdown = drawdown
	}

	return maxDrawdown, currentDrawdown, drawdownCurve
}

func (c *resultCollector) finalize(ctx context.Context, exchange accountQuerier, initialBalance float64) (int, int) {
	c.finalizePendingClosingOrders()
	account, err := exchange.QueryAccount(ctx)
	if err == nil {
		total := fixedpoint.Zero
		if balance, ok := account.Balance(c.quoteCurrency); ok {
			total = balance.Total()
			c.lastCashTotal = total
			c.hasLastCashTotal = true
		}
		if !c.netPosition.IsZero() && len(c.candles) > 0 {
			lastClose, err := fixedpoint.NewFromString(c.candles[len(c.candles)-1].Close)
			if err == nil && lastClose.Sign() > 0 {
				total = total.Add(c.netPosition.Mul(lastClose))
			} else if !c.netPosition.IsZero() {
				msg := fmt.Sprintf("最终持仓 %.0f 股无法按市价估值（最新收盘价非正），最终权益不含持仓市值。", c.netPosition.Float64())
				log.Printf("backtest: %s", msg)
				c.result.AddRuntimeError(msg)
			}
		} else if !c.netPosition.IsZero() {
			msg := fmt.Sprintf("最终持仓 %.0f 股无法按市价估值（最新收盘价缺失），最终权益不含持仓市值。", c.netPosition.Float64())
			log.Printf("backtest: %s", msg)
			c.result.AddRuntimeError(msg)
		}
		c.result.FinalBalance = total.Float64()
	}

	for _, state := range c.orderBook {
		state.entry.Warmup = c.isWarmupOrder(state)
		c.result.OrderBook = append(c.result.OrderBook, state.entry)
	}

	c.result.PnLCurve = c.pnlCurve
	c.result.MaxDrawdown, c.result.CurrentDrawdown, c.result.DrawdownCurve = buildDrawdownMetrics(c.pnlCurve)
	c.result.Candles = c.candles
	c.result.PnL = c.result.FinalBalance - initialBalance
	c.result.TradeStatsVersion = closedTradeStatsVersion
	c.result.TotalFills = len(c.result.Trades)
	c.result.TotalTrades = c.closedTrades
	c.result.WinRate = 0
	if c.result.TotalTrades > 0 {
		c.result.WinRate = float64(c.winningTrades) / float64(c.result.TotalTrades)
	}

	return c.totalOrders, c.totalFilledOrders
}
