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
  activeMarketDataInstrumentId: Ref<string>;
  isMarketDataSwitching: Ref<boolean>;
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
  selectInstrument(input: {
    market: string;
    symbol: string;
    period?: string;
  }): void;
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
    activeMarketDataInstrumentId,
    isMarketDataSwitching,
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
  let marketDataBackgroundRefreshTimer: ReturnType<typeof setTimeout> | null =
    null;

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

  function normalizeInstrumentId(market: string, symbol: string): string {
    return `${market.trim().toUpperCase()}.${symbol.trim().toUpperCase()}`;
  }

  function clearCurrentMarketDataResults(): void {
    if (marketDataBackgroundRefreshTimer != null) {
      clearTimeout(marketDataBackgroundRefreshTimer);
      marketDataBackgroundRefreshTimer = null;
    }
    marketDataSnapshot.value = null;
    marketSecurityDetails.value = null;
    marketDataCandles.value = null;
    resetMarketDataRealtimeState();
  }

  function clearCurrentMarketDataCandles(): void {
    marketDataCandles.value = null;
    resetMarketDataRealtimeState();
  }

  function selectInstrument(input: {
    market: string;
    symbol: string;
    period?: string;
  }): void {
    const parsedInstrument = options.normalizeInstrumentParts(
      {
        market: input.market,
        symbol: input.symbol,
      },
      input.market,
    );
    if (parsedInstrument == null) {
      return;
    }

    const nextPeriod =
      input.period == null ? marketDataQueryPeriod.value : normalizeKlinePeriod(input.period);
    const nextInstrumentId = normalizeInstrumentId(
      parsedInstrument.market,
      parsedInstrument.symbol,
    );
    const instrumentChanged =
      activeMarketDataInstrumentId.value !== nextInstrumentId;
    const periodChanged = marketDataQueryPeriod.value !== nextPeriod;

    marketDataQueryMarket.value = parsedInstrument.market;
    marketDataQuerySymbol.value = parsedInstrument.symbol;
    marketDataQueryPeriod.value = nextPeriod;
    activeMarketDataInstrumentId.value = nextInstrumentId;

    if (instrumentChanged || periodChanged) {
      marketDataQueryRequestId += 1;
      activeMarketDataQuery = null;
    }

    if (instrumentChanged) {
      isMarketDataSwitching.value = true;
      clearCurrentMarketDataResults();
      isMarketDataSwitching.value = false;
    } else if (periodChanged) {
      clearCurrentMarketDataCandles();
    }
  }

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
        : normalizeInstrumentId(currentInstrument.market, currentInstrument.symbol);

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

  function scheduleMarketDataBackgroundRefresh(input: {
    market: string;
    symbol: string;
    instrumentId: string;
    period: string;
  }): void {
    if (marketDataBackgroundRefreshTimer != null) {
      return;
    }

    marketDataBackgroundRefreshTimer = setTimeout(() => {
      marketDataBackgroundRefreshTimer = null;
      void (async () => {
        if (
          activeMarketDataInstrumentId.value !== input.instrumentId ||
          marketDataQueryPeriod.value !== input.period
        ) {
          return;
        }

        const encodedMarket = encodeURIComponent(input.market);
        const encodedSymbol = encodeURIComponent(input.symbol);
        const [snapshotResult, securityDetailsResult] = await Promise.allSettled([
          options.fetchEnvelope<MarketDataSnapshotQueryResult>(
            `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
          ),
          options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
            `/api/v1/market-data/securities/${encodedMarket}/${encodedSymbol}`,
          ),
        ]);

        if (
          activeMarketDataInstrumentId.value !== input.instrumentId ||
          marketDataQueryPeriod.value !== input.period
        ) {
          return;
        }

        if (snapshotResult.status === "fulfilled") {
          marketDataSnapshot.value = mergeRealtimeBarStateIntoSnapshot(
            normalizeMarketDataSnapshotQueryResult(snapshotResult.value),
          );
        }
        if (securityDetailsResult.status === "fulfilled") {
          marketSecurityDetails.value = normalizeMarketSecurityDetailsQueryResult(
            securityDetailsResult.value,
          );
        }

        scheduleMarketSnapshotBackgroundRefresh();
      })();
    }, 1000);
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
    const requestInstrumentId = normalizeInstrumentId(market, symbol);
    const requestInstrumentChanged =
      activeMarketDataInstrumentId.value !== requestInstrumentId;
    const requestPeriodChanged = marketDataQueryPeriod.value !== period;

    function isCurrentRequest(): boolean {
      return (
        marketDataQueryRequestId === requestId &&
        activeMarketDataInstrumentId.value === requestInstrumentId &&
        marketDataQueryPeriod.value === period
      );
    }

    let promise: Promise<void>;
    promise = (async (): Promise<void> => {
      marketDataQueryMarket.value = market;
      marketDataQuerySymbol.value = symbol;
      marketDataQueryPeriod.value = period;
      marketDataQueryLimit.value = requestedLimit;
      activeMarketDataInstrumentId.value = requestInstrumentId;
      if (queryOptions.appendOlder !== true) {
        if (requestInstrumentChanged) {
          isMarketDataSwitching.value = true;
          clearCurrentMarketDataResults();
          isMarketDataSwitching.value = false;
        } else if (requestPeriodChanged || marketDataCandles.value != null) {
          clearCurrentMarketDataCandles();
        }
      }
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
        const earlyErrors: string[] = [];
        const recordEarlyError = (error: unknown): void => {
          if (!isCurrentRequest()) {
            return;
          }
          earlyErrors.push(
            error instanceof Error
              ? error.message
              : "部分行情查询加载失败。",
          );
          marketDataQueryError.value = earlyErrors.join(" / ");
        };
        const snapshotQuery =
          options.fetchEnvelope<MarketDataSnapshotQueryResult>(
            `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
          );
        const securityDetailsQuery =
          options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
            `/api/v1/market-data/securities/${encodedMarket}/${encodedSymbol}`,
          );
        const candlesQuery =
          options.fetchEnvelope<MarketDataCandlesQueryResult>(
            `/api/v1/market-data/candles/${encodedMarket}/${encodedSymbol}?${candleParams.toString()}`,
          );

        void snapshotQuery
          .then((result) => {
            if (!isCurrentRequest()) {
              return;
            }
            marketDataSnapshot.value = mergeRealtimeBarStateIntoSnapshot(
              normalizeMarketDataSnapshotQueryResult(result),
            );
          })
          .catch(recordEarlyError);
        void securityDetailsQuery
          .then((result) => {
            if (!isCurrentRequest()) {
              return;
            }
            marketSecurityDetails.value =
              normalizeMarketSecurityDetailsQueryResult(result);
          })
          .catch(recordEarlyError);
        void candlesQuery
          .then((result) => {
            if (!isCurrentRequest()) {
              return;
            }
            if (queryOptions.appendOlder === true) {
              return;
            }
            const normalized = normalizeMarketDataCandlesQueryResult(result);
            marketDataCandles.value = normalized;
            marketDataSnapshot.value = mergeRealtimeBarStateIntoSnapshot(
              marketDataSnapshot.value,
            );
            resetMarketDataRealtimeState();
          })
          .catch(recordEarlyError);

        const [snapshotResult, securityDetailsResult, candlesResult] = await Promise.allSettled([
          snapshotQuery,
          securityDetailsQuery,
          candlesQuery,
        ]);

        if (!isCurrentRequest()) {
          return;
        }

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
        if (!isCurrentRequest()) {
          return;
        }
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
        if (isCurrentRequest()) {
          scheduleMarketSnapshotBackgroundRefresh();
          scheduleMarketDataBackgroundRefresh({
            market,
            symbol,
            instrumentId: requestInstrumentId,
            period,
          });
          isLoadingMarketDataQuery.value = false;
          isMarketDataSwitching.value = false;
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
    selectInstrument,
  };
}
