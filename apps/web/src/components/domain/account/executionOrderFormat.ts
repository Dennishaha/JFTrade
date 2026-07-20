import type { ExecutionOrdersResponse } from "@/contracts";

import {
  formatExecutionOrderStatusLabel,
  formatOrderTypeLabel,
  isFinalExecutionOrderStatus,
} from "../../../composables/consoleDataFormatting";

export type AccountExecutionOrder = ExecutionOrdersResponse["orders"][number];

export function formatOrderKind(order: AccountExecutionOrder): string {
  switch (order.orderKind) {
    case "option_combo":
      return "期权组合";
    case "event_single":
      return "预测单腿";
    case "event_parlay":
      return "预测 Parlay";
    default:
      return formatOrderTypeLabel(order.orderType);
  }
}

export function orderStatusClass(status: string): string {
  const normalized = status.toUpperCase();
  if (normalized.includes("REJECT") || normalized.includes("FAIL")) {
    return "tv-status--error";
  }
  if (isFinalExecutionOrderStatus(status)) {
    if (normalized === "FILLED") return "tv-status--success";
    if (normalized === "EXPIRED") return "tv-status--warning";
    return "tv-status--info";
  }
  if (normalized.includes("CANCEL")) {
    return "tv-status--warning";
  }
  return "tv-status--info";
}

export function formatExecutionStatusTransition(
  previousStatus: string | null | undefined,
  nextStatus: string | null | undefined,
): string {
  const nextLabel = formatExecutionOrderStatusLabel(nextStatus);
  if ((previousStatus ?? "").trim() === "") {
    return `首次发现，当前状态：${nextLabel}`;
  }
  return `${formatExecutionOrderStatusLabel(previousStatus)} → ${nextLabel}`;
}
