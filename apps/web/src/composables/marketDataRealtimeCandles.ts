import { resolveMarketDataRealtimeCandleDisplayAt } from "./marketDataRealtimeBuckets";

import type {
  MarketDataCandlesQueryResult,
  MarketDataSession,
} from "./marketDataRealtime";

type MarketDataCandle = MarketDataCandlesQueryResult["candles"][number];
type MarketDataInstrument = MarketDataCandlesQueryResult["request"]["instrument"];

const MAX_REALTIME_TRIM_BATCH = 256;
const realtimeRetentionLimits = new WeakMap<MarketDataCandle[], number>();

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

export function findMarketDataCandleAt(
  candles: readonly MarketDataCandle[],
  at: string,
): MarketDataCandle | undefined {
  const lastIndex = candles.length - 1;
  const last = candles[lastIndex];
  if (last?.at === at) {
    return last;
  }

  // Realtime buckets normally live at the tail. Keep first-match semantics for
  // older or externally supplied series without scanning on every live tick.
  for (let index = 0; index < lastIndex; index += 1) {
    const candle = candles[index];
    if (candle?.at === at) {
      return candle;
    }
  }
  return undefined;
}

export function mergeMarketDataCandles(
  current: MarketDataCandlesQueryResult | null,
  next: MarketDataCandlesQueryResult,
): MarketDataCandlesQueryResult {
  if (current == null) {
    return next;
  }
  if (
    current.request.instrument.instrumentId !==
      next.request.instrument.instrumentId ||
    current.request.period !== next.request.period
  ) {
    return next;
  }

  // A candles request can finish after websocket ticks have already updated
  // the active bucket.  Use the payload resolution time to decide which close
  // is newer, while retaining the widest OHLC range and largest per-bar
  // volume.  This prevents a late historical response from rolling an active
  // candle (and its volume baseline) backwards.
  const preferCurrentOverlap =
    compareResolvedAt(current.meta.resolvedAt, next.meta.resolvedAt) > 0;
  const byKey = new Map<string, MarketDataCandle>();
  for (const candle of current.candles) {
    byKey.set(candleKey(candle), candle);
  }
  for (const candle of next.candles) {
    const key = candleKey(candle);
    const existing = byKey.get(key);
    byKey.set(
      key,
      existing == null
        ? candle
        : mergeOverlappingCandle(existing, candle, preferCurrentOverlap),
    );
  }
  const candles = [...byKey.values()].sort(
    (left, right) => new Date(left.at).getTime() - new Date(right.at).getTime(),
  );
  const freshestMeta = preferCurrentOverlap ? current.meta : next.meta;

  return {
    ...next,
    candles,
    totalReturned: candles.length,
    meta: {
      ...next.meta,
      ...freshestMeta,
    },
  };
}

function candleKey(candle: MarketDataCandle): string {
  return `${candle.period}:${candle.at}`;
}

function compareResolvedAt(left: string, right: string): number {
  const leftTime = Date.parse(left);
  const rightTime = Date.parse(right);
  if (!Number.isFinite(leftTime) || !Number.isFinite(rightTime)) {
    return 0;
  }
  return leftTime - rightTime;
}

function mergeOverlappingCandle(
  current: MarketDataCandle,
  next: MarketDataCandle,
  preferCurrent: boolean,
): MarketDataCandle {
  const preferred = preferCurrent ? current : next;
  const secondary = preferCurrent ? next : current;
  const merged: MarketDataCandle = {
    ...secondary,
    ...preferred,
    period: preferred.period,
    at: preferred.at,
    open: preferred.open,
    high: Math.max(current.high, next.high),
    low: Math.min(current.low, next.low),
    close: preferred.close,
    // Both values are per-bar volumes. They are comparable, never additive.
    volume: Math.max(current.volume, next.volume),
  };
  const displayAt = preferred.displayAt ?? secondary.displayAt;
  if (displayAt !== undefined) {
    merged.displayAt = displayAt;
  }
  const session = preferred.session ?? secondary.session;
  if (session !== undefined) {
    merged.session = session;
  }
  return merged;
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
    const result: MarketDataCandlesQueryResult = {
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
    rememberRealtimeRetentionLimit(result.candles, input.limit);
    return result;
  }

  return upsertOwnedRealtimeCandle(
    input.current,
    realtimeCandle,
    input.limit,
    input.source,
    input.resolvedAt,
  );
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
    const result: MarketDataCandlesQueryResult = {
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
    rememberRealtimeRetentionLimit(result.candles, input.limit);
    return result;
  }

  return upsertOwnedRealtimeCandle(
    input.current,
    tickCandle,
    input.limit,
    input.source,
    input.resolvedAt,
  );
}

function rememberRealtimeRetentionLimit(
  candles: MarketDataCandle[],
  requestedLimit: number,
): number {
  const remembered = realtimeRetentionLimits.get(candles);
  if (remembered != null) {
    return remembered;
  }
  const normalizedLimit = Number.isFinite(requestedLimit)
    ? Math.max(1, Math.trunc(requestedLimit))
    : 1;
  const retentionLimit = Math.max(normalizedLimit, candles.length);
  realtimeRetentionLimits.set(candles, retentionLimit);
  return retentionLimit;
}

function upsertOwnedRealtimeCandle(
  current: MarketDataCandlesQueryResult,
  candle: MarketDataCandle,
  requestedLimit: number,
  source: string | null,
  resolvedAt: string,
): MarketDataCandlesQueryResult {
  const candles = current.candles;
  const retentionLimit = rememberRealtimeRetentionLimit(candles, requestedLimit);
  const lastIndex = candles.length - 1;
  const last = candles[lastIndex];

  if (last != null && last.period === candle.period && last.at === candle.at) {
    candles[lastIndex] = candle;
  } else if (last == null || compareCandles(last, candle) < 0) {
    candles.push(candle);
  } else {
    const insertionIndex = findCandleInsertionIndex(candles, candle);
    const existing = candles[insertionIndex];
    if (existing?.period === candle.period && existing.at === candle.at) {
      candles[insertionIndex] = candle;
    } else {
      candles.splice(insertionIndex, 0, candle);
    }
  }

  const trimBatch = Math.max(
    1,
    Math.min(MAX_REALTIME_TRIM_BATCH, Math.ceil(retentionLimit * 0.1)),
  );
  if (candles.length >= retentionLimit + trimBatch) {
    candles.splice(0, candles.length - retentionLimit);
  }

  return {
    ...current,
    candles,
    totalReturned: candles.length,
    meta: {
      ...current.meta,
      source,
      resolvedAt,
      fromCache: false,
    },
  };
}

function findCandleInsertionIndex(
  candles: MarketDataCandle[],
  candle: MarketDataCandle,
): number {
  let low = 0;
  let high = candles.length;
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    const candidate = candles[middle];
    if (candidate != null && compareCandles(candidate, candle) < 0) {
      low = middle + 1;
    } else {
      high = middle;
    }
  }
  return low;
}

function compareCandles(left: MarketDataCandle, right: MarketDataCandle): number {
  const leftTime = Date.parse(left.at);
  const rightTime = Date.parse(right.at);
  if (
    Number.isFinite(leftTime) &&
    Number.isFinite(rightTime) &&
    leftTime !== rightTime
  ) {
    return leftTime - rightTime;
  }
  if (left.at !== right.at) {
    return left.at.localeCompare(right.at);
  }
  return left.period.localeCompare(right.period);
}
