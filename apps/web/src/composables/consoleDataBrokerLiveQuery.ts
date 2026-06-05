import type { Ref } from "vue";

import {
  type BrokerCashFlowsResponse,
  type BrokerFillsResponse,
  type BrokerFundsResponse,
  type BrokerMarginRatiosResponse,
  type BrokerMaxTradeQuantityResponse,
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type BrokerReadFeatureKey,
  type ExecutionOrdersResponse,
  type SystemStatusResponse,
  emptyBrokerCashFlows,
  emptyBrokerFills,
  emptyBrokerFunds,
  emptyBrokerMarginRatios,
  emptyBrokerMaxTradeQuantity,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyExecutionOrders,
} from "@jftrade/ui-contracts";

import { fetchEnvelope } from "./apiClient";

interface CreateConsoleDataBrokerLiveQueryControllerOptions {
  systemStatus: Ref<SystemStatusResponse>;
  brokerCashFlows: Ref<BrokerCashFlowsResponse>;
  brokerFills: Ref<BrokerFillsResponse>;
  brokerFunds: Ref<BrokerFundsResponse>;
  brokerMarginRatios: Ref<BrokerMarginRatiosResponse>;
  brokerMaxTradeQuantity: Ref<BrokerMaxTradeQuantityResponse>;
  brokerPositions: Ref<BrokerPositionsResponse>;
  brokerOrders: Ref<BrokerOrdersResponse>;
  activeExecutionOrders: Ref<ExecutionOrdersResponse>;
  historicalExecutionOrders: Ref<ExecutionOrdersResponse>;
  isLoadingBrokerOrders: Ref<boolean>;
  isLoadingHistoricalOrders: Ref<boolean>;
  historicalOrdersError: Ref<string>;
  isLoadingBrokerFills: Ref<boolean>;
  isLoadingBrokerMarginRatios: Ref<boolean>;
  isLoadingBrokerMaxTradeQuantity: Ref<boolean>;
  resolveBrokerReadFeatureQueryRequirements: (
    feature: BrokerReadFeatureKey,
    context?: {
      market?: string | null;
      tradingEnvironment?: string | null;
    },
  ) => {
    supported: boolean;
    supportsHistory: boolean;
    requiresSymbols: boolean;
    requiresClearingDate: boolean;
    requiresPrice: boolean;
    requiresOrderIdEx: boolean;
  };
  supportsBrokerReadFeature: (
    feature: BrokerReadFeatureKey,
    context?: {
      market?: string | null;
      tradingEnvironment?: string | null;
    },
  ) => boolean;
  loadPortfolioLiveData: (input: {
    brokerId: string;
    brokerQuery: string;
  }) => Promise<void>;
}

export function createConsoleDataBrokerLiveQueryController(
  options: CreateConsoleDataBrokerLiveQueryControllerOptions,
) {
  let peripheralRequestToken = 0;
  let maxTradeQuantityRequestToken = 0;
  let historicalRequestToken = 0;

  function parseBrokerQueryContext(brokerQuery: string): {
    tradingEnvironment: string;
    accountId: string;
    market: string;
  } {
    const params = new URLSearchParams(brokerQuery);
    return {
      tradingEnvironment: params.get("tradingEnvironment")?.trim() ?? "",
      accountId: params.get("accountId")?.trim() ?? "",
      market: params.get("market")?.trim() ?? "",
    };
  }

  function executionOrdersUrl(input: {
    brokerId: string;
    brokerQuery: string;
    scope?: string;
  }): string {
    const brokerContext = parseBrokerQueryContext(input.brokerQuery);
    const params = new URLSearchParams();
    params.set("brokerId", input.brokerId);
    params.set(
      "tradingEnvironment",
      brokerContext.tradingEnvironment ||
        options.systemStatus.value.defaultTradingEnvironment,
    );
    if (brokerContext.accountId !== "") {
      params.set("accountId", brokerContext.accountId);
    }
    if (brokerContext.market !== "") {
      params.set("market", brokerContext.market);
    }
    if (input.scope) {
      params.set("scope", input.scope);
    }
    return `/api/v1/execution/orders?${params.toString()}`;
  }

  function recentHistoryRange(): { startTime: string; endTime: string } {
    const endTime = new Date();
    const startTime = new Date(endTime.getTime() - 30 * 24 * 60 * 60 * 1000);
    return {
      startTime: startTime.toISOString(),
      endTime: endTime.toISOString(),
    };
  }

  function resetPeripheralReads(): void {
    options.brokerCashFlows.value = emptyBrokerCashFlows;
    options.brokerFills.value = emptyBrokerFills;
    options.brokerMarginRatios.value = emptyBrokerMarginRatios;
    options.isLoadingBrokerFills.value = false;
    options.isLoadingBrokerMarginRatios.value = false;
  }

  function clearBrokerMaxTradeQuantity(): void {
    maxTradeQuantityRequestToken += 1;
    options.brokerMaxTradeQuantity.value = emptyBrokerMaxTradeQuantity;
    options.isLoadingBrokerMaxTradeQuantity.value = false;
  }

  async function loadBrokerCashFlows(input: {
    brokerId: string;
    brokerQuery: string;
    tradingEnvironment: string;
    market: string;
    clearingDate: string;
  }): Promise<void> {
    options.brokerCashFlows.value = emptyBrokerCashFlows;

    const requirements = options.resolveBrokerReadFeatureQueryRequirements(
      "cashFlows",
      {
        market: input.market,
        tradingEnvironment: input.tradingEnvironment,
      },
    );

    if (!requirements.supported) {
      return;
    }

    const params = new URLSearchParams(input.brokerQuery);
    if (requirements.requiresClearingDate) {
      const clearingDate = input.clearingDate.trim();
      if (clearingDate === "") {
        return;
      }
      params.set("clearingDate", clearingDate);
    }

    try {
      options.brokerCashFlows.value =
        await fetchEnvelope<BrokerCashFlowsResponse>(
          `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/cash-flows?${params.toString()}`,
        );
    } catch (error) {
      options.brokerCashFlows.value = {
        ...emptyBrokerCashFlows,
        connectivity: "disconnected",
        lastError:
          error instanceof Error
            ? error.message
            : "券商资金流水加载失败。",
      };
    }
  }

  async function loadBrokerPeripheralReads(input: {
    brokerId: string;
    brokerQuery: string;
    funds: BrokerFundsResponse;
    positions: BrokerPositionsResponse;
  }): Promise<void> {
    const requestToken = ++peripheralRequestToken;
    const queryContext = parseBrokerQueryContext(input.brokerQuery);
    const tradingEnvironment =
      input.funds.summary?.tradingEnvironment ??
      queryContext.tradingEnvironment ??
      options.systemStatus.value.defaultTradingEnvironment;
    const accountId =
      input.funds.summary?.accountId ?? queryContext.accountId ?? "0";
    const market = input.funds.summary?.market ?? queryContext.market ?? "";

    const positionSymbols = Array.from(
      new Set(
        input.positions.positions
          .map((position) => position.symbol?.trim())
          .filter((symbol): symbol is string => symbol != null && symbol !== ""),
      ),
    ).slice(0, 24);

    const fillRange = recentHistoryRange();
    const baseParams = new URLSearchParams({
      tradingEnvironment,
      accountId,
      market,
    });

    const fillsRequirements = options.resolveBrokerReadFeatureQueryRequirements(
      "fills",
      {
        market,
        tradingEnvironment,
      },
    );
    const fillsSupported = fillsRequirements.supported;
    const marginRatioRequirements =
      options.resolveBrokerReadFeatureQueryRequirements("marginRatios", {
        market,
        tradingEnvironment,
      });
    const marginRatiosSupported =
      marginRatioRequirements.supported &&
      (!marginRatioRequirements.requiresSymbols || positionSymbols.length > 0);

    options.brokerFills.value = emptyBrokerFills;
    options.brokerMarginRatios.value = emptyBrokerMarginRatios;
    options.isLoadingBrokerFills.value = fillsSupported;
    options.isLoadingBrokerMarginRatios.value = marginRatiosSupported;

    const fillsPromise = !fillsSupported
      ? Promise.resolve(emptyBrokerFills)
      : (() => {
          const fillParams = new URLSearchParams(baseParams);
          if (fillsRequirements.supportsHistory) {
            fillParams.set("scope", "history");
            fillParams.set("startTime", fillRange.startTime);
            fillParams.set("endTime", fillRange.endTime);
          }
          return fetchEnvelope<BrokerFillsResponse>(
            `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/fills?${fillParams.toString()}`,
          ).catch((error) => ({
            ...emptyBrokerFills,
            connectivity: "degraded",
            lastError:
              error instanceof Error ? error.message : "券商成交加载失败。",
          }));
        })();

    const marginRatiosPromise = !marginRatiosSupported
      ? Promise.resolve(emptyBrokerMarginRatios)
      : (() => {
          const marginParams = new URLSearchParams(baseParams);
          if (marginRatioRequirements.requiresSymbols) {
            for (const symbol of positionSymbols) {
              marginParams.append("symbol", symbol);
            }
          }
          return fetchEnvelope<BrokerMarginRatiosResponse>(
            `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/margin-ratios?${marginParams.toString()}`,
          ).catch((error) => ({
            ...emptyBrokerMarginRatios,
            connectivity: "degraded",
            lastError:
              error instanceof Error ? error.message : "融资融券参数加载失败。",
          }));
        })();

    const [fills, marginRatios] = await Promise.all([
      fillsPromise,
      marginRatiosPromise,
    ]);
    if (requestToken !== peripheralRequestToken) {
      return;
    }

    options.brokerFills.value = fills;
    options.brokerMarginRatios.value = marginRatios;
    options.isLoadingBrokerFills.value = false;
    options.isLoadingBrokerMarginRatios.value = false;
  }

  async function loadBrokerMaxTradeQuantity(input: {
    brokerId: string;
    tradingEnvironment: string;
    accountId?: string;
    market: string;
    symbol: string;
    orderType: string;
    price: number;
    orderIdEx?: string;
    adjustSideAndLimit?: number;
    session?: string;
    positionId?: number;
  }): Promise<void> {
    const requestToken = ++maxTradeQuantityRequestToken;
    const brokerId = input.brokerId.trim();
    const tradingEnvironment = input.tradingEnvironment.trim();
    const market = input.market.trim();
    const orderType = input.orderType.trim();
    const requirements = options.resolveBrokerReadFeatureQueryRequirements(
      "maxTradeQuantity",
      { market, tradingEnvironment },
    );
    const trimmedSymbol = input.symbol.trim().toUpperCase();
    const symbol = trimmedSymbol.includes(".")
      ? trimmedSymbol
      : market === ""
        ? trimmedSymbol
        : `${market}.${trimmedSymbol}`;

    options.brokerMaxTradeQuantity.value = emptyBrokerMaxTradeQuantity;
    options.isLoadingBrokerMaxTradeQuantity.value = false;

    if (
      brokerId === "" ||
      tradingEnvironment === "" ||
      market === "" ||
      symbol === "" ||
      orderType === "" ||
      !requirements.supported ||
      (requirements.requiresPrice && input.price <= 0)
    ) {
      return;
    }

    options.isLoadingBrokerMaxTradeQuantity.value = true;
    const params = new URLSearchParams({
      tradingEnvironment,
      market,
      symbol,
      orderType,
    });
    if (requirements.requiresPrice || input.price > 0) {
      params.set("price", String(input.price));
    }
    if ((input.accountId ?? "").trim() !== "") {
      params.set("accountId", input.accountId!.trim());
    }
    if ((input.orderIdEx ?? "").trim() !== "") {
      params.set("orderIdEx", input.orderIdEx!.trim());
    }
    if (input.adjustSideAndLimit != null) {
      params.set("adjustSideAndLimit", String(input.adjustSideAndLimit));
    }
    if ((input.session ?? "").trim() !== "") {
      params.set("session", input.session!.trim());
    }
    if (input.positionId != null) {
      params.set("positionId", String(input.positionId));
    }

    try {
      const response = await fetchEnvelope<BrokerMaxTradeQuantityResponse>(
        `/api/v1/brokers/${encodeURIComponent(brokerId)}/max-trade-qtys?${params.toString()}`,
      );
      if (requestToken !== maxTradeQuantityRequestToken) {
        return;
      }
      options.brokerMaxTradeQuantity.value = response;
    } catch (error) {
      if (requestToken !== maxTradeQuantityRequestToken) {
        return;
      }
      options.brokerMaxTradeQuantity.value = {
        ...emptyBrokerMaxTradeQuantity,
        connectivity: "degraded",
        lastError:
          error instanceof Error ? error.message : "最大可交易数量估算失败。",
      };
    } finally {
      if (requestToken === maxTradeQuantityRequestToken) {
        options.isLoadingBrokerMaxTradeQuantity.value = false;
      }
    }
  }

  async function loadHistoricalExecutionOrders(input: {
    brokerId: string;
    brokerQuery: string;
  }): Promise<void> {
    const requestToken = ++historicalRequestToken;
    options.isLoadingHistoricalOrders.value = true;
    options.historicalOrdersError.value = "";

    try {
      const allOrders = await fetchEnvelope<ExecutionOrdersResponse>(
        executionOrdersUrl(input),
      );
      if (requestToken !== historicalRequestToken) {
        return;
      }

      // Split results: active orders go to activeExecutionOrders, historical go to historicalExecutionOrders
      options.historicalExecutionOrders.value = {
        orders: allOrders.orders,
      };

      // Also update active orders from the full response (in case new active orders appeared)
      options.activeExecutionOrders.value = {
        orders: allOrders.orders,
      };
    } catch (error) {
      if (requestToken !== historicalRequestToken) {
        return;
      }
      options.historicalOrdersError.value =
        error instanceof Error ? error.message : "历史订单加载失败。";
      options.historicalExecutionOrders.value = emptyExecutionOrders;
    } finally {
      if (requestToken === historicalRequestToken) {
        options.isLoadingHistoricalOrders.value = false;
      }
    }
  }

  async function loadBrokerLiveData(input: {
    brokerId: string;
    brokerQuery: string;
    futuBrokerReadsPaused: boolean;
  }): Promise<void> {
    let funds: BrokerFundsResponse = emptyBrokerFunds;
    let positions: BrokerPositionsResponse = emptyBrokerPositions;
    options.isLoadingBrokerOrders.value = true;

    try {
      const liveBrokerDataPromise: Promise<
        readonly [
          BrokerFundsResponse,
          BrokerPositionsResponse,
          BrokerOrdersResponse,
        ]
      > = input.futuBrokerReadsPaused
        ? Promise.resolve([
            emptyBrokerFunds,
            emptyBrokerPositions,
            emptyBrokerOrders,
          ] as const)
        : Promise.all([
            fetchEnvelope<BrokerFundsResponse>(
              `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/funds?${input.brokerQuery}`,
            ),
            fetchEnvelope<BrokerPositionsResponse>(
              `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/positions?${input.brokerQuery}`,
            ),
            fetchEnvelope<BrokerOrdersResponse>(
              `/api/v1/brokers/${encodeURIComponent(input.brokerId)}/orders?${input.brokerQuery}`,
            ),
          ]).then(([nextFunds, nextPositions, orders]) => [nextFunds, nextPositions, orders] as const);

      // Phase 1: Load active orders only (fast, no history sync)
      const [[nextFunds, nextPositions, orders], , activeOrders] =
        await Promise.all([
        liveBrokerDataPromise,
        options.loadPortfolioLiveData({
          brokerId: input.brokerId,
          brokerQuery: input.brokerQuery,
        }),
        fetchEnvelope<ExecutionOrdersResponse>(executionOrdersUrl({ ...input, scope: "active" })),
      ]);

      funds = nextFunds;
      positions = nextPositions;
      options.brokerFunds.value = funds;
      options.brokerPositions.value = positions;
      options.brokerOrders.value = orders;
      options.activeExecutionOrders.value = activeOrders;
      options.isLoadingBrokerOrders.value = false;

      // Phase 2: Load historical orders in the background (slower)
      // Historical orders are now lazy-loaded — only triggered when user visits history tab
      // via loadHistoricalExecutionOrders(). We no longer auto-load here.
    } catch {
      options.isLoadingBrokerOrders.value = false;
    }

    if (input.futuBrokerReadsPaused) {
      resetPeripheralReads();
      return;
    }

    await Promise.all([
      loadBrokerCashFlows({
        brokerId: input.brokerId,
        brokerQuery: input.brokerQuery,
        tradingEnvironment:
          funds.summary?.tradingEnvironment ??
          options.systemStatus.value.defaultTradingEnvironment,
        market: funds.summary?.market ?? parseBrokerQueryContext(input.brokerQuery).market,
        clearingDate: funds.checkedAt.slice(0, 10),
      }),
      loadBrokerPeripheralReads({
        brokerId: input.brokerId,
        brokerQuery: input.brokerQuery,
        funds,
        positions,
      }),
    ]);
  }

  return {
    clearBrokerMaxTradeQuantity,
    loadBrokerLiveData,
    loadBrokerMaxTradeQuantity,
    loadHistoricalExecutionOrders,
  };
}
