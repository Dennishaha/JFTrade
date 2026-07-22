import {
  resolveMarketDataVolumeSequence,
  type MarketDataRealtimeVolumeSequenceState,
} from "./marketDataRealtimeVolumeSequence";

export interface MarketDataRealtimeBarVolumeState {
  instrumentId: string;
  period: string;
  bucketAt: string;
  currentBarVolume: number;
  sequence: MarketDataRealtimeVolumeSequenceState;
}

export interface MarketDataRealtimeBarVolumeUpdateInput {
  previousState: MarketDataRealtimeBarVolumeState | null;
  instrumentId: string;
  period: string;
  bucketAt: string | null;
  observedAt: string;
  cumulativeVolume?: number | null | undefined;
  volumeDelta?: number | null | undefined;
  existingCandleVolume: number | null;
  existingCandleUnfinalized: boolean;
}

export interface MarketDataRealtimeBarVolumeResolution {
  currentBarVolume: number | null;
  ignored: boolean;
  nextState: MarketDataRealtimeBarVolumeState | null;
}

function normalizedExistingVolume(value: number | null): number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : 0;
}

export function resolveMarketDataBarVolumeValue(
  state: MarketDataRealtimeBarVolumeState,
  existingCandleVolume: number | null,
): number {
  return Math.max(
    state.currentBarVolume,
    normalizedExistingVolume(existingCandleVolume),
  );
}

export function resolveMarketDataBarVolumeUpdate(
  input: MarketDataRealtimeBarVolumeUpdateInput,
): MarketDataRealtimeBarVolumeResolution {
  const previousState = input.previousState;

  if (input.bucketAt == null) {
    return {
      currentBarVolume: null,
      ignored: false,
      nextState: null,
    };
  }

  const sameSeries =
    previousState != null &&
    previousState.instrumentId === input.instrumentId &&
    previousState.period === input.period;
  const sameBucket = sameSeries && previousState.bucketAt === input.bucketAt;
  if (
    sameSeries &&
    Date.parse(input.bucketAt) < Date.parse(previousState.bucketAt)
  ) {
    return {
      currentBarVolume: null,
      ignored: true,
      nextState: previousState,
    };
  }

  const sequenceResolution = resolveMarketDataVolumeSequence({
    previousState: sameSeries ? previousState.sequence : null,
    observedAt: input.observedAt,
    cumulativeVolume: input.cumulativeVolume,
    volumeDelta: input.volumeDelta,
  });
  if (sequenceResolution.ignored || sequenceResolution.nextState == null) {
    return {
      currentBarVolume: null,
      ignored: true,
      nextState: previousState,
    };
  }

  const existingCandleVolume = normalizedExistingVolume(
    input.existingCandleVolume,
  );
  const previousBarVolume = sameBucket
    ? previousState.currentBarVolume
    : 0;
  const baseBarVolume = Math.max(previousBarVolume, existingCandleVolume);
  const rebasedFromCandle =
    input.existingCandleUnfinalized &&
    input.existingCandleVolume != null &&
    (!sameBucket || existingCandleVolume > previousBarVolume);
  const incrementalVolume =
    rebasedFromCandle && sequenceResolution.source === "cumulative"
      ? 0
      : sequenceResolution.deltaVolume;
  const currentBarVolume = baseBarVolume + incrementalVolume;

  return {
    currentBarVolume,
    ignored: false,
    nextState: {
      instrumentId: input.instrumentId,
      period: input.period,
      bucketAt: input.bucketAt,
      currentBarVolume,
      sequence: sequenceResolution.nextState,
    },
  };
}
