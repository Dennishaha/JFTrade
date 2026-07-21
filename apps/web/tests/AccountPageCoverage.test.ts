// @vitest-environment jsdom

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

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

vi.mock("../src/composables/apiClient", () => ({
  fetchEnvelope: (...args: unknown[]) => mocks.fetchEnvelope(...args),
  fetchEnvelopeWithInit: (...args: unknown[]) => mocks.fetchEnvelopeWithInit(...args),
}));

vi.mock("../src/composables/useConsoleData", () => ({
  useConsoleData: () => consoleDataState,
}));

vi.mock("../src/composables/useNotifications", () => ({
  useNotifications: () => ({ push: mocks.pushNotification }),
}));

const wrappers: VueWrapper[] = [];

function createOrder(overrides: Record<string, unknown> = {}) {
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
    updatedAt: "2026-07-16T10:00:00Z",
    ...overrides,
  };
}

function createConsoleDataState() {
  return {
    brokerCashFlows: ref({ connectivity: "connected", cashFlows: [], lastError: "" }),
    brokerFills: ref({ fills: [], lastError: "" }),
    brokerFunds: ref({ summary: null as Record<string, unknown> | null }),
    brokerMarginRatios: ref({ connectivity: "connected", marginRatios: [], lastError: "" }),
    brokerOrderFees: ref({ fees: [] }),
    brokerPositions: ref({ checkedAt: "", positions: [] }),
    brokerRuntime: ref({
      descriptor: { id: "futu", displayName: "Futu", capabilities: [{ market: "US" }] },
      session: { connectivity: "connected" },
      accounts: [
        {
          accountId: "REAL-001",
          tradingEnvironment: "REAL",
          accountType: "CASH",
          securityFirm: "FUTU",
          marketAuthorities: ["US"],
        },
      ],
    }),
    executionEventsError: ref(""),
    executionOrderEvents: ref({ internalOrderId: "", events: [] }),
    activeExecutionOrders: ref({ orders: [] as unknown[] }),
    historicalExecutionOrders: ref({ orders: [] as unknown[] }),
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
    portfolioCashBalances: ref({ balances: [] }),
    portfolioPositions: ref({ positions: [] }),
    resolveBrokerReadFeatureQueryRequirements: mocks.resolveBrokerReadFeatureQueryRequirements,
    selectedBrokerAccount: ref(null),
    selectedExecutionOrder: ref(null),
    selectedExecutionOrderId: ref(""),
    supportsBrokerReadFeature: mocks.supportsBrokerReadFeature,
    systemStatus: ref({ defaultTradingEnvironment: "REAL" }),
  };
}

function mountAccountPage() {
  const wrapper = mount(AccountPage);
  wrappers.push(wrapper);
  return {
    wrapper,
    setup: wrapper.vm.$.setupState as Record<string, unknown>,
  };
}

function read<T>(value: unknown): T {
  return value !== null && typeof value === "object" && "value" in value
    ? (value as { value: T }).value
    : value as T;
}

async function activateTab(wrapper: VueWrapper, label: string) {
  const tab = wrapper
    .findAll('button[role="tab"]')
    .find((candidate) => candidate.text().includes(label));
  expect(tab, `tab ${label}`).toBeDefined();
  await tab!.trigger("click");
  await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  consoleDataState = createConsoleDataState();
  mocks.fetchEnvelope.mockResolvedValue({ orders: [] });
  mocks.fetchEnvelopeWithInit.mockResolvedValue({ message: "撤单已接受" });
  mocks.loadExecutionOrderDetails.mockResolvedValue(undefined);
  mocks.loadHistoricalExecutionOrders.mockResolvedValue(undefined);
  mocks.supportsBrokerReadFeature.mockReturnValue(false);
  mocks.resolveBrokerReadFeatureQueryRequirements.mockReturnValue({ supportsHistory: false });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  window.history.pushState({}, "", "/");
});

describe("AccountPage coverage boundaries", () => {
  it("uses funds context when present and falls back to the active environment when it is absent", async () => {
    const { setup } = mountAccountPage();
    read<{ summary: Record<string, unknown> | null }>(consoleDataState.brokerFunds).summary = {
      accountId: "SIM-9",
      tradingEnvironment: "SIMULATE",
      market: "HK",
    };
    await (setup.refreshExecutionOrders as () => Promise<void>)();
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      expect.stringContaining("accountId=SIM-9"),
    );

    read<{ summary: Record<string, unknown> | null }>(consoleDataState.brokerFunds).summary = null;
    read<{ accounts: unknown[] }>(consoleDataState.brokerRuntime).accounts = [];
    await (setup.refreshExecutionOrders as () => Promise<void>)();
    expect(mocks.fetchEnvelope).toHaveBeenLastCalledWith(
      expect.stringContaining("tradingEnvironment=REAL"),
    );
    // 没有选中订单时，历史详情侧栏展示费用占位提示而不是费用能力错误。
    await activateTab(wrappers[0]!, "历史");
    expect(wrappers[0]!.text()).toContain("请选择一笔订单");
  });

  it("opens pending-order events and submits cancellation through the account-scoped refresh path", async () => {
    const order = createOrder();
    read<{ orders: unknown[] }>(consoleDataState.activeExecutionOrders).orders = [order];
    const { wrapper } = mountAccountPage();
    await nextTick();

    await activateTab(wrapper, "订单");
    const eventButton = wrapper.findAll("button").find((button) => button.text() === "查看事件");
    expect(eventButton).toBeDefined();
    await eventButton?.trigger("click");
    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order-1");
    expect(read<string>(wrapper.vm.$.setupState.activeTab)).toBe("history");

    await activateTab(wrapper, "订单");
    const cancelButton = wrapper.findAll("button").find((button) => button.text() === "撤单");
    expect(cancelButton).toBeDefined();
    await cancelButton?.trigger("click");

    expect(mocks.fetchEnvelopeWithInit).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("确认撤销订单");
    await wrapper.get('[data-testid="action-confirm-submit"]').trigger("click");
    expect(mocks.fetchEnvelopeWithInit).toHaveBeenCalledWith(
      "/api/v1/execution/orders/order-1/cancel",
      { method: "POST" },
    );
    await vi.waitFor(() =>
      expect(mocks.pushNotification).toHaveBeenCalledWith(
        expect.objectContaining({ title: expect.stringContaining("已提交撤单") }),
      ),
    );
  });
});
