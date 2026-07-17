import { ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type ExecutionOrderSummaryResponse,
  type ExecutionOrdersResponse,
  emptyBrokerOrderFees,
  emptyExecutionOrderEvents,
  emptyExecutionOrders,
} from "@/contracts";
import { createConsoleDataExecutionOrdersController } from "../src/composables/consoleDataExecutionOrders";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

function buildOrder(): ExecutionOrderSummaryResponse {
  return {
    internalOrderId: "order-1",
    brokerId: "futu",
    brokerOrderId: "broker-1",
    brokerOrderIdEx: "ex-1",
    source: "broker",
    sourceDetail: "broker.current",
    tradingEnvironment: "SIMULATE",
    accountId: "account-1",
    market: "HK",
    symbol: "HK.00700",
    side: "BUY",
    orderType: "LIMIT",
    status: "FILLED",
    requestedQuantity: 100,
    requestedPrice: 380,
    filledQuantity: 100,
    filledAveragePrice: 380,
    remark: null,
    lastError: null,
    lastErrorCode: null,
    lastErrorSource: null,
    submittedAt: "2026-07-16T00:00:00Z",
    updatedAt: "2026-07-16T00:00:00Z",
    createdAt: "2026-07-16T00:00:00Z",
  };
}

function createController(options: {
  orders?: ExecutionOrderSummaryResponse[];
  feesSupported?: boolean;
} = {}) {
  const activeExecutionOrders = ref<ExecutionOrdersResponse>({
    ...emptyExecutionOrders,
    orders: options.orders ?? [],
  });
  const state = {
    activeExecutionOrders,
    historicalExecutionOrders: ref<ExecutionOrdersResponse>(emptyExecutionOrders),
    executionOrderEvents: ref(emptyExecutionOrderEvents),
    selectedExecutionOrderId: ref(""),
    isLoadingExecutionEvents: ref(false),
    isLoadingOrderFees: ref(false),
    executionEventsError: ref(""),
    brokerOrderFees: ref(emptyBrokerOrderFees),
    orderFeesError: ref(""),
    resolveBrokerReadFeatureQueryRequirements: vi.fn(() => ({
      supported: options.feesSupported ?? true,
      supportsHistory: false,
      requiresSymbols: false,
      requiresClearingDate: false,
      requiresPrice: false,
      requiresOrderIdEx: true,
    })),
    supportsBrokerReadFeature: vi.fn(() => options.feesSupported ?? true),
  };
  return {
    controller: createConsoleDataExecutionOrdersController(state),
    state,
  };
}

describe("execution-order detail fallbacks", () => {
  it("clears stale event data when an unknown order's event endpoint fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw "network disconnected";
    }));
    const { controller, state } = createController();

    await controller.loadExecutionOrderDetails("missing-order");

    expect(state.selectedExecutionOrderId.value).toBe("missing-order");
    expect(state.executionEventsError.value).toBe("订单事件加载失败。");
    expect(state.executionOrderEvents.value).toEqual({
      internalOrderId: "missing-order",
      events: [],
    });
    expect(state.isLoadingOrderFees.value).toBe(false);
    expect(state.resolveBrokerReadFeatureQueryRequirements).not.toHaveBeenCalled();
  });

  it("does not ask a broker for fees when that market/environment feature is unavailable", async () => {
    const fetchMock = vi.fn(async () => createResponse({
      internalOrderId: "order-1",
      events: [],
    }));
    vi.stubGlobal("fetch", fetchMock);
    const { controller, state } = createController({
      orders: [buildOrder()],
      feesSupported: false,
    });

    await controller.loadExecutionOrderDetails("order-1");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(state.resolveBrokerReadFeatureQueryRequirements).toHaveBeenCalledWith(
      "orderFees",
      { market: "HK", tradingEnvironment: "SIMULATE" },
    );
    expect(state.brokerOrderFees.value).toEqual(emptyBrokerOrderFees);
    expect(state.orderFeesError.value).toBe("");
  });

  it("preserves a friendly fee error when the broker transport rejects a non-Error value", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/events")) {
        return createResponse({ internalOrderId: "order-1", events: [] });
      }
      throw "fees temporarily unavailable";
    }));
    const { controller, state } = createController({ orders: [buildOrder()] });

    await controller.loadExecutionOrderDetails("order-1");

    expect(state.executionEventsError.value).toBe("");
    expect(state.orderFeesError.value).toBe("券商订单费用加载失败。");
    expect(state.isLoadingOrderFees.value).toBe(false);
  });
});
