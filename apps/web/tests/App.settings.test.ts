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

describe("Settings page", () => {
  it("renders persisted broker settings and managed accounts", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/system/storage/overview"))
        return createResponse(emptyStorageOverview);
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
                  websocketKey: "persisted-key",
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
              id: "acct-1",
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
          ],
        });
      }
      if (url.includes("/api/v1/system/futu-opend/install-guide")) {
        return createResponse({
          brokerId: "futu",
          title: "Futu OpenD 安装引导",
          description:
            "OpenD 由用户从富途官方下载安装到本机或服务器，JFTrade 券商组件只通过配置的 OpenD API 地址连接它。",
          options: [
            {
              id: "gui",
              label: "图形交互版 OpenD",
              description: "适合本机桌面环境。",
              url: "https://openapi.futunn.com/futu-api-doc/quick/opend-base.html",
              recommended: true,
            },
            {
              id: "command-line",
              label: "命令行版 OpenD",
              description: "适合服务器或无人值守环境。",
              url: "https://openapi.futunn.com/futu-api-doc/opend/opend-cmd.html",
              recommended: false,
            },
          ],
          nextSteps: [
            "按富途官方文档完成安装与登录。",
            "回到 Futu Integration 填写 OpenD 连接信息并确认已开启 WebSocket。",
            "如果 OpenD 配置了 WebSocket 密码，请在 JFTrade 的 WebSocket Password / Key 中填写同一明文密码；命令行版 OpenD 则在 FutuOpenD.xml 或 -cfg_file 参数文件中维护 websocket_key_md5。",
          ],
          settings: {
            host: "127.0.0.1",
            apiPort: 11110,
            websocketPort: 11111,
            useEncryption: false,
            websocketKeyRequired: false,
          },
        });
      }
      if (url.includes("/api/v1/plugins")) {
        return createResponse({
          targetDir: "/tmp/jftrade-settings-plugins",
          plugins: [],
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
          session: {
            ...emptyBrokerRuntime.session,
            connectivity: "connected",
            globalState: {
              quoteLoggedIn: true,
              tradeLoggedIn: true,
              serverVersion: "1005",
              programStatus: "READY",
              timestamp: "2026-05-18T08:49:53.168Z",
              markets: [],
            },
            accountsDiscovered: 1,
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
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/settings");

    expect(wrapper.text()).toContain("Settings / Configuration");
    expect(wrapper.text()).toContain("Futu Integration");
    expect(wrapper.text()).toContain("OpenD 连接状态");
    expect(wrapper.text()).toContain("OpenD install guide");
    expect(wrapper.text()).toContain("WebSocket Password / Key");
    expect(wrapper.text()).toContain("图形交互版 OpenD");
    expect(wrapper.text()).toContain("命令行版 OpenD");
    expect(wrapper.text()).toContain("Primary sim");
    expect(wrapper.text()).toContain("OpenD discovered accounts");

    await flushRequests();

    expect(wrapper.text()).toContain("OpenD WebSocket 已连接");
    expect(wrapper.text()).toContain("当前参数已通过运行时检测");
    expect(wrapper.text()).toContain("WebSocket 127.0.0.1:11111");
    expect(wrapper.text()).toContain("已登录");

    expect(wrapper.html()).toContain(
      "https://openapi.futunn.com/futu-api-doc/quick/opend-base.html",
    );
    expect(wrapper.html()).toContain(
      "https://openapi.futunn.com/futu-api-doc/opend/opend-cmd.html",
    );
    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/plugins/install-futu-opend/install"),
      expect.anything(),
    );

    wrapper.unmount();
  });
});
