// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyBrokerSettings,
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

describe("TopBar trading environment switch", () => {
  it("filters account list in picker by environment and auto-selects the first available account", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "SIMULATE",
        });
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse(emptyStorageOverview);
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
          accounts: [],
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
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);

      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
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
            {
              accountId: "REAL-001",
              tradingEnvironment: "REAL",
              accountType: "MARGIN",
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/overview");
    await flushRequests();

    const pickerOpenButton = wrapper.get(
      '[data-testid="topbar-broker-account-picker-open"]',
    );
    await pickerOpenButton.trigger("click");

    const initialItemTexts = wrapper.findAll('[data-testid="topbar-broker-account-item"]').map(
      (item) => item.text(),
    );

    expect(initialItemTexts.some((text) => text.includes("SIM-001"))).toBe(
      true,
    );
    expect(initialItemTexts.some((text) => text.includes("REAL-001"))).toBe(
      false,
    );

    const pickerCloseButton = wrapper.get(
      '[data-testid="topbar-broker-account-picker-close"]',
    );
    await pickerCloseButton.trigger("click");

    const statusRequestsBeforeSwitch = fetchMock.mock.calls.filter(
      ([request]) => String(request).includes("/api/v1/system/status"),
    ).length;

    const environmentSwitch = wrapper.get(
      '[data-testid="topbar-trading-environment-real"]',
    );
    await environmentSwitch.trigger("click");
    await flushRequests();

    expect(pickerOpenButton.text()).toContain("REAL-001");

    await pickerOpenButton.trigger("click");

    const brokerAccountFilterInput = wrapper.get(
      '[data-testid="topbar-broker-account-filter"]',
    );
    await brokerAccountFilterInput.setValue("REAL-001");
    await flushRequests();

    const realOnlyItemTexts = wrapper.findAll('[data-testid="topbar-broker-account-item"]').map(
      (item) => item.text(),
    );

    expect(realOnlyItemTexts.some((text) => text.includes("REAL-001"))).toBe(
      true,
    );
    expect(realOnlyItemTexts.some((text) => text.includes("SIM-001"))).toBe(
      false,
    );

    const statusRequestsAfterSwitch = fetchMock.mock.calls.filter(
      ([request]) => String(request).includes("/api/v1/system/status"),
    ).length;

    expect(statusRequestsAfterSwitch).toBeGreaterThan(
      statusRequestsBeforeSwitch,
    );

    wrapper.unmount();
  });

  it("prefers the first favorite account when switching environment", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "SIMULATE",
        });
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse(emptyStorageOverview);
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
          accounts: [],
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
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);

      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
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
            {
              accountId: "REAL-001",
              tradingEnvironment: "REAL",
              accountType: "MARGIN",
              accountRole: null,
              securityFirm: "FUTUSECURITIES",
              marketAuthorities: ["US"],
              simulatedAccountType: null,
            },
            {
              accountId: "REAL-002",
              tradingEnvironment: "REAL",
              accountType: "MARGIN",
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/overview");
    await flushRequests();

    const pickerOpenButton = wrapper.get(
      '[data-testid="topbar-broker-account-picker-open"]',
    );

    const switchToReal = wrapper.get(
      '[data-testid="topbar-trading-environment-real"]',
    );
    await switchToReal.trigger("click");
    await flushRequests();

    await pickerOpenButton.trigger("click");

    const real002Row = wrapper
      .findAll('[data-testid="topbar-broker-account-item"]')
      .find((item) => item.text().includes("REAL-002"));
    expect(real002Row).toBeDefined();

    const real002FavoriteButton = real002Row?.get(
      '[data-testid="topbar-broker-account-item-favorite"]',
    );
    await real002FavoriteButton?.trigger("click");

    const pickerCloseButton = wrapper.get(
      '[data-testid="topbar-broker-account-picker-close"]',
    );
    await pickerCloseButton.trigger("click");

    const switchToSim = wrapper.get(
      '[data-testid="topbar-trading-environment-simulate"]',
    );
    await switchToSim.trigger("click");
    await flushRequests();

    await switchToReal.trigger("click");
    await flushRequests();

    expect(pickerOpenButton.text()).toContain("REAL-002");

    wrapper.unmount();
  });

  it("submits the instrument code when pressing Enter in the topbar input", async () => {
    window.sessionStorage.removeItem("jftrade.workspace.layout.v1");

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse({
          ...emptySystemStatus,
          defaultTradingEnvironment: "SIMULATE",
        });
      }
      if (url.includes("/api/v1/system/storage/overview")) {
        return createResponse(emptyStorageOverview);
      }
      if (url.includes("/api/v1/settings/brokers")) {
        return createResponse(emptyBrokerSettings);
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
      if (url.includes("/api/v1/system/worker/broker-order-updates"))
        return createResponse(emptyWorkerBrokerOrderUpdates);

      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
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
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/overview");

    const codeInput = wrapper.get(
      '[data-testid="topbar-instrument-code"]',
    );

    await codeInput.setValue("aapl");
    await codeInput.trigger("keydown.enter");
    await flushRequests();

    const storedPrefs = JSON.parse(
      window.sessionStorage.getItem("jftrade.workspace.layout.v1") ?? "{}",
    ) as { market?: string; symbol?: string };

    expect(storedPrefs.market).toBe("HK");
    expect(storedPrefs.symbol).toBe("AAPL");

    wrapper.unmount();
  });
});
