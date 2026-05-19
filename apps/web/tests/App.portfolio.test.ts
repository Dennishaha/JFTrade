// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
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
  PortfolioCashBalancesResponse,
  PortfolioCashReconciliationResponse,
  PortfolioPositionsResponse,
  PortfolioReconciliationResponse,
} from "@jftrade/ui-contracts";

import { MockEventSource, createResponse, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

describe("Portfolio page", () => {
  it("shows projected cash balances and portfolio reconciliation", async () => {
    const portfolioCashBalances: PortfolioCashBalancesResponse = {
      ...emptySystemStatus,
      balances: [
        {
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          currency: "HKD",
          cashBalance: 55981.5,
          updatedAt: "2026-05-16T00:01:00.000Z",
        },
      ],
    };

    const portfolioPositions: PortfolioPositionsResponse = {
      ...emptySystemStatus,
      positions: [
        {
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "HK",
          symbol: "HK.00700",
          quantity: 50,
          averagePrice: 319.5,
          marketValue: 15975,
          updatedAt: "2026-05-16T00:01:00.000Z",
        },
      ],
    };

    const portfolioCashReconciliation: PortfolioCashReconciliationResponse = {
      ...emptySystemStatus,
      connectivity: "connected",
      balances: [
        {
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          currency: "HKD",
          status: "different",
          projectedCashBalance: 55981.5,
          brokerCash: 88000,
          cashDelta: -32018.5,
          brokerAvailableWithdrawalCash: 87000,
          brokerNetCashPower: 50000,
          projectedUpdatedAt: "2026-05-16T00:01:00.000Z",
        },
      ],
    };

    const portfolioReconciliation: PortfolioReconciliationResponse = {
      ...emptySystemStatus,
      connectivity: "connected",
      positions: [
        {
          brokerId: "futu",
          tradingEnvironment: "REAL",
          accountId: "REAL-001",
          market: "HK",
          symbol: "HK.00700",
          symbolName: "Tencent",
          status: "different",
          projectedQuantity: 50,
          brokerQuantity: 100,
          quantityDelta: -50,
          projectedAveragePrice: 319.5,
          brokerAverageCostPrice: 300,
          averagePriceDelta: 19.5,
          projectedRealizedPnl: 480,
          brokerRealizedPnl: 500,
          realizedPnlDelta: -20,
          projectedUpdatedAt: "2026-05-16T00:01:00.000Z",
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
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(portfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(portfolioPositions);
      if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
        return createResponse(portfolioCashReconciliation);
      if (url.includes("/api/v1/portfolio/futu/reconciliation"))
        return createResponse(portfolioReconciliation);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(emptyExecutionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/portfolio");

    expect(wrapper.text()).toContain("Projected Cash");
    expect(wrapper.text()).toContain("55981.5");
    expect(wrapper.text()).toContain("Portfolio Reconciliation");
    expect(wrapper.text()).toContain("-50");

    wrapper.unmount();
  });
});
