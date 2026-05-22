export interface MarketDataRealtimeBarVolumeState {
  instrumentId: string;
  period: string;
  bucketAt: string;
  baselineCumulativeVolume: number;
  baseBarVolume: number;
}

export interface MarketDataRealtimeBarVolumeUpdateInput {
  previousState: MarketDataRealtimeBarVolumeState | null;
  instrumentId: string;
  period: string;
  bucketAt: string | null;
  cumulativeVolume: number;
  existingCandleVolume: number | null;
  existingCandleUnfinalized: boolean;
}

export interface MarketDataRealtimeBarVolumeResolution {
  currentBarVolume: number | null;
  nextState: MarketDataRealtimeBarVolumeState | null;
}

export function resolveMarketDataBarVolumeValue(
  state: MarketDataRealtimeBarVolumeState,
  cumulativeVolume: number,
  existingCandleVolume: number | null,
): number {
  const incrementalVolume = Math.max(
    0,
    cumulativeVolume - state.baselineCumulativeVolume,
  );
  return Math.max(
    state.baseBarVolume + incrementalVolume,
    existingCandleVolume ?? 0,
  );
}

export function resolveMarketDataBarVolumeUpdate(
  input: MarketDataRealtimeBarVolumeUpdateInput,
): MarketDataRealtimeBarVolumeResolution {
  const previousState = input.previousState;

  if (input.bucketAt == null) {
    return {
      currentBarVolume: null,
      nextState: null,
    };
  }

  if (!Number.isFinite(input.cumulativeVolume) || input.cumulativeVolume < 0) {
    return {
      currentBarVolume: input.existingCandleVolume,
      nextState: previousState,
    };
  }

  const shouldResetState =
    previousState == null ||
    previousState.instrumentId !== input.instrumentId ||
    previousState.period !== input.period ||
    previousState.bucketAt !== input.bucketAt ||
    input.cumulativeVolume < previousState.baselineCumulativeVolume;

  let nextState = previousState;
  if (shouldResetState) {
    nextState = {
      instrumentId: input.instrumentId,
      period: input.period,
      bucketAt: input.bucketAt,
      baselineCumulativeVolume: input.cumulativeVolume,
      baseBarVolume: input.existingCandleVolume ?? 0,
    };
  } else if (
    previousState != null &&
    input.existingCandleUnfinalized &&
    input.existingCandleVolume != null &&
    input.existingCandleVolume > previousState.baseBarVolume
  ) {
    nextState = {
      ...previousState,
      baselineCumulativeVolume: input.cumulativeVolume,
      baseBarVolume: input.existingCandleVolume,
    };
  }

  if (nextState == null) {
    return {
      currentBarVolume: input.existingCandleVolume,
      nextState,
    };
  }

  const incrementalVolume = Math.max(
    0,
    input.cumulativeVolume - nextState.baselineCumulativeVolume,
  );
  return {
    currentBarVolume: resolveMarketDataBarVolumeValue(
      nextState,
      input.cumulativeVolume,
      input.existingCandleVolume,
    ),
    nextState,
  };
}