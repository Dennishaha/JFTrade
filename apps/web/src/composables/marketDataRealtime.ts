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

export interface MarketDataExtendedQuote {
  price?: number | null;
  highPrice?: number | null;
  lowPrice?: number | null;
  volume?: number | null;
  turnover?: number | null;
  changeVal?: number | null;
  changeRate?: number | null;
  amplitude?: number | null;
}

export interface MarketDataExtendedQuoteBlocks {
  preMarket?: MarketDataExtendedQuote | null;
  afterMarket?: MarketDataExtendedQuote | null;
  overnight?: MarketDataExtendedQuote | null;
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

interface MarketDataTickLiveEvent {
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
    if (!isMarketDataTickLiveEvent(input.event)) {
      return null;
    }

    if (input.event.instrument.instrumentId !== input.currentInstrumentId) {
      return null;
    }

    const observedAt = resolveMarketDataRealtimeTickObservedAt({
      eventAt: input.event.at,
      snapshot: input.event.snapshot,
    });
    const priceStateResult = resolveMarketDataCurrentBarPriceState(
      input.event,
      input,
    );
    const nextInput = {
      ...input,
      candles: priceStateResult.candles,
    };
    const currentBarVolume = resolveMarketDataCurrentBarVolume(
      input.event,
      nextInput,
    );
    const snapshot = {
      request: input.event.instrument,
      snapshot: {
        ...input.event.snapshot,
        observedAt,
        barVolume: currentBarVolume,
        barOpen: priceStateResult.priceState?.open ?? null,
        barHigh: priceStateResult.priceState?.high ?? null,
        barLow: priceStateResult.priceState?.low ?? null,
      },
      meta: {
        instrumentId: input.event.instrument.instrumentId,
        source: input.event.source,
        resolvedAt: input.event.at,
        fromCache: false,
      },
    };

    return {
      snapshot,
      candles:
        input.period === "tick"
          ? upsertMarketDataTickCandle({
              current: nextInput.candles,
              instrument: input.event.instrument,
              limit: input.limit,
              source: input.event.source,
              resolvedAt: input.event.at,
              price: input.event.snapshot.price,
              observedAt,
              currentBarVolume,
              session: input.event.snapshot.session,
            })
          : priceStateResult.priceState == null
            ? nextInput.candles
            : upsertMarketDataRealtimeCandle({
                current: nextInput.candles,
                instrument: input.event.instrument,
                period: priceStateResult.priceState.period,
                limit: input.limit,
                source: input.event.source,
                resolvedAt: input.event.at,
                price: input.event.snapshot.price,
                currentBarVolume,
                bucketAt: priceStateResult.priceState.bucketAt,
                open: priceStateResult.priceState.open,
                high: priceStateResult.priceState.high,
                low: priceStateResult.priceState.low,
                session: input.event.snapshot.session,
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