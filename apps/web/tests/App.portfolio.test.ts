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
  emptySystemStatus,
} from "@/contracts";
import type {
  PortfolioCashBalancesResponse,
  PortfolioPositionsResponse,
} from "@/contracts";

import {
  MockWebSocket,
  createResponse,
  enabledFutuBrokerSettings,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("Account page portfolio route redirect", () => {
  it("shows account cash balances and broker-sourced positions", async () => {
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

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status"))
        return createResponse(emptySystemStatus);
      if (url.includes("/api/v1/settings/brokers"))
        return createResponse(
          enabledFutuBrokerSettings([
            {
              accountId: "REAL-001",
              displayName: "Futu Securities",
              tradingEnvironment: "REAL",
              market: "HK",
            },
          ]),
        );
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
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(emptyExecutionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/portfolio");

    expect(wrapper.text()).toContain("我的账户");
    // 默认持仓 tab 展示券商兼容持仓；多币种现金余额在「资金」tab。
    const positionRow = wrapper
      .findAll("tbody tr")
      .find((candidate) => candidate.text().includes("00700"));
    expect(positionRow?.text()).toContain("券商");
    expect(wrapper.text()).not.toContain("投影");

    const fundsTab = wrapper
      .findAll('button[role="tab"]')
      .find((candidate) => candidate.text().includes("资金"));
    expect(fundsTab).toBeDefined();
    await fundsTab!.trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("多币种现金余额");
    expect(wrapper.text()).toContain("55,981.5");

    wrapper.unmount();
  });
});
