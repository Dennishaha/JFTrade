import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
  MarketSecurityDetailsQueryResult,
} from "../src/composables/marketDataRealtime";

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const mocks = vi.hoisted(() => ({
  scheduleMarketSnapshotBackgroundRefresh: vi.fn(),
}));

vi.mock("../src/composables/marketDataSnapshotRefresh", () => ({
  createMarketDataSnapshotRefresher: () => ({
    scheduleMarketSnapshotBackgroundRefresh:
      mocks.scheduleMarketSnapshotBackgroundRefresh,
  }),
}));

import { createMarketDataQueryController } from "../src/composables/marketDataQuery";

function createSnapshotResult(
  market: string,
  symbol: string,
  price: number,
): MarketDataSnapshotQueryResult {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    snapshot: {
      price,
      bid: price - 0.1,
      ask: price + 0.1,
      previousClosePrice: price - 1,
      volume: 1000,
      turnover: 100000,
      observedAt: "2026-07-03T12:00:00.000Z",
      at: "2026-07-03T12:00:00.000Z",
      session: "regular",
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createSecurityDetailsResult(
  market: string,
  symbol: string,
): MarketSecurityDetailsQueryResult {
  const instrumentId = `${market}.${symbol}`;
  return {
    request: {
      market,
      symbol,
      instrumentId,
    },
    security: {
      instrumentId,
      market,
      symbol,
      securityId: 1,
      name: symbol,
      securityType: "Eqty",
      exchangeType: `${market}_EXCHANGE`,
      listTime: "2024-01-01",
      listTimestamp: 1704067200,
      delisting: false,
      lotSize: 100,
      isSuspend: false,
      priceSpread: 0.01,
      updateTime: "2026-07-03 12:00:00",
      updateTimestamp: 1783070400,
      highPrice: 101,
      openPrice: 99,
      lowPrice: 98,
      lastClosePrice: 100,
      currentPrice: 100.5,
      volume: 1000,
      turnover: 100000,
      turnoverRate: 1,
      askPrice: 100.6,
      bidPrice: 100.4,
      askVolume: 100,
      bidVolume: 100,
      amplitude: 0.02,
      averagePrice: 100.2,
      bidAskRatio: 1,
      volumeRatio: 1,
      highest52WeeksPrice: 120,
      lowest52WeeksPrice: 80,
      highestHistoryPrice: 150,
      lowestHistoryPrice: 50,
      closePrice5Minute: 100.1,
      highPrecisionVolume: 1000,
      highPrecisionAskVol: 100,
      highPrecisionBidVol: 100,
    },
    meta: {
      instrumentId,
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createCandlesResult(
  market: string,
  symbol: string,
  period: string,
  candles: MarketDataCandlesQueryResult["candles"],
): MarketDataCandlesQueryResult {
  return {
    request: {
      instrument: {
        market,
        symbol,
        instrumentId: `${market}.${symbol}`,
      },
      period,
      limit: candles.length,
    },
    candles,
    totalReturned: candles.length,
    meta: {
      instrumentId: `${market}.${symbol}`,
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createController() {
  const state = {
    marketDataQueryMarket: ref(""),
    marketDataQuerySymbol: ref(""),
    marketDataQueryPeriod: ref(""),
    marketDataQueryLimit: ref(2),
    activeMarketDataInstrumentId: ref(""),
    isMarketDataSwitching: ref(false),
    marketDataSnapshot: ref<MarketDataSnapshotQueryResult | null>(null),
    marketSecurityDetails: ref<MarketSecurityDetailsQueryResult | null>(null),
    marketDataCandles: ref<MarketDataCandlesQueryResult | null>(null),
    isLoadingMarketDataQuery: ref(false),
    marketDataQueryError: ref(""),
    lastDataRefreshedAt: ref(0),
  };

  const fetchEnvelope = vi.fn();
  const controller = createMarketDataQueryController({
    state,
    fetchEnvelope,
    normalizeInstrumentParts: (input, fallbackMarket) => {
      const market = (input.market ?? fallbackMarket ?? "").trim().toUpperCase();
      const symbol = (input.symbol ?? "").trim().toUpperCase();
      return market === "" || symbol === "" ? null : { market, symbol };
    },
  });

  return {
    controller,
    state,
    fetchEnvelope,
  };
}

afterEach(() => {
  mocks.scheduleMarketSnapshotBackgroundRefresh.mockReset();
  vi.useRealTimers();
});

describe("createMarketDataQueryController", () => {
  it("resets all market data when the instrument changes and only clears candles on period changes", () => {
    const { controller, state } = createController();

    state.marketDataSnapshot.value = createSnapshotResult("HK", "00700", 380);
    state.marketSecurityDetails.value = createSecurityDetailsResult("HK", "00700");
    state.marketDataCandles.value = createCandlesResult("HK", "00700", "1m", [
      {
        period: "1m",
        open: 379,
        high: 381,
        low: 378.5,
        close: 380,
        volume: 1000,
        at: "2026-07-03T11:59:00.000Z",
      },
    ]);
    state.lastDataRefreshedAt.value = 123;

    controller.selectInstrument({
      market: " us ",
      symbol: " aapl ",
      period: "5M",
    });

    expect(state.marketDataQueryMarket.value).toBe("US");
    expect(state.marketDataQuerySymbol.value).toBe("AAPL");
    expect(state.marketDataQueryPeriod.value).toBe("5m");
    expect(state.activeMarketDataInstrumentId.value).toBe("US.AAPL");
    expect(state.marketDataSnapshot.value).toBeNull();
    expect(state.marketSecurityDetails.value).toBeNull();
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.lastDataRefreshedAt.value).toBe(0);
    expect(state.isMarketDataSwitching.value).toBe(false);

    state.marketDataSnapshot.value = createSnapshotResult("US", "AAPL", 200);
    state.marketSecurityDetails.value = createSecurityDetailsResult("US", "AAPL");
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "5m", [
      {
        period: "5m",
        open: 198,
        high: 200,
        low: 197.5,
        close: 199.5,
        volume: 2000,
        at: "2026-07-03T12:00:00.000Z",
      },
    ]);

    controller.selectInstrument({
      market: "US",
      symbol: "AAPL",
      period: "1h",
    });

    expect(state.marketDataSnapshot.value?.request.instrumentId).toBe("US.AAPL");
    expect(state.marketSecurityDetails.value?.request.instrumentId).toBe(
      "US.AAPL",
    );
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.marketDataQueryPeriod.value).toBe("1h");
  });

  it("validates query inputs and clears non-append requests on synchronous fetch failures", async () => {
    vi.useFakeTimers();
    const { controller, state, fetchEnvelope } = createController();

    await controller.loadQuery();
    expect(state.marketDataQueryError.value).toBe("请填写市场、标的和 K 线周期。");

    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "bad-period";
    state.marketDataQueryLimit.value = 2;
    await controller.loadQuery();
    expect(state.marketDataQueryError.value).toContain("不支持的 K 线周期");

    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 0;
    await controller.loadQuery();
    expect(state.marketDataQueryError.value).toBe("K 线查询条数必须是正整数。");

    state.marketDataQueryLimit.value = 2;
    state.marketDataSnapshot.value = createSnapshotResult("US", "AAPL", 200);
    state.marketSecurityDetails.value = createSecurityDetailsResult("US", "AAPL");
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "1m", [
      {
        period: "1m",
        open: 199,
        high: 200,
        low: 198.8,
        close: 199.5,
        volume: 1000,
        at: "2026-07-03T11:59:00.000Z",
      },
    ]);

    fetchEnvelope.mockImplementation(() => {
      throw new Error("snapshot endpoint unavailable");
    });

    await controller.loadQuery();

    expect(state.marketDataQueryError.value).toBe("snapshot endpoint unavailable");
    expect(state.marketDataSnapshot.value).toBeNull();
    expect(state.marketSecurityDetails.value).toBeNull();
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.isLoadingMarketDataQuery.value).toBe(false);
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenCalledTimes(1);
  });

  it("deduplicates identical tick queries, applies tick defaults, and runs the background refresh timer", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-03T12:00:00.000Z"));
    const { controller, state, fetchEnvelope } = createController();

    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "tick";
    state.marketDataQueryLimit.value = 10;

    fetchEnvelope
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 200))
      .mockRejectedValueOnce(new Error("security lagging"))
      .mockResolvedValueOnce(
        createCandlesResult("US", "AAPL", "tick", [
          {
            period: "tick",
            open: 200,
            high: 200,
            low: 200,
            close: 200,
            volume: 0,
            at: "2026-07-03T11:59:59.000Z",
          },
        ]),
      )
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 201))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"));

    const first = controller.loadQuery();
    const second = controller.loadQuery();

    await Promise.all([first, second]);

    expect(fetchEnvelope).toHaveBeenCalledTimes(3);
    expect(fetchEnvelope.mock.calls[2]?.[0]).toContain(
      "limit=20000",
    );
    expect(fetchEnvelope.mock.calls[2]?.[0]).toContain(
      "fromTime=2026-07-03T11%3A45%3A00.000Z",
    );
    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(200);
    expect(state.marketSecurityDetails.value).toBeNull();
    expect(state.marketDataCandles.value?.candles).toHaveLength(1);
    expect(state.marketDataQueryError.value).toBe("security lagging");
    expect(state.isLoadingMarketDataQuery.value).toBe(false);

    await vi.advanceTimersByTimeAsync(1000);

    expect(fetchEnvelope).toHaveBeenCalledTimes(5);
    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(201);
    expect(state.marketSecurityDetails.value?.request.instrumentId).toBe(
      "US.AAPL",
    );
  });

  it("preserves existing results when appending older history and updates matching tick events", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-03T12:30:00.000Z"));
    const { controller, state, fetchEnvelope } = createController();

    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "tick";
    state.marketDataQueryLimit.value = 2;
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    state.marketDataSnapshot.value = createSnapshotResult("US", "AAPL", 200);
    state.marketSecurityDetails.value = createSecurityDetailsResult("US", "AAPL");
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "tick", [
      {
        period: "tick",
        open: 200.5,
        high: 200.5,
        low: 200.5,
        close: 200.5,
        volume: 0,
        at: "2026-07-03T12:29:59.000Z",
      },
    ]);

    fetchEnvelope
      .mockRejectedValueOnce(new Error("snapshot timeout"))
      .mockRejectedValueOnce(new Error("security timeout"))
      .mockResolvedValueOnce(
        createCandlesResult("US", "AAPL", "tick", [
          {
            period: "tick",
            open: 199.8,
            high: 199.8,
            low: 199.8,
            close: 199.8,
            volume: 0,
            at: "2026-07-03T12:29:58.000Z",
          },
        ]),
      )
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 202))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"));

    await controller.loadQuery({
      appendOlder: true,
      fromTime: "2026-07-03T12:00:00.000Z",
      toTime: "2026-07-03T12:10:00.000Z",
    });

    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(200);
    expect(state.marketSecurityDetails.value?.request.instrumentId).toBe(
      "US.AAPL",
    );
    expect(state.marketDataCandles.value?.candles.map((candle) => candle.at)).toEqual(
      [
        "2026-07-03T12:29:58.000Z",
        "2026-07-03T12:29:59.000Z",
      ],
    );
    expect(state.marketDataQueryError.value).toBe(
      "snapshot timeout / security timeout",
    );

    controller.applyTickEvent({
      type: "market-data.tick",
      at: "2026-07-03T12:30:01.000Z",
      brokerId: "futu",
      instrument: {
        market: "HK",
        symbol: "00700",
        instrumentId: "HK.00700",
      },
      snapshot: {
        price: 380,
        bid: 379.9,
        ask: 380.1,
        volume: 1000,
        turnover: 380000,
        at: "2026-07-03T12:30:01.000Z",
      },
      source: "test",
    });

    expect(state.marketDataSnapshot.value?.request.instrumentId).toBe("US.AAPL");

    controller.applyTickEvent({
      type: "market-data.tick",
      at: "2026-07-03T12:30:02.000Z",
      brokerId: "futu",
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      snapshot: {
        price: 202,
        bid: 201.9,
        ask: 202.1,
        volume: 1015,
        turnover: 384000,
        at: "2026-07-03T12:30:02.000Z",
        session: "regular",
      },
      source: "test",
    });

    controller.applyTickEvent({
      type: "market-data.tick",
      at: "2026-07-03T12:30:03.000Z",
      brokerId: "futu",
      instrument: {
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      },
      snapshot: {
        price: 203,
        bid: 202.9,
        ask: 203.1,
        volume: 1030,
        turnover: 387000,
        at: "2026-07-03T12:30:03.000Z",
        session: "regular",
      },
      source: "test",
    });

    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(203);
    expect(state.marketDataCandles.value?.candles.at(-1)).toMatchObject({
      period: "tick",
      close: 203,
      volume: 15,
      at: "2026-07-03T12:30:03.000Z",
      session: "regular",
    });
    expect(state.lastDataRefreshedAt.value).toBe(1783081800000);
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenCalledTimes(3);
  });
});
