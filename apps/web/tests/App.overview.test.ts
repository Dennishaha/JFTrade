// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrderFees,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrderEvents,
  emptyMarketDataSubscriptions,
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

import { MockEventSource, createResponse, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

describe("Overview page", () => {
  it("renders the workstation overview on root redirect", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "SIMULATE",
          message: "workspace ready",
          persistence: {
            ...emptySystemStatus.persistence,
            status: "ok",
          },
          strategyRuntime: {
            activeStrategies: 1,
          },
          broker: {
            displayName: "Futu Securities",
            capabilities: [
              { market: "HK", supportsQuote: true, supportsTrade: true },
            ],
          },
        });
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse({
          ...emptyStorageOverview,
          recentAuditLogs: [
            {
              id: "audit-1",
              action: "workspace.loaded",
              targetType: "console",
              targetId: "overview",
              createdAt: "2026-05-17T00:00:00.000Z",
            },
          ],
          recentExecutionCommands: [
            {
              id: "command-1",
              brokerId: "futu",
              operation: "PLACE",
              actorType: "user",
              actorId: "trader-1",
              idempotencyKey: "place-1",
              internalOrderId: "internal-1",
              requestHash: "hash",
              requestJson: "{}",
              completedAt: "2026-05-17T00:00:01.000Z",
              createdAt: "2026-05-17T00:00:00.000Z",
              updatedAt: "2026-05-17T00:00:01.000Z",
            },
          ],
        });
      }
      if (url.includes("/api/v1/system/real-trade-approvals")) {
        return createResponse({
          ...emptyRealTradeApprovals,
          entries: [
            {
              id: "approval-1",
              brokerId: "futu",
              tradingEnvironment: "REAL",
              accountId: "ACC-1",
              market: "HK",
              symbol: "HK.00700",
              operation: "PLACE",
              decision: "approved",
              operatorId: "risk-admin",
              ticketId: "ticket-1",
              reason: "validated",
              requestedAt: "2026-05-17T00:00:00.000Z",
              approvedAt: "2026-05-17T00:00:01.000Z",
              createdAt: "2026-05-17T00:00:01.000Z",
            },
          ],
        });
      }
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
      if (url.includes("/api/v1/system/real-trade-risk-limits")) {
        return createResponse({
          ...emptyRealTradeRiskState,
          riskEnabled: true,
          effectiveMaxOrderQuantity: 100,
          effectiveMaxOrderNotional: 20000,
        });
      }
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
          ...emptyBrokerRuntime,
          session: {
            ...emptyBrokerRuntime.session,
            connectivity: "connected",
          },
          accounts: [
            {
              accountId: "SIM-001",
              tradingEnvironment: "SIMULATE",
              accountType: "CASH",
              accountRole: null,
              securityFirm: "FUTUSECURITIES",
              marketAuthorities: ["HK"],
              simulatedAccountType: "STOCK",
            },
          ],
        });
      }
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse(emptyBrokerFunds);
      if (url.includes("/api/v1/brokers/futu/cash-flows"))
        return createResponse(emptyBrokerCashFlows);
      if (url.includes("/api/v1/brokers/futu/order-fees"))
        return createResponse(emptyBrokerOrderFees);
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(emptyBrokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders")) {
        return createResponse({
          ...emptyBrokerOrders,
          orders: [
            {
              accountId: "SIM-001",
              tradingEnvironment: "SIMULATE",
              market: "HK",
              brokerOrderId: "9001",
              brokerOrderIdEx: "ex-9001",
              symbol: "HK.00700",
              symbolName: "Tencent",
              side: "BUY",
              orderType: "NORMAL",
              status: "SUBMITTED",
              quantity: 100,
              filledQuantity: 50,
              price: 320,
              filledAveragePrice: 319.5,
              submittedAt: "2026-05-17 10:00:00",
              updatedAt: "2026-05-17 10:01:00",
              remark: null,
              lastError: null,
              timeInForce: "DAY",
              currency: "HKD",
            },
          ],
        });
      }
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions")) {
        return createResponse({
          ...emptyPortfolioPositions,
          positions: [
            {
              brokerId: "futu",
              tradingEnvironment: "SIMULATE",
              accountId: "SIM-001",
              market: "HK",
              symbol: "HK.00700",
              quantity: 50,
              averagePrice: 319.5,
              marketValue: 15975,
              updatedAt: "2026-05-17T00:00:00.000Z",
            },
          ],
        });
      }
      if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
        return createResponse(emptyPortfolioCashReconciliation);
      if (url.includes("/api/v1/portfolio/futu/reconciliation"))
        return createResponse(emptyPortfolioReconciliation);
      if (url.includes("/api/v1/execution/orders/internal-1/events"))
        return createResponse(emptyExecutionOrderEvents);
      if (url.includes("/api/v1/execution/orders")) {
        return createResponse({
          orders: [
            {
              internalOrderId: "internal-1",
              brokerId: "futu",
              brokerOrderId: "9001",
              brokerOrderIdEx: "ex-9001",
              tradingEnvironment: "SIMULATE",
              accountId: "SIM-001",
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
              lastError: null,
              lastErrorCode: null,
              lastErrorSource: null,
              submittedAt: "2026-05-17 10:00:00",
              updatedAt: "2026-05-17 10:01:00",
              createdAt: "2026-05-17T00:00:00.000Z",
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/subscriptions"))
        return createResponse(emptyMarketDataSubscriptions);
      if (url.includes("/api/v1/market-data/snapshots/HK/00700")) {
        return createResponse({
          request: {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
          },
          snapshot: {
            price: 320.5,
            bid: 320.4,
            ask: 320.6,
            volume: 1280000,
            turnover: 410240000,
            at: "2026-05-17T01:30:00.000Z",
          },
          meta: {
            instrumentId: "HK.00700",
            source: "api-sample-cache",
            resolvedAt: "2026-05-17T01:30:00.000Z",
            fromCache: true,
          },
        });
      }
      if (url.includes("/api/v1/market-data/candles/HK/00700")) {
        return createResponse({
          request: {
            instrument: {
              market: "HK",
              symbol: "00700",
              instrumentId: "HK.00700",
            },
            period: "1m",
            limit: 3,
          },
          candles: [
            {
              period: "1m",
              open: 320,
              high: 320.8,
              low: 319.9,
              close: 320.5,
              volume: 18000,
              at: "2026-05-17T01:29:00.000Z",
            },
          ],
          totalReturned: 1,
          meta: {
            instrumentId: "HK.00700",
            source: "api-sample-cache",
            resolvedAt: "2026-05-17T01:30:00.000Z",
            fromCache: true,
          },
        });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/overview");

    expect(wrapper.text()).toContain("工作台概览");
    expect(wrapper.text()).toContain("行情焦点");
    expect(wrapper.text()).toContain("订单执行概览");
    expect(wrapper.text()).toContain("文档与运维准备度");

    wrapper.unmount();
  });
});
