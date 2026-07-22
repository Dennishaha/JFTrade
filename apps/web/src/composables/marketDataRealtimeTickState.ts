import {
  resolveMarketDataVolumeSequence,
  type MarketDataRealtimeVolumeSequenceState,
} from "./marketDataRealtimeVolumeSequence";

export interface MarketDataRealtimeTickVolumeState {
  instrumentId: string;
  period: "tick";
  bucketAt: string;
  currentSampleVolume: number;
  sequence: MarketDataRealtimeVolumeSequenceState;
}

export interface MarketDataRealtimeTickVolumeUpdateInput {
  previousState: MarketDataRealtimeTickVolumeState | null;
  instrumentId: string;
  bucketAt: string;
  observedAt: string;
  cumulativeVolume?: number | null | undefined;
  volumeDelta?: number | null | undefined;
}

export interface MarketDataRealtimeTickVolumeResolution {
  deltaVolume: number;
  ignored: boolean;
  nextState: MarketDataRealtimeTickVolumeState | null;
}

export function resolveMarketDataTickVolumeUpdate(
  input: MarketDataRealtimeTickVolumeUpdateInput,
): MarketDataRealtimeTickVolumeResolution {
  const previousState = input.previousState;
  const sameSeries =
    previousState != null &&
    previousState.instrumentId === input.instrumentId &&
    previousState.period === "tick";
  const sequenceResolution = resolveMarketDataVolumeSequence({
    previousState: sameSeries && previousState != null ? previousState.sequence : null,
    observedAt: input.observedAt,
    cumulativeVolume: input.cumulativeVolume,
    volumeDelta: input.volumeDelta,
  });
  if (sequenceResolution.ignored || sequenceResolution.nextState == null) {
    return {
      deltaVolume: 0,
      ignored: true,
      nextState: previousState,
    };
  }

  const sameBucket =
    sameSeries && previousState != null && previousState.bucketAt === input.bucketAt;
  const deltaVolume =
    sequenceResolution.isDuplicate && sameBucket && previousState != null
      ? previousState.currentSampleVolume
      : sequenceResolution.deltaVolume;

  return {
    deltaVolume,
    ignored: false,
    nextState: {
      instrumentId: input.instrumentId,
      period: "tick",
      bucketAt: input.bucketAt,
      currentSampleVolume: deltaVolume,
      sequence: sequenceResolution.nextState,
    },
  };
}
