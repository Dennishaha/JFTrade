package backtest

import internalrunmodel "github.com/jftrade/jftrade-main/pkg/backtest/internal/runmodel"

type RunConfig = internalrunmodel.RunConfig
type InstrumentSpec = internalrunmodel.InstrumentSpec
type TradeEvent = internalrunmodel.TradeEvent
type OrderBookEntry = internalrunmodel.OrderBookEntry
type PnLPoint = internalrunmodel.PnLPoint
type DrawdownPoint = internalrunmodel.DrawdownPoint
type Candle = internalrunmodel.Candle
type HeikinAshiSeed = internalrunmodel.HeikinAshiSeed

// RunResult is the stable public backtest result contract.
type RunResult = internalrunmodel.RunResult // @name backtest.RunResult
type TradingCosts = internalrunmodel.TradingCosts
type FeeSchedule = internalrunmodel.FeeSchedule
type FeeRule = internalrunmodel.FeeRule
type FeeBreakdownEntry = internalrunmodel.FeeBreakdownEntry
