import { vi } from "vitest"

import {
  emptyBrokerFunds,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptyExecutionOrders,
  emptyMarketDataSubscriptions,
  emptyPortfolioCashBalances,
  emptyPortfolioPositions,
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
} from "@/contracts"
import type {
  BrokerRuntimeResponse,
  StrategyDefinitionDocument,
  SystemStatusResponse,
} from "@/contracts"
import { PINE_WORKER_RUNTIME } from "../src/components/strategy-runtime/strategyRuntimeIdentity"

import { createResponse, MockWebSocket, flushRequests } from "./helpers"
import { createStrategyPineAnalyzeResponse } from "./strategyPageAnalyzeMock"
import { setCurrentStrategySystemStatus } from "./strategyPageTestState"

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
    sourceFormat?: "pine-v6";
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
  lifecycleErrorByAction?: Partial<Record<"start" | "pause" | "stop", { message: string; code?: string; status?: number }>>;
}

export function buildFetchMock(options: BuildFetchMockOptions) {
  const systemStatus = options.systemStatus ?? emptySystemStatus
  const brokerRuntime = options.brokerRuntime ?? emptyBrokerRuntime
  setCurrentStrategySystemStatus(systemStatus)
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

  function syncPineVersion(script: string, version: string) {
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
      runtime: definition.runtime ?? PINE_WORKER_RUNTIME,
      sourceFormat: definition.sourceFormat ?? "pine-v6",
      visualModel: definition.visualModel ?? null,
    }
  }

  function createErrorResponse(message: string, status = 400, code = "BAD_REQUEST"): Response {
    return {
      ok: false,
      status,
      json: async () => ({
        ok: false,
        error: {
          code,
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

  function normalizeMarketInstrumentRequest(payload: Record<string, unknown>) {
    const candidate = normalizeInstrumentId(payload.instrumentId)
    const marketAndCode = candidate.includes(".")
      ? candidate.split(".", 2)
      : [
          String(payload.market ?? "HK").trim().toUpperCase(),
          String(payload.code ?? "").trim().toUpperCase(),
        ]
    const market = marketAndCode[0] ?? ""
    const code = marketAndCode[1] ?? ""
    if (
      market === ""
      || code === ""
      || market === "CN"
      || /\s/.test(market)
      || /\s/.test(code)
    ) {
      return null
    }
    return {
      market: market === "SH" || market === "SZ" ? "CN" : market,
      prefix: market,
      code,
      symbol: `${market}.${code}`,
      instrumentId: `${market}.${code}`,
      resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
    }
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
      sourceFormat?: "pine-v6";
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
    strategy.runtime = definition.runtime ?? PINE_WORKER_RUNTIME
    strategy.sourceFormat = definition.sourceFormat ?? "pine-v6"
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
        const runtime = strategy.runtime ?? PINE_WORKER_RUNTIME
        const sourceFormat = strategy.sourceFormat ?? "pine-v6"
        const startable =
          strategy.startable
          ?? (sourceFormat === "pine-v6" && runtime === PINE_WORKER_RUNTIME)
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
    const runtimeRiskMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/runtime-risk$/)
    const refreshDefinitionMatch = url.match(/\/api\/v1\/strategies\/([^/]+)\/refresh-definition$/)
    const instanceMatch = url.match(/\/api\/v1\/strategies\/([^/]+)$/)
    const syncProgressMatch = url.match(/\/api\/v1\/backtests\/sync\/([^/]+)$/)

    if (url.includes("/api/v1/settings/brokers"))
      return createResponse(emptyBrokerSettings)
    if (url.includes("/api/v1/backtests/sync") && method === "POST")
      return createResponse({ taskId: "sync-native-1", message: "sync queued" })
    if (syncProgressMatch && method === "GET")
      return createResponse({
        taskId: decodeURIComponent(syncProgressMatch[1]),
        status: "completed",
        symbol: "HK.00700",
        currentInterval: "5m",
        totalIntervals: 1,
        completedIntervals: 1,
        totalBatches: 1,
        completedBatches: 1,
        retries: 0,
        startedAt: mutationTimestamp,
        updatedAt: mutationTimestamp,
      })
    if (syncProgressMatch && method === "DELETE")
      return createResponse({ taskId: decodeURIComponent(syncProgressMatch[1]), status: "cancelled" })
    if (url.includes("/api/v1/market-data/subscriptions"))
      return createResponse(emptyMarketDataSubscriptions)
    if (url.includes("/api/v1/market-data/markets")) {
      return createResponse({
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
          {
            code: "US",
            resolvedMarket: "US",
            preferredPrefix: "US",
            displayName: "US",
            quoteCurrency: "USD",
            supportsExtendedHours: true,
            requiresExchangePrefix: false,
            aliases: ["NYSE", "NASDAQ"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
          {
            code: "SH",
            resolvedMarket: "CN",
            preferredPrefix: "SH",
            displayName: "Shanghai",
            quoteCurrency: "CNY",
            supportsExtendedHours: false,
            requiresExchangePrefix: true,
            aliases: ["CNSH"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
          {
            code: "SZ",
            resolvedMarket: "CN",
            preferredPrefix: "SZ",
            displayName: "Shenzhen",
            quoteCurrency: "CNY",
            supportsExtendedHours: false,
            requiresExchangePrefix: true,
            aliases: ["CNSZ"],
            regularSessions: [],
            precision: { price: 2, quote: 2 },
            tickSize: 0.01,
          },
        ],
        defaultMarket: "HK",
        updatedAt: "2026-06-12T00:00:00Z",
      })
    }
    if (url.includes("/api/v1/market-data/instruments?")) {
      const requestURL = new URL(url, "http://localhost")
      const requestedMarket = (requestURL.searchParams.get("market") ?? "HK").trim().toUpperCase()
      const rawQuery = (requestURL.searchParams.get("query") ?? "").trim().toUpperCase().replace(":", ".")
      if (rawQuery === "") {
        return createResponse({ query: "", totalReturned: 0, entries: [] })
      }
      const embedded = rawQuery.includes(".") ? rawQuery.split(".", 2) : null
      const market = embedded?.[0] ?? requestedMarket
      const code = embedded?.[1] ?? rawQuery
      return createResponse({
        requestedMarket,
        query: rawQuery,
        resolutionStatus: "resolved",
        totalReturned: 1,
        entries: [{
          market,
          resolvedMarket: market === "SH" || market === "SZ" ? "CN" : market,
          instrumentId: `${market}.${code}`,
          code,
          symbol: code,
          name: null,
          securityType: "STOCK",
          lotSize: 1,
          source: "test-static",
        }],
        failures: [],
      })
    }
    if (url.includes("/api/v1/market-data/instruments/normalize")) {
      const payload = await readJsonBody(init, request)
      const normalized = normalizeMarketInstrumentRequest(payload)
      if (normalized === null) {
        return createErrorResponse("invalid instrument")
      }
      return createResponse(normalized)
    }
    if (url.includes("/api/v1/strategy-pine/analyze")) {
      const payload = await readJsonBody(init, request)
      return createStrategyPineAnalyzeResponse(payload)
    }
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
    if (url.includes("/api/v1/execution/orders"))
      return createResponse(emptyExecutionOrders)
    if (url.endsWith("/api/v1/strategy-definitions") && method === "POST") {
      const payload = await readJsonBody(init, request)
      const definitionId = String(payload.id ?? "").trim() || `pine-strategy-${runtimeState.definitions.length + 1}`
      const saved: StrategyDefinitionDocument = {
        id: definitionId,
        name: String(payload.name ?? "").trim(),
        version: "0.1.0",
        description: String(payload.description ?? "").trim(),
        runtime: PINE_WORKER_RUNTIME,
        sourceFormat: "pine-v6",
        script: syncPineVersion(String(payload.script ?? ""), "0.1.0"),
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
        runtime: PINE_WORKER_RUNTIME,
        sourceFormat: "pine-v6",
        script: syncPineVersion(scriptCandidate, nextVersion),
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
      const sourceFormat = definition.sourceFormat ?? "pine-v6"
      const runtime = sourceFormat === "pine-v6" ? PINE_WORKER_RUNTIME : definition.runtime
      const startable =
        sourceFormat === "pine-v6" && runtime === PINE_WORKER_RUNTIME
      const binding = normalizeBinding(payload, {
        symbol: definition.symbol ?? "",
        interval: definition.interval ?? "5m",
      })
      const instance = {
        id: instanceId,
        pluginId: PINE_WORKER_RUNTIME,
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
          ...(sourceFormat === "pine-v6"
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
    if (runtimeRiskMatch && method === "PUT") {
      const instanceId = decodeURIComponent(runtimeRiskMatch[1])
      const instance = runtimeState.strategies.find((item) => item.id === instanceId)
      if (instance === undefined) {
        throw new Error(`Unknown strategy instance: ${instanceId}`)
      }
      const runtimeRisk = await readJsonBody(init, request)
      instance.params = {
        ...instance.params,
        runtimeRisk,
      }
      runtimeState.auditById[instanceId] = [
        ...(runtimeState.auditById[instanceId] ?? []),
        {
          instanceId,
          kind: "runtime_risk.updated",
          detail: String(runtimeRisk.mode ?? "off"),
          at: mutationTimestamp,
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
      const lifecycleError = options.lifecycleErrorByAction?.[action as "start" | "pause" | "stop"]
      if (lifecycleError !== undefined) {
        return createErrorResponse(
          lifecycleError.message,
          lifecycleError.status ?? 400,
          lifecycleError.code ?? "BAD_REQUEST",
        )
      }
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

export { buildPineScript, buildRuntimeAccount } from "./strategyPageScriptFixtures"

export { MockWebSocket, flushRequests }
