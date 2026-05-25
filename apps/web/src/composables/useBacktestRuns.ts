import { computed, reactive, ref, type ComputedRef } from "vue";

import type { BacktestTrade, BacktestPnlPoint, BacktestCandle } from "../components/BacktestChart.vue";
import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

interface BacktestOrderBookEntry {
  orderId: string;
  clientOrderId?: string;
  symbol: string;
  side: string;
  quantity: number;
  orderType?: string;
  orderPrice?: number;
  submittedAt?: string;
  status: string;
  filledQuantity?: number;
  filledPrice?: number;
  filledAt?: string;
}

interface BacktestRunResult {
  symbol: string;
  interval: string;
  startTime: string;
  endTime: string;
  quoteCurrency?: string;
  finalBalance: number;
  pnl: number;
  totalTrades: number;
  winRate: number;
  trades?: BacktestTrade[];
  orderBook?: BacktestOrderBookEntry[];
  pnlCurve?: BacktestPnlPoint[];
  candles?: BacktestCandle[];
  logs?: string[];
  runtimeErrors?: string[];
  error?: string;
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
    symbol: string;
    interval: string;
    startTime: string;
    endTime: string;
    initialBalance: number;
  };
  result?: BacktestRunResult;
  createdAt: string;
  updatedAt: string;
}

export interface BacktestFormState {
  definitionId: string;
  instrumentId: string;
  interval: string;
  syncStartTime: string;
  syncEndTime: string;
  backtestStartTime: string;
  backtestEndTime: string;
  initialBalance: number;
  warmupCandles: number;
  rehabType: string;
}

interface UseBacktestRunsOptions {
  formState: ComputedRef<BacktestFormState>;
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

  async function loadRuns() {
    try {
      const data = await fetchEnvelope<{ runs: BacktestRun[] }>(
        "/api/v1/backtests",
      );
      runs.value = data.runs ?? [];
    } catch {
      // backtests may not be available yet
    }
  }

  async function syncKlines() {
    const formState = options.formState.value;
    syncing.value = true;
    error.value = "";
    syncTaskId.value = "";
    syncProgress.value = null;
    stopSyncPolling();

    try {
      const data = await fetchEnvelopeWithInit<{ taskId: string; message: string }>(
        "/api/v1/backtests/sync",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            symbol: formState.instrumentId,
            intervals: [formState.interval],
            since: formState.syncStartTime,
            until: formState.syncEndTime,
            rehabType: formState.rehabType,
          }),
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
      const data = await fetchEnvelopeWithInit<{ id: string; status: string }>(
        "/api/v1/backtests",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            definitionId: formState.definitionId,
            symbol: formState.instrumentId,
            interval: formState.interval,
            startTime: formState.backtestStartTime,
            endTime: formState.backtestEndTime,
            initialBalance: formState.initialBalance,
            warmupCandles: formState.warmupCandles,
            rehabType: formState.rehabType,
          }),
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