// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import AccountPage from "../src/pages/AccountPage.vue";
import {
  buttonStub,
  emptyStub,
  passthroughStub,
  tabStub,
  tabsStub,
  windowItemStub,
  windowStub,
} from "./helpers";

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
  const wrapper = mount(AccountPage, {
    global: {
      stubs: {
        PageHeader: {
          props: ["eyebrow", "title", "description", "stats"],
          template:
            "<section><div>{{ eyebrow }}</div><div>{{ title }}</div><div>{{ description }}</div><div v-for='stat in stats' :key='stat.label'>{{ stat.label }}:{{ String(stat.value) }}</div></section>",
        },
        SectionHeader: {
          props: ["title", "description"],
          template: "<header>{{ title }}{{ description }}</header>",
        },
        "v-card": passthroughStub,
        "v-card-text": passthroughStub,
        "v-chip": passthroughStub,
        "v-btn": buttonStub,
        "v-alert": passthroughStub,
        "v-empty-state": emptyStub,
        "v-progress-circular": passthroughStub,
        "v-tabs": tabsStub,
        "v-tab": tabStub,
        "v-window": windowStub,
        "v-window-item": windowItemStub,
      },
    },
  });
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

function writeSetupValue(
  setup: SetupState,
  key: string,
  value: unknown,
): void {
  const current = setup[key];
  if (current !== null && typeof current === "object" && "value" in current) {
    (current as { value: unknown }).value = value;
    return;
  }
  setup[key] = value;
}

function setRefValue<T>(target: unknown, value: T): void {
  (target as { value: T }).value = value;
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
});

describe("AccountPage business flows", () => {
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
    expect(readSetupValue<string>(setup.accountTitle)).toBe("REAL-001");
    expect(readSetupValue<string>(setup.accountSubtitle)).toContain("REAL-001");
    expect(readSetupValue<Array<{ source: string }>>(setup.accountPositions)).toEqual([
      expect.objectContaining({ source: "投影", symbol: "US.AAPL" }),
      expect.objectContaining({ source: "投影", symbol: " US.AAPL " }),
      expect.objectContaining({ source: "投影", symbol: " " }),
    ]);
    expect(readSetupValue<number>(setup.totalCash)).toBe(1200);
    expect(readSetupValue<number>(setup.totalMarketValue)).toBe(331);
    expect(readSetupValue<Array<{ label: string; tone?: string }>>(
      setup.accountHeaderStats,
    )).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: "在途订单", tone: "warn" }),
      ]),
    );
    expect(readSetupValue<{ brokerId: string; accountId: string; market: string }>(
      setup.activeBrokerReadContext,
    )).toEqual({
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    expect(readSetupValue<string[]>(setup.marginRatioSymbols)).toEqual(["US.AAPL"]);
    expect(readSetupValue<string>(setup.brokerFillsDescription)).toContain("当前刷新窗口");
    expect(readSetupValue<boolean>(setup.supportsBrokerCashFlows)).toBe(false);
    expect(readSetupValue<boolean>(setup.supportsBrokerMarginRatios)).toBe(false);
    expect(readSetupValue<Array<unknown>>(setup.pendingOrders)).toHaveLength(1);
    expect(readSetupValue<Array<unknown>>(setup.historicalOrders)).toHaveLength(1);
    expect(call<string>("executionOrdersUrl")).toContain(
      "brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    );
    expect(call<string>("formatExecutionStatusTransition", "", "SUBMITTED")).toContain(
      "首次发现",
    );
    expect(call<string>("resolveOrderChipColor", "REJECTED")).toBe("info");
    expect(call<string>("resolveOrderChipColor", "FAILED_TO_ROUTE")).toBe("error");
    expect(call<string>("formatMoney", 220, "USD")).toContain("USD");
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
    expect(readSetupValue<string>(setup.accountTitle)).toBe("Margin Account");
    expect(readSetupValue<string>(setup.accountSubtitle)).toContain("SIM-002");
    expect(readSetupValue<Array<{ source: string; averagePrice: number }>>(
      setup.accountPositions,
    )).toEqual([
      expect.objectContaining({ source: "券商", averagePrice: 300 }),
    ]);
    expect(readSetupValue<number>(setup.totalCash)).toBe(9000);
    expect(readSetupValue<boolean>(setup.supportsSelectedExecutionOrderFees)).toBe(
      true,
    );
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
    expect(call<string>("formatExecutionStatusTransition", "NEW", "FILLED")).toContain(
      "→",
    );
    expect(wrapper.text()).toContain("平台费");
    expect(wrapper.text()).toContain("fill-1-ex");
    expect(wrapper.text()).toContain("Dividend");

    writeSetupValue(setup, "activeTab", "history");
    await nextTick();
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery:
        "brokerId=futu&tradingEnvironment=SIMULATE&accountId=SIM-002&market=HK",
    });

    mocks.loadHistoricalExecutionOrders.mockClear();
    mocks.loadExecutionOrderDetails.mockClear();

    call<void>("openOrderEvents", "hist-1");
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
      .findAll("button")
      .find((candidate) => candidate.text().includes("hist-1"));
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
    expect(call<string>("resolveOrderChipColor", "CANCEL_REQUESTED")).toBe(
      "warning",
    );
  });
});
