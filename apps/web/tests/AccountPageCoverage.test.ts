// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
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
  const wrapper = mount(AccountPage, {
    global: {
      stubs: {
        PageHeader: { template: "<header><slot /></header>" },
        SectionHeader: { template: "<section><slot /></section>" },
        TradingScopeBar: true,
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
  window.history.pushState({}, "", "/");
});

describe("AccountPage coverage boundaries", () => {
  it("uses funds context when present and falls back to the active environment when it is absent", async () => {
    const { setup, wrapper } = mountAccountPage();
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
    expect(read<boolean>(setup.supportsSelectedExecutionOrderFees)).toBe(false);
    wrapper.unmount();
  });

  it("opens pending-order events and submits cancellation through the account-scoped refresh path", async () => {
    const order = createOrder();
    read<{ orders: unknown[] }>(consoleDataState.activeExecutionOrders).orders = [order];
    const { wrapper } = mountAccountPage();
    await nextTick();

    const eventButton = wrapper.findAll("button").find((button) => button.text() === "查看事件");
    const cancelButton = wrapper.findAll("button").find((button) => button.text() === "撤单");
    expect(eventButton).toBeDefined();
    expect(cancelButton).toBeDefined();
    await eventButton?.trigger("click");
    await cancelButton?.trigger("click");

    expect(mocks.loadExecutionOrderDetails).toHaveBeenCalledWith("order-1");
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
    wrapper.unmount();
  });
});
