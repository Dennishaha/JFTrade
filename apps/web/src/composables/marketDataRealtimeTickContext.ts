import type { KlineCandle } from "../charting/kline";

import { resolveMarketDataRealtimeBucketStart } from "./marketDataRealtimeBuckets";

interface MarketDataRealtimeTickSnapshotLike {
  price: number;
  volume: number;
  at: string;
  observedAt?: string | null;
}

export function resolveMarketDataRealtimeTickObservedAt(input: {
  eventAt: string;
  snapshot: Pick<MarketDataRealtimeTickSnapshotLike, "at" | "observedAt">;
}): string {
  const snapshotObservedAt = input.snapshot.observedAt?.trim();
  if (snapshotObservedAt != null && snapshotObservedAt !== "") {
    return snapshotObservedAt;
  }

  const eventObservedAt = input.eventAt.trim();
  return eventObservedAt === "" ? input.snapshot.at : eventObservedAt;
}

export function resolveMarketDataRealtimeTickBucketAt(input: {
  period: string;
  candles: readonly KlineCandle[];
  eventAt: string;
  snapshot: MarketDataRealtimeTickSnapshotLike;
}): string | null {
  return resolveMarketDataRealtimeBucketStart(input.period, input.candles, {
    price: input.snapshot.price,
    volume: input.snapshot.volume,
    at: input.snapshot.at,
    observedAt: resolveMarketDataRealtimeTickObservedAt({
      eventAt: input.eventAt,
      snapshot: input.snapshot,
    }),
  });
}