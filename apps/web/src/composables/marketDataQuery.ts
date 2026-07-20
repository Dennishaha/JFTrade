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
import {
  createMarketDataSnapshotRefresher,
  type MarketSnapshotRefreshTarget,
} from "./marketDataSnapshotRefresh";
import { withBrokerProvider } from "./brokerProviderSelection";

export interface LoadMarketDataQueryOptions {
  appendOlder?: boolean;
  fromTime?: string;
  toTime?: string;
  /** When true, skip clearing existing data before loading (useful for visibility recovery). */
  preserveExisting?: boolean;
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
  lastDataRefreshedAt: Ref<number>;
}

interface MarketDataQueryControllerOptions {
  state: MarketDataQueryStateRefs;
  fetchEnvelope: <T>(path: string) => Promise<T>;
  normalizeInstrumentParts: NormalizeInstrumentParts;
  resolveBrokerId?: () => string;
}

export interface MarketDataQueryController {
  applyTickEvent(event: unknown): void;
  dispose(): void;
  invalidateProviderSelection(): void;
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
    lastDataRefreshedAt,
  } = options.state;
  const marketDataRealtime = createMarketDataRealtimeController();

  let activeMarketDataQuery: {
    key: string;
    promise: Promise<void>;
    requestId: number;
  } | null = null;
  let marketDataQueryRequestId = 0;
  let loadedBrokerId = "";

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
  const {
    scheduleMarketSnapshotBackgroundRefresh,
    stopMarketSnapshotBackgroundRefresh,
  } =
    createMarketDataSnapshotRefresher({
      marketSecurityDetails,
      fallbackIntervalMs: 3_000,
      fallbackRefresh: refreshMarketDataFallback,
    });

  function normalizeInstrumentId(market: string, symbol: string): string {
    return `${market.trim().toUpperCase()}.${symbol.trim().toUpperCase()}`;
  }

  function clearCurrentMarketDataResults(): void {
    scheduleMarketSnapshotBackgroundRefresh(null);
    marketDataSnapshot.value = null;
    marketSecurityDetails.value = null;
    marketDataCandles.value = null;
    resetMarketDataRealtimeState();
  }

  function clearCurrentMarketDataCandles(): void {
    marketDataCandles.value = null;
    resetMarketDataRealtimeState();
  }

  function invalidateProviderSelection(): void {
    marketDataQueryRequestId += 1;
    activeMarketDataQuery = null;
    isLoadingMarketDataQuery.value = false;
    isMarketDataSwitching.value = true;
    marketDataQueryError.value = "";
    lastDataRefreshedAt.value = 0;
    clearCurrentMarketDataResults();
    loadedBrokerId =
      options.resolveBrokerId?.().trim().toLowerCase() ?? "";
    isMarketDataSwitching.value = false;
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
      // Any in-flight request now belongs to a different query target. Its
      // finally block intentionally cannot clear this flag, so release the
      // stale loading state as part of the target switch.
      isLoadingMarketDataQuery.value = false;
    }

    if (instrumentChanged) {
      isMarketDataSwitching.value = true;
      lastDataRefreshedAt.value = 0;
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
      currentSnapshot: marketDataSnapshot.value,
      candles: marketDataCandles.value,
      period: marketDataQueryPeriod.value,
      limit: marketDataQueryLimit.value,
    });
    if (result == null) {
      return;
    }

    marketDataSnapshot.value = result.snapshot;
    marketDataCandles.value = result.candles;
    lastDataRefreshedAt.value = Date.now();
    scheduleMarketSnapshotBackgroundRefresh(currentRefreshTarget());
  }

  function currentRefreshTarget(): MarketSnapshotRefreshTarget | null {
    const market = marketDataQueryMarket.value.trim().toUpperCase();
    const symbol = marketDataQuerySymbol.value.trim().toUpperCase();
    const instrumentId = activeMarketDataInstrumentId.value.trim().toUpperCase();
    if (market === "" || symbol === "" || instrumentId === "") {
      return null;
    }
    return { market, symbol, instrumentId };
  }

  async function refreshMarketDataFallback(
    target: MarketSnapshotRefreshTarget,
  ): Promise<{ retryAfterMs?: number }> {
    const brokerId = options.resolveBrokerId?.().trim().toLowerCase() ?? "";
    if (
      activeMarketDataInstrumentId.value !== target.instrumentId ||
      (options.resolveBrokerId?.().trim().toLowerCase() ?? "") !== brokerId
    ) {
      return {};
    }
    const encodedMarket = encodeURIComponent(target.market);
    const encodedSymbol = encodeURIComponent(target.symbol);
    const [snapshotResult, securityDetailsResult] = await Promise.allSettled([
      options.fetchEnvelope<MarketDataSnapshotQueryResult>(
        withBrokerProvider(
          `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
          brokerId,
        ),
      ),
      options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
        withBrokerProvider(
          `/api/v1/market-data/securities/${encodedMarket}/${encodedSymbol}`,
          brokerId,
        ),
      ),
    ]);
    if (
      activeMarketDataInstrumentId.value !== target.instrumentId ||
      (options.resolveBrokerId?.().trim().toLowerCase() ?? "") !== brokerId
    ) {
      return {};
    }

    let refreshed = false;
    if (snapshotResult.status === "fulfilled") {
      marketDataSnapshot.value = mergeRealtimeBarStateIntoSnapshot(
        normalizeMarketDataSnapshotQueryResult(snapshotResult.value),
      );
      refreshed = true;
    }
    if (securityDetailsResult.status === "fulfilled") {
      marketSecurityDetails.value = normalizeMarketSecurityDetailsQueryResult(
        securityDetailsResult.value,
      );
      refreshed = true;
    }
    if (refreshed) {
      lastDataRefreshedAt.value = Date.now();
    }
    const retryAfterMs = [snapshotResult, securityDetailsResult].reduce(
      (current, result) =>
        result.status === "rejected"
          ? Math.max(current, retryAfterFromError(result.reason))
          : current,
      0,
    );
    return retryAfterMs > 0 ? { retryAfterMs } : {};
  }

  function retryAfterFromError(error: unknown): number {
    const value = (error as { retryAfterMs?: unknown } | null)?.retryAfterMs;
    return typeof value === "number" && Number.isFinite(value) && value > 0
      ? value
      : 0;
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
    const brokerId =
      options.resolveBrokerId?.().trim().toLowerCase() ?? "";

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
      brokerId,
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
    const requestProviderChanged = loadedBrokerId !== brokerId;

    function isCurrentRequest(): boolean {
      return (
        marketDataQueryRequestId === requestId &&
        activeMarketDataInstrumentId.value === requestInstrumentId &&
        marketDataQueryPeriod.value === period &&
        (options.resolveBrokerId?.().trim().toLowerCase() ?? "") === brokerId
      );
    }

    let promise: Promise<void>;
    promise = (async (): Promise<void> => {
      marketDataQueryMarket.value = market;
      marketDataQuerySymbol.value = symbol;
      marketDataQueryPeriod.value = period;
      marketDataQueryLimit.value = requestedLimit;
      activeMarketDataInstrumentId.value = requestInstrumentId;
      if (queryOptions.appendOlder !== true && queryOptions.preserveExisting !== true) {
        if (requestInstrumentChanged || requestProviderChanged) {
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
            withBrokerProvider(
              `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
              brokerId,
            ),
          );
        const securityDetailsQuery =
          options.fetchEnvelope<MarketSecurityDetailsQueryResult>(
            withBrokerProvider(
              `/api/v1/market-data/securities/${encodedMarket}/${encodedSymbol}`,
              brokerId,
            ),
          );
        const candlesQuery =
          options.fetchEnvelope<MarketDataCandlesQueryResult>(
            withBrokerProvider(
              `/api/v1/market-data/candles/${encodedMarket}/${encodedSymbol}?${candleParams.toString()}`,
              brokerId,
            ),
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
          scheduleMarketSnapshotBackgroundRefresh({
            market, symbol, instrumentId: requestInstrumentId,
          });
          lastDataRefreshedAt.value = Date.now();
          loadedBrokerId = brokerId;
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
    dispose: stopMarketSnapshotBackgroundRefresh,
    invalidateProviderSelection,
    loadQuery,
    selectInstrument,
  };
}
