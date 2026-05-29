import { computed, reactive, ref, type ComputedRef } from "vue";

import type { BacktestStartRequestPayload, BacktestSyncRequestPayload } from "@jftrade/ui-contracts";

import type { BacktestTrade, BacktestPnlPoint, BacktestDrawdownPoint, BacktestCandle } from "../components/BacktestChart.vue";
import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { resolveInstrumentRef } from "./instrumentRef";

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
    market?: string;
    code?: string;
    symbol: string;
    interval: string;
    startTime: string;
    endTime: string;
    initialBalance: number;
    rehabType?: string;
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
    market?: string;
    code?: string;
    symbol: string;
    interval: string;
    startTime: string;
    endTime: string;
    initialBalance: number;
    rehabType?: string;
  };
  result?: BacktestRunResultTransport | undefined;
  createdAt: string;
  updatedAt: string;
}

export interface BacktestFormState {
  definitionId: string;
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
}

interface UseBacktestRunsOptions {
  formState: ComputedRef<BacktestFormState>;
}

export function buildBacktestInstrumentPayload(
  formState: Pick<BacktestFormState, "market" | "code" | "instrumentId">,
): { market: string; code: string; symbol: string } | null {
  const instrument = resolveInstrumentRef(
    {
      market: formState.market,
      code: formState.code,
      instrumentId: formState.instrumentId,
    },
    formState.market,
  );
  if (instrument == null) {
    return null;
  }
  return {
    market: instrument.market,
    code: instrument.code,
    symbol: instrument.instrumentId,
  };
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

  const filteredRuns = computed(() =>
    [...runs.value].sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    ),
  );

  function toggleRun(runId: string) {
    expandedRuns[runId] = !expandedRuns[runId];
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

  async function loadRuns() {
    try {
      const data = await fetchEnvelope<{ runs: BacktestRunTransport[] }>(
        "/api/v1/backtests",
      );
      runs.value = (data.runs ?? []).map(normalizeRun);
    } catch {
      // backtests may not be available yet
    }
  }

  async function syncKlines() {
    const formState = options.formState.value;
    const instrument = buildBacktestInstrumentPayload(formState);
    if (instrument == null) {
      error.value = "同步启动失败: 请先输入有效的市场与代码";
      return;
    }
    syncing.value = true;
    error.value = "";
    syncTaskId.value = "";
    syncProgress.value = null;
    stopSyncPolling();

    try {
      const payload: BacktestSyncRequestPayload = {
        market: instrument.market,
        code: instrument.code,
        symbol: instrument.symbol,
        intervals: [formState.interval],
        since: formState.syncStartTime,
        until: formState.syncEndTime,
        rehabType: formState.rehabType,
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
    const instrument = buildBacktestInstrumentPayload(formState);
    if (instrument == null) {
      error.value = "启动回测失败: 请先输入有效的市场与代码";
      return;
    }

    running.value = true;
    error.value = "";
    try {
      const payload: BacktestStartRequestPayload = {
        definitionId: formState.definitionId,
        market: instrument.market,
        code: instrument.code,
        symbol: instrument.symbol,
        interval: formState.interval,
        startTime: formState.backtestStartTime,
        endTime: formState.backtestEndTime,
        initialBalance: formState.initialBalance,
        rehabType: formState.rehabType,
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
    polling.value = setInterval(async () => {
      try {
        const data = await fetchEnvelope<{ id: string; status: string }>(
          `/api/v1/backtests/${runId}/status`,
        );
        if (data.status === "completed" || data.status === "failed") {
          stopPolling();
          await loadRuns();
        }
      } catch {
        // ignore poll errors
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
    filteredRuns,
    toggleRun,
    loadRuns,
    syncKlines,
    cancelSync,
    startBacktest,
  };
}