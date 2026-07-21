// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
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
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

function createFutuDefaults() {
  return {
    type: "futu",
    host: "127.0.0.1",
    apiPort: 11110,
    websocketPort: 11111,
    maxWebSocketConnections: 20,
    useEncryption: false,
    websocketKey: "",
    tradeMarket: "HK",
    securityFirm: "FUTUSECURITIES",
  };
}

describe("OOBE onboarding", () => {
  it("does not probe OpenD before the user saves an enabled futu integration", async () => {
    let completed = false;
    let lastBrokerId = "";
    let savedIntegration: null | {
      brokerId: string;
      enabled: boolean;
      config: ReturnType<typeof createFutuDefaults>;
      updatedAt: string;
      createdAt: string;
    } = null;
    let openDHealthRequests = 0;
    let runtimeRequests = 0;

    const fetchMock = vi.fn(
      async (input: string | URL | Request, init?: RequestInit) => {
        const url = String(input);
        const method = String(init?.method ?? "GET").toUpperCase();

        if (url.includes("/api/v1/settings/onboarding") && method === "PUT") {
          const payload = JSON.parse(String(init?.body ?? "{}")) as {
            completed?: boolean;
            dismissed?: boolean;
            lastBrokerId?: string;
          };
          completed = payload.completed === true || payload.dismissed === true;
          lastBrokerId = payload.lastBrokerId?.trim() || lastBrokerId;
          return createResponse({
            ...emptyOnboardingState,
            state: {
              completed,
              lastBrokerId,
            },
            shouldShowOobe: !completed,
            recommendedBrokerId: "futu",
            reasons: [],
            brokers: [
              {
                descriptor: {
                  id: "futu",
                  displayName: "Futu OpenAPI via OpenD",
                  environments: ["SIMULATE", "REAL"],
                  capabilities: [],
                  notes: [],
                },
                enabled: savedIntegration?.enabled ?? false,
                available: true,
                configured: savedIntegration != null,
              },
            ],
          });
        }

        if (url.includes("/api/v1/settings/onboarding")) {
          return createResponse({
            state: { completed, lastBrokerId },
            shouldShowOobe: !completed,
            recommendedBrokerId: "futu",
            reasons: [],
            brokers: [
              {
                descriptor: {
                  id: "futu",
                  displayName: "Futu OpenAPI via OpenD",
                  environments: ["SIMULATE", "REAL"],
                  capabilities: [],
                  notes: [],
                },
                enabled: savedIntegration?.enabled ?? false,
                available: true,
                configured: savedIntegration != null,
              },
            ],
          });
        }

        if (url.includes("/api/v1/system/status")) {
          return createResponse(emptySystemStatus);
        }
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
        if (
          url.includes("/api/v1/settings/brokers/futu/integration") &&
          method === "PUT"
        ) {
          const payload = JSON.parse(String(init?.body ?? "{}")) as {
            enabled: boolean;
            config: ReturnType<typeof createFutuDefaults>;
          };
          savedIntegration = {
            brokerId: "futu",
            enabled: payload.enabled,
            config: payload.config,
            updatedAt: "2026-06-03T00:00:00Z",
            createdAt: "2026-06-03T00:00:00Z",
          };
          return createResponse(savedIntegration);
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
                integration: savedIntegration,
                defaults: createFutuDefaults(),
              },
            ],
            accounts: [],
          });
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
        if (url.includes("/api/v1/system/futu-opend")) {
          openDHealthRequests += 1;
          return createResponse({
            checkedAt: "2026-06-03T00:00:00Z",
            status: "healthy",
            runtime: {
              connectivity: "connected",
              host: "127.0.0.1",
              port: 11110,
              apiPort: 11110,
              websocketPort: 11111,
              useEncryption: false,
              websocketKeyConfigured: false,
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
        if (url.includes("/api/v1/brokers/futu/runtime")) {
          runtimeRequests += 1;
          return createResponse({
            descriptor: {
              id: "futu",
              displayName: "Futu OpenAPI via OpenD",
              environments: ["SIMULATE", "REAL"],
              capabilities: [],
              notes: [],
            },
            session: {
              brokerId: "futu",
              displayName: "Futu OpenAPI via OpenD",
              connection: {
                host: "127.0.0.1",
                apiPort: 11110,
                websocketPort: 11111,
                port: 11110,
                useEncryption: false,
              },
              connectivity: "connected",
              checkedAt: "2026-06-03T00:00:00Z",
              lastError: null,
              globalState: {
                quoteLoggedIn: true,
                tradeLoggedIn: true,
                serverVersion: "1005",
                programStatus: "READY",
                timestamp: "2026-06-03T00:00:00Z",
                markets: [],
              },
              accountsDiscovered: 1,
              liveWebSocketClients: {
                connected: 0,
                limit: 20,
                atLimit: false,
              },
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
      },
    );

    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { router, wrapper } = await mountApp("/workspace");
    await flushRequests();

    expect(router.currentRoute.value.path).toBe("/oobe");
    expect(wrapper.text()).toContain("运行时依赖与券商接入配置");
    expect(wrapper.text()).toContain("必需依赖已满足");
    expect(wrapper.text()).toContain("Futu OpenAPI via OpenD");
    expect(openDHealthRequests).toBe(0);
    expect(runtimeRequests).toBe(0);

    const dependencyNextButton = wrapper
      .findAll("button")
      .find((button) => button.text().includes("下一步"));
    expect(dependencyNextButton?.exists()).toBe(true);
    await dependencyNextButton!.trigger("click");
    await flushRequests();

    const brokerButton = wrapper
      .findAll("button")
      .find((button) => button.text().includes("Futu OpenAPI via OpenD"));
    expect(brokerButton?.exists()).toBe(true);
    await brokerButton!.trigger("click");
    await flushRequests();

    const nextButtons = wrapper
      .findAll("button")
      .filter((button) => button.text().includes("下一步"));
    await nextButtons.at(-1)!.trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("保存并检测 OpenD");
    expect(openDHealthRequests).toBe(0);
    expect(runtimeRequests).toBe(0);

    const enableCheckbox = wrapper.find("input[type='checkbox']");
    expect(enableCheckbox.exists()).toBe(true);
    await enableCheckbox.setValue(true);
    await flushRequests();

    const saveButton = wrapper
      .findAll("button")
      .find((button) => button.text().includes("保存并检测 OpenD"));
    expect(saveButton?.exists()).toBe(true);
    await saveButton!.trigger("click");
    await flushRequests();

    expect(openDHealthRequests).toBeGreaterThan(0);
    expect(runtimeRequests).toBeGreaterThan(0);
    expect(wrapper.text()).toContain("OpenD 已连接");

    const connectionNextButton = wrapper
      .findAll("button")
      .filter((button) => button.text().includes("下一步"))
      .at(-1);
    await connectionNextButton!.trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("OpenD 发现的账户");
    expect(wrapper.text()).toContain("SIM-001");

    wrapper.unmount();
  });
});
