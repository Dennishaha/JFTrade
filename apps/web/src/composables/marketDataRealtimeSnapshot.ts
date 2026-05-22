import { resolveMarketDataRealtimeBucketStart } from "./marketDataRealtimeBuckets";
import {
  resolveMarketDataBarVolumeValue,
  type MarketDataRealtimeBarVolumeState,
} from "./marketDataRealtimeBarVolumeState";
import type { MarketDataRealtimeBarPriceState } from "./marketDataRealtimeBarPriceState";
import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
} from "./marketDataRealtime";

interface MarketDataRealtimeSnapshotContext {
  candles: MarketDataCandlesQueryResult | null;
  period: string;
}

interface MergeMarketDataSnapshotInput {
  current: MarketDataSnapshotQueryResult | null;
  context: MarketDataRealtimeSnapshotContext;
  barPriceState: MarketDataRealtimeBarPriceState | null;
  barVolumeState: MarketDataRealtimeBarVolumeState | null;
}

export function mergeMarketDataSnapshot(
  input: MergeMarketDataSnapshotInput,
): MarketDataSnapshotQueryResult | null {
  if (
    input.current == null ||
    input.current.snapshot == null ||
    input.context.period === "tick"
  ) {
    return input.current;
  }

  const snapshot = input.current.snapshot;
  const bucketAt = resolveMarketDataRealtimeBucketStart(
    input.context.period,
    input.context.candles?.candles ?? [],
    {
      price: snapshot.price,
      volume: snapshot.volume,
      at: snapshot.at,
      observedAt: snapshot.observedAt ?? snapshot.at,
    },
  );
  if (bucketAt == null) {
    return input.current;
  }

  const existingCandle = input.context.candles?.candles.find(
    (candle) => candle.at === bucketAt,
  );
  let nextSnapshot = snapshot;
  if (existingCandle != null) {
    nextSnapshot = {
      ...nextSnapshot,
      barOpen: nextSnapshot.barOpen ?? existingCandle.open,
      barHigh: nextSnapshot.barHigh ?? existingCandle.high,
      barLow: nextSnapshot.barLow ?? existingCandle.low,
      barVolume: nextSnapshot.barVolume ?? existingCandle.volume,
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

  if (
    input.barVolumeState != null &&
    input.barVolumeState.instrumentId === input.current.request.instrumentId &&
    input.barVolumeState.period === input.context.period &&
    input.barVolumeState.bucketAt === bucketAt &&
    Number.isFinite(nextSnapshot.volume) &&
    nextSnapshot.volume >= 0
  ) {
    nextSnapshot = {
      ...nextSnapshot,
      barVolume: resolveMarketDataBarVolumeValue(
        input.barVolumeState,
        nextSnapshot.volume,
        existingCandle?.volume ?? null,
      ),
    };
  }

  return nextSnapshot === snapshot
    ? input.current
    : {
        ...input.current,
        snapshot: nextSnapshot,
      };
}