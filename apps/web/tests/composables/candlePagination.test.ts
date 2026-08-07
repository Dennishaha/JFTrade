import type { MarketDataCandlesQueryResult } from "@/composables/market-data/marketDataRealtime";

import { describe, expect, it } from "vitest";

import {
  marketDataCandlePagination,
  validateOlderMarketDataCandlePage,
} from "@/composables/market-data/candlePagination";

function candlePage(
  at: string[],
  pagination?: MarketDataCandlesQueryResult["pagination"],
): MarketDataCandlesQueryResult {
  return {
    request: {
      instrument: { market: "US", symbol: "AAPL", instrumentId: "US.AAPL" },
      period: "1m",
      limit: at.length,
    },
    candles: at.map((candleAt, index) => ({
      period: "1m",
      open: 100 + index,
      high: 101 + index,
      low: 99 + index,
      close: 100.5 + index,
      volume: 100,
      at: candleAt,
    })),
    totalReturned: at.length,
    meta: {
      instrumentId: "US.AAPL",
      source: "test",
      resolvedAt: "2026-08-07T00:00:00.000Z",
      fromCache: false,
    },
    pagination,
  };
}

describe("market-data candle pagination", () => {
  it("accepts only verified provider pagination cursors", () => {
    const first = "2026-08-06T12:08:00.000Z";
    const result = candlePage([first, "2026-08-06T12:09:00.000Z"]);

    expect(marketDataCandlePagination(result)).toEqual({
      hasMore: false,
      nextBefore: "",
    });

    result.pagination = { hasMore: true, nextBefore: ` ${first} ` };
    expect(marketDataCandlePagination(result)).toEqual({
      hasMore: true,
      nextBefore: first,
    });

    result.pagination = { hasMore: true, nextBefore: "" };
    expect(marketDataCandlePagination(result)).toEqual({
      hasMore: false,
      nextBefore: "",
    });

    result.pagination = {
      hasMore: true,
      nextBefore: "2026-08-06T12:07:00.000Z",
    };
    expect(marketDataCandlePagination(result)).toEqual({
      hasMore: false,
      nextBefore: "",
    });
  });

  it("rejects malformed or non-progressing older pages without weakening terminal pages", () => {
    const before = "2026-08-06T12:10:00.000Z";
    const valid = candlePage(
      ["2026-08-06T12:08:00.000Z", "2026-08-06T12:09:00.000Z"],
      { hasMore: true, nextBefore: "2026-08-06T12:08:00.000Z" },
    );

    expect(() => validateOlderMarketDataCandlePage(valid, before, null)).not.toThrow();

    const invalidCases: Array<[MarketDataCandlesQueryResult, string]> = [
      [
        candlePage(["not-a-timestamp"], { hasMore: false }),
        "K 线时间戳无效",
      ],
      [
        candlePage(
          ["2026-08-06T12:09:00.000Z", "2026-08-06T12:08:00.000Z"],
          { hasMore: false },
        ),
        "K 线时间戳重复或未按时间递增",
      ],
      [
        candlePage([], {
          hasMore: false,
          nextBefore: "2026-08-06T12:08:00.000Z",
        }),
        "历史终点页包含下一游标",
      ],
      [candlePage([], { hasMore: true, nextBefore: "" }), "可继续页面没有 K 线"],
      [
        candlePage(["2026-08-06T12:08:00.000Z"], {
          hasMore: true,
          nextBefore: "2026-08-06T12:07:00.000Z",
        }),
        "下一游标不等于最早 K 线",
      ],
    ];

    for (const [result, reason] of invalidCases) {
      expect(() => validateOlderMarketDataCandlePage(result, before, null)).toThrow(
        reason,
      );
    }
  });
});
