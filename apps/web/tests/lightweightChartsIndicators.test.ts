import { describe, expect, it } from "vitest";

import type { KlineCandle } from "../src/charting/kline";
import {
  computeAtr,
  computeCci,
  computeExponentialMovingAverage,
  computeKdj,
  computeMacd,
  computeSimpleMovingAverage,
  computeWilliamsR,
} from "../src/charting/lightweightChartsIndicators";

function expectIndicatorValues(
  actual: Array<{ value: number }>,
  expected: number[],
): void {
  expect(actual).toHaveLength(expected.length);
  actual.forEach((point, index) => {
    expect(point.value).toBeCloseTo(expected[index]!, 10);
  });
}

function buildCandle(overrides: Partial<KlineCandle>): KlineCandle {
  return {
    at: "2026-05-17T01:30:00.000Z",
    open: 100,
    high: 101,
    low: 99,
    close: 100,
    volume: 1000,
    ...overrides,
  };
}

describe("lightweightChartsIndicators", () => {
  it("computes simple moving averages with null warmup slots", () => {
    expect(computeSimpleMovingAverage([1, 2, 3, 4], 2)).toEqual([
      null,
      1.5,
      2.5,
      3.5,
    ]);
  });

  it("computes exponential moving averages incrementally", () => {
    const values = computeExponentialMovingAverage([1, 2, 3], 2);
    expect(values[0]).toBe(1);
    expect(values[1]).toBeCloseTo(1.6666667, 6);
    expect(values[2]).toBeCloseTo(2.5555556, 6);
  });

    it("computes KDJ from rolling highs and lows without changing values", () => {
      const candles: KlineCandle[] = [
        { at: "2026-05-20T09:30:00.000Z", open: 10, high: 11, low: 9, close: 10, volume: 100 },
        { at: "2026-05-20T09:31:00.000Z", open: 10, high: 13, low: 10, close: 12, volume: 120 },
        { at: "2026-05-20T09:32:00.000Z", open: 12, high: 12, low: 10, close: 11, volume: 110 },
        { at: "2026-05-20T09:33:00.000Z", open: 11, high: 14, low: 11, close: 13, volume: 130 },
      ];

      const result = computeKdj(candles);
      expectIndicatorValues(result.k, [50, 58.333333333333336, 55.555555555555564, 63.703703703703716]);
      expectIndicatorValues(result.d, [50, 52.77777777777778, 53.70370370370371, 57.037037037037045]);
      expectIndicatorValues(result.j, [50, 69.44444444444446, 59.25925925925927, 77.03703703703706]);
    });

  it("computes macd series for rising closes", () => {
    const candles = [
      buildCandle({ at: "2026-05-17T01:30:00.000Z", close: 100 }),
      buildCandle({ at: "2026-05-17T01:31:00.000Z", close: 102 }),
      buildCandle({ at: "2026-05-17T01:32:00.000Z", close: 104 }),
    ];

    const macd = computeMacd(candles);
    expect(macd.diff).toHaveLength(3);
    expect(macd.dea).toHaveLength(3);
    expect(macd.histogram).toHaveLength(3);
    expect(macd.histogram[0]?.value).toBe(0);
    expect(macd.histogram[2]?.value ?? 0).toBeGreaterThan(0);
  });

  it("computes ATR with a rolling true-range sum", () => {
    const candles: KlineCandle[] = [
      { at: "2026-05-20T09:30:00.000Z", open: 9, high: 10, low: 8, close: 9, volume: 100 },
      { at: "2026-05-20T09:31:00.000Z", open: 11, high: 13, low: 10, close: 12, volume: 120 },
      { at: "2026-05-20T09:32:00.000Z", open: 13, high: 15, low: 11, close: 13, volume: 110 },
      { at: "2026-05-20T09:33:00.000Z", open: 13, high: 14, low: 12, close: 13, volume: 130 },
    ];

    expectIndicatorValues(computeAtr(candles, 2), [3, 4, 3]);
  });

  it("keeps flat candles at neutral kdj values", () => {
    const candles = [
      buildCandle({ at: "2026-05-17T01:30:00.000Z", open: 100, high: 100, low: 100, close: 100 }),
      buildCandle({ at: "2026-05-17T01:31:00.000Z", open: 100, high: 100, low: 100, close: 100 }),
      buildCandle({ at: "2026-05-17T01:32:00.000Z", open: 100, high: 100, low: 100, close: 100 }),
    ];

    const kdj = computeKdj(candles);
    expect(kdj.k.map((point) => point.value)).toEqual([50, 50, 50]);
    expect(kdj.d.map((point) => point.value)).toEqual([50, 50, 50]);
    expect(kdj.j.map((point) => point.value)).toEqual([50, 50, 50]);
  });

  it("computes atr after the warmup window", () => {
    const candles = [
      buildCandle({ at: "2026-05-17T01:30:00.000Z", high: 105, low: 100, close: 103 }),
      buildCandle({ at: "2026-05-17T01:31:00.000Z", high: 108, low: 102, close: 107 }),
      buildCandle({ at: "2026-05-17T01:32:00.000Z", high: 110, low: 104, close: 109 }),
    ];

    const atr = computeAtr(candles, 3);
    expect(atr).toHaveLength(1);
    expect(atr[0]?.value).toBeCloseTo(5.6666667, 6);
  });

  it("computes cci for a trending window", () => {
    const candles = [
      buildCandle({ at: "2026-05-17T01:30:00.000Z", high: 105, low: 99, close: 104 }),
      buildCandle({ at: "2026-05-17T01:31:00.000Z", high: 108, low: 102, close: 107 }),
      buildCandle({ at: "2026-05-17T01:32:00.000Z", high: 112, low: 106, close: 111 }),
    ];

    const cci = computeCci(candles, 3);
    expect(cci).toHaveLength(1);
    expect(cci[0]?.value).toBeCloseTo(100, 10);
  });

  it("computes williams r inside the expected range", () => {
    const candles = [
      buildCandle({ at: "2026-05-17T01:30:00.000Z", high: 105, low: 99, close: 104 }),
      buildCandle({ at: "2026-05-17T01:31:00.000Z", high: 108, low: 102, close: 107 }),
      buildCandle({ at: "2026-05-17T01:32:00.000Z", high: 110, low: 104, close: 105 }),
    ];

    const williamsR = computeWilliamsR(candles, 3);
    expect(williamsR).toHaveLength(1);
    expect(williamsR[0]?.value).toBeCloseTo(-45.454545, 5);
  });

  it("computes Williams %R from rolling extrema", () => {
    const candles: KlineCandle[] = [
      { at: "2026-05-20T09:30:00.000Z", open: 10, high: 11, low: 9, close: 10, volume: 100 },
      { at: "2026-05-20T09:31:00.000Z", open: 11, high: 12, low: 10, close: 11, volume: 120 },
      { at: "2026-05-20T09:32:00.000Z", open: 12, high: 13, low: 11, close: 12, volume: 110 },
      { at: "2026-05-20T09:33:00.000Z", open: 13, high: 14, low: 12, close: 13, volume: 130 },
    ];

    expectIndicatorValues(computeWilliamsR(candles, 3), [-25, -25]);
  });

  it("expires extrema and typical prices as indicator windows advance", () => {
    const kdjCandles = Array.from({ length: 10 }, (_, index) =>
      buildCandle({
        at: `2026-05-17T01:${String(30 + index).padStart(2, "0")}:00.000Z`,
        open: 100 - index,
        high: 110 - index,
        low: 90 + index,
        close: index === 9 ? 101 : 100,
      }),
    );
    const kdj = computeKdj(kdjCandles);
    expect(kdj.k).toHaveLength(10);
    expect(kdj.k[9]?.value).toBeGreaterThan(50);

    const cci = computeCci(
      [100, 101, 103, 106].map((close, index) =>
        buildCandle({
          at: `2026-05-17T02:${String(index).padStart(2, "0")}:00.000Z`,
          high: close + 1,
          low: close - 1,
          close,
        }),
      ),
      3,
    );
    expect(cci).toHaveLength(2);
    expect(cci[1]?.value).toBeGreaterThan(0);
  });
});
