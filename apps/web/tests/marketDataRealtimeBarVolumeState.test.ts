import { describe, expect, it } from "vitest";

import {
  resolveMarketDataBarVolumeValue,
  resolveMarketDataBarVolumeUpdate,
  type MarketDataRealtimeBarVolumeState,
} from "../src/composables/marketDataRealtimeBarVolumeState";

describe("marketDataRealtimeBarVolumeState", () => {
  it("clears state when there is no active bucket", () => {
    const previousState: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1282000,
      baseBarVolume: 200,
    };

    expect(
      resolveMarketDataBarVolumeUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: null,
        cumulativeVolume: 1282200,
        existingCandleVolume: null,
        existingCandleUnfinalized: false,
      }),
    ).toEqual({
      currentBarVolume: null,
      nextState: null,
    });
  });

  it("hydrates from the current candle on first sample", () => {
    expect(
      resolveMarketDataBarVolumeUpdate({
        previousState: null,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        cumulativeVolume: 1282100,
        existingCandleVolume: 240,
        existingCandleUnfinalized: true,
      }),
    ).toEqual({
      currentBarVolume: 240,
      nextState: {
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        baselineCumulativeVolume: 1282100,
        baseBarVolume: 240,
      },
    });
  });

  it("adds incremental volume within the same bucket", () => {
    const previousState: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1282000,
      baseBarVolume: 0,
    };

    expect(
      resolveMarketDataBarVolumeUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        cumulativeVolume: 1282200,
        existingCandleVolume: null,
        existingCandleUnfinalized: false,
      }),
    ).toEqual({
      currentBarVolume: 200,
      nextState: previousState,
    });
  });

  it("rebases to the latest unfinalized candle volume", () => {
    const previousState: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1282000,
      baseBarVolume: 0,
    };

    expect(
      resolveMarketDataBarVolumeUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        cumulativeVolume: 1282200,
        existingCandleVolume: 240,
        existingCandleUnfinalized: true,
      }),
    ).toEqual({
      currentBarVolume: 240,
      nextState: {
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        baselineCumulativeVolume: 1282200,
        baseBarVolume: 240,
      },
    });
  });

  it("keeps the previous state when cumulative volume is invalid", () => {
    const previousState: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1282200,
      baseBarVolume: 240,
    };

    expect(
      resolveMarketDataBarVolumeUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        cumulativeVolume: Number.NaN,
        existingCandleVolume: 240,
        existingCandleUnfinalized: true,
      }),
    ).toEqual({
      currentBarVolume: 240,
      nextState: previousState,
    });
  });

  it("reuses the same bar volume calculation across update paths", () => {
    const state: MarketDataRealtimeBarVolumeState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      baselineCumulativeVolume: 1282200,
      baseBarVolume: 240,
    };

    expect(resolveMarketDataBarVolumeValue(state, 1282500, 240)).toBe(540);
  });
});