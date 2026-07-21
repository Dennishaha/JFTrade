// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  BrokerFundsResponse,
  BrokerOrdersResponse,
  BrokerPositionsResponse,
  BrokerRuntimeResponse,
  ExecutionOrdersResponse,
  PortfolioCashBalancesResponse,
  PortfolioPositionsResponse,
  StorageOverviewResponse,
  SystemStatusResponse,
} from "@/contracts";
import {
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
  emptyStorageOverview,
  emptySystemStatus,
} from "@/contracts";
import { formatLocalDateTime } from "@/utils/dateTime";

import {
  MockWebSocket,
  createLiveEnvelope,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

function findConsoleEventStream(): MockWebSocket | undefined {
  return MockWebSocket.instances.find((instance) =>
    instance.url.includes("/api/v1/ws/live"),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("Console Stream", () => {
  it("refreshes the console after receiving a websocket snapshot", async () => {
    const systemStatus: SystemStatusResponse = {
      ...emptySystemStatus,
      defaultTradingEnvironment: "SIMULATE",
      broker: {
        ...emptySystemStatus.broker,
        capabilities: [
          { market: "HK", supportsQuote: true, supportsTrade: true },
        ],
      },
    };
    const storageOverview: StorageOverviewResponse = emptyStorageOverview;
    const brokerRuntime: BrokerRuntimeResponse = {
      ...emptyBrokerRuntime,
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
    };
    const brokerFunds: BrokerFundsResponse = {
      ...emptyBrokerFunds,
      checkedAt: "2026-05-17T00:00:00.000Z",
      connectivity: "connected",
      summary: {
        accountId: "SIM-001",
        tradingEnvironment: "SIMULATE",
        market: "HK",
        currency: "HKD",
        totalAssets: 100000,
        securitiesAssets: 0,
        fundAssets: 0,
        bondAssets: 0,
        cash: 100000,
        marketValue: 0,
        longMarketValue: 0,
        shortMarketValue: 0,
        purchasingPower: 100000,
        shortSellingPower: 0,
        netCashPower: 100000,
        availableWithdrawalCash: 100000,
        maxWithdrawal: 100000,
        availableFunds: null,
        frozenCash: 0,
        pendingAsset: 0,
        unrealizedPnl: null,
        realizedPnl: null,
        initialMargin: null,
        maintenanceMargin: null,
        marginCallMargin: null,
        riskStatus: null,
      },
      currencyBalances: [],
      marketAssets: [],
    };
    const brokerPositions: BrokerPositionsResponse = emptyBrokerPositions;
    const brokerOrders: BrokerOrdersResponse = emptyBrokerOrders;
    const portfolioCashBalances: PortfolioCashBalancesResponse =
      emptyPortfolioCashBalances;
    const portfolioPositions: PortfolioPositionsResponse =
      emptyPortfolioPositions;
    const executionOrders: ExecutionOrdersResponse = emptyExecutionOrders;

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse(systemStatus);
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse(storageOverview);
      }
      if (url.includes("/api/v1/system/real-trade-approvals")) {
        return createResponse({ ...emptyStorageOverview, entries: [] });
      }
      if (url.includes("/api/v1/system/real-trade-hard-stops")) {
        return createResponse({
          entries: [],
          blockedOperations: ["PLACE", "MODIFY"],
          allowsCancel: true,
        });
      }
      if (url.includes("/api/v1/system/real-trade-hard-stop-events")) {
        return createResponse({
          entries: [],
          blockedOperations: ["PLACE", "MODIFY"],
          allowsCancel: true,
          realTradingEnabled: false,
        });
      }
      if (url.includes("/api/v1/system/real-trade-kill-switch-events")) {
        return createResponse({
          ...emptySystemStatus.realTradingKillSwitch,
          entries: [],
          realTradingEnabled: false,
        });
      }
      if (url.includes("/api/v1/system/real-trade-kill-switch")) {
        return createResponse({
          realTradingEnabled: false,
          runtimeActive: false,
          killSwitchActive: false,
          killSwitchSource: null,
          blockedOperations: ["PLACE", "MODIFY"],
          allowsCancel: true,
          entry: null,
        });
      }
      if (url.includes("/api/v1/system/real-trade-risk-events")) {
        return createResponse({
          realTradingEnabled: false,
          riskEnabled: false,
          runtimeRiskConfigured: false,
          runtimeConfiguredMaxOrderQuantity: null,
          runtimeConfiguredMaxOrderNotional: null,
          effectiveMaxOrderQuantity: null,
          effectiveMaxOrderNotional: null,
          maxOrderQuantity: null,
          maxOrderNotional: null,
          entries: [],
        });
      }
      if (url.includes("/api/v1/system/real-trade-risk-limits")) {
        return createResponse({
          realTradingEnabled: false,
          riskEnabled: false,
          runtimeRiskConfigured: false,
          runtimeConfiguredMaxOrderQuantity: null,
          runtimeConfiguredMaxOrderNotional: null,
          effectiveMaxOrderQuantity: null,
          effectiveMaxOrderNotional: null,
          entry: null,
        });
      }
      if (url.includes("/api/v1/system/worker/broker-order-updates")) {
        return createResponse({
          ...emptyStorageOverview,
          subscriptions: [],
          recentInvalidations: [],
          brokers: [],
          runtime: { lastStoppedAt: null, stoppedSubscriptions: null },
        });
      }
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse(brokerRuntime);
      }
      if (url.includes("/api/v1/brokers/futu/funds")) {
        return createResponse(brokerFunds);
      }
      if (url.includes("/api/v1/brokers/futu/positions")) {
        return createResponse(brokerPositions);
      }
      if (url.includes("/api/v1/brokers/futu/orders")) {
        return createResponse(brokerOrders);
      }
      if (url.includes("/api/v1/portfolio/futu/cash-balances")) {
        return createResponse(portfolioCashBalances);
      }
      if (url.includes("/api/v1/portfolio/futu/positions")) {
        return createResponse(portfolioPositions);
      }
      if (url.includes("/api/v1/execution/orders")) {
        return createResponse(executionOrders);
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp();
    const consoleStream = findConsoleEventStream();

    expect(consoleStream?.url).toContain("/api/v1/ws/live");

    const initialFetchCount = fetchMock.mock.calls.length;

    const payload = {
      type: "console.refresh",
      at: "2026-05-17T00:02:00.000Z",
      checkedAt: "2026-05-17T00:02:00.000Z",
    };
    consoleStream?.emitMessage(createLiveEnvelope(payload, {
      source: "system",
      entityId: "console",
    }));

    await flushRequests();

    expect(fetchMock.mock.calls.length).toBeGreaterThan(initialFetchCount);
    expect(wrapper.text()).not.toContain("事件流");
    expect(wrapper.text()).not.toContain(formatLocalDateTime("2026-05-17T00:02:00.000Z"));

    wrapper.unmount();
    expect(consoleStream?.closed).toBe(true);
  });
});
