import { describe, expect, it } from "vitest";

import {
  overlayRealtimeTickCandle,
  resolveRealtimeBucketStart,
} from "../src/charting/kline";

describe("kline realtime bucket resolution", () => {
  it("updates the existing daily candle in the same unfinished bucket", () => {
    const candles = [
      {
        at: "2026-05-17T16:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
    ];
    const snapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-17T20:31:00.000Z",
      observedAt: "2026-05-17T20:31:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "1d")).toBe(
      "2026-05-17T16:00:00.000Z",
    );
    expect(overlayRealtimeTickCandle(candles, snapshot, "1d")).toEqual([
      {
        at: "2026-05-17T16:00:00.000Z",
        open: 100,
        high: 102,
        low: 99,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("moves daily snapshots onto the current day instead of reusing the previous candle", () => {
    const candles = [
      {
        at: "2026-05-16T16:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
    ];
    const snapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-17T09:31:00.000Z",
      observedAt: "2026-05-17T09:31:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "1d")).toBe(
      "2026-05-17T00:00:00.000Z",
    );
    expect(overlayRealtimeTickCandle(candles, snapshot, "1d")).toEqual([
      {
        at: "2026-05-16T16:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
      {
        at: "2026-05-17T00:00:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("keeps weekly snapshots on the current week boundary", () => {
    const candles = [
      {
        at: "2026-05-12T16:00:00.000Z",
        open: 100,
        high: 103,
        low: 99,
        close: 102,
        volume: 5600,
      },
    ];
    const snapshot = {
      price: 104,
      volume: 5800,
      at: "2026-05-18T09:31:00.000Z",
      observedAt: "2026-05-18T09:31:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "1w")).toBe(
      "2026-05-18T00:00:00.000Z",
    );
  });

  it("does not append a realtime intraday candle across a stale history gap", () => {
    const candles = [
      {
        at: "2026-05-15T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
    ];
    const snapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-20T10:06:00.000Z",
      observedAt: "2026-05-20T10:06:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "5m")).toBeNull();
    expect(overlayRealtimeTickCandle(candles, snapshot, "5m")).toEqual(candles);
  });

  it("adds the next intraday realtime bucket when history is fresh", () => {
    const candles = [
      {
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
    ];
    const snapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-20T10:06:00.000Z",
      observedAt: "2026-05-20T10:06:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "5m")).toBe(
      "2026-05-20T10:05:00.000Z",
    );
    expect(overlayRealtimeTickCandle(candles, snapshot, "5m")).toEqual([
      {
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
      {
        at: "2026-05-20T10:05:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });
});