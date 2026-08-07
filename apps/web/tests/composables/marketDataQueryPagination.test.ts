import type {
  MarketDataCandlesQueryResult,
  MarketDataSnapshotQueryResult,
  MarketSecurityDetailsQueryResult,
} from "@/composables/market-data/marketDataRealtime";

import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

vi.mock("@/composables/market-data/marketDataSnapshotRefresh", () => ({
  createMarketDataSnapshotRefresher: () => ({
    scheduleMarketSnapshotBackgroundRefresh: vi.fn(),
    stopMarketSnapshotBackgroundRefresh: vi.fn(),
  }),
}));

import { createMarketDataQueryController } from "@/composables/market-data/marketDataQuery";

function candlePage(at: string[]): MarketDataCandlesQueryResult {
  return {
    request: {
      instrument: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      period: "1m",
      limit: at.length,
    },
    candles: at.map((candleAt, index) => ({
      period: "1m",
      open: 199 + index,
      high: 200 + index,
      low: 198 + index,
      close: 199.5 + index,
      volume: 100,
      at: candleAt,
    })),
    totalReturned: at.length,
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-07-03T12:00:00.000Z",
      fromCache: false,
    },
  };
}

function createController() {
  const state = {
    marketDataQueryMarket: ref("US"),
    marketDataQuerySymbol: ref("AAPL"),
    marketDataQueryPeriod: ref("1m"),
    marketDataQueryLimit: ref(2),
    activeMarketDataInstrumentId: ref("US.AAPL"),
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
    requestSnapshot: (path) => fetchEnvelope(path),
    requestSecurityDetails: (path) => fetchEnvelope(path),
    requestCandles: (path) => fetchEnvelope(path),
    normalizeInstrumentParts: (input, fallbackMarket) => {
      const market = (input.market ?? fallbackMarket ?? "").trim().toUpperCase();
      const symbol = (input.symbol ?? "").trim().toUpperCase();
      return market === "" || symbol === "" ? null : { market, symbol };
    },
  });

  return { controller, fetchEnvelope, state };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("market-data initial candle pagination", () => {
  it("keeps the existing pagination cursor during a preserved query refresh", async () => {
    const { controller, fetchEnvelope, state } = createController();
    const existingBefore = "2026-07-03T12:28:00.000Z";
    state.hasMoreMarketDataHistory.value = true;
    state.marketDataNextBefore.value = existingBefore;
    state.marketDataCandles.value = candlePage([existingBefore]);
    const refreshed = candlePage(["2026-07-03T12:29:00.000Z"]);
    refreshed.pagination = {
      hasMore: true,
      nextBefore: "2026-07-03T12:29:00.000Z",
    };
    fetchEnvelope
      .mockRejectedValueOnce(new Error("snapshot unavailable"))
      .mockRejectedValueOnce(new Error("details unavailable"))
      .mockResolvedValueOnce(refreshed);

    await controller.loadQuery({ preserveExisting: true });

    expect(state.marketDataCandles.value?.candles.map((candle) => candle.at)).toEqual([
      existingBefore,
      "2026-07-03T12:29:00.000Z",
    ]);
    expect(state.hasMoreMarketDataHistory.value).toBe(true);
    expect(state.marketDataNextBefore.value).toBe(existingBefore);
  });

  it("drops a superseded initial candle page before it can update the chart", async () => {
    const { controller, fetchEnvelope, state } = createController();
    const delayedCandles = deferred<MarketDataCandlesQueryResult>();
    fetchEnvelope
      .mockRejectedValueOnce(new Error("snapshot unavailable"))
      .mockRejectedValueOnce(new Error("details unavailable"))
      .mockReturnValueOnce(delayedCandles.promise);

    const pending = controller.loadQuery();
    await vi.waitFor(() => expect(fetchEnvelope).toHaveBeenCalledTimes(3));

    controller.selectInstrument({ market: "HK", symbol: "00700", period: "1m" });
    delayedCandles.resolve(candlePage(["2026-07-03T12:29:00.000Z"]));
    await pending;

    expect(state.activeMarketDataInstrumentId.value).toBe("HK.00700");
    expect(state.marketDataCandles.value).toBeNull();
    expect(state.marketDataNextBefore.value).toBe("");
  });
});
