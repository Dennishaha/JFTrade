import { computed, reactive, ref, type ComputedRef } from "vue";

import type { BacktestStartRequestPayload, BacktestSyncRequestPayload } from "@/contracts";

import type { BacktestTrade, BacktestPnlPoint, BacktestDrawdownPoint, BacktestCandle } from "../components/BacktestChart.vue";
import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

type BacktestDecimalTransport = string | number;

interface BacktestTradeView extends BacktestTrade {
  priceText?: string | undefined;
  qtyText?: string | undefined;
}

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
}

interface BacktestTradeTransport {
  time: string;
  side: string;
  price: BacktestDecimalTransport;
  qty: BacktestDecimalTransport;
  pnl?: number;
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
}

interface BacktestRunResult {
  symbol: string;
  interval: string;
  startTime: string;
  endTime: string;
  quoteCurrency?: string | undefined;
  finalBalance: number;
  pnl: number;
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

interface SyncProgress {
  taskId: string;
  status: string;
  symbol: string;
  currentInterval: string;
  totalIntervals: number;
  completedIntervals: number;
  totalBatches: number;
  completedBatches: number;
  retries: number;
  error?: string;
  startedAt: string;
  updatedAt: string;
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
    interval: string;
    startTime: string;
    endTime: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
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
    interval: string;
    startTime: string;
    endTime: string;
    initialBalance: number;
    rehabType?: string;
    useExtendedHours?: boolean;
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
  interval: string;
  syncStartTime: string;
  syncEndTime: string;
  backtestStartTime: string;
  backtestEndTime: string;
  initialBalance: number;
  rehabType: string;
  useExtendedHours: boolean;
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

export function useBacktestRuns(options: UseBacktestRunsOptions) {
  const runs = ref<BacktestRun[]>([]);
  const running = ref(false);
  const syncing = ref(false);
  const syncTaskId = ref("");
  const syncProgress = ref<SyncProgress | null>(null);
  const syncPolling = ref<ReturnType<typeof setInterval> | null>(null);
  const polling = ref<ReturnType<typeof setInterval> | null>(null);
  const error = ref("");

  const expandedRuns = reactive<Record<string, boolean>>({});
  const detailLoading = reactive<Record<string, boolean>>({});
  const detailErrors = reactive<Record<string, string>>({});

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
      runs.value = mergeRunsById([
        ...runs.value.filter((run) => run.id !== runId),
        normalizeRun(detail),
      ]);
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
    return {
      time: trade.time,
      side: trade.side,
      price: price.value ?? 0,
      qty: qty.value ?? 0,
      ...(trade.pnl !== undefined ? { pnl: trade.pnl } : {}),
      priceText: price.text,
      qtyText: qty.text,
    };
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
    };
  }

  function normalizeRunResult(result: BacktestRunResultTransport): BacktestRunResult {
    return {
      ...result,
      trades: result.trades?.map(normalizeTrade),
      orderBook: result.orderBook?.map(normalizeOrderBookEntry),
      candles: result.candles?.map(normalizeCandle),
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

  async function loadRuns() {
    try {
      const data = await fetchEnvelope<{ runs: BacktestRunTransport[] }>(
        "/api/v1/backtests",
      );
      runs.value = mergeRunsById([
        ...runs.value,
        ...((data.runs ?? []).map(normalizeRun)),
      ]);
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
    runs.value = runs.value.filter((run) => run.id !== normalizedRunID);
  }

  async function syncKlines() {
    const formState = options.formState.value;
    syncing.value = true;
    error.value = "";
    syncTaskId.value = "";
    syncProgress.value = null;
    stopSyncPolling();

    try {
      const instrument = await resolveBacktestInstrumentPayload(
        formState,
        options.normalizeInstrument,
      );
      if (instrument == null) {
        error.value = "同步启动失败: 请先输入有效的市场与代码";
        syncing.value = false;
        return;
      }
      const payload: BacktestSyncRequestPayload = {
        market: instrument.market,
        code: instrument.code,
        symbol: instrument.symbol,
        intervals: [formState.interval],
        since: formState.syncStartTime,
        until: formState.syncEndTime,
        rehabType: formState.rehabType,
        sessionScope: resolveSyncSessionScope(formState),
      };
      const data = await fetchEnvelopeWithInit<{ taskId: string; message: string }>(
        "/api/v1/backtests/sync",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );

      syncTaskId.value = data.taskId;
      startSyncPolling();
    } catch (cause) {
      syncing.value = false;
      error.value = `同步启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
    }
  }

  function startSyncPolling() {
    stopSyncPolling();
    syncPolling.value = setInterval(async () => {
      try {
        const progress = await fetchEnvelope<SyncProgress>(
          `/api/v1/backtests/sync/${syncTaskId.value}`,
        );
        syncProgress.value = progress;
        if (progress.status === "completed" || progress.status === "cancelled") {
          syncing.value = false;
          stopSyncPolling();
          return;
        }
        if (progress.status === "failed") {
          syncing.value = false;
          stopSyncPolling();
          error.value = `同步失败: ${progress.error || "未知错误"} (重试 ${progress.retries} 次)`;
        }
      } catch {
        // ignore poll errors
      }
    }, 1500);
  }

  function stopSyncPolling() {
    if (syncPolling.value) {
      clearInterval(syncPolling.value);
      syncPolling.value = null;
    }
  }

  async function cancelSync() {
    if (!syncTaskId.value) return;

    try {
      await fetchEnvelopeWithInit(`/api/v1/backtests/sync/${syncTaskId.value}`, {
        method: "DELETE",
      });
    } catch {
      // best effort
    }

    syncing.value = false;
    stopSyncPolling();
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
      const payload: BacktestStartRequestPayload = {
        definitionId: formState.definitionId,
        definitionVersion: formState.definitionVersion,
        market: instrument.market,
        code: instrument.code,
        symbol: instrument.symbol,
        interval: formState.interval,
        startTime: formState.backtestStartTime,
        endTime: formState.backtestEndTime,
        initialBalance: formState.initialBalance,
        rehabType: formState.rehabType,
        useExtendedHours: formState.useExtendedHours,
      };
      const data = await fetchEnvelopeWithInit<{ id: string; status: string }>(
        "/api/v1/backtests",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );
      startPolling(data.id);
      await loadRuns();
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
        if (isTerminalBacktestStatus(data.status)) {
          stopPolling();
          await loadRuns();
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
