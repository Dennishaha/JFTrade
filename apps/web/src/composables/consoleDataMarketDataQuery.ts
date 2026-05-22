import { ref } from "vue";

import {
  fetchEnvelope,
} from "./apiClient";
import {
  normalizeInstrumentParts,
} from "./consoleDataMarketInstruments";
import {
  createMarketDataQueryController,
  type LoadMarketDataQueryOptions,
} from "./marketDataQuery";
import {
  type MarketDataCandlesQueryResult,
  type MarketSecurityDetailsQueryResult,
  type MarketDataSnapshotQueryResult,
} from "./marketDataRealtime";

export function createConsoleDataMarketDataQuerySlice() {
  const marketDataQueryMarket = ref("HK");
  const marketDataQuerySymbol = ref("00700");
  const marketDataQueryPeriod = ref("1m");
  const marketDataQueryLimit = ref(500);
  const marketDataSnapshot = ref<MarketDataSnapshotQueryResult | null>(null);
  const marketSecurityDetails = ref<MarketSecurityDetailsQueryResult | null>(null);
  const marketDataCandles = ref<MarketDataCandlesQueryResult | null>(null);
  const isLoadingMarketDataQuery = ref(false);
  const marketDataQueryError = ref("");

  const marketDataQueryController = createMarketDataQueryController({
    state: {
      marketDataQueryMarket,
      marketDataQuerySymbol,
      marketDataQueryPeriod,
      marketDataQueryLimit,
      marketDataSnapshot,
      marketSecurityDetails,
      marketDataCandles,
      isLoadingMarketDataQuery,
      marketDataQueryError,
    },
    fetchEnvelope,
    normalizeInstrumentParts,
  });

  function applyMarketDataTickEvent(event: unknown): void {
    marketDataQueryController.applyTickEvent(event);
  }

  async function loadMarketDataQuery(
    options: LoadMarketDataQueryOptions = {},
  ): Promise<void> {
    return marketDataQueryController.loadQuery(options);
  }

  return {
    applyMarketDataTickEvent,
    isLoadingMarketDataQuery,
    loadMarketDataQuery,
    marketDataCandles,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    marketSecurityDetails,
    marketDataSnapshot,
  };
}