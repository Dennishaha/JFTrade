import { computed, type Ref } from "vue";

import {
  type BrokerOrderFeesResponse,
  type BrokerReadFeatureKey,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  emptyBrokerOrderFees,
  emptyExecutionOrderEvents,
} from "@/contracts";

import { fetchEnvelope } from "./apiClient";

interface CreateConsoleDataExecutionOrdersControllerOptions {
  activeExecutionOrders: Ref<ExecutionOrdersResponse>;
  historicalExecutionOrders: Ref<ExecutionOrdersResponse>;
  executionOrderEvents: Ref<ExecutionOrderEventsResponse>;
  selectedExecutionOrderId: Ref<string>;
  isLoadingExecutionEvents: Ref<boolean>;
  isLoadingOrderFees: Ref<boolean>;
  executionEventsError: Ref<string>;
  brokerOrderFees: Ref<BrokerOrderFeesResponse>;
  orderFeesError: Ref<string>;
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
}

export function createConsoleDataExecutionOrdersController(
  options: CreateConsoleDataExecutionOrdersControllerOptions,
) {
  const selectedExecutionOrder = computed(() => {
    const id = options.selectedExecutionOrderId.value;
    if (id === "") return null;
    // Search active first, then historical
    return (
      options.activeExecutionOrders.value.orders.find(
        (order) => order.internalOrderId === id,
      ) ??
      options.historicalExecutionOrders.value.orders.find(
        (order) => order.internalOrderId === id,
      ) ??
      null
    );
  });

  function findOrderById(internalOrderId: string) {
    return (
      options.activeExecutionOrders.value.orders.find(
        (candidate) => candidate.internalOrderId === internalOrderId,
      ) ??
      options.historicalExecutionOrders.value.orders.find(
        (candidate) => candidate.internalOrderId === internalOrderId,
      ) ??
      null
    );
  }

  async function loadExecutionOrderEvents(
    internalOrderId: string,
  ): Promise<void> {
    options.executionEventsError.value = "";
    options.isLoadingExecutionEvents.value = true;

    try {
      options.executionOrderEvents.value =
        await fetchEnvelope<ExecutionOrderEventsResponse>(
          `/api/v1/execution/orders/${encodeURIComponent(internalOrderId)}/events`,
        );
    } catch (error) {
      options.executionEventsError.value =
        error instanceof Error
          ? error.message
          : "订单事件加载失败。";
      options.executionOrderEvents.value = {
        ...emptyExecutionOrderEvents,
        internalOrderId,
      };
    } finally {
      options.isLoadingExecutionEvents.value = false;
    }
  }

  async function loadExecutionOrderFees(
    internalOrderId: string,
  ): Promise<void> {
    options.orderFeesError.value = "";
    options.brokerOrderFees.value = emptyBrokerOrderFees;

    const order = findOrderById(internalOrderId);
    const brokerOrderIdEx = order?.brokerOrderIdEx?.trim();

    if (order == null) {
      return;
    }

    const requirements = options.resolveBrokerReadFeatureQueryRequirements(
      "orderFees",
      {
        market: order.market,
        tradingEnvironment: order.tradingEnvironment,
      },
    );

    if (
      !requirements.supported ||
      !options.supportsBrokerReadFeature("orderFees", {
        market: order.market,
        tradingEnvironment: order.tradingEnvironment,
      })
    ) {
      return;
    }

    if (
      requirements.requiresOrderIdEx &&
      (brokerOrderIdEx == null || brokerOrderIdEx === "")
    ) {
      options.orderFeesError.value = "当前订单缺少券商扩展单号，暂无法查询费用。";
      return;
    }

    if (brokerOrderIdEx == null || brokerOrderIdEx === "") {
      return;
    }

    options.isLoadingOrderFees.value = true;

    try {
      options.brokerOrderFees.value = await fetchEnvelope<BrokerOrderFeesResponse>(
        `/api/v1/brokers/${encodeURIComponent(order.brokerId)}/order-fees?tradingEnvironment=${encodeURIComponent(order.tradingEnvironment)}&accountId=${encodeURIComponent(order.accountId)}&market=${encodeURIComponent(order.market)}&orderIdEx=${encodeURIComponent(brokerOrderIdEx)}`,
      );
    } catch (error) {
      options.orderFeesError.value =
        error instanceof Error
          ? error.message
          : "券商订单费用加载失败。";
    } finally {
      options.isLoadingOrderFees.value = false;
    }
  }

  async function loadExecutionOrderDetails(
    internalOrderId: string,
  ): Promise<void> {
    options.selectedExecutionOrderId.value = internalOrderId;

    await Promise.all([
      loadExecutionOrderEvents(internalOrderId),
      loadExecutionOrderFees(internalOrderId),
    ]);
  }

  return {
    loadExecutionOrderDetails,
    selectedExecutionOrder,
  };
}
