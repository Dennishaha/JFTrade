import { onScopeDispose, ref } from "vue";

import type { BacktestSyncRequestPayload } from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { createBacktestLiveReducer } from "./liveEventReducers";
import { getLiveEventBus } from "./liveEventBus";

export interface KlineSyncProgress {
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

export function useKlineSyncTask() {
  const syncing = ref(false);
  const syncTaskId = ref("");
  const syncProgress = ref<KlineSyncProgress | null>(null);
  const syncError = ref("");
  const syncPolling = ref<ReturnType<typeof setInterval> | null>(null);
  const liveReducer = createBacktestLiveReducer({
    activeTaskId: () => syncTaskId.value,
    applyProgress,
  });
  const stopLiveEvents = getLiveEventBus().subscribe(liveReducer.handle);

  onScopeDispose(() => {
    stopLiveEvents();
    stopSyncPolling();
  }, true);

  async function startSync(payload: BacktestSyncRequestPayload): Promise<KlineSyncProgress | null> {
    syncing.value = true;
    syncTaskId.value = "";
    syncProgress.value = null;
    syncError.value = "";
    stopSyncPolling();

    try {
      const started = await fetchEnvelopeWithInit<{ taskId: string; message: string }>(
        "/api/v1/backtests/sync",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );
      syncTaskId.value = started.taskId;
      return await waitForSyncCompletion(started.taskId);
    } catch (cause) {
      syncing.value = false;
      syncError.value = `同步启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
      return null;
    }
  }

  function startSyncPolling(taskId = syncTaskId.value): void {
    stopSyncPolling();
    if (taskId.trim() === "") {
      return;
    }
    syncPolling.value = setInterval(() => {
      void refreshSyncProgress(taskId);
    }, 1500);
  }

  async function waitForSyncCompletion(taskId: string): Promise<KlineSyncProgress | null> {
    return new Promise((resolve) => {
      const poll = async () => {
        const progress = await refreshSyncProgress(taskId);
        if (progress === null) {
          setTimeout(poll, 1500);
          return;
        }
        if (isTerminalSyncStatus(progress.status)) {
          stopSyncPolling();
          syncing.value = false;
          if (progress.status === "failed") {
            syncError.value = `同步失败: ${progress.error || "未知错误"} (重试 ${progress.retries} 次)`;
          }
          resolve(progress);
          return;
        }
        setTimeout(poll, 1500);
      };
      void poll();
    });
  }

  async function refreshSyncProgress(taskId = syncTaskId.value): Promise<KlineSyncProgress | null> {
    if (taskId.trim() === "") {
      return null;
    }
    try {
      const progress = await fetchEnvelope<KlineSyncProgress>(
        `/api/v1/backtests/sync/${encodeURIComponent(taskId)}`,
      );
      applyProgress(progress);
      return progress;
    } catch {
      return null;
    }
  }

  async function cancelSync(): Promise<void> {
    const taskId = syncTaskId.value.trim();
    if (taskId === "") {
      return;
    }
    try {
      await fetchEnvelopeWithInit(`/api/v1/backtests/sync/${encodeURIComponent(taskId)}`, {
        method: "DELETE",
      });
    } catch {
      // best effort
    }
    syncing.value = false;
    stopSyncPolling();
  }

  function stopSyncPolling(): void {
    if (syncPolling.value !== null) {
      clearInterval(syncPolling.value);
      syncPolling.value = null;
    }
  }

  function applyProgress(progress: KlineSyncProgress): void {
    syncProgress.value = progress;
    if (progress.status === "completed" || progress.status === "cancelled") {
      syncing.value = false;
      stopSyncPolling();
    }
    if (progress.status === "failed") {
      syncing.value = false;
      stopSyncPolling();
      syncError.value = `同步失败: ${progress.error || "未知错误"} (重试 ${progress.retries} 次)`;
    }
  }

  return {
    syncing,
    syncTaskId,
    syncProgress,
    syncError,
    startSync,
    startSyncPolling,
    refreshSyncProgress,
    cancelSync,
    stopSyncPolling,
  };
}

export function isTerminalSyncStatus(status: string): boolean {
  return status === "completed" || status === "cancelled" || status === "failed";
}
