import type {
  BacktestCandleDto,
  BacktestFeeRuleDto,
  BacktestFeeScheduleDto,
  BacktestOrderBookEntryDto,
  BacktestRunResultDto,
  BacktestRunStateDto,
  BacktestTradeEventDto,
  BacktestTradingCostsDto,
  RunModelTradingCostsDto,
} from "@/contracts";
import { normalizeChartType, type HeikinAshiSeed } from "@/charting/kline";
import type {
  BacktestFeeRulePayload,
  BacktestFeeSchedulePayload,
  BacktestTradingCostsPayload,
} from "@/types";
import type {
  BacktestCandleView,
  BacktestFeeBreakdownEntry,
  BacktestOrderBookEntry,
  BacktestRun,
  BacktestRunResult,
  BacktestTradeView,
} from "@/composables/backtest/backtestRunModels";

type BacktestDecimalTransport = string | number;
type BacktestTradeTransport = BacktestTradeEventDto;
type BacktestCandleTransport = BacktestCandleDto;
type BacktestOrderBookEntryTransport = BacktestOrderBookEntryDto;
type BacktestRunResultTransport = BacktestRunResultDto;
type BacktestRunTransport = BacktestRunStateDto;

export function normalizeDecimalTransport(value: BacktestDecimalTransport | undefined): {
  value?: number;
  text?: string;
} {
  if (value === undefined) {
    return {};
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) {
      return {};
    }
    return { value, text: String(value) };
  }
  const text = value.trim();
  if (text === "") {
    return {};
  }
  const parsed = Number(text);
  if (!Number.isFinite(parsed)) {
    return { text };
  }
  return { value: parsed, text };
}

export function normalizeTrade(trade: BacktestTradeTransport): BacktestTradeView {
  const price = normalizeDecimalTransport(trade.price);
  const qty = normalizeDecimalTransport(trade.qty);
  const normalized: BacktestTradeView = {
    time: trade.time ?? "",
    side: trade.side ?? "",
    price: price.value ?? 0,
    qty: qty.value ?? 0,
  };
  if (trade.pnl !== undefined) normalized.pnl = trade.pnl;
  if (trade.brokerFee !== undefined) normalized.brokerFee = trade.brokerFee;
  if (trade.marketFee !== undefined) normalized.marketFee = trade.marketFee;
  if (trade.totalFee !== undefined) normalized.totalFee = trade.totalFee;
  if (trade.feeCurrency !== undefined) normalized.feeCurrency = trade.feeCurrency;
  if (trade.warmup !== undefined) normalized.warmup = trade.warmup;
  if (price.text !== undefined) normalized.priceText = price.text;
  if (qty.text !== undefined) normalized.qtyText = qty.text;
  return normalized;
}

export function normalizeCandle(candle: BacktestCandleTransport): BacktestCandleView {
  const open = normalizeDecimalTransport(candle.open);
  const high = normalizeDecimalTransport(candle.high);
  const low = normalizeDecimalTransport(candle.low);
  const close = normalizeDecimalTransport(candle.close);
  const volume = normalizeDecimalTransport(candle.volume);
  return {
    time: candle.time ?? "",
    open: open.value ?? 0,
    high: high.value ?? 0,
    low: low.value ?? 0,
    close: close.value ?? 0,
    volume: volume.value ?? 0,
    openText: open.text,
    highText: high.text,
    lowText: low.text,
    closeText: close.text,
    volumeText: volume.text,
  };
}
export function normalizeHeikinAshiSeed(value: unknown): HeikinAshiSeed | undefined {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const raw = value as { open?: unknown; close?: unknown };
  const open = normalizeDecimalTransport(
    typeof raw.open === "string" || typeof raw.open === "number"
      ? raw.open
      : undefined,
  );
  const close = normalizeDecimalTransport(
    typeof raw.close === "string" || typeof raw.close === "number"
      ? raw.close
      : undefined,
  );
  if (open.value === undefined || close.value === undefined) {
    return undefined;
  }
  return { open: open.value, close: close.value };
}

export function normalizeOrderBookEntry(entry: BacktestOrderBookEntryTransport): BacktestOrderBookEntry {
  const quantity = normalizeDecimalTransport(entry.quantity);
  const orderPrice = normalizeDecimalTransport(entry.orderPrice);
  const filledQuantity = normalizeDecimalTransport(entry.filledQuantity);
  const filledPrice = normalizeDecimalTransport(entry.filledPrice);
  return {
    orderId: entry.orderId ?? "",
    clientOrderId: entry.clientOrderId,
    symbol: entry.symbol ?? "",
    side: entry.side ?? "",
    quantity: quantity.value ?? 0,
    quantityText: quantity.text,
    orderType: entry.orderType,
    orderPrice: orderPrice.value,
    orderPriceText: orderPrice.text,
    submittedAt: entry.submittedAt,
    status: entry.status ?? "",
    filledQuantity: filledQuantity.value,
    filledQuantityText: filledQuantity.text,
    filledPrice: filledPrice.value,
    filledPriceText: filledPrice.text,
    filledAt: entry.filledAt,
    brokerFee: entry.brokerFee,
    marketFee: entry.marketFee,
    totalFee: entry.totalFee,
    feeCurrency: entry.feeCurrency,
    warmup: entry.warmup,
  };
}

export function normalizeFeeCategory(
  value: string | undefined,
): BacktestFeeRulePayload["category"] {
  switch (value) {
    case "exchange":
    case "clearing":
    case "regulatory":
    case "tax":
      return value;
    default:
      return "broker";
  }
}

export function normalizeFeeBasis(
  value: string | undefined,
): BacktestFeeRulePayload["basis"] {
  switch (value) {
    case "share":
    case "order":
      return value;
    default:
      return "notional";
  }
}

export function normalizeFeeSide(
  value: string | undefined,
): BacktestFeeRulePayload["side"] {
  switch (value) {
    case "buy":
    case "sell":
    case "both":
      return value;
    default:
      return undefined;
  }
}

export function normalizeFeeRule(
  rule: BacktestFeeRuleDto,
): BacktestFeeRulePayload {
  const side = normalizeFeeSide(rule.side);
  return {
    id: rule.id ?? "",
    ...(rule.label == null ? {} : { label: rule.label }),
    category: normalizeFeeCategory(rule.category),
    ...(side == null ? {} : { side }),
    basis: normalizeFeeBasis(rule.basis),
    ...(rule.rate == null ? {} : { rate: rule.rate }),
    ...(rule.fixedAmount == null ? {} : { fixedAmount: rule.fixedAmount }),
    ...(rule.minAmount == null ? {} : { minAmount: rule.minAmount }),
    ...(rule.maxAmount == null ? {} : { maxAmount: rule.maxAmount }),
    ...(rule.maxRate == null ? {} : { maxRate: rule.maxRate }),
    ...(rule.rounding == null ? {} : { rounding: rule.rounding }),
    ...(rule.currency == null ? {} : { currency: rule.currency }),
    ...(rule.appliesTo == null ? {} : { appliesTo: rule.appliesTo }),
    ...(rule.effectiveFrom == null ? {} : { effectiveFrom: rule.effectiveFrom }),
    ...(rule.effectiveTo == null ? {} : { effectiveTo: rule.effectiveTo }),
    ...(rule.sourceUrl == null ? {} : { sourceUrl: rule.sourceUrl }),
  };
}

export function normalizeFeeSchedule(
  schedule: BacktestFeeScheduleDto | undefined,
): BacktestFeeSchedulePayload | undefined {
  if (schedule == null) return undefined;
  const mode =
    schedule.mode === "market_preset" ||
    schedule.mode === "custom" ||
    schedule.mode === "script" ||
    schedule.mode === "none"
      ? schedule.mode
      : undefined;
  return {
    ...(mode == null ? {} : { mode }),
    ...(schedule.presetId == null ? {} : { presetId: schedule.presetId }),
    ...(schedule.rules == null
      ? {}
      : { rules: schedule.rules.map(normalizeFeeRule) }),
  };
}

export function normalizeTradingCosts(
  costs:
    | BacktestTradingCostsDto
    | RunModelTradingCostsDto
    | undefined,
): BacktestTradingCostsPayload | undefined {
  if (costs == null) return undefined;
  const brokerFees = normalizeFeeSchedule(costs.brokerFees);
  const marketFees = normalizeFeeSchedule(costs.marketFees);
  return {
    ...(brokerFees == null ? {} : { brokerFees }),
    ...(marketFees == null ? {} : { marketFees }),
  };
}

export function normalizeRunResult(result: BacktestRunResultTransport): BacktestRunResult {
  const rawResult = result as {
    chartType?: unknown;
    heikinAshiSeed?: unknown;
  };
  const hasResultChartType = Object.prototype.hasOwnProperty.call(
    rawResult,
    "chartType",
  );
  const trades = result.trades?.map(normalizeTrade);
  const orderBook = result.orderBook?.map(normalizeOrderBookEntry);
  const candles = result.candles?.map(normalizeCandle);
  const feeBreakdown = result.feeBreakdown?.map((entry) => ({
    ruleId: entry.ruleId ?? "",
    label: entry.label ?? "",
    group: entry.group ?? "",
    category: entry.category ?? "",
    currency: entry.currency ?? "",
    amount: entry.amount ?? 0,
    count: entry.count ?? 0,
  }));
  const pnlCurve = result.pnlCurve?.map((point) => ({
    time: point.time ?? "",
    equity: point.equity ?? 0,
  }));
  const drawdownCurve = result.drawdownCurve?.map((point) => ({
    time: point.time ?? "",
    drawdown: point.drawdown ?? 0,
  }));
  const tradingCosts = normalizeTradingCosts(result.tradingCosts);
  const heikinAshiSeed = normalizeHeikinAshiSeed(rawResult.heikinAshiSeed);
  return {
    symbol: result.symbol ?? "",
    marketDataProvider: (result as { marketDataProvider?: string }).marketDataProvider,
    interval: result.interval ?? "",
    ...(hasResultChartType
      ? { chartType: normalizeChartType(rawResult.chartType) }
      : {}),
    ...(heikinAshiSeed == null ? {} : { heikinAshiSeed }),
    startTime: result.startTime ?? "",
    endTime: result.endTime ?? "",
    quoteCurrency: result.quoteCurrency,
    finalBalance: result.finalBalance ?? 0,
    pnl: result.pnl ?? 0,
    totalBrokerFees: result.totalBrokerFees,
    totalMarketFees: result.totalMarketFees,
    totalFees: result.totalFees,
    feeBreakdown,
    tradingCosts,
    maxDrawdown: result.maxDrawdown,
    currentDrawdown: result.currentDrawdown,
    tradeStatsVersion: result.tradeStatsVersion,
    totalFills: result.totalFills,
    totalTrades: result.totalTrades ?? 0,
    winRate: result.winRate ?? 0,
    trades,
    orderBook,
    candles,
    pnlCurve,
    drawdownCurve,
    warningTotal: result.warningTotal,
    warningsTruncated: result.warningsTruncated,
    ignoredOrders: result.ignoredOrders,
    executionModel:
      result.executionModel === "conservative-bar-v1"
        ? result.executionModel
        : undefined,
    runtimeErrors: result.runtimeErrors,
    runtimeErrorCounts: result.runtimeErrorCounts,
    runtimeErrorTotal: result.runtimeErrorTotal,
    runtimeErrorsTruncated: result.runtimeErrorsTruncated,
    warnings: result.warnings,
    logs: result.logs,
    error: result.error,
  };
}

export function normalizeRun(run: BacktestRunTransport): BacktestRun {
  const request = run.request ?? {};
  const tradingCosts = normalizeTradingCosts(request.tradingCosts);
  return {
    id: run.id ?? "",
    status: run.status ?? "",
    marketDataProvider: (run as { marketDataProvider?: string }).marketDataProvider,
    request: {
      definitionId: request.definitionId ?? "",
      ...(request.definitionVersion == null
        ? {}
        : { definitionVersion: request.definitionVersion }),
      ...(request.market == null ? {} : { market: request.market }),
      ...(request.code == null ? {} : { code: request.code }),
      symbol: request.symbol ?? "",
      ...(request.instrumentType == null
        ? {}
        : { instrumentType: request.instrumentType }),
      interval: request.interval ?? "",
      chartType: normalizeChartType((request as { chartType?: unknown }).chartType),
      ...(request.startDate == null ? {} : { startDate: request.startDate }),
      ...(request.endDate == null ? {} : { endDate: request.endDate }),
      startTime: request.startTime ?? "",
      endTime: request.endTime ?? "",
      ...(request.marketTimezone == null
        ? {}
        : { marketTimezone: request.marketTimezone }),
      initialBalance: request.initialBalance ?? 0,
      ...(request.rehabType == null ? {} : { rehabType: request.rehabType }),
      ...(request.useExtendedHours == null
        ? {}
        : { useExtendedHours: request.useExtendedHours }),
      ...(tradingCosts == null ? {} : { tradingCosts }),
      ...(request.executionModel === "conservative-bar-v1"
        ? { executionModel: request.executionModel }
        : {}),
    },
    result: run.result ? normalizeRunResult(run.result) : undefined,
    createdAt: run.createdAt ?? "",
    updatedAt: run.updatedAt ?? "",
  };
}
