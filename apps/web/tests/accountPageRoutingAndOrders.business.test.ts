// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";

import type { AccountExecutionOrder } from "../src/components/domain/account/ActiveOrdersTable.vue";
import {
  dedupeExecutionOrders,
  initialAccountTabFromLocation,
} from "../src/features/accountPage";

function executionOrder(
  internalOrderId: string,
  brokerOrderId: string,
  brokerOrderIdEx: string,
): AccountExecutionOrder {
  return {
    brokerId: "futu",
    accountId: "SIM-001",
    tradingEnvironment: "SIMULATE",
    market: "US",
    internalOrderId,
    brokerOrderId,
    brokerOrderIdEx,
    symbol: "US.AAPL",
    symbolName: "Apple",
    side: "BUY",
    orderType: "LIMIT",
    source: "broker",
    sourceDetail: "manual",
    status: "SUBMITTED",
    requestedQuantity: 1,
    filledQuantity: 0,
    updatedAt: "2026-07-29T00:00:00Z",
  };
}

afterEach(() => {
  window.history.replaceState({}, "", "/");
});

describe("account page routing and order identity", () => {
  it("opens order history for a deep link without an explicit tab", () => {
    window.history.replaceState({}, "", "/account");
    expect(initialAccountTabFromLocation("internal-order-1")).toBe("history");
  });

  it("deduplicates orders with extended and internal broker identity fallbacks", () => {
    const orders = [
      executionOrder("internal-a", "", "extended-a"),
      executionOrder("internal-b", "", "extended-a"),
      executionOrder("internal-c", "", ""),
      executionOrder("internal-c", "", ""),
    ];

    expect(dedupeExecutionOrders(orders).map((order) => order.internalOrderId)).toEqual([
      "internal-a",
      "internal-c",
    ]);
  });
});
