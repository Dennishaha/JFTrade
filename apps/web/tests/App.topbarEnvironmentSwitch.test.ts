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
} from "@/contracts";

import {
  MockWebSocket,
  createLiveEnvelope,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

async function waitForShellData(): Promise<void> {
  await flushRequests();
  await new Promise((resolve) => setTimeout(resolve, 50));
  await flushRequests();
}

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

function findLiveEventStream(): MockWebSocket | undefined {
  return MockWebSocket.instances.find((instance) =>
    instance.url.includes("/api/v1/ws/live"),
  );
}

describe("TopBar trading environment switch", () => {
  it("filters account list in picker by environment and auto-selects the first available account", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
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
        return createResponse({
          ...emptyBrokerPositions,
          positions: [
            {
              accountId: "SIM-001",
              tradingEnvironment: "SIMULATE",
              market: "HK",
              symbol: "HK.00700",
              symbolName: "Tencent",
              quantity: 100,
              sellableQuantity: 100,
              lastPrice: 320,
              costPrice: 300,
              averageCostPrice: 300,
              marketValue: 32000,
              unrealizedPnl: 2000,
              realizedPnl: 0,
            },
          ],
        });
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
      if (url.includes("/api/v1/market-data/markets")) {
        return createResponse({
          defaultMarket: "HK",
          updatedAt: "2026-06-12T00:00:00.000Z",
          markets: [
            {
              code: "HK",
              resolvedMarket: "HK",
              preferredPrefix: "HK",
              displayName: "Hong Kong",
              quoteCurrency: "HKD",
              supportsExtendedHours: false,
              requiresExchangePrefix: false,
              aliases: ["HKEX"],
              regularSessions: [],
              precision: { price: 3, quote: 3 },
              tickSize: 0.001,
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/subscriptions")) {
        return createResponse({
          ...emptyMarketDataSubscriptions,
          entries: [
            {
              key: "SNAPSHOT:HK.00700",
              channel: "SNAPSHOT",
              market: "HK",
              symbol: "00700",
              instrumentId: "HK.00700",
              interval: null,
              depthLevel: null,
              consumers: ["web:test-shell-only"],
              refCount: 1,
              createdAt: "2026-05-17T00:00:00.000Z",
              updatedAt: "2026-05-17T00:01:00.000Z",
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/instruments/normalize")) {
        const body = JSON.parse(String(init?.body ?? "{}")) as {
          market?: string;
          code?: string;
          instrumentId?: string;
        };
        const market = (body.market ?? "HK").trim().toUpperCase();
        const code = (body.instrumentId ?? body.code ?? "").trim().toUpperCase();
        return createResponse({
          market,
          prefix: market,
          code,
          symbol: `${market}.${code}`,
          instrumentId: `${market}.${code}`,
          resolvedMarket: market,
        });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/test-shell-only");
    await waitForShellData();
    const liveStream = findLiveEventStream();

    expect(
      wrapper.get('[data-testid="topbar-instrument-market"]').findAll("option"),
    ).toHaveLength(1);

    liveStream?.emitMessage(
      createLiveEnvelope(
        {
          type: "system.notification",
          id: "topbar-notification-1",
          at: "2026-07-03T00:00:00.000Z",
          level: "warn",
          title: "OpenD 连接状态变化",
          message: "行情未登录",
          source: "futu-opend",
          brokerId: "futu",
          category: "broker.connection",
        },
        {
          source: "notification",
          entityId: "topbar-notification-1",
          eventId: "topbar-notification-1",
        },
      ),
    );
    await flushRequests();
    expect(
      Number(wrapper.find('button[title="通知"] .tv-badge').text()),
    ).toBeGreaterThan(0);

    const commandPaletteButton = wrapper.findAll("button").find((button) =>
      button.attributes("title")?.includes("命令面板"),
    );
    expect(commandPaletteButton).toBeDefined();
    await commandPaletteButton!.trigger("click");
    await flushRequests();
    expect(
      wrapper.find('input[placeholder="键入命令或搜索路由…"]').exists(),
    ).toBe(true);

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

    const favoriteButton = wrapper.get(
      '[data-testid="topbar-broker-account-item-favorite"]',
    );
    expect(favoriteButton.text()).toBe("☆");
    await favoriteButton.trigger("click");
    await flushRequests();
    expect(
      wrapper.find('[data-testid="topbar-broker-account-item-favorite"]').text(),
    ).toBe("★");

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

    await wrapper
      .findAll('[data-testid="topbar-broker-account-item"] button')
      [0]!.trigger("click");
    await flushRequests();
    expect(pickerOpenButton.text()).toContain("REAL-001");

    const themeToggle = wrapper.findAll("button").find((button) =>
      button.attributes("title")?.startsWith("主题："),
    );
    expect(themeToggle).toBeDefined();
    const themeBefore = themeToggle!.text();
    await themeToggle!.trigger("click");
    await flushRequests();
    expect(themeToggle!.text()).not.toBe(themeBefore);

    await wrapper.get('button[title="通知"]').trigger("click");
    expect(
      wrapper.get('[data-testid="rightdock-tab-notifications"]').classes(),
    ).toContain("is-active");
    await wrapper.get('button[title="AI 助手"]').trigger("click");
    expect(wrapper.get('[data-testid="rightdock-tab-ai"]').classes()).toContain(
      "is-active",
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
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
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
      if (url.includes("/api/v1/market-data/markets")) {
        return createResponse({
          defaultMarket: "HK",
          updatedAt: "2026-06-12T00:00:00.000Z",
          markets: [
            {
              code: "HK",
              resolvedMarket: "HK",
              preferredPrefix: "HK",
              displayName: "Hong Kong",
              quoteCurrency: "HKD",
              supportsExtendedHours: false,
              requiresExchangePrefix: false,
              aliases: ["HKEX"],
              regularSessions: [],
              precision: { price: 3, quote: 3 },
              tickSize: 0.001,
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/instruments/normalize")) {
        const body = JSON.parse(String(init?.body ?? "{}")) as {
          market?: string;
          code?: string;
          instrumentId?: string;
        };
        const rawInstrument = (body.instrumentId ?? body.code ?? "")
          .trim()
          .toUpperCase()
          .replace(":", ".");
        const embedded = rawInstrument.includes(".")
          ? rawInstrument.split(".", 2)
          : null;
        const market = (embedded?.[0] ?? body.market ?? "HK").trim().toUpperCase();
        const code = (embedded?.[1] ?? rawInstrument).trim().toUpperCase();
        return createResponse({
          market,
          prefix: market,
          code,
          symbol: `${market}.${code}`,
          instrumentId: `${market}.${code}`,
          resolvedMarket: market,
        });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/test-shell-only");
    await waitForShellData();

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

    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
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
      if (url.includes("/api/v1/market-data/markets")) {
        return createResponse({
          defaultMarket: "HK",
          updatedAt: "2026-06-12T00:00:00.000Z",
          markets: [
            {
              code: "HK",
              resolvedMarket: "HK",
              preferredPrefix: "HK",
              displayName: "Hong Kong",
              quoteCurrency: "HKD",
              supportsExtendedHours: false,
              requiresExchangePrefix: false,
              aliases: ["HKEX"],
              regularSessions: [],
              precision: { price: 3, quote: 3 },
              tickSize: 0.001,
            },
          ],
        });
      }
      if (url.includes("/api/v1/market-data/instruments/normalize")) {
        const body = JSON.parse(String(init?.body ?? "{}")) as {
          market?: string;
          code?: string;
          instrumentId?: string;
        };
        const rawInstrument = (body.instrumentId ?? body.code ?? "")
          .trim()
          .toUpperCase()
          .replace(":", ".");
        const embedded = rawInstrument.includes(".")
          ? rawInstrument.split(".", 2)
          : null;
        const market = (embedded?.[0] ?? body.market ?? "HK").trim().toUpperCase();
        const code = (embedded?.[1] ?? rawInstrument).trim().toUpperCase();
        return createResponse({
          market,
          prefix: market,
          code,
          symbol: `${market}.${code}`,
          instrumentId: `${market}.${code}`,
          resolvedMarket: market,
        });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/test-shell-only");
    await waitForShellData();

    const codeInput = wrapper.get(
      '[data-testid="topbar-instrument-code"]',
    );

    await codeInput.setValue("aapl");
    await wrapper.get('[data-testid="topbar-instrument-form"]').trigger("submit");
    await flushRequests();

    const storedPrefs = JSON.parse(
      window.sessionStorage.getItem("jftrade.workspace.layout.v1") ?? "{}",
    ) as { market?: string; symbol?: string };

    expect(storedPrefs.market).toBe("HK");
    expect(storedPrefs.symbol).toBe("AAPL");

    wrapper.unmount();
  });
});
