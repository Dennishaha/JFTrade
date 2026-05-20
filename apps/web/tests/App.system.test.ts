// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
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
  emptyWorkerBrokerOrderUpdates,
} from "@jftrade/ui-contracts";
import type { SystemStatusResponse } from "@jftrade/ui-contracts";

import {
  MockEventSource,
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

const systemStatus: SystemStatusResponse = {
  defaultTradingEnvironment: "REAL",
  message: "runtime ready",
  realTradingEnabled: true,
  realTradingKillSwitch: {
    active: false,
    envConfiguredActive: false,
    controlPlaneActive: false,
    blockedOperations: ["PLACE", "MODIFY"],
    allowsCancel: true,
  },
  realTradingRisk: {
    enabled: true,
    maxOrderQuantity: 100,
    maxOrderNotional: 20000,
    envConfiguredMaxOrderQuantity: 100,
    envConfiguredMaxOrderNotional: 20000,
    controlPlaneActive: false,
    controlPlaneMaxOrderQuantity: null,
    controlPlaneMaxOrderNotional: null,
    riskConfigSource: "ENV",
  },
  broker: {
    displayName: "Futu Securities",
    capabilities: [{ market: "HK", supportsQuote: true, supportsTrade: true }],
  },
  persistence: {
    engine: "sqlite",
    status: "ok",
    migrated: true,
    tables: ["schema_migrations"],
    pendingMigrations: [],
  },
  strategyRuntime: {
    activeStrategies: 2,
  },
};

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
  MockWebSocket.instances = [];
});

describe("System page", () => {
  it("shows system status and worker broker subscription health", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status"))
        return createResponse(systemStatus);
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse({
          ...emptyStorageOverview,
          recentAuditLogs: [
            {
              id: "audit-1",
              action: "api.bootstrap",
              targetType: "service",
              targetId: "api",
              createdAt: "2026-05-16T00:00:00.000Z",
            },
          ],
          recentExecutionCommands: [],
        });
      }
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
      if (url.includes("/api/v1/system/worker/broker-order-updates")) {
        return createResponse({
          ...emptyWorkerBrokerOrderUpdates,
          subscriptions: [
            {
              subscriptionKey: "futu:REAL:REAL-001:HK",
              brokerId: "futu",
              tradingEnvironment: "REAL",
              accountId: "REAL-001",
              market: "HK",
              status: "retrying",
              lastAction: "worker.broker-order-updates.subscribe-failed",
              lastActionAt: "2026-05-16T00:00:00.000Z",
              lastError: "OpenD unavailable",
              lastErrorContext: {
                summary: "OpenD unavailable",
                rawMessage: "OpenD unavailable",
                code: null,
                reason: null,
                category: "subscription",
              },
              consecutiveFailures: 2,
              retryDelayMs: 4000,
              backoffUntil: "2026-05-16T00:00:04.000Z",
            },
          ],
          recentInvalidations: [
            {
              subscriptionKey: "futu:REAL:REAL-001:HK",
              brokerId: "futu",
              tradingEnvironment: "REAL",
              accountId: "REAL-001",
              market: "HK",
              kind: "DISCONNECTED",
              message: "code=1006, reason=network down",
              errorContext: {
                summary: "network down (code 1006)",
                rawMessage: "code=1006, reason=network down",
                code: "1006",
                reason: "network down",
                category: "connection",
              },
              consecutiveFailures: 3,
              retryDelayMs: 8000,
              backoffUntil: "2026-05-16T00:00:08.000Z",
              createdAt: "2026-05-16T00:00:00.000Z",
            },
          ],
          brokers: [
            {
              brokerId: "futu",
              lastAction: "worker.broker-order-updates.invalidated",
              lastActionAt: "2026-05-16T00:00:00.000Z",
              connectivity: "degraded",
              lastError: "code=1006, reason=network down",
              accountsDiscovered: 1,
              activeSubscriptions: 0,
              retryingSubscriptions: 1,
              inactiveSubscriptions: 0,
              backoffSubscriptions: 1,
              disconnectedBackoffSubscriptions: 1,
              subscribeFailedBackoffSubscriptions: 0,
              errorBackoffSubscriptions: 0,
              dominantBackoffSource: "DISCONNECTED",
              dominantBackoffCount: 1,
              longestBackoffSource: "DISCONNECTED",
              longestBackoffRemainingMs: 8000,
              longestBackoffSubscriptionKey: "futu:REAL:REAL-001:HK",
              longestBackoffMarket: "HK",
              longestBackoffTradingEnvironment: "REAL",
              longestBackoffAccountId: "REAL-001",
              topBackoffHotspots: [
                {
                  subscriptionKey: "futu:REAL:REAL-001:HK",
                  source: "DISCONNECTED",
                  remainingMs: 8000,
                  backoffUntil: "2026-05-16T00:00:08.000Z",
                  lastActionAt: "2026-05-16T00:00:00.000Z",
                  tradingEnvironment: "REAL",
                  accountId: "REAL-001",
                  market: "HK",
                  reason: "code=1006, reason=network down",
                  reasonContext: {
                    summary: "network down (code 1006)",
                    rawMessage: "code=1006, reason=network down",
                    code: "1006",
                    reason: "network down",
                    category: "connection",
                  },
                },
              ],
              layeredBackoffSummaries: [
                {
                  tradingEnvironment: "REAL",
                  accountId: "REAL-001",
                  activeSubscriptions: 0,
                  retryingSubscriptions: 1,
                  inactiveSubscriptions: 0,
                  backoffSubscriptions: 2,
                  dominantBackoffSource: "DISCONNECTED",
                  dominantBackoffCount: 1,
                  longestBackoffRemainingMs: 8000,
                  topBackoffMarket: "HK",
                },
              ],
              recentInvalidationCount: 1,
              lastInvalidationKind: "DISCONNECTED",
              lastInvalidationAt: "2026-05-16T00:00:00.000Z",
              backoffActive: true,
              backoffSource: "DISCONNECTED",
              backoffUntil: "2026-05-16T00:00:08.000Z",
              backoffRemainingMs: 8000,
            },
          ],
        });
      }
      if (url.includes("/api/v1/brokers/futu/runtime"))
        return createResponse(emptyBrokerRuntime);
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse(emptyBrokerFunds);
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(emptyBrokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders"))
        return createResponse(emptyBrokerOrders);
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(emptyPortfolioPositions);
      if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
        return createResponse(emptyPortfolioCashReconciliation);
      if (url.includes("/api/v1/portfolio/futu/reconciliation"))
        return createResponse(emptyPortfolioReconciliation);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(emptyExecutionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/system");
    const liveSocket = MockWebSocket.instances[0];

    expect(liveSocket?.url).toBe("ws://127.0.0.1:3000/api/v1/ws/live");

    liveSocket?.emitMessage({
      type: "heartbeat",
      at: "2026-05-16T00:30:00.000Z",
    });
    await flushRequests();

    expect(wrapper.text()).toContain("JFTRADE");
    expect(wrapper.text()).toContain("系统运行状态");
    expect(wrapper.text()).toContain("Worker Broker Subscription Health");
    expect(wrapper.text()).toContain("Worker Backoff Hotspots");
    expect(wrapper.text()).toContain("Layered Backoff Governance");
    expect(wrapper.text()).toContain("REAL / REAL-001");
    expect(wrapper.text()).toContain("OpenD unavailable");
    expect(wrapper.text()).toContain("network down (code 1006)");
    expect(wrapper.text()).toContain("WS");
    expect(wrapper.text()).toContain("2026-05-16T00:30:00.000Z");

    wrapper.unmount();
  });
});
