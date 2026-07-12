// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

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
} from "@/contracts";


import {
  MockWebSocket,
  createResponse,
  flushRequests,
  mountApp,
} from "./helpers";

const workspaceViewState = {
  rightDockOpen: true,
  rightDockTab: "ai",
  paneSizes: {
    main: [72, 28],
    leftColumn: [60, 40],
    bottom: [60, 40],
    rightColumn: [60, 40],
  },
};

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
  window.sessionStorage.clear();
  window.localStorage.clear();
  (
    window as Window & {
      __JFTRADE_RUNTIME_CONFIG__?: { authRequired?: boolean };
    }
  ).__JFTRADE_RUNTIME_CONFIG__ = undefined;
});

function buildAuthWorkspaceFetchMock() {
  let authenticated = false;

  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const method = String(init?.method ?? "GET").toUpperCase();

    if (url.includes("/api/v1/auth/session")) {
      return createResponse({
        authenticated,
        csrfToken: authenticated ? "csrf-token" : "",
      });
    }
    if (url.includes("/api/v1/auth/login") && method === "POST") {
      authenticated = true;
      return createResponse({
        authenticated: true,
        csrfToken: "csrf-token",
      });
    }
    if (url.includes("/api/v1/auth/logout") && method === "POST") {
      authenticated = false;
      return createResponse({ authenticated: false });
    }
    if (url.includes("/api/v1/adk/agents")) {
      return createResponse({
        agents: [
          {
            id: "agent-1",
            name: "投资分析助手",
            instruction: "test",
            providerId: "provider-1",
            model: "gpt-4o-mini",
            tools: ["strategy.save_draft"],
            skills: [],
            permissionMode: "approval",
            memoryEnabled: true,
            recentUserWindow: 6,
            workMode: "chat",
            loopMaxIterations: 5,
            status: "ENABLED",
            createdAt: "2026-06-09T00:00:00Z",
            updatedAt: "2026-06-09T00:00:00Z",
          },
        ],
      });
    }
    if (url.includes("/api/v1/adk/providers")) {
      return createResponse({
        providers: [
          {
            id: "provider-1",
            displayName: "OpenAI",
            baseUrl: "https://api.openai.com/v1",
            model: "gpt-4o-mini",
            enabled: true,
            default: true,
            hasApiKey: true,
            createdAt: "2026-06-09T00:00:00Z",
            updatedAt: "2026-06-09T00:00:00Z",
          },
        ],
      });
    }
    if (url.includes("/api/v1/adk/tools")) {
      return createResponse({
        tools: [
          {
            name: "strategy.save_draft",
            displayName: "Save Draft",
            description: "save draft",
            category: "strategy",
            permission: "write_strategy",
            allowedModes: ["approval", "less_approval", "all"],
            requiresApprovalIn: ["approval"],
            inputSchema: {},
          },
        ],
      });
    }
    if (url.includes("/api/v1/adk/approvals")) {
      return createResponse({ approvals: [] });
    }
    if (url.includes("/api/v1/adk/sessions")) {
      return createResponse({ sessions: [] });
    }
    if (url.includes("/api/v1/market-data/subscriptions")) {
      if (method === "POST") {
        return createResponse({
          totalActiveSubscriptions: 1,
          quota: {
            totalUsed: 1,
            totalLimit: 500,
            totalRemaining: 499,
            byMarket: [
              { market: "HK", used: 1, limit: null, remaining: null },
            ],
          },
          entries: [
            {
              key: "SNAPSHOT:HK.00700",
              channel: "SNAPSHOT",
              market: "HK",
              symbol: "00700",
              instrumentId: "HK.00700",
              interval: null,
              depthLevel: null,
              consumers: ["web:market-page:test"],
              refCount: 1,
              createdAt: "2026-05-17T00:00:00.000Z",
              updatedAt: "2026-05-17T00:01:00.000Z",
            },
          ],
        });
      }
      return createResponse(emptyMarketDataSubscriptions);
    }
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
          openPrice: 319.8,
          highPrice: 321,
          lowPrice: 319.6,
          previousClosePrice: 318.9,
          volume: 1280000,
          turnover: 410240000,
          at: "2026-05-17T01:30:00.000Z",
          session: "regular",
        },
        meta: {
          instrumentId: "HK.00700",
          source: "api-sample-cache",
          resolvedAt: "2026-05-17T01:30:00.000Z",
          fromCache: true,
        },
      });
    }
    if (url.includes("/api/v1/market-data/securities/HK/00700")) {
      return createResponse({
        request: {
          market: "HK",
          symbol: "00700",
          instrumentId: "HK.00700",
        },
        security: null,
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
      return createResponse({
        market: "HK",
        prefix: "HK",
        code: "00700",
        symbol: "HK.00700",
        instrumentId: "HK.00700",
        resolvedMarket: "HK",
      });
    }
    if (url.includes("/api/v1/market-data/instruments")) {
      return createResponse({
        query:
          new URL(String(url), "http://127.0.0.1:3000").searchParams.get(
            "query",
          ) ?? "",
        totalReturned: 1,
        entries: [
          {
            market: "HK",
            symbol: "00700",
            instrumentId: "HK.00700",
            name: "Tencent Holdings",
            securityType: "STOCK",
            lotSize: 100,
            exchange: "HKEX",
            status: "NORMAL",
            source: "seed",
            updatedAt: "2026-05-17T00:00:00.000Z",
            brokerMappings: [],
          },
        ],
      });
    }
    if (url.includes("/api/v1/system/status")) {
      return createResponse(emptySystemStatus);
    }
    if (url.includes("/api/v1/system/storage/overview")) {
      return createResponse(emptyStorageOverview);
    }
    if (url.includes("/api/v1/system/real-trade-approvals")) {
      return createResponse(emptyRealTradeApprovals);
    }
    if (url.includes("/api/v1/system/real-trade-hard-stops")) {
      return createResponse(emptyRealTradeHardStops);
    }
    if (url.includes("/api/v1/system/real-trade-hard-stop-events")) {
      return createResponse(emptyRealTradeHardStopEvents);
    }
    if (url.includes("/api/v1/system/real-trade-kill-switch-events")) {
      return createResponse(emptyRealTradeKillSwitchEvents);
    }
    if (url.includes("/api/v1/system/real-trade-kill-switch")) {
      return createResponse(emptyRealTradeKillSwitchState);
    }
    if (url.includes("/api/v1/system/real-trade-risk-events")) {
      return createResponse(emptyRealTradeRiskEvents);
    }
    if (url.includes("/api/v1/system/real-trade-risk-limits")) {
      return createResponse(emptyRealTradeRiskState);
    }
    if (url.includes("/api/v1/system/worker/broker-order-updates")) {
      return createResponse(emptyWorkerBrokerOrderUpdates);
    }
    if (url.includes("/api/v1/system/futu-opend/install-guide")) {
      return createResponse({
        brokerId: "futu",
        title: "Futu OpenD",
        description: "Install and configure OpenD.",
        options: [],
        nextSteps: [],
        settings: {
          host: "127.0.0.1",
          apiPort: 11110,
          websocketPort: 11111,
          useEncryption: false,
          websocketKeyRequired: false,
        },
      });
    }
    if (url.includes("/api/v1/brokers/futu/runtime")) {
      return createResponse(emptyBrokerRuntime);
    }
    if (url.includes("/api/v1/brokers/futu/funds")) {
      return createResponse(emptyBrokerFunds);
    }
    if (url.includes("/api/v1/brokers/futu/positions")) {
      return createResponse(emptyBrokerPositions);
    }
    if (url.includes("/api/v1/brokers/futu/orders")) {
      return createResponse(emptyBrokerOrders);
    }
    if (url.includes("/api/v1/brokers/futu/cash-flows")) {
      return createResponse(emptyBrokerCashFlows);
    }
    if (url.includes("/api/v1/portfolio/futu/cash-balances")) {
      return createResponse(emptyPortfolioCashBalances);
    }
    if (url.includes("/api/v1/portfolio/futu/positions")) {
      return createResponse(emptyPortfolioPositions);
    }
    if (url.includes("/api/v1/portfolio/futu/cash-reconciliation")) {
      return createResponse(emptyPortfolioCashReconciliation);
    }
    if (url.includes("/api/v1/portfolio/futu/reconciliation")) {
      return createResponse(emptyPortfolioReconciliation);
    }
    if (url.includes("/api/v1/execution/orders")) {
      return createResponse(emptyExecutionOrders);
    }

    throw new Error(`Unexpected request: ${url}`);
  });
}

describe("App auth and dock assistant integration", () => {
  it("does not let a Web client bypass login through a standalone desktop route", async () => {
    (
      window as Window & {
        __JFTRADE_RUNTIME_CONFIG__?: {
          authRequired?: boolean;
          desktopMode?: boolean;
        };
      }
    ).__JFTRADE_RUNTIME_CONFIG__ = {
      authRequired: true,
      desktopMode: false,
    };
    vi.stubGlobal("fetch", buildAuthWorkspaceFetchMock());

    const { wrapper } = await mountApp("/desktop-logs");
    await flushRequests();

    expect(wrapper.text()).toContain("JFTrade Web 登录");
    wrapper.unmount();
  });

  it("authenticates into workspace and mounts the shared dock assistant path", async () => {
    (
      window as Window & {
        __JFTRADE_RUNTIME_CONFIG__?: { authRequired?: boolean };
      }
    ).__JFTRADE_RUNTIME_CONFIG__ = { authRequired: true };
    window.sessionStorage.setItem(
      "jftrade.workspace.view.v1",
      JSON.stringify(workspaceViewState),
    );

    const fetchMock = buildAuthWorkspaceFetchMock();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { router, wrapper } = await mountApp("/workspace");
    await flushRequests();

    expect(wrapper.text()).toContain("JFTrade Web 登录");

    await wrapper.get("#web-access-password").setValue("test-web-password");
    await wrapper.get("form").trigger("submit");
    await flushRequests();

    expect(router.currentRoute.value.path).toBe("/workspace");
    expect(wrapper.text()).not.toContain("JFTrade Web 登录");
    expect(
      wrapper.get('[data-testid="rightdock-tab-ai"]').classes(),
    ).toContain("is-active");
    expect(wrapper.find(".adk-shell--mobile").exists()).toBe(true);
    expect(wrapper.find(".adk-mobile-toolbar").exists()).toBe(true);
    expect(wrapper.find(".adk-thread--mobile").exists()).toBe(true);
    expect(wrapper.find(".adk-chat-thread--mobile").exists()).toBe(true);
    expect(wrapper.find(".adk-empty--mobile").exists()).toBe(true);
    expect(
      wrapper.find("[data-testid='adk-mobile-composer-summary']").exists(),
    ).toBe(true);
    expect(wrapper.find(".adk-agent-select").exists()).toBe(false);
    expect(wrapper.find(".adk-provider-select").exists()).toBe(false);
    expect(
      wrapper.find("[data-testid='adk-mobile-session-drawer']").exists(),
    ).toBe(false);
    expect(wrapper.text()).toContain("开始与智能体对话");

    await wrapper.get("[data-testid='adk-mobile-sessions-toggle']").trigger(
      "click",
    );
    await nextTick();
    expect(
      wrapper.find("[data-testid='adk-mobile-session-drawer']").exists(),
    ).toBe(true);
    expect(wrapper.find(".adk-mobile-sheet").exists()).toBe(false);

    await wrapper.get("[data-testid='adk-mobile-composer-toggle']").trigger(
      "click",
    );
    await nextTick();
    expect(wrapper.find(".adk-agent-select").exists()).toBe(true);
    expect(wrapper.find(".adk-provider-select").exists()).toBe(true);

    const requestedUrls = fetchMock.mock.calls.map(([input]) => String(input));
    expect(requestedUrls).toEqual(
      expect.arrayContaining([
        "/api/v1/auth/session",
        "/api/v1/auth/login",
        "/api/v1/adk/agents",
        "/api/v1/adk/providers",
        "/api/v1/adk/tools",
        "/api/v1/adk/sessions",
      ]),
    );

    await wrapper.get("[data-testid='web-logout']").trigger("click");
    await flushRequests();
    expect(wrapper.text()).toContain("JFTrade Web 登录");

    wrapper.unmount();
  });
});
