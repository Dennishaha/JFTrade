// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { MarketDataSubscriptionsResponse } from "../src/contracts";
import type { MarketInstrumentReference } from "../src/composables/consoleDataSystemState";

const storageKey = "jftrade.market-data.consumer.market-page";

afterEach(() => {
  vi.resetModules();
  vi.unstubAllGlobals();
  window.sessionStorage?.clear();
});

describe("createStableWebConsumerId", () => {
  it("keeps cloned sessionStorage bases but separates browser windows", async () => {
    window.sessionStorage.setItem(storageKey, "web:market-page:cloned");

    vi.stubGlobal("crypto", {
      randomUUID: vi.fn(() => "window-a"),
    });
    const firstModule = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const firstId = firstModule.createStableWebConsumerId("market-page");

    vi.resetModules();
    vi.stubGlobal("crypto", {
      randomUUID: vi.fn(() => "window-b"),
    });
    const secondModule = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const secondId = secondModule.createStableWebConsumerId("market-page");

    expect(firstId).toBe("web:market-page:cloned:window:window-a");
    expect(secondId).toBe("web:market-page:cloned:window:window-b");
    expect(firstId).not.toBe(secondId);
    expect(window.sessionStorage.getItem(storageKey)).toBe(
      "web:market-page:cloned",
    );
  });

  it("returns a stable id within the same page instance", async () => {
    vi.stubGlobal("crypto", {
      randomUUID: vi
        .fn()
        .mockReturnValueOnce("window-a")
        .mockReturnValueOnce("base-a"),
    });
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );

    expect(module.createStableWebConsumerId("market-page")).toBe(
      "web:market-page:base-a:window:window-a",
    );
    expect(module.createStableWebConsumerId("market-page")).toBe(
      "web:market-page:base-a:window:window-a",
    );
  });

  it("strips a cloned window suffix and falls back when randomUUID is unavailable", async () => {
    window.sessionStorage.setItem(
      storageKey,
      "web:market-page:base-old:window:window-old",
    );
    vi.stubGlobal("crypto", {});
    vi.spyOn(Math, "random")
      .mockReturnValueOnce(0.25)
      .mockReturnValueOnce(0.5);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );

    const consumerId = module.createStableWebConsumerId("market-page");

    expect(consumerId).toMatch(/^web:market-page:base-old:window:/);
    expect(window.sessionStorage.getItem(storageKey)).toBe(
      "web:market-page:base-old",
    );
  });
});

describe("createConsoleDataMarketSubscriptionsController", () => {
  it("sends subscription acquire and release requests with instrument arrays", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          ok: true,
          data: {
            totalActiveSubscriptions: 0,
            quota: {
              totalUsed: 0,
              totalLimit: null,
              totalRemaining: null,
              byMarket: [],
            },
            entries: [],
          },
          timestamp: "2026-06-15T00:00:00Z",
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const controller = module.createConsoleDataMarketSubscriptionsController({
      marketDataSubscriptions: ref({
        totalActiveSubscriptions: 0,
        quota: {
          totalUsed: 0,
          totalLimit: null,
          totalRemaining: null,
          byMarket: [],
        },
        entries: [],
      }),
      marketInstrumentReferences: ref([]),
      marketDataQueryMarket: ref("hk"),
      marketDataQuerySymbol: ref("00700"),
      isLoadingMarketData: ref(false),
      marketDataError: ref(""),
    });

    await controller.acquireMarketDataSubscription({
      consumerId: "web:chart",
      channel: "KLINE",
      interval: "k_60m",
    });
    await controller.releaseMarketDataSubscription({
      consumerId: "web:chart",
      channel: "KLINE",
      interval: "k_60m",
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({
      consumerId: "web:chart",
      instruments: [
        {
          market: "HK",
          symbol: "00700",
          channel: "KLINE",
          interval: "1h",
        },
      ],
    });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body as string)).toEqual({
      consumerId: "web:chart",
      instruments: [
        {
          market: "HK",
          symbol: "00700",
          channel: "KLINE",
          interval: "1h",
        },
      ],
    });
  });

  it("loads subscriptions and merges instrument reference searches", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/instruments?")) {
        return successResponse({
          entries: [
            { instrumentId: "HK.00700", market: "HK", symbol: "00700", name: "Tencent" },
            { instrumentId: "HK.09988", market: "HK", symbol: "09988", name: "Alibaba" },
          ],
          total: 2,
        });
      }
      return successResponse(emptySubscriptions());
    });
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module);
    harness.marketInstrumentReferences.value = [
      { instrumentId: "HK.00700", market: "HK", symbol: "00700", name: "Old name" },
    ] as MarketInstrumentReference[];

    await harness.controller.loadMarketDataSubscriptions();
    const response = await harness.controller.loadMarketInstrumentReferences("  9988  ");

    expect(harness.isLoadingMarketData.value).toBe(false);
    expect(response.entries).toHaveLength(2);
    expect(harness.marketInstrumentReferences.value).toHaveLength(2);
    expect(harness.marketInstrumentReferences.value[0]?.name).toBe("Tencent");
    expect(fetchMock.mock.calls[1]?.[0]).toContain(
      "/api/v1/market-data/instruments?limit=50&market=HK&query=9988",
    );
  });

  it("validates acquire and release inputs before touching subscription quota", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module, { market: " ", symbol: " " });

    await harness.controller.acquireMarketDataSubscription({ consumerId: "web:chart" });
    expect(harness.marketDataError.value).toBe("申请实时订阅前请填写市场和标的。");
    await harness.controller.releaseMarketDataSubscription({ consumerId: "web:chart" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("uses snapshot defaults and keepalive for page-lifecycle release", async () => {
    const fetchMock = vi.fn(async () => successResponse(emptySubscriptions()));
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module, { market: "us", symbol: "aapl" });

    await harness.controller.subscribeCurrentMarketData();
    await harness.controller.releaseMarketDataSubscription({
      consumerId: "web:workspace",
      keepalive: true,
    });

    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      consumerId: "web:manual-market-data",
      instruments: [{ market: "US", symbol: "AAPL", channel: "SNAPSHOT" }],
    });
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({ keepalive: true });
  });

  it("heartbeats active consumers and ignores blank consumer identifiers", async () => {
    const fetchMock = vi.fn(async () => successResponse(emptySubscriptions()));
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module);

    await harness.controller.heartbeatMarketDataConsumer("   ");
    await harness.controller.heartbeatMarketDataConsumer("web:chart:1");

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      consumerId: "web:chart:1",
    });
  });

  it("unsubscribes all channels and surfaces server failures without leaving loading stuck", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(successResponse(emptySubscriptions()))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ok: false,
            error: { code: "SUBSCRIPTION_FAILED", message: "OpenD unavailable" },
            timestamp: "2026-06-15T00:00:00Z",
          }),
          { status: 503, headers: { "Content-Type": "application/json" } },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module);

    await harness.controller.unsubscribeAllMarketData();
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe("DELETE");
    await harness.controller.loadMarketDataSubscriptions();

    expect(harness.marketDataError.value).toBe("OpenD unavailable");
    expect(harness.isLoadingMarketData.value).toBe(false);
  });

  it("reports acquire, release, heartbeat, and unsubscribe failures", async () => {
    const failure = () =>
      new Response(
        JSON.stringify({
          ok: false,
          error: { code: "FAILED", message: "subscription backend failed" },
          timestamp: "2026-06-15T00:00:00Z",
        }),
        { status: 500, headers: { "Content-Type": "application/json" } },
      );
    const fetchMock = vi.fn(async () => failure());
    vi.stubGlobal("fetch", fetchMock);
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const harness = createControllerHarness(module);

    await harness.controller.acquireMarketDataSubscription({ consumerId: "web:chart" });
    expect(harness.marketDataError.value).toBe("subscription backend failed");
    await harness.controller.releaseMarketDataSubscription({ consumerId: "web:chart" });
    expect(harness.marketDataError.value).toBe("subscription backend failed");
    await harness.controller.heartbeatMarketDataConsumer("web:chart");
    expect(harness.marketDataError.value).toBe("subscription backend failed");
    await harness.controller.unsubscribeAllMarketData();
    expect(harness.marketDataError.value).toBe("subscription backend failed");
    expect(harness.isLoadingMarketData.value).toBe(false);
  });
});

function emptySubscriptions(): MarketDataSubscriptionsResponse {
  return {
    totalActiveSubscriptions: 0,
    quota: {
      totalUsed: 0,
      totalLimit: null,
      totalRemaining: null,
      byMarket: [],
    },
    entries: [],
  };
}

function successResponse(data: unknown): Response {
  return new Response(
    JSON.stringify({ ok: true, data, timestamp: "2026-06-15T00:00:00Z" }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

function createControllerHarness(
  module: typeof import("../src/composables/consoleDataMarketSubscriptions"),
  query: { market?: string; symbol?: string } = {},
) {
  const marketDataSubscriptions = ref(emptySubscriptions());
  const marketInstrumentReferences = ref<MarketInstrumentReference[]>([]);
  const marketDataQueryMarket = ref(query.market ?? "hk");
  const marketDataQuerySymbol = ref(query.symbol ?? "00700");
  const isLoadingMarketData = ref(false);
  const marketDataError = ref("");
  return {
    controller: module.createConsoleDataMarketSubscriptionsController({
      marketDataSubscriptions,
      marketInstrumentReferences,
      marketDataQueryMarket,
      marketDataQuerySymbol,
      isLoadingMarketData,
      marketDataError,
    }),
    marketDataSubscriptions,
    marketInstrumentReferences,
    isLoadingMarketData,
    marketDataError,
  };
}
