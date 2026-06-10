import type {
  BrokerRuntimeResponse,
  ExecutionOrdersResponse,
  FutuOpenDHealthResponse,
} from "@/contracts";

import type { BrokerAccountSelectionOption } from "./consoleDataBrokerAccountSelection";

export function resolveConsoleDataBrokerLiveSelection(input: {
  activeBrokerId: string;
  selectedAccount: BrokerAccountSelectionOption | null;
  runtime: BrokerRuntimeResponse;
  opendHealth: FutuOpenDHealthResponse;
}) {
  return {
    nextSelectedBrokerAccountKey: input.selectedAccount?.selectionKey ?? null,
    brokerIdForQueries: input.selectedAccount?.brokerId ?? input.activeBrokerId,
    futuBrokerReadsPaused:
      input.runtime.descriptor.id === "futu" &&
      (input.opendHealth.diagnosis.manualRetryRequired ||
        input.runtime.session.connectivity !== "connected"),
  };
}

export function resolveConsoleDataExecutionSelection(input: {
  currentSelectedExecutionOrderId: string;
  executionOrders: ExecutionOrdersResponse;
}) {
  const nextSelectedExecutionOrderId =
    input.executionOrders.orders.find(
      (order) =>
        order.internalOrderId === input.currentSelectedExecutionOrderId,
    )?.internalOrderId ?? input.executionOrders.orders[0]?.internalOrderId ?? "";

  return {
    nextSelectedExecutionOrderId,
    shouldResetExecutionDetails:
      nextSelectedExecutionOrderId !== "" &&
      nextSelectedExecutionOrderId !== input.currentSelectedExecutionOrderId,
    shouldClearExecutionDetails: nextSelectedExecutionOrderId === "",
  };
}