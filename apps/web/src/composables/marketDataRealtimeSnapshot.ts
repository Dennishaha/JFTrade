import { resolveMarketDataRealtimeBucketStart } from "./marketDataRealtimeBuckets";
import {
  resolveMarketDataBarVolumeValue,
  type MarketDataRealtimeBarVolumeState,
} from "./marketDataRealtimeBarVolumeState";
import type { MarketDataRealtimeBarPriceState } from "./marketDataRealtimeBarPriceState";
import type { MarketDataRealtimeTickVolumeState } from "./marketDataRealtimeTickState";
import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
} from "./marketDataRealtime";
import { findMarketDataCandleAt } from "./marketDataRealtimeCandles";

interface MarketDataRealtimeSnapshotContext {
  candles: MarketDataCandlesQueryResult | null;
  period: string;
}

interface MergeMarketDataSnapshotInput {
  current: MarketDataSnapshotQueryResult | null;
  context: MarketDataRealtimeSnapshotContext;
  barPriceState: MarketDataRealtimeBarPriceState | null;
  barVolumeState: MarketDataRealtimeBarVolumeState | null;
  tickVolumeState: MarketDataRealtimeTickVolumeState | null;
}

export function mergeMarketDataSnapshot(
  input: MergeMarketDataSnapshotInput,
): MarketDataSnapshotQueryResult | null {
  if (
    input.current == null ||
    input.current.snapshot == null
  ) {
    return input.current;
  }

  const snapshot = input.current.snapshot;
  if (input.context.period === "tick") {
    const timelineAt = snapshot.observedAt ?? snapshot.at;
    const existingCandle =
      input.context.candles == null
        ? undefined
        : findMarketDataCandleAt(input.context.candles.candles, timelineAt);
    const trustedBarVolume =
      input.tickVolumeState != null &&
      input.tickVolumeState.instrumentId === input.current.request.instrumentId &&
      input.tickVolumeState.period === "tick" &&
      input.tickVolumeState.bucketAt === timelineAt
        ? input.tickVolumeState.currentSampleVolume
        : existingCandle?.volume ?? null;
    if (snapshot.barVolume === trustedBarVolume) {
      return input.current;
    }
    return {
      ...input.current,
      snapshot: {
        ...snapshot,
        barVolume: trustedBarVolume,
      },
    };
  }

  const bucketAt = resolveMarketDataRealtimeBucketStart(
    input.context.period,
    input.context.candles?.candles ?? [],
    {
      price: snapshot.price,
      // Volume is irrelevant to bucket resolution; keep the cumulative quote
      // field out of every bar-volume derivation path.
      volume: 0,
      at: snapshot.at,
      observedAt: snapshot.observedAt ?? snapshot.at,
    },
  );
  if (bucketAt == null) {
    return input.current;
  }

  const existingCandle =
    input.context.candles == null
      ? undefined
      : findMarketDataCandleAt(input.context.candles.candles, bucketAt);
  let nextSnapshot = snapshot;
  if (existingCandle != null) {
    nextSnapshot = {
      ...nextSnapshot,
      barOpen: nextSnapshot.barOpen ?? existingCandle.open,
      barHigh: nextSnapshot.barHigh ?? existingCandle.high,
      barLow: nextSnapshot.barLow ?? existingCandle.low,
    };
  }

  if (
    input.barPriceState != null &&
    input.barPriceState.instrumentId === input.current.request.instrumentId &&
    input.barPriceState.period === input.context.period &&
    input.barPriceState.bucketAt === bucketAt
  ) {
    nextSnapshot = {
      ...nextSnapshot,
      barOpen: input.barPriceState.open,
      barHigh: Math.max(
        input.barPriceState.high,
        existingCandle?.high ?? input.barPriceState.high,
      ),
      barLow: Math.min(
        input.barPriceState.low,
        existingCandle?.low ?? input.barPriceState.low,
      ),
    };
  }

  const trustedBarVolume =
    input.barVolumeState != null &&
    input.barVolumeState.instrumentId === input.current.request.instrumentId &&
    input.barVolumeState.period === input.context.period &&
    input.barVolumeState.bucketAt === bucketAt
      ? resolveMarketDataBarVolumeValue(
          input.barVolumeState,
          existingCandle?.volume ?? null,
        )
      : existingCandle?.volume ?? null;
  if (nextSnapshot.barVolume !== trustedBarVolume) {
    nextSnapshot = {
      ...nextSnapshot,
      barVolume: trustedBarVolume,
    };
  }

  return nextSnapshot === snapshot
    ? input.current
    : {
        ...input.current,
        snapshot: nextSnapshot,
      };
}
