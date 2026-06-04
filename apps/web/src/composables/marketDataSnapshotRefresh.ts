import type { Ref } from "vue";

import type { MarketSecurityDetailsQueryResult } from "./marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "./marketSecurityNormalization";
import {
  getSharedLiveSocketHub,
  type MarketSecurityDetailsLiveStreamEvent,
} from "./sharedLiveSocket";

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
  const hub = getSharedLiveSocketHub();
  const ownerId = hub.createOwnerId("security-details");
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

    hub.setSecurityDetailsTarget(ownerId, target);
  }

  return {
    scheduleMarketSnapshotBackgroundRefresh,
    stopMarketSnapshotBackgroundRefresh: () => {
      closeMarketSecurityDetailsStream();
      removeListener();
    },
  };
}

function isMarketSecurityDetailsEvent(
  event: { type: string },
): event is MarketSecurityDetailsLiveStreamEvent {
  return event.type === "market.security-details";
}
