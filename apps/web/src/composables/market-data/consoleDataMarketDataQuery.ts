import { computed, ref } from "vue";

import type { MarketProfileDto } from "@/types";

import { apiGetPath } from "@/composables/shared/apiClient";
import {
  normalizeInstrumentParts,
} from "@/composables/market-data/consoleDataMarketInstruments";
import { resolveMarketInstrumentCandidates } from "@/composables/market-data/instrumentResolver";
import { useMarketProfiles } from "@/composables/market-data/marketProfiles";
import {
  createMarketDataQueryController,
  type LoadMarketDataQueryOptions,
} from "@/composables/market-data/marketDataQuery";
import {
  type MarketDataCandlesQueryResult,
  type MarketSecurityDetailsQueryResult,
  type MarketDataSnapshotQueryResult,
  normalizeMarketDataCandlesQueryResult,
  normalizeMarketDataSnapshotQueryResult,
} from "@/composables/market-data/marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "@/composables/market-data/marketSecurityNormalization";

interface ProviderFallbackInstrument {
  market: string;
  symbol: string;
}

const DEFAULT_PROVIDER_FALLBACK_INSTRUMENTS: readonly ProviderFallbackInstrument[] = [
  { market: "HK", symbol: "00700" },
];

interface ProviderInstrumentIdentity {
  market: string;
  symbol: string;
  instrumentId: string;
}

function normalizeMarket(value: string): string {
  return value.trim().toUpperCase();
}

function profileIdentity(profile: MarketProfileDto): string {
  return normalizeMarket(profile.resolvedMarket);
}

function providerInstrumentIdentity(
  market: string,
  symbol: string,
): ProviderInstrumentIdentity | null {
  const normalizedMarket = normalizeMarket(market);
  const normalizedSymbol = symbol.trim().toUpperCase().replace(":", ".");
  if (normalizedMarket === "" || normalizedSymbol === "") {
    return null;
  }
  return {
    market: normalizedMarket,
    symbol: normalizedSymbol,
    instrumentId: `${normalizedMarket}.${normalizedSymbol}`,
  };
}

function candidateMatchesProviderInstrument(
  candidate: { instrumentId: string },
  expected: ProviderInstrumentIdentity,
): boolean {
  return candidate.instrumentId
    .trim()
    .toUpperCase()
    .replace(":", ".") === expected.instrumentId;
}

async function isSelectableProviderInstrument(
  market: string,
  symbol: string,
): Promise<boolean> {
  const expected = providerInstrumentIdentity(market, symbol);
  if (expected == null) {
    return false;
  }
  try {
    const resolution = await resolveMarketInstrumentCandidates({
      market: expected.market,
      query: expected.instrumentId,
      limit: 1,
    });
    const candidate = resolution.entries[0];
    return (
      resolution.resolutionStatus === "resolved" &&
      resolution.entries.length === 1 &&
      candidate?.selectable === true &&
      candidateMatchesProviderInstrument(candidate, expected)
    );
  } catch {
    return false;
  }
}

export function createConsoleDataMarketDataQuerySlice() {
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
  const isLoadingOlderMarketData = ref(false);
  const hasMoreMarketDataHistory = ref(false);
  const marketDataNextBefore = ref("");
  const marketDataOlderError = ref("");
  const marketDataQueryError = ref("");
  const lastDataRefreshedAt = ref(0);
  const {
    defaultMarket,
    findMarketProfile,
    marketOptions,
  } = useMarketProfiles();

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
      isLoadingOlderMarketData,
      hasMoreMarketDataHistory,
      marketDataNextBefore,
      marketDataOlderError,
      marketDataQueryError,
      lastDataRefreshedAt,
    },
    requestSnapshot: async (path) =>
      normalizeMarketDataSnapshotQueryResult(
        await apiGetPath(
          "/api/v1/market-data/snapshots/{market}/{symbol}",
          path,
        ),
      ),
    requestSecurityDetails: async (path) =>
      normalizeMarketSecurityDetailsQueryResult(
        await apiGetPath(
          "/api/v1/market-data/securities/{market}/{symbol}",
          path,
        ),
      ),
    requestCandles: async (path) =>
      normalizeMarketDataCandlesQueryResult(
        await apiGetPath(
          "/api/v1/market-data/candles/{market}/{symbol}",
          path,
        ),
      ),
    normalizeInstrumentParts,
  });

  function invalidateMarketDataProvider(): void {
    marketDataQueryController.invalidateProviderSelection();
  }

  async function reconcileMarketDataProvider(
    fallbackInstruments: ProviderFallbackInstrument[],
  ): Promise<boolean> {
    invalidateMarketDataProvider();
    const currentMarket = normalizeMarket(marketDataQueryMarket.value);
    const currentSymbol = marketDataQuerySymbol.value.trim().toUpperCase();
    if (
      currentSymbol !== "" &&
      activeMarketDataInstrumentId.value !== "" &&
      findMarketProfile(currentMarket) != null
    ) {
      if (await isSelectableProviderInstrument(currentMarket, currentSymbol)) {
        return true;
      }
    }

    const fallbackMarket = [
      defaultMarket.value,
      ...marketOptions.value.map((option) => option.value),
    ]
      .map(normalizeMarket)
      .find((market) => findMarketProfile(market) != null) ?? "";
    const preferredProfile = findMarketProfile(fallbackMarket);
    const supportedFallbacks = fallbackInstruments.flatMap((instrument) => {
      const profile = findMarketProfile(instrument.market);
      return profile == null ? [] : [{ instrument, profile }];
    });
    const orderedFallbacks =
      preferredProfile == null
        ? supportedFallbacks
        : [
            ...supportedFallbacks.filter(
              ({ profile }) =>
                profileIdentity(profile) === profileIdentity(preferredProfile),
            ),
            ...supportedFallbacks.filter(
              ({ profile }) =>
                profileIdentity(profile) !== profileIdentity(preferredProfile),
            ),
          ];
    const fallbackCandidates = [
      ...orderedFallbacks,
      ...DEFAULT_PROVIDER_FALLBACK_INSTRUMENTS.flatMap((instrument) => {
        const profile = findMarketProfile(instrument.market);
        return profile == null ? [] : [{ instrument, profile }];
      }),
    ];
    const checked = new Set<string>();
    const currentIdentity = providerInstrumentIdentity(
      currentMarket,
      currentSymbol,
    );
    if (currentIdentity != null) {
      checked.add(currentIdentity.instrumentId);
    }
    for (const fallback of fallbackCandidates) {
      const identity = providerInstrumentIdentity(
        fallback.instrument.market,
        fallback.instrument.symbol,
      );
      if (identity == null || checked.has(identity.instrumentId)) {
        continue;
      }
      checked.add(identity.instrumentId);
      if (
        await isSelectableProviderInstrument(
          identity.market,
          identity.symbol,
        )
      ) {
        marketDataQueryController.selectInstrument(fallback.instrument);
        return activeMarketDataInstrumentId.value !== "";
      }
    }

    marketDataQueryMarket.value = fallbackMarket;
    marketDataQuerySymbol.value = "";
    activeMarketDataInstrumentId.value = "";
    return false;
  }

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
    isLoadingOlderMarketData,
    hasMoreMarketDataHistory,
    invalidateMarketDataProvider,
    isMarketDataSwitching,
    lastDataRefreshedAt,
    loadMarketDataQuery,
    marketDataCandles,
    marketDataQueryError,
    marketDataNextBefore,
    marketDataOlderError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    reconcileMarketDataProvider,
    marketSecurityDetails,
    marketDataSnapshot,
    selectMarketDataInstrument,
  };
}
