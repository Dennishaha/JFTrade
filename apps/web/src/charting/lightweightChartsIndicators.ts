import type { UTCTimestamp } from "lightweight-charts";

import type { KlineCandle } from "./kline";

type IndicatorPoint = { time: UTCTimestamp; value: number };

type RollingHighLow = {
  highestHighs: number[];
  lowestLows: number[];
};

function toIndicatorTimestamp(at: string): UTCTimestamp {
  return Math.floor(new Date(at).getTime() / 1000) as UTCTimestamp;
}

function calculateRollingHighLow(
  candles: readonly KlineCandle[],
  period: number,
): RollingHighLow {
  const highestHighs = new Array<number>(candles.length);
  const lowestLows = new Array<number>(candles.length);
  const highDeque: number[] = [];
  const lowDeque: number[] = [];

  for (let index = 0; index < candles.length; index += 1) {
    const windowStart = Math.max(0, index - period + 1);
    const candle = candles[index]!;

    while (highDeque.length > 0 && highDeque[0]! < windowStart) {
      highDeque.shift();
    }
    while (lowDeque.length > 0 && lowDeque[0]! < windowStart) {
      lowDeque.shift();
    }

    while (
      highDeque.length > 0 &&
      candles[highDeque[highDeque.length - 1]!]!.high <= candle.high
    ) {
      highDeque.pop();
    }
    while (
      lowDeque.length > 0 &&
      candles[lowDeque[lowDeque.length - 1]!]!.low >= candle.low
    ) {
      lowDeque.pop();
    }

    highDeque.push(index);
    lowDeque.push(index);
    highestHighs[index] = candles[highDeque[0]!]!.high;
    lowestLows[index] = candles[lowDeque[0]!]!.low;
  }

  return { highestHighs, lowestLows };
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
  const multiplier = 2 / 10;
  const diff: IndicatorPoint[] = new Array(candles.length);
  const dea: IndicatorPoint[] = new Array(candles.length);
  const histogram: IndicatorPoint[] = new Array(candles.length);
  let previousSignal: number | null = null;

  for (let index = 0; index < candles.length; index += 1) {
    const candle = candles[index]!;
    const diffValue = (ema12[index] ?? 0) - (ema26[index] ?? 0);
    const deaValue: number =
      previousSignal == null
        ? diffValue
        : previousSignal + (diffValue - previousSignal) * multiplier;
    previousSignal = deaValue;
    const timestamp = toIndicatorTimestamp(candle.at);

    diff[index] = { time: timestamp, value: diffValue };
    dea[index] = { time: timestamp, value: deaValue };
    histogram[index] = { time: timestamp, value: (diffValue - deaValue) * 2 };
  }

  return { diff, dea, histogram };
}

export function computeKdj(candles: readonly KlineCandle[]): {
  k: IndicatorPoint[];
  d: IndicatorPoint[];
  j: IndicatorPoint[];
} {
  const { highestHighs, lowestLows } = calculateRollingHighLow(candles, 9);
  let previousK = 50;
  let previousD = 50;

  return candles.reduce(
    (result, candle, index) => {
      const highestHigh = highestHighs[index]!;
      const lowestLow = lowestLows[index]!;
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

export function computeAtr(
  candles: readonly KlineCandle[],
  period = 14,
): IndicatorPoint[] {
  const result: IndicatorPoint[] = [];
  const trueRanges = new Array<number>(candles.length);
  let rollingSum = 0;

  for (let index = 0; index < candles.length; index += 1) {
    const candle = candles[index]!;
    const previousClose = candles[index - 1]?.close ?? candle.close;
    const trueRange =
      index === 0
        ? candle.high - candle.low
        : Math.max(
            candle.high - candle.low,
            Math.abs(candle.high - previousClose),
            Math.abs(candle.low - previousClose),
          );

    trueRanges[index] = trueRange;
    rollingSum += trueRange;
    if (index >= period) {
      rollingSum -= trueRanges[index - period]!;
    }

    if (index + 1 >= period) {
      result.push({
        time: toIndicatorTimestamp(candle.at),
        value: rollingSum / period,
      });
    }
  }

  return result;
}

export function computeCci(
  candles: readonly KlineCandle[],
  period = 20,
): IndicatorPoint[] {
  const typicalPrices = candles.map((candle) => (candle.high + candle.low + candle.close) / 3);
  const result: IndicatorPoint[] = [];
  let rollingSum = 0;

  for (let index = 0; index < candles.length; index += 1) {
    rollingSum += typicalPrices[index]!;
    if (index >= period) {
      rollingSum -= typicalPrices[index - period]!;
    }
    if (index + 1 < period) {
      continue;
    }

    const mean = rollingSum / period;
    let meanDeviation = 0;
    for (let cursor = index - period + 1; cursor <= index; cursor += 1) {
      meanDeviation += Math.abs(typicalPrices[cursor]! - mean);
    }
    meanDeviation /= period;
    const value = meanDeviation === 0 ? 0 : (typicalPrices[index]! - mean) / (0.015 * meanDeviation);
    result.push({ time: toIndicatorTimestamp(candles[index]!.at), value });
  }

  return result;
}

export function computeWilliamsR(
  candles: readonly KlineCandle[],
  period = 14,
): IndicatorPoint[] {
  const { highestHighs, lowestLows } = calculateRollingHighLow(candles, period);
  const result: IndicatorPoint[] = [];

  for (let index = period - 1; index < candles.length; index += 1) {
    const candle = candles[index]!;
    const highestHigh = highestHighs[index]!;
    const lowestLow = lowestLows[index]!;
    const range = highestHigh - lowestLow;
    result.push({
      time: toIndicatorTimestamp(candle.at),
      value: range === 0 ? -50 : ((highestHigh - candle.close) / range) * -100,
    });
  }

  return result;
}