package backtest

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

type backtestSourceExchange struct {
	market types.Market
}

func newBacktestSourceExchange(spec InstrumentSpec) *backtestSourceExchange {
	return &backtestSourceExchange{market: marketFromInstrumentSpec(spec)}
}

func resolveInstrumentSpec(symbol string, input InstrumentSpec) InstrumentSpec {
	input.Symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if profile, ok := marketpkg.ProfileForSymbol(symbol); ok {
		if input.QuoteCurrency == "" {
			input.QuoteCurrency = profile.QuoteCurrency
		}
		if input.PricePrecision == 0 {
			input.PricePrecision = profile.PricePrecision
		}
		if input.QuotePrecision == 0 {
			input.QuotePrecision = profile.QuotePrecision
		}
		if input.TickSize <= 0 {
			input.TickSize = profile.TickSize
		}
	}
	if input.TickSize <= 0 {
		input.TickSize = 0.01
	}
	if input.QuoteCurrency == "" {
		input.QuoteCurrency = resolveBacktestQuoteCurrency(symbol, "")
	}
	if input.LotSize <= 0 {
		switch {
		case strings.HasPrefix(input.Symbol, "SH."), strings.HasPrefix(input.Symbol, "SZ."):
			input.LotSize = 100
		case strings.HasPrefix(input.Symbol, "HK."):
			input.LotSize = 1
			input.MissingCriticalRules = true
			input.Warnings = append(input.Warnings, fmt.Sprintf("market rule warning: lot size unavailable for %s; orders are ignored until a provider supplies quantity rules", input.Symbol))
		default:
			input.LotSize = 1
		}
	}
	if input.QuantityStep <= 0 {
		input.QuantityStep = input.LotSize
	}
	return input
}

func marketFromInstrumentSpec(spec InstrumentSpec) types.Market {
	return types.Market{
		Exchange: types.ExchangeBacktest, Symbol: spec.Symbol, LocalSymbol: spec.Symbol,
		PricePrecision: spec.PricePrecision, QuotePrecision: spec.QuotePrecision,
		VolumePrecision: spec.VolumePrecision, QuoteCurrency: spec.QuoteCurrency,
		MinQuantity: fixedpoint.NewFromFloat(spec.LotSize),
		StepSize:    fixedpoint.NewFromFloat(spec.QuantityStep),
		TickSize:    fixedpoint.NewFromFloat(spec.TickSize),
	}
}

func (e *backtestSourceExchange) Name() types.ExchangeName    { return types.ExchangeBacktest }
func (e *backtestSourceExchange) PlatformFeeCurrency() string { return e.market.QuoteCurrency }
func (e *backtestSourceExchange) NewStream() types.Stream {
	stream := types.NewStandardStream()
	return &stream
}
func (e *backtestSourceExchange) QueryMarkets(context.Context) (types.MarketMap, error) {
	return types.MarketMap{e.market.Symbol: e.market}, nil
}
func (e *backtestSourceExchange) QueryTicker(context.Context, string) (*types.Ticker, error) {
	return nil, fmt.Errorf("backtest source does not query live tickers")
}
func (e *backtestSourceExchange) QueryTickers(context.Context, ...string) (map[string]types.Ticker, error) {
	return nil, fmt.Errorf("backtest source does not query live tickers")
}
func (e *backtestSourceExchange) QueryKLines(context.Context, string, types.Interval, types.KLineQueryOptions) ([]types.KLine, error) {
	return nil, fmt.Errorf("backtest source reads klines from the replay store")
}
func (e *backtestSourceExchange) QueryAccount(context.Context) (*types.Account, error) {
	return types.NewAccount(), nil
}
func (e *backtestSourceExchange) QueryAccountBalances(context.Context) (types.BalanceMap, error) {
	return types.BalanceMap{}, nil
}
func (e *backtestSourceExchange) SubmitOrder(context.Context, types.SubmitOrder) (*types.Order, error) {
	return nil, fmt.Errorf("backtest source does not submit orders")
}
func (e *backtestSourceExchange) QueryOpenOrders(context.Context, string) ([]types.Order, error) {
	return nil, nil
}
func (e *backtestSourceExchange) CancelOrders(context.Context, ...types.Order) error { return nil }
