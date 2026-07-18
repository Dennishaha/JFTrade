import { computed, ref, watch } from "vue";

import {
  fetchEnvelope,
} from "./apiClient";
import {
  normalizeInstrumentParts,
} from "./consoleDataMarketInstruments";
import { useBrokerProviderSelection } from "./brokerProviderSelection";
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
  const { selectedBrokerId } = useBrokerProviderSelection();
  const marketDataQueryMarket = ref("HK");
  const marketDataQuerySymbol = ref("00700");
  const marketDataQueryPeriod = ref("1m");
  const marketDataQueryLimit = ref(500);
  const activeMarketDataInstrumentId = ref("HK.00700");
  const isMarketDataSwitching = ref(false);
  const marketDataSnapshot = ref<MarketDataSnapshotQueryResult | null>(null);
  const marketSecurityDetails = ref<MarketSecurityDetailsQueryResult | null>(null);
  const marketDataCandles = ref<MarketDataCandlesQueryResult | null>(null);
  const isLoadingMarketDataQuery = ref(false);
  const marketDataQueryError = ref("");
  const lastDataRefreshedAt = ref(0);

  function isMarketDataStale(maxAgeMs = 30_000): boolean {
    if (lastDataRefreshedAt.value === 0) return true;
    return Date.now() - lastDataRefreshedAt.value > maxAgeMs;
  }

  const marketDataQueryController = createMarketDataQueryController({
    state: {
      marketDataQueryMarket,
      marketDataQuerySymbol,
      marketDataQueryPeriod,
      marketDataQueryLimit,
      activeMarketDataInstrumentId,
      isMarketDataSwitching,
      marketDataSnapshot,
      marketSecurityDetails,
      marketDataCandles,
      isLoadingMarketDataQuery,
      marketDataQueryError,
      lastDataRefreshedAt,
    },
    fetchEnvelope,
    normalizeInstrumentParts,
    resolveBrokerId: () => selectedBrokerId.value,
  });
  watch(selectedBrokerId, () => {
    marketDataQueryController.invalidateProviderSelection();
  });

  const currentMarketDataSnapshot = computed(() =>
    marketDataSnapshot.value?.request.instrumentId.trim().toUpperCase() ===
    activeMarketDataInstrumentId.value
      ? marketDataSnapshot.value
      : null,
  );
  const currentMarketSecurityDetails = computed(() =>
    marketSecurityDetails.value?.request.instrumentId.trim().toUpperCase() ===
    activeMarketDataInstrumentId.value
      ? marketSecurityDetails.value
      : null,
  );
  const currentMarketDataCandles = computed(() => {
    const result = marketDataCandles.value;
    return result?.request.instrument.instrumentId.trim().toUpperCase() ===
      activeMarketDataInstrumentId.value &&
      result.request.period === marketDataQueryPeriod.value
      ? result
      : null;
  });

  function selectMarketDataInstrument(input: {
    market: string;
    symbol: string;
    period?: string;
  }): void {
    marketDataQueryController.selectInstrument(input);
  }

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
    activeMarketDataInstrumentId,
    currentMarketDataCandles,
    currentMarketDataSnapshot,
    currentMarketSecurityDetails,
    disposeMarketDataQuery: marketDataQueryController.dispose,
    isMarketDataStale,
    isLoadingMarketDataQuery,
    isMarketDataSwitching,
    lastDataRefreshedAt,
    loadMarketDataQuery,
    marketDataCandles,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    marketSecurityDetails,
    marketDataSnapshot,
    selectMarketDataInstrument,
  };
}
