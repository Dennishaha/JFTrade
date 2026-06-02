import type { Ref } from "vue";

import { normalizeKlinePeriod } from "../charting/kline";
import {
  createMarketDataRealtimeController,
  type MarketDataCandlesQueryResult,
  type MarketSecurityDetailsQueryResult,
  type MarketDataSnapshotQueryResult,
  normalizeMarketDataCandlesQueryResult,
  normalizeMarketDataSnapshotQueryResult,
} from "./marketDataRealtime";
import { normalizeMarketSecurityDetailsQueryResult } from "./marketSecurityNormalization";
import { createMarketDataSnapshotRefresher } from "./marketDataSnapshotRefresh";

export interface LoadMarketDataQueryOptions {
  appendOlder?: boolean;
  fromTime?: string;
  toTime?: string;
}

interface NormalizeInstrumentInput {
  market?: string | null;
  symbol?: string | null;
}

type NormalizeInstrumentParts = (
  input: NormalizeInstrumentInput,
  fallbackMarket?: string,
) => { market: string; symbol: string } | null;

interface MarketDataQueryStateRefs {
  marketDataQueryMarket: Ref<string>;
  marketDataQuerySymbol: Ref<string>;
  marketDataQueryPeriod: Ref<string>;
  marketDataQueryLimit: Ref<number>;
  marketDataSnapshot: Ref<MarketDataSnapshotQueryResult | null>;
  marketSecurityDetails: Ref<MarketSecurityDetailsQueryResult | null>;
  marketDataCandles: Ref<MarketDataCandlesQueryResult | null>;
  isLoadingMarketDataQuery: Ref<boolean>;
  marketDataQueryError: Ref<string>;
}

interface MarketDataQueryControllerOptions {
  state: MarketDataQueryStateRefs;
  fetchEnvelope: <T>(path: string) => Promise<T>;
  normalizeInstrumentParts: NormalizeInstrumentParts;
}

export interface MarketDataQueryController {
  applyTickEvent(event: unknown): void;
  loadQuery(options?: LoadMarketDataQueryOptions): Promise<void>;
}

const DEFAULT_TICK_QUERY_LIMIT = 20_000;
const DEFAULT_TICK_QUERY_LOOKBACK_MS = 15 * 60 * 1000;

export function createMarketDataQueryController(
  options: MarketDataQueryControllerOptions,
): MarketDataQueryController {
  const {
    marketDataCandles,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketSecurityDetails,
    marketDataQuerySymbol,
    marketDataSnapshot,
    isLoadingMarketDataQuery,
  } = options.state;
  const marketDataRealtime = createMarketDataRealtimeController();

  let activeMarketDataQuery: {
    key: string;
    promise: Promise<void>;
    requestId: number;
  } | null = null;
  let marketDataQueryRequestId = 0;

  function mergeMarketDataCandles(
    current: MarketDataCandlesQueryResult | null,
    next: MarketDataCandlesQueryResult,
  ): MarketDataCandlesQueryResult {
    return marketDataRealtime.mergeCandles(current, next);
  }

  function resetMarketDataRealtimeState(): void {
    marketDataRealtime.reset();
  }

  function mergeRealtimeBarStateIntoSnapshot(
    current: MarketDataSnapshotQueryResult | null,
  ): MarketDataSnapshotQueryResult | null {
    return marketDataRealtime.mergeSnapshot(current, {
      candles: marketDataCandles.value,
      period: marketDataQueryPeriod.value,
    });
  }
  const { scheduleMarketSnapshotBackgroundRefresh } =
    createMarketDataSnapshotRefresher({
      marketSecurityDetails,
    });

  function applyTickEvent(event: unknown): void {
    const currentInstrument = options.normalizeInstrumentParts(
      {
        market: marketDataQueryMarket.value,
        symbol: marketDataQuerySymbol.value,
      },
      marketDataQueryMarket.value,
    );
    const currentInstrumentId =
      currentInstrument == null
        ? ""
        : `${currentInstrument.market}.${currentInstrument.symbol}`;

    const result = marketDataRealtime.applyTickEvent({
      event,
      currentInstrumentId,
      candles: marketDataCandles.value,
      period: marketDataQueryPeriod.value,
      limit: marketDataQueryLimit.value,
    });
    if (result == null) {
      return;
    }

    marketDataSnapshot.value = result.snapshot;
    marketDataCandles.value = result.candles;
    scheduleMarketSnapshotBackgroundRefresh();
  }

  async function loadQuery(
    queryOptions: LoadMarketDataQueryOptions = {},
  ): Promise<void> {
    const parsedInstrument = options.normalizeInstrumentParts(
      {
        market: marketDataQueryMarket.value,
        symbol: marketDataQuerySymbol.value,
      },
      marketDataQueryMarket.value,
    );
    const market = parsedInstrument?.market ?? "";
    const symbol = parsedInstrument?.symbol ?? "";
    const rawPeriod = marketDataQueryPeriod.value.trim();
    const requestedLimit = Number(marketDataQueryLimit.value);

    marketDataQueryError.value = "";

    if (market === "" || symbol === "" || rawPeriod === "") {
      marketDataQueryError.value =
        "请填写市场、标的和 K 线周期。";
      return;
    }

    let period: string;
    try {
      period = normalizeKlinePeriod(rawPeriod);
    } catch (error) {
      marketDataQueryError.value =
        error instanceof Error ? error.message : "不支持的 K 线周期。";
      return;
    }

    if (!Number.isInteger(requestedLimit) || requestedLimit <= 0) {
      marketDataQueryError.value = "K 线查询条数必须是正整数。";
      return;
    }

    const effectiveLimit =
      period === "tick"
        ? Math.max(requestedLimit, DEFAULT_TICK_QUERY_LIMIT)
        : requestedLimit;
    const effectiveFromTime =
      period === "tick" &&
      queryOptions.fromTime == null &&
      queryOptions.toTime == null &&
      queryOptions.appendOlder !== true
        ? new Date(Date.now() - DEFAULT_TICK_QUERY_LOOKBACK_MS).toISOString()
        : queryOptions.fromTime;

    const queryKey = JSON.stringify({
      market,
      symbol,
      period,
      limit: effectiveLimit,
      fromTime: effectiveFromTime ?? null,
      toTime: queryOptions.toTime ?? null,
      appendOlder: queryOptions.appendOlder === true,
    });
    if (activeMarketDataQuery?.key === queryKey) {
      return activeMarketDataQuery.promise;
    }

    const requestId = marketDataQueryRequestId + 1;
    marketDataQueryRequestId = requestId;

    let promise: Promise<void>;
    promise = (async (): Promise<void> => {
      marketDataQueryMarket.value = market;
      marketDataQuerySymbol.value = symbol;
      marketDataQueryPeriod.value = period;
      marketDataQueryLimit.value = requestedLimit;
      isLoadingMarketDataQuery.value = true;

      try {
        const encodedMarket = encodeURIComponent(market);
        const encodedSymbol = encodeURIComponent(symbol);
        const candleParams = new URLSearchParams({
          period,
          limit: String(effectiveLimit),
          refresh: "true",
        });
        if (effectiveFromTime != null) {
          candleParams.set("fromTime", effectiveFromTime);
        }
        if (queryOptions.toTime != null) {
          candleParams.set("toTime", queryOptions.toTime);
        }
        const [snapshotResult, securityDetailsResult, candlesResult] = await Promise.allSettled([
          options.fetchEnvelope<MarketDataSnapshotQueryResult>(
            `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
          ),
          options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
            `/api/v1/market-data/securities/${encodedMarket}/${encodedSymbol}`,
          ),
          options.fetchEnvelope<MarketDataCandlesQueryResult>(
            `/api/v1/market-data/candles/${encodedMarket}/${encodedSymbol}?${candleParams.toString()}`,
          ),
        ]);

        marketDataSnapshot.value =
          snapshotResult.status === "fulfilled"
            ? normalizeMarketDataSnapshotQueryResult(snapshotResult.value)
            : queryOptions.appendOlder === true
              ? marketDataSnapshot.value
              : null;
        marketSecurityDetails.value =
          securityDetailsResult.status === "fulfilled"
            ? normalizeMarketSecurityDetailsQueryResult(securityDetailsResult.value)
            : queryOptions.appendOlder === true
              ? marketSecurityDetails.value
              : null;
        marketDataCandles.value =
          candlesResult.status === "fulfilled"
            ? queryOptions.appendOlder === true
              ? mergeMarketDataCandles(
                  marketDataCandles.value,
                  normalizeMarketDataCandlesQueryResult(candlesResult.value),
                )
              : normalizeMarketDataCandlesQueryResult(candlesResult.value)
            : queryOptions.appendOlder === true
              ? marketDataCandles.value
              : null;

        marketDataSnapshot.value = mergeRealtimeBarStateIntoSnapshot(
          marketDataSnapshot.value,
        );

        if (queryOptions.appendOlder !== true) {
          resetMarketDataRealtimeState();
        }

        const partialErrors = [snapshotResult, securityDetailsResult, candlesResult]
          .filter((result) => result.status === "rejected")
          .map((result) =>
            result.reason instanceof Error
              ? result.reason.message
              : "部分行情查询加载失败。",
          );
        if (partialErrors.length > 0) {
          marketDataQueryError.value = partialErrors.join(" / ");
        }
      } catch (error) {
        marketDataQueryError.value =
          error instanceof Error
            ? error.message
            : "行情查询加载失败。";
        if (queryOptions.appendOlder !== true) {
          marketDataSnapshot.value = null;
          marketSecurityDetails.value = null;
          marketDataCandles.value = null;
          resetMarketDataRealtimeState();
        }
      } finally {
        scheduleMarketSnapshotBackgroundRefresh();
        if (marketDataQueryRequestId === requestId) {
          isLoadingMarketDataQuery.value = false;
        }
        if (activeMarketDataQuery?.requestId === requestId) {
          activeMarketDataQuery = null;
        }
      }
    })();

    activeMarketDataQuery = { key: queryKey, promise, requestId };
    return promise;
  }

  return {
    applyTickEvent,
    loadQuery,
  };
}
