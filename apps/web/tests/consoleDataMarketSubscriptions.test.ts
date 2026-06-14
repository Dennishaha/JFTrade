// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

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
});
