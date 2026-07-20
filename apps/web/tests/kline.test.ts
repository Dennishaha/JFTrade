import { describe, expect, it } from "vitest";

import {
  KLINE_PERIODS,
  formatKlinePeriodLabel,
  overlayRealtimeTickCandle,
  normalizeKlinePeriod,
  normalizeKlineIndicators,
  resolveKlineCandleDisplayAt,
  resolveKlinePeriodDurationMs,
  resolveRealtimeBucketStart,
} from "../src/charting/kline";

describe("kline realtime bucket resolution", () => {
  it("displays intraday candles at the bucket end without changing the bucket key", () => {
    const at = "2026-05-20T10:00:00.000Z";
    const expectedByPeriod = new Map([
      ["tick", at],
      ["1m", "2026-05-20T10:01:00.000Z"],
      ["3m", "2026-05-20T10:03:00.000Z"],
      ["5m", "2026-05-20T10:05:00.000Z"],
      ["10m", "2026-05-20T10:10:00.000Z"],
      ["15m", "2026-05-20T10:15:00.000Z"],
      ["30m", "2026-05-20T10:30:00.000Z"],
      ["1h", "2026-05-20T11:00:00.000Z"],
      ["1d", at],
      ["1w", at],
      ["1mo", at],
    ]);

    for (const { value: period } of KLINE_PERIODS) {
      const candle = {
        period,
        at,
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      };

      expect(
        resolveKlineCandleDisplayAt(candle),
      ).toBe(expectedByPeriod.get(period));
      expect(candle.at).toBe(at);
    }
  });

  it("uses explicit realtime display time without changing the bucket key", () => {
    expect(
      resolveKlineCandleDisplayAt({
        period: "5m",
        at: "2026-05-20T10:05:00.000Z",
        displayAt: "2026-05-20T10:06:30.000Z",
        open: 100,
        high: 102,
        low: 99,
        close: 101,
        volume: 1500,
      }),
    ).toBe("2026-05-20T10:06:30.000Z");
  });

  it("updates the existing daily candle in the same unfinished bucket", () => {
    const candles = [
      {
        period: "1d",
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
        period: "1d",
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
        period: "1d",
        at: "2026-05-17T00:00:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("creates the current monthly bucket from a realtime snapshot", () => {
    const candles = [
      {
        period: "1mo",
        at: "2026-05-01T00:00:00.000Z",
        open: 100,
        high: 105,
        low: 98,
        close: 102,
        volume: 1200,
      },
    ];
    const snapshot = {
      price: 110,
      volume: 1600,
      at: "2026-06-20T12:30:00.000Z",
      observedAt: "2026-06-20T12:30:00.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "1mo")).toBe(
      "2026-06-01T00:00:00.000Z",
    );
    expect(overlayRealtimeTickCandle(candles, snapshot, "1mo").at(-1)).toEqual({
      period: "1mo",
      at: "2026-06-01T00:00:00.000Z",
      open: 102,
      high: 110,
      low: 102,
      close: 110,
      volume: 1600,
    });
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
        period: "5m",
        at: "2026-05-20T10:05:00.000Z",
        displayAt: "2026-05-20T10:10:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("keeps the 1m bucket key stable while showing the bucket end", () => {
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
      at: "2026-05-20T10:01:30.000Z",
      observedAt: "2026-05-20T10:01:30.000Z",
    };

    expect(resolveRealtimeBucketStart(candles, snapshot, "1m")).toBe(
      "2026-05-20T10:01:00.000Z",
    );
    expect(overlayRealtimeTickCandle(candles, snapshot, "1m")).toEqual([
      {
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
      {
        period: "1m",
        at: "2026-05-20T10:01:00.000Z",
        displayAt: "2026-05-20T10:02:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("supports 10m duration, preserves session metadata, and leaves unknown periods untouched", () => {
    expect(resolveKlinePeriodDurationMs("10m")).toBe(10 * 60_000);
    expect(resolveKlinePeriodDurationMs("3m")).toBe(3 * 60_000);

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
    const sessionSnapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-20T10:06:00.000Z",
      observedAt: "2026-05-20T10:06:00.000Z",
      session: "after_hours",
    };

    expect(overlayRealtimeTickCandle(candles, sessionSnapshot, "5m")).toEqual([
      candles[0],
      {
        period: "5m",
        at: "2026-05-20T10:05:00.000Z",
        displayAt: "2026-05-20T10:10:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
        session: "after_hours",
      },
    ]);

    expect(
      overlayRealtimeTickCandle(candles, sessionSnapshot, "unknown-period"),
    ).toEqual(candles);
  });

  it("ignores invalid realtime timestamps and stale earlier buckets", () => {
    const candles = [
      {
        at: "2026-05-20T10:08:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
    ];

    expect(
      resolveRealtimeBucketStart(
        candles,
        {
          price: 102,
          volume: 1500,
          at: "not-a-date",
          observedAt: "not-a-date",
        },
        "5m",
      ),
    ).toBeNull();

    expect(
      resolveRealtimeBucketStart(
        candles,
        {
          price: 102,
          volume: 1500,
          at: "2026-05-20T10:04:00.000Z",
          observedAt: "2026-05-20T10:04:00.000Z",
        },
        "5m",
      ),
    ).toBeNull();
  });

  it("keeps the normalized last historical candle when the realtime bucket starts at its display time", () => {
    const candles = [
      {
        period: "1m",
        at: "2026-05-20T10:54:00.000Z",
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
      at: "2026-05-20T10:55:30.000Z",
      observedAt: "2026-05-20T10:55:30.000Z",
    };

    expect(overlayRealtimeTickCandle(candles, snapshot, "1m")).toEqual([
      {
        period: "1m",
        at: "2026-05-20T10:54:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      },
      {
        period: "1m",
        at: "2026-05-20T10:55:00.000Z",
        displayAt: "2026-05-20T10:56:00.000Z",
        open: 100.5,
        high: 102,
        low: 100.5,
        close: 102,
        volume: 1500,
      },
    ]);
  });

  it("keeps MA and EMA indicators in canonical order", () => {
    expect(
      normalizeKlineIndicators(["ema20", "ma5", "volume", "unknown"]),
    ).toEqual(["volume", "ma5", "ema20"]);
    expect(normalizeKlineIndicators([])).toEqual(["volume"]);
  });

  it("normalizes period aliases, labels, and duration lookups", () => {
    expect(normalizeKlinePeriod(" K_60M ")).toBe("1h");
    expect(normalizeKlinePeriod("60min")).toBe("1h");
    expect(normalizeKlinePeriod("1W")).toBe("1w");
    expect(formatKlinePeriodLabel("2h")).toBe("2H");
    expect(resolveKlinePeriodDurationMs("1w")).toBe(7 * 24 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("1mo")).toBe(30 * 24 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("tick")).toBeNull();
    expect(resolveKlinePeriodDurationMs("unsupported")).toBeNull();
    expect(() => normalizeKlinePeriod("invalid")).toThrow(
      "不支持的 K 线周期：invalid",
    );
  });

  it("returns null for empty candle times, unsupported bucket displays, and unsupported realtime periods", () => {
    expect(
      resolveRealtimeBucketStart(
        [],
        {
          price: 101,
          volume: 10,
          at: "2026-05-20T10:00:00.000Z",
        },
        "2h",
      ),
    ).toBeNull();

    expect(
      resolveRealtimeBucketStart(
        [],
        {
          price: 101,
          volume: 10,
          at: "",
        },
        "5m",
      ),
    ).toBeNull();

    expect(
      resolveKlineCandleDisplayAt({
        period: "2h",
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 1200,
      }),
    ).toBe("2026-05-20T10:00:00.000Z");
  });

  it("supports every intraday bucket truncation path without reusing invalid history bars", () => {
    const snapshot = {
      price: 102,
      volume: 1500,
      at: "2026-05-20T10:31:45.000Z",
      observedAt: "2026-05-20T10:31:45.000Z",
    };
    expect(resolveRealtimeBucketStart([], snapshot, "3m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "10m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "15m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "30m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "1h")).toBe("2026-05-20T10:00:00.000Z");
    expect(
      resolveRealtimeBucketStart(
        [
          {
            at: "",
            open: 100,
            high: 101,
            low: 99,
            close: 100.5,
            volume: 1200,
          },
        ],
        snapshot,
        "5m",
      ),
    ).toBe("2026-05-20T10:30:00.000Z");
  });

  it("creates tick overlays with session metadata and ignores invalid snapshot times", () => {
    const candles = [
      {
        period: "tick" as const,
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 100,
        low: 100,
        close: 100,
        volume: 0,
      },
    ];

    expect(
      overlayRealtimeTickCandle(
        candles,
        {
          price: 101.2,
          volume: 1000,
          barVolume: 12,
          at: "2026-05-20T10:00:01.000Z",
          observedAt: "2026-05-20T10:00:01.000Z",
          session: "regular",
        },
        "tick",
      ),
    ).toEqual([
      candles[0],
      {
        period: "tick",
        at: "2026-05-20T10:00:01.000Z",
        open: 101.2,
        high: 101.2,
        low: 101.2,
        close: 101.2,
        volume: 12,
        session: "regular",
      },
    ]);

    expect(
      overlayRealtimeTickCandle(
        candles,
        {
          price: 101.2,
          volume: 1000,
          at: "not-a-time",
        },
        "tick",
      ),
    ).toEqual(candles);
    expect(overlayRealtimeTickCandle(candles, null, "1m")).toEqual(candles);
  });
});
