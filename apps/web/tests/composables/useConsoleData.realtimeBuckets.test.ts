// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import { overlayRealtimeTickCandle } from "../../src/charting/kline";
import {
  provideConsoleDataStore,
} from "@/composables/workspace/useConsoleData";
import { provideWorkspaceLayoutStore } from "@/composables/workspace/useWorkspaceLayout";
import { createResponse, flushRequests } from "../helpers";

function createConsoleStore() {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;

  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceLayoutStore();
      store = provideConsoleDataStore(workspaceLayout);
      return () => null;
    },
  });

  mount(Host);

  if (store == null) {
    throw new Error("Failed to create console data store.");
  }

  return store;
}

afterEach(() => {
  vi.useRealTimers();
  window.localStorage?.clear();
  vi.unstubAllGlobals();
});

describe("console data realtime kline overlay", () => {
  it("does not restart REST polling when live ticks keep arriving", async () => {
    vi.useFakeTimers();

    const store = createConsoleStore();
    let snapshotCalls = 0;
    let securityDetailsCalls = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);

        if (url.includes("/api/v1/market-data/snapshots/US/AAPL")) {
          snapshotCalls += 1;
          return createResponse({
            request: {
              market: "US",
              symbol: "AAPL",
              instrumentId: "US.AAPL",
            },
            snapshot: {
              price: 213.4,
              bid: 213.3,
              ask: 213.5,
              openPrice: 212.1,
              highPrice: 214.2,
              lowPrice: 211.8,
              previousClosePrice: 212.9,
              lastClosePrice: 210.4,
              volume: 1450000,
              turnover: 309000000,
              at: "2026-05-21T13:29:58.000Z",
              observedAt: "2026-05-21T13:29:58.000Z",
              session: snapshotCalls === 1 ? "pre" : "regular",
              extendedHours: snapshotCalls === 1,
            },
            meta: {
              instrumentId: "US.AAPL",
              source: "bbgo:futu",
              resolvedAt: "2026-05-21T13:29:58.000Z",
              fromCache: false,
            },
          });
        }

        if (url.includes("/api/v1/market-data/securities/US/AAPL")) {
          securityDetailsCalls += 1;
          return createResponse({
            request: {
              market: "US",
              symbol: "AAPL",
              instrumentId: "US.AAPL",
            },
            security: {
              instrumentId: "US.AAPL",
              market: "US",
              symbol: "AAPL",
              securityId: 1001,
              name: "Apple Inc.",
              securityType: "Eqty",
              exchangeType: "US_NASDAQ",
              listTime: "1980-12-12",
              lotSize: 1,
              isSuspend: false,
              priceSpread: 0.01,
              updateTime: "2026-05-21 09:29:58",
              highPrice: 214.2,
              openPrice: 212.1,
              lowPrice: 211.8,
              lastClosePrice: 212.9,
              currentPrice: 213.4,
              volume: 1450000,
              turnover: 309000000,
              turnoverRate: 0.82,
            },
            meta: {
              instrumentId: "US.AAPL",
              source: "bbgo:futu",
              resolvedAt: "2026-05-21T13:29:58.000Z",
              fromCache: false,
            },
          });
        }

        if (url.includes("/api/v1/market-data/candles/US/AAPL")) {
          return createResponse({
            request: {
              instrument: {
                market: "US",
                symbol: "AAPL",
                instrumentId: "US.AAPL",
              },
              period: "1m",
              limit: 3,
            },
            candles: [],
            totalReturned: 0,
            meta: {
              instrumentId: "US.AAPL",
              source: "bbgo:futu",
              resolvedAt: "2026-05-21T13:29:58.000Z",
              fromCache: false,
            },
          });
        }

        throw new Error(`Unexpected request: ${url}`);
      }),
    );

    store.marketDataQueryMarket.value = "US";
    store.marketDataQuerySymbol.value = "AAPL";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataQueryLimit.value = 3;

    await store.loadMarketDataQuery();

    await vi.advanceTimersByTimeAsync(400);
    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-21T13:29:58.400Z",
      brokerId: "futu",
      cumulativeVolume: 1450200,
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      snapshot: {
        price: 213.5,
        bid: 213.4,
        ask: 213.6,
        openPrice: 212.1,
        highPrice: 214.2,
        lowPrice: 211.8,
        previousClosePrice: 212.9,
        lastClosePrice: 210.4,
        volume: 1450200,
        turnover: 309100000,
        at: "2026-05-21T13:29:58.400Z",
      },
      source: "futu",
    });

    await vi.advanceTimersByTimeAsync(400);
    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-21T13:29:58.800Z",
      brokerId: "futu",
      cumulativeVolume: 1450400,
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      snapshot: {
        price: 213.6,
        bid: 213.5,
        ask: 213.7,
        openPrice: 212.1,
        highPrice: 214.2,
        lowPrice: 211.8,
        previousClosePrice: 212.9,
        lastClosePrice: 210.4,
        volume: 1450400,
        turnover: 309200000,
        at: "2026-05-21T13:29:58.800Z",
      },
      source: "futu",
    });

    await vi.advanceTimersByTimeAsync(200);

    expect(snapshotCalls).toBe(1);
    expect(securityDetailsCalls).toBe(1);

    vi.useRealTimers();
  });

  it("uses websocket event time when snapshot.at is still on the previous minute", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "1m",
        limit: 3,
      },
      candles: [
        {
          period: "1m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:29:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "api-sample-cache",
        resolvedAt: "2026-05-17T01:29:59.000Z",
        fromCache: true,
      },
    };

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:05.000Z",
      brokerId: "futu",
      cumulativeVolume: 1282000,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:29:59.000Z",
      },
      source: "futu",
    });

    expect(store.marketDataSnapshot.value?.snapshot?.observedAt).toBe(
      "2026-05-17T01:30:05.000Z",
    );

    expect(
      overlayRealtimeTickCandle(
        store.marketDataCandles.value?.candles ?? [],
        store.marketDataSnapshot.value?.snapshot ?? null,
        store.marketDataQueryPeriod.value,
      ),
    ).toEqual([
      {
        period: "1m",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
        at: "2026-05-17T01:29:00.000Z",
      },
      {
        period: "1m",
        at: "2026-05-17T01:30:00.000Z",
        displayAt: "2026-05-17T01:31:00.000Z",
        open: 320.5,
        high: 321.8,
        low: 320.5,
        close: 321.8,
        volume: 0,
      },
    ]);
  });

  it("keeps reconnect recovery ticks on the current 1m bucket", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "1m",
        limit: 3,
      },
      candles: [
        {
          period: "1m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:29:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "api-sample-cache",
        resolvedAt: "2026-05-17T01:29:59.800Z",
        fromCache: true,
      },
    };

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:00.250Z",
      brokerId: "futu",
      cumulativeVolume: 1282000,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:29:59.800Z",
        observedAt: "2026-05-17T01:30:00.250Z",
      },
      source: "futu",
    });

    expect(
      store.marketDataCandles.value?.candles.find(
        (candle) => candle.at === "2026-05-17T01:29:00.000Z",
      )?.close,
    ).toBe(320.5);
    expect(
      store.marketDataCandles.value?.candles.find(
        (candle) => candle.at === "2026-05-17T01:30:00.000Z",
      ),
    ).toEqual({
      period: "1m",
      at: "2026-05-17T01:30:00.000Z",
      displayAt: "2026-05-17T01:31:00.000Z",
      open: 320.5,
      high: 321.8,
      low: 320.5,
      close: 321.8,
      volume: 0,
    });
  });

  it("keeps the current 1m overlay high and low across multiple ticks in the same bucket", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "1m",
        limit: 3,
      },
      candles: [
        {
          period: "1m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:29:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "api-sample-cache",
        resolvedAt: "2026-05-17T01:29:59.000Z",
        fromCache: true,
      },
    };

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:05.000Z",
      brokerId: "futu",
      cumulativeVolume: 1282000,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:30:05.000Z",
      },
      source: "futu",
    });

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:20.000Z",
      brokerId: "futu",
      cumulativeVolume: 1282100,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 319.7,
        bid: 319.6,
        ask: 319.8,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282100,
        turnover: 411020000,
        at: "2026-05-17T01:30:20.000Z",
      },
      source: "futu",
    });

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:45.000Z",
      brokerId: "futu",
      cumulativeVolume: 1282200,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.1,
        bid: 321,
        ask: 321.2,
        openPrice: 319.8,
        highPrice: 322,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282200,
        turnover: 411090000,
        at: "2026-05-17T01:30:45.000Z",
      },
      source: "futu",
    });

    expect(store.marketDataSnapshot.value?.snapshot?.barOpen).toBe(320.5);
    expect(store.marketDataSnapshot.value?.snapshot?.barHigh).toBe(321.8);
    expect(store.marketDataSnapshot.value?.snapshot?.barLow).toBe(319.7);

    expect(
      overlayRealtimeTickCandle(
        store.marketDataCandles.value?.candles ?? [],
        store.marketDataSnapshot.value?.snapshot ?? null,
        store.marketDataQueryPeriod.value,
      ),
    ).toEqual([
      {
        period: "1m",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
        at: "2026-05-17T01:29:00.000Z",
      },
      {
        period: "1m",
        at: "2026-05-17T01:30:00.000Z",
        displayAt: "2026-05-17T01:31:00.000Z",
        open: 320.5,
        high: 321.8,
        low: 319.7,
        close: 321.1,
        volume: 200,
      },
    ]);
  });

  it("reuses the current bucket returned by the API instead of seeding from the previous close", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "1m",
        limit: 3,
      },
      candles: [
        {
          period: "1m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:29:00.000Z",
        },
        {
          period: "1m",
          open: 321.2,
          high: 322.5,
          low: 320.9,
          close: 321.8,
          volume: 240,
          at: "2026-05-17T01:30:00.000Z",
        },
      ],
      totalReturned: 2,
      meta: {
        instrumentId: "HK.00700",
        source: "bbgo:futu",
        resolvedAt: "2026-05-17T01:30:10.000Z",
        fromCache: false,
      },
    };

    store.applyMarketDataTickEvent({
      type: "market-data.tick",
      at: "2026-05-17T01:30:20.000Z",
      brokerId: "futu",
      cumulativeVolume: 1282100,
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.4,
        bid: 321.3,
        ask: 321.5,
        openPrice: 319.8,
        highPrice: 322.6,
        lowPrice: 319.6,
        previousClosePrice: 318.9,
        volume: 1282100,
        turnover: 411020000,
        at: "2026-05-17T01:30:20.000Z",
      },
      source: "futu",
    });

    expect(store.marketDataSnapshot.value?.snapshot?.barOpen).toBe(321.2);
    expect(store.marketDataSnapshot.value?.snapshot?.barHigh).toBe(322.5);
    expect(store.marketDataSnapshot.value?.snapshot?.barLow).toBe(320.9);

    expect(
      store.marketDataCandles.value?.candles.find(
        (candle) => candle.at === "2026-05-17T01:30:00.000Z",
      ),
    ).toEqual({
      period: "1m",
      open: 321.2,
      high: 322.5,
      low: 320.9,
      close: 321.4,
      volume: 240,
      at: "2026-05-17T01:30:00.000Z",
      displayAt: "2026-05-17T01:31:00.000Z",
    });
  });

  it("hydrates the latest bar volume from the current candle on initial query load", async () => {
    const store = createConsoleStore();

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => {
        const url = String(input);

        if (url.includes("/api/v1/market-data/snapshots/HK/00700")) {
          return createResponse({
            request: {
              market: "HK",
              symbol: "00700",
              instrumentId: "HK.00700",
            },
            snapshot: {
              price: 321.4,
              bid: 321.3,
              ask: 321.5,
              openPrice: 319.8,
              highPrice: 322.6,
              lowPrice: 319.6,
              previousClosePrice: 318.9,
              volume: 1282100,
              turnover: 411020000,
              at: "2026-05-17T01:30:20.000Z",
            },
            meta: {
              instrumentId: "HK.00700",
              source: "bbgo:futu",
              resolvedAt: "2026-05-17T01:30:20.000Z",
              fromCache: false,
            },
          });
        }

        if (url.includes("/api/v1/market-data/candles/HK/00700")) {
          return createResponse({
            request: {
              instrument: {
                market: "HK",
                symbol: "00700",
                instrumentId: "HK.00700",
              },
              period: "1m",
              limit: 3,
            },
            candles: [
              {
                period: "1m",
                open: 320,
                high: 320.8,
                low: 319.9,
                close: 320.5,
                volume: 18000,
                at: "2026-05-17T01:29:00.000Z",
              },
              {
                period: "1m",
                open: 321.2,
                high: 322.5,
                low: 320.9,
                close: 321.8,
                volume: 240,
                at: "2026-05-17T01:30:00.000Z",
              },
            ],
            totalReturned: 2,
            meta: {
              instrumentId: "HK.00700",
              source: "bbgo:futu",
              resolvedAt: "2026-05-17T01:30:10.000Z",
              fromCache: false,
            },
          });
        }

        throw new Error(`Unexpected request: ${url}`);
      }),
    );

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataQueryLimit.value = 3;

    await store.loadMarketDataQuery();

    expect(store.marketDataSnapshot.value?.snapshot?.barVolume).toBe(240);

    expect(
      overlayRealtimeTickCandle(
        store.marketDataCandles.value?.candles ?? [],
        store.marketDataSnapshot.value?.snapshot ?? null,
        store.marketDataQueryPeriod.value,
      ).at(-1),
    ).toEqual({
      period: "1m",
      at: "2026-05-17T01:30:00.000Z",
      displayAt: "2026-05-17T01:31:00.000Z",
      open: 321.2,
      high: 322.5,
      low: 320.9,
      close: 321.4,
      volume: 240,
    });
  });

  it("splits realtime 1m candles when observed time moves into the next bucket", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "1m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "1m",
        limit: 3,
      },
      candles: [
        {
          period: "1m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:29:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "api-sample-cache",
        resolvedAt: "2026-05-17T01:29:59.000Z",
        fromCache: true,
      },
    };

    for (const tick of [
      { at: "2026-05-17T01:30:05.000Z", price: 321.8, volume: 1282000 },
      { at: "2026-05-17T01:30:45.000Z", price: 321.1, volume: 1282200 },
      { at: "2026-05-17T01:31:30.000Z", price: 322.4, volume: 1282600 },
    ]) {
      store.applyMarketDataTickEvent({
        type: "market-data.tick",
        at: tick.at,
        brokerId: "futu",
        cumulativeVolume: tick.volume,
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        snapshot: {
          price: tick.price,
          bid: tick.price - 0.1,
          ask: tick.price + 0.1,
          openPrice: 319.8,
          highPrice: 322.4,
          lowPrice: 319.6,
          previousClosePrice: 318.9,
          volume: tick.volume,
          turnover: 411000000,
          at: tick.at,
        },
        source: "futu",
      });
    }

    expect(
      overlayRealtimeTickCandle(
        store.marketDataCandles.value?.candles ?? [],
        store.marketDataSnapshot.value?.snapshot ?? null,
        store.marketDataQueryPeriod.value,
      ),
    ).toEqual([
      {
        period: "1m",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
        at: "2026-05-17T01:29:00.000Z",
      },
      {
        period: "1m",
        open: 320.5,
        high: 321.8,
        low: 320.5,
        close: 321.1,
        volume: 200,
        at: "2026-05-17T01:30:00.000Z",
        displayAt: "2026-05-17T01:31:00.000Z",
      },
      {
        period: "1m",
        open: 321.1,
        high: 322.4,
        low: 321.1,
        close: 322.4,
        volume: 400,
        at: "2026-05-17T01:31:00.000Z",
        displayAt: "2026-05-17T01:32:00.000Z",
      },
    ]);
  });

  it("uses realtime display time and bucket splitting outside 1m periods", () => {
    const store = createConsoleStore();

    store.marketDataQueryMarket.value = "HK";
    store.marketDataQuerySymbol.value = "00700";
    store.marketDataQueryPeriod.value = "5m";
    store.marketDataCandles.value = {
      request: {
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        period: "5m",
        limit: 3,
      },
      candles: [
        {
          period: "5m",
          open: 320,
          high: 320.8,
          low: 319.9,
          close: 320.5,
          volume: 18000,
          at: "2026-05-17T01:25:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "api-sample-cache",
        resolvedAt: "2026-05-17T01:29:59.000Z",
        fromCache: true,
      },
    };

    for (const tick of [
      { at: "2026-05-17T01:30:05.000Z", price: 321.8, volume: 1282000 },
      { at: "2026-05-17T01:34:45.000Z", price: 321.1, volume: 1282200 },
      { at: "2026-05-17T01:35:30.000Z", price: 322.4, volume: 1282600 },
    ]) {
      store.applyMarketDataTickEvent({
        type: "market-data.tick",
        at: tick.at,
        brokerId: "futu",
        cumulativeVolume: tick.volume,
        instrument: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        snapshot: {
          price: tick.price,
          bid: tick.price - 0.1,
          ask: tick.price + 0.1,
          openPrice: 319.8,
          highPrice: 322.4,
          lowPrice: 319.6,
          previousClosePrice: 318.9,
          volume: tick.volume,
          turnover: 411000000,
          at: tick.at,
        },
        source: "futu",
      });
    }

    expect(
      overlayRealtimeTickCandle(
        store.marketDataCandles.value?.candles ?? [],
        store.marketDataSnapshot.value?.snapshot ?? null,
        store.marketDataQueryPeriod.value,
      ),
    ).toEqual([
      {
        period: "5m",
        open: 320,
        high: 320.8,
        low: 319.9,
        close: 320.5,
        volume: 18000,
        at: "2026-05-17T01:25:00.000Z",
      },
      {
        period: "5m",
        open: 320.5,
        high: 321.8,
        low: 320.5,
        close: 321.1,
        volume: 200,
        at: "2026-05-17T01:30:00.000Z",
        displayAt: "2026-05-17T01:35:00.000Z",
      },
      {
        period: "5m",
        open: 321.1,
        high: 322.4,
        low: 321.1,
        close: 322.4,
        volume: 400,
        at: "2026-05-17T01:35:00.000Z",
        displayAt: "2026-05-17T01:40:00.000Z",
      },
    ]);
  });
});

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function createSnapshotPayload(market: string, symbol: string, price: number) {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      openPrice: price - 1,
      highPrice: price + 1,
      lowPrice: price - 1,
      previousClosePrice: price - 2,
      volume: 1000,
      turnover: price * 1000,
      at: "2026-05-22T01:30:00.000Z",
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-05-22T01:30:00.000Z",
      fromCache: false,
    },
  };
}

function createSecurityPayload(market: string, symbol: string, name: string) {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    security: {
      instrumentId,
      market,
      symbol,
      securityId: 1,
      name,
      securityType: "Eqty",
      exchangeType: `${market}_TEST`,
      listTime: "2024-01-01",
      lotSize: 1,
      isSuspend: false,
      priceSpread: 0.01,
      updateTime: "2026-05-22 09:30:00",
      currentPrice: 1,
      highPrice: 1,
      openPrice: 1,
      lowPrice: 1,
      lastClosePrice: 1,
      volume: 1000,
      turnover: 1000,
      turnoverRate: 1,
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-05-22T01:30:00.000Z",
      fromCache: false,
    },
  };
}

function createCandlesPayload(
  market: string,
  symbol: string,
  period: string,
  close: number,
) {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      instrument: {
        market,
        symbol,
        instrumentId,
      },
      period,
      limit: 3,
    },
    candles: [
      {
        period,
        open: close - 1,
        high: close + 1,
        low: close - 2,
        close,
        volume: 100,
        at: "2026-05-22T01:30:00.000Z",
      },
    ],
    totalReturned: 1,
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-05-22T01:30:00.000Z",
      fromCache: false,
    },
  };
}
