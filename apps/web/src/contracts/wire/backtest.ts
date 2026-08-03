import type { components } from "@/generated/openapi";

export type BacktestStartRequestDto =
  components["schemas"]["backtest.StartRequest"];

export type BacktestRunResultDto =
  components["schemas"]["backtest.RunResult"];

export type BacktestRunStateDto =
  components["schemas"]["backtest.RunState"];

export type BacktestTradingCostsDto =
  components["schemas"]["backtest.TradingCosts"];

export type BacktestFeeRuleDto =
  components["schemas"]["runmodel.FeeRule"];

export type BacktestFeeScheduleDto =
  components["schemas"]["runmodel.FeeSchedule"];

export type BacktestTradeEventDto =
  components["schemas"]["runmodel.TradeEvent"];

export type BacktestCandleDto = components["schemas"]["runmodel.Candle"];

export type BacktestOrderBookEntryDto =
  components["schemas"]["runmodel.OrderBookEntry"];

export type RunModelTradingCostsDto =
  components["schemas"]["runmodel.TradingCosts"];

export type BacktestSyncRequestDto =
  components["schemas"]["backtest.SyncRequest"];
