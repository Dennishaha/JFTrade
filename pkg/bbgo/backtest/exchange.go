package backtest

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/service"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type Exchange struct {
	sourceName       types.ExchangeName
	publicExchange   types.Exchange
	srv              service.BackTestable
	currentTime      time.Time
	account          *types.Account
	config           *bbgo.Backtest
	markets          types.MarketMap
	MarketDataStream types.StandardStreamEmitter
	Src              *ExchangeDataSource
	pendingKLine     *types.KLine
}

type ExchangeDataSource struct {
	C         chan types.KLine
	Exchange  *Exchange
	Session   *bbgo.ExchangeSession
	Callbacks []func(types.KLine, *ExchangeDataSource)
}

func NewExchange(sourceName types.ExchangeName, sourceExchange types.Exchange, srv service.BackTestable, config *bbgo.Backtest) (*Exchange, error) {
	if sourceExchange == nil {
		return nil, fmt.Errorf("source exchange is required")
	}
	if config == nil {
		config = &bbgo.Backtest{}
	}
	markets, err := sourceExchange.QueryMarkets(context.Background())
	if err != nil {
		return nil, err
	}
	accountConfig := config.GetAccount(sourceName.String())
	account := types.NewAccount()
	account.MakerFeeRate = accountConfig.MakerFeeRate
	account.TakerFeeRate = accountConfig.TakerFeeRate
	account.AccountType = types.AccountTypeSpot
	account.UpdateBalances(accountConfig.Balances.BalanceMap())
	return &Exchange{
		sourceName:     sourceName,
		publicExchange: sourceExchange,
		srv:            srv,
		config:         config,
		account:        account,
		currentTime:    config.StartTime.Time(),
		markets:        markets,
	}, nil
}

func (e *Exchange) Name() types.ExchangeName {
	return types.ExchangeBacktest
}

func (e *Exchange) PlatformFeeCurrency() string {
	if e.publicExchange != nil {
		return e.publicExchange.PlatformFeeCurrency()
	}
	return ""
}

func (e *Exchange) NewStream() types.Stream {
	return &types.BacktestStream{StandardStreamEmitter: &types.StandardStream{}}
}

func (e *Exchange) Prepare(*bbgo.Config) error {
	return nil
}

func (e *Exchange) BindUserData(stream types.StandardStreamEmitter) {
	// JFTrade drives order and trade events through its own order executor.
}

func (e *Exchange) CloseMarketData() error {
	return nil
}

func (e *Exchange) ConsumeKLine(kline types.KLine, interval types.Interval) {
	kline.Interval = interval
	kline.Closed = true
	if e.pendingKLine == nil {
		e.pendingKLine = &kline
		return
	}

	emitKLine := *e.pendingKLine
	e.currentTime = emitKLine.EndTime.Time()
	if e.MarketDataStream != nil {
		e.MarketDataStream.EmitKLineClosed(emitKLine)
	}
	if e.Src != nil {
		for _, callback := range e.Src.Callbacks {
			callback(emitKLine, e.Src)
		}
	}
	e.pendingKLine = &kline
}

func (e *Exchange) QueryMarkets(context.Context) (types.MarketMap, error) {
	out := types.MarketMap{}
	maps.Copy(out, e.markets)
	return out, nil
}

func (e *Exchange) QueryTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	return e.publicExchange.QueryTicker(ctx, symbol)
}

func (e *Exchange) QueryTickers(ctx context.Context, symbols ...string) (map[string]types.Ticker, error) {
	return e.publicExchange.QueryTickers(ctx, symbols...)
}

func (e *Exchange) QueryKLines(ctx context.Context, symbol string, interval types.Interval, options types.KLineQueryOptions) ([]types.KLine, error) {
	return e.publicExchange.QueryKLines(ctx, symbol, interval, options)
}

func (e *Exchange) QueryAccount(context.Context) (*types.Account, error) {
	return e.account, nil
}

func (e *Exchange) QueryAccountBalances(context.Context) (types.BalanceMap, error) {
	if e.account == nil {
		return types.BalanceMap{}, nil
	}
	return e.account.Balances(), nil
}

func (e *Exchange) SubmitOrder(context.Context, types.SubmitOrder) (*types.Order, error) {
	return nil, fmt.Errorf("backtest exchange native submit order is not enabled")
}

func (e *Exchange) QueryOpenOrders(context.Context, string) ([]types.Order, error) {
	return nil, nil
}

func (e *Exchange) CancelOrders(context.Context, ...types.Order) error {
	return nil
}
