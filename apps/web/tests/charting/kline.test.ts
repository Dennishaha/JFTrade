import { describe, expect, it } from "vitest";

import {
  KLINE_PERIODS,
  formatKlinePeriodLabel,
  normalizeChartType,
  overlayRealtimeTickCandle,
  normalizeKlinePeriod,
  normalizeKlineIndicators,
  resolveKlineBucketDisplayAt,
  resolveKlineCandleDisplayAt,
  resolveKlinePeriodDurationMs,
  resolveRealtimeBucketStart,
  toHeikinAshiCandles,
  transformKlineCandles,
} from "../../src/charting/kline";

describe("Kline chart types", () => {
  it("normalizes legacy chart types and derives immutable Heikin Ashi candles", () => {
    const candles = [
      {
        period: "1m",
        at: "2026-05-20T10:00:00.000Z",
        displayAt: "2026-05-20T10:01:00.000Z",
        open: 10,
        high: 14,
        low: 9,
        close: 13,
        volume: 100,
        session: "regular",
      },
      {
        period: "1m",
        at: "2026-05-20T10:01:00.000Z",
        displayAt: "2026-05-20T10:02:00.000Z",
        open: 13,
        high: 16,
        low: 12,
        close: 15,
        volume: 120,
        session: "regular",
      },
      {
        period: "1m",
        at: "2026-05-20T10:02:00.000Z",
        displayAt: "2026-05-20T10:03:00.000Z",
        open: 15,
        high: 18,
        low: 14,
        close: 16,
        volume: 140,
        session: "regular",
      },
    ];
    const source = candles.map((candle) => ({ ...candle }));

    const heikinAshi = toHeikinAshiCandles(candles);

    expect(heikinAshi).toEqual([
      expect.objectContaining({
        open: 11.5,
        high: 14,
        low: 9,
        close: 11.5,
        volume: 100,
        at: "2026-05-20T10:00:00.000Z",
        displayAt: "2026-05-20T10:01:00.000Z",
        session: "regular",
      }),
      expect.objectContaining({
        open: 11.5,
        high: 16,
        low: 11.5,
        close: 14,
        volume: 120,
        at: "2026-05-20T10:01:00.000Z",
      }),
      expect.objectContaining({
        open: 12.75,
        high: 18,
        low: 12.75,
        close: 15.75,
        volume: 140,
        at: "2026-05-20T10:02:00.000Z",
      }),
    ]);
    expect(candles).toEqual(source);
    expect(heikinAshi[0]).not.toBe(candles[0]);
    expect(transformKlineCandles(candles, "standard")[0]).not.toBe(candles[0]);
    expect(normalizeChartType("heikinashi")).toBe("heikinashi");
    expect(normalizeChartType(" HEIKINASHI ")).toBe("heikinashi");
    expect(normalizeChartType("legacy")).toBe("standard");
    expect(normalizeChartType(null)).toBe("standard");
  });

  it("recomputes the recursive HA open after older candles are prepended", () => {
    const initial = [
      {
        at: "2026-05-20T10:01:00.000Z",
        open: 13,
        high: 16,
        low: 12,
        close: 15,
        volume: 120,
      },
      {
        at: "2026-05-20T10:02:00.000Z",
        open: 15,
        high: 18,
        low: 14,
        close: 16,
        volume: 140,
      },
    ];
    const earlier = {
      at: "2026-05-20T10:00:00.000Z",
      open: 10,
      high: 14,
      low: 9,
      close: 13,
      volume: 100,
    };

    const beforePrepend = toHeikinAshiCandles(initial);
    const afterPrepend = toHeikinAshiCandles([earlier, ...initial]);

    expect(afterPrepend.slice(1).map((candle) => candle.open)).not.toEqual(
      beforePrepend.map((candle) => candle.open),
    );
    expect(afterPrepend.map((candle) => candle.at)).toEqual([
      earlier.at,
      ...initial.map((candle) => candle.at),
    ]);
  });

  it("continues a backtest HA series from its hidden warmup seed", () => {
    const candles = [{
      at: "2026-05-20T10:02:00.000Z",
      open: 15,
      high: 21,
      low: 13,
      close: 19,
      volume: 140,
    }];

    const transformed = transformKlineCandles(candles, "heikinashi", {
      open: 11,
      close: 14,
    });

    expect(transformed[0]).toMatchObject({
      open: 12.5,
      high: 21,
      low: 12.5,
      close: 17,
      volume: 140,
    });
    expect(transformKlineCandles(candles, "standard", { open: 11, close: 14 }))
      .toEqual(candles);
  });

  it("defensively handles invalid HA seeds, timestamps, and source candles", () => {
    const candle = (
      at: string,
      overrides: Partial<{
        open: number;
        high: number;
        low: number;
        close: number;
      }> = {},
    ) => ({
      at,
      open: 10,
      high: 14,
      low: 9,
      close: 13,
      volume: 100,
      ...overrides,
    });
    const validAt = "2026-05-20T10:00:00.000Z";

    expect(
      toHeikinAshiCandles([candle(validAt), candle("invalid-time")]).map(
        (item) => item.at,
      ),
    ).toEqual([validAt, "invalid-time"]);
    expect(
      toHeikinAshiCandles([candle("invalid-time"), candle(validAt)]).map(
        (item) => item.at,
      ),
    ).toEqual([validAt, "invalid-time"]);
    expect(
      toHeikinAshiCandles([
        candle("invalid-first"),
        candle("invalid-second"),
      ]).map((item) => item.at),
    ).toEqual(["invalid-first", "invalid-second"]);
    expect(
      toHeikinAshiCandles([
        candle(validAt),
        candle(validAt, { open: 11, close: 12 }),
      ]).map((item) => item.open),
    ).toEqual([11.5, 11.5]);

    const malformedCandle = candle(validAt, { open: Number.NaN });
    const derivedMalformed = toHeikinAshiCandles([malformedCandle]);
    expect(derivedMalformed[0]).toEqual(malformedCandle);
    expect(derivedMalformed[0]).not.toBe(malformedCandle);

    expect(
      toHeikinAshiCandles([candle(validAt)], {
        open: Number.POSITIVE_INFINITY,
        close: 10,
      })[0]?.open,
    ).toBe(11.5);
    expect(
      toHeikinAshiCandles([candle(validAt)], {
        open: 10,
        close: Number.POSITIVE_INFINITY,
      })[0]?.open,
    ).toBe(11.5);
  });
});

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
      barVolume: 1500,
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
      barVolume: 1500,
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
      barVolume: 1600,
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
      barVolume: 1500,
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
      barVolume: 1500,
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
      barVolume: 1500,
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

  it("sorts delayed tick overlays and finds a non-tail active bucket", () => {
    const ticks = [
      { period: "tick", at: "2026-05-20T10:00:02.000Z", open: 102, high: 102, low: 102, close: 102, volume: 1 },
      { period: "tick", at: "2026-05-20T10:00:04.000Z", open: 104, high: 104, low: 104, close: 104, volume: 1 },
    ];
    const delayed = overlayRealtimeTickCandle(ticks, {
      price: 101,
      at: "2026-05-20T10:00:01.000Z",
      observedAt: "2026-05-20T10:00:01.000Z",
    }, "tick");
    expect(delayed.map((candle) => candle.at)).toEqual([
      "2026-05-20T10:00:01.000Z",
      "2026-05-20T10:00:02.000Z",
      "2026-05-20T10:00:04.000Z",
    ]);

    const bars = [
      { period: "5m", at: "2026-05-20T10:00:00.000Z", open: 100, high: 101, low: 99, close: 100, volume: 10 },
      { period: "5m", at: "2026-05-20T10:05:00.000Z", open: 100, high: 102, low: 99, close: 101, volume: 20 },
      { period: "5m", at: "2026-05-20T10:10:00.000Z", open: 101, high: 103, low: 100, close: 102, volume: 30 },
    ];
    expect(overlayRealtimeTickCandle(bars, {
      price: 102.5,
      barVolume: 25,
      at: "2026-05-20T10:06:00.000Z",
      observedAt: "2026-05-20T10:06:00.000Z",
    }, "5m")[1]).toMatchObject({ at: "2026-05-20T10:05:00.000Z", close: 102.5, volume: 25 });

    expect(resolveKlineBucketDisplayAt("1m", "invalid")).toBeNull();
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
      barVolume: 1500,
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
    expect(normalizeKlinePeriod("K_120M")).toBe("2h");
    expect(normalizeKlinePeriod("180min")).toBe("3h");
    expect(normalizeKlinePeriod("240M")).toBe("4h");
    expect(resolveKlinePeriodDurationMs("2h")).toBe(2 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("3h")).toBe(3 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("4h")).toBe(4 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("1w")).toBe(7 * 24 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("1mo")).toBe(30 * 24 * 60 * 60_000);
    expect(resolveKlinePeriodDurationMs("tick")).toBeNull();
    expect(resolveKlinePeriodDurationMs("unsupported")).toBeNull();
    expect(() => normalizeKlinePeriod("invalid")).toThrow(
      "不支持的 K 线周期：invalid",
    );
  });

  it("supports higher-hour realtime periods and rejects invalid candle times", () => {
    expect(
      resolveRealtimeBucketStart(
        [],
        {
          price: 101,
          at: "2026-05-20T10:00:00.000Z",
        },
        "2h",
      ),
    ).toBe("2026-05-20T10:00:00.000Z");

    expect(
      resolveRealtimeBucketStart(
        [],
        {
          price: 101,
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
    ).toBe("2026-05-20T12:00:00.000Z");

    expect(
      resolveRealtimeBucketStart(
        [
          {
            period: "2h",
            at: "2026-05-20T13:30:00.000Z",
            open: 100,
            high: 101,
            low: 99,
            close: 100.5,
            volume: 1200,
          },
        ],
        {
          price: 101,
          at: "2026-05-20T14:15:00.000Z",
        },
        "2h",
      ),
    ).toBe("2026-05-20T13:30:00.000Z");
  });

  it("supports every intraday bucket truncation path without reusing invalid history bars", () => {
    const snapshot = {
      price: 102,
      at: "2026-05-20T10:31:45.000Z",
      observedAt: "2026-05-20T10:31:45.000Z",
    };
    expect(resolveRealtimeBucketStart([], snapshot, "3m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "10m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "15m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "30m")).toBe("2026-05-20T10:30:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "1h")).toBe("2026-05-20T10:00:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "2h")).toBe("2026-05-20T10:00:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "3h")).toBe("2026-05-20T09:00:00.000Z");
    expect(resolveRealtimeBucketStart([], snapshot, "4h")).toBe("2026-05-20T08:00:00.000Z");
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
          at: "not-a-time",
        },
        "tick",
      ),
    ).toEqual(candles);
    expect(overlayRealtimeTickCandle(candles, null, "1m")).toEqual(candles);
  });

  it("does not use receipt time for an AKShare snapshot without quote time", () => {
    const candles = [
      {
        period: "1m" as const,
        at: "2026-05-20T10:00:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 12,
      },
    ];

    expect(
      overlayRealtimeTickCandle(
        candles,
        {
          price: 102,
          at: "",
          observedAt: "2026-05-20T21:25:00.000Z",
          brokerId: "akshare",
        },
        "1m",
      ),
    ).toEqual(candles);
  });

  it("never treats legacy cumulative snapshot volume as per-bar volume", () => {
    const legacyCumulativeVolume = 999_999;
    const candles = [
      {
        period: "1m",
        at: "2026-05-20T10:00:00.000Z",
        displayAt: "2026-05-20T10:01:00.000Z",
        open: 100,
        high: 101,
        low: 99,
        close: 100.5,
        volume: 37,
      },
    ];
    const legacySnapshot = {
      price: 102,
      volume: legacyCumulativeVolume,
      at: "2026-05-20T10:00:30.000Z",
    };

    const existingBucket = overlayRealtimeTickCandle(
      candles,
      legacySnapshot,
      "1m",
    ).at(-1);
    expect(existingBucket?.volume).toBe(37);
    expect(existingBucket?.volume).not.toBe(legacyCumulativeVolume);

    const newBucket = overlayRealtimeTickCandle(
      candles,
      {
        ...legacySnapshot,
        at: "2026-05-20T10:01:30.000Z",
      },
      "1m",
    ).at(-1);
    expect(newBucket?.volume).toBeNull();
    expect(newBucket?.volume).not.toBe(legacyCumulativeVolume);

    const tick = overlayRealtimeTickCandle(
      [],
      {
        ...legacySnapshot,
        at: "2026-05-20T10:02:00.000Z",
      },
      "tick",
    ).at(-1);
    expect(tick?.volume).toBeNull();
    expect(tick?.volume).not.toBe(legacyCumulativeVolume);
  });

  it("reuses an already updated tail candle without scanning or sorting history", () => {
    let historicalAtReads = 0;
    const candles = [
      {
        period: "1m",
        get at() {
          historicalAtReads += 1;
          return "2026-07-03T11:59:00.000Z";
        },
        displayAt: "2026-07-03T12:00:00.000Z",
        open: 199,
        high: 200,
        low: 198,
        close: 199.5,
        volume: 100,
      },
      {
        period: "1m",
        at: "2026-07-03T12:00:00.000Z",
        displayAt: "2026-07-03T12:01:00.000Z",
        open: 200,
        high: 202,
        low: 199,
        close: 201.5,
        volume: 250,
        session: "regular",
      },
    ];

    const overlaid = overlayRealtimeTickCandle(
      candles,
      {
        price: 201.5,
        barVolume: 250,
        barOpen: 200,
        barHigh: 202,
        barLow: 199,
        at: "2026-07-03T12:00:30.000Z",
        observedAt: "2026-07-03T12:00:31.000Z",
        session: "regular",
      },
      "1m",
    );

    expect(overlaid).toBe(candles);
    expect(historicalAtReads).toBe(0);
  });
});
