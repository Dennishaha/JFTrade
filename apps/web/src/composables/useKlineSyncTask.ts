import { onScopeDispose, ref } from "vue";

import type { BacktestSyncRequestPayload } from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";
import { createBacktestLiveReducer } from "./liveEventReducers";
import { getLiveEventBus } from "./liveEventBus";

const SYNC_POLL_INTERVAL_MS = 1500;
const MAX_CONSECUTIVE_SYNC_POLL_FAILURES = 3;

interface SyncWaitSession {
  taskId: string;
  timer: ReturnType<typeof setTimeout> | null;
  resolve: (progress: KlineSyncProgress | null) => void;
  settled: boolean;
}

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
  const syncPolling = ref<ReturnType<typeof setTimeout> | null>(null);
  let activeWait: SyncWaitSession | null = null;
  let refreshPollingGeneration = 0;
  let syncRequestGeneration = 0;
  let disposed = false;
  const liveReducer = createBacktestLiveReducer({
    activeTaskId: () => syncTaskId.value,
    applyProgress,
  });
  const stopLiveEvents = getLiveEventBus().subscribe(liveReducer.handle);

  onScopeDispose(() => {
    disposed = true;
    syncRequestGeneration += 1;
    syncing.value = false;
    stopLiveEvents();
    stopSyncPolling();
  }, true);

  async function startSync(payload: BacktestSyncRequestPayload): Promise<KlineSyncProgress | null> {
    if (disposed) {
      return null;
    }

    stopSyncPolling();
    const generation = ++syncRequestGeneration;
    syncing.value = true;
    syncTaskId.value = "";
    syncProgress.value = null;
    syncError.value = "";

    try {
      const started = await fetchEnvelopeWithInit<{ taskId: string; message: string }>(
        "/api/v1/backtests/sync",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );
      if (disposed || generation !== syncRequestGeneration) {
        return null;
      }
      syncTaskId.value = started.taskId;
      return await waitForSyncCompletion(started.taskId);
    } catch (cause) {
      if (disposed || generation !== syncRequestGeneration) {
        return null;
      }
      syncing.value = false;
      syncError.value = `同步启动失败: ${cause instanceof Error ? cause.message : String(cause)}`;
      return null;
    }
  }

  function startSyncPolling(taskId = syncTaskId.value): void {
    stopSyncPolling();
    if (disposed || taskId.trim() === "") {
      return;
    }
    const generation = refreshPollingGeneration;
    let consecutiveFailures = 0;
    const poll = async () => {
      if (disposed || generation !== refreshPollingGeneration) {
        return;
      }
      const progress = await refreshSyncProgress(taskId);
      if (disposed || generation !== refreshPollingGeneration) {
        return;
      }
      consecutiveFailures = progress == null ? consecutiveFailures + 1 : 0;
      if (consecutiveFailures >= MAX_CONSECUTIVE_SYNC_POLL_FAILURES) {
        syncing.value = false;
        syncError.value = `同步状态轮询失败: 连续 ${consecutiveFailures} 次无法读取同步进度`;
        stopRefreshInterval();
        return;
      }
      if (progress == null || !isTerminalSyncStatus(progress.status)) {
        syncPolling.value = setTimeout(() => void poll(), SYNC_POLL_INTERVAL_MS);
      }
    };
    syncPolling.value = setTimeout(() => void poll(), SYNC_POLL_INTERVAL_MS);
  }

  async function waitForSyncCompletion(taskId: string): Promise<KlineSyncProgress | null> {
    finishActiveWait(null);

    return new Promise((resolve) => {
      const session: SyncWaitSession = {
        taskId,
        timer: null,
        resolve,
        settled: false,
      };
      activeWait = session;
      let consecutiveFailures = 0;

      const poll = async () => {
        if (disposed || activeWait !== session) {
          finishWait(session, null);
          return;
        }

        const progress = await refreshSyncProgress(taskId);
        if (disposed || activeWait !== session) {
          finishWait(session, null);
          return;
        }
        if (progress === null) {
          consecutiveFailures += 1;
          if (consecutiveFailures >= MAX_CONSECUTIVE_SYNC_POLL_FAILURES) {
            syncing.value = false;
            syncError.value = `同步状态轮询失败: 连续 ${consecutiveFailures} 次无法读取同步进度`;
            finishWait(session, null);
            return;
          }
          session.timer = setTimeout(() => void poll(), SYNC_POLL_INTERVAL_MS);
          return;
        }
        consecutiveFailures = 0;
        if (isTerminalSyncStatus(progress.status)) {
          syncing.value = false;
          if (progress.status === "failed") {
            syncError.value = `同步失败: ${progress.error || "未知错误"} (重试 ${progress.retries} 次)`;
          }
          finishWait(session, progress);
          return;
        }
        session.timer = setTimeout(() => void poll(), SYNC_POLL_INTERVAL_MS);
      };
      void poll();
    });
  }

  async function refreshSyncProgress(taskId = syncTaskId.value): Promise<KlineSyncProgress | null> {
    if (taskId.trim() === "") {
      return null;
    }
    const requestGeneration = syncRequestGeneration;
    const pollingGeneration = refreshPollingGeneration;
    try {
      const progress = await fetchEnvelope<KlineSyncProgress>(
        `/api/v1/backtests/sync/${encodeURIComponent(taskId)}`,
      );
      if (
        disposed ||
        requestGeneration !== syncRequestGeneration ||
        pollingGeneration !== refreshPollingGeneration
      ) {
        return null;
      }
      applyProgress(progress);
      return progress;
    } catch {
      return null;
    }
  }

  async function cancelSync(): Promise<void> {
    syncRequestGeneration += 1;
    const taskId = syncTaskId.value.trim();
    syncing.value = false;
    stopSyncPolling();
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
  }

  function stopSyncPolling(): void {
    stopRefreshInterval();
    finishActiveWait(null);
  }

  function applyProgress(progress: KlineSyncProgress): void {
    syncProgress.value = progress;
    if (progress.status === "completed" || progress.status === "cancelled") {
      syncing.value = false;
      stopRefreshInterval();
      finishMatchingWait(progress);
    }
    if (progress.status === "failed") {
      syncing.value = false;
      stopRefreshInterval();
      syncError.value = `同步失败: ${progress.error || "未知错误"} (重试 ${progress.retries} 次)`;
      finishMatchingWait(progress);
    }
  }

  function stopRefreshInterval(): void {
    refreshPollingGeneration += 1;
    if (syncPolling.value !== null) {
      clearTimeout(syncPolling.value);
      syncPolling.value = null;
    }
  }

  function finishMatchingWait(progress: KlineSyncProgress): void {
    const session = activeWait;
    if (session != null && session.taskId === progress.taskId) {
      finishWait(session, progress);
    }
  }

  function finishActiveWait(progress: KlineSyncProgress | null): void {
    if (activeWait != null) {
      finishWait(activeWait, progress);
    }
  }

  function finishWait(
    session: SyncWaitSession,
    progress: KlineSyncProgress | null,
  ): void {
    if (session.settled) {
      return;
    }
    session.settled = true;
    if (session.timer !== null) {
      clearTimeout(session.timer);
      session.timer = null;
    }
    if (activeWait === session) {
      activeWait = null;
    }
    session.resolve(progress);
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
