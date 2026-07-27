package strategy

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type liveCommandTestSizer struct {
	symbol        string
	quoteCurrency string
	account       *types.Account

	mu          sync.RWMutex
	netPosition fixedpoint.Value
	lastPrice   fixedpoint.Value
}

func newLiveCommandTestSizer(symbol string, quoteCurrency string, account *types.Account) *liveCommandTestSizer {
	return &liveCommandTestSizer{
		symbol:        strings.TrimSpace(symbol),
		quoteCurrency: strings.TrimSpace(quoteCurrency),
		account:       account,
	}
}

func (sizer *liveCommandTestSizer) onKLineClosed(kline types.KLine) {
	if sizer == nil || (sizer.symbol != "" && kline.Symbol != sizer.symbol) {
		return
	}
	sizer.mu.Lock()
	sizer.lastPrice = kline.Close
	sizer.mu.Unlock()
}

func (sizer *liveCommandTestSizer) onOrderUpdate(order types.Order) {
	if sizer == nil || (order.Status != types.OrderStatusFilled && order.Status != types.OrderStatusPartiallyFilled) {
		return
	}
	if sizer.symbol != "" && order.Symbol != sizer.symbol {
		return
	}
	executed := order.ExecutedQuantity
	if executed.IsZero() && order.Status == types.OrderStatusFilled {
		executed = order.Quantity
	}
	if executed.Sign() <= 0 {
		return
	}
	sizer.mu.Lock()
	defer sizer.mu.Unlock()
	switch order.Side {
	case types.SideTypeBuy:
		sizer.netPosition = sizer.netPosition.Add(executed)
	case types.SideTypeSell:
		sizer.netPosition = sizer.netPosition.Sub(executed)
	}
}

func (sizer *liveCommandTestSizer) NetPosition() fixedpoint.Value {
	if sizer == nil {
		return fixedpoint.Zero
	}
	sizer.mu.RLock()
	defer sizer.mu.RUnlock()
	return sizer.netPosition
}

func (sizer *liveCommandTestSizer) QuantityForCommand(
	command WorkerOrderCommand,
	market types.Market,
) (fixedpoint.Value, error) {
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
		position := sizer.NetPosition().Abs()
		if position.Sign() <= 0 {
			return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires an open position", command.ID)
		}
		quantity := position.Mul(percent)
		if quantity.Compare(position) > 0 {
			quantity = position
		}
		return quantity, nil
	default:
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s does not support quantity pct", command.ID)
	}
}

func (sizer *liveCommandTestSizer) entryQuantity(
	command WorkerOrderCommand,
	market types.Market,
	percent fixedpoint.Value,
) (fixedpoint.Value, error) {
	price := fixedpoint.NewFromFloat(command.LimitPrice)
	if price.Sign() <= 0 {
		price = fixedpoint.NewFromFloat(command.StopPrice)
	}
	if price.Sign() <= 0 {
		sizer.mu.RLock()
		price = sizer.lastPrice
		sizer.mu.RUnlock()
	}
	if price.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires a positive price", command.ID)
	}
	if sizer.account == nil {
		return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct account is required")
	}
	quoteCurrency := strings.TrimSpace(market.QuoteCurrency)
	if quoteCurrency == "" {
		quoteCurrency = sizer.quoteCurrency
	}
	balance, _ := sizer.account.Balance(quoteCurrency)
	equity := balance.Total()
	if equity.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires positive equity", command.ID)
	}
	return equity.Mul(percent).Div(price), nil
}

type groupedLiveWarnings struct {
	IgnoredOrders int
	WarningTotal  int
	Warnings      []string

	counts map[string]int
	index  map[string]int
}

func (warnings *groupedLiveWarnings) AddIgnoredOrderWarning(message string) {
	warnings.IgnoredOrders++
	warnings.WarningTotal++
	warnings.Warnings = append(warnings.Warnings, message)
}

func (warnings *groupedLiveWarnings) AddIgnoredOrderWarningGroup(key string, message string) {
	warnings.IgnoredOrders++
	if warnings.counts == nil {
		warnings.counts = make(map[string]int)
		warnings.index = make(map[string]int)
	}
	warnings.counts[key]++
	count := warnings.counts[key]
	if count == 1 {
		warnings.WarningTotal++
		warnings.index[key] = len(warnings.Warnings)
		warnings.Warnings = append(warnings.Warnings, message)
		return
	}
	warnings.Warnings[warnings.index[key]] = fmt.Sprintf(
		"%s (occurred %d times; first occurrence shown)",
		message,
		count,
	)
}

func testLiveCommandMarket() types.Market {
	return types.Market{
		Exchange:      types.ExchangeName("live-test"),
		Symbol:        "US.AAPL",
		BaseCurrency:  "AAPL",
		QuoteCurrency: "USD",
		MinQuantity:   fixedpoint.NewFromFloat(0.0001),
		MinNotional:   fixedpoint.NewFromFloat(1),
	}
}

func liveCommandTestKLine(start time.Time, closePrice float64) types.KLine {
	price := fixedpoint.NewFromFloat(closePrice)
	return types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
		Open:      price,
		High:      price,
		Low:       price,
		Close:     price,
		Volume:    fixedpoint.NewFromFloat(1000),
	}
}
