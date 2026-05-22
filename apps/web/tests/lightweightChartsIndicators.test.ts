import { describe, expect, it } from "vitest";

import type { KlineCandle } from "../src/charting/kline";
import {
  computeExponentialMovingAverage,
  computeKdj,
  computeMacd,
  computeSimpleMovingAverage,
} from "../src/charting/lightweightChartsIndicators";

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
});