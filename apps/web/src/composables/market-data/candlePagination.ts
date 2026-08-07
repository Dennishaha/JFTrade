import type { MarketDataCandlesQueryResult } from "@/composables/market-data/marketDataRealtime";

export interface MarketDataCandlePaginationState {
  hasMore: boolean;
  nextBefore: string;
}

export function marketDataCandlePagination(
  result: MarketDataCandlesQueryResult,
): MarketDataCandlePaginationState {
  const nextBefore = result.pagination?.nextBefore?.trim() ?? "";
  const firstCandleAt = result.candles[0]?.at ?? "";
  const hasMore =
    result.pagination?.hasMore === true &&
    nextBefore !== "" &&
    nextBefore === firstCandleAt;
  return { hasMore, nextBefore: hasMore ? nextBefore : "" };
}

function invalidOlderCandlePage(reason: string): Error {
  return new Error(`历史 K 线分页响应无效：${reason}`);
}

function parseCandleTimestamp(value: string): number {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) {
    throw invalidOlderCandlePage("K 线时间戳无效");
  }
  return parsed;
}

export function validateOlderMarketDataCandlePage(
  result: MarketDataCandlesQueryResult,
  before: string,
  current: MarketDataCandlesQueryResult | null,
): void {
  const beforeAt = parseCandleTimestamp(before);
  const currentTimes = new Set(current?.candles.map((candle) => candle.at));
  let previousAt = Number.NEGATIVE_INFINITY;

  for (const candle of result.candles) {
    const candleAt = parseCandleTimestamp(candle.at);
    if (candleAt >= beforeAt) {
      throw invalidOlderCandlePage("K 线未严格早于分页游标");
    }
    if (candleAt <= previousAt) {
      throw invalidOlderCandlePage("K 线时间戳重复或未按时间递增");
    }
    if (currentTimes.has(candle.at)) {
      throw invalidOlderCandlePage("K 线页与已有历史重复");
    }
    previousAt = candleAt;
  }

  const pagination = result.pagination;
  if (pagination == null || typeof pagination.hasMore !== "boolean") {
    throw invalidOlderCandlePage("缺少分页元数据");
  }
  const nextBefore = pagination.nextBefore?.trim() ?? "";
  if (!pagination.hasMore) {
    if (nextBefore !== "") {
      throw invalidOlderCandlePage("历史终点页包含下一游标");
    }
    return;
  }
  if (result.candles.length === 0) {
    throw invalidOlderCandlePage("可继续页面没有 K 线");
  }
  if (nextBefore !== result.candles[0]?.at) {
    throw invalidOlderCandlePage("下一游标不等于最早 K 线");
  }
  if (parseCandleTimestamp(nextBefore) >= beforeAt) {
    throw invalidOlderCandlePage("下一游标没有向更早历史推进");
  }
}
