import type { Ref } from "vue";

import type { MarketDataSnapshotQueryResult } from "./marketDataRealtime";

interface MarketSnapshotRefreshTarget {
  market: string;
  symbol: string;
  instrumentId: string;
}

interface MarketDataSnapshotRefresherOptions {
  marketDataSnapshot: Ref<MarketDataSnapshotQueryResult | null>;
  fetchEnvelope: <T>(path: string) => Promise<T>;
  mergeRealtimeBarStateIntoSnapshot: (
    current: MarketDataSnapshotQueryResult | null,
  ) => MarketDataSnapshotQueryResult | null;
}

const US_MARKET_SNAPSHOT_BACKGROUND_REFRESH_MS = 5_000;

export function createMarketDataSnapshotRefresher(
  options: MarketDataSnapshotRefresherOptions,
) {
  let marketSnapshotRefreshTimer: ReturnType<typeof setTimeout> | null = null;

  function clearMarketSnapshotRefreshTimer(): void {
    if (marketSnapshotRefreshTimer == null) {
      return;
    }

    window.clearTimeout(marketSnapshotRefreshTimer);
    marketSnapshotRefreshTimer = null;
  }

  function resolveMarketSnapshotRefreshTarget(): MarketSnapshotRefreshTarget | null {
    const request = options.marketDataSnapshot.value?.request;
    if (request == null) {
      return null;
    }

    const market = request.market.trim().toUpperCase();
    const symbol = request.symbol.trim().toUpperCase();
    if (market !== "US" || symbol === "") {
      return null;
    }

    return {
      market,
      symbol,
      instrumentId: request.instrumentId,
    };
  }

  async function refreshMarketSnapshotInBackground(
    target: MarketSnapshotRefreshTarget,
  ): Promise<void> {
    try {
      const activeTarget = resolveMarketSnapshotRefreshTarget();
      if (activeTarget == null || activeTarget.instrumentId !== target.instrumentId) {
        return;
      }

      const snapshot = await options.fetchEnvelope<MarketDataSnapshotQueryResult>(
        `/api/v1/market-data/snapshots/${encodeURIComponent(target.market)}/${encodeURIComponent(target.symbol)}?refresh=true`,
      );

      const latestTarget = resolveMarketSnapshotRefreshTarget();
      if (latestTarget == null || latestTarget.instrumentId !== target.instrumentId) {
        return;
      }

      options.marketDataSnapshot.value =
        options.mergeRealtimeBarStateIntoSnapshot(snapshot);
    } catch {
      // Keep the current snapshot and retry on the next background interval.
    } finally {
      const latestTarget = resolveMarketSnapshotRefreshTarget();
      if (latestTarget != null && latestTarget.instrumentId === target.instrumentId) {
        scheduleMarketSnapshotBackgroundRefresh();
      }
    }
  }

  function scheduleMarketSnapshotBackgroundRefresh(): void {
    clearMarketSnapshotRefreshTimer();

    const target = resolveMarketSnapshotRefreshTarget();
    if (target == null) {
      return;
    }

    marketSnapshotRefreshTimer = window.setTimeout(() => {
      marketSnapshotRefreshTimer = null;
      void refreshMarketSnapshotInBackground(target);
    }, US_MARKET_SNAPSHOT_BACKGROUND_REFRESH_MS);
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
  };
}