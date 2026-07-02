import { describe, expect, it } from "vitest";

import {
  mergeMarketDataCandles,
  upsertMarketDataRealtimeCandle,
  upsertMarketDataTickCandle,
} from "../src/composables/marketDataRealtimeCandles";
import type { MarketDataCandlesQueryResult } from "../src/composables/marketDataRealtime";

describe("marketDataRealtimeCandles", () => {
  it("uses the first snapshot unchanged when no current series exists", () => {
    const next = buildCandlesResult();

    expect(mergeMarketDataCandles(null, next)).toBe(next);
  });

  it("deduplicates matching candles and keeps them time sorted", () => {
    const current: MarketDataCandlesQueryResult = {
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
          high: 321,
          low: 319.8,
          close: 320.4,
          volume: 100,
          at: "2026-05-17T01:31:00.000Z",
        },
      ],
      totalReturned: 1,
      meta: {
        instrumentId: "HK.00700",
        source: "cache",
        resolvedAt: "2026-05-17T01:31:59.000Z",
        fromCache: true,
      },
    };

    const next: MarketDataCandlesQueryResult = {
      ...current,
      candles: [
        {
          period: "1m",
          open: 319.5,
          high: 320.2,
          low: 319.4,
          close: 320.1,
          volume: 80,
          at: "2026-05-17T01:30:00.000Z",
        },
        {
          period: "1m",
          open: 320,
          high: 321.2,
          low: 319.8,
          close: 320.9,
          volume: 120,
          at: "2026-05-17T01:31:00.000Z",
        },
      ],
      totalReturned: 2,
      meta: {
        instrumentId: "HK.00700",
        source: "realtime",
        resolvedAt: "2026-05-17T01:31:30.000Z",
        fromCache: false,
      },
    };

    expect(mergeMarketDataCandles(current, next)).toEqual({
      ...next,
      candles: next.candles,
      totalReturned: 2,
    });
  });

  it("creates a realtime bar with session metadata and normalized display time", () => {
    const result = upsertMarketDataRealtimeCandle({
      current: null,
      instrument: buildCandlesResult().request.instrument,
      period: "1m",
      limit: 200,
      source: "futu",
      resolvedAt: "2026-05-17T01:31:30.000Z",
      price: 321.2,
      currentBarVolume: null,
      bucketAt: "2026-05-17T01:31:00.000Z",
      open: 320,
      high: 321.5,
      low: 319.8,
      session: "REGULAR",
    });

    expect(result).toMatchObject({
      request: { period: "1m", limit: 200 },
      totalReturned: 1,
      candles: [
        {
          period: "1m",
          open: 320,
          high: 321.5,
          low: 319.8,
          close: 321.2,
          volume: 0,
          at: "2026-05-17T01:31:00.000Z",
          session: "REGULAR",
        },
      ],
      meta: { source: "futu", fromCache: false },
    });
    expect(result.candles[0]?.displayAt).toBeTruthy();
  });

  it("replaces the current realtime bucket while preserving historical bars", () => {
    const current = buildCandlesResult();
    const result = upsertMarketDataRealtimeCandle({
      current,
      instrument: current.request.instrument,
      period: "1m",
      limit: 3,
      source: "websocket",
      resolvedAt: "2026-05-17T01:31:45.000Z",
      price: 322,
      currentBarVolume: 180,
      bucketAt: current.candles[0]!.at,
      open: 320,
      high: 322.2,
      low: 319.8,
    });

    expect(result.candles).toHaveLength(1);
    expect(result.candles[0]).toMatchObject({ close: 322, volume: 180 });
    expect(result.meta).toMatchObject({
      instrumentId: "HK.00700",
      source: "websocket",
      resolvedAt: "2026-05-17T01:31:45.000Z",
      fromCache: false,
    });
  });

  it("creates and merges tick candles without inventing session or volume", () => {
    const instrument = buildCandlesResult().request.instrument;
    const first = upsertMarketDataTickCandle({
      current: null,
      instrument,
      limit: 2,
      source: "snapshot",
      resolvedAt: "2026-05-17T01:31:30.000Z",
      price: 320.5,
      observedAt: "2026-05-17T01:31:30.000Z",
      currentBarVolume: null,
    });
    const second = upsertMarketDataTickCandle({
      current: first,
      instrument,
      limit: 2,
      source: "websocket",
      resolvedAt: "2026-05-17T01:31:31.000Z",
      price: 320.8,
      observedAt: "2026-05-17T01:31:31.000Z",
      currentBarVolume: 20,
      session: "REGULAR",
    });

    expect(first.candles[0]).toEqual({
      period: "tick",
      open: 320.5,
      high: 320.5,
      low: 320.5,
      close: 320.5,
      volume: 0,
      at: "2026-05-17T01:31:30.000Z",
    });
    expect(second.candles).toHaveLength(2);
    expect(second.candles[1]).toMatchObject({
      close: 320.8,
      volume: 20,
      session: "REGULAR",
    });
    expect(second.meta.source).toBe("websocket");
  });
});

function buildCandlesResult(): MarketDataCandlesQueryResult {
  return {
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
        high: 321,
        low: 319.8,
        close: 320.4,
        volume: 100,
        at: "2026-05-17T01:31:00.000Z",
      },
    ],
    totalReturned: 1,
    meta: {
      instrumentId: "HK.00700",
      source: "cache",
      resolvedAt: "2026-05-17T01:31:59.000Z",
      fromCache: true,
    },
  };
}
