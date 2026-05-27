// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerFunds,
  emptyBrokerOrderFees,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrderEvents,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@jftrade/ui-contracts";
import type {
  BrokerOrderFeesResponse,
  ExecutionOrderEventsResponse,
  ExecutionOrderSummaryResponse,
  ExecutionOrdersResponse,
} from "@jftrade/ui-contracts";

import { MockEventSource, createResponse, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

describe("Account page execution route redirect", () => {
  it("shows pending orders, order events, and broker order fees", async () => {
    const executionOrders: ExecutionOrdersResponse = {
      ...emptySystemStatus,
      orders: [
        {
          internalOrderId: "order-internal-1",
          brokerId: "futu",
          brokerOrderId: "9001",
          brokerOrderIdEx: "ex-9001",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "HK",
          symbol: "HK.00700",
          side: "BUY",
          orderType: "NORMAL",
          status: "SUBMITTED",
          requestedQuantity: 100,
          requestedPrice: 320,
          filledQuantity: 50,
          filledAveragePrice: 319.5,
          remark: null,
          lastError: "modify rejected by broker",
          lastErrorCode: "MODIFY_REJECTED",
          lastErrorSource: "command.modify.broker",
          submittedAt: "2026-05-16 10:00:00",
          updatedAt: "2026-05-16 10:01:00",
          createdAt: "2026-05-16T00:00:00.000Z",
        },
      ] satisfies ExecutionOrderSummaryResponse[],
    };

    const executionOrderEvents: ExecutionOrderEventsResponse = {
      ...emptyExecutionOrderEvents,
      internalOrderId: "order-internal-1",
      events: [
        {
          id: "event-1",
          internalOrderId: "order-internal-1",
          eventType: "COMMAND_PLACE_ACCEPTED",
          previousStatus: null,
          nextStatus: "SUBMITTED",
          payloadJson: '{"ok":true}',
          createdAt: "2026-05-16T00:00:00.000Z",
        },
      ],
    };

    const brokerOrderFees: BrokerOrderFeesResponse = {
      ...emptyBrokerOrderFees,
      checkedAt: "2026-05-16T00:01:00.000Z",
      connectivity: "connected",
      fees: [
        {
          brokerOrderId: "9001",
          totalFee: 18.5,
          currency: "HKD",
          details: [{ title: "Commission", amount: 10 }],
        },
      ],
    };

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status"))
        return createResponse(emptySystemStatus);
      if (url.includes("/api/v1/system/storage/overview"))
        return createResponse(emptyStorageOverview);
      if (url.includes("/api/v1/system/real-trade-approvals"))
        return createResponse(emptyRealTradeApprovals);
      if (url.includes("/api/v1/system/real-trade-hard-stops"))
        return createResponse(emptyRealTradeHardStops);
      if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
        return createResponse(emptyRealTradeHardStopEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
        return createResponse(emptyRealTradeKillSwitchEvents);
      if (url.includes("/api/v1/system/real-trade-kill-switch"))
        return createResponse(emptyRealTradeKillSwitchState);
      if (url.includes("/api/v1/system/real-trade-risk-events"))
        return createResponse(emptyRealTradeRiskEvents);
      if (url.includes("/api/v1/system/real-trade-risk-limits"))
        return createResponse(emptyRealTradeRiskState);
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);
      if (url.includes("/api/v1/brokers/futu/runtime"))
        return createResponse(emptyBrokerRuntime);
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse(emptyBrokerFunds);
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(emptyBrokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders"))
        return createResponse(emptyBrokerOrders);
      if (url.includes("/api/v1/brokers/futu/order-fees"))
        return createResponse(brokerOrderFees);
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(emptyPortfolioPositions);
      if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
        return createResponse(emptyPortfolioCashReconciliation);
      if (url.includes("/api/v1/portfolio/futu/reconciliation"))
        return createResponse(emptyPortfolioReconciliation);
      if (url.includes("/api/v1/execution/orders/order-internal-1/events"))
        return createResponse(executionOrderEvents);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(executionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/execution");

    expect(wrapper.text()).toContain("我的账户");
    expect(wrapper.text()).toContain("在途订单");
    expect(wrapper.text()).toContain("下单已受理");
    expect(wrapper.text()).toContain("18.5 HKD");

    wrapper.unmount();
  });
});
