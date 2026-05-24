package backtest

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

type accountQuerier interface {
	QueryAccount(context.Context) (*types.Account, error)
}

type resultCollector struct {
	symbol           string
	strategyInterval types.Interval
	quoteCurrency    string
	warmupUntil      time.Time
	result           *RunResult

	filledOrders   []types.Order
	allOrders      []types.Order
	netPosition    fixedpoint.Value
	pnlCurve       []PnLPoint
	candles        []Candle
	warnedBadClose bool
}

func newResultCollector(symbol string, strategyInterval types.Interval, quoteCurrency string, warmupUntil time.Time, result *RunResult) *resultCollector {
	return &resultCollector{
		symbol:           symbol,
		strategyInterval: strategyInterval,
		quoteCurrency:    quoteCurrency,
		warmupUntil:      warmupUntil,
		result:           result,
	}
}

func (c *resultCollector) onOrderUpdate(order types.Order) {
	c.allOrders = append(c.allOrders, order)
	if order.Status != types.OrderStatusFilled {
		log.Printf("backtest: ORDER id=%d status=%s %s %s", order.OrderID, order.Status, order.Symbol, order.Side)
		return
	}

	c.filledOrders = append(c.filledOrders, order)
	log.Printf("backtest: FILLED id=%d %s %s qty=%s price=%s", order.OrderID, order.Symbol, order.Side, order.Quantity.String(), order.AveragePrice.String())
	switch order.Side {
	case types.SideTypeBuy:
		c.netPosition = c.netPosition.Add(order.Quantity)
	case types.SideTypeSell:
		c.netPosition = c.netPosition.Sub(order.Quantity)
	}
}

func (c *resultCollector) onKLineClosed(ctx context.Context, exchange accountQuerier, kline types.KLine) {
	if kline.Symbol != c.symbol {
		return
	}
	if kline.EndTime.Time().Before(c.warmupUntil) {
		return
	}
	if kline.Interval == c.strategyInterval {
		c.candles = append(c.candles, Candle{
			Time:   kline.EndTime.Time().Format(time.RFC3339),
			Open:   kline.Open.Float64(),
			High:   kline.High.Float64(),
			Low:    kline.Low.Float64(),
			Close:  kline.Close.Float64(),
			Volume: kline.Volume.Float64(),
		})
	}

	account, err := exchange.QueryAccount(ctx)
	if err != nil {
		return
	}

	total := fixedpoint.Zero
	if balance, ok := account.Balances()[c.quoteCurrency]; ok {
		total = balance.Total()
	}
	if !c.netPosition.IsZero() && kline.Close.Sign() > 0 {
		total = total.Add(c.netPosition.Mul(kline.Close))
	} else if !c.netPosition.IsZero() && !c.warnedBadClose {
		c.warnedBadClose = true
		msg := fmt.Sprintf("回测期间发现非正收盘价 (%.4f)，持仓市值无法计算，权益曲线可能不准确。请检查K线数据或复权方式。", kline.Close.Float64())
		log.Printf("backtest: %s", msg)
		c.result.addRuntimeError(msg)
	}

	c.pnlCurve = append(c.pnlCurve, PnLPoint{
		Time:   kline.EndTime.Time().Format(time.RFC3339),
		Equity: total.Float64(),
	})
}

func (c *resultCollector) finalize(ctx context.Context, exchange accountQuerier, initialBalance float64) (int, int) {
	account, err := exchange.QueryAccount(ctx)
	if err == nil {
		total := fixedpoint.Zero
		if balance, ok := account.Balances()[c.quoteCurrency]; ok {
			total = balance.Total()
		}
		if !c.netPosition.IsZero() && len(c.candles) > 0 && c.candles[len(c.candles)-1].Close > 0 {
			lastClose := fixedpoint.NewFromFloat(c.candles[len(c.candles)-1].Close)
			total = total.Add(c.netPosition.Mul(lastClose))
		} else if !c.netPosition.IsZero() {
			msg := fmt.Sprintf("最终持仓 %.0f 股无法按市价估值（最新收盘价非正），最终权益不含持仓市值。", c.netPosition.Float64())
			log.Printf("backtest: %s", msg)
			c.result.addRuntimeError(msg)
		}
		c.result.FinalBalance = total.Float64()
	}

	for _, order := range c.filledOrders {
		if order.UpdateTime.Time().Before(c.warmupUntil) {
			continue
		}
		price := order.AveragePrice
		if price.IsZero() {
			price = order.Price
		}
		c.result.Trades = append(c.result.Trades, TradeEvent{
			Time:  order.UpdateTime.Time().Format(time.RFC3339),
			Side:  string(order.Side),
			Price: price.Float64(),
			Qty:   order.Quantity.Float64(),
		})
	}

	c.result.PnLCurve = c.pnlCurve
	c.result.Candles = c.candles
	c.result.PnL = c.result.FinalBalance - initialBalance
	c.result.TotalTrades = len(c.filledOrders)
	if c.result.TotalTrades > 0 {
		wins := 0
		for _, order := range c.filledOrders {
			if order.AveragePrice.Compare(fixedpoint.Zero) > 0 && order.Side == types.SideTypeSell {
				wins++
			}
		}
		c.result.WinRate = float64(wins) / float64(c.result.TotalTrades)
	}

	return len(c.allOrders), len(c.filledOrders)
}
