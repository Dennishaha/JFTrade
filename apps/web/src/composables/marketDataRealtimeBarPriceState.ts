export interface MarketDataRealtimeBarPriceState {
  instrumentId: string;
  period: string;
  bucketAt: string;
  open: number;
  high: number;
  low: number;
}

export interface MarketDataRealtimeBarPriceCandleSeed {
  open: number;
  high: number;
  low: number;
}

export interface MarketDataRealtimeBarPriceUpdateInput {
  previousState: MarketDataRealtimeBarPriceState | null;
  instrumentId: string;
  period: string;
  bucketAt: string | null;
  price: number;
  existingCandle: MarketDataRealtimeBarPriceCandleSeed | null;
  lastHistoricalClose: number | null;
}

export interface MarketDataRealtimeBarPriceResolution {
  nextState: MarketDataRealtimeBarPriceState | null;
  shouldFinalizePreviousBucket: boolean;
}

export function resolveMarketDataBarPriceUpdate(
  input: MarketDataRealtimeBarPriceUpdateInput,
): MarketDataRealtimeBarPriceResolution {
  if (input.bucketAt == null) {
    return {
      nextState: null,
      shouldFinalizePreviousBucket: false,
    };
  }

  const previousState = input.previousState;
  const shouldResetState =
    previousState == null ||
    previousState.instrumentId !== input.instrumentId ||
    previousState.period !== input.period ||
    previousState.bucketAt !== input.bucketAt;
  const shouldFinalizePreviousBucket =
    previousState != null &&
    previousState.instrumentId === input.instrumentId &&
    previousState.period === input.period &&
    previousState.bucketAt !== input.bucketAt;

  if (shouldResetState) {
    const baseOpen =
      input.existingCandle?.open ?? input.lastHistoricalClose ?? input.price;
    return {
      nextState: {
        instrumentId: input.instrumentId,
        period: input.period,
        bucketAt: input.bucketAt,
        open: baseOpen,
        high: Math.max(input.existingCandle?.high ?? baseOpen, input.price),
        low: Math.min(input.existingCandle?.low ?? baseOpen, input.price),
      },
      shouldFinalizePreviousBucket,
    };
  }

  return {
    nextState: {
      ...previousState,
      open: input.existingCandle?.open ?? previousState.open,
      high: Math.max(
        previousState.high,
        input.existingCandle?.high ?? previousState.high,
        input.price,
      ),
      low: Math.min(
        previousState.low,
        input.existingCandle?.low ?? previousState.low,
        input.price,
      ),
    },
    shouldFinalizePreviousBucket,
  };
}