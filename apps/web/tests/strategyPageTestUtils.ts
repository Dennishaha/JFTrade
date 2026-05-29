import { mount } from "@vue/test-utils"
import { createPinia } from "pinia"
import { defineComponent, h, nextTick } from "vue"
import { createMemoryHistory, createRouter, RouterView } from "vue-router"
import { vi } from "vitest"

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
} from "@jftrade/ui-contracts"
import type {
  BrokerRuntimeResponse,
  StrategyDefinitionDocument,
  SystemStatusResponse,
} from "@jftrade/ui-contracts"
import { provideConsoleDataStore } from "../src/composables/useConsoleData"
import { provideThemeStore } from "../src/composables/useTheme"
import { provideUIColorPreferencesStore } from "../src/composables/useUIColorPreferences"
import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout"
import StrategyPage from "../src/pages/StrategyPage.vue"

import {
  MockEventSource,
  createResponse,
  dialogStub,
  flushRequests,
} from "./helpers"

let currentStrategySystemStatus: SystemStatusResponse = emptySystemStatus
let currentConsoleDataStore: ReturnType<typeof provideConsoleDataStore> | null = null

const StrategyPageTestRoot = defineComponent({
  setup() {
    const themeStore = provideThemeStore()
    provideUIColorPreferencesStore(themeStore.theme)
    const workspaceLayout = provideWorkspaceLayoutStore()
    const consoleData = provideConsoleDataStore(workspaceLayout)
    currentConsoleDataStore = consoleData
    consoleData.systemStatus.value = currentStrategySystemStatus
    return () => h(RouterView)
  },
})

const OverviewTestPage = defineComponent({
  setup() {
    return () => h("div", "Overview")
  },
})

export async function mountStrategyPage(path = "/strategy") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/strategy", component: StrategyPage as never },
      { path: "/overview", component: OverviewTestPage as never },
    ],
  })
  await router.push(path)
  await router.isReady()

  const wrapper = mount(StrategyPageTestRoot, {
    global: {
      plugins: [createPinia(), router],
      stubs: {
        "v-dialog": dialogStub,
      },
    },
  })

  await flushRequests()

  return { router, wrapper }
}

export type StrategyPageWrapper = Awaited<ReturnType<typeof mountStrategyPage>>["wrapper"]

export function resetStrategyPageTestState() {
  MockEventSource.instances = []
  currentStrategySystemStatus = emptySystemStatus
  currentConsoleDataStore = null
}

export function getCurrentConsoleDataStore() {
  return currentConsoleDataStore
}

export async function settleStrategyWorkspace() {
  await flushRequests()
  await flushRequests()
  await nextTick()
  await nextTick()
}

function hasDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  return (
    wrapper.find('[data-testid="instantiate-strategy-definition"]').exists()
    || wrapper.find('[data-testid="toggle-strategy-templates-section"]').exists()
    || wrapper.find('[data-testid="strategy-templates-section"]').exists()
  )
}

async function ensureStrategyDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (hasDesignWorkspace(wrapper)) {
      return
    }
    if (!wrapper.find('[data-testid="strategy-workspace-tab-design"]').exists()) {
      return
    }
    await wrapper.get('[data-testid="strategy-workspace-tab-design"]').trigger("click")
    await settleStrategyWorkspace()
  }
}

export async function openStrategyWorkspaceTab(
  wrapper: StrategyPageWrapper,
  tab: "runtime" | "design",
) {
  await wrapper
    .get(`[data-testid="strategy-workspace-tab-${tab}"]`)
    .trigger("click")
  await settleStrategyWorkspace()
  if (tab === "design") {
    await ensureStrategyDesignWorkspace(wrapper)
  }
}

export async function openStrategyDesignWorkspace(
  wrapper: StrategyPageWrapper,
) {
  await openStrategyWorkspaceTab(wrapper, "design")
}

export async function showStrategyCodeEditor(
  wrapper: StrategyPageWrapper,
  mode: "split" | "code" = "code",
) {
  await wrapper
    .get(`[data-testid="strategy-display-mode-${mode}"]`)
    .trigger("click")
  await settleStrategyWorkspace()
}

export async function openNewStrategyFromRuntime(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click")
    await settleStrategyWorkspace()
    await wrapper.get('[data-testid="strategy-new-definition"]').trigger("click")
    await settleStrategyWorkspace()
    if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
      return
    }
  }
}

export async function openCreateInstancePanel(
  wrapper: StrategyPageWrapper,
) {
  for (let attempt = 0; attempt < 4; attempt += 1) {
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return
    }
    await wrapper.get('[data-testid="strategy-create-menu-toggle"]').trigger("click")
    await settleStrategyWorkspace()
    await wrapper.get('[data-testid="strategy-new-instance"]').trigger("click")
    await settleStrategyWorkspace()
    if (wrapper.find('[data-testid="strategy-create-instance-panel"]').exists()) {
      return
    }
  }
}

export async function appendSymbolTags(
  wrapper: StrategyPageWrapper,
  selector: string,
  symbols: string[],
) {
  for (const symbol of symbols) {
    const input = wrapper.get(selector)
    await input.setValue(symbol)
    await input.trigger("keydown", { key: "Enter" })
    await settleStrategyWorkspace()
  }
}

export async function appendInstrumentTags(
  wrapper: StrategyPageWrapper,
  selectors: {
    market: string
    code: string
  },
  instruments: Array<{
    market: string
    code: string
  }>,
) {
  for (const instrument of instruments) {
    await wrapper.get(selectors.market).setValue(instrument.market)
    await wrapper.get(selectors.code).setValue(instrument.code)
    await wrapper.get(selectors.code).trigger("keydown", { key: "Enter" })
    await settleStrategyWorkspace()
  }
}

export async function openStrategyTemplatesPanel(
  wrapper: StrategyPageWrapper,
) {
  await ensureStrategyDesignWorkspace(wrapper)
  if (wrapper.find('[data-testid="strategy-templates-section"]').exists()) {
    return
  }
  await wrapper.get('[data-testid="toggle-strategy-templates-section"]').trigger("click")
  await settleStrategyWorkspace()
}

export async function waitForSelector(
  wrapper: StrategyPageWrapper,
  selector: string,
  attempts = 40,
) {
  for (let index = 0; index < attempts; index += 1) {
    if (wrapper.find(selector).exists()) {
      return
    }
    await settleStrategyWorkspace()
  }
}

type BuildFetchMockOptions = {
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
      instruments?: {
        market: string;
        code: string;
      }[];
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
}

export function buildFetchMock(options: BuildFetchMockOptions) {
  const systemStatus = options.systemStatus ?? emptySystemStatus
  const brokerRuntime = options.brokerRuntime ?? emptyBrokerRuntime
  currentStrategySystemStatus = systemStatus
  const definitions = options.definitions ?? []
  const strategies = options.strategies ?? []
  const logsById = options.logsById ?? {}
  const auditById = options.auditById ?? {}
  const mutationTimestamp = "2026-05-23T00:05:00.000Z"

  function nextPatchVersion(version: string) {
    const match = version.trim().match(/^(\d+)\.(\d+)\.(\d+)$/)
    if (!match) {
      return "0.1.1"
    }
    return `${match[1]}.${match[2]}.${Number.parseInt(match[3], 10) + 1}`
  }

  function syncDslVersion(script: string, version: string) {
    if (script.trim() === "") {
      return script
    }
    if (/^version\s+/m.test(script)) {
      return script.replace(/^version\s+.*$/m, `version ${version}`)
    }
    const lines = script.split("\n")
    if (lines.length <= 1) {
      return `${script}\nversion ${version}`
    }
    lines.splice(1, 0, `version ${version}`)
    return lines.join("\n")
  }

  function cloneDefinition(definition: StrategyDefinitionDocument): StrategyDefinitionDocument {
    return {
      ...definition,
      runtime: definition.runtime ?? "dsl-go-plan",
      sourceFormat: definition.sourceFormat ?? "dsl-v1",
      visualModel: definition.visualModel ?? null,
    }
  }

  function createErrorResponse(message: string, status = 400): Response {
    return {
      ok: false,
      status,
      json: async () => ({
        ok: false,
        error: {
          code: "BAD_REQUEST",
          message,
        },
        timestamp: mutationTimestamp,
      }),
    } as Response
  }

  function normalizeInstrumentId(value: unknown) {
    const normalized = typeof value === "string" ? value.trim().toUpperCase() : ""
    if (normalized === "") {
      return ""
    }
    if (normalized.includes(":")) {
      const [market, symbol] = normalized.split(":", 2)
      if ((market ?? "") !== "" && (symbol ?? "") !== "") {
        return `${market}.${symbol}`
      }
    }
    return normalized
  }

  function normalizeBindingInstrument(value: unknown) {
    if (value == null || typeof value !== "object" || Array.isArray(value)) {
      return null
    }
    const record = value as Record<string, unknown>
    const market = String(record.market ?? "").trim().toUpperCase()
    const code = String(record.code ?? "").trim().toUpperCase()
    const instrumentId = normalizeInstrumentId(`${market}.${code}`)
    if (instrumentId === "" || !instrumentId.includes(".")) {
      return null
    }
    const [resolvedMarket, resolvedCode] = instrumentId.split(".", 2)
    if ((resolvedMarket ?? "") === "" || (resolvedCode ?? "") === "") {
      return null
    }
    return {
      market: resolvedMarket,
      code: resolvedCode,
    }
  }

  function normalizeBinding(
    rawBinding: unknown,
    params: Record<string, unknown>,
  ) {
    const bindingRecord = rawBinding && typeof rawBinding === "object" && !Array.isArray(rawBinding)
      ? rawBinding as Record<string, unknown>
      : {}
    const rawInstruments = Array.isArray(bindingRecord.instruments)
      ? bindingRecord.instruments
      : Array.isArray(params.instruments)
        ? params.instruments
        : []
    const instruments = Array.from(new Map(
      rawInstruments
        .map((value) => normalizeBindingInstrument(value))
        .filter((value): value is NonNullable<ReturnType<typeof normalizeBindingInstrument>> => value !== null)
        .map((value) => [`${value.market}.${value.code}`, value] as const),
    ).values())
    const rawSymbols = Array.isArray(bindingRecord.symbols)
      ? bindingRecord.symbols
      : Array.isArray(params.symbols)
        ? params.symbols
        : typeof params.symbol === "string" && params.symbol.trim() !== ""
          ? [params.symbol]
          : []
    const symbols = instruments.length > 0
      ? instruments.map((value) => `${value.market}.${value.code}`)
      : Array.from(new Set(
          rawSymbols
            .map((value) => normalizeInstrumentId(value))
            .filter((value) => value !== ""),
        ))
    const interval = typeof bindingRecord.interval === "string" && bindingRecord.interval.trim() !== ""
      ? bindingRecord.interval.trim()
      : typeof params.interval === "string" && params.interval.trim() !== ""
        ? params.interval.trim()
        : "5m"
    const executionMode = bindingRecord.executionMode === "notify_only"
      ? "notify_only"
      : bindingRecord.executionMode === "live"
        ? "live"
        : params.executionMode === "notify_only"
          ? "notify_only"
          : "live"
    const brokerAccountRecord = bindingRecord.brokerAccount && typeof bindingRecord.brokerAccount === "object" && !Array.isArray(bindingRecord.brokerAccount)
      ? bindingRecord.brokerAccount as Record<string, unknown>
      : params.brokerAccount && typeof params.brokerAccount === "object" && !Array.isArray(params.brokerAccount)
        ? params.brokerAccount as Record<string, unknown>
        : null
    const brokerAccount = brokerAccountRecord === null
      ? null
      : {
          brokerId: String(brokerAccountRecord.brokerId ?? "").trim().toLowerCase(),
          accountId: String(brokerAccountRecord.accountId ?? "").trim(),
          tradingEnvironment: String(brokerAccountRecord.tradingEnvironment ?? "").trim().toUpperCase(),
          market: String(brokerAccountRecord.market ?? "").trim().toUpperCase(),
        }

    return {
      instruments,
      symbols,
      interval,
      executionMode,
      brokerAccount,
    } as const
  }

  async function readJsonBody(init?: RequestInit, request?: Request | null) {
    if (request != null) {
      const text = await request.clone().text()
      return text === "" ? {} : JSON.parse(text)
    }
    if (typeof init?.body === "string") {
      return init.body === "" ? {} : JSON.parse(init.body)
    }
    return {}
  }

  const runtimeDefinitions = definitions.map((definition) => cloneDefinition(definition))

  function strategyUsesDefinition(
    strategy: {
      definition: { strategyId: string };
      params: Record<string, unknown>;
    },
    definitionId: string,
  ) {
    if (strategy.definition.strategyId === definitionId) {
      return true
    }
    return typeof strategy.params.definitionId === "string"
      && strategy.params.definitionId.trim() === definitionId
  }

  function buildDefinitionSync(strategy: {
    definition: { strategyId: string; version: string };
    params: Record<string, unknown>;
    status: "RUNNING" | "PAUSED" | "STOPPED";
  }) {
    const definitionId = strategy.definition.strategyId || String(strategy.params.definitionId ?? "").trim()
    const definition = runtimeState.definitions.find((item) => item.id === definitionId)
    if (definition === undefined) {
      return null
    }
    const appliedVersion = strategy.definition.version
    const latestVersion = definition.version
    const isLatest = appliedVersion === latestVersion
    return {
      definitionId,
      appliedVersion,
      latestVersion,
      isLatest,
      canApplyLatest: !isLatest && strategy.status === "STOPPED",
      blockedReason: !isLatest && strategy.status !== "STOPPED"
        ? "当前实例不是 STOPPED，先停止后才能刷新到最新策略。"
        : null,
    }
  }

  function applyDefinitionSnapshot(
    strategy: {
      definition: { strategyId: string; name: string; version: string };
      runtime?: string;
      sourceFormat?: "dsl-v1";
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
    },
    definition: StrategyDefinitionDocument,
  ) {
    const binding = normalizeBinding(strategy.binding, strategy.params)
    strategy.definition = {
      strategyId: definition.id,
      name: definition.name,
      version: definition.version,
    }
    strategy.runtime = definition.runtime ?? "dsl-go-plan"
    strategy.sourceFormat = definition.sourceFormat ?? "dsl-v1"
    strategy.params = {
      ...strategy.params,
      runtime: strategy.runtime,
      sourceFormat: strategy.sourceFormat,
      definitionId: definition.id,
      instruments: binding.instruments.map((instrument) => ({ ...instrument })),
      symbols: [...binding.symbols],
      symbol: binding.symbols[0] ?? "",
      interval: binding.interval,
      executionMode: binding.executionMode,
      script: definition.script,
      compiledAt: definition.updatedAt,
      ...(binding.brokerAccount === null
        ? {}
        : {
            brokerAccount: { ...binding.brokerAccount },
          }),
    }
  }

  function serializeStrategy<T extends {
    definition: { strategyId: string; name: string; version: string };
    binding?: {
      instruments?: {
        market: string;
        code: string;
      }[];
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
    status: "RUNNING" | "PAUSED" | "STOPPED";
  }>(strategy: T) {
    return {
      ...strategy,
      definition: { ...strategy.definition },
      binding: strategy.binding == null
        ? strategy.binding
        : {
            ...strategy.binding,
            instruments: strategy.binding.instruments == null
              ? strategy.binding.instruments
              : strategy.binding.instruments.map((instrument) => ({ ...instrument })),
            symbols: [...(strategy.binding.symbols ?? [])],
            brokerAccount: strategy.binding.brokerAccount == null
              ? strategy.binding.brokerAccount
              : { ...strategy.binding.brokerAccount },
          },
      params: {
        ...strategy.params,
      },
      logs: [...strategy.logs],
      runtimeObservation: strategy.runtimeObservation == null
        ? strategy.runtimeObservation
        : {
            ...strategy.runtimeObservation,
            activeSymbols: [...strategy.runtimeObservation.activeSymbols],
          },
      definitionSync: buildDefinitionSync(strategy),
    }
  }

  const runtimeState = {
    definitions: runtimeDefinitions,
    strategies: strategies.map((strategy) => ({
      ...(() => {
        const runtime = strategy.runtime ?? "dsl-go-plan"
        const sourceFormat = strategy.sourceFormat ?? "dsl-v1"
        const startable =
          strategy.startable
          ?? (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan")
        const binding = normalizeBinding(strategy.binding, strategy.params)
        return {
          ...strategy,
          runtime,
          sourceFormat,
          startable,
          binding,
          params: {
            ...strategy.params,
            instruments: binding.instruments.map((instrument) => ({ ...instrument })),
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
        }
      })(),
    })),
    logsById: { ...logsById },
    auditById: { ...auditById },
  }

  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const request = input instanceof Request ? input : null
    const url = String(input)
    const method = request?.method ?? init?.method ?? "GET"
    const logsMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/logs/)
    const auditMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/audit/)
    const instantiateMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/]+)\/instantiate/)
    const applyLinkedInstancesMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/]+)\/apply-linked-instances$/)
    const definitionMatch = url.match(/\/api\/v1\/strategy-definitions\/([^/]+)$/)
    const lifecycleMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/(start|pause|stop)/)
    const refreshDefinitionMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/refresh-definition$/)
    const instanceMatch = url.match(/\/api\/v1\/strategies\/([^/]+)$/)

    if (url.includes("/api/v1/market-data/subscriptions"))
      return createResponse(emptyMarketDataSubscriptions)
    if (url.includes("/api/v1/system/status"))
      return createResponse(systemStatus)
    if (url.includes("/api/v1/system/storage/overview"))
      return createResponse(emptyStorageOverview)
    if (url.includes("/api/v1/system/real-trade-approvals"))
      return createResponse(emptyRealTradeApprovals)
    if (url.includes("/api/v1/system/real-trade-hard-stops"))
      return createResponse(emptyRealTradeHardStops)
    if (url.includes("/api/v1/system/real-trade-hard-stop-events"))
      return createResponse(emptyRealTradeHardStopEvents)
    if (url.includes("/api/v1/system/real-trade-kill-switch-events"))
      return createResponse(emptyRealTradeKillSwitchEvents)
    if (url.includes("/api/v1/system/real-trade-kill-switch"))
      return createResponse(emptyRealTradeKillSwitchState)
    if (url.includes("/api/v1/system/real-trade-risk-events"))
      return createResponse(emptyRealTradeRiskEvents)
    if (url.includes("/api/v1/system/real-trade-risk-limits"))
      return createResponse(emptyRealTradeRiskState)
    if (url.includes("/api/v1/system/worker/broker-order-updates"))
      return createResponse(emptyWorkerBrokerOrderUpdates)
    if (url.includes("/api/v1/brokers/futu/runtime"))
      return createResponse(brokerRuntime)
    if (url.includes("/api/v1/brokers/futu/funds"))
      return createResponse(emptyBrokerFunds)
    if (url.includes("/api/v1/brokers/futu/positions"))
      return createResponse(emptyBrokerPositions)
    if (url.includes("/api/v1/brokers/futu/orders"))
      return createResponse(emptyBrokerOrders)
    if (url.includes("/api/v1/portfolio/futu/cash-balances"))
      return createResponse(emptyPortfolioCashBalances)
    if (url.includes("/api/v1/portfolio/futu/positions"))
      return createResponse(emptyPortfolioPositions)
    if (url.includes("/api/v1/portfolio/futu/cash-reconciliation"))
      return createResponse(emptyPortfolioCashReconciliation)
    if (url.includes("/api/v1/portfolio/futu/reconciliation"))
      return createResponse(emptyPortfolioReconciliation)
    if (url.includes("/api/v1/execution/orders"))
      return createResponse(emptyExecutionOrders)
    if (url.endsWith("/api/v1/strategy-definitions") && method === "POST") {
      const payload = await readJsonBody(init, request)
      const definitionId = String(payload.id ?? "").trim() || `dsl-strategy-${runtimeState.definitions.length + 1}`
      const saved: StrategyDefinitionDocument = {
        id: definitionId,
        name: String(payload.name ?? "").trim(),
        version: "0.1.0",
        description: String(payload.description ?? "").trim(),
        runtime: "dsl-go-plan",
        sourceFormat: "dsl-v1",
        script: syncDslVersion(String(payload.script ?? ""), "0.1.0"),
        visualModel: payload.visualModel ?? null,
        createdAt: mutationTimestamp,
        updatedAt: mutationTimestamp,
      }
      runtimeState.definitions.unshift(saved)
      return createResponse(saved)
    }
    if (definitionMatch && method === "PUT") {
      const definitionId = decodeURIComponent(definitionMatch[1])
      const existingIndex = runtimeState.definitions.findIndex((item) => item.id === definitionId)
      if (existingIndex === -1) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      const existing = runtimeState.definitions[existingIndex]
      const payload = await readJsonBody(init, request)
      const name = String(payload.name ?? existing.name).trim()
      const description = String(payload.description ?? existing.description).trim()
      const scriptCandidate = String(payload.script ?? existing.script)
      const visualModel = payload.visualModel ?? existing.visualModel ?? null
      const changed =
        name !== existing.name
        || description !== existing.description
        || scriptCandidate !== existing.script
        || JSON.stringify(visualModel) !== JSON.stringify(existing.visualModel ?? null)
      const nextVersion = changed ? nextPatchVersion(existing.version) : existing.version
      const saved: StrategyDefinitionDocument = {
        ...existing,
        id: definitionId,
        name,
        description,
        runtime: "dsl-go-plan",
        sourceFormat: "dsl-v1",
        script: syncDslVersion(scriptCandidate, nextVersion),
        visualModel,
        version: nextVersion,
        updatedAt: changed ? mutationTimestamp : existing.updatedAt,
      }
      runtimeState.definitions.splice(existingIndex, 1, saved)
      return createResponse(saved)
    }
    if (definitionMatch && method === "DELETE") {
      const definitionId = decodeURIComponent(definitionMatch[1])
      const existingIndex = runtimeState.definitions.findIndex((item) => item.id === definitionId)
      if (existingIndex === -1) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      const linkedStrategies = runtimeState.strategies.filter((strategy) => strategyUsesDefinition(strategy, definitionId))
      if (linkedStrategies.length > 0) {
        return createErrorResponse(
          `当前有 ${linkedStrategies.length} 个实例仍关联该策略，请先删除对应实例再删除。实例: ${linkedStrategies.map((strategy) => strategy.id).join(", ")}`,
        )
      }
      const [removed] = runtimeState.definitions.splice(existingIndex, 1)
      return createResponse(removed)
    }
    if (instantiateMatch && method === "POST") {
      const definitionId = decodeURIComponent(instantiateMatch[1])
      const definition = runtimeState.definitions.find((item) => item.id === definitionId)
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      const payload = await readJsonBody(init, request)
      const instanceId = `${definitionId}-instance`
      const sourceFormat = definition.sourceFormat ?? "dsl-v1"
      const runtime = sourceFormat === "dsl-v1" ? "dsl-go-plan" : definition.runtime
      const startable =
        (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan")
        || (sourceFormat === "dsl-v1" && runtime === "dsl-go-plan")
      const binding = normalizeBinding(payload, {
        symbol: definition.symbol ?? "",
        interval: definition.interval ?? "5m",
      })
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
          instruments: binding.instruments.map((instrument) => ({ ...instrument })),
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
      }
      runtimeState.strategies = [instance, ...runtimeState.strategies.filter((item) => item.id !== instanceId)]
      runtimeState.logsById[instanceId] = [
        `${definition.updatedAt} instantiated strategy from definition ${definition.id}`,
      ]
      runtimeState.auditById[instanceId] = [
        {
          instanceId,
          kind: "instantiated",
          detail: `${definition.id} | interval=${binding.interval} | mode=${binding.executionMode}`,
          at: definition.updatedAt,
        },
      ]
      return createResponse(serializeStrategy(instance))
    }
    if (applyLinkedInstancesMatch && method === "POST") {
      const definitionId = decodeURIComponent(applyLinkedInstancesMatch[1])
      const definition = runtimeState.definitions.find((item) => item.id === definitionId)
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      const result = {
        definitionId,
        latestVersion: definition.version,
        totalLinked: 0,
        applied: [] as string[],
        alreadyLatest: [] as string[],
        skippedBusy: [] as string[],
      }
      for (const strategy of runtimeState.strategies) {
        if (!strategyUsesDefinition(strategy, definitionId)) {
          continue
        }
        result.totalLinked += 1
        if (strategy.status !== "STOPPED") {
          result.skippedBusy.push(strategy.id)
          continue
        }
        if (strategy.definition.version === definition.version) {
          result.alreadyLatest.push(strategy.id)
          continue
        }
        applyDefinitionSnapshot(strategy, definition)
        result.applied.push(strategy.id)
      }
      return createResponse(result)
    }
    if (instanceMatch && method === "PUT") {
      const instanceId = decodeURIComponent(instanceMatch[1])
      const instance = runtimeState.strategies.find((item) => item.id === instanceId)
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`)
      }
      const payload = await readJsonBody(init, request)
      const binding = normalizeBinding(payload, instance.params)
      instance.binding = binding
      instance.params = {
        ...instance.params,
        instruments: binding.instruments.map((instrument) => ({ ...instrument })),
        symbols: [...binding.symbols],
        symbol: binding.symbols[0] ?? "",
        interval: binding.interval,
        executionMode: binding.executionMode,
        ...(binding.brokerAccount === null
          ? {}
          : {
              brokerAccount: { ...binding.brokerAccount },
            }),
      }
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        "2026-05-23T00:00:00.000Z updated strategy binding",
      ]
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: "binding.updated",
          detail: `${binding.symbols.join(", ") || "未绑定"} / ${binding.interval}`,
          at: "2026-05-23T00:00:00.000Z",
        },
      ]
      return createResponse(serializeStrategy(instance))
    }
    if (instanceMatch && method === "DELETE") {
      const instanceId = decodeURIComponent(instanceMatch[1])
      const instanceIndex = runtimeState.strategies.findIndex((item) => item.id === instanceId)
      if (instanceIndex === -1) {
        throw new Error(`Unknown strategy instance: ${instanceId}`)
      }
      const [removed] = runtimeState.strategies.splice(instanceIndex, 1)
      delete runtimeState.logsById[instanceId]
      delete runtimeState.auditById[instanceId]
      return createResponse(serializeStrategy(removed))
    }
    if (refreshDefinitionMatch && method === "POST") {
      const instanceId = decodeURIComponent(refreshDefinitionMatch[1])
      const instance = runtimeState.strategies.find((item) => item.id === instanceId)
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`)
      }
      const definitionId = instance.definition.strategyId || String(instance.params.definitionId ?? "").trim()
      const definition = runtimeState.definitions.find((item) => item.id === definitionId)
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      if (instance.status !== "STOPPED") {
        throw new Error("strategy instance must be stopped before refreshing definition")
      }
      applyDefinitionSnapshot(instance, definition)
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        `${mutationTimestamp} refreshed strategy definition ${definition.id}`,
      ]
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: "definition.refreshed",
          detail: `${definition.id} | ${definition.version}`,
          at: mutationTimestamp,
        },
      ]
      return createResponse(serializeStrategy(instance))
    }
    if (lifecycleMatch && method === "POST") {
      const instanceId = decodeURIComponent(lifecycleMatch[1])
      const action = lifecycleMatch[2]
      const instance = runtimeState.strategies.find((item) => item.id === instanceId)
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`)
      }
      const nextStatus = action === "start" ? "RUNNING" : action === "pause" ? "PAUSED" : "STOPPED"
      instance.status = nextStatus
      runtimeState.logsById[instanceId] = [
        ...(runtimeState.logsById[instanceId] ?? []),
        `2026-05-23T00:00:00.000Z ${action}ed strategy ${instance.definition.strategyId}`,
      ]
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: action === "start" ? "started" : action === "pause" ? "paused" : "stopped",
          detail: action === "pause" ? "manual pause" : `manual ${action}`,
          at: "2026-05-23T00:00:00.000Z",
        },
      ]
      return createResponse(serializeStrategy(instance))
    }
    if (definitionMatch && method === "GET") {
      const definitionId = decodeURIComponent(definitionMatch[1])
      const definition = runtimeState.definitions.find((item) => item.id === definitionId)
      if (definition === undefined) {
        throw new Error(`Unknown strategy definition: ${definitionId}`)
      }
      return createResponse(cloneDefinition(definition))
    }
    if (url.includes("/api/v1/strategy-definitions"))
      return createResponse(runtimeState.definitions.map((definition) => cloneDefinition(definition)))
    if (logsMatch) {
      const instanceId = decodeURIComponent(logsMatch[1])
      const requestUrl = new URL(url, "http://localhost")
      const limit = Number.parseInt(requestUrl.searchParams.get("limit") ?? "500", 10)
      const offset = Number.parseInt(requestUrl.searchParams.get("offset") ?? "0", 10)
      const logs = runtimeState.logsById[instanceId] ?? []
      const pagedLogs = logs.slice(offset, offset + limit)
      return createResponse({
        instanceId,
        logs: pagedLogs,
        page: {
          limit,
          offset,
          total: logs.length,
          returned: pagedLogs.length,
          hasMore: offset + pagedLogs.length < logs.length,
        },
      })
    }
    if (auditMatch) {
      const instanceId = decodeURIComponent(auditMatch[1])
      const requestUrl = new URL(url, "http://localhost")
      const limit = Number.parseInt(requestUrl.searchParams.get("limit") ?? "500", 10)
      const offset = Number.parseInt(requestUrl.searchParams.get("offset") ?? "0", 10)
      const entries = runtimeState.auditById[instanceId] ?? []
      const pagedEntries = entries.slice(offset, offset + limit)
      return createResponse({
        instanceId,
        entries: pagedEntries,
        page: {
          limit,
          offset,
          total: entries.length,
          returned: pagedEntries.length,
          hasMore: offset + pagedEntries.length < entries.length,
        },
      })
    }
    if (url.includes("/api/v1/strategies")) {
      return createResponse(runtimeState.strategies.map((strategy) => serializeStrategy(strategy)))
    }

    throw new Error(`Unexpected request: ${url}`)
  })
}

export function buildDslScript(
  name: string,
  body: string[] = ['log "close"'],
  options?: {
    version?: string;
    symbol?: string;
    interval?: string;
  },
) {
  const version = options?.version ?? "0.1.0"
  const symbol = options?.symbol ?? "00700"
  const interval = options?.interval ?? "1m"

  return [
    `strategy ${name}`,
    `version ${version}`,
    `symbol ${symbol}`,
    `interval ${interval}`,
    "",
    "on kline_close:",
    ...body.map((line) => `  ${line}`),
  ].join("\n")
}

export function buildRuntimeAccount(overrides?: Partial<BrokerRuntimeResponse>): BrokerRuntimeResponse {
  return {
    ...emptyBrokerRuntime,
    ...overrides,
    accounts: overrides?.accounts ?? emptyBrokerRuntime.accounts,
  }
}

export { MockEventSource, flushRequests }