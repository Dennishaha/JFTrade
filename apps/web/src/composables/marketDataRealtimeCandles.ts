import { resolveMarketDataRealtimeCandleDisplayAt } from "./marketDataRealtimeBuckets";

import type {
  MarketDataCandlesQueryResult,
  MarketDataSession,
} from "./marketDataRealtime";

type MarketDataCandle = MarketDataCandlesQueryResult["candles"][number];
type MarketDataInstrument = MarketDataCandlesQueryResult["request"]["instrument"];

interface UpsertMarketDataRealtimeCandleInput {
  current: MarketDataCandlesQueryResult | null;
  instrument: MarketDataInstrument;
  period: string;
  limit: number;
  source: string | null;
  resolvedAt: string;
  price: number;
  currentBarVolume: number | null;
  bucketAt: string;
  open: number;
  high: number;
  low: number;
  session?: MarketDataSession | string | null | undefined;
}

interface UpsertMarketDataTickCandleInput {
  current: MarketDataCandlesQueryResult | null;
  instrument: MarketDataInstrument;
  limit: number;
  source: string | null;
  resolvedAt: string;
  price: number;
  observedAt: string;
  currentBarVolume: number | null;
  session?: MarketDataSession | string | null | undefined;
}

export function mergeMarketDataCandles(
  current: MarketDataCandlesQueryResult | null,
  next: MarketDataCandlesQueryResult,
): MarketDataCandlesQueryResult {
  if (current == null) {
    return next;
  }

  const byKey = new Map(
    [...current.candles, ...next.candles].map((candle) => [
      `${candle.period}:${candle.at}`,
      candle,
    ]),
  );
  const candles = [...byKey.values()].sort(
    (left, right) => new Date(left.at).getTime() - new Date(right.at).getTime(),
  );

  return {
    ...next,
    candles,
    totalReturned: candles.length,
  };
}

export function upsertMarketDataRealtimeCandle(
  input: UpsertMarketDataRealtimeCandleInput,
): MarketDataCandlesQueryResult {
  const realtimeCandle: MarketDataCandle = {
    period: input.period,
    open: input.open,
    high: input.high,
    low: input.low,
    close: input.price,
    volume: input.currentBarVolume ?? 0,
    at: input.bucketAt,
  };

  const displayAt = resolveMarketDataRealtimeCandleDisplayAt(
    input.period,
    input.bucketAt,
  );
  if (displayAt != null) {
    realtimeCandle.displayAt = displayAt;
  }
  if (typeof input.session === "string") {
    realtimeCandle.session = input.session;
  }

  if (input.current == null) {
    return {
      request: {
        instrument: input.instrument,
        period: input.period,
        limit: input.limit,
      },
      candles: [realtimeCandle],
      totalReturned: 1,
      meta: {
        instrumentId: input.instrument.instrumentId,
        source: input.source,
        resolvedAt: input.resolvedAt,
        fromCache: false,
      },
    };
  }

  return mergeMarketDataCandles(input.current, {
    ...input.current,
    candles: [realtimeCandle],
    totalReturned: 1,
    meta: {
      ...input.current.meta,
      source: input.source,
      resolvedAt: input.resolvedAt,
      fromCache: false,
    },
  });
}

export function upsertMarketDataTickCandle(
  input: UpsertMarketDataTickCandleInput,
): MarketDataCandlesQueryResult {
  const tickCandle: MarketDataCandle = {
    period: "tick",
    open: input.price,
    high: input.price,
    low: input.price,
    close: input.price,
    volume: input.currentBarVolume ?? 0,
    at: input.observedAt,
  };
  if (typeof input.session === "string") {
    tickCandle.session = input.session;
  }

  if (input.current == null) {
    return {
      request: {
        instrument: input.instrument,
        period: "tick",
        limit: input.limit,
      },
      candles: [tickCandle],
      totalReturned: 1,
      meta: {
        instrumentId: input.instrument.instrumentId,
        source: input.source,
        resolvedAt: input.resolvedAt,
        fromCache: false,
      },
    };
  }

  return mergeMarketDataCandles(input.current, {
    ...input.current,
    candles: [tickCandle],
    totalReturned: 1,
    meta: {
      ...input.current.meta,
      source: input.source,
      resolvedAt: input.resolvedAt,
      fromCache: false,
    },
  });
}