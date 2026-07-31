import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiGet: vi.fn(),
  apiGetPath: vi.fn(),
  apiPost: vi.fn(),
}));

vi.mock("@/composables/shared/apiClient", () => ({
  apiGet: mocks.apiGet,
  apiGetPath: mocks.apiGetPath,
  apiPost: mocks.apiPost,
}));

import { createConsoleDataMarketDataQuerySlice } from "@/composables/market-data/consoleDataMarketDataQuery";
import {
  refreshMarketProfiles,
  resetMarketProfilesForTests,
} from "@/composables/market-data/marketProfiles";

beforeEach(() => {
  vi.clearAllMocks();
  resetMarketProfilesForTests();
  mocks.apiGet.mockResolvedValue({
    defaultMarket: "US",
    markets: [{ code: "US", resolvedMarket: "US" }],
  });
  mocks.apiGetPath.mockRejectedValue(new Error("test request stopped"));
});

function instrumentResolution(instrumentId: string) {
  const separator = instrumentId.indexOf(".");
  const market = instrumentId.slice(0, separator).toUpperCase();
  const symbol = instrumentId.slice(separator + 1).toUpperCase();
  return {
    requestedMarket: market,
    query: instrumentId,
    resolutionStatus: "resolved",
    totalReturned: 1,
    entries: [
      {
        market,
        resolvedMarket: market,
        instrumentId: `${market}.${symbol}`,
        code: symbol,
        symbol,
        selectable: true,
      },
    ],
    failures: [],
  };
}

function mockExactInstrumentLookups(validInstrumentIds: string[]): void {
  const valid = new Set(validInstrumentIds.map((id) => id.toUpperCase()));
  mocks.apiGetPath.mockImplementation(async (_template, path) => {
    const requestPath = String(path);
    if (!requestPath.includes("/api/v1/market-data/instruments?")) {
      throw new Error("test request stopped");
    }
    const query = new URL(requestPath, "http://localhost").searchParams.get(
      "query",
    );
    if (query != null && valid.has(query.toUpperCase())) {
      return instrumentResolution(query);
    }
    return {
      requestedMarket: "HK",
      query: query ?? "",
      resolutionStatus: "not_found",
      totalReturned: 0,
      entries: [],
      failures: [],
    };
  });
}

afterEach(() => {
  resetMarketProfilesForTests();
});

describe("console market-data provider scope", () => {
  it("moves an unsupported HK query to a known US instrument before reloading", async () => {
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();

    mockExactInstrumentLookups(["US.AAPL"]);
    const shouldLoad = await marketData.reconcileMarketDataProvider([
      { market: "HK", symbol: "00700" },
      { market: "US", symbol: "AAPL" },
    ]);

    expect(shouldLoad).toBe(true);
    expect(marketData.marketDataQueryMarket.value).toBe("US");
    expect(marketData.marketDataQuerySymbol.value).toBe("AAPL");
    expect(marketData.activeMarketDataInstrumentId.value).toBe("US.AAPL");

    await marketData.loadMarketDataQuery();

    const requestedPaths = mocks.apiGetPath.mock.calls
      .map(([, path]) => String(path))
      .filter((path) => !path.includes("/market-data/instruments?"));
    expect(requestedPaths).toHaveLength(3);
    expect(requestedPaths.every((path) => path.includes("/US/AAPL"))).toBe(
      true,
    );
    expect(requestedPaths.some((path) => path.includes("/HK/00700"))).toBe(
      false,
    );
    marketData.disposeMarketDataQuery();
  });

  it("clears an unsupported instrument when no provider-compatible fallback exists", async () => {
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();

    const shouldLoad = await marketData.reconcileMarketDataProvider([
      { market: "HK", symbol: "00700" },
    ]);

    expect(shouldLoad).toBe(false);
    expect(marketData.marketDataQueryMarket.value).toBe("US");
    expect(marketData.marketDataQuerySymbol.value).toBe("");
    expect(marketData.activeMarketDataInstrumentId.value).toBe("");
    expect(mocks.apiGetPath).not.toHaveBeenCalled();
    marketData.disposeMarketDataQuery();
  });

  it("keeps the active instrument when the refreshed provider still supports its market", async () => {
    mocks.apiGet.mockResolvedValue({
      defaultMarket: "HK",
      markets: [
        { code: "HK", resolvedMarket: "HK" },
        { code: "US", resolvedMarket: "US" },
      ],
    });
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();

    mockExactInstrumentLookups(["HK.00700"]);
    const shouldLoad = await marketData.reconcileMarketDataProvider([
      { market: "US", symbol: "AAPL" },
    ]);

    expect(shouldLoad).toBe(true);
    expect(marketData.marketDataQueryMarket.value).toBe("HK");
    expect(marketData.marketDataQuerySymbol.value).toBe("00700");
    expect(marketData.activeMarketDataInstrumentId.value).toBe("HK.00700");
    marketData.disposeMarketDataQuery();
  });

  it("clears the query when provider market metadata is unavailable", async () => {
    mocks.apiGet.mockRejectedValue(new Error("provider markets unavailable"));
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();

    const shouldLoad = await marketData.reconcileMarketDataProvider([
      { market: "US", symbol: "AAPL" },
    ]);

    expect(shouldLoad).toBe(false);
    expect(marketData.marketDataQueryMarket.value).toBe("");
    expect(marketData.marketDataQuerySymbol.value).toBe("");
    expect(marketData.activeMarketDataInstrumentId.value).toBe("");
    expect(mocks.apiGetPath).not.toHaveBeenCalled();
    marketData.disposeMarketDataQuery();
  });

  it("replaces a market-supported but provider-unknown instrument", async () => {
    mocks.apiGet.mockResolvedValue({
      defaultMarket: "HK",
      markets: [
        { code: "HK", resolvedMarket: "HK" },
        { code: "US", resolvedMarket: "US" },
      ],
    });
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();
    marketData.selectMarketDataInstrument({ market: "HK", symbol: "HTIMAIN" });
    mockExactInstrumentLookups(["HK.00700"]);

    const shouldLoad = await marketData.reconcileMarketDataProvider([
      { market: "HK", symbol: "HTIMAIN" },
      { market: "HK", symbol: "00700" },
    ]);

    expect(shouldLoad).toBe(true);
    expect(marketData.marketDataQueryMarket.value).toBe("HK");
    expect(marketData.marketDataQuerySymbol.value).toBe("00700");
    expect(marketData.activeMarketDataInstrumentId.value).toBe("HK.00700");
    const lookupQueries = mocks.apiGetPath.mock.calls
      .map(([, path]) => String(path))
      .filter((path) => path.includes("/market-data/instruments?"));
    expect(lookupQueries).toHaveLength(2);
    expect(lookupQueries[0]).toContain("query=HK.HTIMAIN");
    expect(lookupQueries[1]).toContain("query=HK.00700");
    marketData.disposeMarketDataQuery();
  });

  it("uses the default Futu instrument when no local fallback candidates exist", async () => {
    mocks.apiGet.mockResolvedValue({
      defaultMarket: "HK",
      markets: [{ code: "HK", resolvedMarket: "HK" }],
    });
    await refreshMarketProfiles();
    const marketData = createConsoleDataMarketDataQuerySlice();
    marketData.selectMarketDataInstrument({ market: "HK", symbol: "HTIMAIN" });
    mockExactInstrumentLookups(["HK.00700"]);

    const shouldLoad = await marketData.reconcileMarketDataProvider([]);

    expect(shouldLoad).toBe(true);
    expect(marketData.activeMarketDataInstrumentId.value).toBe("HK.00700");
    marketData.disposeMarketDataQuery();
  });
});
