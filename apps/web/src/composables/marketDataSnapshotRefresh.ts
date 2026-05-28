import type { Ref } from "vue";

import type {
  MarketSecurityDetailsQueryResult,
  MarketDataSnapshotQueryResult,
} from "./marketDataRealtime";
import { normalizeMarketDataSnapshotQueryResult } from "./marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "./marketSecurityNormalization";

interface MarketSnapshotRefreshTarget {
  market: string;
  symbol: string;
  instrumentId: string;
}

interface MarketDataSnapshotRefresherOptions {
  marketDataSnapshot: Ref<MarketDataSnapshotQueryResult | null>;
  marketSecurityDetails: Ref<MarketSecurityDetailsQueryResult | null>;
  fetchEnvelope: <T>(path: string) => Promise<T>;
  mergeRealtimeBarStateIntoSnapshot: (
    current: MarketDataSnapshotQueryResult | null,
  ) => MarketDataSnapshotQueryResult | null;
}

const MARKET_PANEL_BACKGROUND_REFRESH_MS = 1_000;

export function createMarketDataSnapshotRefresher(
  options: MarketDataSnapshotRefresherOptions,
) {
  let marketSnapshotRefreshTimer: ReturnType<typeof setTimeout> | null = null;
  let marketSnapshotRefreshTimerInstrumentId = "";

  function clearMarketSnapshotRefreshTimer(): void {
    if (marketSnapshotRefreshTimer == null) {
      return;
    }

    window.clearTimeout(marketSnapshotRefreshTimer);
    marketSnapshotRefreshTimer = null;
    marketSnapshotRefreshTimerInstrumentId = "";
  }

  function resolveMarketSnapshotRefreshTarget(): MarketSnapshotRefreshTarget | null {
    const request = options.marketDataSnapshot.value?.request;
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
      const securityDetails = await options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
        `/api/v1/market-data/securities/${encodeURIComponent(target.market)}/${encodeURIComponent(target.symbol)}`,
      );

      const latestTarget = resolveMarketSnapshotRefreshTarget();
      if (latestTarget == null || latestTarget.instrumentId !== target.instrumentId) {
        return;
      }

      options.marketDataSnapshot.value =
        options.mergeRealtimeBarStateIntoSnapshot(
          normalizeMarketDataSnapshotQueryResult(snapshot),
        );
      options.marketSecurityDetails.value =
        normalizeMarketSecurityDetailsQueryResult(securityDetails);
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
    const target = resolveMarketSnapshotRefreshTarget();
    if (target == null) {
      clearMarketSnapshotRefreshTimer();
      return;
    }

    if (
      marketSnapshotRefreshTimer != null &&
      marketSnapshotRefreshTimerInstrumentId === target.instrumentId
    ) {
      return;
    }

    clearMarketSnapshotRefreshTimer();

    marketSnapshotRefreshTimer = window.setTimeout(() => {
      marketSnapshotRefreshTimer = null;
      marketSnapshotRefreshTimerInstrumentId = "";
      void refreshMarketSnapshotInBackground(target);
    }, MARKET_PANEL_BACKGROUND_REFRESH_MS);
    marketSnapshotRefreshTimerInstrumentId = target.instrumentId;
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
  };
}