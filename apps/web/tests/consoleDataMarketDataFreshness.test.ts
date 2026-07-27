import { afterEach, describe, expect, it } from "vitest";
import { nextTick } from "vue";

import { createConsoleDataMarketDataQuerySlice } from "../src/composables/consoleDataMarketDataQuery";
import {
  resetBrokerProviderSelectionForTests,
  useBrokerProviderSelection,
} from "../src/composables/brokerProviderSelection";

afterEach(() => {
  resetBrokerProviderSelectionForTests();
});

describe("console market-data freshness", () => {
  it("treats an untouched query as stale before a first backend response arrives", () => {
    const marketData = createConsoleDataMarketDataQuerySlice();

    expect(marketData.isMarketDataStale()).toBe(true);

    marketData.lastDataRefreshedAt.value = Date.now();
    expect(marketData.isMarketDataStale(1_000)).toBe(false);
    marketData.lastDataRefreshedAt.value = Date.now() - 1_001;
    expect(marketData.isMarketDataStale(1_000)).toBe(true);
  });

  it("invalidates displayed market data when the provider changes", async () => {
    const marketData = createConsoleDataMarketDataQuerySlice();
    marketData.marketDataSnapshot.value = {
      request: { market: "HK", symbol: "00700", instrumentId: "HK.00700" },
      snapshot: null,
      meta: {
        instrumentId: "HK.00700",
        source: "test",
        resolvedAt: "2026-07-18T00:00:00Z",
        fromCache: false,
      },
    };
    marketData.lastDataRefreshedAt.value = 123;

    useBrokerProviderSelection().selectBrokerProvider("alpha");
    await nextTick();

    expect(marketData.marketDataSnapshot.value).toBeNull();
    expect(marketData.lastDataRefreshedAt.value).toBe(0);
    marketData.disposeMarketDataQuery();
  });
});
