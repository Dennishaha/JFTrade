import type { Ref } from "vue";

import { buildApiUrl } from "./apiClient";
import { createEventSourceStream } from "./eventSourceStream";
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
  let marketSecurityDetailsStreamInstrumentId = "";
  const marketSecurityDetailsStream =
    createEventSourceStream<MarketSecurityDetailsQueryResult>({
      onMessage: (payload) => {
        const activeTarget = resolveMarketSnapshotRefreshTarget();
        if (
          activeTarget == null ||
          activeTarget.instrumentId !== marketSecurityDetailsStreamInstrumentId
        ) {
          return;
        }
        options.marketSecurityDetails.value =
          normalizeMarketSecurityDetailsQueryResult(payload);
      },
    });

  function closeMarketSecurityDetailsStream(): void {
    marketSecurityDetailsStream.disconnect(false);
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
      marketSecurityDetailsStream.activeUrl.value != null &&
      marketSecurityDetailsStreamInstrumentId === target.instrumentId
    ) {
      return;
    }

    closeMarketSecurityDetailsStream();

    const url = buildApiUrl(
      `/api/sse/market/securities/${encodeURIComponent(target.market)}/${encodeURIComponent(target.symbol)}`,
    );
    marketSecurityDetailsStreamInstrumentId = target.instrumentId;
    marketSecurityDetailsStream.connect(url);
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
    stopMarketSnapshotBackgroundRefresh: closeMarketSecurityDetailsStream,
  };
}
