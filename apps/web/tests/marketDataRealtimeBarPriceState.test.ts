import { describe, expect, it } from "vitest";

import {
  resolveMarketDataBarPriceUpdate,
  type MarketDataRealtimeBarPriceState,
} from "../src/composables/marketDataRealtimeBarPriceState";

describe("marketDataRealtimeBarPriceState", () => {
  it("resets to null when there is no active bucket", () => {
    const previousState: MarketDataRealtimeBarPriceState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      open: 320.5,
      high: 321.8,
      low: 319.7,
    };

    expect(
      resolveMarketDataBarPriceUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: null,
        price: 321.1,
        existingCandle: null,
        lastHistoricalClose: 320.5,
      }),
    ).toEqual({
      nextState: null,
      shouldFinalizePreviousBucket: false,
    });
  });

  it("seeds a new bucket from the last historical close", () => {
    expect(
      resolveMarketDataBarPriceUpdate({
        previousState: null,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        price: 321.8,
        existingCandle: null,
        lastHistoricalClose: 320.5,
      }),
    ).toEqual({
      nextState: {
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        open: 320.5,
        high: 321.8,
        low: 320.5,
      },
      shouldFinalizePreviousBucket: false,
    });
  });

  it("reuses the API current bucket as the price seed", () => {
    expect(
      resolveMarketDataBarPriceUpdate({
        previousState: null,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        price: 321.4,
        existingCandle: {
          open: 321.2,
          high: 322.5,
          low: 320.9,
        },
        lastHistoricalClose: 320.5,
      }),
    ).toEqual({
      nextState: {
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        open: 321.2,
        high: 322.5,
        low: 320.9,
      },
      shouldFinalizePreviousBucket: false,
    });
  });

  it("updates high and low within the same bucket", () => {
    const previousState: MarketDataRealtimeBarPriceState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      open: 320.5,
      high: 321.8,
      low: 320.5,
    };

    expect(
      resolveMarketDataBarPriceUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:30:00.000Z",
        price: 319.7,
        existingCandle: null,
        lastHistoricalClose: 320.5,
      }),
    ).toEqual({
      nextState: {
        ...previousState,
        high: 321.8,
        low: 319.7,
      },
      shouldFinalizePreviousBucket: false,
    });
  });

  it("signals finalize when the observed bucket moves forward", () => {
    const previousState: MarketDataRealtimeBarPriceState = {
      instrumentId: "HK.00700",
      period: "1m",
      bucketAt: "2026-05-17T01:30:00.000Z",
      open: 320.5,
      high: 321.8,
      low: 320.5,
    };

    expect(
      resolveMarketDataBarPriceUpdate({
        previousState,
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:31:00.000Z",
        price: 322.4,
        existingCandle: null,
        lastHistoricalClose: 321.1,
      }),
    ).toEqual({
      nextState: {
        instrumentId: "HK.00700",
        period: "1m",
        bucketAt: "2026-05-17T01:31:00.000Z",
        open: 321.1,
        high: 322.4,
        low: 321.1,
      },
      shouldFinalizePreviousBucket: true,
    });
  });
});