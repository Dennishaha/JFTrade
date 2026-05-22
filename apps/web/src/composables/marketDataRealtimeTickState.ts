export interface MarketDataRealtimeTickVolumeState {
  instrumentId: string;
  lastCumulativeVolume: number;
}

export interface MarketDataRealtimeTickVolumeResolution {
  deltaVolume: number;
  nextState: MarketDataRealtimeTickVolumeState | null;
}

export function resolveMarketDataTickVolumeUpdate(
  previousState: MarketDataRealtimeTickVolumeState | null,
  instrumentId: string,
  cumulativeVolume: number,
): MarketDataRealtimeTickVolumeResolution {
  if (!Number.isFinite(cumulativeVolume) || cumulativeVolume < 0) {
    return {
      deltaVolume: 0,
      nextState: previousState,
    };
  }

  const deltaVolume =
    previousState == null ||
    previousState.instrumentId !== instrumentId ||
    cumulativeVolume < previousState.lastCumulativeVolume
      ? 0
      : Math.max(0, cumulativeVolume - previousState.lastCumulativeVolume);

  return {
    deltaVolume,
    nextState: {
      instrumentId,
      lastCumulativeVolume: cumulativeVolume,
    },
  };
}