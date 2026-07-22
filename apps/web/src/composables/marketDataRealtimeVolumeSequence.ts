export interface MarketDataRealtimeVolumeSequenceState {
  lastCumulativeVolume: number | null;
  lastObservedAt: string;
  lastObservedAtMs: number;
  lastSampleCumulativeVolume: number | null;
  lastSampleVolumeDelta: number | null;
}

export interface MarketDataRealtimeVolumeSequenceInput {
  previousState: MarketDataRealtimeVolumeSequenceState | null;
  observedAt: string;
  cumulativeVolume?: number | null | undefined;
  volumeDelta?: number | null | undefined;
}

export interface MarketDataRealtimeVolumeSequenceResolution {
  deltaVolume: number;
  ignored: boolean;
  isDuplicate: boolean;
  source: "cumulative" | "delta" | "none";
  nextState: MarketDataRealtimeVolumeSequenceState | null;
}

function finiteNonNegative(value: number | null | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : null;
}

/**
 * Resolves one explicitly-labelled volume sample.
 *
 * `cumulativeVolume` is treated as a monotonically increasing sequence until a
 * newer sample moves backwards, which starts a new sequence. `volumeDelta` is
 * used directly when no cumulative sequence is available, and can seed the
 * first sample of a newly established sequence. No caller should pass a quote
 * snapshot's generic `volume` field here.
 */
export function resolveMarketDataVolumeSequence(
  input: MarketDataRealtimeVolumeSequenceInput,
): MarketDataRealtimeVolumeSequenceResolution {
  const observedAtMs = Date.parse(input.observedAt);
  if (!Number.isFinite(observedAtMs)) {
    return {
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: input.previousState,
    };
  }

  const previousState = input.previousState;
  const cumulativeVolume = finiteNonNegative(input.cumulativeVolume);
  const volumeDelta = finiteNonNegative(input.volumeDelta);

  if (
    previousState != null &&
    (observedAtMs < previousState.lastObservedAtMs ||
      (observedAtMs === previousState.lastObservedAtMs &&
        cumulativeVolume != null &&
        previousState.lastCumulativeVolume != null &&
        cumulativeVolume < previousState.lastCumulativeVolume))
  ) {
    return {
      deltaVolume: 0,
      ignored: true,
      isDuplicate: false,
      source: "none",
      nextState: previousState,
    };
  }

  const isDuplicate =
    previousState != null &&
    observedAtMs === previousState.lastObservedAtMs &&
    cumulativeVolume === previousState.lastSampleCumulativeVolume &&
    volumeDelta === previousState.lastSampleVolumeDelta;

  let deltaVolume = 0;
  let source: MarketDataRealtimeVolumeSequenceResolution["source"] = "none";
  let lastCumulativeVolume = previousState?.lastCumulativeVolume ?? null;

  if (!isDuplicate && cumulativeVolume != null) {
    if (
      previousState?.lastCumulativeVolume != null &&
      cumulativeVolume >= previousState.lastCumulativeVolume
    ) {
      deltaVolume = cumulativeVolume - previousState.lastCumulativeVolume;
      source = "cumulative";
    } else if (volumeDelta != null) {
      deltaVolume = volumeDelta;
      source = "delta";
    }
    lastCumulativeVolume = cumulativeVolume;
  } else if (!isDuplicate && volumeDelta != null) {
    deltaVolume = volumeDelta;
    source = "delta";
    if (lastCumulativeVolume != null) {
      lastCumulativeVolume += volumeDelta;
    }
  }

  return {
    deltaVolume,
    ignored: false,
    isDuplicate,
    source,
    nextState: {
      lastCumulativeVolume,
      lastObservedAt: input.observedAt,
      lastObservedAtMs: observedAtMs,
      lastSampleCumulativeVolume: cumulativeVolume,
      lastSampleVolumeDelta: volumeDelta,
    },
  };
}
