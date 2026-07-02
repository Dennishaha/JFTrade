// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

const mocks = vi.hoisted(() => ({
  fetchEnvelopeWithInit: vi.fn(),
  loadHistoricalExecutionOrders: vi.fn(),
  pushNotification: vi.fn(),
}));

let consoleDataState: ReturnType<typeof createConsoleDataState>;
let notificationsState: { push: typeof mocks.pushNotification };

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelopeWithInit: (...args: unknown[]) =>
    mocks.fetchEnvelopeWithInit(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("../src/composables/useNotifications", () => ({
  useNotifications: () => notificationsState,
}));

import PositionsPanel from "../src/components/workspace/PositionsPanel.vue";

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
    filledAveragePrice: 188.12,
    updatedAt: "2026-06-01T10:00:00Z",
    ...overrides,
  };
}

function makeExecutionEvent(overrides: Record<string, unknown> = {}) {
  return {
    id: "event-1",
    internalOrderId: "order-1",
    eventType: "COMMAND_PLACE_ACCEPTED",
    previousStatus: "NEW",
    nextStatus: "SUBMITTED",
    createdAt: "2026-06-01T10:05:00Z",
    ...overrides,
  };
}

function makePosition(overrides: Record<string, unknown> = {}) {
  return {
    brokerId: "futu",
    accountId: "REAL-001",
    tradingEnvironment: "REAL",
    market: "US",
    symbol: "US.AAPL",
    quantity: 100,
    averagePrice: 188.12,
    marketValue: 18812,
    updatedAt: "2026-06-01T10:00:00Z",
    ...overrides,
  };
}

function createConsoleDataState() {
  return {
    brokerOrders: ref({
      connectivity: "connected",
    }),
    activeExecutionOrders: ref({
      orders: [],
    }),
    historicalExecutionOrders: ref({
      orders: [],
    }),
    executionOrderEvents: ref({
      internalOrderId: "",
      events: [],
    }),
    isLoadingBrokerOrders: ref(false),
    isLoadingHistoricalOrders: ref(false),
    historicalOrdersError: ref(""),
    portfolioPositions: ref({
      positions: [],
    }),
    selectedBrokerAccount: ref(null),
    loadHistoricalExecutionOrders: mocks.loadHistoricalExecutionOrders,
    systemStatus: ref({
      defaultTradingEnvironment: "REAL",
    }),
  };
}

function mountPositionsPanel() {
  const wrapper = mount(PositionsPanel, {
    global: {
      stubs: {
        "v-alert": {
          template: "<div class='v-alert'><slot /></div>",
        },
      },
    },
  });
  wrappers.push(wrapper);
  const setup = wrapper.vm.$.setupState as SetupState;
  const call = <T>(name: string, ...args: unknown[]) =>
    (setup[name] as (...values: unknown[]) => T)(...args);
  return { wrapper, setup, call };
}

function setRefValue<T>(target: unknown, value: T): void {
  (target as { value: T }).value = value;
}

function tabButton(wrapper: VueWrapper, label: string) {
  const button = wrapper
    .findAll("button")
    .find((candidate) => candidate.text().startsWith(label));
  expect(button, `tab button ${label} should exist`).toBeTruthy();
  return button!;
}

async function switchTab(wrapper: VueWrapper, label: string): Promise<void> {
  await tabButton(wrapper, label).trigger("click");
  await nextTick();
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

async function flushUi(): Promise<void> {
  await Promise.resolve();
  await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  consoleDataState = createConsoleDataState();
  notificationsState = { push: mocks.pushNotification };
  mocks.fetchEnvelopeWithInit.mockResolvedValue({
    accepted: true,
    message: "撤单请求已发送",
  });
  mocks.loadHistoricalExecutionOrders.mockResolvedValue(undefined);
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
});

describe("PositionsPanel", () => {
  it("renders loaded positions with counts, connectivity, and directional classes", () => {
    setRefValue(consoleDataState.portfolioPositions, {
      positions: [
        makePosition(),
        makePosition({
          accountId: "REAL-002",
          symbol: "US.TSLA",
          quantity: -25,
          averagePrice: 210.55,
          marketValue: -5263.75,
        }),
      ],
    });

    const { wrapper } = mountPositionsPanel();

    expect(tabButton(wrapper, "持仓").text()).toBe("持仓（2）");
    expect(wrapper.text()).toContain("已连接");
    expect(wrapper.text()).toContain("US.AAPL");
    expect(wrapper.text()).toContain("US.TSLA");
    expect(wrapper.findAll("td.tv-num.tv-up")).toHaveLength(1);
    expect(wrapper.findAll("td.tv-num.tv-down")).toHaveLength(1);
  });

  it("keeps tab counts hidden while broker data is loading and then falls back to empty states", async () => {
    setRefValue(consoleDataState.isLoadingBrokerOrders, true);

    const { wrapper } = mountPositionsPanel();

    expect(tabButton(wrapper, "持仓").text()).toBe("持仓");
    expect(tabButton(wrapper, "近期订单").text()).toBe("近期订单");
    expect(tabButton(wrapper, "事件").text()).toBe("事件");
    expect(wrapper.text()).toContain("正在加载持仓...");

    await switchTab(wrapper, "近期订单");
    expect(wrapper.text()).toContain("正在加载近期订单...");
    expect(wrapper.text()).not.toContain("暂无近期订单");

    await switchTab(wrapper, "事件");
    expect(wrapper.text()).toContain("正在加载订单事件...");
    expect(wrapper.text()).not.toContain("暂无事件");

    setRefValue(consoleDataState.isLoadingBrokerOrders, false);
    await nextTick();

    await switchTab(wrapper, "持仓");
    expect(tabButton(wrapper, "持仓").text()).toBe("持仓（0）");
    expect(wrapper.text()).toContain("暂无持仓");
  });

  it("filters active orders by the selected account scope and marks buy and sell sides", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: " real ",
      market: " us ",
    });
    setRefValue(consoleDataState.activeExecutionOrders, {
      orders: [
        makeExecutionOrder({
          internalOrderId: "keep-buy",
          symbol: "US.AAPL",
          side: "BUY",
          status: "SUBMITTED",
        }),
        makeExecutionOrder({
          internalOrderId: "keep-sell",
          symbol: "US.TSLA",
          side: "SELL",
          status: "PENDING_CANCEL",
        }),
        makeExecutionOrder({
          internalOrderId: "other-account",
          accountId: "REAL-002",
          symbol: "US.MSFT",
        }),
        makeExecutionOrder({
          internalOrderId: "other-market",
          market: "HK",
          symbol: "HK.00700",
        }),
        makeExecutionOrder({
          internalOrderId: "other-env",
          tradingEnvironment: "SIMULATE",
          symbol: "US.NVDA",
        }),
        makeExecutionOrder({
          internalOrderId: "filled-order",
          symbol: "US.META",
          status: "FILLED",
        }),
      ],
    });

    const { wrapper } = mountPositionsPanel();
    await switchTab(wrapper, "近期订单");

    expect(tabButton(wrapper, "近期订单").text()).toBe("近期订单（2）");
    expect(wrapper.text()).toContain("keep-buy");
    expect(wrapper.text()).toContain("keep-sell");
    expect(wrapper.text()).not.toContain("other-account");
    expect(wrapper.text()).not.toContain("other-market");
    expect(wrapper.text()).not.toContain("other-env");
    expect(wrapper.text()).not.toContain("filled-order");

    const buyCell = wrapper.findAll("td").find((cell) => cell.text() === "买入");
    const sellCell = wrapper.findAll("td").find((cell) => cell.text() === "卖出");
    expect(buyCell?.classes()).toContain("tv-up");
    expect(sellCell?.classes()).toContain("tv-down");

    const cancelButtons = wrapper
      .findAll("button")
      .filter((button) => button.text() === "撤单");
    expect(cancelButtons).toHaveLength(1);
  });

  it("falls back to the system default trading environment when no account is selected", async () => {
    setRefValue(consoleDataState.systemStatus, {
      defaultTradingEnvironment: "REAL",
    });
    setRefValue(consoleDataState.activeExecutionOrders, {
      orders: [
        makeExecutionOrder({
          internalOrderId: "real-order",
          tradingEnvironment: "REAL",
        }),
        makeExecutionOrder({
          internalOrderId: "simulate-order",
          tradingEnvironment: "SIMULATE",
        }),
      ],
    });

    const { wrapper, call } = mountPositionsPanel();
    await switchTab(wrapper, "近期订单");

    expect(wrapper.text()).toContain("real-order");
    expect(wrapper.text()).not.toContain("simulate-order");
    expect(call<string>("sideClass", null)).toBe("");
    expect(call<string>("sideClass", "SELL_SHORT")).toBe("tv-down");
  });

  it("applies cancelability rules to final and in-flight execution statuses", () => {
    const { call } = mountPositionsPanel();

    expect(
      call<boolean>("canCancelOrder", makeExecutionOrder({ status: "FILLED" })),
    ).toBe(false);
    expect(
      call<boolean>(
        "canCancelOrder",
        makeExecutionOrder({ status: "CANCEL_REQUESTED" }),
      ),
    ).toBe(false);
    expect(
      call<boolean>(
        "canCancelOrder",
        makeExecutionOrder({ status: "PENDING_CANCEL" }),
      ),
    ).toBe(false);
    expect(
      call<boolean>("canCancelOrder", makeExecutionOrder({ status: "CANCELING" })),
    ).toBe(false);
    expect(
      call<boolean>("canCancelOrder", makeExecutionOrder({ status: "SUBMITTED" })),
    ).toBe(true);
  });

  it("does not submit cancel requests for orders that are already final or cancelling", async () => {
    const { call } = mountPositionsPanel();

    await call("cancelOrder", makeExecutionOrder({ status: "FILLED" }));
    await call(
      "cancelOrder",
      makeExecutionOrder({ internalOrderId: "requested", status: "CANCEL_REQUESTED" }),
    );

    expect(mocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();
    expect(mocks.pushNotification).not.toHaveBeenCalled();
  });

  it("submits cancel requests and records a success notification", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    setRefValue(consoleDataState.activeExecutionOrders, {
      orders: [
        makeExecutionOrder({
          internalOrderId: "cancel-me",
          symbol: "US.AAPL",
        }),
      ],
    });
    const pending = deferred<{ accepted: boolean; message: string }>();
    mocks.fetchEnvelopeWithInit.mockReturnValueOnce(pending.promise);

    const { wrapper } = mountPositionsPanel();
    await switchTab(wrapper, "近期订单");

    const cancelButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "撤单");
    expect(cancelButton).toBeTruthy();
    await cancelButton?.trigger("click");
    await nextTick();

    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/execution/orders/cancel-me/cancel",
      { method: "POST" },
    );
    expect(
      wrapper.findAll("button").some((button) => button.text() === "撤单"),
    ).toBe(false);

    pending.resolve({
      accepted: true,
      message: "撤单请求已转交券商处理",
    });
    await flushUi();

    expect(mocks.pushNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        level: "success",
        title: "已提交撤单 US.AAPL",
        message: "撤单请求已转交券商处理",
        source: "positions-panel",
      }),
    );
    expect(
      wrapper.findAll("button").some((button) => button.text() === "撤单"),
    ).toBe(true);
  });

  it("surfaces broker rejection and fallback messages when cancel requests fail", async () => {
    const { call } = mountPositionsPanel();

    mocks.fetchEnvelopeWithInit.mockRejectedValueOnce(new Error("券商拒绝撤单"));
    await call(
      "cancelOrder",
      makeExecutionOrder({
        internalOrderId: "reject-me",
        symbol: "US.TSLA",
      }),
    );

    expect(mocks.pushNotification).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        level: "error",
        title: "撤单失败 US.TSLA",
        message: "券商拒绝撤单",
        source: "positions-panel",
      }),
    );

    mocks.fetchEnvelopeWithInit.mockRejectedValueOnce(new Error("   "));
    await call(
      "cancelOrder",
      makeExecutionOrder({
        internalOrderId: "blank-message",
        symbol: null,
      }),
    );

    expect(mocks.pushNotification).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        level: "error",
        title: "撤单失败 blank-message",
        message: "撤单请求失败。",
        source: "positions-panel",
      }),
    );
  });

  it("lazy loads broker-scoped historical orders once and keeps account and market filters in the query", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    setRefValue(consoleDataState.historicalExecutionOrders, {
      orders: [
        makeExecutionOrder({
          internalOrderId: "filled-1",
          status: "FILLED",
        }),
        makeExecutionOrder({
          internalOrderId: "cancelled-1",
          status: "CANCELLED",
        }),
        makeExecutionOrder({
          internalOrderId: "pending-1",
          status: "SUBMITTED",
        }),
        makeExecutionOrder({
          internalOrderId: "other-market-historical",
          market: "HK",
          symbol: "HK.00700",
          status: "FILLED",
        }),
      ],
    });

    const { wrapper } = mountPositionsPanel();

    await switchTab(wrapper, "历史订单");

    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledTimes(1);
    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery:
        "brokerId=futu&tradingEnvironment=REAL&accountId=REAL-001&market=US",
    });
    expect(tabButton(wrapper, "历史订单").text()).toBe("历史订单（2）");
    expect(wrapper.text()).toContain("filled-1");
    expect(wrapper.text()).toContain("cancelled-1");
    expect(wrapper.text()).not.toContain("pending-1");
    expect(wrapper.text()).not.toContain("other-market-historical");

    await switchTab(wrapper, "持仓");
    await switchTab(wrapper, "历史订单");

    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledTimes(1);
  });

  it("supports broker-wide historical queries when account and market are not selected", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "",
      tradingEnvironment: "REAL",
      market: "",
    });

    const { wrapper } = mountPositionsPanel();
    await switchTab(wrapper, "历史订单");

    expect(mocks.loadHistoricalExecutionOrders).toHaveBeenCalledWith({
      brokerId: "futu",
      brokerQuery: "brokerId=futu&tradingEnvironment=REAL",
    });
  });

  it("shows empty, loading, and error historical states based on the live query state", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });

    const { wrapper } = mountPositionsPanel();
    await switchTab(wrapper, "历史订单");

    expect(wrapper.text()).toContain("暂无历史订单");

    setRefValue(consoleDataState.isLoadingHistoricalOrders, true);
    await nextTick();
    expect(wrapper.text()).toContain("正在加载历史订单...");

    setRefValue(consoleDataState.isLoadingHistoricalOrders, false);
    setRefValue(consoleDataState.historicalOrdersError, "历史订单同步失败");
    await nextTick();
    expect(wrapper.text()).toContain("历史订单同步失败");
  });

  it("marks historical orders as loaded without fetching when no broker account is selected", async () => {
    const { wrapper } = mountPositionsPanel();

    await switchTab(wrapper, "历史订单");

    expect(mocks.loadHistoricalExecutionOrders).not.toHaveBeenCalled();
    expect(tabButton(wrapper, "历史订单").text()).toBe("历史订单（0）");
    expect(wrapper.text()).toContain("暂无历史订单");
  });

  it("shows only events tied to the currently scoped active execution orders", async () => {
    setRefValue(consoleDataState.selectedBrokerAccount, {
      brokerId: "futu",
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "US",
    });
    setRefValue(consoleDataState.activeExecutionOrders, {
      orders: [
        makeExecutionOrder({
          internalOrderId: "active-1",
          symbol: "US.AAPL",
        }),
        makeExecutionOrder({
          internalOrderId: "other-account",
          accountId: "REAL-002",
          symbol: "US.MSFT",
        }),
      ],
    });
    setRefValue(consoleDataState.executionOrderEvents, {
      internalOrderId: "",
      events: [
        makeExecutionEvent({
          id: "event-visible",
          internalOrderId: "active-1",
        }),
        makeExecutionEvent({
          id: "event-hidden",
          internalOrderId: "other-account",
        }),
      ],
    });

    const { wrapper } = mountPositionsPanel();
    await switchTab(wrapper, "事件");

    expect(tabButton(wrapper, "事件").text()).toBe("事件（1）");
    expect(wrapper.text()).toContain("active-1");
    expect(wrapper.text()).not.toContain("other-account");
    expect(wrapper.text()).toContain("下单已受理");
  });
});
