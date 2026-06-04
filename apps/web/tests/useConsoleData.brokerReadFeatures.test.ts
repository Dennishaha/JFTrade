// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";

import {
  type BrokerFundsResponse,
  type BrokerReadFeatureCapability,
  type BrokerReadFeatureKey,
  type BrokerRuntimeResponse,
  type ExecutionOrdersResponse,
  emptyBrokerCashFlows,
  emptyBrokerFills,
  emptyBrokerFunds,
  emptyBrokerMarginRatios,
  emptyBrokerMaxTradeQuantity,
  emptyBrokerOrderFees,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptyExecutionOrderEvents,
  emptyExecutionOrders,
  emptyFutuOpenDHealth,
  emptyFutuOpenDInstallGuide,
  emptyPluginCatalog,
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

import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";
import { createResponse } from "./helpers";

function createConsoleStore() {
  let store: ReturnType<typeof provideConsoleDataStore> | null = null;

  const Host = defineComponent({
    setup() {
      const workspaceLayout = provideWorkspaceLayoutStore();
      store = provideConsoleDataStore(workspaceLayout);
      return () => null;
    },
  });

  mount(Host, {
    global: {
      plugins: [createPinia()],
    },
  });

  if (store == null) {
    throw new Error("Failed to create console data store.");
  }

  return store;
}

function createReadFeatures(
  overrides: Partial<
    Record<BrokerReadFeatureKey, Partial<BrokerReadFeatureCapability>>
  > = {},
): Record<BrokerReadFeatureKey, BrokerReadFeatureCapability> {
  return {
    funds: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      ...overrides.funds,
    },
    positions: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      ...overrides.positions,
    },
    orders: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      supportsHistory: true,
      ...overrides.orders,
    },
    fills: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      supportsHistory: true,
      ...overrides.fills,
    },
    cashFlows: {
      supportedEnvironments: ["REAL"],
      requiresClearingDate: true,
      ...overrides.cashFlows,
    },
    orderFees: {
      supportedEnvironments: ["REAL"],
      requiresOrderIdEx: true,
      ...overrides.orderFees,
    },
    marginRatios: {
      supportedEnvironments: ["REAL"],
      requiresSymbols: true,
      ...overrides.marginRatios,
    },
    maxTradeQuantity: {
      supportedEnvironments: ["SIMULATE", "REAL"],
      requiresPrice: true,
      ...overrides.maxTradeQuantity,
    },
  };
}

function createBrokerRuntime(
  readFeatureOverrides: Partial<
    Record<BrokerReadFeatureKey, Partial<BrokerReadFeatureCapability>>
  > = {},
): BrokerRuntimeResponse {
  const capability = {
    market: "HK",
    supportsQuote: true,
    supportsTrade: true,
    readFeatures: createReadFeatures(readFeatureOverrides),
  };

  return {
    ...emptyBrokerRuntime,
    descriptor: {
      ...emptyBrokerRuntime.descriptor,
      id: "futu",
      displayName: "Test Broker",
      environments: ["SIMULATE", "REAL"],
      capabilities: [capability],
    },
    session: {
      ...emptyBrokerRuntime.session,
      brokerId: "futu",
      displayName: "Test Broker",
      connectivity: "connected",
      checkedAt: "2026-05-27T00:00:00.000Z",
      accountsDiscovered: 1,
    },
    accounts: [
      {
        accountId: "REAL-001",
        tradingEnvironment: "REAL",
        accountType: "MARGIN",
        accountRole: null,
        securityFirm: null,
        marketAuthorities: ["HK"],
        simulatedAccountType: null,
      },
    ],
  };
}

function applyBrokerRuntime(
  store: ReturnType<typeof provideConsoleDataStore>,
  runtime: BrokerRuntimeResponse,
): void {
  store.brokerRuntime.value = runtime;
  store.systemStatus.value = {
    ...emptySystemStatus,
    defaultBroker: runtime.descriptor.id,
    defaultTradingEnvironment: "REAL",
    broker: runtime.descriptor,
  };
}

function createBrokerFunds(): BrokerFundsResponse {
  return {
    ...emptyBrokerFunds,
    checkedAt: "2026-05-27T00:00:00.000Z",
    connectivity: "connected",
    summary: {
      accountId: "REAL-001",
      tradingEnvironment: "REAL",
      market: "HK",
      currency: "HKD",
      totalAssets: 100000,
      securitiesAssets: 100000,
      fundAssets: 0,
      bondAssets: 0,
      cash: 50000,
      marketValue: 50000,
      longMarketValue: 50000,
      shortMarketValue: 0,
      purchasingPower: 50000,
      shortSellingPower: 0,
      netCashPower: 50000,
      availableWithdrawalCash: 49000,
      maxWithdrawal: 49000,
      availableFunds: 49000,
      frozenCash: 0,
      pendingAsset: 0,
      unrealizedPnl: null,
      realizedPnl: null,
      initialMargin: null,
      maintenanceMargin: null,
      marginCallMargin: null,
      riskStatus: null,
    },
    currencyBalances: [],
    marketAssets: [],
  };
}

function createExecutionOrders(): ExecutionOrdersResponse {
  return {
    ...emptyExecutionOrders,
    orders: [
      {
        internalOrderId: "ord-1",
        brokerId: "futu",
        brokerOrderId: "broker-1",
        brokerOrderIdEx: null,
        tradingEnvironment: "REAL",
        accountId: "REAL-001",
        market: "HK",
        symbol: "HK.00700",
        side: "BUY",
        orderType: "LIMIT",
        status: "SUBMITTED",
        requestedQuantity: 100,
        requestedPrice: 320.5,
        filledQuantity: 0,
        filledAveragePrice: null,
        remark: null,
        lastError: null,
        lastErrorCode: null,
        lastErrorSource: null,
        submittedAt: "2026-05-27T00:00:00.000Z",
        updatedAt: "2026-05-27T00:00:00.000Z",
        createdAt: "2026-05-27T00:00:00.000Z",
      },
    ],
  };
}

function createSystemFetchMock(
  readFeatureOverrides: Partial<
    Record<BrokerReadFeatureKey, Partial<BrokerReadFeatureCapability>>
  > = {},
) {
  const runtime = createBrokerRuntime(readFeatureOverrides);
  const funds = createBrokerFunds();

  return vi.fn(async (input: string | URL | Request) => {
    const url = String(input);

    if (url.includes("/api/v1/system/status")) {
      return createResponse({
        ...emptySystemStatus,
        defaultBroker: "futu",
        defaultTradingEnvironment: "REAL",
        broker: runtime.descriptor,
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
              displayName: "Test Broker",
              environments: ["SIMULATE", "REAL"],
              capabilities: [
                {
                  market: "HK",
                  supportsQuote: true,
                  supportsTrade: true,
                  readFeatures: createReadFeatures(readFeatureOverrides),
                },
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
              updatedAt: "2026-05-27T00:00:00.000Z",
              createdAt: "2026-05-27T00:00:00.000Z",
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
    if (url.includes("/api/v1/plugins")) {
      return createResponse(emptyPluginCatalog);
    }
    if (url.includes("/api/v1/system/futu-opend/health")) {
      return createResponse(emptyFutuOpenDHealth);
    }
    if (url.includes("/api/v1/system/futu-opend/install-guide")) {
      return createResponse(emptyFutuOpenDInstallGuide);
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
    if (url.includes("/api/v1/market-data/instruments")) {
      return createResponse({ query: "", totalReturned: 0, entries: [] });
    }
    if (url.includes("/api/v1/brokers/futu/runtime")) {
      return createResponse(runtime);
    }
    if (url.includes("/api/v1/brokers/futu/funds")) {
      return createResponse(funds);
    }
    if (url.includes("/api/v1/brokers/futu/positions")) {
      return createResponse(emptyBrokerPositions);
    }
    if (url.includes("/api/v1/brokers/futu/orders")) {
      return createResponse(emptyBrokerOrders);
    }
    if (url.includes("/api/v1/brokers/futu/fills")) {
      return createResponse(emptyBrokerFills);
    }
    if (url.includes("/api/v1/brokers/futu/cash-flows")) {
      return createResponse(emptyBrokerCashFlows);
    }
    if (url.includes("/api/v1/brokers/futu/margin-ratios")) {
      return createResponse(emptyBrokerMarginRatios);
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

function requestedUrls(fetchMock: ReturnType<typeof vi.fn>): string[] {
  return fetchMock.mock.calls.map(([input]) => String(input));
}

afterEach(() => {
  window.localStorage?.clear();
  vi.unstubAllGlobals();
});

describe("console data broker readFeatures consumption", () => {
  it("uses history fill parameters when fills capability supports history", async () => {
    const store = createConsoleStore();
    const fetchMock = createSystemFetchMock({
      fills: { supportsHistory: true },
    });
    vi.stubGlobal("fetch", fetchMock);

    await store.loadSystemState({ bypassCooldown: true });

    const fillsUrl = requestedUrls(fetchMock).find((url) =>
      url.includes("/api/v1/brokers/futu/fills"),
    );

    expect(fillsUrl).toBeDefined();
    expect(fillsUrl).toContain("scope=history");
    expect(fillsUrl).toContain("startTime=");
    expect(fillsUrl).toContain("endTime=");
  });

  it("omits history fill parameters when fills capability does not support history", async () => {
    const store = createConsoleStore();
    const fetchMock = createSystemFetchMock({
      fills: { supportsHistory: false },
    });
    vi.stubGlobal("fetch", fetchMock);

    await store.loadSystemState({ bypassCooldown: true });

    const fillsUrl = requestedUrls(fetchMock).find((url) =>
      url.includes("/api/v1/brokers/futu/fills"),
    );

    expect(fillsUrl).toBeDefined();
    expect(fillsUrl).not.toContain("scope=history");
    expect(fillsUrl).not.toContain("startTime=");
    expect(fillsUrl).not.toContain("endTime=");
  });

  it("allows max trade quantity queries without price when capability marks price optional", async () => {
    const store = createConsoleStore();
    applyBrokerRuntime(
      store,
      createBrokerRuntime({
        maxTradeQuantity: { requiresPrice: false },
      }),
    );
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/brokers/futu/max-trade-qtys")) {
        return createResponse(emptyBrokerMaxTradeQuantity);
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    await store.loadBrokerMaxTradeQuantity({
      brokerId: "futu",
      tradingEnvironment: "REAL",
      accountId: "REAL-001",
      market: "HK",
      symbol: "HK.00700",
      orderType: "LIMIT",
      price: 0,
    });

    const maxTradeUrl = requestedUrls(fetchMock)[0];
    expect(maxTradeUrl).toContain("/api/v1/brokers/futu/max-trade-qtys");
    expect(maxTradeUrl).not.toContain("price=");
  });

  it("surfaces missing brokerOrderIdEx only when order-fees capability requires it", async () => {
    const store = createConsoleStore();
    store.executionOrders.value = createExecutionOrders();

    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/api/v1/execution/orders/ord-1/events")) {
        return createResponse({
          ...emptyExecutionOrderEvents,
          internalOrderId: "ord-1",
        });
      }
      if (url.includes("/api/v1/brokers/futu/order-fees")) {
        return createResponse(emptyBrokerOrderFees);
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    applyBrokerRuntime(
      store,
      createBrokerRuntime({
        orderFees: { requiresOrderIdEx: true },
      }),
    );
    await store.loadExecutionOrderDetails("ord-1");

    expect(store.orderFeesError.value).toContain("缺少券商扩展单号");
    expect(
      requestedUrls(fetchMock).filter((url) =>
        url.includes("/api/v1/brokers/futu/order-fees"),
      ),
    ).toHaveLength(0);

    store.orderFeesError.value = "";
    fetchMock.mockClear();
    applyBrokerRuntime(
      store,
      createBrokerRuntime({
        orderFees: { requiresOrderIdEx: false },
      }),
    );
    await store.loadExecutionOrderDetails("ord-1");

    expect(store.orderFeesError.value).toBe("");
    expect(
      requestedUrls(fetchMock).filter((url) =>
        url.includes("/api/v1/brokers/futu/order-fees"),
      ),
    ).toHaveLength(0);
  });
});
