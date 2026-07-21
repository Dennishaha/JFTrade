// @vitest-environment jsdom

import { nextTick } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyExecutionOrders,
  emptyOnboardingState,
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
  delete (
    window as Window & {
      __JFTRADE_RUNTIME_CONFIG__?: { desktopMode?: boolean };
    }
  ).__JFTRADE_RUNTIME_CONFIG__;
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

function runtimeDependencyMockResponse(url: string): Response | null {
  if (url.includes("/api/v1/system/runtime-dependencies")) {
    return createResponse({
      checkedAt: "2026-06-29T00:00:00Z",
      allRequiredSatisfied: true,
      dependencies: [
        {
          id: "node",
          displayName: "Node.js",
          required: true,
          status: "ok",
          minimumVersion: "22.0.0",
          detectedVersion: "22.1.0",
          configuredPath: "",
          effectivePath: "node",
          resolvedPath: "/usr/local/bin/node",
          source: "path",
          homepageUrl: "https://nodejs.org/",
          message: "Node.js 22.1.0 is available.",
        },
      ],
    });
  }
  if (url.includes("/api/v1/settings/pine-worker")) {
    return createResponse({
      backtestWorkerLimit: 2,
      instanceWorkerLimit: 10,
      nodeBinaryPath: "",
    });
  }
  return null;
}

describe("Settings page", () => {
  it("renders persisted broker settings and runtime discovery", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      const dependencyResponse = runtimeDependencyMockResponse(url);
      if (dependencyResponse != null) {
        return dependencyResponse;
      }

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/settings/onboarding")) {
        return createResponse(emptyOnboardingState);
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
      if (url.includes("/api/v1/settings/exchange-calendars")) {
        return createResponse({
          exchangeCalendars: {
            autoRefreshEnabled: true,
            errorNotificationsEnabled: true,
            refreshIntervalHours: 24,
            warmupMarkets: ["US", "HK", "CN"],
            sourcePolicies: [],
            manualOverrides: [],
          },
        });
      }
      if (url.includes("/api/v1/system/exchange-calendars/status")) {
        return createResponse({
          autoRefreshEnabled: true,
          refreshIntervalHours: 24,
          warmupMarkets: ["US", "HK", "CN"],
          markets: [],
          sources: [],
          snapshots: [],
        });
      }
      if (url.includes("/api/v1/system/futu-opend/install-guide")) {
        return createResponse({
          brokerId: "futu",
          title: "Futu OpenD 安装指引",
          description:
            "Install OpenD and return to the console to configure it.",
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
          nextSteps: ["完成安装并登录 OpenD。", "回到富途接入页填写连接信息。"],
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
      if (url.includes("/api/v1/system/futu-opend")) {
        return createResponse({
          checkedAt: "2026-05-18T08:49:53.168Z",
          status: "healthy",
          runtime: {
            connectivity: "connected",
            host: "127.0.0.1",
            port: 11110,
            apiPort: 11110,
            websocketPort: 11111,
            useEncryption: false,
            websocketKeyConfigured: true,
            marketDataTransport: "bbgo-opend-tcp-api",
            quoteLoggedIn: true,
            tradeLoggedIn: true,
            programStatus: "READY",
            serverVersion: "1005",
            lastError: null,
          },
          diagnosis: {
            code: "NONE",
            summary: null,
            manualRetryRequired: false,
            restartOpenDRecommended: false,
          },
          localSocketDiagnostics: {
            websocketEstablishedConnections: 0,
            likelyConnectionSaturation: false,
            topClientProcesses: [],
          },
          localInstallation: {
            platform: "windows",
            installed: false,
            version: null,
            installPath: null,
            guiDetected: false,
            process: {
              running: false,
              pid: null,
              executablePath: null,
            },
          },
          latestVersion: {
            value: null,
            sourceUrl: null,
            checkedAt: null,
            status: "unknown",
            error: null,
          },
          recommendations: [],
        });
      }
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        return createResponse({
          ...emptyBrokerRuntime,
          session: {
            ...emptyBrokerRuntime.session,
            connectivity: "connected",
            checkedAt: "2026-05-18T08:49:53.168Z",
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
      if (url.includes("/api/v1/execution/orders")) {
        return createResponse(emptyExecutionOrders);
      }
      if (url.includes("/api/v1/market-data/instruments")) {
        return createResponse({ query: "", totalReturned: 0, entries: [] });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings");

    expect(wrapper.text()).toContain("设置");
    const mobileSelector = wrapper.get(".settings-page__mobile-selector select");
    expect(
      mobileSelector.findAll("option").map((option) => option.text()),
    ).toEqual([
      "依赖项管理",
      "富途接入",
      "托管账户",
      "账户发现",
      "界面外观",
      "交易所日历",
      "Web 访问",
      "系统通知",
      "PineTS Worker",
      "智能体",
      "开发者工具",
      "数据管理",
      "开源许可",
    ]);
    expect((mobileSelector.element as HTMLSelectElement).value).toBe(
      "runtime-dependencies",
    );
    expect(wrapper.text()).toContain("依赖项管理");
    expect(wrapper.text()).toContain("富途接入");
    expect(wrapper.text()).toContain("交易所日历");
    expect(wrapper.text()).toContain("OpenD 连接状态");
    expect(wrapper.text()).toContain("WebSocket 密码 / 密钥");
    expect(wrapper.text()).toContain("Primary sim");
    expect(wrapper.text()).toContain("OpenD 发现的账户");

    await flushRequests();

    expect(wrapper.text()).toContain("OpenD 已连接");
    expect(wrapper.text()).toContain("当前参数已通过运行时检测");
    expect(wrapper.text()).toContain("OpenD 127.0.0.1:11110");
    expect(wrapper.text()).toContain("已登录");

    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/plugins/install-futu-opend/install"),
      expect.anything(),
    );

    wrapper.unmount();
  });

  it("stays neutral on first-run settings before any integration is saved", async () => {
    let openDHealthRequested = false;
    let runtimeRequested = false;
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      const dependencyResponse = runtimeDependencyMockResponse(url);
      if (dependencyResponse != null) {
        return dependencyResponse;
      }

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/settings/onboarding")) {
        return createResponse(emptyOnboardingState);
      }
      if (url.includes("/api/v1/settings/brokers")) {
        return createResponse({
          brokers: [
            {
              descriptor: {
                id: "futu",
                displayName: "Futu OpenAPI via OpenD",
                environments: ["SIMULATE", "REAL"],
                capabilities: [],
                notes: [],
              },
              integration: null,
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
      if (url.includes("/api/v1/system/futu-opend/install-guide")) {
        return createResponse({
          brokerId: "futu",
          title: "Futu OpenD 安装指引",
          description: "",
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
      if (url.includes("/api/v1/system/futu-opend")) {
        openDHealthRequested = true;
        throw new Error(
          "OpenD health should not be requested before first save",
        );
      }
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        runtimeRequested = true;
        throw new Error(
          "Broker runtime should not be requested before first save",
        );
      }
      if (url.includes("/api/v1/plugins")) {
        return createResponse({ targetDir: "", plugins: [] });
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
      if (url.includes("/api/v1/execution/orders")) {
        return createResponse(emptyExecutionOrders);
      }
      if (url.includes("/api/v1/market-data/instruments")) {
        return createResponse({ query: "", totalReturned: 0, entries: [] });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings");
    await flushRequests();

    expect(openDHealthRequested).toBe(false);
    expect(runtimeRequested).toBe(false);
    expect(wrapper.text()).toContain("待保存");
    expect(wrapper.text()).toContain("填写并保存富途接入配置后");
    expect(wrapper.text()).not.toContain("手动重试 OpenD");

    const discoveryTab = wrapper
      .findAll("button")
      .find((node) => node.text() === "账户发现");
    expect(discoveryTab?.exists()).toBe(true);
    await discoveryTab!.trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("请先在富途接入中填写并保存连接配置");

    wrapper.unmount();
  });

  it("stays neutral when a saved futu integration is disabled", async () => {
    let openDHealthRequested = false;
    let runtimeRequested = false;
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      const dependencyResponse = runtimeDependencyMockResponse(url);
      if (dependencyResponse != null) {
        return dependencyResponse;
      }

      if (url.includes("/api/v1/system/status")) {
        return createResponse(emptySystemStatus);
      }
      if (url.includes("/api/v1/settings/onboarding")) {
        return createResponse(emptyOnboardingState);
      }
      if (url.includes("/api/v1/settings/brokers")) {
        return createResponse({
          brokers: [
            {
              descriptor: {
                id: "futu",
                displayName: "Futu OpenAPI via OpenD",
                environments: ["SIMULATE", "REAL"],
                capabilities: [],
                notes: [],
              },
              integration: {
                brokerId: "futu",
                enabled: false,
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
                updatedAt: "2026-06-03T00:00:00Z",
                createdAt: "2026-06-03T00:00:00Z",
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
      if (url.includes("/api/v1/system/futu-opend/install-guide")) {
        return createResponse({
          brokerId: "futu",
          title: "Futu OpenD 安装指引",
          description: "",
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
      if (url.includes("/api/v1/system/futu-opend")) {
        openDHealthRequested = true;
        throw new Error(
          "OpenD health should not be requested when integration is disabled",
        );
      }
      if (url.includes("/api/v1/brokers/futu/runtime")) {
        runtimeRequested = true;
        throw new Error(
          "Broker runtime should not be requested when integration is disabled",
        );
      }
      if (url.includes("/api/v1/plugins")) {
        return createResponse({ targetDir: "", plugins: [] });
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
      if (url.includes("/api/v1/execution/orders")) {
        return createResponse(emptyExecutionOrders);
      }
      if (url.includes("/api/v1/market-data/instruments")) {
        return createResponse({ query: "", totalReturned: 0, entries: [] });
      }

      throw new Error(`Unexpected request: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings");
    await flushRequests();

    expect(openDHealthRequested).toBe(false);
    expect(runtimeRequested).toBe(false);
    expect(wrapper.text()).toContain("已停用");
    expect(wrapper.text()).toContain("当前富途接入配置已保存，但处于停用状态");
    expect(wrapper.text()).not.toContain("手动重试 OpenD");

    wrapper.unmount();
  });

  it("persists local appearance changes made before initial hydration finishes", async () => {
    let resolveAppearanceFetch: ((value: Response) => void) | null = null;
    const fetchMock = vi.fn(
      (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        const dependencyResponse = runtimeDependencyMockResponse(url);
        if (dependencyResponse != null) {
          return Promise.resolve(dependencyResponse);
        }

        if (
          url.includes("/api/v1/settings/ui") &&
          (init?.method ?? "GET") === "GET"
        ) {
          return new Promise<Response>((resolve) => {
            resolveAppearanceFetch = resolve;
          });
        }
        if (url.includes("/api/v1/settings/ui") && init?.method === "PUT") {
          return Promise.resolve(
            createResponse({
              appearance: {
                upColor: "#0055aa",
                downColor: "#ea3943",
              },
            }),
          );
        }
        if (url.includes("/api/v1/system/status")) {
          return Promise.resolve(createResponse(emptySystemStatus));
        }
        if (url.includes("/api/v1/settings/brokers")) {
          return Promise.resolve(
            createResponse({
              brokers: [],
              accounts: [],
            }),
          );
        }
        if (url.includes("/api/v1/settings/onboarding")) {
          return Promise.resolve(createResponse(emptyOnboardingState));
        }
        if (url.includes("/api/v1/system/futu-opend/install-guide")) {
          return Promise.resolve(
            createResponse({
              brokerId: "futu",
              title: "Futu OpenD 安装指引",
              description: "",
              options: [],
              nextSteps: [],
              settings: {
                host: "127.0.0.1",
                apiPort: 11110,
                websocketPort: 11111,
                useEncryption: false,
                websocketKeyRequired: false,
              },
            }),
          );
        }
        if (url.includes("/api/v1/plugins")) {
          return Promise.resolve(
            createResponse({
              targetDir: "/tmp/jftrade-settings-plugins",
              plugins: [],
            }),
          );
        }
        if (url.includes("/api/v1/system/real-trade-approvals")) {
          return Promise.resolve(createResponse(emptyRealTradeApprovals));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stops")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStops));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stop-events")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStopEvents));
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch-events")) {
          return Promise.resolve(
            createResponse(emptyRealTradeKillSwitchEvents),
          );
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch")) {
          return Promise.resolve(createResponse(emptyRealTradeKillSwitchState));
        }
        if (url.includes("/api/v1/system/real-trade-risk-events")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskEvents));
        }
        if (url.includes("/api/v1/system/real-trade-risk-limits")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskState));
        }
        if (url.includes("/api/v1/brokers/futu/runtime")) {
          return Promise.resolve(createResponse(emptyBrokerRuntime));
        }
        if (url.includes("/api/v1/brokers/futu/funds")) {
          return Promise.resolve(createResponse(emptyBrokerFunds));
        }
        if (url.includes("/api/v1/brokers/futu/positions")) {
          return Promise.resolve(createResponse(emptyBrokerPositions));
        }
        if (url.includes("/api/v1/brokers/futu/orders")) {
          return Promise.resolve(createResponse(emptyBrokerOrders));
        }
        if (url.includes("/api/v1/brokers/futu/cash-flows")) {
          return Promise.resolve(createResponse(emptyBrokerCashFlows));
        }
        if (url.includes("/api/v1/portfolio/futu/cash-balances")) {
          return Promise.resolve(createResponse(emptyPortfolioCashBalances));
        }
        if (url.includes("/api/v1/portfolio/futu/positions")) {
          return Promise.resolve(createResponse(emptyPortfolioPositions));
        }
        if (url.includes("/api/v1/execution/orders")) {
          return Promise.resolve(createResponse(emptyExecutionOrders));
        }
        if (url.includes("/api/v1/market-data/instruments")) {
          return Promise.resolve(
            createResponse({ query: "", totalReturned: 0, entries: [] }),
          );
        }

        throw new Error(`Unexpected request: ${url}`);
      },
    );

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings");

    const appearanceTab = wrapper
      .findAll("button")
      .find((node) => node.text() === "界面外观");
    expect(appearanceTab?.exists()).toBe(true);
    await appearanceTab!.trigger("click");
    await nextTick();

    const textInputs = wrapper.findAll("input[type='text']");
    expect(textInputs.length).toBeGreaterThanOrEqual(2);
    await textInputs[0]!.setValue("#0055aa");
    await textInputs[0]!.trigger("input");

    resolveAppearanceFetch?.(
      createResponse({
        appearance: {
          upColor: "#16c784",
          downColor: "#ea3943",
        },
      }),
    );
    await flushRequests();

    const putCall = fetchMock.mock.calls.find(
      ([, init]) => String(init?.method ?? "GET").toUpperCase() === "PUT",
    );
    expect(putCall).toBeTruthy();
    expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({
      appearance: {
        upColor: "#0055aa",
        downColor: "#ea3943",
      },
    });

    const refreshedTextInputs = wrapper.findAll("input[type='text']");
    expect((refreshedTextInputs[0]!.element as HTMLInputElement).value).toBe(
      "#0055aa",
    );

    wrapper.unmount();
  });

  it("renders and explicitly saves Web access settings", async () => {
    (
      window as Window & {
        __JFTRADE_RUNTIME_CONFIG__?: { desktopMode?: boolean };
      }
    ).__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };
    const fetchMock = vi.fn(
      (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        const dependencyResponse = runtimeDependencyMockResponse(url);
        if (dependencyResponse != null) {
          return Promise.resolve(dependencyResponse);
        }

        if (
          url.includes("/api/v1/settings/security") &&
          (init?.method ?? "GET") === "GET"
        ) {
          return Promise.resolve(createResponse({
            webAccessEnabled: false,
            publicAccessEnabled: false,
            passwordConfigured: false,
          }));
        }
        if (
          url.includes("/api/v1/settings/security") &&
          init?.method === "PUT"
        ) {
          return Promise.resolve(createResponse({
            webAccessEnabled: true,
            publicAccessEnabled: false,
            passwordConfigured: true,
          }));
        }
        if (url.includes("/api/v1/system/status")) {
          return Promise.resolve(createResponse(emptySystemStatus));
        }
        if (url.includes("/api/v1/settings/brokers")) {
          return Promise.resolve(createResponse({ brokers: [], accounts: [] }));
        }
        if (url.includes("/api/v1/settings/onboarding")) {
          return Promise.resolve(createResponse(emptyOnboardingState));
        }
        if (url.includes("/api/v1/plugins")) {
          return Promise.resolve(
            createResponse({
              targetDir: "/tmp/jftrade-settings-plugins",
              plugins: [],
            }),
          );
        }
        if (url.includes("/api/v1/system/real-trade-approvals")) {
          return Promise.resolve(createResponse(emptyRealTradeApprovals));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stops")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStops));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stop-events")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStopEvents));
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch")) {
          return Promise.resolve(createResponse(emptyRealTradeKillSwitchState));
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch-events")) {
          return Promise.resolve(createResponse(emptyRealTradeKillSwitchEvents));
        }
        if (url.includes("/api/v1/system/real-trade-risk")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskState));
        }
        if (url.includes("/api/v1/system/real-trade-risk-events")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskEvents));
        }
        if (url.includes("/api/v1/market-data/instruments")) {
          return Promise.resolve(
            createResponse({ query: "", totalReturned: 0, entries: [] }),
          );
        }
        if (url.includes("/api/v1/execution/orders")) {
          return Promise.resolve(createResponse(emptyExecutionOrders));
        }
        throw new Error(`Unexpected request: ${url}`);
      },
    );

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings/security");
    await flushRequests();

    expect(wrapper.text()).toContain("Web 访问");
    expect(wrapper.text()).toContain("默认仅使用桌面应用，无需密码");
    expect(wrapper.text()).toContain("未开启");

    const checkbox = wrapper.find("[data-testid='web-access-enabled-toggle']");
    expect(checkbox.exists()).toBe(true);
    await checkbox.setValue(true);
    await wrapper
      .get("[data-testid='web-access-new-password']")
      .setValue("browser-web-passphrase");
    await wrapper
      .get("[data-testid='web-access-confirm-password']")
      .setValue("browser-web-passphrase");
    await wrapper
      .get("[data-testid='web-access-settings-form']")
      .trigger("submit");
    await flushRequests();

    const putCall = fetchMock.mock.calls.find(
      ([url, init]) =>
        String(url).includes("/api/v1/settings/security") &&
        init?.method === "PUT",
    );
    expect(putCall).toBeTruthy();
    expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({
      webAccessEnabled: true,
      publicAccessEnabled: false,
      webPort: 6688,
      newPassword: "browser-web-passphrase",
    });
    expect(wrapper.text()).toContain("已开启 · 仅本机");

    wrapper.unmount();
  });

  it("shows Web access settings in desktop mode", async () => {
    (
      window as Window & {
        __JFTRADE_RUNTIME_CONFIG__?: { desktopMode?: boolean };
      }
    ).__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };
    const fetchMock = vi.fn(
      (input: string | URL | Request) => {
        const url = String(input);
        const dependencyResponse = runtimeDependencyMockResponse(url);
        if (dependencyResponse != null) {
          return Promise.resolve(dependencyResponse);
        }
        if (url.includes("/api/v1/settings/security")) {
          return Promise.resolve(createResponse({
            webAccessEnabled: false,
            publicAccessEnabled: false,
            passwordConfigured: false,
          }));
        }
        if (url.includes("/api/v1/system/status")) {
          return Promise.resolve(createResponse(emptySystemStatus));
        }
        if (url.includes("/api/v1/settings/brokers")) {
          return Promise.resolve(createResponse({ brokers: [], accounts: [] }));
        }
        if (url.includes("/api/v1/settings/onboarding")) {
          return Promise.resolve(createResponse(emptyOnboardingState));
        }
        if (url.includes("/api/v1/plugins")) {
          return Promise.resolve(createResponse({ targetDir: "", plugins: [] }));
        }
        if (url.includes("/api/v1/system/real-trade-approvals")) {
          return Promise.resolve(createResponse(emptyRealTradeApprovals));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stops")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStops));
        }
        if (url.includes("/api/v1/system/real-trade-hard-stop-events")) {
          return Promise.resolve(createResponse(emptyRealTradeHardStopEvents));
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch-events")) {
          return Promise.resolve(createResponse(emptyRealTradeKillSwitchEvents));
        }
        if (url.includes("/api/v1/system/real-trade-kill-switch")) {
          return Promise.resolve(createResponse(emptyRealTradeKillSwitchState));
        }
        if (url.includes("/api/v1/system/real-trade-risk-events")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskEvents));
        }
        if (url.includes("/api/v1/system/real-trade-risk-limits")) {
          return Promise.resolve(createResponse(emptyRealTradeRiskState));
        }
        if (url.includes("/api/v1/market-data/instruments")) {
          return Promise.resolve(createResponse({ query: "", totalReturned: 0, entries: [] }));
        }
        if (url.includes("/api/v1/execution/orders")) {
          return Promise.resolve(createResponse(emptyExecutionOrders));
        }
        throw new Error(`Unexpected request: ${url}`);
      },
    );

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/settings/security");
    await flushRequests();

    const mobileSelector = wrapper.get(".settings-page__mobile-selector select");
    expect(
      mobileSelector.findAll("option").map((option) => option.text()),
    ).toContain("Web 访问");
    expect(wrapper.text()).toContain("默认仅使用桌面应用，无需密码");
    expect(wrapper.find("[data-testid='web-access-enabled-toggle']").exists()).toBe(true);

    wrapper.unmount();
  });
});
