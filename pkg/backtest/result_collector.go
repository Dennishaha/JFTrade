package backtest

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

type orderBookEntryState struct {
	entry         OrderBookEntry
	submittedTime time.Time
	filledTime    time.Time
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

	totalOrders         int
	totalFilledOrders   int
	winningFilledOrders int
	orderBook           []orderBookEntryState
	orderBookIndex      map[string]int
	netPosition         fixedpoint.Value
	pnlCurve            []PnLPoint
	candles             []Candle
	warnedBadClose      bool
	lastCashTotal       fixedpoint.Value
	hasLastCashTotal    bool
}

func newResultCollector(symbol string, strategyInterval types.Interval, quoteCurrency string, warmupUntil time.Time, result *RunResult) *resultCollector {
	return &resultCollector{
		symbol:           symbol,
		strategyInterval: strategyInterval,
		quoteCurrency:    quoteCurrency,
		warmupUntil:      warmupUntil,
		result:           result,
		orderBookIndex:   make(map[string]int),
	}
}

func (c *resultCollector) onOrderUpdate(order types.Order) {
	c.recordOrderBookEntry(order)
	c.totalOrders++
	if order.Status != types.OrderStatusFilled {
		log.Printf("backtest: ORDER id=%d status=%s %s %s", order.OrderID, order.Status, order.Symbol, order.Side)
		return
	}

	c.totalFilledOrders++
	if order.AveragePrice.Compare(fixedpoint.Zero) > 0 && order.Side == types.SideTypeSell {
		c.winningFilledOrders++
	}
	log.Printf("backtest: FILLED id=%d %s %s qty=%s price=%s", order.OrderID, order.Symbol, order.Side, order.Quantity.String(), order.AveragePrice.String())
	if !order.UpdateTime.Time().Before(c.warmupUntil) {
		price := order.AveragePrice
		if price.IsZero() {
			price = order.Price
		}
		c.result.Trades = append(c.result.Trades, TradeEvent{
			Time:  order.UpdateTime.Time().UTC().Format(time.RFC3339Nano),
			Side:  string(order.Side),
			Price: price.String(),
			Qty:   order.Quantity.String(),
		})
	}
	switch order.Side {
	case types.SideTypeBuy:
		c.netPosition = c.netPosition.Add(order.Quantity)
	case types.SideTypeSell:
		c.netPosition = c.netPosition.Sub(order.Quantity)
	}
	c.hasLastCashTotal = false
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

	if order.Status == types.OrderStatusFilled {
		if !eventTime.IsZero() && (state.filledTime.IsZero() || state.filledTime.Before(eventTime)) {
			state.filledTime = eventTime
			state.entry.FilledAt = eventTime.Format(time.RFC3339Nano)
		}
		state.entry.FilledQuantity = order.Quantity.String()
		price := order.AveragePrice
		if price.IsZero() {
			price = order.Price
		}
		if !price.IsZero() {
			state.entry.FilledPrice = price.String()
		}
	}
}

func orderBookEntryKey(order types.Order) string {
	if order.OrderID != 0 {
		return fmt.Sprintf("id:%v", order.OrderID)
	}
	if clientOrderID := strings.TrimSpace(order.ClientOrderID); clientOrderID != "" {
		return "client:" + clientOrderID
	}
	return fmt.Sprintf(
		"fallback:%s:%s:%s",
		order.Symbol,
		order.Side,
		order.UpdateTime.Time().UTC().Format(time.RFC3339Nano),
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

	for _, entry := range c.orderBook {
		if !entry.submittedTime.IsZero() && entry.submittedTime.Before(c.warmupUntil) {
			continue
		}
		if entry.submittedTime.IsZero() && !entry.filledTime.IsZero() && entry.filledTime.Before(c.warmupUntil) {
			continue
		}
		c.result.OrderBook = append(c.result.OrderBook, entry.entry)
	}

	c.result.PnLCurve = c.pnlCurve
	c.result.MaxDrawdown, c.result.CurrentDrawdown, c.result.DrawdownCurve = buildDrawdownMetrics(c.pnlCurve)
	c.result.Candles = c.candles
	c.result.PnL = c.result.FinalBalance - initialBalance
	c.result.TotalTrades = c.totalFilledOrders
	if c.result.TotalTrades > 0 {
		c.result.WinRate = float64(c.winningFilledOrders) / float64(c.result.TotalTrades)
	}

	return c.totalOrders, c.totalFilledOrders
}
