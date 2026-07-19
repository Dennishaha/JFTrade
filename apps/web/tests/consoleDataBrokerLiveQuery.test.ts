import type {
  BrokerCashFlowsResponse,
  BrokerFillsResponse,
  BrokerFundsResponse,
  BrokerMarginRatiosResponse,
  BrokerMaxTradeQuantityResponse,
  BrokerOrdersResponse,
  BrokerPositionsResponse,
  BrokerReadFeatureKey,
  ExecutionOrdersResponse,
} from "@/contracts";

import {
  emptyBrokerCashFlows,
  emptyBrokerFills,
  emptyBrokerFunds,
  emptyBrokerMarginRatios,
  emptyBrokerMaxTradeQuantity,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyExecutionOrders,
  emptySystemStatus,
} from "@/contracts";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
}));

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
  apiGetPath: (_template: string, path: string, ...rest: unknown[]) =>
    mocks.fetchEnvelope(path, ...rest),
}));

import { createConsoleDataBrokerLiveQueryController } from "../src/composables/consoleDataBrokerLiveQuery";

interface FeatureRequirements {
  supported: boolean;
  supportsHistory: boolean;
  requiresSymbols: boolean;
  requiresClearingDate: boolean;
  requiresPrice: boolean;
  requiresOrderIdEx: boolean;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function createFeatureRequirements(
  overrides: Partial<
    Record<BrokerReadFeatureKey, Partial<FeatureRequirements>>
  > = {},
): Record<BrokerReadFeatureKey, FeatureRequirements> {
  const base: FeatureRequirements = {
    supported: true,
    supportsHistory: false,
    requiresSymbols: false,
    requiresClearingDate: false,
    requiresPrice: false,
    requiresOrderIdEx: false,
  };

  return {
    funds: { ...base, ...overrides.funds },
    positions: { ...base, ...overrides.positions },
    orders: { ...base, supportsHistory: true, ...overrides.orders },
    fills: { ...base, supportsHistory: true, ...overrides.fills },
    cashFlows: { ...base, requiresClearingDate: true, ...overrides.cashFlows },
    orderFees: { ...base, requiresOrderIdEx: true, ...overrides.orderFees },
    marginRatios: {
      ...base,
      requiresSymbols: true,
      ...overrides.marginRatios,
    },
    maxTradeQuantity: {
      ...base,
      requiresPrice: true,
      ...overrides.maxTradeQuantity,
    },
    orderBook: { ...base, ...overrides.orderBook },
  };
}

function createFunds(
  input: Partial<BrokerFundsResponse> & {
    accountId?: string;
    tradingEnvironment?: string;
    market?: string;
    checkedAt?: string;
  } = {},
): BrokerFundsResponse {
  return {
    ...emptyBrokerFunds,
    checkedAt: input.checkedAt ?? "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    summary: {
      accountId: input.accountId ?? "ACC-1",
      tradingEnvironment: input.tradingEnvironment ?? "REAL",
      market: input.market ?? "US",
      currency: "USD",
      totalAssets: 100000,
      securitiesAssets: 75000,
      fundAssets: 0,
      bondAssets: 0,
      cash: 25000,
      marketValue: 75000,
      longMarketValue: 75000,
      shortMarketValue: 0,
      purchasingPower: 50000,
      shortSellingPower: 0,
      netCashPower: 25000,
      availableWithdrawalCash: 24000,
      maxWithdrawal: 24000,
      availableFunds: 24000,
      frozenCash: 0,
      pendingAsset: 0,
      unrealizedPnl: null,
      realizedPnl: null,
      initialMargin: null,
      maintenanceMargin: null,
      marginCallMargin: null,
      riskStatus: null,
      debtCash: null,
      isPdt: null,
      pdtSeq: null,
      beginningDTBP: null,
      remainingDTBP: null,
      dtCallAmount: null,
      dtStatus: null,
      exposureLevel: null,
      exposureLimit: null,
      usedLimit: null,
      remainingLimit: null,
    },
    currencyBalances: [],
    marketAssets: [],
    ...input,
  };
}

function createPositions(symbols: string[]): BrokerPositionsResponse {
  return {
    ...emptyBrokerPositions,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    positions: symbols.map((symbol, index) => {
      const [market, code] = symbol.split(".", 2);
      return {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: market ?? "",
        symbol: symbol.trim(),
        symbolName: code ?? symbol,
        quantity: index + 1,
        sellableQuantity: index + 1,
        lastPrice: 100 + index,
        costPrice: 95,
        averageCostPrice: 95,
        marketValue: 1000 + index,
        unrealizedPnl: 50,
        realizedPnl: null,
        pnlRatio: 0.05,
        currency: "USD",
      };
    }),
  };
}

function createBrokerOrders(): BrokerOrdersResponse {
  return {
    ...emptyBrokerOrders,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    orders: [
      {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: "US",
        brokerOrderId: "broker-order-1",
        brokerOrderIdEx: "broker-order-ex-1",
        symbol: "US.AAPL",
        symbolName: "Apple",
        side: "BUY",
        orderType: "LIMIT",
        status: "SUBMITTED",
        quantity: 10,
        filledQuantity: 0,
        price: 195.5,
        filledAveragePrice: null,
        submittedAt: "2026-07-02T09:10:00.000Z",
        updatedAt: "2026-07-02T09:10:00.000Z",
        remark: null,
        lastError: null,
        timeInForce: "DAY",
        currency: "USD",
      },
    ],
  };
}

function createExecutionOrders(
  internalOrderId = "exec-1",
  symbol = "US.AAPL",
): ExecutionOrdersResponse {
  return {
    ...emptyExecutionOrders,
    orders: [
      {
        internalOrderId,
        brokerId: "futu",
        brokerOrderId: `broker-${internalOrderId}`,
        brokerOrderIdEx: null,
        source: "system",
        sourceDetail: "command.place",
        tradingEnvironment: "REAL",
        accountId: "ACC-1",
        market: symbol.split(".", 2)[0] ?? "US",
        symbol,
        side: "BUY",
        orderType: "LIMIT",
        status: "SUBMITTED",
        requestedQuantity: 10,
        requestedPrice: 195.5,
        filledQuantity: 0,
        filledAveragePrice: null,
        remark: null,
        lastError: null,
        lastErrorCode: null,
        lastErrorSource: null,
        submittedAt: "2026-07-02T09:10:00.000Z",
        updatedAt: "2026-07-02T09:10:00.000Z",
        createdAt: "2026-07-02T09:10:00.000Z",
      },
    ],
  };
}

function createCashFlows(): BrokerCashFlowsResponse {
  return {
    ...emptyBrokerCashFlows,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    cashFlows: [
      {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: "US",
        cashFlowId: "cash-flow-1",
        clearingDate: "2026-07-02",
        settlementDate: "2026-07-03",
        currency: "USD",
        cashFlowType: "DEPOSIT",
        cashFlowDirection: "IN",
        cashFlowAmount: 5000,
        cashFlowRemark: "Initial funding",
      },
    ],
  };
}

function createFills(): BrokerFillsResponse {
  return {
    ...emptyBrokerFills,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    fills: [
      {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: "US",
        brokerOrderId: "broker-order-1",
        brokerOrderIdEx: "broker-order-ex-1",
        brokerFillId: "fill-1",
        brokerFillIdEx: null,
        symbol: "US.AAPL",
        symbolName: "Apple",
        side: "BUY",
        filledQuantity: 10,
        fillPrice: 195.5,
        filledAt: "2026-07-02T09:10:30.000Z",
        status: "FILLED",
      },
    ],
  };
}

function createMarginRatios(): BrokerMarginRatiosResponse {
  return {
    ...emptyBrokerMarginRatios,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    marginRatios: [
      {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: "US.AAPL",
        isLongPermit: true,
        isShortPermit: false,
        shortPoolRemain: null,
        shortFeeRate: null,
        alertLongRatio: 0.2,
        alertShortRatio: null,
        initialMarginLongRatio: 0.5,
        initialMarginShortRatio: null,
        marginCallLongRatio: 0.3,
        marginCallShortRatio: null,
        maintenanceLongRatio: 0.25,
        maintenanceShortRatio: null,
      },
      {
        accountId: "ACC-1",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: "US.TSLA",
        isLongPermit: true,
        isShortPermit: true,
        shortPoolRemain: 50,
        shortFeeRate: 0.1,
        alertLongRatio: 0.25,
        alertShortRatio: 0.4,
        initialMarginLongRatio: 0.55,
        initialMarginShortRatio: 0.6,
        marginCallLongRatio: 0.35,
        marginCallShortRatio: 0.45,
        maintenanceLongRatio: 0.3,
        maintenanceShortRatio: 0.35,
      },
    ],
  };
}

function createMaxTradeQuantity(): BrokerMaxTradeQuantityResponse {
  return {
    ...emptyBrokerMaxTradeQuantity,
    checkedAt: "2026-07-02T09:15:00.000Z",
    connectivity: "connected",
    maxTradeQuantity: {
      accountId: "ACC-1",
      tradingEnvironment: "REAL",
      market: "HK",
      symbol: "HK.00700",
      orderType: "LIMIT",
      price: 480,
      maxCashBuy: 200,
      maxCashAndMarginBuy: 400,
      maxPositionSell: 50,
      maxSellShort: 100,
      maxBuyBack: 30,
      longRequiredIM: 1000,
      shortRequiredIM: null,
      session: "REGULAR",
    },
  };
}

function requestedUrls(): string[] {
  return mocks.fetchEnvelope.mock.calls.map(([url]) => String(url));
}

function searchParams(url: string): URLSearchParams {
  return new URL(url, "http://localhost").searchParams;
}

function createController(
  overrides: Partial<
    Record<BrokerReadFeatureKey, Partial<FeatureRequirements>>
  > = {},
) {
  const requirements = createFeatureRequirements(overrides);
  const loadPortfolioLiveData = vi.fn().mockResolvedValue(undefined);
  const resolveBrokerReadFeatureQueryRequirements = vi.fn(
    (feature: BrokerReadFeatureKey) => requirements[feature],
  );

  const state = {
    systemStatus: ref({
      ...emptySystemStatus,
      defaultTradingEnvironment: "SIMULATE",
    }),
    brokerCashFlows: ref<BrokerCashFlowsResponse>(emptyBrokerCashFlows),
    brokerFills: ref<BrokerFillsResponse>(emptyBrokerFills),
    brokerFunds: ref<BrokerFundsResponse>(emptyBrokerFunds),
    brokerMarginRatios: ref<BrokerMarginRatiosResponse>(
      emptyBrokerMarginRatios,
    ),
    brokerMaxTradeQuantity: ref<BrokerMaxTradeQuantityResponse>(
      emptyBrokerMaxTradeQuantity,
    ),
    brokerPositions: ref<BrokerPositionsResponse>(emptyBrokerPositions),
    brokerOrders: ref<BrokerOrdersResponse>(emptyBrokerOrders),
    activeExecutionOrders: ref<ExecutionOrdersResponse>(emptyExecutionOrders),
    historicalExecutionOrders: ref<ExecutionOrdersResponse>(emptyExecutionOrders),
    isLoadingBrokerOrders: ref(false),
    isLoadingHistoricalOrders: ref(false),
    historicalOrdersError: ref(""),
    isLoadingBrokerFills: ref(false),
    isLoadingBrokerMarginRatios: ref(false),
    isLoadingBrokerMaxTradeQuantity: ref(false),
  };

  const controller = createConsoleDataBrokerLiveQueryController({
    ...state,
    resolveBrokerReadFeatureQueryRequirements,
    supportsBrokerReadFeature: (feature, context) =>
      resolveBrokerReadFeatureQueryRequirements(feature, context).supported,
    loadPortfolioLiveData,
  });

  return {
    controller,
    state,
    loadPortfolioLiveData,
    resolveBrokerReadFeatureQueryRequirements,
  };
}

afterEach(() => {
  mocks.fetchEnvelope.mockReset();
  vi.useRealTimers();
});

describe("createConsoleDataBrokerLiveQueryController", () => {
  it("clears in-flight max-trade-quantity results and skips invalid requests", async () => {
    const { controller, state } = createController();
    const pending = deferred<BrokerMaxTradeQuantityResponse>();
    mocks.fetchEnvelope.mockReturnValueOnce(pending.promise);

    const request = controller.loadBrokerMaxTradeQuantity({
      brokerId: "futu",
      tradingEnvironment: "REAL",
      accountId: "ACC-1",
      market: "HK",
      symbol: "00700",
      orderType: "LIMIT",
      price: 480,
    });

    expect(state.isLoadingBrokerMaxTradeQuantity.value).toBe(true);

    controller.clearBrokerMaxTradeQuantity();
    pending.resolve(createMaxTradeQuantity());
    await request;

    expect(state.brokerMaxTradeQuantity.value).toEqual(
      emptyBrokerMaxTradeQuantity,
    );
    expect(state.isLoadingBrokerMaxTradeQuantity.value).toBe(false);

    mocks.fetchEnvelope.mockClear();
    await controller.loadBrokerMaxTradeQuantity({
      brokerId: "futu",
      tradingEnvironment: "REAL",
      accountId: "ACC-1",
      market: "HK",
      symbol: "00700",
      orderType: "LIMIT",
      price: 0,
    });

    expect(mocks.fetchEnvelope).not.toHaveBeenCalled();
  });

  it("loads max trade quantity with normalized request params and degrades failures", async () => {
    const { controller, state } = createController();
    mocks.fetchEnvelope.mockResolvedValueOnce(createMaxTradeQuantity());

    await controller.loadBrokerMaxTradeQuantity({
      brokerId: " futu ",
      tradingEnvironment: "REAL",
      accountId: " ACC-1 ",
      market: "HK",
      symbol: "00700",
      orderType: "LIMIT",
      price: 480,
      orderIdEx: "  broker-order-ex-1 ",
      adjustSideAndLimit: 1,
      session: " REGULAR ",
      positionId: 7,
    });

    const firstUrl = requestedUrls()[0] ?? "";
    const firstParams = searchParams(firstUrl);

    expect(firstUrl).toContain("/api/v1/brokers/futu/max-trade-qtys?");
    expect(firstParams.get("symbol")).toBe("HK.00700");
    expect(firstParams.get("accountId")).toBe("ACC-1");
    expect(firstParams.get("orderIdEx")).toBe("broker-order-ex-1");
    expect(firstParams.get("adjustSideAndLimit")).toBe("1");
    expect(firstParams.get("session")).toBe("REGULAR");
    expect(firstParams.get("positionId")).toBe("7");
    expect(state.brokerMaxTradeQuantity.value.maxTradeQuantity?.maxCashBuy).toBe(
      200,
    );
    expect(state.isLoadingBrokerMaxTradeQuantity.value).toBe(false);

    mocks.fetchEnvelope.mockReset();
    mocks.fetchEnvelope.mockRejectedValueOnce(new Error("qty temporarily down"));

    await controller.loadBrokerMaxTradeQuantity({
      brokerId: "futu",
      tradingEnvironment: "REAL",
      accountId: "ACC-1",
      market: "HK",
      symbol: "HK.00700",
      orderType: "LIMIT",
      price: 480,
    });

    expect(state.brokerMaxTradeQuantity.value.connectivity).toBe("degraded");
    expect(state.brokerMaxTradeQuantity.value.lastError).toBe(
      "qty temporarily down",
    );
    expect(state.isLoadingBrokerMaxTradeQuantity.value).toBe(false);
  });

  it("loads historical execution orders with broker query fallbacks and ignores stale responses", async () => {
    const { controller, state } = createController();
    const first = deferred<ExecutionOrdersResponse>();
    const second = deferred<ExecutionOrdersResponse>();
    mocks.fetchEnvelope
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const firstLoad = controller.loadHistoricalExecutionOrders({
      brokerId: "futu",
      brokerQuery: "accountId=ACC-1&market=US",
    });
    const secondLoad = controller.loadHistoricalExecutionOrders({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&accountId=ACC-2&market=HK",
    });

    second.resolve(createExecutionOrders("exec-2", "HK.00700"));
    await secondLoad;
    first.resolve(createExecutionOrders("exec-1", "US.AAPL"));
    await firstLoad;

    const urls = requestedUrls();
    expect(urls[0]).toContain("tradingEnvironment=SIMULATE");
    expect(urls[0]).toContain("accountId=ACC-1");
    expect(urls[1]).toContain("tradingEnvironment=REAL");
    expect(urls[1]).toContain("accountId=ACC-2");
    expect(state.activeExecutionOrders.value.orders[0]?.internalOrderId).toBe(
      "exec-2",
    );
    expect(
      state.historicalExecutionOrders.value.orders[0]?.internalOrderId,
    ).toBe("exec-2");
    expect(state.historicalOrdersError.value).toBe("");
    expect(state.isLoadingHistoricalOrders.value).toBe(false);
  });

  it("surfaces historical execution order failures", async () => {
    const { controller, state } = createController();
    mocks.fetchEnvelope.mockRejectedValueOnce(new Error("history unavailable"));

    await controller.loadHistoricalExecutionOrders({
      brokerId: "futu",
      brokerQuery: "market=US",
    });

    expect(state.historicalOrdersError.value).toBe("history unavailable");
    expect(state.historicalExecutionOrders.value).toEqual(emptyExecutionOrders);
    expect(state.isLoadingHistoricalOrders.value).toBe(false);
  });

  it("loads broker live data and derives peripheral read queries from runtime state", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-03T12:00:00.000Z"));

    const { controller, state, loadPortfolioLiveData } = createController();

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.includes("/api/v1/brokers/futu/funds")) {
        return createFunds({
          checkedAt: "2026-07-02T09:15:00.000Z",
          accountId: "ACC-1",
          tradingEnvironment: "REAL",
          market: "US",
        });
      }
      if (url.includes("/api/v1/brokers/futu/positions")) {
        return createPositions(["US.AAPL", "US.AAPL", " ", "US.TSLA"]);
      }
      if (url.includes("/api/v1/brokers/futu/orders")) {
        return createBrokerOrders();
      }
      if (
        url.includes("/api/v1/execution/orders") &&
        url.includes("scope=active")
      ) {
        return createExecutionOrders("exec-live", "US.AAPL");
      }
      if (url.includes("/api/v1/brokers/futu/cash-flows")) {
        return createCashFlows();
      }
      if (url.includes("/api/v1/brokers/futu/fills")) {
        return createFills();
      }
      if (url.includes("/api/v1/brokers/futu/margin-ratios")) {
        return createMarginRatios();
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    await controller.loadBrokerLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&accountId=ACC-1&market=US",
      futuBrokerReadsPaused: false,
    });

    expect(loadPortfolioLiveData).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&accountId=ACC-1&market=US",
    });
    expect(state.brokerFunds.value.summary?.accountId).toBe("ACC-1");
    expect(state.brokerPositions.value.positions).toHaveLength(4);
    expect(state.brokerOrders.value.orders).toHaveLength(1);
    expect(state.activeExecutionOrders.value.orders[0]?.internalOrderId).toBe(
      "exec-live",
    );
    expect(state.brokerCashFlows.value.cashFlows).toHaveLength(1);
    expect(state.brokerFills.value.fills).toHaveLength(1);
    expect(state.brokerMarginRatios.value.marginRatios).toHaveLength(2);
    expect(state.isLoadingBrokerOrders.value).toBe(false);
    expect(state.isLoadingBrokerFills.value).toBe(false);
    expect(state.isLoadingBrokerMarginRatios.value).toBe(false);

    const urls = requestedUrls();
    const cashFlowUrl = urls.find((url) => url.includes("/cash-flows")) ?? "";
    const fillsUrl = urls.find((url) => url.includes("/fills")) ?? "";
    const marginRatiosUrl =
      urls.find((url) => url.includes("/margin-ratios")) ?? "";

    expect(searchParams(cashFlowUrl).get("clearingDate")).toBe("2026-07-02");
    expect(searchParams(fillsUrl).get("scope")).toBe("history");
    expect(searchParams(fillsUrl).get("startTime")).toBe(
      "2026-06-03T12:00:00.000Z",
    );
    expect(searchParams(fillsUrl).get("endTime")).toBe(
      "2026-07-03T12:00:00.000Z",
    );
    expect(searchParams(marginRatiosUrl).getAll("symbol")).toEqual([
      "US.AAPL",
      "US.TSLA",
    ]);
  });

  it("resets peripheral reads when broker reads are paused and degrades downstream failures", async () => {
    const paused = createController();
    paused.state.brokerCashFlows.value = createCashFlows();
    paused.state.brokerFills.value = createFills();
    paused.state.brokerMarginRatios.value = createMarginRatios();
    mocks.fetchEnvelope.mockResolvedValueOnce(createExecutionOrders("exec-paused"));

    await paused.controller.loadBrokerLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&accountId=ACC-1&market=US",
      futuBrokerReadsPaused: true,
    });

    expect(requestedUrls()).toHaveLength(1);
    expect(requestedUrls()[0]).toContain("/api/v1/execution/orders");
    expect(paused.state.brokerCashFlows.value).toEqual(emptyBrokerCashFlows);
    expect(paused.state.brokerFills.value).toEqual(emptyBrokerFills);
    expect(paused.state.brokerMarginRatios.value).toEqual(
      emptyBrokerMarginRatios,
    );
    expect(paused.state.isLoadingBrokerFills.value).toBe(false);
    expect(paused.state.isLoadingBrokerMarginRatios.value).toBe(false);

    mocks.fetchEnvelope.mockReset();
    const failing = createController({
      cashFlows: { supported: true, requiresClearingDate: false },
      fills: { supported: true, supportsHistory: false },
      marginRatios: { supported: true, requiresSymbols: true },
    });

    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.includes("/api/v1/brokers/futu/funds")) {
        return createFunds();
      }
      if (url.includes("/api/v1/brokers/futu/positions")) {
        return createPositions(["US.AAPL"]);
      }
      if (url.includes("/api/v1/brokers/futu/orders")) {
        return createBrokerOrders();
      }
      if (
        url.includes("/api/v1/execution/orders") &&
        url.includes("scope=active")
      ) {
        return createExecutionOrders("exec-failing");
      }
      if (url.includes("/api/v1/brokers/futu/cash-flows")) {
        throw new Error("cash-flows unavailable");
      }
      if (url.includes("/api/v1/brokers/futu/fills")) {
        throw new Error("fills unavailable");
      }
      if (url.includes("/api/v1/brokers/futu/margin-ratios")) {
        throw new Error("margin-ratios unavailable");
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    await failing.controller.loadBrokerLiveData({
      brokerId: "futu",
      brokerQuery: "tradingEnvironment=REAL&accountId=ACC-1&market=US",
      futuBrokerReadsPaused: false,
    });

    expect(failing.state.brokerCashFlows.value.connectivity).toBe(
      "disconnected",
    );
    expect(failing.state.brokerCashFlows.value.lastError).toBe(
      "cash-flows unavailable",
    );
    expect(failing.state.brokerFills.value.connectivity).toBe("degraded");
    expect(failing.state.brokerFills.value.lastError).toBe("fills unavailable");
    expect(failing.state.brokerMarginRatios.value.connectivity).toBe(
      "degraded",
    );
    expect(failing.state.brokerMarginRatios.value.lastError).toBe(
      "margin-ratios unavailable",
    );
    expect(failing.state.isLoadingBrokerOrders.value).toBe(false);
    expect(failing.state.isLoadingBrokerFills.value).toBe(false);
    expect(failing.state.isLoadingBrokerMarginRatios.value).toBe(false);
  });

  it("keeps newer quantity and history results when stale requests fail later", async () => {
    const { controller, state } = createController();
    const staleQuantity = deferred<BrokerMaxTradeQuantityResponse>();
    const freshQuantity = deferred<BrokerMaxTradeQuantityResponse>();
    mocks.fetchEnvelope
      .mockReturnValueOnce(staleQuantity.promise)
      .mockReturnValueOnce(freshQuantity.promise);
    const staleQuantityLoad = controller.loadBrokerMaxTradeQuantity({
      brokerId: "futu", tradingEnvironment: "REAL", accountId: "ACC-1",
      market: "HK", symbol: "00700", orderType: "LIMIT", price: 480,
    });
    const freshQuantityLoad = controller.loadBrokerMaxTradeQuantity({
      brokerId: "futu", tradingEnvironment: "REAL", accountId: "ACC-1",
      market: "HK", symbol: "00700", orderType: "LIMIT", price: 481,
    });
    freshQuantity.resolve(createMaxTradeQuantity());
    await freshQuantityLoad;
    staleQuantity.reject(new Error("stale quantity failure"));
    await staleQuantityLoad;
    expect(state.brokerMaxTradeQuantity.value.connectivity).toBe("connected");

    const staleHistory = deferred<ExecutionOrdersResponse>();
    const freshHistory = deferred<ExecutionOrdersResponse>();
    mocks.fetchEnvelope.mockReset();
    mocks.fetchEnvelope
      .mockReturnValueOnce(staleHistory.promise)
      .mockReturnValueOnce(freshHistory.promise);
    const staleHistoryLoad = controller.loadHistoricalExecutionOrders({ brokerId: "futu", brokerQuery: "market=US" });
    const freshHistoryLoad = controller.loadHistoricalExecutionOrders({ brokerId: "futu", brokerQuery: "market=HK" });
    freshHistory.resolve(createExecutionOrders("fresh-history", "HK.00700"));
    await freshHistoryLoad;
    staleHistory.reject(new Error("stale history failure"));
    await staleHistoryLoad;
    expect(state.historicalExecutionOrders.value.orders[0]?.internalOrderId).toBe("fresh-history");
    expect(state.historicalOrdersError.value).toBe("");
  });

  it("skips undated cash-flow reads and clears the loading state when primary data fails", async () => {
    const { controller, state } = createController();
    mocks.fetchEnvelope.mockImplementation(async (url: string) =>
      url.includes("/funds") ? createFunds({ checkedAt: "" }) :
        url.includes("/positions") ? createPositions([]) :
          url.includes("/orders") ? createBrokerOrders() :
            url.includes("/execution/orders") ? createExecutionOrders("no-date") : undefined,
    );
    await controller.loadBrokerLiveData({
      brokerId: "futu", brokerQuery: "market=US", futuBrokerReadsPaused: false,
    });
    expect(requestedUrls().some((url) => url.includes("/cash-flows"))).toBe(false);
    mocks.fetchEnvelope.mockImplementation(async (url: string) => {
      if (url.includes("/funds")) throw new Error("broker offline");
      return url.includes("/execution/orders") ? createExecutionOrders("offline") : undefined;
    });
    await controller.loadBrokerLiveData({
      brokerId: "futu", brokerQuery: "market=US", futuBrokerReadsPaused: false,
    });
    expect(state.isLoadingBrokerOrders.value).toBe(false);
  });
});
