// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import {
  formatExecutionStatusTransition,
  orderStatusClass,
} from "../src/components/domain/account/executionOrderFormat";
import { formatMoney } from "../src/utils/numberFormat";
import AccountPage from "../src/pages/AccountPage.vue";

const mocks = vi.hoisted(() => ({
  fetchEnvelope: vi.fn(),
  fetchEnvelopeWithInit: vi.fn(),
  loadExecutionOrderDetails: vi.fn(),
  loadHistoricalExecutionOrders: vi.fn(),
  pushNotification: vi.fn(),
  supportsBrokerReadFeature: vi.fn(),
  resolveBrokerReadFeatureQueryRequirements: vi.fn(),
}));

let consoleDataState: Record<string, unknown>;
let notificationsState: { push: typeof mocks.pushNotification };

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => mocks.fetchEnvelopeWithInit(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("../src/composables/useNotifications", () => ({
  useNotifications: () => notificationsState,
}));

type SetupState = Record<string, unknown>;

const wrappers: VueWrapper[] = [];

function makeExecutionOrder(overrides: Record<string, unknown> = {}) {
  return {
    brokerId: "futu",
    accountId: "REAL-001",
    tradingEnvironment: "REAL",
    market: "US",
    internalOrderId: "order-1",
    brokerOrderId: "9001",
    brokerOrderIdEx: "9001-EX",
    symbol: "US.AAPL",
    symbolName: "Apple",
    side: "BUY",
    orderType: "LIMIT",
    source: "broker",
    sourceDetail: "manual",
    status: "SUBMITTED",
    requestedQuantity: 10,
    filledQuantity: 2,
    updatedAt: "2026-06-01T10:00:00Z",
    ...overrides,
  };
}

function createConsoleDataState() {
  return {
    brokerCashFlows: ref({
      connectivity: "connected",
      cashFlows: [],
      lastError: "",
    }),
    brokerFills: ref({
      fills: [],
      lastError: "",
    }),
    brokerFunds: ref({
      summary: null,
    }),
    brokerMarginRatios: ref({
      connectivity: "connected",
      marginRatios: [],
      lastError: "",
    }),
    brokerOrderFees: ref({
      fees: [],
    }),
    brokerPositions: ref({
      checkedAt: "2026-06-01T00:00:00Z",
      positions: [],
    }),
    brokerRuntime: ref({
      descriptor: {
        id: "futu",
        displayName: "Futu OpenAPI",
        capabilities: [{ market: "US" }],
      },
      session: {
        connectivity: "connected",
      },
      accounts: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          accountType: "CASH",
          securityFirm: "FUTUSECURITIES",
          marketAuthorities: ["US"],
        },
      ],
    }),
    executionEventsError: ref(""),
    executionOrderEvents: ref({
      internalOrderId: "",
      events: [],
    }),
    activeExecutionOrders: ref({
      orders: [],
    }),
    historicalExecutionOrders: ref({
      orders: [],
    }),
    isLoadingBrokerFills: ref(false),
    isLoadingBrokerMarginRatios: ref(false),
    isLoadingBrokerOrders: ref(false),
    isLoadingHistoricalOrders: ref(false),
    historicalOrdersError: ref(""),
    isLoadingExecutionEvents: ref(false),
    isLoadingOrderFees: ref(false),
    loadExecutionOrderDetails: mocks.loadExecutionOrderDetails,
    loadHistoricalExecutionOrders: mocks.loadHistoricalExecutionOrders,
    orderFeesError: ref(""),
    portfolioCashBalances: ref({
      balances: [],
    }),
    portfolioPositions: ref({
      positions: [],
    }),
    portfolioReconciliation: ref({
      positions: [],
    }),
    resolveBrokerReadFeatureQueryRequirements:
      mocks.resolveBrokerReadFeatureQueryRequirements,
    selectedBrokerAccount: ref(null),
    selectedExecutionOrder: ref(null),
    selectedExecutionOrderId: ref(""),
    supportsBrokerReadFeature: mocks.supportsBrokerReadFeature,
    systemStatus: ref({
      defaultTradingEnvironment: "REAL",
    }),
  };
}

function mountAccountPage() {
  const wrapper = mount(AccountPage);
  wrappers.push(wrapper);
  const setup = wrapper.vm.$.setupState as SetupState;
  const call = <T>(name: string, ...args: unknown[]) =>
    (setup[name] as (...values: unknown[]) => T)(...args);
  return { wrapper, setup, call };
}

function readSetupValue<T>(value: unknown): T {
  if (value !== null && typeof value === "object" && "value" in value) {
    return (value as { value: T }).value;
  }
  return value as T;
}

function setRefValue<T>(target: unknown, value: T): void {
  (target as { value: T }).value = value;
}

function tabButtons(wrapper: VueWrapper) {
  return wrapper.findAll('button[role="tab"]');
}

async function activateTab(wrapper: VueWrapper, label: string) {
  const tab = tabButtons(wrapper).find((candidate) =>
    candidate.text().includes(label),
  );
  expect(tab, `tab ${label}`).toBeDefined();
  await tab!.trigger("click");
  await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  consoleDataState = createConsoleDataState();
  notificationsState = { push: mocks.pushNotification };
  mocks.loadExecutionOrderDetails.mockResolvedValue(undefined);
  mocks.loadHistoricalExecutionOrders.mockResolvedValue(undefined);
  mocks.fetchEnvelope.mockResolvedValue({ orders: [] });
  mocks.fetchEnvelopeWithInit.mockResolvedValue({ message: "accepted" });
  mocks.supportsBrokerReadFeature.mockReturnValue(false);
  mocks.resolveBrokerReadFeatureQueryRequirements.mockReturnValue({
    supportsHistory: false,
  });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  window.history.pushState({}, "", "/");
});

describe("AccountPage business flows", () => {
  it("opens an order detail from the account orderId query", async () => {
    window.history.pushState({}, "", "/account?tab=history&orderId=order-query");

    const { setup } = mountAccountPage();
    await nextTick();

    expect(readSetupValue<string>(setup.activeTab)).toBe("history");
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalled();
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order-query");
  });

  it("renders A-share positions with the parent market and exchange identity", () => {
    readSetupValue<{ positions: unknown[] }>(
      consoleDataState.portfolioPositions,
    ).positions = [
      {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "SH",
        symbol: "SH.600519",
        quantity: 10,
        averagePrice: 1500,
        marketValue: 15000,
        updatedAt: "2026-06-01T09:31:00Z",
      },
    ];

    const { wrapper } = mountAccountPage();

    const tabs = tabButtons(wrapper);
    expect(tabs.map((tab) => tab.text())).toEqual([
      "持仓",
      "订单",
      "历史",
      "资金",
    ]);
    expect(tabs[0]?.classes()).toContain("is-active");
    expect(wrapper.text()).toContain("600519");
    expect(wrapper.text()).toContain("上证");
    expect(wrapper.text()).toContain("沪深");
    expect(wrapper.text()).not.toContain("SH.600519");
  });

  it("redirects the legacy ?tab=risk URL to the standalone risk page", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/account", component: { template: "<div />" } },
        { path: "/risk", component: { template: "<div />" } },
      ],
    });
    await router.push("/account?tab=risk");
    await router.isReady();
    const replace = vi.spyOn(router, "replace");

    const wrapper = mount(AccountPage, { global: { plugins: [router] } });
    wrappers.push(wrapper);
    await nextTick();

    expect(replace).toHaveBeenCalledWith("/risk");
  });

  it("falls back to runtime-scoped projected data and dedupes visible orders", async () => {
    const pendingReal = makeExecutionOrder();
    const duplicatePendingReal = makeExecutionOrder({
      internalOrderId: "order-duplicate",
    });
    const pendingSimulate = makeExecutionOrder({
      internalOrderId: "order-sim",
      tradingEnvironment: "SIMULATE",
      accountId: "SIM-001",
    });
    const finalHistorical = makeExecutionOrder({
      internalOrderId: "order-filled",
      status: "FILLED",
    });

    readSetupValue<{ orders: unknown[] }>(
      consoleDataState.activeExecutionOrders,
    ).orders = [pendingReal, duplicatePendingReal, pendingSimulate];
    readSetupValue<{ orders: unknown[] }>(
      consoleDataState.historicalExecutionOrders,
    ).orders = [finalHistorical];
    readSetupValue<{ balances: unknown[] }>(
      consoleDataState.portfolioCashBalances,
    ).balances = [
      {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        currency: "USD",
        cashBalance: 1200,
        updatedAt: "2026-06-01T09:30:00Z",
      },
      {
        brokerId: "futu",
        accountId: "SIM-001",
        tradingEnvironment: "SIMULATE",
        currency: "USD",
        cashBalance: 99,
        updatedAt: "2026-06-01T09:30:00Z",
      },
    ];
    readSetupValue<{ positions: unknown[] }>(
      consoleDataState.portfolioPositions,
    ).positions = [
      {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: "US.AAPL",
        quantity: 2,
        averagePrice: 100,
        marketValue: 220,
        updatedAt: "2026-06-01T09:31:00Z",
      },
      {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: " US.AAPL ",
        quantity: 1,
        averagePrice: 100,
        marketValue: 110,
        updatedAt: "2026-06-01T09:31:30Z",
      },
      {
        brokerId: "futu",
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: " ",
        quantity: 1,
        averagePrice: 1,
        marketValue: 1,
        updatedAt: "2026-06-01T09:32:00Z",
      },
    ];

    const { wrapper, setup, call } = mountAccountPage();
    await nextTick();

    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order-1");
    expect(wrapper.text()).toContain("REAL-001");
    expect(readSetupValue<Array<{ source: string }>>(setup.accountPositions)).toEqual([
      expect.objectContaining({ source: "投影", symbol: "US.AAPL" }),
      expect.objectContaining({ source: "投影", symbol: " US.AAPL " }),
      expect.objectContaining({ source: "投影", symbol: " " }),
    ]);
    expect(readSetupValue<{ brokerId: string; accountId: string; market: string }>(
      setup.activeBrokerReadContext,
    )).toEqual({
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    expect(readSetupValue<string[]>(setup.marginRatioSymbols)).toEqual(["US.AAPL"]);
    expect(readSetupValue<boolean>(setup.supportsBrokerCashFlows)).toBe(false);
    expect(readSetupValue<boolean>(setup.supportsBrokerMarginRatios)).toBe(false);
    expect(readSetupValue<Array<unknown>>(setup.pendingOrders)).toHaveLength(1);
    expect(readSetupValue<Array<unknown>>(setup.historicalOrders)).toHaveLength(1);
    // 在途订单数量以徽标形式显示在「订单」tab 上。
    expect(tabButtons(wrapper)[1]?.text()).toContain("1");
    expect(call<string>("executionOrdersUrl")).toContain(
      "brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    );
    expect(formatExecutionStatusTransition("", "SUBMITTED")).toContain("首次发现");
    expect(orderStatusClass("REJECTED")).toBe("tv-status--error");
    expect(orderStatusClass("FAILED_TO_ROUTE")).toBe("tv-status--error");
    expect(formatMoney(220, "USD")).toContain("USD");

    await activateTab(wrapper, "历史");
    expect(wrapper.text()).toContain("当前刷新窗口");

    await activateTab(wrapper, "资金");
    expect(wrapper.text()).toContain("当前券商未为该交易环境声明资金流水能力。");
    expect(wrapper.text()).toContain("当前券商未为该交易环境声明融资融券参数能力。");
  });

  it("prefers selected-account broker data, renders fees and fills, and lazy-loads history once", async () => {
    const selectedPending = makeExecutionOrder({
      accountId: "SIM-002",
      tradingEnvironment: "SIMULATE",
      market: "HK",
      internalOrderId: "pending-hk",
      symbol: "HK.00700",
    });
    const historicalOrders = Array.from({ length: 55 }, (_, index) =>
      makeExecutionOrder({
        accountId: "SIM-002",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        internalOrderId: `hist-${index + 1}`,
        brokerOrderId: `hist-broker-${index + 1}`,
        brokerOrderIdEx: `hist-broker-ex-${index + 1}`,
        status: index % 2 === 0 ? "FILLED" : "CANCELLED",
        symbol: "HK.00700",
      }),
    );

    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "SIM-002",
      displayName: "Margin Account",
      tradingEnvironment: "SIMULATE",
      market: "HK",
      securityFirm: "FUTUSECURITIES",
    });
    setRefValue(consoleDataState.selectedExecutionOrder, selectedPending);
    setRefValue(consoleDataState.selectedExecutionOrderId, "missing-order");
    setRefValue(consoleDataState.brokerRuntime, {
      descriptor: {
        id: "futu",
        displayName: "Futu OpenAPI",
        capabilities: [{ market: "HK" }],
      },
      session: {
        connectivity: "connected",
      },
      accounts: [
        {
          accountId: "SIM-002",
          tradingEnvironment: "SIMULATE",
          accountType: "MARGIN",
          securityFirm: "FUTUSECURITIES",
          marketAuthorities: ["HK"],
        },
      ],
    });
    setRefValue(consoleDataState.brokerFunds, {
      summary: {
        accountId: "SIM-002",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        currency: "HKD",
        cash: 9000,
        totalAssets: 15000,
        purchasingPower: 5000,
        availableWithdrawalCash: 8000,
        marketValue: null,
        frozenCash: 200,
        maxWithdrawal: 7500,
        shortSellingPower: 1500,
        netCashPower: 4800,
        longMarketValue: 6000,
        shortMarketValue: 0,
        debtCash: 100,
      },
    });
    setRefValue(consoleDataState.brokerPositions, {
      checkedAt: "2026-06-01T09:40:00Z",
      positions: [
        {
          accountId: "SIM-002",
          tradingEnvironment: "SIMULATE",
          market: "HK",
          symbol: "HK.00700",
          symbolName: "Tencent",
          quantity: 100,
          costPrice: 300,
          averageCostPrice: null,
          marketValue: 32000,
          unrealizedPnl: 2000,
          currency: "HKD",
        },
      ],
    });
    readSetupValue<{ orders: unknown[] }>(
      consoleDataState.activeExecutionOrders,
    ).orders = [
      selectedPending,
      makeExecutionOrder({
        accountId: "SIM-002",
        tradingEnvironment: "SIMULATE",
        market: "US",
        internalOrderId: "wrong-market",
      }),
    ];
    readSetupValue<{ orders: unknown[] }>(
      consoleDataState.historicalExecutionOrders,
    ).orders = historicalOrders;
    setRefValue(consoleDataState.executionOrderEvents, {
      internalOrderId: "",
      events: [
        {
          id: "event-1",
          eventType: "ORDER_STATUS",
          createdAt: "2026-06-01T09:45:00Z",
          previousStatus: "SUBMITTED",
          nextStatus: "FILLED",
        },
      ],
    });
    setRefValue(consoleDataState.brokerOrderFees, {
      fees: [
        {
          brokerOrderIdEx: "FEE-9001",
          feeAmount: 12.5,
          feeItems: [
            { title: "平台费", value: 10 },
            { title: "交收费", value: 2.5 },
          ],
        },
      ],
    });
    setRefValue(consoleDataState.brokerFills, {
      fills: [
        {
          brokerFillId: "fill-1",
          brokerFillIdEx: "fill-1-ex",
          symbol: "HK.00700",
          symbolName: "Tencent",
          side: "BUY",
          filledQuantity: 100,
          fillPrice: 320,
          status: "FILLED",
          filledAt: "2026-06-01T09:46:00Z",
        },
      ],
      lastError: "",
    });
    setRefValue(consoleDataState.brokerCashFlows, {
      connectivity: "connected",
      cashFlows: [
        {
          cashFlowId: "flow-1",
          clearingDate: "2026-06-01",
          settlementDate: "2026-06-03",
          cashFlowType: "DIVIDEND",
          cashFlowDirection: "IN",
          cashFlowAmount: 88,
          cashFlowRemark: "Dividend",
          currency: "HKD",
        },
      ],
      lastError: "",
    });
    setRefValue(consoleDataState.brokerMarginRatios, {
      connectivity: "connected",
      marginRatios: [
        {
          symbol: "HK.00700",
          isLongPermit: true,
          isShortPermit: false,
          shortPoolRemain: 0,
          shortFeeRate: 0.02,
          alertLongRatio: 0.9,
          alertShortRatio: 1.2,
          initialMarginLongRatio: 0.5,
          initialMarginShortRatio: 0.8,
          maintenanceLongRatio: 0.3,
          maintenanceShortRatio: 0.6,
        },
      ],
      lastError: "",
    });
    mocks.supportsBrokerReadFeature.mockImplementation((feature: string) =>
      ["cashFlows", "marginRatios", "orderFees"].includes(feature),
    );
    mocks.resolveBrokerReadFeatureQueryRequirements.mockReturnValue({
      supportsHistory: true,
    });

    const { wrapper, setup, call } = mountAccountPage();
    await nextTick();

    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("pending-hk");
    expect(wrapper.text()).toContain("Margin Account");
    expect(wrapper.text()).toContain("SIM-002");
    expect(readSetupValue<Array<{ source: string; averagePrice: number }>>(
      setup.accountPositions,
    )).toEqual([
      expect.objectContaining({ source: "券商", averagePrice: 300 }),
    ]);
    // 资金摘要散落在 sidebar / 资产指标带中，均优先使用券商资金快照。
    expect(wrapper.text()).toContain("9,000 HKD");
    expect(wrapper.text()).toContain("15,000 HKD");
    expect(readSetupValue<{ brokerId: string; accountId: string; market: string }>(
      setup.activeBrokerReadContext,
    )).toEqual({
      brokerId: "futu",
      accountId: "SIM-002",
      tradingEnvironment: "SIMULATE",
      market: "HK",
    });
    expect(call<boolean>("canCancelOrder", selectedPending)).toBe(true);
    expect(
      call<boolean>(
        "canCancelOrder",
        makeExecutionOrder({ status: "PENDING_CANCEL" }),
      ),
    ).toBe(false);
    expect(formatExecutionStatusTransition("NEW", "FILLED")).toContain("→");

    await activateTab(wrapper, "历史");
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery:
        "brokerId=futu&tradingEnvironment=SIMULATE&accountId=SIM-002&market=HK",
    });
    // 费用与成交随历史订单详情一起渲染；券商已声明费用能力，不出现能力缺失提示。
    expect(wrapper.text()).toContain("平台费");
    expect(wrapper.text()).toContain("fill-1-ex");
    expect(wrapper.text()).not.toContain("当前券商未为该交易环境声明费用查询能力。");

    mocks.loadHistoricalExecutionOrders.mockClear();
    mocks.loadExecutionOrderDetails.mockClear();

    call<void>("openOrderEvents", historicalOrders[0]);
    await nextTick();
    expect(readSetupValue<string>(setup.activeTab)).toBe("history");
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("hist-1");
    expect(mocks.loadHistoricalExecutionOrders).not.toHaveBeenCalled();

    call<void>("loadMoreHistoricalOrders");
    expect(readSetupValue<Array<unknown>>(setup.displayedHistoricalOrders)).toHaveLength(
      55,
    );
    expect(readSetupValue<boolean>(setup.hasMoreHistoricalOrders)).toBe(false);

    mocks.loadExecutionOrderDetails.mockClear();
    const historicalOrderButton = wrapper
      .findAll(".order-history__item")
      .find((candidate) => candidate.text().includes("00700"));
    expect(historicalOrderButton).toBeDefined();
    await historicalOrderButton!.trigger("click");
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("hist-1");

    readSetupValue<{ lastError: string; fills: unknown[] }>(consoleDataState.brokerFills).lastError =
      "fills unavailable";
    readSetupValue<{ lastError: string; fills: unknown[] }>(consoleDataState.brokerFills).fills =
      [];
    setRefValue(consoleDataState.executionOrderEvents, {
      internalOrderId: "hist-1",
      events: [],
    });
    setRefValue(consoleDataState.executionEventsError, "events unavailable");
    setRefValue(consoleDataState.orderFeesError, "fees unavailable");
    setRefValue(consoleDataState.brokerOrderFees, { fees: [] });
    await nextTick();
    expect(wrapper.text()).toContain("fills unavailable");
    expect(wrapper.text()).toContain("events unavailable");
    expect(wrapper.text()).toContain("fees unavailable");

    await activateTab(wrapper, "资金");
    expect(wrapper.text()).toContain("Dividend");
  });

  it("submits cancel commands, refreshes orders, and surfaces failures", async () => {
    const selectedOrder = makeExecutionOrder({
      internalOrderId: "order/cancel",
      symbol: null,
    });
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      displayName: "Real Account",
      tradingEnvironment: "REAL",
      market: "US",
    });
    readSetupValue<{ orders: unknown[] }>(
      consoleDataState.activeExecutionOrders,
    ).orders = [selectedOrder];

    const { call } = mountAccountPage();
    await nextTick();

    mocks.loadExecutionOrderDetails.mockClear();
    mocks.fetchEnvelope.mockClear();
    mocks.fetchEnvelopeWithInit.mockClear();
    mocks.pushNotification.mockClear();

    mocks.fetchEnvelopeWithInit.mockResolvedValueOnce({
      message: "撤单已提交",
    });
    mocks.fetchEnvelope.mockResolvedValueOnce({ orders: [selectedOrder] });

    await call<Promise<void>>("cancelOrder", selectedOrder);

    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/execution/orders/order%2Fcancel/cancel",
      { method: "POST" },
    );
    expect(mocks.fetchEnvelope).toHaveBeenCalledWith(
      "/api/v1/execution/orders?brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    );
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order/cancel");
    expect(mocks.pushNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "success",
        message: "撤单已提交",
      }),
    );
    expect(call<boolean>("isCancellingOrder", "order/cancel")).toBe(false);

    mocks.fetchEnvelopeWithInit.mockClear();
    await call<Promise<void>>(
      "cancelOrder",
      makeExecutionOrder({ status: "FILLED", internalOrderId: "done-order" }),
    );
    expect(mocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();

    mocks.pushNotification.mockClear();
    mocks.fetchEnvelopeWithInit.mockRejectedValueOnce(new Error(" "));
    await call<Promise<void>>("cancelOrder", selectedOrder);
    expect(mocks.pushNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "error",
        title: "撤单失败 order/cancel",
        message: "撤单请求失败。",
      }),
    );
    expect(orderStatusClass("CANCEL_REQUESTED")).toBe("tv-status--warning");
  });

  it("keeps account-scoped diagnostics visible and does not query history without a scope", async () => {
    mocks.supportsBrokerReadFeature.mockImplementation((feature: string) =>
      feature === "cashFlows" || feature === "marginRatios",
    );
    setRefValue(consoleDataState.brokerCashFlows, {
      connectivity: "connected",
      cashFlows: [],
      lastError: "现金流水服务暂不可用",
    });
    setRefValue(consoleDataState.brokerMarginRatios, {
      connectivity: "connected",
      marginRatios: [],
      lastError: "融资融券服务暂不可用",
    });
    setRefValue(consoleDataState.brokerPositions, {
      checkedAt: "2026-07-16T09:30:00Z",
      positions: [{
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        market: "US",
        symbol: "US.AAPL",
        symbolName: "Apple",
        quantity: 1,
        costPrice: 100,
        averageCostPrice: 100,
        marketValue: 120,
        unrealizedPnl: 20,
        currency: "USD",
      }],
    });

    const { wrapper, call } = mountAccountPage();
    await nextTick();

    await activateTab(wrapper, "资金");
    expect(wrapper.text()).toContain("现金流水服务暂不可用");
    expect(wrapper.text()).toContain("融资融券服务暂不可用");

    setRefValue(consoleDataState.brokerRuntime, {
      descriptor: { id: "futu", displayName: "Futu", capabilities: [] },
      session: { connectivity: "disconnected" },
      accounts: [],
    });
    setRefValue(consoleDataState.systemStatus, {});
    await nextTick();
    expect(call<boolean>("orderMatchesTradingEnvironment", "REAL")).toBe(false);
    mocks.loadHistoricalExecutionOrders.mockClear();
    call<void>("ensureHistoricalOrdersLoaded");
    expect(mocks.loadHistoricalExecutionOrders).not.toHaveBeenCalled();
  });
});
