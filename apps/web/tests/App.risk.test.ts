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
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@jftrade/ui-contracts";
import type {
  RealTradeHardStopsResponse,
  RealTradeKillSwitchStateResponse,
  RealTradeRiskStateResponse,
} from "@jftrade/ui-contracts";

import { MockEventSource, createResponse, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
});

function buildFetchMock(
  overrides: {
    killSwitchState?: Partial<RealTradeKillSwitchStateResponse>;
    riskState?: Partial<RealTradeRiskStateResponse>;
    hardStops?: Partial<RealTradeHardStopsResponse>;
  } = {},
) {
  const killSwitchState = {
    ...emptyRealTradeKillSwitchState,
    ...overrides.killSwitchState,
  };
  const riskState = { ...emptyRealTradeRiskState, ...overrides.riskState };
  const hardStops = { ...emptyRealTradeHardStops, ...overrides.hardStops };

  return vi.fn(async (input: string | URL | Request) => {
    const url = String(input);

    if (url.includes("/api/v1/system/status"))
      return createResponse(emptySystemStatus);
    if (url.includes("/api/v1/system/storage/overview"))
      return createResponse(emptyStorageOverview);
    if (url.includes("/api/v1/system/real-trade-approvals"))
      return createResponse(emptyRealTradeApprovals);
    if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
      return createResponse(emptyRealTradeHardStopEvents);
    if (url.includes("/api/v1/system/real-trade-hard-stops"))
      return createResponse(hardStops);
    if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
      return createResponse(emptyRealTradeKillSwitchEvents);
    if (url.includes("/api/v1/system/real-trade-kill-switch"))
      return createResponse(killSwitchState);
    if (url.includes("/api/v1/system/real-trade-risk-events"))
      return createResponse(emptyRealTradeRiskEvents);
    if (url.includes("/api/v1/system/real-trade-risk-limits"))
      return createResponse(riskState);
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
    if (url.includes("/api/v1/brokers/futu/cash-flows"))
      return createResponse(emptyBrokerCashFlows);

    throw new Error(`Unexpected request: ${url}`);
  });
}

describe("Risk page", () => {
  it("renders the risk page with nav item and section headings", async () => {
    vi.stubGlobal("fetch", buildFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("风控 / 门禁");
    expect(wrapper.text()).toContain("风控限额");
    expect(wrapper.text()).toContain("熔断开关");
    expect(wrapper.text()).toContain("硬停止");
    expect(wrapper.text()).toContain("实盘审批");
    expect(wrapper.text()).toContain("风控事件日志");

    wrapper.unmount();
  });

  it("shows kill switch INACTIVE when kill switch is off", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({ killSwitchState: { killSwitchActive: false } }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("未激活");

    wrapper.unmount();
  });

  it("shows kill switch ACTIVE and blocked operations when kill switch is on", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        killSwitchState: {
          killSwitchActive: true,
          killSwitchSource: "ENV",
          blockedOperations: ["PLACE", "MODIFY"],
          allowsCancel: true,
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("已激活");
    expect(wrapper.text()).toContain("下单");
    expect(wrapper.text()).toContain("改单");

    wrapper.unmount();
  });

  it("shows active hard stops when present", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        hardStops: {
          entries: [
            {
              id: "hs-1",
              brokerId: "futu",
              tradingEnvironment: "REAL",
              accountId: "ACC-001",
              market: "HK",
              symbol: null,
              operatorId: "system",
              reason: "circuit breaker triggered",
              activatedAt: "2026-05-17T06:00:00.000Z",
              updatedAt: "2026-05-17T06:00:00.000Z",
            },
          ],
          blockedOperations: ["PLACE", "MODIFY"],
          allowsCancel: true,
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("circuit breaker triggered");
    expect(wrapper.text()).toContain("futu");
    expect(wrapper.text()).toContain("1 个活跃");

    wrapper.unmount();
  });

  it("shows effective risk limits when risk state has values", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        riskState: {
          riskEnabled: true,
          effectiveMaxOrderQuantity: 500,
          effectiveMaxOrderNotional: 100000,
          riskConfigSource: "ENV",
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("500");
    expect(wrapper.text()).toContain("100000");

    wrapper.unmount();
  });

  it("shows NONE for hard stops when no active stops", async () => {
    vi.stubGlobal("fetch", buildFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/risk");

    expect(wrapper.text()).toContain("无活跃实盘硬停止");

    wrapper.unmount();
  });

  it("risk nav item appears in navigation", async () => {
    vi.stubGlobal("fetch", buildFetchMock());
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/system");

    expect(wrapper.text()).toContain("风控");
    expect(wrapper.text()).toContain("实盘熔断开关");

    wrapper.unmount();
  });
});
