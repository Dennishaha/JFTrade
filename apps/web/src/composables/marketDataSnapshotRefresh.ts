import type { Ref } from "vue";

import { buildApiUrl } from "./apiClient";
import type { MarketSecurityDetailsQueryResult } from "./marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "./marketSecurityNormalization";

interface MarketSnapshotRefreshTarget {
  market: string;
  symbol: string;
  instrumentId: string;
}

interface MarketDataSnapshotRefresherOptions {
  marketSecurityDetails: Ref<MarketSecurityDetailsQueryResult | null>;
}

export function createMarketDataSnapshotRefresher(
  options: MarketDataSnapshotRefresherOptions,
) {
  let marketSecurityDetailsStream: EventSource | null = null;
  let marketSecurityDetailsStreamInstrumentId = "";

  function closeMarketSecurityDetailsStream(): void {
    marketSecurityDetailsStream?.close();
    marketSecurityDetailsStream = null;
    marketSecurityDetailsStreamInstrumentId = "";
  }

  function resolveMarketSnapshotRefreshTarget(): MarketSnapshotRefreshTarget | null {
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

  function scheduleMarketSnapshotBackgroundRefresh(): void {
    const target = resolveMarketSnapshotRefreshTarget();
    if (target == null) {
      closeMarketSecurityDetailsStream();
      return;
    }

    if (
      marketSecurityDetailsStream != null &&
      marketSecurityDetailsStreamInstrumentId === target.instrumentId
    ) {
      return;
    }

    closeMarketSecurityDetailsStream();

    if (typeof EventSource === "undefined") {
      return;
    }

    const url = buildApiUrl(
      `/api/v1/market-data/securities/${encodeURIComponent(target.market)}/${encodeURIComponent(target.symbol)}`,
    );
    const stream = new EventSource(url);
    marketSecurityDetailsStream = stream;
    marketSecurityDetailsStreamInstrumentId = target.instrumentId;

    stream.onmessage = (event) => {
      if (
        marketSecurityDetailsStream !== stream ||
        marketSecurityDetailsStreamInstrumentId !== target.instrumentId
      ) {
        return;
      }

      try {
        const payload = JSON.parse(event.data as string) as MarketSecurityDetailsQueryResult;
        const activeTarget = resolveMarketSnapshotRefreshTarget();
        if (
          activeTarget == null ||
          activeTarget.instrumentId !== target.instrumentId
        ) {
          return;
        }
        options.marketSecurityDetails.value =
          normalizeMarketSecurityDetailsQueryResult(payload);
      } catch {
        // Ignore malformed SSE payloads and keep the last known details.
      }
    };

    stream.onerror = () => {
      if (marketSecurityDetailsStream !== stream) {
        return;
      }
      // Let the browser handle EventSource retries. We only reset local state
      // when the active target changes or the stream is explicitly closed.
    };
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
    stopMarketSnapshotBackgroundRefresh: closeMarketSecurityDetailsStream,
  };
}
