import { computed, type Ref } from "vue";

import {
  type BrokerOrderFeesResponse,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  emptyBrokerOrderFees,
  emptyExecutionOrderEvents,
} from "@jftrade/ui-contracts";

import { fetchEnvelope } from "./apiClient";

interface CreateConsoleDataExecutionOrdersControllerOptions {
  executionOrders: Ref<ExecutionOrdersResponse>;
  executionOrderEvents: Ref<ExecutionOrderEventsResponse>;
  selectedExecutionOrderId: Ref<string>;
  isLoadingExecutionEvents: Ref<boolean>;
  isLoadingOrderFees: Ref<boolean>;
  executionEventsError: Ref<string>;
  brokerOrderFees: Ref<BrokerOrderFeesResponse>;
  orderFeesError: Ref<string>;
}

export function createConsoleDataExecutionOrdersController(
  options: CreateConsoleDataExecutionOrdersControllerOptions,
) {
  const selectedExecutionOrder = computed(
    () =>
      options.executionOrders.value.orders.find(
        (order) => order.internalOrderId === options.selectedExecutionOrderId.value,
      ) ?? null,
  );

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
          : "Failed to load execution order events.";
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

    const order = options.executionOrders.value.orders.find(
      (candidate) => candidate.internalOrderId === internalOrderId,
    );

    if (
      order == null ||
      order.brokerOrderId == null ||
      order.tradingEnvironment !== "REAL"
    ) {
      return;
    }

    options.isLoadingOrderFees.value = true;

    try {
      options.brokerOrderFees.value = await fetchEnvelope<BrokerOrderFeesResponse>(
        `/api/v1/brokers/${encodeURIComponent(order.brokerId)}/order-fees?tradingEnvironment=${encodeURIComponent(order.tradingEnvironment)}&accountId=${encodeURIComponent(order.accountId)}&market=${encodeURIComponent(order.market)}&orderId=${encodeURIComponent(order.brokerOrderId)}`,
      );
    } catch (error) {
      options.orderFeesError.value =
        error instanceof Error
          ? error.message
          : "Failed to load broker order fees.";
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