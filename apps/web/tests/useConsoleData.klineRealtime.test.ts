// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it } from "vitest";
import { defineComponent } from "vue";

import { overlayRealtimeTickCandle } from "../src/charting/kline";
import {
  provideConsoleDataStore,
} from "../src/composables/useConsoleData";
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";

function createConsoleStore() {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;

  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceLayoutStore();
      store = provideConsoleDataStore(workspaceLayout);
      return () => null;
    },
  });

  mount(Host, {
    global: {
      plugins: [createPinia()],
    },
  });

  if (store == null) {
    throw new Error("Failed to create console data store.");
  }

  return store;
}

afterEach(() => {
  window.localStorage?.clear();
});

describe("console data realtime kline overlay", () => {
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
        displayAt: "2026-05-17T01:30:05.000Z",
        open: 320.5,
        high: 321.8,
        low: 320.5,
        close: 321.8,
        volume: 0,
      },
    ]);
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
        displayAt: "2026-05-17T01:30:45.000Z",
        open: 320.5,
        high: 321.8,
        low: 319.7,
        close: 321.1,
        volume: 200,
      },
    ]);
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
      },
      {
        period: "1m",
        open: 321.1,
        high: 322.4,
        low: 321.1,
        close: 322.4,
        volume: 0,
        at: "2026-05-17T01:31:00.000Z",
        displayAt: "2026-05-17T01:31:30.000Z",
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
      },
      {
        period: "5m",
        open: 321.1,
        high: 322.4,
        low: 321.1,
        close: 322.4,
        volume: 0,
        at: "2026-05-17T01:35:00.000Z",
        displayAt: "2026-05-17T01:35:30.000Z",
      },
    ]);
  });
});