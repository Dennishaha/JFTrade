import { describe, expect, it } from "vitest";

import { mergeMarketDataCandles } from "../src/composables/marketDataRealtimeCandles";
import type { MarketDataCandlesQueryResult } from "../src/composables/marketDataRealtime";

describe("marketDataRealtimeCandles", () => {
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
});