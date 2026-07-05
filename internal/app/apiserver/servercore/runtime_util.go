package servercore

import (
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func strategyRuntimeBrokerReadQuery(binding strategyInstanceBinding) broker.ReadQuery {
	query := broker.ReadQuery{}
	if binding.BrokerAccount == nil {
		return query
	}
	query.AccountID = strings.TrimSpace(binding.BrokerAccount.AccountID)
	query.TradingEnvironment = strings.TrimSpace(binding.BrokerAccount.TradingEnvironment)
	query.Market = strings.TrimSpace(binding.BrokerAccount.Market)
	return query
}

func strategyRuntimeBrokerPlaceOrderQuery(binding strategyInstanceBinding, symbol string) broker.PlaceOrderQuery {
	readQuery := strategyRuntimeBrokerReadQuery(binding)
	if strings.TrimSpace(readQuery.Market) == "" {
		readQuery.Market = strategyRuntimeMarketFromSymbol(symbol, "")
	}
	return broker.PlaceOrderQuery{
		ReadQuery: readQuery,
		Symbol:    symbol,
	}
}

func strategyRuntimeBrokerID(binding strategyInstanceBinding) string {
	if binding.BrokerAccount == nil || strings.TrimSpace(binding.BrokerAccount.BrokerID) == "" {
		return "futu"
	}
	return strings.ToLower(strings.TrimSpace(binding.BrokerAccount.BrokerID))
}

func strategyRuntimeDefinitionID(instance managedStrategyInstance) string {
	definitionID := jftradeOptionalTypeAssertion[string](instance.Params["definitionId"])
	return strings.TrimSpace(definitionID)
}

func strategyRuntimeDisplayName(instance managedStrategyInstance, runner *strategySymbolRuntime) string {
	name := strings.TrimSpace(instance.Definition.Name)
	if name == "" && runner != nil {
		name = strings.TrimSpace(runner.name)
	}
	if name == "" {
		name = strings.TrimSpace(instance.Definition.StrategyID)
	}
	if name == "" {
		name = strings.TrimSpace(instance.ID)
	}
	return name
}

func strategyRuntimeSideLabel(side bbgotypes.SideType) string {
	if strings.EqualFold(string(side), string(bbgotypes.SideTypeSell)) {
		return "卖出"
	}
	return "买入"
}

func strategyRuntimeFormatPrice(value float64) string {
	if value <= 0 {
		return "-"
	}
	return strategyRuntimeFormatNumber(value)
}

func strategyRuntimeFormatNumber(value float64) string {
	return runtimecontrol.FormatNumber(value)
}

func strategyRuntimeIntervalDuration(interval bbgotypes.Interval) (duration time.Duration, ok bool) {
	defer func() {
		if recover() != nil {
			duration = 0
			ok = false
		}
	}()
	duration = interval.Duration()
	return duration, duration > 0
}

func strategyRuntimeBucketWindow(tradeTime time.Time, interval bbgotypes.Interval) (time.Time, time.Time) {
	duration := interval.Duration()
	start := tradeTime.UTC().Truncate(duration)
	end := start.Add(duration).Add(-time.Millisecond)
	return start, end
}

func strategyRuntimeTradeKLine(exchange bbgotypes.ExchangeName, symbol string, interval bbgotypes.Interval, trade bbgotypes.Trade, start time.Time, end time.Time) bbgotypes.KLine {
	quoteVolume := trade.QuoteQuantity
	if quoteVolume.Sign() <= 0 {
		quoteVolume = trade.Quantity.Mul(trade.Price)
	}
	kline := bbgotypes.KLine{
		Exchange:    exchange,
		Symbol:      symbol,
		StartTime:   bbgotypes.Time(start),
		EndTime:     bbgotypes.Time(end),
		Interval:    interval,
		Open:        trade.Price,
		Close:       trade.Price,
		High:        trade.Price,
		Low:         trade.Price,
		Volume:      trade.Quantity,
		QuoteVolume: quoteVolume,
		Closed:      false,
	}
	if strings.EqualFold(string(trade.Side), string(bbgotypes.SideTypeBuy)) {
		kline.TakerBuyBaseAssetVolume = trade.Quantity
		kline.TakerBuyQuoteAssetVolume = quoteVolume
	}
	if trade.ID > 0 {
		kline.LastTradeID = trade.ID
	}
	kline.NumberOfTrades = 1
	return kline
}

func cloneStrategyRuntimeFundsSnapshot(snapshot *broker.FundsSnapshot) *broker.FundsSnapshot {
	if snapshot == nil {
		return nil
	}
	copyValue := *snapshot
	copyValue.CurrencyBalances = append([]broker.CurrencyBalanceSnapshot(nil), snapshot.CurrencyBalances...)
	return &copyValue
}

func cloneStrategyRuntimePositions(positions []broker.PositionSnapshot) []broker.PositionSnapshot {
	if len(positions) == 0 {
		return nil
	}
	return append([]broker.PositionSnapshot(nil), positions...)
}

func strategyRuntimePositionToControl(position broker.PositionSnapshot) runtimecontrol.Position {
	return runtimecontrol.Position{
		Market:           position.Market,
		Symbol:           position.Symbol,
		Quantity:         position.Quantity,
		SellableQuantity: position.SellableQuantity,
	}
}

func strategyRuntimePositionsToControl(positions []broker.PositionSnapshot) []runtimecontrol.Position {
	if len(positions) == 0 {
		return nil
	}
	result := make([]runtimecontrol.Position, 0, len(positions))
	for _, position := range positions {
		result = append(result, strategyRuntimePositionToControl(position))
	}
	return result
}

func strategyRuntimePositionMatchesSymbol(position broker.PositionSnapshot, symbol string) bool {
	return runtimecontrol.PositionMatchesSymbol(strategyRuntimePositionToControl(position), symbol)
}

func buildStrategyRuntimeAccount(funds *broker.FundsSnapshot, positions []broker.PositionSnapshot, market bbgotypes.Market, symbol string) *bbgotypes.Account {
	account := bbgotypes.NewAccount()
	account.CanDeposit = true
	account.CanTrade = true
	account.CanWithdraw = true
	if funds != nil {
		account.RawAccount = funds
		if funds.TotalAssets != nil {
			account.TotalAccountValue = fixedpoint.NewFromFloat(*funds.TotalAssets)
		}
		for _, balance := range funds.CurrencyBalances {
			currency := strings.ToUpper(strings.TrimSpace(balance.Currency))
			if currency == "" {
				continue
			}
			entry := bbgotypes.NewZeroBalance(currency)
			if balance.NetCashPower != nil {
				entry.Available = fixedpoint.NewFromFloat(*balance.NetCashPower)
				entry.NetAsset = fixedpoint.NewFromFloat(*balance.NetCashPower)
			}
			if balance.Cash != nil && balance.NetCashPower == nil {
				entry.Available = fixedpoint.NewFromFloat(*balance.Cash)
				entry.NetAsset = fixedpoint.NewFromFloat(*balance.Cash)
			}
			if balance.AvailableWithdrawalCash != nil {
				entry.MaxWithdrawAmount = fixedpoint.NewFromFloat(*balance.AvailableWithdrawalCash)
			}
			account.SetBalance(currency, entry)
		}
		if len(funds.CurrencyBalances) == 0 {
			currency := strings.ToUpper(strings.TrimSpace(market.QuoteCurrency))
			if currency == "" && funds.Currency != nil {
				currency = strings.ToUpper(strings.TrimSpace(*funds.Currency))
			}
			if currency != "" {
				entry := bbgotypes.NewZeroBalance(currency)
				if funds.AvailableFunds != nil {
					entry.Available = fixedpoint.NewFromFloat(*funds.AvailableFunds)
					entry.NetAsset = fixedpoint.NewFromFloat(*funds.AvailableFunds)
				}
				if funds.MaxWithdrawal != nil {
					entry.MaxWithdrawAmount = fixedpoint.NewFromFloat(*funds.MaxWithdrawal)
				}
				account.SetBalance(currency, entry)
			}
		}
	}

	baseCurrency := strings.ToUpper(strings.TrimSpace(market.BaseCurrency))
	if baseCurrency != "" {
		baseEntry := bbgotypes.NewZeroBalance(baseCurrency)
		for _, position := range positions {
			if !strategyRuntimePositionMatchesSymbol(position, symbol) {
				continue
			}
			baseEntry.Available = baseEntry.Available.Add(fixedpoint.NewFromFloat(position.SellableQuantity))
			baseEntry.NetAsset = baseEntry.NetAsset.Add(fixedpoint.NewFromFloat(position.Quantity))
			lockedQuantity := position.Quantity - position.SellableQuantity
			if lockedQuantity > 0 {
				baseEntry.Locked = baseEntry.Locked.Add(fixedpoint.NewFromFloat(lockedQuantity))
			}
		}
		if baseEntry.Available.Sign() > 0 || baseEntry.Locked.Sign() > 0 || baseEntry.NetAsset.Sign() > 0 {
			account.SetBalance(baseCurrency, baseEntry)
		}
	}
	return account
}

func strategyRuntimeMarketFromSymbol(symbol string, defaultMarket string) string {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(normalized, ".") {
		parts := strings.SplitN(normalized, ".", 2)
		if strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if strings.Contains(normalized, ":") {
		parts := strings.SplitN(normalized, ":", 2)
		if strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.ToUpper(strings.TrimSpace(defaultMarket))
}

func strategyRuntimeStartError(err error) (int, string) {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "required"),
		strings.Contains(message, "missing"),
		strings.Contains(message, "invalid"),
		strings.Contains(message, "unsupported"),
		strings.Contains(message, "already running"):
		return 400, "BAD_REQUEST"
	default:
		return 502, "STRATEGY_RUNTIME_START_FAILED"
	}
}

func strategyRuntimeMaxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func strategyRuntimeSortedSymbols(symbols map[string]*strategySymbolRuntime) []string {
	result := make([]string, 0, len(symbols))
	for symbol := range symbols {
		result = append(result, symbol)
	}
	sort.Strings(result)
	return result
}

func strategyRuntimeMaxTime(left time.Time, right time.Time) time.Time {
	return runtimecontrol.MaxTime(left, right)
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
