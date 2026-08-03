// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import {
  refreshMarketProfiles,
  resetMarketProfilesForTests,
  useMarketProfiles,
} from "@/composables/market-data/marketProfiles";
import { resetSharedLiveSocketHubForTests } from "@/composables/market-data/sharedLiveSocket";
import { provideConsoleDataStore } from "@/composables/workspace/useConsoleData";
import {
  provideWorkspaceTradingPreferencesStore,
  type WorkspaceTradingPreferencesStore,
} from "@/composables/workspace/useWorkspaceLayout";
import { createResponse } from "../helpers";

interface ConsoleHarness {
  consoleData: ReturnType<typeof provideConsoleDataStore>;
  trading: WorkspaceTradingPreferencesStore;
  wrapper: VueWrapper;
}

function createConsoleHarness(): ConsoleHarness {
  let consoleData: ReturnType<typeof provideConsoleDataStore> | null = null;
  let trading: WorkspaceTradingPreferencesStore | null = null;
  const Host = defineComponent({
    setup() {
      trading = provideWorkspaceTradingPreferencesStore();
      consoleData = provideConsoleDataStore(trading);
      return () => null;
    },
  });
  const wrapper = mount(Host);
  if (consoleData == null || trading == null) {
    throw new Error("failed to create console data harness");
  }
  return { consoleData, trading, wrapper };
}

function usProviderFetchMock() {
  return vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/api/v1/market-data/markets")) {
      return createResponse({
        defaultMarket: "US",
        markets: [{ code: "US", resolvedMarket: "US" }],
      });
    }
    if (url.includes("/api/v1/market-data/instruments?")) {
      return createResponse({
        requestedMarket: "US",
        query: "US.AAPL",
        resolutionStatus: "resolved",
        totalReturned: 1,
        entries: [
          {
            market: "US",
            resolvedMarket: "US",
            instrumentId: "US.AAPL",
            code: "AAPL",
            symbol: "AAPL",
            selectable: true,
          },
        ],
        failures: [],
      });
    }
    if (
      url.includes("/api/v1/market-data/snapshots/") ||
      url.includes("/api/v1/market-data/securities/") ||
      url.includes("/api/v1/market-data/candles/")
    ) {
      throw new Error("test request stopped");
    }
    throw new Error(`unexpected request ${url}`);
  });
}

type ProviderMarketProfiles = {
  defaultMarket: string;
  markets: Array<Record<string, unknown>>;
};

function providerSwitchFetchMock(
  initial: ProviderMarketProfiles,
  replacement: ProviderMarketProfiles,
  selectableAfterSwitch: string[],
) {
  let switched = false;
  const selectable = new Set(
    selectableAfterSwitch.map((instrumentId) => instrumentId.toUpperCase()),
  );
  const fetchMock = vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/api/v1/market-data/markets")) {
      return createResponse(switched ? replacement : initial);
    }
    if (url.includes("/api/v1/market-data/instruments?")) {
      const query =
        new URL(url, "http://localhost").searchParams.get("query")?.toUpperCase() ?? "";
      const [market = "", ...symbolParts] = query.split(".");
      const symbol = symbolParts.join(".");
      const resolved = selectable.has(query);
      return createResponse({
        requestedMarket: market,
        query,
        resolutionStatus: resolved ? "resolved" : "not_found",
        totalReturned: resolved ? 1 : 0,
        entries: resolved
          ? [{
              market,
              resolvedMarket: market,
              instrumentId: query,
              code: symbol,
              symbol,
              selectable: true,
            }]
          : [],
        failures: [],
      });
    }
    if (
      url.includes("/api/v1/market-data/snapshots/") ||
      url.includes("/api/v1/market-data/securities/") ||
      url.includes("/api/v1/market-data/candles/")
    ) {
      throw new Error("test request stopped");
    }
    throw new Error(`unexpected request ${url}`);
  });
  return {
    fetchMock,
    switchProvider: () => {
      switched = true;
    },
  };
}

afterEach(() => {
  resetMarketProfilesForTests();
  resetSharedLiveSocketHubForTests();
  window.localStorage.clear();
  window.sessionStorage.clear();
  vi.unstubAllGlobals();
});

describe("console data provider switching", () => {
  it("persists a provider-compatible fallback before issuing the replacement query", async () => {
    const fetchMock = usProviderFetchMock();
    vi.stubGlobal("fetch", fetchMock);
    await refreshMarketProfiles();
    const harness = createConsoleHarness();
    harness.consoleData.marketInstrumentReferences.value = [
      {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
        name: "Apple",
        securityType: "EQUITY",
        lotSize: 1,
        exchange: "NASDAQ",
        status: "NORMAL",
        source: "test",
        updatedAt: "2026-07-30T00:00:00Z",
      },
    ];

    await harness.consoleData.reloadMarketDataProvider();

    expect(harness.consoleData.activeMarketDataInstrumentId.value).toBe(
      "US.AAPL",
    );
    expect(harness.trading.prefs.value).toMatchObject({
      market: "US",
      symbol: "AAPL",
    });
    const queryUrls = fetchMock.mock.calls
      .map(([input]) => String(input))
      .filter(
        (url) =>
          !url.includes("/market-data/markets") &&
          !url.includes("/market-data/instruments?"),
      );
    expect(queryUrls).toHaveLength(3);
    expect(queryUrls.every((url) => url.includes("/US/AAPL"))).toBe(true);
    expect(queryUrls.some((url) => url.includes("/HK/00700"))).toBe(false);

    harness.consoleData.dispose();
    harness.wrapper.unmount();
  });

  it("clears persisted and active instruments without querying an unsupported market", async () => {
    const fetchMock = usProviderFetchMock();
    vi.stubGlobal("fetch", fetchMock);
    await refreshMarketProfiles();
    const harness = createConsoleHarness();

    await harness.consoleData.reloadMarketDataProvider();

    expect(harness.consoleData.marketDataQueryMarket.value).toBe("US");
    expect(harness.consoleData.marketDataQuerySymbol.value).toBe("");
    expect(harness.consoleData.activeMarketDataInstrumentId.value).toBe("");
    expect(harness.trading.prefs.value).toMatchObject({
      market: "US",
      symbol: "",
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    harness.consoleData.dispose();
    harness.wrapper.unmount();
  });

  it("refreshes Futu market profiles before reconciling an extended market with Yahoo", async () => {
    const provider = providerSwitchFetchMock(
      {
        defaultMarket: "JP",
        markets: [
          { code: "US", resolvedMarket: "US" },
          { code: "JP", resolvedMarket: "JP" },
        ],
      },
      {
        defaultMarket: "US",
        markets: [
          { code: "US", resolvedMarket: "US" },
          { code: "HK", resolvedMarket: "HK" },
          { code: "SH", resolvedMarket: "CN", preferredPrefix: "SH" },
          { code: "SZ", resolvedMarket: "CN", preferredPrefix: "SZ" },
        ],
      },
      ["US.AAPL"],
    );
    vi.stubGlobal("fetch", provider.fetchMock);
    await refreshMarketProfiles();
    expect(useMarketProfiles().marketOptions.value.map((option) => option.value)).toEqual([
      "US",
      "JP",
    ]);

    const harness = createConsoleHarness();
    harness.consoleData.selectWorkspaceInstrument({ market: "JP", symbol: "7203" });
    harness.consoleData.marketInstrumentReferences.value = [
      {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
        name: "Apple",
        securityType: "EQUITY",
        lotSize: 1,
        exchange: "NASDAQ",
        status: "NORMAL",
        source: "test",
        updatedAt: "2026-07-30T00:00:00Z",
      },
    ];

    provider.switchProvider();
    await harness.consoleData.reloadMarketDataProvider();

    expect(harness.consoleData.activeMarketDataInstrumentId.value).toBe("US.AAPL");
    expect(harness.trading.prefs.value).toMatchObject({ market: "US", symbol: "AAPL" });
    expect(useMarketProfiles().marketOptions.value.map((option) => option.value)).toEqual([
      "US",
      "HK",
      "CN",
    ]);
    const urls = provider.fetchMock.mock.calls.map(([input]) => String(input));
    expect(urls.filter((url) => url.includes("/market-data/markets"))).toHaveLength(2);
    expect(urls.some((url) => url.includes("/JP/7203"))).toBe(false);
    expect(urls.some((url) => url.includes("/US/AAPL"))).toBe(true);

    harness.consoleData.dispose();
    harness.wrapper.unmount();
  });

  it("refreshes Yahoo profiles before retaining a Futu-supported market and exposing its extensions", async () => {
    const provider = providerSwitchFetchMock(
      {
        defaultMarket: "US",
        markets: [
          { code: "US", resolvedMarket: "US" },
          { code: "HK", resolvedMarket: "HK" },
          { code: "SH", resolvedMarket: "CN", preferredPrefix: "SH" },
          { code: "SZ", resolvedMarket: "CN", preferredPrefix: "SZ" },
        ],
      },
      {
        defaultMarket: "HK",
        markets: [
          { code: "US", resolvedMarket: "US" },
          { code: "HK", resolvedMarket: "HK" },
          { code: "JP", resolvedMarket: "JP" },
        ],
      },
      ["US.AAPL"],
    );
    vi.stubGlobal("fetch", provider.fetchMock);
    await refreshMarketProfiles();
    const harness = createConsoleHarness();
    harness.consoleData.selectWorkspaceInstrument({ market: "US", symbol: "AAPL" });

    provider.switchProvider();
    await harness.consoleData.reloadMarketDataProvider();

    expect(harness.consoleData.activeMarketDataInstrumentId.value).toBe("US.AAPL");
    expect(harness.trading.prefs.value).toMatchObject({ market: "US", symbol: "AAPL" });
    expect(useMarketProfiles().marketOptions.value.map((option) => option.value)).toEqual([
      "US",
      "HK",
      "JP",
    ]);
    const urls = provider.fetchMock.mock.calls.map(([input]) => String(input));
    expect(urls.filter((url) => url.includes("/market-data/markets"))).toHaveLength(2);
    expect(urls.some((url) => url.includes("/US/AAPL"))).toBe(true);

    harness.consoleData.dispose();
    harness.wrapper.unmount();
  });
});
