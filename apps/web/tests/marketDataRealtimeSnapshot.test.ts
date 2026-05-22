import { describe, expect, it } from "vitest";

import type { MarketDataRealtimeBarPriceState } from "../src/composables/marketDataRealtimeBarPriceState";
import type { MarketDataRealtimeBarVolumeState } from "../src/composables/marketDataRealtimeBarVolumeState";
import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
} from "../src/composables/marketDataRealtime";
import { mergeMarketDataSnapshot } from "../src/composables/marketDataRealtimeSnapshot";

describe("marketDataRealtimeSnapshot", () => {
  it("merges historical candle values and matching realtime bar state into the snapshot", () => {
    const current: MarketDataSnapshotQueryResult = {
      request: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:30:05.000Z",
        observedAt: "2026-05-17T01:30:05.000Z",
      },
      meta: {
        instrumentId: "HK.00700",
        source: "realtime",
        resolvedAt: "2026-05-17T01:30:05.000Z",
        fromCache: false,
      },
    };
    const context: { candles: MarketDataCandlesQueryResult; period: string } = {
      period: "1m",
      candles: {
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
            open: 320.5,
            high: 321,
            low: 320.2,
            close: 320.9,
            volume: 18000,
            at: "2026-05-17T01:30:00.000Z",
          },
        ],
        totalReturned: 1,
        meta: {
          instrumentId: "HK.00700",
          source: "cache",
          resolvedAt: "2026-05-17T01:30:00.000Z",
          fromCache: true,
        },
      },
    };
    const barPriceState: MarketDataRealtimeBarPriceState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      open: 320.5,
      high: 321.8,
      low: 320.1,
    };
    const barVolumeState: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1280000,
      baseBarVolume: 18000,
    };

    expect(
      mergeMarketDataSnapshot({
        current,
        context,
        barPriceState,
        barVolumeState,
      }),
    ).toEqual({
      ...current,
      snapshot: {
        ...current.snapshot,
        barOpen: 320.5,
        barHigh: 321.8,
        barLow: 320.1,
        barVolume: 20000,
      },
    });
  });

  it("returns the original snapshot result unchanged for tick period", () => {
    const current: MarketDataSnapshotQueryResult = {
      request: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 321.8,
        bid: 321.7,
        ask: 321.9,
        volume: 1282000,
        turnover: 411000000,
        at: "2026-05-17T01:30:05.000Z",
      },
      meta: {
        instrumentId: "HK.00700",
        source: "realtime",
        resolvedAt: "2026-05-17T01:30:05.000Z",
        fromCache: false,
      },
    };

    expect(
      mergeMarketDataSnapshot({
        current,
        context: {
          candles: null,
          period: "tick",
        },
        barPriceState: null,
        barVolumeState: null,
      }),
    ).toBe(current);
  });
});