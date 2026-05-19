// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
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
import type { SystemStatusResponse } from "@jftrade/ui-contracts";

import {
  MockEventSource,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

function buildFetchMock(options: {
  systemStatus?: SystemStatusResponse;
  strategies?: Array<{
    id: string;
    definition: {
      strategyId: string;
      name: string;
      version: string;
    };
    params: Record<string, unknown>;
    status: "RUNNING" | "PAUSED" | "STOPPED";
    createdAt: string;
    logs: string[];
  }>;
  logsById?: Record<string, string[]>;
  auditById?: Record<
    string,
    Array<{
      instanceId: string;
      kind: string;
      detail?: string;
      at: string;
    }>
  >;
}) {
  const systemStatus = options.systemStatus ?? emptySystemStatus;
  const strategies = options.strategies ?? [];
  const logsById = options.logsById ?? {};
  const auditById = options.auditById ?? {};

  return vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    const logsMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/logs/);
    const auditMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/audit/);

    if (url.includes("/api/v1/market-data/subscriptions"))
      return createResponse(emptyMarketDataSubscriptions);
    if (url.includes("/api/v1/system/status"))
      return createResponse(systemStatus);
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
      return createResponse(emptyPortfolioCashBalances);
    if (url.includes("/api/v1/portfolio/futu/positions"))
      return createResponse(emptyPortfolioPositions);
    if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
      return createResponse(emptyPortfolioCashReconciliation);
    if (url.includes("/api/v1/portfolio/futu/reconciliation"))
      return createResponse(emptyPortfolioReconciliation);
    if (url.includes("/api/v1/execution/orders"))
      return createResponse(emptyExecutionOrders);
    if (logsMatch) {
      const instanceId = decodeURIComponent(logsMatch[1]);
      return createResponse({
        instanceId,
        logs: logsById[instanceId] ?? [],
      });
    }
    if (auditMatch) {
      const instanceId = decodeURIComponent(auditMatch[1]);
      return createResponse({
        instanceId,
        entries: auditById[instanceId] ?? [],
      });
    }
    if (url.includes("/api/v1/strategies")) return createResponse(strategies);

    throw new Error(`Unexpected request: ${url}`);
  });
}

describe("Strategy page", () => {
  it("lists strategies and shows the selected strategy logs and audit", async () => {
    const strategies = [
      {
        id: "instance-1",
        definition: {
          strategyId: "s-mean-revert",
          name: "Mean Revert",
          version: "1.0.0",
        },
        params: {
          threshold: 10,
        },
        status: "RUNNING" as const,
        createdAt: "2026-05-16T00:00:00.000Z",
        logs: [],
      },
      {
        id: "instance-2",
        definition: {
          strategyId: "s-breakout",
          name: "Breakout",
          version: "1.0.0",
        },
        params: {
          window: 20,
        },
        status: "PAUSED" as const,
        createdAt: "2026-05-16T00:01:00.000Z",
        logs: [],
      },
    ];
    const systemStatus: SystemStatusResponse = {
      ...emptySystemStatus,
      defaultTradingEnvironment: "REAL",
      realTradingEnabled: true,
    };

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus,
        strategies,
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z started strategy s-mean-revert",
            "2026-05-16T00:00:02.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
          "instance-2": ["2026-05-16T00:01:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-mean-revert",
              at: "2026-05-16T00:00:00.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "tick",
              detail: "QUOTE_SNAPSHOT HK.00700",
              at: "2026-05-16T00:00:02.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              at: "2026-05-16T00:01:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    expect(wrapper.text()).toContain("Strategy Instances");
    expect(wrapper.text()).toContain("Mean Revert");
    expect(wrapper.text()).toContain("Breakout");
    expect(wrapper.text()).toContain("tick QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("Strategy Audit");
    expect(wrapper.text()).toContain("QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("REAL");

    wrapper.unmount();
  });

  it("switches selected strategy and refreshes logs and audit", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus: {
          ...emptySystemStatus,
          realTradingKillSwitch: {
            ...emptySystemStatus.realTradingKillSwitch,
            active: true,
          },
        },
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
          {
            id: "instance-2",
            definition: {
              strategyId: "s-beta",
              name: "Beta",
              version: "1.0.0",
            },
            params: { slow: 13 },
            status: "PAUSED",
            createdAt: "2026-05-16T00:02:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": ["2026-05-16T00:00:00.000Z started strategy s-alpha"],
          "instance-2": ["2026-05-16T00:02:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-alpha",
              at: "2026-05-16T00:00:00.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              detail: "manual pause",
              at: "2026-05-16T00:02:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/strategy");

    await wrapper.get('[data-testid="strategy-instance-2"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("paused strategy execution");
    expect(wrapper.text()).toContain("manual pause");
    expect(wrapper.text()).toContain("ACTIVE");

    wrapper.unmount();
  });
});
