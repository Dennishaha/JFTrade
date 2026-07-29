import type { AccountExecutionOrder } from "../components/domain/account/ActiveOrdersTable.vue";

export type AccountTab = "positions" | "orders" | "history" | "funds";

export const ACCOUNT_TABS: ReadonlyArray<{
  value: AccountTab;
  label: string;
}> = [
  { value: "positions", label: "持仓" },
  { value: "orders", label: "订单" },
  { value: "history", label: "历史" },
  { value: "funds", label: "资金" },
];

export function initialExecutionOrderIdFromLocation(): string {
  if (typeof window === "undefined") return "";
  return new URLSearchParams(window.location.search).get("orderId")?.trim() ?? "";
}

export function normalizeAccountTab(
  raw: string | null | undefined,
): AccountTab | null {
  switch (raw) {
    case "positions":
    case "orders":
    case "history":
    case "funds":
      return raw;
    default:
      return null;
  }
}

export function initialAccountTabFromLocation(orderId: string): AccountTab {
  if (typeof window === "undefined") return "positions";
  const requestedTab = normalizeAccountTab(
    new URLSearchParams(window.location.search).get("tab")?.trim(),
  );
  return requestedTab ?? (orderId === "" ? "positions" : "history");
}

function executionOrderDisplayKey(order: AccountExecutionOrder): string {
  const brokerOrderIdentity =
    order.brokerOrderId?.trim() ||
    order.brokerOrderIdEx?.trim() ||
    order.internalOrderId;
  return [
    order.brokerId,
    order.accountId,
    order.tradingEnvironment,
    order.market,
    brokerOrderIdentity,
  ].join("|");
}

export function dedupeExecutionOrders(
  orders: AccountExecutionOrder[],
): AccountExecutionOrder[] {
  const seen = new Set<string>();
  return orders.filter((order) => {
    const key = executionOrderDisplayKey(order);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}
