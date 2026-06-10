import type {
  MarketDataExtendedQuote,
  MarketDataExtendedQuoteBlocks,
  MarketSecurityDetails,
  MarketSecurityDetailsQueryResult,
  MarketSecurityEquityDetails,
  MarketSecurityFutureDetails,
  MarketSecurityIndexDetails,
  MarketSecurityOptionDetails,
  MarketSecurityPlateDetails,
  MarketSecurityRef,
  MarketSecurityTrustDetails,
  MarketSecurityWarrantDetails,
} from "@/contracts";

import {
  finalizeMarketDataRealtimeCandleDisplayAt,
  resolveMarketDataRealtimeBucketStart,
} from "./marketDataRealtimeBuckets";
import {
  resolveMarketDataTickVolumeUpdate,
  type MarketDataRealtimeTickVolumeState,
} from "./marketDataRealtimeTickState";
import {
  resolveMarketDataBarVolumeUpdate,
  type MarketDataRealtimeBarVolumeState,
} from "./marketDataRealtimeBarVolumeState";
import {
  resolveMarketDataBarPriceUpdate,
  type MarketDataRealtimeBarPriceState,
} from "./marketDataRealtimeBarPriceState";
import {
  mergeMarketDataCandles,
  upsertMarketDataRealtimeCandle,
  upsertMarketDataTickCandle,
} from "./marketDataRealtimeCandles";
import { mergeMarketDataSnapshot } from "./marketDataRealtimeSnapshot";
import {
  resolveMarketDataRealtimeTickBucketAt,
  resolveMarketDataRealtimeTickObservedAt,
} from "./marketDataRealtimeTickContext";

export type MarketDataSession =
  | "regular"
  | "pre"
  | "after"
  | "overnight"
  | "closed"
  | "unknown";

export type {
  MarketDataExtendedQuote,
  MarketDataExtendedQuoteBlocks,
  MarketSecurityDetails,
  MarketSecurityDetailsQueryResult,
  MarketSecurityEquityDetails,
  MarketSecurityFutureDetails,
  MarketSecurityIndexDetails,
  MarketSecurityOptionDetails,
  MarketSecurityPlateDetails,
  MarketSecurityRef,
  MarketSecurityTrustDetails,
  MarketSecurityWarrantDetails,
};

const marketDataExtendedQuoteNumberKeys = [
  "price",
  "highPrice",
  "lowPrice",
  "volume",
  "turnover",
  "changeVal",
  "changeRate",
  "amplitude",
] as const;

const marketDataSnapshotNumberKeys = [
  "price",
  "bid",
  "ask",
  "openPrice",
  "highPrice",
  "lowPrice",
  "previousClosePrice",
  "lastClosePrice",
  "volume",
  "turnover",
  "barVolume",
  "barOpen",
  "barHigh",
  "barLow",
] as const;

const marketDataCandleNumberKeys = [
  "open",
  "high",
  "low",
  "close",
  "volume",
] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function normalizeNumberish(value: unknown): number | undefined {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function normalizeFields<T extends Record<string, unknown> | null | undefined>(
  value: T,
  keys: readonly string[],
): T {
  if (!isRecord(value)) {
    return value;
  }
  const normalized: Record<string, unknown> = { ...value };
  for (const key of keys) {
    if (!(key in normalized)) {
      continue;
    }
    const current = normalized[key];
    if (current == null) {
      continue;
    }
    const parsed = normalizeNumberish(current);
    if (parsed !== undefined) {
      normalized[key] = parsed;
    }
  }
  return normalized as T;
}

function normalizeMarketDataExtendedQuote(
  value: unknown,
): MarketDataExtendedQuote | null {
  return normalizeFields(
    isRecord(value) ? value : null,
    marketDataExtendedQuoteNumberKeys,
  ) as MarketDataExtendedQuote | null;
}

function normalizeMarketDataExtendedQuoteBlocks(
  value: unknown,
): MarketDataExtendedQuoteBlocks | null {
  if (!isRecord(value)) {
    return null;
  }
  return {
    ...value,
    preMarket: normalizeMarketDataExtendedQuote(value.preMarket),
    afterMarket: normalizeMarketDataExtendedQuote(value.afterMarket),
    overnight: normalizeMarketDataExtendedQuote(value.overnight),
  } as MarketDataExtendedQuoteBlocks;
}

function normalizeMarketDataSnapshotValue(
  value: unknown,
): MarketDataSnapshotQueryResult["snapshot"] {
  const snapshot = normalizeFields(
    isRecord(value) ? value : null,
    marketDataSnapshotNumberKeys,
  );
  if (!isRecord(snapshot)) {
    return null;
  }
  return {
    ...snapshot,
    extended: normalizeMarketDataExtendedQuoteBlocks(snapshot.extended),
  } as MarketDataSnapshotQueryResult["snapshot"];
}

function normalizeMarketDataCandle(
  value: unknown,
): MarketDataCandlesQueryResult["candles"][number] | null {
  const candle = normalizeFields(
    isRecord(value) ? value : null,
    marketDataCandleNumberKeys,
  );
  if (!isRecord(candle)) {
    return null;
  }
  return candle as MarketDataCandlesQueryResult["candles"][number];
}

export function normalizeMarketDataSnapshotQueryResult(
  result: MarketDataSnapshotQueryResult,
): MarketDataSnapshotQueryResult {
  return {
    ...result,
    snapshot: normalizeMarketDataSnapshotValue(result.snapshot),
  };
}

export function normalizeMarketDataCandlesQueryResult(
  result: MarketDataCandlesQueryResult,
): MarketDataCandlesQueryResult {
  return {
    ...result,
    candles: Array.isArray(result.candles)
      ? result.candles
          .map((candle) => normalizeMarketDataCandle(candle))
          .filter(
            (candle): candle is MarketDataCandlesQueryResult["candles"][number] =>
              candle != null,
          )
      : [],
  };
}

export interface MarketDataSnapshotQueryResult {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  snapshot: {
    price: number;
    bid: number;
    ask: number;
    openPrice?: number | null;
    highPrice?: number | null;
    lowPrice?: number | null;
    previousClosePrice?: number | null;
    lastClosePrice?: number | null;
    volume: number;
    turnover: number;
    at: string;
    observedAt?: string | null;
    barVolume?: number | null;
    barOpen?: number | null;
    barHigh?: number | null;
    barLow?: number | null;
    session?: MarketDataSession | string | null;
    extendedHours?: boolean | null;
    extended?: MarketDataExtendedQuoteBlocks | null;
  } | null;
  meta: {
    instrumentId: string;
    source: string | null;
    resolvedAt: string;
    fromCache: boolean;
  };
}

export interface MarketDataCandlesQueryResult {
  request: {
    instrument: {
      market: string;
      symbol: string;
      instrumentId: string;
    };
    period: string;
    limit: number;
  };
  candles: Array<{
    period: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
    at: string;
    displayAt?: string | null;
    session?: MarketDataSession | string | null;
  }>;
  totalReturned: number;
  meta: {
    instrumentId: string;
    source: string | null;
    resolvedAt: string;
    fromCache: boolean;
    extendedHours?: boolean | null;
    session?: string | null;
  };
}

export interface MarketDataTickLiveEvent {
  type: "market-data.tick";
  at: string;
  brokerId: string;
  instrument: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  snapshot: {
    price: number;
    bid: number;
    ask: number;
    openPrice?: number | null;
    highPrice?: number | null;
    lowPrice?: number | null;
    previousClosePrice?: number | null;
    lastClosePrice?: number | null;
    volume: number;
    turnover: number;
    at: string;
    observedAt?: string | null;
    barVolume?: number | null;
    session?: MarketDataSession | string | null;
    extendedHours?: boolean | null;
    extended?: MarketDataExtendedQuoteBlocks | null;
  };
  source: string | null;
}

function isMarketDataTickLiveEvent(
  event: unknown,
): event is MarketDataTickLiveEvent {
  return (
    typeof event === "object" &&
    event !== null &&
    "type" in event &&
    event.type === "market-data.tick" &&
    "instrument" in event &&
    "snapshot" in event
  );
}

export function normalizeMarketDataTickLiveEvent(
  event: unknown,
): MarketDataTickLiveEvent | null {
  if (!isMarketDataTickLiveEvent(event)) {
    return null;
  }
  const snapshot = normalizeMarketDataSnapshotValue(event.snapshot);
  if (snapshot == null) {
    return null;
  }
  return {
    ...event,
    snapshot,
  } as MarketDataTickLiveEvent;
}

interface MarketDataRealtimeContext {
  candles: MarketDataCandlesQueryResult | null;
  period: string;
}

interface ApplyMarketDataTickEventInput extends MarketDataRealtimeContext {
  event: unknown;
  currentInstrumentId: string;
  limit: number;
}

interface ApplyMarketDataTickEventResult {
  snapshot: MarketDataSnapshotQueryResult;
  candles: MarketDataCandlesQueryResult | null;
}

export interface MarketDataRealtimeController {
  reset(): void;
  mergeCandles(
    current: MarketDataCandlesQueryResult | null,
    next: MarketDataCandlesQueryResult,
  ): MarketDataCandlesQueryResult;
  mergeSnapshot(
    current: MarketDataSnapshotQueryResult | null,
    context: MarketDataRealtimeContext,
  ): MarketDataSnapshotQueryResult | null;
  applyTickEvent(
    input: ApplyMarketDataTickEventInput,
  ): ApplyMarketDataTickEventResult | null;
}

export function createMarketDataRealtimeController(): MarketDataRealtimeController {
  let marketDataRealtimeBarVolumeState: MarketDataRealtimeBarVolumeState | null =
    null;
  let marketDataRealtimeBarPriceState: MarketDataRealtimeBarPriceState | null =
    null;
  let marketDataRealtimeTickVolumeState: MarketDataRealtimeTickVolumeState | null =
    null;

  function mergeCandles(
    current: MarketDataCandlesQueryResult | null,
    next: MarketDataCandlesQueryResult,
  ): MarketDataCandlesQueryResult {
    return mergeMarketDataCandles(current, next);
  }

  function reset(): void {
    marketDataRealtimeBarVolumeState = null;
    marketDataRealtimeBarPriceState = null;
    marketDataRealtimeTickVolumeState = null;
  }

  function resolveMarketDataTickSampleVolume(
    event: MarketDataTickLiveEvent,
  ): number {
    const resolution = resolveMarketDataTickVolumeUpdate(
      marketDataRealtimeTickVolumeState,
      event.instrument.instrumentId,
      event.snapshot.volume,
    );
    marketDataRealtimeTickVolumeState = resolution.nextState;
    return resolution.deltaVolume;
  }

  function resolveMarketDataCurrentBarPriceState(
    event: MarketDataTickLiveEvent,
    context: MarketDataRealtimeContext,
  ): {
    priceState: MarketDataRealtimeBarPriceState | null;
    candles: MarketDataCandlesQueryResult | null;
  } {
    const bucketAt = resolveMarketDataRealtimeTickBucketAt({
      period: context.period,
      candles: context.candles?.candles ?? [],
      eventAt: event.at,
      snapshot: event.snapshot,
    });
    if (bucketAt == null) {
      marketDataRealtimeBarPriceState = null;
      return {
        priceState: null,
        candles: context.candles,
      };
    }

    let candles = context.candles;
    const existingCandle = candles?.candles.find((candle) => candle.at === bucketAt);
    const previousState = marketDataRealtimeBarPriceState;
    const resolution = resolveMarketDataBarPriceUpdate({
      previousState,
      instrumentId: event.instrument.instrumentId,
      period: context.period,
      bucketAt,
      price: event.snapshot.price,
      existingCandle:
        existingCandle == null
          ? null
          : {
              open: existingCandle.open,
              high: existingCandle.high,
              low: existingCandle.low,
            },
      lastHistoricalClose: candles?.candles.at(-1)?.close ?? null,
    });

    if (resolution.shouldFinalizePreviousBucket && previousState != null) {
      candles = finalizeMarketDataRealtimeCandleDisplayAt(
        previousState.period,
        previousState.bucketAt,
        candles,
      );
    }

    marketDataRealtimeBarPriceState = resolution.nextState;

    return {
      priceState: marketDataRealtimeBarPriceState,
      candles,
    };
  }

  function resolveMarketDataCurrentBarVolume(
    event: MarketDataTickLiveEvent,
    context: MarketDataRealtimeContext,
  ): number | null {
    if (context.period === "tick") {
      return resolveMarketDataTickSampleVolume(event);
    }

    const bucketAt = resolveMarketDataRealtimeTickBucketAt({
      period: context.period,
      candles: context.candles?.candles ?? [],
      eventAt: event.at,
      snapshot: event.snapshot,
    });
    if (bucketAt == null) {
      marketDataRealtimeBarVolumeState = null;
      return null;
    }

    const existingCandle = context.candles?.candles.find(
      (candle) => candle.at === bucketAt,
    );
    const resolution = resolveMarketDataBarVolumeUpdate({
      previousState: marketDataRealtimeBarVolumeState,
      instrumentId: event.instrument.instrumentId,
      period: context.period,
      bucketAt,
      cumulativeVolume: event.snapshot.volume,
      existingCandleVolume: existingCandle?.volume ?? null,
      existingCandleUnfinalized:
        existingCandle != null && existingCandle.displayAt == null,
    });
    marketDataRealtimeBarVolumeState = resolution.nextState;
    return resolution.currentBarVolume;
  }

  function mergeSnapshot(
    current: MarketDataSnapshotQueryResult | null,
    context: MarketDataRealtimeContext,
  ): MarketDataSnapshotQueryResult | null {
    return mergeMarketDataSnapshot({
      current,
      context,
      barPriceState: marketDataRealtimeBarPriceState,
      barVolumeState: marketDataRealtimeBarVolumeState,
    });
  }

  function applyTickEvent(
    input: ApplyMarketDataTickEventInput,
  ): ApplyMarketDataTickEventResult | null {
    const event = normalizeMarketDataTickLiveEvent(input.event);
    if (event == null) {
      return null;
    }

    if (event.instrument.instrumentId !== input.currentInstrumentId) {
      return null;
    }

    const observedAt = resolveMarketDataRealtimeTickObservedAt({
      eventAt: event.at,
      snapshot: event.snapshot,
    });
    const priceStateResult = resolveMarketDataCurrentBarPriceState(
      event,
      input,
    );
    const nextInput = {
      ...input,
      candles: priceStateResult.candles,
    };
    const currentBarVolume = resolveMarketDataCurrentBarVolume(
      event,
      nextInput,
    );
    const snapshot = {
      request: event.instrument,
      snapshot: {
        ...event.snapshot,
        observedAt,
        barVolume: currentBarVolume,
        barOpen: priceStateResult.priceState?.open ?? null,
        barHigh: priceStateResult.priceState?.high ?? null,
        barLow: priceStateResult.priceState?.low ?? null,
      },
      meta: {
        instrumentId: event.instrument.instrumentId,
        source: event.source,
        resolvedAt: event.at,
        fromCache: false,
      },
    };

    return {
      snapshot,
      candles:
        input.period === "tick"
          ? upsertMarketDataTickCandle({
              current: nextInput.candles,
              instrument: event.instrument,
              limit: input.limit,
              source: event.source,
              resolvedAt: event.at,
              price: event.snapshot.price,
              observedAt,
              currentBarVolume,
              session: event.snapshot.session,
            })
          : priceStateResult.priceState == null
            ? nextInput.candles
            : upsertMarketDataRealtimeCandle({
                current: nextInput.candles,
                instrument: event.instrument,
                period: priceStateResult.priceState.period,
                limit: input.limit,
                source: event.source,
                resolvedAt: event.at,
                price: event.snapshot.price,
                currentBarVolume,
                bucketAt: priceStateResult.priceState.bucketAt,
                open: priceStateResult.priceState.open,
                high: priceStateResult.priceState.high,
                low: priceStateResult.priceState.low,
                session: event.snapshot.session,
              }),
    };
  }

  return {
    reset,
    mergeCandles,
    mergeSnapshot,
    applyTickEvent,
  };
}