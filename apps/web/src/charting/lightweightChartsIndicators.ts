import type { UTCTimestamp } from "lightweight-charts";

import type { KlineCandle } from "./kline";

type IndicatorPoint = { time: UTCTimestamp; value: number };

function toIndicatorTimestamp(at: string): UTCTimestamp {
  return Math.floor(new Date(at).getTime() / 1000) as UTCTimestamp;
}

export function computeExponentialMovingAverage(
  values: readonly number[],
  period: number,
): Array<number | null> {
  const multiplier = 2 / (period + 1);
  let previous: number | null = null;

  return values.map((value) => {
    previous = previous == null ? value : previous + (value - previous) * multiplier;
    return previous;
  });
}

export function computeSimpleMovingAverage(
  values: readonly number[],
  period: number,
): Array<number | null> {
  const result = new Array<number | null>(values.length).fill(null);
  let rollingSum = 0;

  for (let index = 0; index < values.length; index += 1) {
    const currentValue = values[index]!;
    rollingSum += currentValue;
    if (index >= period) {
      const trailingValue = values[index - period]!;
      rollingSum -= trailingValue;
    }

    if (index + 1 >= period) {
      result[index] = rollingSum / period;
    }
  }

  return result;
}

export function computeMacd(candles: readonly KlineCandle[]): {
  diff: IndicatorPoint[];
  dea: IndicatorPoint[];
  histogram: IndicatorPoint[];
} {
  const closes = candles.map((candle) => candle.close);
  const ema12 = computeExponentialMovingAverage(closes, 12);
  const ema26 = computeExponentialMovingAverage(closes, 26);
  const diff = closes.map((_, index) => {
    const fast = ema12[index];
    const slow = ema26[index];
    return fast == null || slow == null ? null : fast - slow;
  });
  const dea = computeExponentialMovingAverage(
    diff.map((value) => value ?? 0),
    9,
  );

  return candles.reduce(
    (result, candle, index) => {
      const timestamp = toIndicatorTimestamp(candle.at);
      const diffValue = diff[index];
      const deaValue = dea[index];
      if (diffValue == null || deaValue == null) {
        return result;
      }

      const histogramValue = (diffValue - deaValue) * 2;
      result.diff.push({ time: timestamp, value: diffValue });
      result.dea.push({ time: timestamp, value: deaValue });
      result.histogram.push({ time: timestamp, value: histogramValue });
      return result;
    },
    {
      diff: [] as IndicatorPoint[],
      dea: [] as IndicatorPoint[],
      histogram: [] as IndicatorPoint[],
    },
  );
}

export function computeKdj(candles: readonly KlineCandle[]): {
  k: IndicatorPoint[];
  d: IndicatorPoint[];
  j: IndicatorPoint[];
} {
  let previousK = 50;
  let previousD = 50;

  return candles.reduce(
    (result, candle, index) => {
      const window = candles.slice(Math.max(0, index - 8), index + 1);
      const highestHigh = Math.max(...window.map((item) => item.high));
      const lowestLow = Math.min(...window.map((item) => item.low));
      const rsv =
        highestHigh === lowestLow
          ? 50
          : ((candle.close - lowestLow) / (highestHigh - lowestLow)) * 100;
      const nextK = (2 * previousK + rsv) / 3;
      const nextD = (2 * previousD + nextK) / 3;
      const nextJ = 3 * nextK - 2 * nextD;
      const timestamp = toIndicatorTimestamp(candle.at);

      result.k.push({ time: timestamp, value: nextK });
      result.d.push({ time: timestamp, value: nextD });
      result.j.push({ time: timestamp, value: nextJ });

      previousK = nextK;
      previousD = nextD;
      return result;
    },
    {
      k: [] as IndicatorPoint[],
      d: [] as IndicatorPoint[],
      j: [] as IndicatorPoint[],
    },
  );
}