import { useQuery } from "@tanstack/vue-query";
import { computed, markRaw, reactive, ref, type ComputedRef } from "vue";

import type {
  BacktestFeeRulePayload,
  BacktestFeeSchedulePayload,
  BacktestStartRequestPayload,
  BacktestSyncRequestPayload,
  BacktestTradingCostsPayload,
} from "@/contracts";

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
}

interface BacktestTradeTransport {
  time: string;
  side: string;
  price: BacktestDecimalTransport;
  qty: BacktestDecimalTransport;
  pnl?: number;
  brokerFee?: number;
  marketFee?: number;
  totalFee?: number;
  feeCurrency?: string;
}

interface BacktestCandleTransport {
  time: string;
  open: BacktestDecimalTransport;
  high: BacktestDecimalTransport;
  low: BacktestDecimalTransport;
  close: BacktestDecimalTransport;
  volume: BacktestDecimalTransport;
}

interface BacktestOrderBookEntryTransport {
  orderId: string;
  clientOrderId?: string | undefined;
  symbol: string;
  side: string;
  quantity: BacktestDecimalTransport;
  orderType?: string | undefined;
  orderPrice?: BacktestDecimalTransport | undefined;
  submittedAt?: string | undefined;
  status: string;
  filledQuantity?: BacktestDecimalTransport | undefined;
  filledPrice?: BacktestDecimalTransport | undefined;
  filledAt?: string | undefined;
  brokerFee?: number | undefined;
  marketFee?: number | undefined;
  totalFee?: number | undefined;
  feeCurrency?: string | undefined;
}

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
  totalTrades: number;
  winRate: number;
  trades?: BacktestTradeView[] | undefined;
  orderBook?: BacktestOrderBookEntry[] | undefined;
  pnlCurve?: BacktestPnlPoint[] | undefined;
  drawdownCurve?: BacktestDrawdownPoint[] | undefined;
  candles?: BacktestCandleView[] | undefined;
  logs?: string[] | undefined;
  runtimeErrors?: string[] | undefined;
  runtimeErrorCounts?: Record<string, number> | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
  error?: string | undefined;
}

interface BacktestRunResultTransport {
  symbol: string;
  interval: string;
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
  totalTrades: number;
  winRate: number;
  trades?: BacktestTradeTransport[] | undefined;
  orderBook?: BacktestOrderBookEntryTransport[] | undefined;
  pnlCurve?: BacktestPnlPoint[] | undefined;
  drawdownCurve?: BacktestDrawdownPoint[] | undefined;
  candles?: BacktestCandleTransport[] | undefined;
  logs?: string[] | undefined;
  runtimeErrors?: string[] | undefined;
  runtimeErrorCounts?: Record<string, number> | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
  error?: string | undefined;
}

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
    startDate?: string;
    endDate?: string;
    startTime: string;
    endTime: string;
    marketTimezone?: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
    tradingCosts?: BacktestTradingCostsPayload;
  };
  result?: BacktestRunResult | undefined;
  createdAt: string;
  updatedAt: string;
}

interface BacktestRunTransport {
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
    startDate?: string;
    endDate?: string;
    startTime: string;
    endTime: string;
    marketTimezone?: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
    tradingCosts?: BacktestTradingCostsPayload;
  };
  result?: BacktestRunResultTransport | undefined;
  createdAt: string;
  updatedAt: string;
}

export interface BacktestFormState {
  definitionId: string;
  definitionVersion: string;
  market: string;
  code: string;
  instrumentId: string;
  instrumentType: string;
  interval: string;
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
    startDate: formState.startDate,
    endDate: formState.endDate,
    initialBalance: formState.initialBalance,
    rehabType: formState.rehabType,
    useExtendedHours: formState.useExtendedHours,
    tradingCosts: buildBacktestTradingCostsPayload(formState, instrument.market),
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
  const polling = ref<ReturnType<typeof setInterval> | null>(null);
  const error = ref("");
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
      time: trade.time,
      side: trade.side,
      price: price.value ?? 0,
      qty: qty.value ?? 0,
    };
    if (trade.pnl !== undefined) normalized.pnl = trade.pnl;
    if (trade.brokerFee !== undefined) normalized.brokerFee = trade.brokerFee;
    if (trade.marketFee !== undefined) normalized.marketFee = trade.marketFee;
    if (trade.totalFee !== undefined) normalized.totalFee = trade.totalFee;
    if (trade.feeCurrency !== undefined) normalized.feeCurrency = trade.feeCurrency;
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
      time: candle.time,
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

  function normalizeOrderBookEntry(entry: BacktestOrderBookEntryTransport): BacktestOrderBookEntry {
    const quantity = normalizeDecimalTransport(entry.quantity);
    const orderPrice = normalizeDecimalTransport(entry.orderPrice);
    const filledQuantity = normalizeDecimalTransport(entry.filledQuantity);
    const filledPrice = normalizeDecimalTransport(entry.filledPrice);
    return {
      orderId: entry.orderId,
      clientOrderId: entry.clientOrderId,
      symbol: entry.symbol,
      side: entry.side,
      quantity: quantity.value ?? 0,
      quantityText: quantity.text,
      orderType: entry.orderType,
      orderPrice: orderPrice.value,
      orderPriceText: orderPrice.text,
      submittedAt: entry.submittedAt,
      status: entry.status,
      filledQuantity: filledQuantity.value,
      filledQuantityText: filledQuantity.text,
      filledPrice: filledPrice.value,
      filledPriceText: filledPrice.text,
      filledAt: entry.filledAt,
      brokerFee: entry.brokerFee,
      marketFee: entry.marketFee,
      totalFee: entry.totalFee,
      feeCurrency: entry.feeCurrency,
    };
  }

  function normalizeRunResult(result: BacktestRunResultTransport): BacktestRunResult {
    const trades = result.trades?.map(normalizeTrade);
    const orderBook = result.orderBook?.map(normalizeOrderBookEntry);
    const candles = result.candles?.map(normalizeCandle);
    return {
      ...result,
      trades: trades ? markRaw(trades) : undefined,
      orderBook: orderBook ? markRaw(orderBook) : undefined,
      candles: candles ? markRaw(candles) : undefined,
      pnlCurve: result.pnlCurve ? markRaw(result.pnlCurve) : undefined,
      drawdownCurve: result.drawdownCurve
        ? markRaw(result.drawdownCurve)
        : undefined,
      runtimeErrors: result.runtimeErrors
        ? markRaw(result.runtimeErrors)
        : undefined,
      logs: result.logs ? markRaw(result.logs) : undefined,
    };
  }

  function normalizeRun(run: BacktestRunTransport): BacktestRun {
    return {
      ...run,
      result: run.result ? normalizeRunResult(run.result) : undefined,
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
    const data = await apiGet<{ runs: BacktestRunTransport[] }, "/api/v1/backtests">(
      "/api/v1/backtests",
    );
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

  async function deleteRun(runId: string) {
    const normalizedRunID = runId.trim();
    if (normalizedRunID === "") {
      return;
    }

    const targetRun = runs.value.find((run) => run.id === normalizedRunID);
    const isTerminal = targetRun != null && isTerminalBacktestStatus(targetRun.status);
    if (isTerminal) {
      try {
        await fetchEnvelopeWithInit<{ deleted: boolean; id: string }>(
          `/api/v1/backtests/${encodeURIComponent(normalizedRunID)}`,
          {
            method: "DELETE",
          },
        );
      } catch {
        // Fallback: remove from local list if the server cannot delete this run.
      }
    }

    delete expandedRuns[normalizedRunID];
    queryClient.setQueryData<BacktestRun[]>(
      backtestRunsQueryKey,
      (current) => (current ?? []).filter((run) => run.id !== normalizedRunID),
    );
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
    let consecutiveFailures = 0;
    polling.value = setInterval(async () => {
      try {
        const data = await fetchEnvelope<{ id: string; status: string }>(
          `/api/v1/backtests/${runId}/status`,
        );
        consecutiveFailures = 0;
        patchBacktestRunStatus(runId, data.status);
        if (isTerminalBacktestStatus(data.status)) {
          stopPolling();
          await queryClient.invalidateQueries({ queryKey: backtestRunsQueryKey });
          await refreshRuns();
        }
      } catch (cause) {
        consecutiveFailures += 1;
        if (consecutiveFailures >= 3) {
          stopPolling();
          error.value = `回测状态轮询失败: ${cause instanceof Error ? cause.message : String(cause)}`;
        }
      }
    }, 2000);
  }

  function stopPolling() {
    if (polling.value) {
      clearInterval(polling.value);
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
