import { watch, type Ref } from "vue";

import type { MarketSecurityDetailsQueryResult } from "./marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "./marketSecurityNormalization";
import {
  getSharedLiveSocketHub,
  type MarketSecurityDetailsLiveStreamEvent,
} from "./sharedLiveSocket";

export interface MarketSnapshotRefreshTarget {
  market: string;
  symbol: string;
  instrumentId: string;
}

export interface MarketSnapshotFallbackResult {
  retryAfterMs?: number;
}

interface MarketDataSnapshotRefresherOptions {
  marketSecurityDetails: Ref<MarketSecurityDetailsQueryResult | null>;
  fallbackIntervalMs?: number;
  fallbackRefresh?: (
    target: MarketSnapshotRefreshTarget,
  ) => Promise<MarketSnapshotFallbackResult | void>;
}

export function createMarketDataSnapshotRefresher(
  options: MarketDataSnapshotRefresherOptions,
) {
  const hub = getSharedLiveSocketHub();
  const ownerId = hub.createOwnerId("security-details");
  const fallbackIntervalMs = Math.max(1, options.fallbackIntervalMs ?? 3_000);
  let explicitTarget: MarketSnapshotRefreshTarget | null | undefined;
  let fallbackTimer: ReturnType<typeof setTimeout> | null = null;
  let fallbackInFlight = false;
  let stopped = false;
  const removeListener = hub.addEventListener((event) => {
    if (!isMarketSecurityDetailsEvent(event)) {
      return;
    }
    const activeTarget = resolveMarketSnapshotRefreshTarget();
    if (activeTarget == null || activeTarget.instrumentId !== event.request.instrumentId) {
      return;
    }
    options.marketSecurityDetails.value =
      normalizeMarketSecurityDetailsQueryResult(event);
  });

  function closeMarketSecurityDetailsStream(): void {
    hub.setSecurityDetailsTarget(ownerId, null);
    clearFallbackTimer();
  }

  function resolveMarketSnapshotRefreshTarget(): MarketSnapshotRefreshTarget | null {
    if (explicitTarget !== undefined) {
      return explicitTarget;
    }
    const request = options.marketSecurityDetails.value?.request;
    if (request == null) {
      return null;
    }

    const market = request.market.trim().toUpperCase();
    const symbol = request.symbol.trim().toUpperCase();
    if (market === "" || symbol === "") {
      return null;
    }

    return {
      market,
      symbol,
      instrumentId: request.instrumentId,
    };
  }

  function scheduleMarketSnapshotBackgroundRefresh(
    target?: MarketSnapshotRefreshTarget | null,
  ): void {
    if (arguments.length > 0) {
      explicitTarget = target == null ? null : normalizeRefreshTarget(target);
    }
    const resolvedTarget = resolveMarketSnapshotRefreshTarget();
    if (resolvedTarget == null) {
      closeMarketSecurityDetailsStream();
      return;
    }

    hub.setSecurityDetailsTarget(ownerId, resolvedTarget);
    syncFallbackTimer();
  }

  function shouldFallback(): boolean {
    const connectionState = hub.connectionState.value;
    return (
      !stopped &&
      options.fallbackRefresh != null &&
      resolveMarketSnapshotRefreshTarget() != null &&
      (connectionState === "connecting" ||
        connectionState === "disconnected" ||
        connectionState === "error" ||
        connectionState === "unsupported") &&
      (typeof document === "undefined" || document.visibilityState !== "hidden")
    );
  }

  function syncFallbackTimer(delayMs = fallbackIntervalMs): void {
    if (!shouldFallback()) {
      clearFallbackTimer();
      return;
    }
    if (fallbackTimer != null || fallbackInFlight) {
      return;
    }
    fallbackTimer = setTimeout(() => {
      fallbackTimer = null;
      void runFallbackRefresh();
    }, Math.max(fallbackIntervalMs, delayMs));
  }

  async function runFallbackRefresh(): Promise<void> {
    const target = resolveMarketSnapshotRefreshTarget();
    if (!shouldFallback() || target == null || options.fallbackRefresh == null) {
      return;
    }
    fallbackInFlight = true;
    let retryAfterMs = fallbackIntervalMs;
    try {
      const result = await options.fallbackRefresh(target);
      if (result?.retryAfterMs != null && Number.isFinite(result.retryAfterMs)) {
        retryAfterMs = Math.max(retryAfterMs, result.retryAfterMs);
      }
    } catch (error) {
      const candidate = (error as { retryAfterMs?: unknown } | null)?.retryAfterMs;
      if (typeof candidate === "number" && Number.isFinite(candidate)) {
        retryAfterMs = Math.max(retryAfterMs, candidate);
      }
    } finally {
      fallbackInFlight = false;
      syncFallbackTimer(retryAfterMs);
    }
  }

  function clearFallbackTimer(): void {
    if (fallbackTimer != null) {
      clearTimeout(fallbackTimer);
      fallbackTimer = null;
    }
  }

  function handleVisibilityChange(): void {
    syncFallbackTimer();
  }

  const stopConnectionWatch = watch(hub.connectionState, () => {
    syncFallbackTimer();
  });
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", handleVisibilityChange);
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
    stopMarketSnapshotBackgroundRefresh: () => {
      stopped = true;
      closeMarketSecurityDetailsStream();
      stopConnectionWatch();
      if (typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", handleVisibilityChange);
      }
      removeListener();
    },
  };
}

function normalizeRefreshTarget(
  target: MarketSnapshotRefreshTarget,
): MarketSnapshotRefreshTarget {
  return {
    market: target.market.trim().toUpperCase(),
    symbol: target.symbol.trim().toUpperCase(),
    instrumentId: target.instrumentId.trim().toUpperCase(),
  };
}

function isMarketSecurityDetailsEvent(
  event: { type: string },
): event is MarketSecurityDetailsLiveStreamEvent {
  return event.type === "market.security-details";
}
