import type { Ref } from "vue";

import type { StrategyInstanceItem } from "@/types";
import { displayStrategyStatus } from "./strategyRuntimePresentation";

const ACTIVE_REFRESH_MS = 1_000;
const IDLE_REFRESH_MS = 3_000;

interface RefreshOptions {
  selectedStrategy: Readonly<Ref<StrategyInstanceItem | null>>;
  activeStrategyCount: Readonly<Ref<number>>;
  busyStates: ReadonlyArray<Ref<boolean>>;
  refresh: () => Promise<void>;
}

export function useStrategyRuntimeRefresh(options: RefreshOptions) {
  let refreshTimer: number | null = null;

  function clearStrategyRuntimeRefreshTimer(): void {
    if (refreshTimer === null) return;
    window.clearTimeout(refreshTimer);
    refreshTimer = null;
  }

  function resolveStrategyRuntimeRefreshMs(): number {
    const strategy = options.selectedStrategy.value;
    const selectedStatus = strategy === null ? "" : displayStrategyStatus(strategy);
    return options.activeStrategyCount.value > 0
      || selectedStatus === "RUNNING"
      || selectedStatus === "PAUSED"
      ? ACTIVE_REFRESH_MS
      : IDLE_REFRESH_MS;
  }

  function shouldDeferStrategyRuntimeRefresh(): boolean {
    return options.busyStates.some((state) => state.value);
  }

  function scheduleStrategyRuntimeRefresh(): void {
    if (typeof window === "undefined") return;
    clearStrategyRuntimeRefreshTimer();
    if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
    refreshTimer = window.setTimeout(() => {
      void options.refresh();
    }, resolveStrategyRuntimeRefreshMs());
  }

  function handleStrategyRuntimeVisibilityChange(): void {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
      clearStrategyRuntimeRefreshTimer();
      return;
    }
    void options.refresh();
  }

  return {
    clearStrategyRuntimeRefreshTimer,
    resolveStrategyRuntimeRefreshMs,
    shouldDeferStrategyRuntimeRefresh,
    scheduleStrategyRuntimeRefresh,
    handleStrategyRuntimeVisibilityChange,
  };
}
