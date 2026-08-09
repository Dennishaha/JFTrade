import { ref } from "vue";

import { apiGetPath } from "@/composables/shared/apiClient";
import { isFinalExecutionOrderStatus } from "@/composables/shared/consoleDataFormatting";
import { usePolling } from "@/composables/shared/usePolling";
import { mapExecutionOrderDetails } from "./tradingApiMappers";
import {
  normalizeOptionalText,
  resolveOrderFailureReason,
  type OrderFeedback,
} from "./orderEntryModels";

interface OrderFeedbackNotification {
  level: "warn";
  title: string;
  message: string;
  source: "order-entry";
}

export function useOrderFeedback(
  notify: (notification: OrderFeedbackNotification) => void,
) {
  const lastOrderFeedback = ref<OrderFeedback | null>(null);
  const isRefreshingOrderFeedback = ref(false);
  let pollingOrderFeedbackId = "";

  const orderFeedbackPollIntervalMs = 2_000;
  const orderFeedbackMaxPolls = 60;
  const orderFeedbackPolling = usePolling(
    async () => {
      if (pollingOrderFeedbackId === "") return false;
      const shouldContinue = await refreshOrderFeedbackOnce(
        pollingOrderFeedbackId,
        false,
      );
      if (!shouldContinue) pollingOrderFeedbackId = "";
      return shouldContinue;
    },
    {
      intervalMs: orderFeedbackPollIntervalMs,
      maxRuns: orderFeedbackMaxPolls,
    },
  );

  function stopOrderFeedbackPolling(): void {
    pollingOrderFeedbackId = "";
    orderFeedbackPolling.stop();
  }

  function scheduleOrderFeedbackRefresh(
    internalOrderId: string,
    resetRunCount = false,
  ): void {
    pollingOrderFeedbackId = internalOrderId;
    orderFeedbackPolling.start({ resetRunCount });
  }

  async function refreshOrderFeedbackOnce(
    internalOrderId: string,
    manual: boolean,
  ): Promise<boolean> {
    if (internalOrderId === "") return false;
    if (isRefreshingOrderFeedback.value) return true;
    isRefreshingOrderFeedback.value = true;
    try {
      const details = mapExecutionOrderDetails(
        await apiGetPath(
          "/api/v1/execution/orders/{internalOrderId}",
          `/api/v1/execution/orders/${encodeURIComponent(internalOrderId)}`,
        ),
      );
      const feedback = lastOrderFeedback.value;
      if (feedback == null || feedback.internalOrderId !== internalOrderId) {
        return false;
      }
      feedback.brokerOrderId = normalizeOptionalText(details.order.brokerOrderId);
      feedback.brokerOrderIdEx = normalizeOptionalText(
        details.order.brokerOrderIdEx,
      );
      feedback.orderStatus = normalizeOptionalText(details.order.status);
      feedback.rawBrokerStatus = normalizeOptionalText(
        details.order.rawBrokerStatus,
      );
      feedback.latestEvent = details.recentEvents.at(-1) ?? null;
      feedback.checkedAt = normalizeOptionalText(details.checkedAt);
      return !isFinalExecutionOrderStatus(feedback.orderStatus);
    } catch (error) {
      if (manual) {
        notify({
          level: "warn",
          title: "订单状态刷新失败",
          message: resolveOrderFailureReason(error),
          source: "order-entry",
        });
      }
      return true;
    } finally {
      isRefreshingOrderFeedback.value = false;
    }
  }

  async function refreshOrderFeedback(
    internalOrderId: string,
    manual = false,
  ): Promise<void> {
    const shouldContinue = await refreshOrderFeedbackOnce(
      internalOrderId,
      manual,
    );
    if (shouldContinue) {
      scheduleOrderFeedbackRefresh(internalOrderId);
    } else {
      stopOrderFeedbackPolling();
    }
  }

  function startOrderFeedbackPolling(internalOrderId: string): void {
    scheduleOrderFeedbackRefresh(internalOrderId, true);
  }

  return {
    isRefreshingOrderFeedback,
    lastOrderFeedback,
    orderFeedbackMaxPolls,
    orderFeedbackPollIntervalMs,
    orderFeedbackPolling,
    refreshOrderFeedback,
    refreshOrderFeedbackOnce,
    scheduleOrderFeedbackRefresh,
    startOrderFeedbackPolling,
    stopOrderFeedbackPolling,
  };
}
