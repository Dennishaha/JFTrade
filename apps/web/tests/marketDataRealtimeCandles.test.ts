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
        resolvedAt: "2026-05-17T01:32:00.000Z",
        fromCache: false,
      },
    };

    expect(mergeMarketDataCandles(current, next)).toEqual({
      ...next,
      candles: next.candles,
      totalReturned: 2,
    });
  });

  it("does not let a late historical response roll back a realtime bucket", () => {
    const current = buildCandlesResult();
    current.candles[0] = {
      ...current.candles[0]!,
      high: 322,
      close: 321.8,
      volume: 180,
    };
    current.meta = {
      ...current.meta,
      source: "websocket",
      resolvedAt: "2026-05-17T01:31:45.000Z",
      fromCache: false,
    };
    const staleHistory: MarketDataCandlesQueryResult = {
      ...buildCandlesResult(),
      candles: [
        {
          ...buildCandlesResult().candles[0]!,
          high: 321.2,
          low: 319.5,
          close: 320.9,
          volume: 120,
        },
      ],
      meta: {
        ...buildCandlesResult().meta,
        source: "historical",
        resolvedAt: "2026-05-17T01:31:30.000Z",
        fromCache: false,
      },
    };

    const merged = mergeMarketDataCandles(current, staleHistory);

    expect(merged.candles).toEqual([
      {
        period: "1m",
        open: 320,
        high: 322,
        low: 319.5,
        close: 321.8,
        volume: 180,
        at: "2026-05-17T01:31:00.000Z",
      },
    ]);
    expect(merged.meta).toMatchObject({
      source: "websocket",
      resolvedAt: "2026-05-17T01:31:45.000Z",
      fromCache: false,
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
    const ownedCandles = current.candles;
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
    expect(result.candles).toBe(ownedCandles);
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

  it("keeps a bounded realtime tick window without rebuilding its array", () => {
    const instrument = buildCandlesResult().request.instrument;
    let result = upsertMarketDataTickCandle({
      current: null,
      instrument,
      limit: 20,
      source: "websocket",
      resolvedAt: "2026-05-17T01:30:00.000Z",
      price: 320,
      observedAt: "2026-05-17T01:30:00.000Z",
      currentBarVolume: 1,
    });
    const ownedCandles = result.candles;

    for (let index = 1; index < 200; index += 1) {
      const at = new Date(
        Date.parse("2026-05-17T01:30:00.000Z") + index * 1000,
      ).toISOString();
      result = upsertMarketDataTickCandle({
        current: result,
        instrument,
        limit: 20,
        source: "websocket",
        resolvedAt: at,
        price: 320 + index / 100,
        observedAt: at,
        currentBarVolume: 1,
      });
    }

    expect(result.candles).toBe(ownedCandles);
    expect(result.candles.length).toBeLessThanOrEqual(21);
    expect(result.totalReturned).toBe(result.candles.length);
    expect(result.candles.at(-1)?.at).toBe("2026-05-17T01:33:19.000Z");
  });

  it("keeps snapshots isolated by series and tolerates invalid resolution clocks", () => {
    const current = buildCandlesResult();
    const otherPeriod = {
      ...buildCandlesResult(),
      request: { ...buildCandlesResult().request, period: "5m" },
    };
    expect(mergeMarketDataCandles(current, otherPeriod)).toBe(otherPeriod);

    current.meta.resolvedAt = "invalid-current-clock";
    const next = buildCandlesResult();
    next.meta.resolvedAt = "invalid-next-clock";
    next.candles[0] = { ...next.candles[0]!, close: 321.5 };
    expect(mergeMarketDataCandles(current, next).candles[0]?.close).toBe(321.5);
  });

  it("inserts and replaces delayed realtime ticks in chronological order", () => {
    const instrument = buildCandlesResult().request.instrument;
    let result = upsertMarketDataTickCandle({
      current: null,
      instrument,
      limit: 10,
      source: "websocket",
      resolvedAt: "2026-05-17T01:30:00.000Z",
      price: 320,
      observedAt: "2026-05-17T01:30:00.000Z",
      currentBarVolume: 1,
    });
    for (const [seconds, price] of [[2, 322], [4, 324]] as const) {
      const at = `2026-05-17T01:30:0${seconds}.000Z`;
      result = upsertMarketDataTickCandle({
        current: result,
        instrument,
        limit: 10,
        source: "websocket",
        resolvedAt: at,
        price,
        observedAt: at,
        currentBarVolume: 1,
      });
    }

    result = upsertMarketDataTickCandle({
      current: result,
      instrument,
      limit: 10,
      source: "websocket",
      resolvedAt: "2026-05-17T01:30:03.000Z",
      price: 323,
      observedAt: "2026-05-17T01:30:03.000Z",
      currentBarVolume: 2,
    });
    result = upsertMarketDataTickCandle({
      current: result,
      instrument,
      limit: 10,
      source: "websocket",
      resolvedAt: "2026-05-17T01:30:02.000Z",
      price: 325,
      observedAt: "2026-05-17T01:30:02.000Z",
      currentBarVolume: 3,
    });

    expect(result.candles.map((candle) => candle.at)).toEqual([
      "2026-05-17T01:30:00.000Z",
      "2026-05-17T01:30:02.000Z",
      "2026-05-17T01:30:03.000Z",
      "2026-05-17T01:30:04.000Z",
    ]);
    expect(result.candles[1]).toMatchObject({ close: 325, volume: 3 });
  });

  it("orders malformed timestamps deterministically without dropping ticks", () => {
    const instrument = buildCandlesResult().request.instrument;
    const current = buildCandlesResult();
    current.request.period = "tick";
    current.candles = [
      {
        period: "tick",
        at: "z-clock",
        open: 1,
        high: 1,
        low: 1,
        close: 1,
        volume: 1,
      },
    ];
    const result = upsertMarketDataTickCandle({
      current,
      instrument,
      limit: 3,
      source: "websocket",
      resolvedAt: "a-clock",
      price: 2,
      observedAt: "a-clock",
      currentBarVolume: 1,
    });
    expect(result.candles.map((candle) => candle.at)).toEqual([
      "a-clock",
      "z-clock",
    ]);

    current.candles.push({
      period: "1m",
      at: "z-clock",
      open: 3,
      high: 3,
      low: 3,
      close: 3,
      volume: 1,
    });
    expect(
      upsertMarketDataTickCandle({
        current,
        instrument,
        limit: 3,
        source: "websocket",
        resolvedAt: "z-clock",
        price: 4,
        observedAt: "z-clock",
        currentBarVolume: 1,
      }).candles.at(-1)?.period,
    ).toBe("tick");
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
