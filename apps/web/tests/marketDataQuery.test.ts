import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
  MarketSecurityDetailsQueryResult,
} from "../src/composables/marketDataRealtime";

import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const mocks = vi.hoisted(() => ({
  fallbackRefresh: vi.fn(),
  scheduleMarketSnapshotBackgroundRefresh: vi.fn(),
  stopMarketSnapshotBackgroundRefresh: vi.fn(),
}));

vi.mock("../src/composables/marketDataSnapshotRefresh", () => ({
  createMarketDataSnapshotRefresher: (options: {
    fallbackRefresh: (...args: unknown[]) => Promise<unknown>;
  }) => {
    mocks.fallbackRefresh.mockImplementation(options.fallbackRefresh);
    return {
      scheduleMarketSnapshotBackgroundRefresh:
        mocks.scheduleMarketSnapshotBackgroundRefresh,
      stopMarketSnapshotBackgroundRefresh:
        mocks.stopMarketSnapshotBackgroundRefresh,
    };
  },
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

function createController(resolveBrokerId: () => string = () => "") {
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
    isLoadingOlderMarketData: ref(false),
    hasMoreMarketDataHistory: ref(false),
    marketDataNextBefore: ref(""),
    marketDataOlderError: ref(""),
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
    resolveBrokerId,
  });

  return {
    controller,
    state,
    fetchEnvelope,
  };
}

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

afterEach(() => {
  mocks.fallbackRefresh.mockReset();
  mocks.scheduleMarketSnapshotBackgroundRefresh.mockReset();
  mocks.stopMarketSnapshotBackgroundRefresh.mockReset();
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
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenCalledTimes(2);
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenLastCalledWith({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
    });
  });

  it("deduplicates identical tick queries and leaves background refresh to the live channel", async () => {
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
      );

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

    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetchEnvelope).toHaveBeenCalledTimes(3);
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenLastCalledWith({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
    });
  });

  it("loads only older candles, preserves accumulated history, and updates matching tick events", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-03T12:30:00.000Z"));
    const { controller, state, fetchEnvelope } = createController();

    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 2;
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    state.marketDataSnapshot.value = createSnapshotResult("US", "AAPL", 200);
    state.marketSecurityDetails.value = createSecurityDetailsResult("US", "AAPL");
    state.hasMoreMarketDataHistory.value = true;
    state.marketDataNextBefore.value = "2026-07-03T12:29:59.000Z";
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "1m", [
      {
        period: "1m",
        open: 200.5,
        high: 200.5,
        low: 200.5,
        close: 200.5,
        volume: 0,
        at: "2026-07-03T12:29:59.000Z",
      },
    ]);

    const olderPage = createCandlesResult("US", "AAPL", "1m", [
      {
        period: "1m",
        open: 199.8,
        high: 199.8,
        low: 199.8,
        close: 199.8,
        volume: 0,
        at: "2026-07-03T12:29:58.000Z",
      },
    ]);
    olderPage.pagination = {
      hasMore: true,
      nextBefore: "2026-07-03T12:29:58.000Z",
    };
    fetchEnvelope.mockResolvedValueOnce(olderPage);

    await controller.loadQuery({
      appendOlder: true,
      before: "2026-07-03T12:29:59.000Z",
    });

    expect(fetchEnvelope).toHaveBeenCalledTimes(1);
    expect(fetchEnvelope.mock.calls[0]?.[0]).toContain(
      "before=2026-07-03T12%3A29%3A59.000Z",
    );
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
    expect(state.marketDataQueryError.value).toBe("");
    expect(state.marketDataNextBefore.value).toBe(
      "2026-07-03T12:29:58.000Z",
    );

    fetchEnvelope.mockRejectedValueOnce(new Error("older timeout"));
    await controller.loadQuery({ appendOlder: true });
    expect(state.marketDataOlderError.value).toBe("older timeout");

    state.marketDataQueryPeriod.value = "tick";
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
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenCalledTimes(2);
  });

  it("does not page Tick history and discards an older page after the target changes", async () => {
    const { controller, state, fetchEnvelope } = createController();
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "tick";
    state.marketDataQueryLimit.value = 2;
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    state.hasMoreMarketDataHistory.value = true;
    state.marketDataNextBefore.value = "2026-07-03T12:00:00.000Z";

    await controller.loadQuery({ appendOlder: true });
    expect(fetchEnvelope).not.toHaveBeenCalled();

    state.marketDataQueryPeriod.value = "1m";
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "1m", []);
    state.hasMoreMarketDataHistory.value = false;
    await controller.loadQuery({ appendOlder: true });
    expect(fetchEnvelope).not.toHaveBeenCalled();

    state.hasMoreMarketDataHistory.value = true;
    const older = createDeferred<MarketDataCandlesQueryResult>();
    fetchEnvelope.mockReturnValueOnce(older.promise);
    const pending = controller.loadQuery({ appendOlder: true });
    await controller.loadQuery({ appendOlder: true });
    expect(fetchEnvelope).toHaveBeenCalledTimes(1);

    controller.selectInstrument({
      market: "HK",
      symbol: "00700",
      period: "1m",
    });
    older.resolve(
      createCandlesResult("US", "AAPL", "1m", [
        {
          period: "1m",
          open: 200,
          high: 201,
          low: 199,
          close: 200.5,
          volume: 100,
          at: "2026-07-03T11:59:00.000Z",
        },
      ]),
    );
    await pending;

    expect(state.activeMarketDataInstrumentId.value).toBe("HK.00700");
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.isLoadingOlderMarketData.value).toBe(false);
  });

  it("ignores invalid selections and prevents an old rejected request from overwriting a newer symbol", async () => {
    const { controller, state, fetchEnvelope } = createController();
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 2;

    controller.selectInstrument({ market: " ", symbol: " ", period: "5m" });
    expect(state.marketDataQueryMarket.value).toBe("US");
    expect(state.marketDataQueryPeriod.value).toBe("1m");

    const snapshot = createDeferred<MarketDataSnapshotQueryResult>();
    const security = createDeferred<MarketSecurityDetailsQueryResult>();
    const candles = createDeferred<MarketDataCandlesQueryResult>();
    fetchEnvelope
      .mockReturnValueOnce(snapshot.promise)
      .mockReturnValueOnce(security.promise)
      .mockReturnValueOnce(candles.promise);

    const oldRequest = controller.loadQuery();
    controller.selectInstrument({ market: "HK", symbol: "00700", period: "1m" });
    snapshot.reject(new Error("old US snapshot failed"));
    security.resolve(createSecurityDetailsResult("US", "AAPL"));
    candles.resolve(createCandlesResult("US", "AAPL", "1m", []));
    await oldRequest;

    expect(state.activeMarketDataInstrumentId.value).toBe("HK.00700");
    expect(state.marketDataSnapshot.value).toBeNull();
    expect(state.marketDataQueryError.value).toBe("");
  });

  it("discards a disconnected-channel fallback response after the query target changes", async () => {
    vi.useFakeTimers();
    const { controller, state, fetchEnvelope } = createController();
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 2;
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    state.marketDataCandles.value = createCandlesResult("US", "AAPL", "1m", []);

    fetchEnvelope
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 200))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"))
      .mockResolvedValueOnce(createCandlesResult("US", "AAPL", "1m", []));

    await controller.loadQuery();
    expect(fetchEnvelope).toHaveBeenCalledTimes(3);

    const backgroundSnapshot = createDeferred<MarketDataSnapshotQueryResult>();
    const backgroundSecurity = createDeferred<MarketSecurityDetailsQueryResult>();
    fetchEnvelope
      .mockReturnValueOnce(backgroundSnapshot.promise)
      .mockReturnValueOnce(backgroundSecurity.promise);
    const fallback = mocks.fallbackRefresh({
      market: "US",
      symbol: "AAPL",
      instrumentId: "US.AAPL",
    });

    state.activeMarketDataInstrumentId.value = "HK.00700";
    backgroundSnapshot.resolve(createSnapshotResult("US", "AAPL", 999));
    backgroundSecurity.resolve(createSecurityDetailsResult("US", "AAPL"));
    await fallback;

    expect(state.marketDataSnapshot.value?.snapshot?.price).not.toBe(999);
    expect(state.activeMarketDataInstrumentId.value).toBe("HK.00700");
    controller.dispose();
    expect(mocks.stopMarketSnapshotBackgroundRefresh).toHaveBeenCalledOnce();
  });

  it("refreshes snapshots and security details through the disconnected-channel fallback", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-03T12:30:00.000Z"));
    const { state, fetchEnvelope } = createController(() => "futu");
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    fetchEnvelope
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 205))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"));

    await expect(
      mocks.fallbackRefresh({
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      }),
    ).resolves.toEqual({});

    expect(fetchEnvelope.mock.calls.map(([path]) => path)).toEqual([
      "/api/v1/market-data/snapshots/US/AAPL?refresh=true&brokerId=futu",
      "/api/v1/market-data/securities/US/AAPL?brokerId=futu",
    ]);
    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(205);
    expect(state.marketSecurityDetails.value?.request.instrumentId).toBe(
      "US.AAPL",
    );
    expect(state.lastDataRefreshedAt.value).toBe(1783081800000);
  });

  it("propagates the longest valid fallback retry delay", async () => {
    const { state, fetchEnvelope } = createController();
    state.activeMarketDataInstrumentId.value = "US.AAPL";
    fetchEnvelope
      .mockRejectedValueOnce({ retryAfterMs: 2_500 })
      .mockRejectedValueOnce({ retryAfterMs: Number.NaN });

    await expect(
      mocks.fallbackRefresh({
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      }),
    ).resolves.toEqual({ retryAfterMs: 2_500 });
    expect(state.marketDataSnapshot.value).toBeNull();
    expect(state.marketSecurityDetails.value).toBeNull();
  });

  it("updates a matching live tick without scheduling an incomplete refresh target", () => {
    const { controller, state } = createController();
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.activeMarketDataInstrumentId.value = "";

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
      },
      source: "test",
    });

    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(203);
    expect(mocks.scheduleMarketSnapshotBackgroundRefresh).toHaveBeenCalledWith(
      null,
    );
  });

  it("skips fallback reads when the provider changes before dispatch", async () => {
    let providerRead = 0;
    const { state, fetchEnvelope } = createController(() =>
      providerRead++ === 0 ? "alpha" : "beta",
    );
    state.activeMarketDataInstrumentId.value = "US.AAPL";

    await expect(
      mocks.fallbackRefresh({
        market: "US",
        symbol: "AAPL",
        instrumentId: "US.AAPL",
      }),
    ).resolves.toEqual({});
    expect(fetchEnvelope).not.toHaveBeenCalled();
  });

  it("does not surface a synchronous setup failure after the user has already switched instruments", async () => {
    const { controller, state, fetchEnvelope } = createController();
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 2;

    fetchEnvelope.mockImplementation(() => {
      controller.selectInstrument({ market: "HK", symbol: "00700", period: "5m" });
      throw new Error("obsolete request setup failed");
    });

    await controller.loadQuery();

    expect(state.activeMarketDataInstrumentId.value).toBe("HK.00700");
    expect(state.marketDataQueryError.value).toBe("");
    expect(state.isLoadingMarketDataQuery.value).toBe(false);
  });

  it("routes all workspace reads through the selected broker and invalidates old provider data", async () => {
    let brokerId = "alpha";
    const { controller, state, fetchEnvelope } = createController(
      () => brokerId,
    );
    state.marketDataQueryMarket.value = "US";
    state.marketDataQuerySymbol.value = "AAPL";
    state.marketDataQueryPeriod.value = "1m";
    state.marketDataQueryLimit.value = 2;
    fetchEnvelope
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 200))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"))
      .mockResolvedValueOnce(createCandlesResult("US", "AAPL", "1m", []))
      .mockResolvedValueOnce(createSnapshotResult("US", "AAPL", 201))
      .mockResolvedValueOnce(createSecurityDetailsResult("US", "AAPL"))
      .mockResolvedValueOnce(createCandlesResult("US", "AAPL", "1m", []));

    await controller.loadQuery();
    expect(fetchEnvelope.mock.calls.slice(0, 3).map(([path]) => path)).toEqual(
      expect.arrayContaining([
        "/api/v1/market-data/snapshots/US/AAPL?refresh=true&brokerId=alpha",
        "/api/v1/market-data/securities/US/AAPL?brokerId=alpha",
        expect.stringContaining(
          "/api/v1/market-data/candles/US/AAPL?period=1m",
        ),
      ]),
    );
    expect(
      String(fetchEnvelope.mock.calls[2]?.[0]),
    ).toContain("brokerId=alpha");

    brokerId = "beta";
    controller.invalidateProviderSelection();
    expect(state.marketDataSnapshot.value).toBeNull();
    expect(state.marketSecurityDetails.value).toBeNull();
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.lastDataRefreshedAt.value).toBe(0);
    await controller.loadQuery();
    expect(
      fetchEnvelope.mock.calls.slice(3).every(([path]) =>
        String(path).includes("brokerId=beta"),
      ),
    ).toBe(true);
    expect(state.marketDataSnapshot.value?.snapshot?.price).toBe(201);
  });
});
