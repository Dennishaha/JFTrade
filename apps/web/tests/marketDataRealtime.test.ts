import { describe, expect, it } from "vitest";

import type {
  MarketDataCandlesQueryResult,
  MarketDataTickLiveEvent,
} from "../src/composables/marketDataRealtime";
import {
  createMarketDataRealtimeController,
  normalizeMarketDataCandlesQueryResult,
  normalizeMarketDataSnapshotQueryResult,
  normalizeMarketDataTickLiveEvent,
} from "../src/composables/marketDataRealtime";

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
    cumulativeVolume: 1_500,
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
  it("normalizes numeric upstream payloads without inventing values for invalid fields", () => {
    const snapshot = normalizeMarketDataSnapshotQueryResult({
      request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      snapshot: {
        price: " 201.5 ",
        bid: "",
        ask: "not-a-number",
        volume: "1500",
        extended: {
          preMarket: { price: "199.25", volume: "12" },
          afterMarket: null,
          overnight: "invalid",
        },
      } as never,
    });
    const candles = normalizeMarketDataCandlesQueryResult({
      ...buildCandles("1m", []),
      candles: [
        { period: "1m", at: "2026-07-16T00:00:00Z", open: "200", high: "201.5", low: "199.5", close: "201", volume: "50" },
        null as never,
        { period: "1m", at: "2026-07-16T00:01:00Z", open: "bad", high: 202, low: 200, close: 201, volume: Number.POSITIVE_INFINITY },
      ],
    });

    expect(snapshot.snapshot).toMatchObject({ price: 201.5, bid: "", ask: "not-a-number", volume: 1500 });
    expect(snapshot.snapshot?.extended?.preMarket).toMatchObject({ price: 199.25, volume: 12 });
    expect(snapshot.snapshot?.extended?.overnight).toBeNull();
    expect(candles.candles).toHaveLength(2);
    expect(candles.candles[0]).toMatchObject({ open: 200, high: 201.5, low: 199.5, close: 201, volume: 50 });
    expect(candles.candles[1]).toMatchObject({ open: "bad", high: 202, volume: Number.POSITIVE_INFINITY });
  });

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

  it("retains nullable fields and rejects tick envelopes without a usable snapshot", () => {
    const normalized = normalizeMarketDataSnapshotQueryResult({
      request: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      snapshot: {
        price: 201.5,
        bid: 201.4,
        ask: 201.6,
        volume: 1_500,
        turnover: 300_000,
        at: "2026-07-03T12:00:30.000Z",
        barOpen: null,
        barHigh: false as never,
      },
      meta: {
        instrumentId: "US.AAPL",
        source: "test",
        resolvedAt: "2026-07-03T12:00:30.000Z",
        fromCache: false,
      },
    });
    expect(normalized.snapshot).toMatchObject({ barOpen: null, barHigh: false });

    expect(
      normalizeMarketDataSnapshotQueryResult({
        ...normalized,
        snapshot: null,
      }).snapshot,
    ).toBeNull();
    expect(
      normalizeMarketDataTickLiveEvent({
        ...buildTickEvent(),
        snapshot: null,
      }),
    ).toBeNull();
    expect(
      normalizeMarketDataTickLiveEvent({
        ...buildTickEvent(),
        cumulativeVolume: " 1500 ",
        volumeDelta: "12",
      }),
    ).toMatchObject({ cumulativeVolume: 1500, volumeDelta: 12 });
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

  it("reads only the tail candle on the normal realtime update path", () => {
    const controller = createMarketDataRealtimeController();
    let historicalAtReads = 0;
    const candles = buildCandles("1m", [
      {
        period: "1m",
        get at() {
          historicalAtReads += 1;
          return "2026-07-03T11:59:00.000Z";
        },
        open: 199,
        high: 200,
        low: 198.5,
        close: 199.5,
        volume: 900,
      },
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

    expect(result?.candles?.candles.at(-1)?.close).toBe(201.5);
    expect(historicalAtReads).toBe(0);
  });

  it("preserves comparison prices when an incremental tick omits snapshot context", () => {
    const controller = createMarketDataRealtimeController();
    const currentSnapshot = normalizeMarketDataSnapshotQueryResult({
      request: { market: "SZ", symbol: "000858", instrumentId: "SZ.000858" },
      snapshot: {
        price: 74.1,
        bid: 74.09,
        ask: 74.11,
        previousClosePrice: 72.76,
        lastClosePrice: 72.76,
        volume: 9_000_000,
        turnover: 660_000_000,
        at: "2026-07-20T01:44:00.000Z",
        session: "unknown",
      },
      meta: {
        instrumentId: "SZ.000858",
        source: "snapshot",
        resolvedAt: "2026-07-20T01:44:00.000Z",
        fromCache: false,
      },
    });

    const result = controller.applyTickEvent({
      event: buildTickEvent({
        instrument: {
          market: "SZ",
          symbol: "000858",
          instrumentId: "SZ.000858",
        },
        snapshot: {
          price: 74.2,
          bid: 74.19,
          ask: 74.21,
          previousClosePrice: null,
          lastClosePrice: null,
          volume: 9_100_000,
          turnover: 670_000_000,
          at: "2026-07-20T01:44:01.000Z",
          session: "unknown",
        },
      }),
      currentInstrumentId: "SZ.000858",
      currentSnapshot,
      candles: null,
      period: "1m",
      limit: 50,
    });

    expect(result?.snapshot.snapshot).toMatchObject({
      price: 74.2,
      previousClosePrice: 72.76,
      lastClosePrice: 72.76,
    });
  });

  it("never treats the legacy snapshot volume as a bar-volume sample", () => {
    const controller = createMarketDataRealtimeController();
    const event = buildTickEvent({ cumulativeVolume: undefined });
    event.snapshot.volume = 9_999_999;

    const result = controller.applyTickEvent({
      event,
      currentInstrumentId: "US.AAPL",
      candles: null,
      period: "1m",
      limit: 50,
    });

    expect(result?.snapshot.snapshot.barVolume).toBe(0);
    expect(result?.candles?.candles.at(-1)?.volume).toBe(0);
  });

  it("carries an explicit cumulative sequence across buckets and ignores old events", () => {
    const controller = createMarketDataRealtimeController();
    const firstEvent = buildTickEvent({ cumulativeVolume: 1_500 });
    const first = controller.applyTickEvent({
      event: firstEvent,
      currentInstrumentId: "US.AAPL",
      candles: null,
      period: "1m",
      limit: 50,
    });
    const secondAt = "2026-07-03T12:01:05.000Z";
    const secondEvent = buildTickEvent({
      at: secondAt,
      cumulativeVolume: 1_620,
      snapshot: {
        ...firstEvent.snapshot,
        price: 202,
        at: secondAt,
        observedAt: secondAt,
        volume: 1_620,
      },
    });
    const second = controller.applyTickEvent({
      event: secondEvent,
      currentInstrumentId: "US.AAPL",
      currentSnapshot: first?.snapshot,
      candles: first?.candles ?? null,
      period: "1m",
      limit: 50,
    });

    expect(second?.candles?.candles.at(-1)).toMatchObject({
      at: "2026-07-03T12:01:00.000Z",
      volume: 120,
    });

    const oldAt = "2026-07-03T12:00:50.000Z";
    expect(
      controller.applyTickEvent({
        event: buildTickEvent({
          at: oldAt,
          cumulativeVolume: 1_580,
          snapshot: {
            ...firstEvent.snapshot,
            at: oldAt,
            observedAt: oldAt,
            volume: 1_580,
          },
        }),
        currentInstrumentId: "US.AAPL",
        currentSnapshot: second?.snapshot,
        candles: second?.candles ?? null,
        period: "1m",
        limit: 50,
      }),
    ).toBeNull();
  });
});
