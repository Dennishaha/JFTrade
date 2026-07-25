import { useQuery } from "@tanstack/vue-query";
import { computed, onScopeDispose, reactive, ref, type ComputedRef } from "vue";

import type {
  BacktestFeeRulePayload,
  BacktestFeeSchedulePayload,
  BacktestStartRequestPayload,
  BacktestSyncRequestPayload,
  BacktestTradingCostsPayload,
} from "@/contracts";
import type { components } from "@/generated/openapi";

import {
  normalizeChartType,
  type ChartType,
  type HeikinAshiSeed,
} from "../charting/kline";
import type { BacktestTrade, BacktestPnlPoint, BacktestDrawdownPoint, BacktestCandle } from "../components/BacktestChart.vue";
import { apiGet, fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { queryClient, queryKeys } from "./serverState";
import { useKlineSyncTask } from "./useKlineSyncTask";

type BacktestDecimalTransport = string | number;

interface BacktestTradeView extends BacktestTrade {
  priceText?: string | undefined;
  qtyText?: string | undefined;
}

type BacktestFeeMode = NonNullable<BacktestFeeSchedulePayload["mode"]>;

interface BacktestCandleView extends BacktestCandle {
  openText?: string | undefined;
  highText?: string | undefined;
  lowText?: string | undefined;
  closeText?: string | undefined;
  volumeText?: string | undefined;
}

interface BacktestOrderBookEntry {
  orderId: string;
  clientOrderId?: string | undefined;
  symbol: string;
  side: string;
  quantity: number;
  quantityText?: string | undefined;
  orderType?: string | undefined;
  orderPrice?: number | undefined;
  orderPriceText?: string | undefined;
  submittedAt?: string | undefined;
  status: string;
  filledQuantity?: number | undefined;
  filledQuantityText?: string | undefined;
  filledPrice?: number | undefined;
  filledPriceText?: string | undefined;
  filledAt?: string | undefined;
  brokerFee?: number | undefined;
  marketFee?: number | undefined;
  totalFee?: number | undefined;
  feeCurrency?: string | undefined;
  warmup?: boolean | undefined;
}

type BacktestTradeTransport = components["schemas"]["runmodel.TradeEvent"];
type BacktestCandleTransport = components["schemas"]["runmodel.Candle"];
type BacktestOrderBookEntryTransport =
  components["schemas"]["runmodel.OrderBookEntry"];

interface BacktestFeeBreakdownEntry {
  ruleId: string;
  label: string;
  group: string;
  category: string;
  currency: string;
  amount: number;
  count: number;
}

interface BacktestRunResult {
  symbol: string;
  interval: string;
  chartType?: ChartType | undefined;
  startTime: string;
  endTime: string;
  quoteCurrency?: string | undefined;
  finalBalance: number;
  pnl: number;
  totalBrokerFees?: number | undefined;
  totalMarketFees?: number | undefined;
  totalFees?: number | undefined;
  feeBreakdown?: BacktestFeeBreakdownEntry[] | undefined;
  tradingCosts?: BacktestTradingCostsPayload | undefined;
  maxDrawdown?: number | undefined;
  currentDrawdown?: number | undefined;
  tradeStatsVersion?: number | undefined;
  totalFills?: number | undefined;
  totalTrades: number;
  winRate: number;
  trades?: BacktestTradeView[] | undefined;
  orderBook?: BacktestOrderBookEntry[] | undefined;
  pnlCurve?: BacktestPnlPoint[] | undefined;
  drawdownCurve?: BacktestDrawdownPoint[] | undefined;
  candles?: BacktestCandleView[] | undefined;
  heikinAshiSeed?: HeikinAshiSeed | undefined;
  logs?: string[] | undefined;
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
  warningsTruncated?: boolean | undefined;
  ignoredOrders?: number | undefined;
  executionModel?: "conservative-bar-v1" | undefined;
  runtimeErrors?: string[] | undefined;
  runtimeErrorCounts?: Record<string, number> | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
  error?: string | undefined;
}

type BacktestRunResultTransport = components["schemas"]["backtest.RunResult"];

interface BacktestRun {
  id: string;
  status: string;
  request: {
    definitionId: string;
    definitionVersion?: string;
    market?: string;
    code?: string;
    symbol: string;
    instrumentType?: string;
    interval: string;
    chartType: ChartType;
    startDate?: string;
    endDate?: string;
    startTime: string;
    endTime: string;
    marketTimezone?: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
    tradingCosts?: BacktestTradingCostsPayload;
    executionModel?: "conservative-bar-v1";
  };
  result?: BacktestRunResult | undefined;
  createdAt: string;
  updatedAt: string;
}

type BacktestRunTransport = components["schemas"]["backtest.RunState"];

export interface BacktestFormState {
  definitionId: string;
  definitionVersion: string;
  market: string;
  code: string;
  instrumentId: string;
  instrumentType: string;
  interval: string;
  chartType: ChartType;
  startDate: string;
  endDate: string;
  initialBalance: number;
  rehabType: string;
  useExtendedHours: boolean;
  brokerFeeMode: "market_preset" | "custom" | "script" | "none";
  marketFeeMode: "market_preset" | "custom" | "none";
  brokerFeeRules: BacktestFeeRulePayload[];
  marketFeeRules: BacktestFeeRulePayload[];
}

interface UseBacktestRunsOptions {
  formState: ComputedRef<BacktestFormState>;
  normalizeInstrument: (
    input: Pick<BacktestFormState, "market" | "code" | "instrumentId">,
  ) => Promise<{ market: string; prefix: string; code: string; instrumentId: string }>;
}

async function resolveBacktestInstrumentPayload(
  formState: Pick<BacktestFormState, "market" | "code" | "instrumentId">,
  resolver: UseBacktestRunsOptions["normalizeInstrument"],
): Promise<{ market: string; code: string; symbol: string } | null> {
  const normalized = await resolver(formState);
  const market = normalized.prefix.trim().toUpperCase();
  const code = normalized.code.trim().toUpperCase();
  const symbol = normalized.instrumentId.trim().toUpperCase();
  if (market === "" || code === "" || symbol === "") {
    return null;
  }
  return { market, code, symbol };
}

function resolveSyncSessionScope(formState: Pick<BacktestFormState, "useExtendedHours">): "regular" | "extended" {
  return formState.useExtendedHours ? "extended" : "regular";
}

export function buildBacktestStartRequestPayload(
  formState: BacktestFormState,
  instrument: { market: string; code: string; symbol: string },
): BacktestStartRequestPayload {
  return {
    definitionId: formState.definitionId,
    definitionVersion: formState.definitionVersion,
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.symbol,
    instrumentType: normalizeBacktestInstrumentType(formState.instrumentType),
    interval: formState.interval,
    chartType: normalizeChartType(formState.chartType),
    startDate: formState.startDate,
    endDate: formState.endDate,
    initialBalance: formState.initialBalance,
    rehabType: formState.rehabType,
    useExtendedHours: formState.useExtendedHours,
    tradingCosts: buildBacktestTradingCostsPayload(formState, instrument.market),
    executionModel: "conservative-bar-v1",
  };
}

function normalizeBacktestInstrumentType(value: string): "stock" | "etf" {
  return value.trim().toLowerCase() === "etf" ? "etf" : "stock";
}

function presetIdForMarket(market: string, group: "broker" | "market"): string {
  const normalized = market.trim().toUpperCase();
  if (group === "broker") {
    if (normalized === "HK") return "futu_hk_hk_stock_2026_06_30";
    if (normalized === "US") return "futu_hk_us_stock_2026_06_30";
    return "";
  }
  if (normalized === "HK") return "hkex_hk_stock_2026_06_30";
  if (normalized === "US") return "us_stock_market_fees_2026_06_30";
  if (normalized === "CN" || normalized === "SH" || normalized === "SZ") {
    return "stock_connect_a_share_market_fees_2026_06_30";
  }
  return "";
}

function buildBacktestTradingCostsPayload(
  formState: BacktestFormState,
  market: string,
): BacktestTradingCostsPayload {
  return {
    brokerFees: buildFeeSchedulePayload(formState.brokerFeeMode, presetIdForMarket(market, "broker"), formState.brokerFeeRules),
    marketFees: buildFeeSchedulePayload(formState.marketFeeMode, presetIdForMarket(market, "market"), formState.marketFeeRules),
  };
}

function buildFeeSchedulePayload(
  mode: BacktestFeeMode,
  presetId: string,
  rules: BacktestFeeRulePayload[],
): BacktestFeeSchedulePayload {
  const schedule: BacktestFeeSchedulePayload = { mode };
  if (mode === "market_preset" && presetId !== "") {
    schedule.presetId = presetId;
  }
  if (mode === "custom") {
    schedule.rules = rules;
  }
  return schedule;
}

export function buildBacktestSyncRequestPayload(
  formState: BacktestFormState,
  instrument: { market: string; code: string; symbol: string },
): BacktestSyncRequestPayload {
  return {
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.symbol,
    intervals: [formState.interval],
    startDate: formState.startDate,
    endDate: formState.endDate,
    rehabType: formState.rehabType,
    sessionScope: resolveSyncSessionScope(formState),
  };
}

export function useBacktestRuns(options: UseBacktestRunsOptions) {
  const running = ref(false);
  const polling = ref<ReturnType<typeof setTimeout> | null>(null);
  const error = ref("");
  let pollingGeneration = 0;
  let disposed = false;
  const {
    syncing,
    syncProgress,
    syncError,
    startSync,
    cancelSync: cancelKlineSync,
  } = useKlineSyncTask();

  const expandedRuns = reactive<Record<string, boolean>>({});
  const detailLoading = reactive<Record<string, boolean>>({});
  const detailErrors = reactive<Record<string, string>>({});
  const backtestRunsQueryKey = queryKeys.backtestRuns();
  const runsQuery = useQuery({
    queryKey: backtestRunsQueryKey,
    queryFn: fetchBacktestRuns,
    enabled: false,
  }, queryClient);

  const runs = computed(() => runsQuery.data.value ?? []);

  onScopeDispose(() => {
    disposed = true;
    stopPolling();
  }, true);

  const filteredRuns = computed(() =>
    [...runs.value].sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    ),
  );

  async function toggleRun(runId: string) {
    expandedRuns[runId] = true;
    const current = runs.value.find((run) => run.id === runId);
    if (current?.result) {
      return;
    }
    if (detailLoading[runId]) {
      return;
    }
    detailLoading[runId] = true;
    detailErrors[runId] = "";
    try {
      const detail = await fetchEnvelope<BacktestRunTransport>(
        `/api/v1/backtests/${encodeURIComponent(runId)}`,
      );
      const normalized = normalizeRun(detail);
      queryClient.setQueryData(queryKeys.backtestRun(runId), normalized);
      patchBacktestRuns([normalized]);
    } catch (cause) {
      detailErrors[runId] = `加载回测详情失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    } finally {
      detailLoading[runId] = false;
    }
  }

  function normalizeDecimalTransport(value: BacktestDecimalTransport | undefined): {
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

  function normalizeTrade(trade: BacktestTradeTransport): BacktestTradeView {
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

  function normalizeCandle(candle: BacktestCandleTransport): BacktestCandleView {
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

  function normalizeHeikinAshiSeed(value: unknown): HeikinAshiSeed | undefined {
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

  function normalizeOrderBookEntry(entry: BacktestOrderBookEntryTransport): BacktestOrderBookEntry {
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

  function normalizeFeeCategory(
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

  function normalizeFeeBasis(
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

  function normalizeFeeSide(
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

  function normalizeFeeRule(
    rule: components["schemas"]["runmodel.FeeRule"],
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

  function normalizeFeeSchedule(
    schedule: components["schemas"]["runmodel.FeeSchedule"] | undefined,
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

  function normalizeTradingCosts(
    costs:
      | components["schemas"]["backtest.TradingCosts"]
      | components["schemas"]["runmodel.TradingCosts"]
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

  function normalizeRunResult(result: BacktestRunResultTransport): BacktestRunResult {
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

  function normalizeRun(run: BacktestRunTransport): BacktestRun {
    const request = run.request ?? {};
    const tradingCosts = normalizeTradingCosts(request.tradingCosts);
    return {
      id: run.id ?? "",
      status: run.status ?? "",
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

  function pickPreferredRun(existingRun: BacktestRun, candidateRun: BacktestRun): BacktestRun {
    const existingUpdatedAt = new Date(existingRun.updatedAt).getTime();
    const candidateUpdatedAt = new Date(candidateRun.updatedAt).getTime();

    if (Number.isFinite(candidateUpdatedAt) && Number.isFinite(existingUpdatedAt)) {
      if (candidateUpdatedAt > existingUpdatedAt) {
        return candidateRun;
      }
      if (candidateUpdatedAt < existingUpdatedAt) {
        return existingRun;
      }
    }

    if (candidateRun.result && !existingRun.result) {
      return candidateRun;
    }
    if (existingRun.result && !candidateRun.result) {
      return existingRun;
    }
    if (candidateRun.status === "completed" && existingRun.status !== "completed") {
      return candidateRun;
    }
    return candidateRun;
  }

  function mergeRunsById(nextRuns: BacktestRun[]): BacktestRun[] {
    const merged = new Map<string, BacktestRun>();

    for (const run of nextRuns) {
      const existing = merged.get(run.id);
      merged.set(run.id, existing ? pickPreferredRun(existing, run) : run);
    }

    return Array.from(merged.values());
  }

  function patchBacktestRuns(nextRuns: BacktestRun[]): void {
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) => mergeRunsById([...(current ?? []), ...nextRuns]),
    );
  }

  function patchBacktestRunStatus(runId: string, status: string): void {
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) =>
        (current ?? []).map((run) =>
          run.id === runId ? { ...run, status } : run,
        ),
    );
    queryClient.setQueryData<BacktestRun | undefined>(
      queryKeys.backtestRun(runId),
      (current) => current == null ? current : { ...current, status },
    );
  }

  async function fetchBacktestRuns(): Promise<BacktestRun[]> {
    const data = await apiGet("/api/v1/backtests");
    return (data.runs ?? []).map(normalizeRun);
  }

  async function refreshRuns(): Promise<void> {
    const data = await fetchBacktestRuns();
    patchBacktestRuns(data);
  }

  async function loadRuns() {
    try {
      const data = await queryClient.ensureQueryData({
        queryKey: backtestRunsQueryKey,
        queryFn: fetchBacktestRuns,
      });
      patchBacktestRuns(data);
    } catch {
      // backtests may not be available yet
    }
  }

  async function deleteRun(runId: string): Promise<boolean> {
    const normalizedRunID = runId.trim();
    if (normalizedRunID === "") {
      return false;
    }

    const targetRun = runs.value.find((run) => run.id === normalizedRunID);
    const isTerminal = targetRun != null && isTerminalBacktestStatus(targetRun.status);
    if (!isTerminal) {
      return false;
    }

    error.value = "";
    try {
      const result = await fetchEnvelopeWithInit<{ deleted: boolean; id: string }>(
        `/api/v1/backtests/${encodeURIComponent(normalizedRunID)}`,
        {
          method: "DELETE",
        },
      );
      if (!result.deleted) {
        throw new Error("服务端未确认删除");
      }
    } catch (cause) {
      error.value = `删除回测记录失败: ${cause instanceof Error ? cause.message : String(cause)}`;
      return false;
    }

    delete expandedRuns[normalizedRunID];
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) => (current ?? []).filter((run) => run.id !== normalizedRunID),
    );
    return true;
  }

  async function syncKlines() {
    const formState = options.formState.value;
    error.value = "";

    try {
      const instrument = await resolveBacktestInstrumentPayload(
        formState,
        options.normalizeInstrument,
      );
      if (instrument == null) {
        error.value = "同步启动失败: 请先输入有效的市场与代码";
        return;
      }
      const payload = buildBacktestSyncRequestPayload(formState, instrument);
      await startSync(payload);
      if (syncError.value !== "") {
        error.value = syncError.value;
      }
    } catch (cause) {
      error.value = `同步启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    }
  }

  async function cancelSync() {
    await cancelKlineSync();
    if (syncProgress.value) {
      syncProgress.value.status = "cancelled";
    }
  }

  async function startBacktest() {
    const formState = options.formState.value;
    if (!formState.definitionId) return;

    running.value = true;
    error.value = "";
    try {
      const instrument = await resolveBacktestInstrumentPayload(
        formState,
        options.normalizeInstrument,
      );
      if (instrument == null) {
        error.value = "启动回测失败: 请先输入有效的市场与代码";
        return;
      }
      const payload = buildBacktestStartRequestPayload(formState, instrument);
      const data = await fetchEnvelopeWithInit<{ id: string; status: string }>(
        "/api/v1/backtests",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );
      startPolling(data.id);
      await queryClient.invalidateQueries({ queryKey: backtestRunsQueryKey });
      await refreshRuns();
    } catch (cause) {
      error.value = `启动回测失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    } finally {
      running.value = false;
    }
  }

  function startPolling(runId: string) {
    stopPolling();
    if (disposed) {
      return;
    }

    const generation = pollingGeneration;
    let consecutiveFailures = 0;
    const poll = async () => {
      if (disposed || generation !== pollingGeneration) {
        return;
      }
      try {
        const data = await fetchEnvelope<{ id: string; status: string }>(
          `/api/v1/backtests/${runId}/status`,
        );
        if (disposed || generation !== pollingGeneration) {
          return;
        }
        consecutiveFailures = 0;
        patchBacktestRunStatus(runId, data.status);
        if (isTerminalBacktestStatus(data.status)) {
          stopPolling();
          await queryClient.invalidateQueries({ queryKey: backtestRunsQueryKey });
          if (disposed) {
            return;
          }
          await refreshRuns();
          return;
        }
      } catch (cause) {
        if (disposed || generation !== pollingGeneration) {
          return;
        }
        consecutiveFailures += 1;
        if (consecutiveFailures >= 3) {
          stopPolling();
          error.value = `回测状态轮询失败: ${cause instanceof Error ? cause.message : String(cause)}`;
          return;
        }
      }

      if (!disposed && generation === pollingGeneration) {
        polling.value = setTimeout(() => void poll(), 2000);
      }
    };
    polling.value = setTimeout(() => void poll(), 2000);
  }

  function stopPolling() {
    pollingGeneration += 1;
    if (polling.value !== null) {
      clearTimeout(polling.value);
      polling.value = null;
    }
  }

  return {
    runs,
    running,
    syncing,
    syncProgress,
    error,
    expandedRuns,
    detailLoading,
    detailErrors,
    filteredRuns,
    toggleRun,
    deleteRun,
    loadRuns,
    syncKlines,
    cancelSync,
    startBacktest,
  };
}

function isTerminalBacktestStatus(status: string | undefined): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}
