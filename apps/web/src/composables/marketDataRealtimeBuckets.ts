import type {
  KlineCandle,
  RealtimeKlineSnapshot,
} from "../charting/kline";
import {
  resolveKlineBucketDisplayAt,
  resolveRealtimeBucketStart,
} from "../charting/kline";

export function resolveMarketDataRealtimeCandleDisplayAt(
  period: string,
  bucketAt: string,
): string | null {
  return resolveKlineBucketDisplayAt(period, bucketAt);
}

export function resolveMarketDataRealtimeBucketStart(
  period: string,
  candles: readonly KlineCandle[],
  snapshot: RealtimeKlineSnapshot,
): string | null {
  if (period === "tick") {
    return null;
  }

  return resolveRealtimeBucketStart(candles, snapshot, period);
}

export function finalizeMarketDataRealtimeCandleDisplayAt<
  TCandle extends { period: string; at: string; displayAt?: string | null },
  TSeries extends { candles: TCandle[] },
>(
  period: string,
  bucketAt: string,
  series: TSeries | null,
): TSeries | null {
  if (series == null) {
    return null;
  }

  const displayAt = resolveMarketDataRealtimeCandleDisplayAt(period, bucketAt);
  if (displayAt == null) {
    return series;
  }

  return {
    ...series,
    candles: series.candles.map((candle) =>
      candle.period === period && candle.at === bucketAt
        ? ({ ...candle, displayAt } as TCandle)
        : candle,
    ),
  };
}