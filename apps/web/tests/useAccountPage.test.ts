// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

const mocks = vi.hoisted(() => ({
  apiGetPath: vi.fn(),
  apiPostPathAction: vi.fn(),
  loadExecutionOrderDetails: vi.fn(),
  loadHistoricalExecutionOrders: vi.fn(),
  loadSystemState: vi.fn(),
  pushNotification: vi.fn(),
  supportsBrokerReadFeature: vi.fn(),
}));

let consoleDataState: Record<string, unknown>;

vi.mock("@/composables/shared/apiClient", () => ({
  apiGetPath: (...args: unknown[]) => mocks.apiGetPath(...args),
  apiPostPathAction: (...args: unknown[]) => mocks.apiPostPathAction(...args),
}));

vi.mock("@/composables/workspace/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("@/composables/shared/useNotifications", () => ({
  useNotifications: () => ({ push: mocks.pushNotification }),
}));

import type { AccountExecutionOrder } from "../src/components/domain/account/ActiveOrdersTable.vue";
import { useAccountPage } from "@/composables/trading/useAccountPage";

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
  } as AccountExecutionOrder;
}

function createConsoleDataState() {
  return {
    activeExecutionOrders: ref({ orders: [] }),
    activeExecutionOrdersError: ref(""),
    historicalExecutionOrders: ref({ orders: [] }),
    brokerFunds: ref({ summary: null }),
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
      session: { connectivity: "connected" },
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
    executionOrderEvents: ref({ internalOrderId: "", events: [] }),
    historicalOrdersError: ref(""),
    isLoadingHistoricalOrders: ref(false),
    loadExecutionOrderDetails: mocks.loadExecutionOrderDetails,
    loadHistoricalExecutionOrders: mocks.loadHistoricalExecutionOrders,
    loadSystemState: mocks.loadSystemState,
    portfolioPositions: ref({ positions: [] }),
    portfolioLiveDataError: ref(""),
    selectedBrokerAccount: ref(null),
    selectedExecutionOrderId: ref(""),
    supportsBrokerReadFeature: mocks.supportsBrokerReadFeature,
    systemStatus: ref({ defaultTradingEnvironment: "REAL" }),
  };
}

function mountAccountPage() {
  let page!: ReturnType<typeof useAccountPage>;
  const wrapper = mount(
    defineComponent({
      setup() {
        page = useAccountPage();
        return () => h("div");
      },
    }),
  );
  wrappers.push(wrapper);
  return { page, wrapper };
}

beforeEach(() => {
  vi.clearAllMocks();
  consoleDataState = createConsoleDataState();
  mocks.loadExecutionOrderDetails.mockResolvedValue(undefined);
  mocks.loadHistoricalExecutionOrders.mockResolvedValue(undefined);
  mocks.loadSystemState.mockResolvedValue(undefined);
  mocks.apiGetPath.mockResolvedValue({ orders: [] });
  mocks.apiPostPathAction.mockResolvedValue({ message: "accepted" });
  mocks.supportsBrokerReadFeature.mockReturnValue(false);
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  window.history.pushState({}, "", "/");
});

describe("useAccountPage", () => {
  it("opens the history tab and loads the requested order from a deep link", async () => {
    window.history.pushState({}, "", "/account?tab=history&orderId=order-9");

    const { page } = mountAccountPage();
    await flushPromises();

    expect(page.activeTab.value).toBe("history");
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery:
        "brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    });
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order-9");
  });

  it("scopes and dedupes visible orders to the active trading environment", async () => {
    (consoleDataState.activeExecutionOrders as { value: unknown }).value = {
      orders: [
        makeExecutionOrder(),
        makeExecutionOrder({ internalOrderId: "order-duplicate" }),
        makeExecutionOrder({
          internalOrderId: "order-sim",
          tradingEnvironment: "SIMULATE",
          accountId: "SIM-001",
        }),
      ],
    };
    (consoleDataState.historicalExecutionOrders as { value: unknown }).value = {
      orders: [makeExecutionOrder({ internalOrderId: "order-filled", status: "FILLED" })],
    };

    const { page } = mountAccountPage();
    await flushPromises();

    expect(page.pendingOrders.value.map((order) => order.internalOrderId)).toEqual([
      "order-1",
    ]);
    expect(page.historicalOrders.value.map((order) => order.internalOrderId)).toEqual([
      "order-filled",
    ]);
    expect(page.activeBrokerReadContext.value).toEqual({
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    expect(page.executionOrdersUrl()).toBe(
      "/api/v1/execution/orders?brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    );
  });

  it("prefers broker positions and derives margin-ratio symbols from them", async () => {
    (consoleDataState.brokerPositions as { value: unknown }).value = {
      checkedAt: "2026-06-01T09:40:00Z",
      positions: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          market: "US",
          symbol: "US.AAPL",
          symbolName: "Apple",
          quantity: 100,
          costPrice: 300,
          averageCostPrice: 310,
          marketValue: 32000,
          unrealizedPnl: 2000,
          currency: "USD",
        },
      ],
    };

    const { page } = mountAccountPage();
    await flushPromises();

    expect(page.accountPositions.value).toEqual([
      expect.objectContaining({
        symbol: "US.AAPL",
        averagePrice: 310,
        source: "券商",
        updatedAt: "2026-06-01T09:40:00Z",
      }),
    ]);
    expect(page.marginRatioSymbols.value).toEqual(["US.AAPL"]);
  });

  it("builds the orders URL from the trading environment when no account scope is known", async () => {
    (consoleDataState.brokerRuntime as { value: unknown }).value = {
      descriptor: { id: "futu", displayName: "Futu", capabilities: [] },
      session: { connectivity: "disconnected" },
      accounts: [],
    };

    const { page } = mountAccountPage();
    await flushPromises();

    expect(page.activeBrokerReadContext.value).toBeNull();
    expect(page.executionOrdersUrl()).toBe(
      "/api/v1/execution/orders?tradingEnvironment=REAL",
    );

    (consoleDataState.systemStatus as { value: unknown }).value = {};
    expect(page.executionOrdersUrl()).toBe("/api/v1/execution/orders");
    expect(page.orderMatchesTradingEnvironment("REAL")).toBe(false);
  });

  it("gates cancel confirmation on order state and surfaces the broker result", async () => {
    const order = makeExecutionOrder({ internalOrderId: "order/cancel" });
    const { page } = mountAccountPage();
    await flushPromises();

    expect(page.canCancelOrder(order)).toBe(true);
    expect(page.canCancelOrder(makeExecutionOrder({ status: "FILLED" }))).toBe(false);
    expect(
      page.canCancelOrder(makeExecutionOrder({ status: "PENDING_CANCEL" })),
    ).toBe(false);

    page.requestCancelOrder(makeExecutionOrder({ status: "FILLED" }));
    expect(page.pendingCancelOrder.value).toBeNull();

    page.requestCancelOrder(order);
    expect(page.pendingCancelOrder.value).toEqual(order);
    expect(page.pendingCancelMessage.value).toContain("确认撤销订单 US.AAPL");

    mocks.apiPostPathAction.mockResolvedValueOnce({ message: "撤单已提交" });
    page.confirmCancelOrder();
    await flushPromises();

    expect(page.pendingCancelOrder.value).toBeNull();
    expect(mocks.apiPostPathAction).toHaveBeenCalledWith(
      "/api/v1/execution/orders/{internalOrderId}/cancel",
      "/api/v1/execution/orders/order%2Fcancel/cancel",
    );
    expect(mocks.apiGetPath).toHaveBeenCalledWith(
      "/api/v1/execution/orders",
      expect.stringContaining("/api/v1/execution/orders?"),
    );
    expect(mocks.pushNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "success",
        message: "撤单已提交",
        source: "account-page",
      }),
    );
    expect(page.isCancellingOrder("order/cancel")).toBe(false);
  });

  it("routes combo cancels to the combos endpoint and reports failures", async () => {
    const comboOrder = makeExecutionOrder({
      internalOrderId: "combo/1",
      orderKind: "option_combo",
    });
    const { page } = mountAccountPage();
    await flushPromises();

    await page.cancelOrder(comboOrder);
    expect(mocks.apiPostPathAction).toHaveBeenCalledWith(
      "/api/v1/execution/combos/{internalOrderId}/cancel",
      "/api/v1/execution/combos/combo%2F1/cancel",
    );

    mocks.apiPostPathAction.mockRejectedValueOnce(new Error(" "));
    await page.cancelOrder(makeExecutionOrder());
    expect(mocks.pushNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "error",
        message: "撤单请求失败。",
        source: "account-page",
      }),
    );

    mocks.apiPostPathAction.mockClear();
    await page.cancelOrder(makeExecutionOrder({ status: "FILLED" }));
    expect(mocks.apiPostPathAction).not.toHaveBeenCalled();
  });

  it("marks an order as cancelling while the cancel request is in flight", async () => {
    const order = makeExecutionOrder();
    let releaseCancel: ((value: { message: string }) => void) | undefined;
    mocks.apiPostPathAction.mockImplementationOnce(
      () =>
        new Promise<{ message: string }>((resolve) => {
          releaseCancel = resolve;
        }),
    );

    const { page } = mountAccountPage();
    await flushPromises();

    const pending = page.cancelOrder(order);
    expect(page.isCancellingOrder("order-1")).toBe(true);
    expect(page.canCancelOrder(order)).toBe(false);

    releaseCancel?.({ message: "撤单已提交" });
    await pending;
    expect(page.isCancellingOrder("order-1")).toBe(false);
  });

  it("refreshes account data in the background on an interval and stops on unmount", async () => {
    vi.useFakeTimers();
    try {
      const { wrapper } = mountAccountPage();

      await vi.advanceTimersByTimeAsync(5_000);
      expect(mocks.loadSystemState).toHaveBeenCalledWith({ background: true });

      mocks.loadSystemState.mockClear();
      wrapper.unmount();
      await vi.advanceTimersByTimeAsync(60_000);
      expect(mocks.loadSystemState).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("skips background refreshes while the page is hidden", async () => {
    vi.useFakeTimers();
    const visibilityState = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("hidden");
    try {
      mountAccountPage();

      await vi.advanceTimersByTimeAsync(10_000);
      expect(mocks.loadSystemState).not.toHaveBeenCalled();

      visibilityState.mockReturnValue("visible");
      await vi.advanceTimersByTimeAsync(5_000);
      expect(mocks.loadSystemState).toHaveBeenCalledTimes(1);
    } finally {
      visibilityState.mockRestore();
      vi.useRealTimers();
    }
  });

  it("prevents overlapping manual refreshes and reloads history every time", async () => {
    let releaseSystemState: (() => void) | undefined;
    mocks.loadSystemState.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          releaseSystemState = resolve;
        }),
    );

    const { page } = mountAccountPage();
    await flushPromises();

    const first = page.refreshAccountData();
    const second = page.refreshAccountData();
    releaseSystemState?.();
    await Promise.all([first, second]);

    expect(mocks.loadSystemState).toHaveBeenCalledTimes(1);
    expect(mocks.loadSystemState).toHaveBeenCalledWith({ bypassCooldown: true });
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery:
        "brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    });
    expect(page.isRefreshingAccount.value).toBe(false);
  });

  it("lazy-loads history once and pages through the display limit", async () => {
    (consoleDataState.historicalExecutionOrders as { value: unknown }).value = {
      orders: Array.from({ length: 60 }, (_, index) =>
        makeExecutionOrder({
          internalOrderId: `hist-${index + 1}`,
          brokerOrderId: `hist-broker-${index + 1}`,
          status: "FILLED",
        }),
      ),
    };

    const { page } = mountAccountPage();
    await flushPromises();

    page.ensureHistoricalOrdersLoaded();
    page.ensureHistoricalOrdersLoaded();
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledTimes(1);

    expect(page.displayedHistoricalOrders.value).toHaveLength(50);
    expect(page.hasMoreHistoricalOrders.value).toBe(true);

    page.loadMoreHistoricalOrders();
    expect(page.displayedHistoricalOrders.value).toHaveLength(60);
    expect(page.hasMoreHistoricalOrders.value).toBe(false);
  });
});
