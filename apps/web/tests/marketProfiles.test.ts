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
    expect(profiles.pricePrecisionForMarket("US")).toBe(2);
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
    expect(profiles.pricePrecisionForMarket("HK")).toBe(3);
    expect(profiles.supportsExtendedHoursForMarket("US")).toBe(false);
  });

  it("collapses exchange-level China profiles into one A-share option", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          defaultMarket: "HK",
          updatedAt: "2026-07-13T00:00:00.000Z",
          markets: [
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
            {
              code: "SH",
              resolvedMarket: "CN",
              preferredPrefix: "SH",
              displayName: "Shanghai",
              quoteCurrency: "CNY",
              supportsExtendedHours: false,
              requiresExchangePrefix: true,
              aliases: ["CNSH"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
            {
              code: "SZ",
              resolvedMarket: "CN",
              preferredPrefix: "SZ",
              displayName: "Shenzhen",
              quoteCurrency: "CNY",
              supportsExtendedHours: false,
              requiresExchangePrefix: true,
              aliases: ["CNSZ"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
          ],
        }),
      ),
    );

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    await profiles.loadMarketProfiles();

    expect(profiles.marketOptions.value).toEqual([
      { value: "HK", title: "港股 HK" },
      { value: "CN", title: "沪深" },
    ]);
    expect(profiles.quoteCurrencyForMarket("SH")).toBe("CNY");
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

  it("deduplicates canonical profiles, keeps aliases, and falls back for unknown markets", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        createResponse({
          defaultMarket: " ",
          updatedAt: "2026-07-16T00:00:00.000Z",
          markets: [
            {
              code: "US",
              resolvedMarket: "US",
              preferredPrefix: "US",
              displayName: "",
              quoteCurrency: "USD",
              supportsExtendedHours: true,
              requiresExchangePrefix: false,
              aliases: ["NASDAQ"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
            {
              code: "US",
              resolvedMarket: "US",
              preferredPrefix: "US",
              displayName: "United States",
              quoteCurrency: "USD",
              supportsExtendedHours: true,
              requiresExchangePrefix: false,
              aliases: ["NYSE"],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
            {
              code: "",
              resolvedMarket: "",
              preferredPrefix: "",
              displayName: "invalid",
              quoteCurrency: "",
              supportsExtendedHours: false,
              requiresExchangePrefix: false,
              aliases: [],
              regularSessions: [],
              precision: { price: 2, quote: 2 },
              tickSize: 0.01,
            },
          ],
        }),
      ),
    );

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    await profiles.loadMarketProfiles();

    expect(profiles.defaultMarket.value).toBe("HK");
    expect(profiles.marketOptions.value).toEqual([{ value: "US", title: "美股 US" }]);
    expect(profiles.findMarketProfile(" nyse ")?.quoteCurrency).toBe("USD");
    expect(profiles.findMarketProfile(" ")).toBeNull();
    expect(profiles.quoteCurrencyForMarket("missing")).toBe("HKD");
  });

  it("shares an in-flight profile request instead of issuing duplicate market metadata calls", async () => {
    let resolveResponse: ((value: Response) => void) | undefined;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveResponse = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    const first = profiles.loadMarketProfiles();
    const second = profiles.loadMarketProfiles();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    resolveResponse!(createResponse({ defaultMarket: "US", markets: [] }));
    await Promise.all([first, second]);
    expect(profiles.defaultMarket.value).toBe("US");
    expect(profiles.isLoadingMarketProfiles.value).toBe(false);
  });

  it("keeps unfamiliar market names intelligible without hiding their exchange code", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => createResponse({
        defaultMarket: "OTC",
        markets: [
          {
            code: "OTC",
            resolvedMarket: "OTC",
            preferredPrefix: "OTC",
            displayName: "Over The Counter",
            quoteCurrency: "USD",
            supportsExtendedHours: false,
            requiresExchangePrefix: false,
            aliases: [],
            regularSessions: [],
            precision: { price: 4, quote: 4 },
            tickSize: 0.0001,
          },
          {
            code: "X",
            resolvedMarket: "X",
            preferredPrefix: "X",
            displayName: " ",
            quoteCurrency: "USD",
            supportsExtendedHours: false,
            requiresExchangePrefix: false,
            aliases: [],
            regularSessions: [],
            precision: { price: 4, quote: 4 },
            tickSize: 0.0001,
          },
        ],
      })),
    );

    const module = await loadFreshMarketProfilesModule();
    const profiles = module.useMarketProfiles();
    await profiles.loadMarketProfiles();

    expect(profiles.marketOptions.value).toEqual([
      { value: "OTC", title: "Over The Counter OTC" },
      { value: "X", title: "X" },
    ]);
  });
});
