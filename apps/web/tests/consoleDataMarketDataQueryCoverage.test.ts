import { describe, expect, it } from "vitest";

import { createConsoleDataMarketDataQuerySlice } from "../src/composables/consoleDataMarketDataQuery";

describe("console market-data freshness", () => {
  it("treats an untouched query as stale before a first backend response arrives", () => {
    const marketData = createConsoleDataMarketDataQuerySlice();

    expect(marketData.isMarketDataStale()).toBe(true);

    marketData.lastDataRefreshedAt.value = Date.now();
    expect(marketData.isMarketDataStale(1_000)).toBe(false);
    marketData.lastDataRefreshedAt.value = Date.now() - 1_001;
    expect(marketData.isMarketDataStale(1_000)).toBe(true);
  });
});
