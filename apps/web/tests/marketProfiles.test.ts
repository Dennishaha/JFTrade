import { afterEach, describe, expect, it, vi } from "vitest";

import { createResponse } from "./helpers";

async function loadFreshMarketProfilesModule() {
  vi.resetModules();
  return import("../src/composables/marketProfiles");
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("marketProfiles", () => {
  it("loads market profiles from the backend", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse({
        defaultMarket: "HK",
        updatedAt: "2026-06-12T00:00:00.000Z",
        markets: [
          {
            code: "US",
            resolvedMarket: "US",
            preferredPrefix: "US",
            displayName: "US",
            quoteCurrency: "USD",
            supportsExtendedHours: true,
            requiresExchangePrefix: false,
            aliases: ["NYSE"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
          {
            code: "HK",
            resolvedMarket: "HK",
            preferredPrefix: "HK",
            displayName: "Hong Kong",
            quoteCurrency: "HKD",
            supportsExtendedHours: false,
            requiresExchangePrefix: false,
            aliases: ["HKEX"],
            regularSessions: [],
            precision: { price: 3, quote: 3 },
            tickSize: 0.001,
          },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    await profiles.loadMarketProfiles();

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/market-data/markets"),
      expect.objectContaining({ credentials: "include" }),
    );
    expect(profiles.marketOptions.value).toEqual([
      { value: "US", title: "美股 US" },
      { value: "HK", title: "港股 HK" },
    ]);
    expect(profiles.quoteCurrencyForMarket("US")).toBe("USD");
    expect(profiles.supportsExtendedHoursForMarket("HK")).toBe(false);
  });

  it("exposes an empty profile list when metadata loading fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("offline");
      }),
    );

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    await profiles.loadMarketProfiles();

    expect(profiles.marketOptions.value).toEqual([]);
    expect(profiles.marketProfiles.value).toEqual([]);
    expect(profiles.marketProfilesError.value).toBe("offline");
    expect(profiles.supportsExtendedHoursForMarket("US")).toBe(false);
  });

  it("normalizes instruments through the backend API", async () => {
    const fetchMock = vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toEqual({
        market: "SH",
        code: "600519",
      });
      return createResponse({
        market: "CN",
        prefix: "SH",
        code: "600519",
        symbol: "SH.600519",
        instrumentId: "SH.600519",
        resolvedMarket: "CN",
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const module = await loadFreshMarketProfilesModule();
    await expect(
      module.normalizeInstrumentRefWithMarketApi({
        market: "SH",
        code: "600519",
      }),
    ).resolves.toMatchObject({
      market: "CN",
      prefix: "SH",
      code: "600519",
      instrumentId: "SH.600519",
    });
  });
});
