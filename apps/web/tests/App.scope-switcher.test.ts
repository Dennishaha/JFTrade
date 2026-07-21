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
  emptyPortfolioPositions,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptyRealTradeRiskState,
  emptySystemStatus,
} from "@/contracts";

import {
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

function buildSelectionKey(
  brokerId: string,
  tradingEnvironment: string,
  accountId: string,
  market: string,
): string {
  return [brokerId, tradingEnvironment, accountId, market]
    .map((segment) => encodeURIComponent(segment))
    .join("|");
}

describe("Top bar scope switcher", () => {
  it("switches broker-account queries when a different managed account is selected", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "SIMULATE",
        });
      }
      if (url.includes("/api/v1/settings/brokers")) {
        return createResponse({
          brokers: [
            {
              descriptor: {
                id: "futu",
                displayName: "Futu OpenAPI via OpenD",
                environments: ["SIMULATE", "REAL"],
                capabilities: [
                  { market: "HK", supportsQuote: true, supportsTrade: true },
                  { market: "US", supportsQuote: true, supportsTrade: true },
                ],
                notes: [],
              },
              integration: {
                brokerId: "futu",
                enabled: true,
                config: {
                  type: "futu",
                  host: "127.0.0.1",
                  apiPort: 11110,
                  websocketPort: 11111,
                  maxWebSocketConnections: 20,
                  useEncryption: false,
                  websocketKey: "",
                  tradeMarket: "HK",
                  securityFirm: "FUTUSECURITIES",
                },
                updatedAt: "2026-05-17T00:00:00.000Z",
                createdAt: "2026-05-17T00:00:00.000Z",
              },
              defaults: {
                type: "futu",
                host: "127.0.0.1",
                apiPort: 11110,
                websocketPort: 11111,
                maxWebSocketConnections: 20,
                useEncryption: false,
                websocketKey: "",
                tradeMarket: "HK",
                securityFirm: "FUTUSECURITIES",
              },
            },
          ],
          accounts: [
            {
              id: "acct-sim",
              brokerId: "futu",
              accountId: "SIM-001",
              displayName: "Primary sim",
              tradingEnvironment: "SIMULATE",
              market: "HK",
              securityFirm: "FUTUSECURITIES",
              enabled: true,
              updatedAt: "2026-05-17T00:00:00.000Z",
              createdAt: "2026-05-17T00:00:00.000Z",
            },
            {
              id: "acct-real",
              brokerId: "futu",
              accountId: "REAL-001",
              displayName: "Primary real",
              tradingEnvironment: "REAL",
              market: "US",
              securityFirm: "FUTUSECURITIES",
              enabled: true,
              updatedAt: "2026-05-17T00:00:00.000Z",
              createdAt: "2026-05-17T00:00:00.000Z",
            },
          ],
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
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
          ...emptyBrokerRuntime,
          session: {
            ...emptyBrokerRuntime.session,
            connectivity: "connected",
            accountsDiscovered: 2,
          },
          descriptor: {
            ...emptyBrokerRuntime.descriptor,
            capabilities: [
              { market: "HK", supportsQuote: true, supportsTrade: true },
              { market: "US", supportsQuote: true, supportsTrade: true },
            ],
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
            {
              accountId: "REAL-001",
              tradingEnvironment: "REAL",
              accountType: "CASH",
              accountRole: null,
              securityFirm: "FUTUSECURITIES",
              marketAuthorities: ["US"],
              simulatedAccountType: null,
            },
          ],
        });
      }
      if (url.includes("/api/v1/brokers/futu/funds"))
        return createResponse(emptyBrokerFunds);
      if (url.includes("/api/v1/brokers/futu/positions"))
        return createResponse(emptyBrokerPositions);
      if (url.includes("/api/v1/brokers/futu/orders"))
        return createResponse(emptyBrokerOrders);
      if (url.includes("/api/v1/brokers/futu/cash-flows"))
        return createResponse(emptyBrokerCashFlows);
      if (url.includes("/api/v1/portfolio/futu/cash-balances"))
        return createResponse(emptyPortfolioCashBalances);
      if (url.includes("/api/v1/portfolio/futu/positions"))
        return createResponse(emptyPortfolioPositions);
      if (url.includes("/api/v1/execution/orders"))
        return createResponse(emptyExecutionOrders);

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/broker");
    await flushRequests();
    const switchToReal = wrapper.get(
      '[data-testid="topbar-trading-environment-real"]',
    );
    await switchToReal.trigger("click");
    await flushRequests();

    const pickerOpenButton = wrapper.get(
      '[data-testid="topbar-broker-account-picker-open"]',
    );
    await pickerOpenButton.trigger("click");
    await flushRequests();
    const realAccountItem = wrapper
      .findAll('[data-testid="topbar-broker-account-item"]')
      .find((item) => item.text().includes("REAL-001"));
    expect(realAccountItem).toBeDefined();

    await realAccountItem!.trigger("click");
    await flushRequests();

    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).includes(
          "/api/v1/brokers/futu/funds?tradingEnvironment=REAL&accountId=REAL-001&market=US",
        ),
      ),
    ).toBe(true);
    expect(wrapper.text()).toContain("Primary real");

    wrapper.unmount();
  });
});
