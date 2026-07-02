import { describe, expect, it } from "vitest";

import type {
  MarketDataCandlesQueryResult,
  MarketDataTickLiveEvent,
} from "../src/composables/marketDataRealtime";
import { createMarketDataRealtimeController } from "../src/composables/marketDataRealtime";

function buildTickEvent(
  overrides: Partial<MarketDataTickLiveEvent> = {},
): MarketDataTickLiveEvent {
  const instrument = {
    market: "US",
    symbol: "AAPL",
    instrumentId: "US.AAPL",
    ...overrides.instrument,
  };
  const snapshot = {
    price: 201.5,
    bid: 201.4,
    ask: 201.6,
    previousClosePrice: 200,
    volume: 1_500,
    turnover: 300_000,
    at: "2026-07-03T12:00:30.000Z",
    observedAt: "2026-07-03T12:00:31.000Z",
    session: "regular",
    ...overrides.snapshot,
  };

  return {
    type: "market-data.tick",
    at: "2026-07-03T12:00:31.000Z",
    brokerId: "futu",
    instrument,
    snapshot,
    source: "live",
    ...overrides,
  };
}

function buildCandles(
  period: string,
  candles: MarketDataCandlesQueryResult["candles"],
): MarketDataCandlesQueryResult {
  return {
    request: {
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      period,
      limit: candles.length,
    },
    candles,
    totalReturned: candles.length,
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

describe("marketDataRealtime", () => {
  it("ignores invalid or foreign tick events", () => {
    const controller = createMarketDataRealtimeController();

    expect(
      controller.applyTickEvent({
        event: { type: "not-a-tick" },
        currentInstrumentId: "US.AAPL",
        candles: null,
        period: "1m",
        limit: 50,
      }),
    ).toBeNull();

    expect(
      controller.applyTickEvent({
        event: buildTickEvent({
          instrument: {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
          },
        }),
        currentInstrumentId: "US.AAPL",
        candles: null,
        period: "1m",
        limit: 50,
      }),
    ).toBeNull();
  });

  it("keeps candles unchanged when the realtime bucket cannot be resolved", () => {
    const controller = createMarketDataRealtimeController();
    const candles = buildCandles("1m", [
      {
        period: "1m",
        at: "2026-07-03T12:00:00.000Z",
        open: 200,
        high: 201,
        low: 199.5,
        close: 200.5,
        volume: 1_000,
      },
    ]);

    const result = controller.applyTickEvent({
      event: buildTickEvent({
        at: "invalid",
        snapshot: {
          at: "invalid",
          observedAt: "",
          price: 202,
          bid: 201.9,
          ask: 202.1,
          previousClosePrice: 200,
          volume: 1_800,
          turnover: 320_000,
          session: "regular",
        },
      }),
      currentInstrumentId: "US.AAPL",
      candles,
      period: "1m",
      limit: 50,
    });

    expect(result?.candles).toBe(candles);
    expect(result?.snapshot.snapshot.barOpen).toBeNull();
    expect(result?.snapshot.snapshot.barHigh).toBeNull();
    expect(result?.snapshot.snapshot.barLow).toBeNull();
    expect(result?.snapshot.snapshot.barVolume).toBeNull();
  });

  it("reuses the current candle bucket and updates realtime bar fields", () => {
    const controller = createMarketDataRealtimeController();
    const candles = buildCandles("1m", [
      {
        period: "1m",
        at: "2026-07-03T12:00:00.000Z",
        open: 200,
        high: 201,
        low: 199.5,
        close: 200.5,
        volume: 1_000,
      },
    ]);

    const result = controller.applyTickEvent({
      event: buildTickEvent(),
      currentInstrumentId: "US.AAPL",
      candles,
      period: "1m",
      limit: 50,
    });

    expect(result?.snapshot.snapshot.barOpen).toBe(200);
    expect(result?.snapshot.snapshot.barHigh).toBe(201.5);
    expect(result?.snapshot.snapshot.barLow).toBe(199.5);
    expect(result?.snapshot.snapshot.barVolume).toBeGreaterThan(0);
    expect(result?.candles?.candles[0]).toMatchObject({
      at: "2026-07-03T12:00:00.000Z",
      high: 201.5,
      low: 199.5,
      close: 201.5,
    });
  });
});
