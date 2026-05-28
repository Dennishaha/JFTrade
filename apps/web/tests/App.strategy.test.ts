// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h, nextTick } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";

import {
  emptyBrokerCashFlows,
  emptyBrokerSettings,
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
} from "@jftrade/ui-contracts";
import type {
  BrokerRuntimeResponse,
  StrategyDefinitionDocument,
  SystemStatusResponse,
} from "@jftrade/ui-contracts";
import StrategyLogicFlowDesigner from "../src/components/StrategyLogicFlowDesigner.vue";
import { provideConsoleDataStore } from "../src/composables/useConsoleData";
import { provideThemeStore } from "../src/composables/useTheme";
import { provideUIColorPreferencesStore } from "../src/composables/useUIColorPreferences";
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";
import StrategyPage from "../src/pages/StrategyPage.vue";

import {
  MockEventSource,
  createResponse,
  dialogStub,
  flushRequests,
} from "./helpers";

let currentStrategySystemStatus: SystemStatusResponse = emptySystemStatus;
let currentConsoleDataStore: ReturnType<typeof provideConsoleDataStore> | null = null;

const StrategyPageTestRoot = defineComponent({
  setup() {
    const themeStore = provideThemeStore();
    provideUIColorPreferencesStore(themeStore.theme);
    const workspaceLayout = provideWorkspaceLayoutStore();
    const consoleData = provideConsoleDataStore(workspaceLayout);
    currentConsoleDataStore = consoleData;
    consoleData.systemStatus.value = currentStrategySystemStatus;
    return () => h(RouterView);
  },
});

const OverviewTestPage = defineComponent({
  setup() {
    return () => h("div", "Overview");
  },
});

async function mountStrategyPage(path = "/strategy") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/strategy", component: StrategyPage as never },
      { path: "/overview", component: OverviewTestPage as never },
    ],
  });
  await router.push(path);
  await router.isReady();

  const wrapper = mount(StrategyPageTestRoot, {
    global: {
      plugins: [createPinia(), router],
      stubs: {
        "v-dialog": dialogStub,
      },
    },
  });

  await flushRequests();

  return { router, wrapper };
}

type StrategyPageWrapper = Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"];

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
  currentStrategySystemStatus = emptySystemStatus;
  currentConsoleDataStore = null;
});

async function settleStrategyWorkspace() {
  await flushRequests();
  await flushRequests();
  await nextTick();
  await nextTick();
}

function hasDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  return (
    wrapper.find('[data-testid="instantiate-strategy-definition"]').exists()
    || wrapper.find('[data-testid="toggle-strategy-templates-section"]').exists()
    || wrapper.find('[data-testid="strategy-templates-section"]').exists()
  );
}

async function openStrategyWorkspaceTab(
  wrapper: StrategyPageWrapper,
  tab: "runtime" | "design",
) {
  await wrapper
    .get(`[data-testid="strategy-workspace-tab-${tab}"]`)
    .trigger("click");
  await settleStrategyWorkspace();
  if (tab === "design") {
    await ensureStrategyDesignWorkspace(wrapper);
  }
}

async function ensureStrategyDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (hasDesignWorkspace(wrapper)) {
      return;
    }
    if (!wrapper.find('[data-testid="strategy-workspace-tab-design"]').exists()) {
      return;
    }
    await wrapper.get('[data-testid="strategy-workspace-tab-design"]').trigger("click");
    await settleStrategyWorkspace();
  }
}

async function openStrategyDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  await openStrategyWorkspaceTab(wrapper, "design");
}

async function showStrategyCodeEditor(
  wrapper: StrategyPageWrapper,
  mode: "split" | "code" = "code",
) {
  await wrapper
    .get(`[data-testid="strategy-display-mode-${mode}"]`)
    .trigger("click");
  await settleStrategyWorkspace();
}

async function openNewStrategyFromRuntime(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return;
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click");
    await settleStrategyWorkspace();
    await wrapper.get('[data-testid="strategy-new-definition"]').trigger("click");
    await settleStrategyWorkspace();
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return;
    }
  }
}

async function openCreateInstancePanel(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return;
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click");
    await settleStrategyWorkspace();
    await wrapper.get('[data-testid="strategy-new-instance"]').trigger("click");
    await settleStrategyWorkspace();
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return;
    }
  }
}

async function appendSymbolTags(
  wrapper: StrategyPageWrapper,
  selector: string,
  symbols: string[],
) {
  for (const symbol of symbols) {
    const input = wrapper.get(selector);
    await input.setValue(symbol);
    await input.trigger("keydown", { key: "Enter" });
    await settleStrategyWorkspace();
  }
}

async function openStrategyTemplatesPanel(
  wrapper: StrategyPageWrapper,
) {
  await ensureStrategyDesignWorkspace(wrapper);
  if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
    return;
  }
  await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click");
  await settleStrategyWorkspace();
}

async function waitForSelector(
  wrapper: StrategyPageWrapper,
  selector: string,
  attempts = 40,
) {
  for (let index = 0; index < attempts; index += 1) {
    if (wrapper.find(selector).exists()) {
      return;
    }
    await settleStrategyWorkspace();
  }
}

function buildFetchMock(options: {
  systemStatus?: SystemStatusResponse;
  brokerRuntime?: BrokerRuntimeResponse;
  definitions?: StrategyDefinitionDocument[];
  strategies?: Array<{
    id: string;
    pluginId?: string;
    definition: {
      strategyId: string;
      name: string;
      version: string;
    };
    runtime?: string;
    sourceFormat?: "dsl-v1";
    startable?: boolean;
    binding?: {
      symbols?: string[];
      interval?: string;
      executionMode?: "live" | "notify_only";
      brokerAccount?: {
        brokerId: string;
        accountId: string;
        tradingEnvironment: string;
        market: string;
      } | null;
    };
    params: Record<string, unknown>;
    status: "RUNNING" | "PAUSED" | "STOPPED";
    createdAt: string;
    logs: string[];
    runtimeObservation?: {
      actualStatus: "RUNNING" | "PAUSED" | "STOPPED";
      activeSymbols: string[];
      lastClosedKlineAt?: string | null;
      lastSignalAt?: string | null;
      lastOrderAt?: string | null;
      lastErrorAt?: string | null;
      lastError?: string | null;
      updatedAt?: string | null;
    } | null;
  }>;
  logsById?: Record<string, string[]>;
  auditById?: Record<
    string,
    Array<{
      instanceId: string;
      kind: string;
      detail?: string;
      at: string;
    }>
  >;
}) {
  const systemStatus = options.systemStatus ?? emptySystemStatus;
  const brokerRuntime = options.brokerRuntime ?? emptyBrokerRuntime;
  currentStrategySystemStatus = systemStatus;
  const definitions = options.definitions ?? [];
  const strategies = options.strategies ?? [];
  const logsById = options.logsById ?? {};
  const auditById = options.auditById ?? {};

  function normalizeInstrumentId(value: unknown) {
    const normalized = typeof value === "string" ? value.trim().toUpperCase() : "";
    if (normalized === "") {
      return "";
    }
    if (normalized.includes(":")) {
      const [market, symbol] = normalized.split(":", 2);
      if ((market ?? "") !== "" && (symbol ?? "") !== "") {
        return `${market}.${symbol}`;
      }
    }
    return normalized;
  }

  function normalizeBinding(
    rawBinding: unknown,
    params: Record<string, unknown>,
  ) {
    const bindingRecord = rawBinding && typeof rawBinding === "object" && !Array.isArray(rawBinding)
      ? rawBinding as Record<string, unknown>
      : {};
    const rawSymbols = Array.isArray(bindingRecord.symbols)
      ? bindingRecord.symbols
      : Array.isArray(params.symbols)
        ? params.symbols
        : typeof params.symbol === "string" && params.symbol.trim() !== ""
          ? [params.symbol]
          : [];
    const symbols = Array.from(new Set(
      rawSymbols
        .map((value) => normalizeInstrumentId(value))
        .filter((value) => value !== ""),
    ));
    const interval = typeof bindingRecord.interval === "string" && bindingRecord.interval.trim() !== ""
      ? bindingRecord.interval.trim()
      : typeof params.interval === "string" && params.interval.trim() !== ""
        ? params.interval.trim()
        : "5m";
    const executionMode = bindingRecord.executionMode === "notify_only"
      ? "notify_only"
      : bindingRecord.executionMode === "live"
        ? "live"
        : params.executionMode === "notify_only"
          ? "notify_only"
          : "live";
    const brokerAccountRecord = bindingRecord.brokerAccount && typeof bindingRecord.brokerAccount === "object" && !Array.isArray(bindingRecord.brokerAccount)
      ? bindingRecord.brokerAccount as Record<string, unknown>
      : params.brokerAccount && typeof params.brokerAccount === "object" && !Array.isArray(params.brokerAccount)
        ? params.brokerAccount as Record<string, unknown>
        : null;
    const brokerAccount = brokerAccountRecord === null
      ? null
      : {
          brokerId: String(brokerAccountRecord.brokerId ?? "").trim().toLowerCase(),
          accountId: String(brokerAccountRecord.accountId ?? "").trim(),
          tradingEnvironment: String(brokerAccountRecord.tradingEnvironment ?? "").trim().toUpperCase(),
          market: String(brokerAccountRecord.market ?? "").trim().toUpperCase(),
        };

    return {
      symbols,
      interval,
      executionMode,
      brokerAccount,
    } as const;
  }

  async function readJsonBody(init?: RequestInit, request?: Request | null) {
    if (request != null) {
      const text = await request.clone().text();
      return text === "" ? {} : JSON.parse(text);
    }
    if (typeof init?.body === "string") {
      return init.body === "" ? {} : JSON.parse(init.body);
    }
    return {};
  }

  const runtimeState = {
    strategies: strategies.map((strategy) => ({
      ...(() => {
        const runtime = strategy.runtime ?? "dsl-go-plan";
        const sourceFormat = strategy.sourceFormat ?? "dsl-v1";
        const startable =
          strategy.startable
          ?? (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan");
        const binding = normalizeBinding(strategy.binding, strategy.params);
        return {
          ...strategy,
          runtime,
          sourceFormat,
          startable,
          binding,
          params: {
            ...strategy.params,
            symbols: [...binding.symbols],
            symbol: binding.symbols[0] ?? "",
            interval: binding.interval,
            executionMode: binding.executionMode,
            ...(binding.brokerAccount === null
              ? {}
              : {
                  brokerAccount: { ...binding.brokerAccount },
                }),
          },
          definition: { ...strategy.definition },
          logs: [...strategy.logs],
        };
      })(),
    })),
    logsById: { ...logsById },
    auditById: { ...auditById },
  };

  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(input);
    const method = request?.method ?? init?.method ?? "GET";
    const logsMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/logs/);
    const auditMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/audit/);
    const instantiateMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/]+)\/instantiate/);
    const lifecycleMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/(start|pause|stop)/);
    const instanceMatch = url.match(/\/api\/v1\/strategies\/([^/]+)$/);

    if (url.includes("/api/v1/market-data/subscriptions"))
      return createResponse(emptyMarketDataSubscriptions);
    if (url.includes("/api/v1/system/status"))
      return createResponse(systemStatus);
    if (url.includes("/api/v1/system/storage/overview"))
      return createResponse(emptyStorageOverview);
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
    if (url.includes("/api/v1/brokers/futu/runtime"))
      return createResponse(brokerRuntime);
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
    if (instantiateMatch && method === "POST") {
      const definitionId = decodeURIComponent(instantiateMatch[1]);
      const definition = definitions.find((item) => item.id === definitionId);
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`);
      }
      const payload = await readJsonBody(init, request);
      const instanceId = `${definitionId}-instance`;
      const sourceFormat = definition.sourceFormat ?? "dsl-v1";
      const runtime = sourceFormat === "dsl-v1" ? "dsl-go-plan" : definition.runtime;
      const startable =
        (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan") ||
        (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan");
      const binding = normalizeBinding(payload, {
        symbol: definition.symbol ?? "",
        interval: definition.interval ?? "5m",
      });
      const instance = {
        id: instanceId,
        pluginId: "dsl-go-plan",
        definition: {
          strategyId: definition.id,
          name: definition.name,
          version: definition.version,
        },
        runtime,
        sourceFormat,
        startable,
        binding,
        params: {
          runtime,
          sourceFormat,
          definitionId: definition.id,
          symbols: [...binding.symbols],
          symbol: binding.symbols[0] ?? "",
          interval: binding.interval,
          executionMode: binding.executionMode,
          ...(binding.brokerAccount === null
            ? {}
            : {
                brokerAccount: { ...binding.brokerAccount },
              }),
          script: definition.script,
          ...(sourceFormat === "dsl-v1"
            ? {
                compiledAt: definition.updatedAt,
                compiledHooks: ["on_kline_close"],
                compiledRequirements: {
                  indicators: [
                    { alias: "fast", kind: "ma", key: "ma:EMA:5:day" },
                    { alias: "", kind: "protect", key: "risk:trailingStop:auto:2:day:4:session" },
                  ],
                  requiresPosition: true,
                  requiresAvailableCash: true,
                  requiresMarginBuyingPower: false,
                  requiresShortSellingPower: false,
                  requiresTotalAccountValue: false,
                },
              }
            : {}),
        },
        status: "STOPPED" as const,
        createdAt: definition.updatedAt,
        logs: [],
      };
      runtimeState.strategies = [instance, ...runtimeState.strategies.filter((item) => item.id !== instanceId)];
      runtimeState.logsById[instanceId] = [
        `${definition.updatedAt} instantiated strategy from definition ${definition.id}`,
      ];
      runtimeState.auditById[instanceId] = [
        {
          instanceId,
          kind: "instantiated",
          detail: `${definition.id} | interval=${binding.interval} | mode=${binding.executionMode}`,
          at: definition.updatedAt,
        },
      ];
      return createResponse(instance);
    }
    if (instanceMatch && method === "PUT") {
      const instanceId = decodeURIComponent(instanceMatch[1]);
      const instance = runtimeState.strategies.find((item) => item.id === instanceId);
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`);
      }
      const payload = await readJsonBody(init, request);
      const binding = normalizeBinding(payload, instance.params);
      instance.binding = binding;
      instance.params = {
        ...instance.params,
        symbols: [...binding.symbols],
        symbol: binding.symbols[0] ?? "",
        interval: binding.interval,
        executionMode: binding.executionMode,
        ...(binding.brokerAccount === null
          ? {}
          : {
              brokerAccount: { ...binding.brokerAccount },
            }),
      };
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        "2026-05-23T00:00:00.000Z updated strategy binding",
      ];
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: "binding.updated",
          detail: `${binding.symbols.join(", ") || "未绑定"} / ${binding.interval}`,
          at: "2026-05-23T00:00:00.000Z",
        },
      ];
      return createResponse(instance);
    }
    if (instanceMatch && method === "DELETE") {
      const instanceId = decodeURIComponent(instanceMatch[1]);
      const instanceIndex = runtimeState.strategies.findIndex((item) => item.id === instanceId);
      if (instanceIndex === -1) {
        throw new Error(`Unknown strategy instance: ${instanceId}`);
      }
      const [removed] = runtimeState.strategies.splice(instanceIndex, 1);
      delete runtimeState.logsById[instanceId];
      delete runtimeState.auditById[instanceId];
      return createResponse(removed);
    }
    if (lifecycleMatch && method === "POST") {
      const instanceId = decodeURIComponent(lifecycleMatch[1]);
      const action = lifecycleMatch[2];
      const instance = runtimeState.strategies.find((item) => item.id === instanceId);
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`);
      }
      const nextStatus = action === "start" ? "RUNNING" : action === "pause" ? "PAUSED" : "STOPPED";
      instance.status = nextStatus;
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        `2026-05-23T00:00:00.000Z ${action}ed strategy ${instance.definition.strategyId}`,
      ];
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: action === "start" ? "started" : action === "pause" ? "paused" : "stopped",
          detail: action === "pause" ? "manual pause" : `manual ${action}`,
          at: "2026-05-23T00:00:00.000Z",
        },
      ];
      return createResponse(instance);
    }
    if (url.includes("/api/v1/strategy-definitions"))
      return createResponse(definitions);
    if (logsMatch) {
      const instanceId = decodeURIComponent(logsMatch[1]);
      return createResponse({
        instanceId,
        logs: runtimeState.logsById[instanceId] ?? [],
      });
    }
    if (auditMatch) {
      const instanceId = decodeURIComponent(auditMatch[1]);
      return createResponse({
        instanceId,
        entries: runtimeState.auditById[instanceId] ?? [],
      });
    }
    if (url.includes("/api/v1/strategies")) return createResponse(runtimeState.strategies);

    throw new Error(`Unexpected request: ${url}`);
  });
}

function buildDslScript(
  name: string,
  body: string[] = ['log "close"'],
  options?: {
    version?: string;
    symbol?: string;
    interval?: string;
  },
) {
  const version = options?.version ?? "0.1.0";
  const symbol = options?.symbol ?? "00700";
  const interval = options?.interval ?? "1m";

  return [
    `strategy ${name}`,
    `version ${version}`,
    `symbol ${symbol}`,
    `interval ${interval}`,
    "",
    "on kline_close:",
    ...body.map((line) => `  ${line}`),
  ].join("\n");
}

function buildRuntimeAccount(overrides?: Partial<BrokerRuntimeResponse>): BrokerRuntimeResponse {
  return {
    ...emptyBrokerRuntime,
    ...overrides,
    accounts: overrides?.accounts ?? emptyBrokerRuntime.accounts,
  };
}

describe("Strategy page", () => {
  it("lists strategies and shows the selected strategy logs and audit", async () => {
    const strategies = [
      {
        id: "instance-1",
        definition: {
          strategyId: "s-mean-revert",
          name: "Mean Revert",
          version: "1.0.0",
        },
        binding: {
          symbols: ["HK.00700"],
          interval: "5m",
          executionMode: "live" as const,
        },
        params: {
          threshold: 10,
        },
        status: "RUNNING" as const,
        createdAt: "2026-05-16T00:00:00.000Z",
        logs: [],
      },
      {
        id: "instance-2",
        definition: {
          strategyId: "s-breakout",
          name: "Breakout",
          version: "1.0.0",
        },
        binding: {
          symbols: ["US.AAPL"],
          interval: "15m",
          executionMode: "notify_only" as const,
        },
        params: {
          window: 20,
        },
        status: "PAUSED" as const,
        createdAt: "2026-05-16T00:01:00.000Z",
        logs: [],
      },
    ];
    const systemStatus: SystemStatusResponse = {
      ...emptySystemStatus,
      defaultTradingEnvironment: "REAL",
      realTradingEnabled: true,
    };

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus,
        strategies,
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z started strategy s-mean-revert",
            "2026-05-16T00:00:02.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
          "instance-2": ["2026-05-16T00:01:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-mean-revert",
              at: "2026-05-16T00:00:00.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "tick",
              detail: "QUOTE_SNAPSHOT HK.00700",
              at: "2026-05-16T00:00:02.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              at: "2026-05-16T00:01:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");
    await waitForSelector(wrapper, '[data-testid="strategy-instance-1"]');

    expect(wrapper.text()).toContain("策略实例");
    expect(wrapper.text()).toContain("Mean Revert");
    expect(wrapper.text()).toContain("Breakout");
    expect(wrapper.text()).toContain("tick QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("运行审计");
    expect(wrapper.text()).toContain("QUOTE_SNAPSHOT HK.00700");
    expect(wrapper.text()).toContain("REAL");
    expect(wrapper.text()).toContain("仅通知");
    expect(wrapper.get('[data-testid="strategy-status-instance-1"]').classes()).toContain("strategy-status-badge--running");
    expect(wrapper.get('[data-testid="strategy-status-instance-2"]').classes()).toContain("strategy-status-badge--paused");

    wrapper.unmount();
  });

  it("shows activity tabs, importance filters, and params dialog", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-mean-revert",
              name: "Mean Revert",
              version: "1.0.0",
            },
            params: {
              window: 20,
              threshold: 1.8,
            },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": [
            "2026-05-16T00:00:00.000Z ERROR order rejected for HK.00700",
            "2026-05-16T00:00:02.000Z paused strategy execution",
            "2026-05-16T00:00:03.000Z tick QUOTE_SNAPSHOT HK.00700",
          ],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "failed",
              detail: "order rejected for HK.00700",
              at: "2026-05-16T00:00:05.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "paused",
              detail: "manual guardrail pause",
              at: "2026-05-16T00:00:06.000Z",
            },
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "runtime ready",
              at: "2026-05-16T00:00:07.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");
    await waitForSelector(wrapper, '[data-testid="strategy-instance-1"]');

    expect(wrapper.findAll('[data-testid="strategy-log-entry"]')).toHaveLength(3);

    await wrapper.get('[data-testid="strategy-activity-filter-error"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.findAll('[data-testid="strategy-log-entry"]')).toHaveLength(1);
    expect(wrapper.get('[data-testid="strategy-log-list"]').text()).toContain("ERROR order rejected for HK.00700");

    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.findAll('[data-testid="strategy-audit-entry"]')).toHaveLength(1);
    expect(wrapper.get('[data-testid="strategy-audit-list"]').text()).toContain("order rejected for HK.00700");

    await wrapper.get('[data-testid="strategy-open-params-dialog"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-params-dialog"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-params-dialog"]').text()).toContain('"window": 20');
    expect(wrapper.get('[data-testid="strategy-params-dialog"]').text()).toContain('"threshold": 1.8');

    wrapper.unmount();
  });

  it("creates, updates, and deletes a strategy instance with bindings", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        brokerRuntime: buildRuntimeAccount({
          descriptor: {
            ...emptyBrokerRuntime.descriptor,
            id: "futu",
          },
          accounts: [
            {
              accountId: "123456",
              tradingEnvironment: "SIMULATE",
              marketAuthorities: ["US"],
              securityFirm: "futu-securities",
            },
          ],
        }),
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );
    vi.stubGlobal("confirm", vi.fn().mockReturnValue(true));

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-current-binding-summary"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("运行控制");

    await openCreateInstancePanel(wrapper);

    await appendSymbolTags(wrapper, '[data-testid="strategy-instance-symbols"]', ["us:aapl", "hk:00700"]);
    await wrapper.get('[data-testid="strategy-instance-interval"]').setValue("15m");
    await wrapper.get('[data-testid="strategy-instance-execution-mode"]').setValue("notify_only");
    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("DSL Breakout");
    expect(wrapper.text()).toContain("US.AAPL, HK.00700");
    expect(wrapper.text()).toContain("15m");
    expect(wrapper.text()).toContain("仅通知");

    expect(wrapper.find('[data-testid="strategy-edit-instance-panel"]').exists()).toBe(false);
    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-edit-instance-panel"]').exists()).toBe(true);

    await appendSymbolTags(wrapper, '[data-testid="strategy-edit-symbols"]', ["us:msft"]);
    await wrapper.get('[data-testid="strategy-edit-interval"]').setValue("30m");
    await wrapper.get('[data-testid="strategy-edit-execution-mode"]').setValue("live");
    await wrapper.get('[data-testid="strategy-update-binding"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("US.MSFT");
    expect(wrapper.text()).toContain("30m");
    expect(wrapper.text()).toContain("已更新实例绑定");

    await wrapper.get('[data-testid="strategy-current-binding-summary"]').trigger("click");
    await settleStrategyWorkspace();

    await wrapper.get('[data-testid="strategy-delete-instance"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("暂无策略实例");

    wrapper.unmount();
  });

  it("shows the add menu and expands the instance composer only on demand", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("新增策略");
    expect(wrapper.text()).toContain("新增实例");

    await wrapper.get('[data-testid="strategy-new-instance"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-instance-dialog"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-create-instance-close"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("tokenizes pasted symbols into tags in the instance composer", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-breakout",
            name: "DSL Breakout",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            script: buildDslScript("DSL Breakout"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
        strategies: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");
    await openCreateInstancePanel(wrapper);

    await wrapper.get('[data-testid="strategy-instance-symbols"]').trigger("paste", {
      clipboardData: {
        getData: () => "us:tme\nhk:00700",
      },
    });
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("US.TME");
    expect(wrapper.text()).toContain("HK.00700");

    wrapper.unmount();
  });

  it("filters broker accounts in the searchable selector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
      brokerRuntime: buildRuntimeAccount({
        descriptor: {
          ...emptyBrokerRuntime.descriptor,
          id: "futu",
        },
        accounts: [
          {
            accountId: "123456",
            tradingEnvironment: "SIMULATE",
            marketAuthorities: ["US"],
            securityFirm: "futu-securities",
          },
          {
            accountId: "654321",
            tradingEnvironment: "REAL",
            marketAuthorities: ["HK"],
            securityFirm: "futu-securities",
          },
        ],
      }),
      definitions: [
        {
          id: "dsl-breakout",
          name: "DSL Breakout",
          version: "0.1.0",
          description: "dsl strategy",
          runtime: "dsl-go-plan",
          script: buildDslScript("DSL Breakout"),
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
      strategies: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    expect(currentConsoleDataStore).not.toBeNull();
    currentConsoleDataStore!.brokerSettings.value = {
      ...emptyBrokerSettings,
      accounts: [
        {
          id: "managed-1",
          brokerId: "futu",
          accountId: "123456",
          displayName: "模拟 US 123456",
          tradingEnvironment: "SIMULATE",
          market: "US",
          securityFirm: "futu-securities",
          enabled: true,
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
        {
          id: "managed-2",
          brokerId: "futu",
          accountId: "654321",
          displayName: "实盘 HK 654321",
          tradingEnvironment: "REAL",
          market: "HK",
          securityFirm: "futu-securities",
          enabled: true,
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
    };
    currentConsoleDataStore!.brokerRuntime.value = buildRuntimeAccount({
      descriptor: {
        ...emptyBrokerRuntime.descriptor,
        id: "futu",
      },
      accounts: [
        {
          accountId: "123456",
          tradingEnvironment: "SIMULATE",
          marketAuthorities: ["US"],
          securityFirm: "futu-securities",
        },
        {
          accountId: "654321",
          tradingEnvironment: "REAL",
          marketAuthorities: ["HK"],
          securityFirm: "futu-securities",
        },
      ],
    });
    await settleStrategyWorkspace();
    await openStrategyWorkspaceTab(wrapper, "runtime");
    await openCreateInstancePanel(wrapper);

    await wrapper.get('[data-testid="strategy-instance-account"]').trigger("click");
    await settleStrategyWorkspace();
    await wrapper.get('[data-testid="strategy-instance-account-search"]').setValue("654321");
    await settleStrategyWorkspace();

    expect(wrapper.find('[data-testid="strategy-instance-account-option-654321"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-instance-account-option-123456"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-instance-account-option-654321"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.get('[data-testid="strategy-instance-account"]').text()).toContain("654321");

    wrapper.unmount();
  });

  it("clears invalid symbols on blur and blocks instance creation", async () => {
    const fetchMock = buildFetchMock({
      definitions: [
        {
          id: "dsl-breakout",
          name: "DSL Breakout",
          version: "0.1.0",
          description: "dsl strategy",
          runtime: "dsl-go-plan",
          script: buildDslScript("DSL Breakout"),
          createdAt: "2026-05-23T00:00:00.000Z",
          updatedAt: "2026-05-23T00:00:00.000Z",
        },
      ],
      strategies: [],
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");
    await openCreateInstancePanel(wrapper);

    const symbolInput = wrapper.get('[data-testid="strategy-instance-symbols"]');
    await symbolInput.setValue("tme");
    await symbolInput.trigger("blur");
    await settleStrategyWorkspace();

    expect((wrapper.get('[data-testid="strategy-instance-symbols"]').element as HTMLInputElement).value).toBe("");
    expect(wrapper.get('[data-testid="strategy-instance-symbols-validation"]').text()).toContain("带市场前缀");

    await wrapper.get('[data-testid="strategy-create-instance"]').trigger("click");
    await settleStrategyWorkspace();

    expect(
      fetchMock.mock.calls.some(([input, init]) => (
        String(input).includes("/instantiate")
        && init?.method === "POST"
      )),
    ).toBe(false);
    expect(wrapper.text()).toContain("已忽略无效交易代码");

    wrapper.unmount();
  });

  it("switches selected strategy and refreshes logs and audit", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        systemStatus: {
          ...emptySystemStatus,
          realTradingKillSwitch: {
            ...emptySystemStatus.realTradingKillSwitch,
            active: true,
          },
        },
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
          },
          {
            id: "instance-2",
            definition: {
              strategyId: "s-beta",
              name: "Beta",
              version: "1.0.0",
            },
            params: { slow: 13 },
            status: "PAUSED",
            createdAt: "2026-05-16T00:02:00.000Z",
            logs: [],
          },
        ],
        logsById: {
          "instance-1": ["2026-05-16T00:00:00.000Z started strategy s-alpha"],
          "instance-2": ["2026-05-16T00:02:00.000Z paused strategy execution"],
        },
        auditById: {
          "instance-1": [
            {
              instanceId: "instance-1",
              kind: "started",
              detail: "s-alpha",
              at: "2026-05-16T00:00:00.000Z",
            },
          ],
          "instance-2": [
            {
              instanceId: "instance-2",
              kind: "paused",
              detail: "manual pause",
              at: "2026-05-16T00:02:10.000Z",
            },
          ],
        },
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    await wrapper.get('[data-testid="strategy-instance-2"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("paused strategy execution");

    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click");
    await settleStrategyWorkspace();

    expect(wrapper.text()).toContain("manual pause");
    expect(wrapper.text()).toContain("已启用");

    wrapper.unmount();
  });

  it("shows runtime observation details for the selected strategy instance", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        strategies: [
          {
            id: "instance-1",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
            runtimeObservation: {
              actualStatus: "RUNNING",
              activeSymbols: ["US.AAPL", "US.MSFT"],
              lastClosedKlineAt: "2026-05-16T00:03:00.000Z",
              lastSignalAt: "2026-05-16T00:03:05.000Z",
              lastOrderAt: "2026-05-16T00:03:06.000Z",
              lastErrorAt: "2026-05-16T00:02:59.000Z",
              lastError: "network glitch",
              updatedAt: "2026-05-16T00:03:06.000Z",
            },
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    await wrapper.get('[data-testid="strategy-instance-1"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-runtime-observation"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("实际运行态");
    expect(wrapper.text()).toContain("US.AAPL, US.MSFT");
    expect(wrapper.text()).toContain("2026-05-16 00:03:00Z");
    expect(wrapper.text()).toContain("network glitch");

    wrapper.unmount();
  });

  it("counts only actual running runtime observations in the runtime header", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        strategies: [
          {
            id: "instance-running",
            definition: {
              strategyId: "s-alpha",
              name: "Alpha",
              version: "1.0.0",
            },
            params: { fast: 5 },
            status: "RUNNING",
            createdAt: "2026-05-16T00:00:00.000Z",
            logs: [],
            runtimeObservation: {
              actualStatus: "RUNNING",
              activeSymbols: ["US.AAPL"],
            },
          },
          {
            id: "instance-stale",
            definition: {
              strategyId: "s-beta",
              name: "Beta",
              version: "1.0.0",
            },
            params: { slow: 13 },
            status: "PAUSED",
            createdAt: "2026-05-16T00:02:00.000Z",
            logs: [],
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");
    await openStrategyWorkspaceTab(wrapper, "runtime");

    expect(wrapper.text()).toContain("1 个活跃实例");
    expect(wrapper.text()).toContain("1 个运行中");

    wrapper.unmount();
  });

  it("shows the DSL strategy design workspace", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    expect(wrapper.text()).toContain("策略运行");
    expect(wrapper.find('[data-testid="strategy-script-editor"]').exists()).toBe(false);

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.text()).toContain("设计");
    expect(wrapper.text()).toContain("策略定义");
    expect(wrapper.text()).toContain("DSL Mean Revert");
    expect(wrapper.text()).toContain("dsl-go-plan");
    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-zoom-fit"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-builder"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("展开创建器");
    expect(wrapper.find('[data-testid="toggle-strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-block-inspector-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.findAll('.strategy-stage__toolbar-card')).toHaveLength(1);
    expect(wrapper.find('[data-testid="sync-visual-script"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="reset-visual-model"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-sync-status"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await showStrategyCodeEditor(wrapper, "code");

    expect(wrapper.text()).toContain("DSL 策略工作台");
    expect(wrapper.get('[data-testid="strategy-script-editor"]').element).toBeTruthy();
    expect(wrapper.html()).toContain("on kline_close:");

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    await openStrategyTemplatesPanel(wrapper);
    expect(wrapper.text()).toContain("双均线系统");
    expect(wrapper.text()).toContain("MACD 动能交易");
    expect(wrapper.text()).toContain("布林带回归交易");

    wrapper.unmount();
  });

  it("supports searching inside the builder while keeping the close control at the launcher position", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const toggle = wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]');
    const variablesToggle = wrapper.get('[data-testid="strategy-logic-flow-variables-toggle"]');
    expect(toggle.text()).toContain("展开创建器");
    expect(variablesToggle.text()).toContain("变量 0");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(false);

    await toggle.trigger("click");
    await flushRequests();

    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("关闭创建器");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(true);

    const initialLabels = wrapper.findAll('.strategy-logic-flow-builder__label').map((item) => item.text());
    expect(initialLabels).toContain("指标条件判断");
    expect(initialLabels).not.toContain("指标数据");
    expect(initialLabels).not.toContain("技术指标");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-search"]').setValue("通知");
    await flushRequests();

    const filteredLabels = wrapper.findAll('.strategy-logic-flow-builder__label').map((item) => item.text());
    expect(filteredLabels).toContain("发送通知");
    expect(filteredLabels).not.toContain("输出日志");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-search"]').setValue("不存在的图块");
    await flushRequests();

    expect(wrapper.text()).toContain("没有匹配的图块");

    await wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').trigger("click");
    await flushRequests();

    expect(wrapper.get('[data-testid="strategy-logic-flow-builder-toggle"]').text()).toContain("展开创建器");
    expect(wrapper.find('.strategy-logic-flow-builder__grid').exists()).toBe(false);

    wrapper.unmount();
  });

  it("collapses the strategy definitions sidebar to free editing space", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(true);

    await wrapper.get('[data-testid="toggle-strategy-definitions"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-definitions-floating"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definition-dsl-mean-revert"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("switches the workspace display modes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-logic-flow-zoom"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-script-editor"]').element).toBeTruthy();

    await wrapper.get('[data-testid="strategy-display-mode-canvas"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("opens and closes floating strategy editor panels from the toolbar", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-mean-revert",
            name: "DSL Mean Revert",
            version: "0.1.0",
            description: "dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Mean Revert"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-visual-builder-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="toggle-strategy-block-inspector-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click");
    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-basic-info-section"]').text()).toContain("元信息");

    await wrapper.get('[data-testid="strategy-display-mode-canvas"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper.get('[data-testid="strategy-display-mode-code"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(false);

    await wrapper.get('[data-testid="strategy-display-mode-split"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("auto opens block details when selecting a visual node and hides them when selection clears", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript(
              "RSI Visual",
              [
                "let rsi_calc_node = rsi(14)",
                "if rsi_calc_node < 30:",
                '  notify "RSI changed"',
              ],
              { version: "0.2.0" },
            ),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "rsi-calc-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "RSI 14 < 30",
                  properties: {
                    blockKind: "technicalIndicator",
                    indicatorType: "rsi",
                    conditionMode: "numeric",
                    operator: "<",
                    threshold: 30,
                    period: 14,
                  },
                },
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-1",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "rsi-calc-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(false);

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "rsi-calc-node");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("图块详情");
    expect(wrapper.text()).toContain("RSI");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", null);
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-block-inspector-card"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("shows getter variable naming and condition input selectors in the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-indicator-bindings",
            name: "DSL Indicator Bindings",
            version: "0.2.0",
            description: "indicator binding inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Indicator Bindings", ['log "seed"'], { version: "0.2.0" }),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "fast-ma",
                  type: "rect",
                  x: 380,
                  y: 150,
                  text: "获取 双均线 EMA 5",
                  properties: {
                    blockKind: "getTechnicalIndicator",
                    indicatorType: "movingAverage",
                    movingAverageType: "EMA",
                    windowSize: 5,
                    variableName: "EMA5",
                  },
                },
                {
                  id: "slow-ma",
                  type: "rect",
                  x: 380,
                  y: 260,
                  text: "获取 双均线 EMA 20",
                  properties: {
                    blockKind: "getTechnicalIndicator",
                    indicatorType: "movingAverage",
                    movingAverageType: "EMA",
                    windowSize: 20,
                    variableName: "EMA20",
                  },
                },
                {
                  id: "ma-condition",
                  type: "diamond",
                  x: 640,
                  y: 205,
                  text: "双均线金叉",
                  properties: {
                    blockKind: "technicalIndicatorCondition",
                    indicatorType: "movingAverage",
                    conditionMode: "pattern",
                    patternType: "goldenCross",
                    inputFastNodeId: "fast-ma",
                    inputSlowNodeId: "slow-ma",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-fast",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "fast-ma",
                },
                {
                  id: "edge-fast-slow",
                  type: "polyline",
                  sourceNodeId: "fast-ma",
                  targetNodeId: "slow-ma",
                },
                {
                  id: "edge-slow-condition",
                  type: "polyline",
                  sourceNodeId: "slow-ma",
                  targetNodeId: "ma-condition",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const variablesToggle = wrapper.get('[data-testid="strategy-logic-flow-variables-toggle"]');
    expect(variablesToggle.text()).toContain("变量 2");

    await variablesToggle.trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-variables"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("EMA5");
    expect(wrapper.text()).toContain("EMA20");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "fast-ma");
    await flushRequests();

    const variableNameInput = wrapper.get('[data-testid="strategy-block-variable-name-input"]');
    expect(variableNameInput.element.getAttribute("placeholder")).toBe("EMA5");
    expect((variableNameInput.element as HTMLInputElement).value).toBe("EMA5");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "ma-condition");
    await flushRequests();

    expect((wrapper.get('[data-testid="strategy-block-indicator-input-fast-select"]').element as HTMLSelectElement).value).toBe("fast-ma");
    expect((wrapper.get('[data-testid="strategy-block-indicator-input-slow-select"]').element as HTMLSelectElement).value).toBe("slow-ma");
    expect(wrapper.text()).toContain("EMA5 · 获取 均线 EMA 5日");
    expect(wrapper.text()).toContain("EMA20 · 获取 均线 EMA 20日");

    wrapper.unmount();
  });

  it("shows entry position policy only for opening order sides in the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-place-order-policy",
            name: "DSL Place Order Policy",
            version: "0.2.0",
            description: "place order policy inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Place Order Policy", ['log "seed"'], { version: "0.2.0" }),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "buy-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "下单",
                  properties: {
                    blockKind: "placeOrder",
                    side: "BUY",
                    orderType: "MARKET",
                    quantityMode: "shares",
                    quantityValue: 100,
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-buy",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "buy-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "buy-node");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("账户仓位百分比");
    expect(wrapper.text()).toContain("当前标的仓位百分比");
    expect(wrapper.text()).toContain("融资可用百分比");
    expect(wrapper.text()).toContain("融券可用百分比");

    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(false);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);

    await wrapper.find('[data-testid="strategy-place-order-side"]').setValue("SELL_SHORT");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(true);
    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(false);

    await wrapper.find('[data-testid="strategy-place-order-side"]').setValue("SELL");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-place-order-entry-position-policy"]').exists()).toBe(false);
    expect((wrapper.get('option[value="marginBuyingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);
    expect((wrapper.get('option[value="shortSellingPowerPercent"]').element as HTMLOptionElement).disabled).toBe(true);

    wrapper.unmount();
  });

  it("disconnects selected flow edges by menu action and keyboard delete", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-edge-disconnect",
            name: "DSL Edge Disconnect",
            version: "0.2.0",
            description: "edge disconnect inspector",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Edge Disconnect", ['log "seed"'], { version: "0.2.0" }),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: { blockKind: "onKLineClosed" },
                },
                {
                  id: "notify-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "发送通知",
                  properties: { blockKind: "notify", message: "edge one" },
                },
                {
                  id: "log-node",
                  type: "rect",
                  x: 700,
                  y: 200,
                  text: "输出日志",
                  properties: { blockKind: "log", message: "edge two" },
                },
              ],
              edges: [
                {
                  id: "edge-root-notify",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "notify-node",
                },
                {
                  id: "edge-notify-log",
                  type: "polyline",
                  sourceNodeId: "notify-node",
                  targetNodeId: "log-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    const designerVm = designer.vm as unknown as {
      selectEdgeById: (edgeId: string | null) => void;
    };

    designerVm.selectEdgeById("edge-root-notify");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-logic-flow-edge-menu"]').exists()).toBe(true);
    expect(
      wrapper.get('[data-testid="strategy-logic-flow-canvas"]').element.contains(
        wrapper.get('[data-testid="strategy-logic-flow-edge-menu"]').element,
      ),
    ).toBe(false);

    await wrapper.get('[data-testid="strategy-logic-flow-edge-disconnect"]').trigger("click");
    await flushRequests();

    let visualModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    expect(visualModel.edges.map((edge) => edge.id)).toEqual(["edge-notify-log"]);

    designerVm.selectEdgeById("edge-notify-log");
    await flushRequests();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Delete" }));
    await flushRequests();

    visualModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    expect(visualModel.edges).toHaveLength(0);
    expect(wrapper.find('[data-testid="strategy-logic-flow-edge-menu"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("auto syncs a saved logic flow model back into DSL", async () => {
    const visualModel = {
      engine: "logic-flow" as const,
      version: 1,
      nodes: [
        {
          id: "on-kline-root",
          type: "circle",
          x: 160,
          y: 200,
          text: "K 线收盘",
          properties: {
            blockKind: "onKLineClosed",
          },
        },
        {
          id: "notify-node",
          type: "rect",
          x: 380,
          y: 200,
          text: "发送通知",
          properties: {
            blockKind: "notify",
            message: "收盘价触发视觉策略",
          },
        },
      ],
      edges: [
        {
          id: "edge-1",
          type: "polyline",
          sourceNodeId: "on-kline-root",
          targetNodeId: "notify-node",
        },
      ],
    };

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-logic-flow",
            name: "DSL Logic Flow",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Logic Flow", ['log "seed"'], { version: "0.2.0" }),
            visualModel,
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("select-node", "rsi-calc-node");
    await flushRequests();

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("strategy DSL Logic Flow");
    expect(scriptEditor.value).not.toContain("manualOnly");

    wrapper.findComponent(StrategyLogicFlowDesigner).vm.$emit("update:modelValue", visualModel);
    await flushRequests();

    expect(scriptEditor.value).toContain("on kline_close:");
    expect(scriptEditor.value).toContain('notify "收盘价触发视觉策略"');

    wrapper.unmount();
  });

  it("syncs handwritten DSL back into flow on blur", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-handwritten",
            name: "DSL Handwritten",
            version: "0.2.0",
            description: "code-first strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("DSL Handwritten", ['notify "close seed"'], { version: "0.2.0" }),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const nextScript = [
      "strategy DSL Handwritten",
      "version 0.2.0",
      "on kline_close:",
      "  notify \"close signal\"",
      "  let rsi14 = rsi(14)",
      "  if rsi14 < 30:",
      "    buy shares 100 policy same_direction type MARKET",
    ].join("\n");

    await wrapper.get('[data-testid="strategy-script-editor"]').setValue(nextScript);
    await wrapper.get('[data-testid="strategy-script-editor"]').trigger("blur");
    await flushRequests();

    const visualModel = wrapper.findComponent(StrategyLogicFlowDesigner)
      .props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;

    expect(visualModel.nodes.some((node) => node.properties.blockKind === "notify")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "getTechnicalIndicator")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "technicalIndicatorCondition")).toBe(true);
    expect(visualModel.nodes.some((node) => node.properties.blockKind === "placeOrder")).toBe(true);
    expect(wrapper.get('[data-testid="strategy-visual-sync-status"]').text()).toContain("DSL 已同步");

    wrapper.unmount();
  });

  it("rewrites DSL when a visual block parameter changes", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-rsi-visual",
            name: "RSI Visual",
            version: "0.2.0",
            description: "visual strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript(
              "RSI Visual",
              [
                "let rsi_calc_node = rsi(14)",
                "if rsi_calc_node < 30:",
                '  notify "RSI changed"',
              ],
              { version: "0.2.0" },
            ),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "rsi-calc-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "RSI 14 < 30",
                  properties: {
                    blockKind: "technicalIndicator",
                    indicatorType: "rsi",
                    conditionMode: "numeric",
                    operator: "<",
                    threshold: 30,
                    period: 14,
                  },
                },
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
                {
                  id: "notify-node",
                  type: "rect",
                  x: 700,
                  y: 200,
                  text: "发送通知",
                  properties: {
                    blockKind: "notify",
                    message: "RSI changed",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-1",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "rsi-calc-node",
                },
                {
                  id: "edge-2",
                  type: "polyline",
                  sourceNodeId: "rsi-calc-node",
                  targetNodeId: "notify-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("let rsi_calc_node = rsi(14)");

    const visualModel = JSON.parse(
      JSON.stringify(
        designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>,
      ),
    ) as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    const indicatorNode = visualModel.nodes.find((node) => node.id === "rsi-calc-node");
    expect(indicatorNode).toBeDefined();
    if (indicatorNode === undefined) {
      return;
    }

    indicatorNode.properties = {
      ...indicatorNode.properties,
      period: 21,
    };
    designer.vm.$emit("update:modelValue", visualModel);
    await flushRequests();

    expect(scriptEditor.value).toContain("let rsi_calc_node = rsi(21)");

    wrapper.unmount();
  });

  it("rewrites risk block mode and window policy from the block inspector", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-risk-inspector",
            name: "Risk Inspector",
            version: "0.2.0",
            description: "risk block inspector",
            runtime: "dsl-go-plan",
            symbol: "US.AAPL",
            interval: "5m",
            script: buildDslScript("Risk Inspector", ['log "seed"'], { version: "0.2.0", symbol: "US.AAPL", interval: "5m" }),
            visualModel: {
              engine: "logic-flow",
              version: 1,
              nodes: [
                {
                  id: "on-kline-root",
                  type: "circle",
                  x: 160,
                  y: 200,
                  text: "K 线收盘",
                  properties: {
                    blockKind: "onKLineClosed",
                  },
                },
                {
                  id: "risk-node",
                  type: "rect",
                  x: 420,
                  y: 200,
                  text: "自动止损 1日 2%",
                  properties: {
                    blockKind: "stopLoss",
                    mode: "stopLoss",
                    direction: "auto",
                    timeValue: 1,
                    timeUnit: "day",
                    percentage: 2,
                    windowPolicy: "continuous",
                  },
                },
              ],
              edges: [
                {
                  id: "edge-root-risk",
                  type: "polyline",
                  sourceNodeId: "on-kline-root",
                  targetNodeId: "risk-node",
                },
              ],
            },
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);
    await showStrategyCodeEditor(wrapper, "split");

    const designer = wrapper.findComponent(StrategyLogicFlowDesigner);
    designer.vm.$emit("select-node", "risk-node");
    await flushRequests();

    expect((wrapper.get('[data-testid="strategy-stop-loss-mode"]').element as HTMLSelectElement).value).toBe("stopLoss");
    expect((wrapper.get('[data-testid="strategy-stop-loss-window-policy"]').element as HTMLSelectElement).value).toBe("continuous");

    await wrapper.get('[data-testid="strategy-stop-loss-mode"]').setValue("trailingStop");
    await flushRequests();

    await wrapper.get('[data-testid="strategy-stop-loss-window-policy"]').setValue("session");
    await flushRequests();

    const updatedModel = designer.props("modelValue") as NonNullable<StrategyDefinitionDocument["visualModel"]>;
    const riskNode = updatedModel.nodes.find((node) => node.id === "risk-node");
    expect(riskNode).toBeDefined();
    expect(riskNode?.text).toBe("自动追踪止损 1日 2% 时段感知");
    expect(riskNode?.properties.mode).toBe("trailingStop");
    expect(riskNode?.properties.windowPolicy).toBe("session");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(scriptEditor.value).toContain("protect auto trailingStop 1 day 2 window session");

    wrapper.unmount();
  });

  it("creates a new draft from the double moving average template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openNewStrategyFromRuntime(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);

    await showStrategyCodeEditor(wrapper, "split");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("已基于「双均线系统」创建新草稿");
    expect(scriptEditor.value).toContain("let dma_fast_ma = ma(MA, 5, day)");
    expect(scriptEditor.value).toContain("let dma_slow_ma = ma(MA, 20, day)");
    expect(scriptEditor.value).toContain("if cross_over(dma_fast_ma, dma_slow_ma):");
    expect(scriptEditor.value).toContain("金叉");

    wrapper.unmount();
  });

  it("allows dismissing strategy notices and errors", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openNewStrategyFromRuntime(wrapper);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("已基于「双均线系统」创建新草稿");

    await wrapper.get('[data-testid="dismiss-strategy-notice-banner"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).not.toContain("已基于「双均线系统」创建新草稿");
    expect(wrapper.find('[data-testid="dismiss-strategy-notice-banner"]').exists()).toBe(false);

    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    await wrapper
      .get('[data-testid="strategy-basic-info-section"] input[placeholder="例如：双均线观察策略"]')
      .setValue("");
    await wrapper.get('[data-testid="save-strategy-definition"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("策略名称不能为空。");

    await wrapper.get('[data-testid="dismiss-strategy-error-banner"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).not.toContain("策略名称不能为空。");
    expect(wrapper.find('[data-testid="dismiss-strategy-error-banner"]').exists()).toBe(false);

    wrapper.unmount();
  });

  it("prompts before leaving the editor for runtime when there are unsaved changes", async () => {
    const confirmMock = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { wrapper } = await mountStrategyPage("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    await wrapper.get('[data-testid="strategy-workspace-tab-runtime"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("双均线系统");

    await wrapper.get('[data-testid="strategy-workspace-tab-runtime"]').trigger("click");
    await flushRequests();

    expect(wrapper.text()).toContain("策略实例");
    expect(confirmMock).toHaveBeenCalledTimes(4);

    wrapper.unmount();
  });

  it("prompts before route-leaving the editor when there are unsaved changes", async () => {
    const confirmMock = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true);

    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );
    vi.stubGlobal("confirm", confirmMock);

    const { router, wrapper } = await mountStrategyPage("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    await router.push("/overview");
    await flushRequests();

    expect(router.currentRoute.value.path).toBe("/overview");
    expect(confirmMock).toHaveBeenCalledTimes(2);

    wrapper.unmount();
  });

  it("shows only templates when starting a new strategy and hides them after selection", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [
          {
            id: "dsl-existing",
            name: "Existing Strategy",
            version: "0.1.0",
            description: "existing dsl strategy",
            runtime: "dsl-go-plan",
            symbol: "00700",
            interval: "1m",
            script: buildDslScript("Existing Strategy"),
            createdAt: "2026-05-23T00:00:00.000Z",
            updatedAt: "2026-05-23T00:00:00.000Z",
          },
        ],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openStrategyDesignWorkspace(wrapper);

    await wrapper.get('[data-testid="toggle-strategy-basic-info-section"]').trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="strategy-basic-info-section"]').text()).toContain("元信息");
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await openStrategyWorkspaceTab(wrapper, "runtime");
    await openNewStrategyFromRuntime(wrapper);

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="strategy-definitions-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-basic-info-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-metadata-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    await wrapper
      .get('[data-testid="strategy-template-double-moving-average"]')
      .trigger("click");
    await flushRequests();

    expect(wrapper.find('[data-testid="strategy-templates-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-code-editor-section"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="strategy-visual-builder-section"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it("creates a new draft from the rsi reversion template", async () => {
    vi.stubGlobal(
      "fetch",
      buildFetchMock({
        definitions: [],
      }),
    );
    vi.stubGlobal(
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountStrategyPage("/strategy");

    await openNewStrategyFromRuntime(wrapper);
    await openStrategyTemplatesPanel(wrapper);
    await wrapper
      .get('[data-testid="strategy-template-rsi-reversion"]')
      .trigger("click");
    await flushRequests();

    await showStrategyCodeEditor(wrapper, "code");

    const scriptEditor = wrapper.get('[data-testid="strategy-script-editor"]')
      .element as HTMLTextAreaElement;
    expect(wrapper.text()).toContain("已基于「RSI 反转交易」创建新草稿");
    expect(scriptEditor.value).toContain("let rsi_getter = rsi(14)");
    expect(scriptEditor.value).toContain("if rsi_getter < 30:");
    expect(scriptEditor.value).toContain("if rsi_getter > 70:");

    wrapper.unmount();
  });

});
