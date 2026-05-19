import { defineStore } from "pinia";
import {
  type InjectionKey,
  computed,
  inject,
  markRaw,
  provide,
  ref,
} from "vue";

import {
  type ApiErrorEnvelope,
  type ApiSuccessEnvelope,
  type BrokerCashFlowsResponse,
  type BrokerFundsResponse,
  type BrokerOrderFeesResponse,
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  type FutuOpenDHealthResponse,
  type FutuOpenDInstallGuideResponse,
  type MarketDataSubscriptionsResponse,
  type PluginCatalogResponse,
  type PluginInstallResponse,
  type PluginOperationDto,
  type PluginUninstallGuidanceDto,
  type PortfolioCashBalancesResponse,
  type PortfolioCashReconciliationResponse,
  type PortfolioPositionsResponse,
  type PortfolioReconciliationResponse,
  type RealTradeApprovalsResponse,
  type RealTradeHardStopEventsResponse,
  type RealTradeHardStopsResponse,
  type RealTradeKillSwitchEventsResponse,
  type RealTradeKillSwitchStateResponse,
  type RealTradeRiskEventsResponse,
  type RealTradeRiskStateResponse,
  type StorageOverviewResponse,
  type SystemStatusResponse,
  type WorkerBrokerOrderUpdateErrorContext,
  type WorkerBrokerOrderUpdatesResponse,
  emptyBrokerCashFlows,
  emptyBrokerFunds,
  emptyBrokerOrderFees,
  emptyBrokerOrders,
  emptyBrokerPositions,
  emptyBrokerRuntime,
  emptyBrokerSettings,
  emptyExecutionOrderEvents,
  emptyExecutionOrders,
  emptyFutuOpenDHealth,
  emptyFutuOpenDInstallGuide,
  emptyMarketDataSubscriptions,
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

import { normalizeKlinePeriod } from "../charting/kline";
import {
  useWorkspaceLayout,
  type WorkspaceLayoutStore,
} from "./useWorkspaceLayout";

type RealTradeHardStopScope = "ACCOUNT" | "MARKET" | "SYMBOL";

interface BrokerAccountSelectionOption {
  selectionKey: string;
  source: "managed" | "runtime";
  brokerId: string;
  accountId: string;
  displayName: string;
  tradingEnvironment: string;
  market: string;
  securityFirm: string | null;
}

interface MarketDataSnapshotQueryResult {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  snapshot: {
    price: number;
    bid: number;
    ask: number;
    openPrice?: number | null;
    highPrice?: number | null;
    lowPrice?: number | null;
    previousClosePrice?: number | null;
    volume: number;
    turnover: number;
    at: string;
  } | null;
  meta: {
    instrumentId: string;
    source: string | null;
    resolvedAt: string;
    fromCache: boolean;
  };
}

interface MarketDataCandlesQueryResult {
  request: {
    instrument: {
      market: string;
      symbol: string;
      instrumentId: string;
    };
    period: string;
    limit: number;
  };
  candles: Array<{
    period: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
    at: string;
  }>;
  totalReturned: number;
  meta: {
    instrumentId: string;
    source: string | null;
    resolvedAt: string;
    fromCache: boolean;
  };
}

interface MarketDataTickLiveEvent {
  type: "market-data.tick";
  at: string;
  brokerId: string;
  instrument: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  snapshot: {
    price: number;
    bid: number;
    ask: number;
    volume: number;
    turnover: number;
    at: string;
  };
  source: string | null;
}

interface LoadMarketDataQueryOptions {
  appendOlder?: boolean;
  fromTime?: string;
  toTime?: string;
}

interface MarketInstrumentReference {
  market: string;
  symbol: string;
  instrumentId: string;
  name: string | null;
  securityType: string | null;
  lotSize: number | null;
  exchange: string | null;
  status: string | null;
  source: string;
  updatedAt: string;
  brokerMappings: Array<{
    brokerId: string;
    brokerMarket: string;
    brokerSymbol: string;
    brokerInstrumentId: string;
    displayName: string | null;
    source: string;
    updatedAt: string;
  }>;
}

interface MarketInstrumentReferenceResponse {
  query: string;
  totalReturned: number;
  entries: MarketInstrumentReference[];
}

export interface MarketInstrumentSearchOption {
  market: string;
  symbol: string;
  instrumentId: string;
  name: string | null;
  label: string;
  lookupValue: string;
  sources: string[];
}

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");
const DEFAULT_TICK_QUERY_LIMIT = 20_000;
const DEFAULT_TICK_QUERY_LOOKBACK_MS = 15 * 60 * 1000;

function buildApiUrl(path: string): string {
  return apiBaseUrl ? `${apiBaseUrl}${path}` : `http://localhost:3000${path}`;
}

async function fetchEnvelope<T>(path: string): Promise<T> {
  const response = await fetch(buildApiUrl(path));
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "Unknown API error" : body.error.message);
  }

  return body.data;
}

async function fetchEnvelopeWithInit<T>(
  path: string,
  init: RequestInit,
): Promise<T> {
  const response = await fetch(buildApiUrl(path), init);
  const body = (await response.json()) as
    | ApiSuccessEnvelope<T>
    | ApiErrorEnvelope;

  if (!response.ok || !body.ok) {
    throw new Error(body.ok ? "Unknown API error" : body.error.message);
  }

  return body.data;
}

function buildBrokerAccountSelectionKey(input: {
  brokerId: string;
  tradingEnvironment: string;
  accountId: string;
  market: string;
}): string {
  return [
    input.brokerId,
    input.tradingEnvironment,
    input.accountId,
    input.market,
  ]
    .map((segment) => encodeURIComponent(segment))
    .join("|");
}

function parseBrokerAccountSelectionKey(key: string | null | undefined): {
  brokerId: string;
} | null {
  if (key == null || key === "") {
    return null;
  }

  const [brokerId] = key.split("|");

  if (brokerId == null || brokerId === "") {
    return null;
  }

  return {
    brokerId: decodeURIComponent(brokerId),
  };
}

function parseMarketInstrumentInput(
  value: string,
  fallbackMarket: string,
): { market: string; symbol: string } | null {
  const normalized = value.trim().toUpperCase();
  if (normalized === "") {
    return null;
  }

  const separator = normalized.includes(":") ? ":" : ".";
  if (normalized.includes(separator)) {
    const [market, symbol] = normalized.split(separator, 2);
    if (market != null && symbol != null && market !== "" && symbol !== "") {
      return { market, symbol };
    }
  }

  const market = fallbackMarket.trim().toUpperCase();
  if (market === "") {
    return null;
  }

  return { market, symbol: normalized };
}

function normalizeInstrumentParts(
  input: {
    market?: string | null;
    symbol?: string | null;
  },
  fallbackMarket?: string,
): { market: string; symbol: string } | null {
  const rawSymbol = input.symbol?.trim().toUpperCase();
  const rawMarket = input.market?.trim().toUpperCase() ?? "";

  if (rawSymbol == null || rawSymbol === "") {
    return null;
  }

  const embeddedSeparator = rawSymbol.includes(":") ? ":" : ".";
  if (rawSymbol.includes(embeddedSeparator)) {
    const [symbolMarket, symbol] = rawSymbol.split(embeddedSeparator, 2);
    if (
      symbolMarket != null &&
      symbol != null &&
      symbolMarket !== "" &&
      symbol !== ""
    ) {
      return {
        market: rawMarket === "" ? symbolMarket : rawMarket,
        symbol,
      };
    }
  }

  const market = rawMarket || fallbackMarket?.trim().toUpperCase() || "";
  if (market === "") {
    return null;
  }

  return { market, symbol: rawSymbol };
}

function createRandomConsumerSuffix(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }

  return Math.random().toString(36).slice(2);
}

function createStableWebConsumerId(scope: string): string {
  const storageKey = `jftrade.market-data.consumer.${scope}`;
  const fallback = `web:${scope}:${createRandomConsumerSuffix()}`;

  if (typeof window === "undefined" || window.sessionStorage == null) {
    return fallback;
  }

  const existing = window.sessionStorage.getItem(storageKey);
  if (existing != null && existing.trim() !== "") {
    return existing;
  }

  window.sessionStorage.setItem(storageKey, fallback);
  return fallback;
}

function createConsoleDataStore(workspaceLayout: WorkspaceLayoutStore) {
  const { prefs, update } = workspaceLayout;
  const systemStatus = ref<SystemStatusResponse>(emptySystemStatus);
  const storageOverview = ref<StorageOverviewResponse>(emptyStorageOverview);
  const brokerSettings = ref<BrokerSettingsResponse>(emptyBrokerSettings);
  const pluginCatalog = ref<PluginCatalogResponse>(emptyPluginCatalog);
  const futuOpenDHealth = ref<FutuOpenDHealthResponse>(emptyFutuOpenDHealth);
  const futuOpenDInstallGuide = ref<FutuOpenDInstallGuideResponse>(
    emptyFutuOpenDInstallGuide,
  );
  const pluginError = ref("");
  const installingPluginIds = ref<string[]>([]);
  const uninstallingPluginIds = ref<string[]>([]);
  const realTradeApprovals = ref<RealTradeApprovalsResponse>(
    emptyRealTradeApprovals,
  );
  const realTradeHardStopEvents = ref<RealTradeHardStopEventsResponse>(
    emptyRealTradeHardStopEvents,
  );
  const realTradeHardStops = ref<RealTradeHardStopsResponse>(
    emptyRealTradeHardStops,
  );
  const realTradeKillSwitchState = ref<RealTradeKillSwitchStateResponse>(
    emptyRealTradeKillSwitchState,
  );
  const realTradeKillSwitchEvents = ref<RealTradeKillSwitchEventsResponse>(
    emptyRealTradeKillSwitchEvents,
  );
  const realTradeRiskState = ref<RealTradeRiskStateResponse>(
    emptyRealTradeRiskState,
  );
  const realTradeRiskEvents = ref<RealTradeRiskEventsResponse>(
    emptyRealTradeRiskEvents,
  );
  const workerBrokerOrderUpdates = ref<WorkerBrokerOrderUpdatesResponse>(
    emptyWorkerBrokerOrderUpdates,
  );
  const brokerRuntime = ref<BrokerRuntimeResponse>(emptyBrokerRuntime);
  const brokerCashFlows = ref<BrokerCashFlowsResponse>(emptyBrokerCashFlows);
  const brokerOrderFees = ref<BrokerOrderFeesResponse>(emptyBrokerOrderFees);
  const brokerFunds = ref<BrokerFundsResponse>(emptyBrokerFunds);
  const brokerPositions = ref<BrokerPositionsResponse>(emptyBrokerPositions);
  const brokerOrders = ref<BrokerOrdersResponse>(emptyBrokerOrders);
  const portfolioCashBalances = ref<PortfolioCashBalancesResponse>(
    emptyPortfolioCashBalances,
  );
  const portfolioCashReconciliation = ref<PortfolioCashReconciliationResponse>(
    emptyPortfolioCashReconciliation,
  );
  const portfolioPositions = ref<PortfolioPositionsResponse>(
    emptyPortfolioPositions,
  );
  const portfolioReconciliation = ref<PortfolioReconciliationResponse>(
    emptyPortfolioReconciliation,
  );
  const executionOrders = ref<ExecutionOrdersResponse>(emptyExecutionOrders);
  const executionOrderEvents = ref<ExecutionOrderEventsResponse>(
    emptyExecutionOrderEvents,
  );
  const selectedExecutionOrderId = ref("");
  const isLoadingExecutionEvents = ref(false);
  const isLoadingOrderFees = ref(false);
  const executionEventsError = ref("");
  const orderFeesError = ref("");
  const marketDataSubscriptions = ref<MarketDataSubscriptionsResponse>(
    emptyMarketDataSubscriptions,
  );
  const marketDataQueryMarket = ref("HK");
  const marketDataQuerySymbol = ref("00700");
  const marketDataQueryPeriod = ref("1m");
  const marketDataQueryLimit = ref(500);
  const marketDataSnapshot = ref<MarketDataSnapshotQueryResult | null>(null);
  const marketDataCandles = ref<MarketDataCandlesQueryResult | null>(null);
  const marketInstrumentReferences = ref<MarketInstrumentReference[]>([]);
  const isLoadingMarketData = ref(false);
  const isLoadingMarketDataQuery = ref(false);
  const marketDataError = ref("");
  const marketDataQueryError = ref("");
  const isLoading = ref(true);
  const loadError = ref("");
  const liveStreamStatus = ref<"disconnected" | "connected" | "degraded">(
    "disconnected",
  );
  const liveStreamCheckedAt = ref("");

  const selectedExecutionOrder = computed(
    () =>
      executionOrders.value.orders.find(
        (order) => order.internalOrderId === selectedExecutionOrderId.value,
      ) ?? null,
  );

  let consoleStream: EventSource | null = null;
  let initialized = false;
  let isBackgroundRefreshing = false;
  let activeSystemStatePromise: Promise<void> | null = null;
  let lastForegroundSystemStateRefreshAt = 0;
  let activeMarketDataQuery: {
    key: string;
    promise: Promise<void>;
    requestId: number;
  } | null = null;
  let marketDataQueryRequestId = 0;

  function resolveActiveBrokerId(options?: {
    settings?: BrokerSettingsResponse;
    status?: SystemStatusResponse;
  }): string {
    const settings = options?.settings ?? brokerSettings.value;
    const status = options?.status ?? systemStatus.value;
    const preferredSelection = parseBrokerAccountSelectionKey(
      prefs.value.selectedBrokerAccountKey,
    );

    if (preferredSelection != null) {
      return preferredSelection.brokerId;
    }

    return (
      settings.accounts.find((account) => account.enabled)?.brokerId ??
      settings.brokers[0]?.descriptor.id ??
      status.defaultBroker ??
      "futu"
    );
  }

  function resolveBrokerAccountOptions(options: {
    activeBrokerId: string;
    settings: BrokerSettingsResponse;
    runtime: BrokerRuntimeResponse;
    fallbackMarket: string;
  }): BrokerAccountSelectionOption[] {
    const selectionOptions: BrokerAccountSelectionOption[] = [];
    const seen = new Set<string>();

    for (const account of options.settings.accounts) {
      if (!account.enabled) {
        continue;
      }

      const selectionKey = buildBrokerAccountSelectionKey({
        brokerId: account.brokerId,
        tradingEnvironment: account.tradingEnvironment,
        accountId: account.accountId,
        market: account.market,
      });

      seen.add(selectionKey);
      selectionOptions.push({
        selectionKey,
        source: "managed",
        brokerId: account.brokerId,
        accountId: account.accountId,
        displayName: account.displayName,
        tradingEnvironment: account.tradingEnvironment,
        market: account.market,
        securityFirm: account.securityFirm,
      });
    }

    if (options.runtime.descriptor.id !== options.activeBrokerId) {
      return selectionOptions;
    }

    for (const account of options.runtime.accounts) {
      const market = account.marketAuthorities[0] ?? options.fallbackMarket;
      const selectionKey = buildBrokerAccountSelectionKey({
        brokerId: options.activeBrokerId,
        tradingEnvironment: account.tradingEnvironment,
        accountId: account.accountId,
        market,
      });

      if (seen.has(selectionKey)) {
        continue;
      }

      seen.add(selectionKey);
      selectionOptions.push({
        selectionKey,
        source: "runtime",
        brokerId: options.activeBrokerId,
        accountId: account.accountId,
        displayName: account.accountId,
        tradingEnvironment: account.tradingEnvironment,
        market,
        securityFirm: account.securityFirm,
      });
    }

    return selectionOptions;
  }

  function resolveSelectedBrokerAccountOption(
    options: readonly BrokerAccountSelectionOption[],
    activeBrokerId: string,
    defaultTradingEnvironment: string,
  ): BrokerAccountSelectionOption | null {
    return (
      options.find(
        (option) =>
          option.selectionKey === prefs.value.selectedBrokerAccountKey,
      ) ??
      options.find(
        (option) =>
          option.brokerId === activeBrokerId &&
          option.tradingEnvironment === defaultTradingEnvironment,
      ) ??
      options.find((option) => option.brokerId === activeBrokerId) ??
      options[0] ??
      null
    );
  }

  const availableBrokerAccounts = computed(() =>
    resolveBrokerAccountOptions({
      activeBrokerId: resolveActiveBrokerId(),
      settings: brokerSettings.value,
      runtime: brokerRuntime.value,
      fallbackMarket:
        brokerRuntime.value.descriptor.capabilities[0]?.market ??
        systemStatus.value.broker.capabilities[0]?.market ??
        "HK",
    }),
  );

  const selectedBrokerAccount = computed(() =>
    resolveSelectedBrokerAccountOption(
      availableBrokerAccounts.value,
      resolveActiveBrokerId(),
      systemStatus.value.defaultTradingEnvironment,
    ),
  );

  const marketInstrumentSearchOptions = computed<
    MarketInstrumentSearchOption[]
  >(() => {
    const byId = new Map<
      string,
      {
        market: string;
        symbol: string;
        name: string | null;
        sources: Set<string>;
      }
    >();

    function addInstrument(
      input: {
        market?: string | null;
        symbol?: string | null;
        name?: string | null;
      },
      source: string,
    ): void {
      const parsed = normalizeInstrumentParts(
        input,
        selectedBrokerAccount.value?.market ?? marketDataQueryMarket.value,
      );
      if (parsed == null) {
        return;
      }

      const instrumentId = `${parsed.market}.${parsed.symbol}`;
      const existing = byId.get(instrumentId);
      if (existing == null) {
        byId.set(instrumentId, {
          market: parsed.market,
          symbol: parsed.symbol,
          name: input.name ?? null,
          sources: new Set([source]),
        });
        return;
      }

      if (existing.name == null && input.name != null) {
        existing.name = input.name;
      }
      existing.sources.add(source);
    }

    for (const reference of marketInstrumentReferences.value) {
      addInstrument(reference, "reference");
      for (const mapping of reference.brokerMappings) {
        addInstrument(
          {
            market: mapping.brokerMarket,
            symbol: mapping.brokerSymbol,
            name: mapping.displayName ?? reference.name,
          },
          `broker:${mapping.brokerId}`,
        );
      }
    }
    for (const entry of marketDataSubscriptions.value.entries) {
      addInstrument(entry, "subscription");
    }
    for (const position of portfolioPositions.value.positions) {
      addInstrument(position, "portfolio");
    }
    for (const position of brokerPositions.value.positions) {
      addInstrument(position, "broker-position");
    }
    for (const order of brokerOrders.value.orders) {
      addInstrument(order, "broker-order");
    }
    for (const order of executionOrders.value.orders) {
      addInstrument(order, "execution-order");
    }

    return [...byId.entries()]
      .map(([instrumentId, item]) => {
        const sources = [...item.sources].sort();
        const nameSuffix = item.name == null ? "" : ` · ${item.name}`;
        return {
          market: item.market,
          symbol: item.symbol,
          instrumentId,
          name: item.name,
          label: `${instrumentId}${nameSuffix} · ${sources.join(", ")}`,
          lookupValue: `${item.market}:${item.symbol}`,
          sources,
        };
      })
      .sort((left, right) =>
        left.instrumentId.localeCompare(right.instrumentId),
      );
  });

  function resolveBrokerQuery(
    selection: BrokerAccountSelectionOption | null,
  ): URLSearchParams {
    if (
      selection != null &&
      selection.brokerId === brokerRuntime.value.descriptor.id
    ) {
      return new URLSearchParams({
        tradingEnvironment: selection.tradingEnvironment,
        accountId: selection.accountId,
        market: selection.market,
      });
    }

    const runtimeAccount =
      brokerRuntime.value.accounts.find(
        (account) =>
          account.tradingEnvironment ===
          systemStatus.value.defaultTradingEnvironment,
      ) ?? brokerRuntime.value.accounts[0];

    return new URLSearchParams({
      tradingEnvironment:
        runtimeAccount?.tradingEnvironment ??
        systemStatus.value.defaultTradingEnvironment,
      accountId: runtimeAccount?.accountId ?? "0",
      market:
        runtimeAccount?.marketAuthorities[0] ??
        brokerRuntime.value.descriptor.capabilities[0]?.market ??
        systemStatus.value.broker.capabilities[0]?.market ??
        "HK",
    });
  }

  async function loadBrokerSettings(): Promise<BrokerSettingsResponse> {
    const response = await fetchEnvelope<BrokerSettingsResponse>(
      "/api/v1/settings/brokers",
    );
    brokerSettings.value = response;
    return response;
  }

  async function requestFutuOpenDManualRetry(): Promise<void> {
    await fetchEnvelopeWithInit("/api/v1/system/futu-opend/manual-retry", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        brokerId: "futu",
      }),
    });

    await loadSystemState();
  }

  async function saveBrokerIntegration(
    brokerId: string,
    payload: {
      enabled: boolean;
      config: NonNullable<
        BrokerSettingsResponse["brokers"][number]["defaults"]
      >;
    },
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/brokers/${encodeURIComponent(brokerId)}/integration`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );

    await loadSystemState();
  }

  async function createManagedBrokerAccount(payload: {
    brokerId: string;
    accountId: string;
    displayName: string;
    tradingEnvironment: string;
    market: string;
    securityFirm?: string;
    enabled: boolean;
  }): Promise<void> {
    await fetchEnvelopeWithInit("/api/v1/settings/broker-accounts", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });

    await loadSystemState();
  }

  async function updateManagedBrokerAccount(
    accountRecordId: string,
    payload: {
      brokerId: string;
      accountId: string;
      displayName: string;
      tradingEnvironment: string;
      market: string;
      securityFirm?: string;
      enabled: boolean;
    },
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/broker-accounts/${encodeURIComponent(accountRecordId)}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );

    await loadSystemState();
  }

  async function deleteManagedBrokerAccount(
    accountRecordId: string,
  ): Promise<void> {
    await fetchEnvelopeWithInit(
      `/api/v1/settings/broker-accounts/${encodeURIComponent(accountRecordId)}`,
      {
        method: "DELETE",
      },
    );

    if (prefs.value.selectedBrokerAccountKey != null) {
      const selectedMatch = brokerSettings.value.accounts.find(
        (account) =>
          buildBrokerAccountSelectionKey({
            brokerId: account.brokerId,
            tradingEnvironment: account.tradingEnvironment,
            accountId: account.accountId,
            market: account.market,
          }) === prefs.value.selectedBrokerAccountKey,
      );

      if (selectedMatch?.id === accountRecordId) {
        update({ selectedBrokerAccountKey: null });
      }
    }

    await loadSystemState();
  }

  async function selectBrokerAccount(
    selectionKey: string | null,
  ): Promise<void> {
    update({ selectedBrokerAccountKey: selectionKey });
    await loadSystemState({ bypassCooldown: true });
  }

  async function loadBrokerCashFlows(options: {
    brokerId: string;
    brokerQuery: string;
    tradingEnvironment: string;
    clearingDate: string;
  }): Promise<void> {
    brokerCashFlows.value = emptyBrokerCashFlows;

    if (options.tradingEnvironment !== "REAL") {
      return;
    }

    try {
      brokerCashFlows.value = await fetchEnvelope<BrokerCashFlowsResponse>(
        `/api/v1/brokers/${encodeURIComponent(options.brokerId)}/cash-flows?${options.brokerQuery}&clearingDate=${encodeURIComponent(options.clearingDate)}`,
      );
    } catch (error) {
      brokerCashFlows.value = {
        ...emptyBrokerCashFlows,
        connectivity: "disconnected",
        lastError:
          error instanceof Error
            ? error.message
            : "Failed to load broker cash flows.",
      };
    }
  }

  async function loadExecutionOrderEvents(
    internalOrderId: string,
  ): Promise<void> {
    executionEventsError.value = "";
    isLoadingExecutionEvents.value = true;

    try {
      executionOrderEvents.value =
        await fetchEnvelope<ExecutionOrderEventsResponse>(
          `/api/v1/execution/orders/${encodeURIComponent(internalOrderId)}/events`,
        );
    } catch (error) {
      executionEventsError.value =
        error instanceof Error
          ? error.message
          : "Failed to load execution order events.";
      executionOrderEvents.value = {
        internalOrderId,
        events: [],
      };
    } finally {
      isLoadingExecutionEvents.value = false;
    }
  }

  async function loadExecutionOrderFees(
    internalOrderId: string,
  ): Promise<void> {
    orderFeesError.value = "";
    brokerOrderFees.value = emptyBrokerOrderFees;

    const order = executionOrders.value.orders.find(
      (candidate) => candidate.internalOrderId === internalOrderId,
    );

    if (
      order == null ||
      order.brokerOrderId == null ||
      order.tradingEnvironment !== "REAL"
    ) {
      return;
    }

    isLoadingOrderFees.value = true;

    try {
      brokerOrderFees.value = await fetchEnvelope<BrokerOrderFeesResponse>(
        `/api/v1/brokers/${encodeURIComponent(order.brokerId)}/order-fees?tradingEnvironment=${encodeURIComponent(order.tradingEnvironment)}&accountId=${encodeURIComponent(order.accountId)}&market=${encodeURIComponent(order.market)}&orderId=${encodeURIComponent(order.brokerOrderId)}`,
      );
    } catch (error) {
      orderFeesError.value =
        error instanceof Error
          ? error.message
          : "Failed to load broker order fees.";
    } finally {
      isLoadingOrderFees.value = false;
    }
  }

  async function loadExecutionOrderDetails(
    internalOrderId: string,
  ): Promise<void> {
    selectedExecutionOrderId.value = internalOrderId;

    await Promise.all([
      loadExecutionOrderEvents(internalOrderId),
      loadExecutionOrderFees(internalOrderId),
    ]);
  }

  async function loadMarketDataSubscriptions(): Promise<void> {
    marketDataError.value = "";
    isLoadingMarketData.value = true;

    try {
      marketDataSubscriptions.value =
        await fetchEnvelope<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
        );
    } catch (error) {
      marketDataError.value =
        error instanceof Error
          ? error.message
          : "Failed to load market data subscriptions.";
    } finally {
      isLoadingMarketData.value = false;
    }
  }

  async function loadMarketInstrumentReferences(
    query = "",
  ): Promise<MarketInstrumentReferenceResponse> {
    const params = new URLSearchParams({
      limit: "50",
      market: marketDataQueryMarket.value.trim().toUpperCase() || "HK",
    });
    if (query.trim() !== "") {
      params.set("query", query.trim());
    }

    const response = await fetchEnvelope<MarketInstrumentReferenceResponse>(
      `/api/v1/market-data/instruments?${params.toString()}`,
    );
    const merged = new Map(
      marketInstrumentReferences.value.map((entry) => [
        entry.instrumentId,
        entry,
      ]),
    );
    for (const entry of response.entries) {
      merged.set(entry.instrumentId, entry);
    }
    marketInstrumentReferences.value = [...merged.values()];
    return response;
  }

  async function acquireMarketDataSubscription(input: {
    consumerId: string;
    market?: string;
    symbol?: string;
    channel?: "SNAPSHOT" | "KLINE" | "TICK" | "ORDER_BOOK";
    interval?: string;
  }): Promise<void> {
    const market = (input.market ?? marketDataQueryMarket.value)
      .trim()
      .toUpperCase();
    const symbol = (input.symbol ?? marketDataQuerySymbol.value)
      .trim()
      .toUpperCase();

    marketDataError.value = "";

    if (market === "" || symbol === "") {
      marketDataError.value =
        "Market and symbol are required to acquire a realtime subscription.";
      return;
    }

    isLoadingMarketData.value = true;

    try {
      marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              consumerId: input.consumerId,
              market,
              symbol,
              channel: input.channel ?? "SNAPSHOT",
              ...(input.interval == null
                ? {}
                : { interval: normalizeKlinePeriod(input.interval) }),
            }),
          },
        );
    } catch (error) {
      marketDataError.value =
        error instanceof Error
          ? error.message
          : "Failed to acquire market data subscription.";
    } finally {
      isLoadingMarketData.value = false;
    }
  }

  async function releaseMarketDataSubscription(input: {
    consumerId: string;
    market?: string;
    symbol?: string;
    channel?: "SNAPSHOT" | "KLINE" | "TICK" | "ORDER_BOOK";
    interval?: string;
    keepalive?: boolean;
  }): Promise<void> {
    const market = (input.market ?? marketDataQueryMarket.value)
      .trim()
      .toUpperCase();
    const symbol = (input.symbol ?? marketDataQuerySymbol.value)
      .trim()
      .toUpperCase();

    if (market === "" || symbol === "") {
      return;
    }

    try {
      marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions/release",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              consumerId: input.consumerId,
              market,
              symbol,
              channel: input.channel ?? "SNAPSHOT",
              ...(input.interval == null
                ? {}
                : { interval: normalizeKlinePeriod(input.interval) }),
            }),
            keepalive: input.keepalive ?? false,
          },
        );
    } catch (error) {
      marketDataError.value =
        error instanceof Error
          ? error.message
          : "Failed to release market data subscription.";
    }
  }

  async function heartbeatMarketDataConsumer(
    consumerId: string,
  ): Promise<void> {
    if (consumerId.trim() === "") {
      return;
    }

    try {
      marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions/heartbeat",
          {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ consumerId }),
          },
        );
    } catch (error) {
      marketDataError.value =
        error instanceof Error
          ? error.message
          : "Failed to heartbeat market data subscription.";
    }
  }

  async function subscribeCurrentMarketData(): Promise<void> {
    await acquireMarketDataSubscription({
      consumerId: "web:manual-market-data",
    });
  }

  async function unsubscribeAllMarketData(): Promise<void> {
    marketDataError.value = "";
    isLoadingMarketData.value = true;

    try {
      marketDataSubscriptions.value =
        await fetchEnvelopeWithInit<MarketDataSubscriptionsResponse>(
          "/api/v1/market-data/subscriptions",
          {
            method: "DELETE",
          },
        );
    } catch (error) {
      marketDataError.value =
        error instanceof Error
          ? error.message
          : "Failed to cancel market data subscriptions.";
    } finally {
      isLoadingMarketData.value = false;
    }
  }

  function mergeMarketDataCandles(
    current: MarketDataCandlesQueryResult | null,
    next: MarketDataCandlesQueryResult,
  ): MarketDataCandlesQueryResult {
    if (current == null) {
      return next;
    }

    const byKey = new Map(
      [...current.candles, ...next.candles].map((candle) => [
        `${candle.period}:${candle.at}`,
        candle,
      ]),
    );
    const candles = [...byKey.values()].sort(
      (left, right) =>
        new Date(left.at).getTime() - new Date(right.at).getTime(),
    );

    return {
      ...next,
      candles,
      totalReturned: candles.length,
    };
  }

  function isMarketDataTickLiveEvent(
    event: unknown,
  ): event is MarketDataTickLiveEvent {
    return (
      typeof event === "object" &&
      event !== null &&
      "type" in event &&
      event.type === "market-data.tick" &&
      "instrument" in event &&
      "snapshot" in event
    );
  }

  function applyMarketDataTickEvent(event: unknown): void {
    if (!isMarketDataTickLiveEvent(event)) {
      return;
    }

    const currentInstrument = normalizeInstrumentParts(
      {
        market: marketDataQueryMarket.value,
        symbol: marketDataQuerySymbol.value,
      },
      marketDataQueryMarket.value,
    );
    const currentInstrumentId =
      currentInstrument == null
        ? ""
        : `${currentInstrument.market}.${currentInstrument.symbol}`;
    if (event.instrument.instrumentId !== currentInstrumentId) {
      return;
    }

    marketDataSnapshot.value = {
      request: event.instrument,
      snapshot: event.snapshot,
      meta: {
        instrumentId: event.instrument.instrumentId,
        source: event.source,
        resolvedAt: event.at,
        fromCache: false,
      },
    };

    if (marketDataQueryPeriod.value !== "tick") {
      return;
    }

    const tickCandle = {
      period: "tick",
      open: event.snapshot.price,
      high: event.snapshot.price,
      low: event.snapshot.price,
      close: event.snapshot.price,
      volume: event.snapshot.volume,
      at: event.snapshot.at,
    };
    const existing = marketDataCandles.value;
    if (existing == null) {
      marketDataCandles.value = {
        request: {
          instrument: event.instrument,
          period: "tick",
          limit: marketDataQueryLimit.value,
        },
        candles: [tickCandle],
        totalReturned: 1,
        meta: {
          instrumentId: event.instrument.instrumentId,
          source: event.source,
          resolvedAt: event.at,
          fromCache: false,
        },
      };
      return;
    }

    marketDataCandles.value = mergeMarketDataCandles(existing, {
      ...existing,
      candles: [tickCandle],
      totalReturned: 1,
      meta: {
        ...existing.meta,
        source: event.source,
        resolvedAt: event.at,
        fromCache: false,
      },
    });
  }

  async function loadMarketDataQuery(
    options: LoadMarketDataQueryOptions = {},
  ): Promise<void> {
    const parsedInstrument = normalizeInstrumentParts(
      {
        market: marketDataQueryMarket.value,
        symbol: marketDataQuerySymbol.value,
      },
      marketDataQueryMarket.value,
    );
    const market = parsedInstrument?.market ?? "";
    const symbol = parsedInstrument?.symbol ?? "";
    const rawPeriod = marketDataQueryPeriod.value.trim();
    const requestedLimit = Number(marketDataQueryLimit.value);

    marketDataQueryError.value = "";

    if (market === "" || symbol === "" || rawPeriod === "") {
      marketDataQueryError.value =
        "Market, symbol and candle period are required.";
      return;
    }

    let period: string;
    try {
      period = normalizeKlinePeriod(rawPeriod);
    } catch (error) {
      marketDataQueryError.value =
        error instanceof Error ? error.message : "Unsupported candle period.";
      return;
    }

    if (!Number.isInteger(requestedLimit) || requestedLimit <= 0) {
      marketDataQueryError.value = "Candle limit must be a positive integer.";
      return;
    }

    const effectiveLimit =
      period === "tick"
        ? Math.max(requestedLimit, DEFAULT_TICK_QUERY_LIMIT)
        : requestedLimit;
    const effectiveFromTime =
      period === "tick" &&
      options.fromTime == null &&
      options.toTime == null &&
      options.appendOlder !== true
        ? new Date(Date.now() - DEFAULT_TICK_QUERY_LOOKBACK_MS).toISOString()
        : options.fromTime;

    const queryKey = JSON.stringify({
      market,
      symbol,
      period,
      limit: effectiveLimit,
      fromTime: effectiveFromTime ?? null,
      toTime: options.toTime ?? null,
      appendOlder: options.appendOlder === true,
    });
    if (activeMarketDataQuery?.key === queryKey) {
      return activeMarketDataQuery.promise;
    }

    const requestId = marketDataQueryRequestId + 1;
    marketDataQueryRequestId = requestId;

    let promise: Promise<void>;
    promise = (async (): Promise<void> => {
      marketDataQueryMarket.value = market;
      marketDataQuerySymbol.value = symbol;
      marketDataQueryPeriod.value = period;
      marketDataQueryLimit.value = requestedLimit;
      isLoadingMarketDataQuery.value = true;

      try {
        const encodedMarket = encodeURIComponent(market);
        const encodedSymbol = encodeURIComponent(symbol);
        const candleParams = new URLSearchParams({
          period,
          limit: String(effectiveLimit),
          refresh: "true",
        });
        if (effectiveFromTime != null) {
          candleParams.set("fromTime", effectiveFromTime);
        }
        if (options.toTime != null) {
          candleParams.set("toTime", options.toTime);
        }
        const [snapshotResult, candlesResult] = await Promise.allSettled([
          fetchEnvelope<MarketDataSnapshotQueryResult>(
            `/api/v1/market-data/snapshots/${encodedMarket}/${encodedSymbol}?refresh=true`,
          ),
          fetchEnvelope<MarketDataCandlesQueryResult>(
            `/api/v1/market-data/candles/${encodedMarket}/${encodedSymbol}?${candleParams.toString()}`,
          ),
        ]);

        marketDataSnapshot.value =
          snapshotResult.status === "fulfilled"
            ? snapshotResult.value
            : options.appendOlder === true
              ? marketDataSnapshot.value
              : null;
        marketDataCandles.value =
          candlesResult.status === "fulfilled"
            ? options.appendOlder === true
              ? mergeMarketDataCandles(
                  marketDataCandles.value,
                  candlesResult.value,
                )
              : candlesResult.value
            : options.appendOlder === true
              ? marketDataCandles.value
              : null;

        const partialErrors = [snapshotResult, candlesResult]
          .filter((result) => result.status === "rejected")
          .map((result) =>
            result.reason instanceof Error
              ? result.reason.message
              : "Failed to load part of market data query.",
          );
        if (partialErrors.length > 0) {
          marketDataQueryError.value = partialErrors.join(" / ");
        }
      } catch (error) {
        marketDataQueryError.value =
          error instanceof Error
            ? error.message
            : "Failed to load market data query.";
        if (options.appendOlder !== true) {
          marketDataSnapshot.value = null;
          marketDataCandles.value = null;
        }
      } finally {
        if (marketDataQueryRequestId === requestId) {
          isLoadingMarketDataQuery.value = false;
        }
        if (activeMarketDataQuery?.requestId === requestId) {
          activeMarketDataQuery = null;
        }
      }
    })();
    activeMarketDataQuery = { key: queryKey, promise, requestId };
    return promise;
  }

  async function loadSystemState(
    options: { background?: boolean; bypassCooldown?: boolean } = {},
  ): Promise<void> {
    const background = options.background === true;
    const now = Date.now();

    if (activeSystemStatePromise != null) {
      return activeSystemStatePromise;
    }

    if (
      !background &&
      options.bypassCooldown !== true &&
      now - lastForegroundSystemStateRefreshAt < 3000
    ) {
      return;
    }

    if (background && isBackgroundRefreshing) {
      return;
    }

    let resolveActiveSystemStatePromise: () => void = () => {};
    activeSystemStatePromise = new Promise<void>((resolve) => {
      resolveActiveSystemStatePromise = resolve;
    });

    if (background) {
      isBackgroundRefreshing = true;
    } else {
      lastForegroundSystemStateRefreshAt = now;
      isLoading.value = true;
      loadError.value = "";
    }

    try {
      const [
        status,
        overview,
        settingsSnapshot,
        realTradeApprovalSummary,
        realTradeHardStopSummary,
        realTradeHardStopEventSummary,
        realTradeKillSwitchStateSummary,
        realTradeKillSwitchSummary,
        realTradeRiskStateSummary,
        realTradeRiskSummary,
        workerBrokerUpdates,
        opendHealth,
        plugins,
        opendInstallGuide,
        instrumentReferenceSnapshot,
      ] = await Promise.all([
        fetchEnvelope<SystemStatusResponse>("/api/v1/system/status"),
        fetchEnvelope<StorageOverviewResponse>(
          "/api/v1/system/storage/overview",
        ),
        fetchEnvelope<BrokerSettingsResponse>("/api/v1/settings/brokers"),
        fetchEnvelope<RealTradeApprovalsResponse>(
          "/api/v1/system/real-trade-approvals",
        ),
        fetchEnvelope<RealTradeHardStopsResponse>(
          "/api/v1/system/real-trade-hard-stops",
        ),
        fetchEnvelope<RealTradeHardStopEventsResponse>(
          "/api/v1/system/real-trade-hard-stop-events",
        ),
        fetchEnvelope<RealTradeKillSwitchStateResponse>(
          "/api/v1/system/real-trade-kill-switch",
        ),
        fetchEnvelope<RealTradeKillSwitchEventsResponse>(
          "/api/v1/system/real-trade-kill-switch-events",
        ),
        fetchEnvelope<RealTradeRiskStateResponse>(
          "/api/v1/system/real-trade-risk-limits",
        ),
        fetchEnvelope<RealTradeRiskEventsResponse>(
          "/api/v1/system/real-trade-risk-events",
        ),
        fetchEnvelope<WorkerBrokerOrderUpdatesResponse>(
          "/api/v1/system/worker/broker-order-updates",
        ),
        fetchEnvelope<FutuOpenDHealthResponse>(
          "/api/v1/system/futu-opend",
        ).catch(() => emptyFutuOpenDHealth),
        fetchEnvelope<PluginCatalogResponse>("/api/v1/plugins"),
        fetchEnvelope<FutuOpenDInstallGuideResponse>(
          "/api/v1/system/futu-opend/install-guide",
        ).catch(() => emptyFutuOpenDInstallGuide),
        fetchEnvelope<MarketInstrumentReferenceResponse>(
          "/api/v1/market-data/instruments?limit=50",
        ).catch(() => ({
          query: "",
          totalReturned: 0,
          entries: [],
        })),
      ]);

      const activeBrokerId = resolveActiveBrokerId({
        settings: settingsSnapshot,
        status,
      });
      const broker = await fetchEnvelope<BrokerRuntimeResponse>(
        `/api/v1/brokers/${encodeURIComponent(activeBrokerId)}/runtime`,
      );

      systemStatus.value = status;
      storageOverview.value = overview;
      brokerSettings.value = settingsSnapshot;
      pluginCatalog.value = plugins;
      futuOpenDInstallGuide.value = opendInstallGuide;
      realTradeApprovals.value = realTradeApprovalSummary;
      realTradeHardStops.value = realTradeHardStopSummary;
      realTradeHardStopEvents.value = realTradeHardStopEventSummary;
      realTradeKillSwitchState.value = realTradeKillSwitchStateSummary;
      realTradeKillSwitchEvents.value = realTradeKillSwitchSummary;
      realTradeRiskState.value = realTradeRiskStateSummary;
      realTradeRiskEvents.value = realTradeRiskSummary;
      workerBrokerOrderUpdates.value = workerBrokerUpdates;
      futuOpenDHealth.value = opendHealth;
      brokerRuntime.value = broker;
      marketInstrumentReferences.value = instrumentReferenceSnapshot.entries;

      const brokerAccountOptions = resolveBrokerAccountOptions({
        activeBrokerId,
        settings: settingsSnapshot,
        runtime: broker,
        fallbackMarket:
          broker.descriptor.capabilities[0]?.market ??
          status.broker.capabilities[0]?.market ??
          "HK",
      });
      const selectedAccount = resolveSelectedBrokerAccountOption(
        brokerAccountOptions,
        activeBrokerId,
        status.defaultTradingEnvironment,
      );
      const nextSelectionKey = selectedAccount?.selectionKey ?? null;

      if (nextSelectionKey !== prefs.value.selectedBrokerAccountKey) {
        update({
          selectedBrokerAccountKey: nextSelectionKey,
        });
      }

      const brokerQuery = resolveBrokerQuery(selectedAccount).toString();
      const brokerIdForQueries = selectedAccount?.brokerId ?? activeBrokerId;
      const futuBrokerReadsPaused =
        broker.descriptor.id === "futu" &&
        (opendHealth.diagnosis.manualRetryRequired ||
          broker.session.connectivity !== "connected");
      const liveBrokerDataPromise: Promise<
        readonly [
          BrokerFundsResponse,
          BrokerPositionsResponse,
          BrokerOrdersResponse,
        ]
      > = futuBrokerReadsPaused
        ? Promise.resolve([
            emptyBrokerFunds,
            emptyBrokerPositions,
            emptyBrokerOrders,
          ] as const)
        : Promise.all([
            fetchEnvelope<BrokerFundsResponse>(
              `/api/v1/brokers/${encodeURIComponent(brokerIdForQueries)}/funds?${brokerQuery}`,
            ),
            fetchEnvelope<BrokerPositionsResponse>(
              `/api/v1/brokers/${encodeURIComponent(brokerIdForQueries)}/positions?${brokerQuery}`,
            ),
            fetchEnvelope<BrokerOrdersResponse>(
              `/api/v1/brokers/${encodeURIComponent(brokerIdForQueries)}/orders?${brokerQuery}`,
            ),
          ]).then(
            ([funds, positions, orders]) => [funds, positions, orders] as const,
          );
      const [
        [funds, positions, orders],
        projectedCashBalances,
        portfolio,
        cashReconciliation,
        reconciliation,
        executionOrderList,
      ] = await Promise.all([
        liveBrokerDataPromise,
        fetchEnvelope<PortfolioCashBalancesResponse>(
          `/api/v1/portfolio/${encodeURIComponent(brokerIdForQueries)}/cash-balances?${brokerQuery}`,
        ),
        fetchEnvelope<PortfolioPositionsResponse>(
          `/api/v1/portfolio/${encodeURIComponent(brokerIdForQueries)}/positions?${brokerQuery}`,
        ),
        fetchEnvelope<PortfolioCashReconciliationResponse>(
          `/api/v1/portfolio/${encodeURIComponent(brokerIdForQueries)}/cash-reconciliation?${brokerQuery}`,
        ),
        fetchEnvelope<PortfolioReconciliationResponse>(
          `/api/v1/portfolio/${encodeURIComponent(brokerIdForQueries)}/reconciliation?${brokerQuery}`,
        ),
        fetchEnvelope<ExecutionOrdersResponse>("/api/v1/execution/orders"),
      ]);

      brokerFunds.value = funds;
      brokerPositions.value = positions;
      brokerOrders.value = orders;
      portfolioCashBalances.value = projectedCashBalances;
      portfolioPositions.value = portfolio;
      portfolioCashReconciliation.value = cashReconciliation;
      portfolioReconciliation.value = reconciliation;
      executionOrders.value = executionOrderList;

      if (futuBrokerReadsPaused) {
        brokerCashFlows.value = emptyBrokerCashFlows;
      } else {
        await loadBrokerCashFlows({
          brokerQuery,
          brokerId: brokerIdForQueries,
          tradingEnvironment:
            funds.summary?.tradingEnvironment ??
            systemStatus.value.defaultTradingEnvironment,
          clearingDate: funds.checkedAt.slice(0, 10),
        });
      }

      const nextSelectedExecutionOrderId =
        executionOrderList.orders.find(
          (order) => order.internalOrderId === selectedExecutionOrderId.value,
        )?.internalOrderId ??
        executionOrderList.orders[0]?.internalOrderId ??
        "";

      if (nextSelectedExecutionOrderId !== "") {
        await loadExecutionOrderDetails(nextSelectedExecutionOrderId);
      } else {
        selectedExecutionOrderId.value = "";
        executionOrderEvents.value = emptyExecutionOrderEvents;
        executionEventsError.value = "";
        brokerOrderFees.value = emptyBrokerOrderFees;
        orderFeesError.value = "";
      }
    } catch (error) {
      if (background) {
        liveStreamStatus.value = "degraded";
      } else {
        loadError.value =
          error instanceof Error
            ? error.message
            : "Failed to load system status.";
      }
    } finally {
      if (background) {
        isBackgroundRefreshing = false;
      } else {
        isLoading.value = false;
      }
      resolveActiveSystemStatePromise();
      activeSystemStatePromise = null;
    }
  }

  function openConsoleStream(): void {
    if (typeof EventSource === "undefined") {
      return;
    }

    consoleStream?.close();
    consoleStream = new EventSource(buildApiUrl("/api/v1/streams/console"));

    consoleStream.onopen = () => {
      liveStreamStatus.value = "connected";
    };

    consoleStream.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as { checkedAt?: string };
        liveStreamCheckedAt.value = payload.checkedAt ?? "";
      } catch {
        liveStreamCheckedAt.value = "";
      }

      void loadSystemState({ background: true });
    };

    consoleStream.onerror = () => {
      liveStreamStatus.value = "degraded";
    };
  }

  async function initialize(): Promise<void> {
    if (initialized) {
      return;
    }

    initialized = true;
    await loadSystemState();
    openConsoleStream();
  }

  async function loadPlugins(): Promise<void> {
    pluginError.value = "";

    try {
      pluginCatalog.value =
        await fetchEnvelope<PluginCatalogResponse>("/api/v1/plugins");
    } catch (error) {
      pluginError.value =
        error instanceof Error ? error.message : "Failed to load plugins.";
    }
  }

  async function installPlugin(pluginId: string): Promise<void> {
    pluginError.value = "";
    installingPluginIds.value = Array.from(
      new Set([...installingPluginIds.value, pluginId]),
    );

    try {
      const response = await fetchEnvelopeWithInit<PluginInstallResponse>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/install`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({}),
        },
      );

      await pollPluginOperation(response.operation.operationId);
      await loadPlugins();
    } catch (error) {
      pluginError.value =
        error instanceof Error ? error.message : "Failed to install plugin.";
    } finally {
      installingPluginIds.value = installingPluginIds.value.filter(
        (id) => id !== pluginId,
      );
    }
  }

  async function uninstallPlugin(pluginId: string): Promise<void> {
    pluginError.value = "";
    uninstallingPluginIds.value = Array.from(
      new Set([...uninstallingPluginIds.value, pluginId]),
    );

    try {
      const response = await fetchEnvelopeWithInit<PluginInstallResponse>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/uninstall`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({}),
        },
      );

      await pollPluginOperation(response.operation.operationId);
      await loadPlugins();
    } catch (error) {
      pluginError.value =
        error instanceof Error ? error.message : "Failed to uninstall plugin.";
    } finally {
      uninstallingPluginIds.value = uninstallingPluginIds.value.filter(
        (id) => id !== pluginId,
      );
    }
  }

  async function loadPluginUninstallGuidance(
    pluginId: string,
  ): Promise<PluginUninstallGuidanceDto | null> {
    pluginError.value = "";

    try {
      return await fetchEnvelope<PluginUninstallGuidanceDto>(
        `/api/v1/plugins/${encodeURIComponent(pluginId)}/uninstall-guidance`,
      );
    } catch (error) {
      pluginError.value =
        error instanceof Error
          ? error.message
          : "Failed to load plugin uninstall guidance.";
      return null;
    }
  }

  async function pollPluginOperation(
    operationId: string,
  ): Promise<PluginOperationDto> {
    for (let attempt = 0; attempt < 30; attempt += 1) {
      const operation = await fetchEnvelope<PluginOperationDto>(
        `/api/v1/plugins/operations/${encodeURIComponent(operationId)}`,
      );

      if (operation.status === "SUCCEEDED" || operation.status === "FAILED") {
        if (operation.status === "FAILED") {
          throw new Error(operation.error ?? operation.message);
        }

        return operation;
      }

      await new Promise((resolve) => setTimeout(resolve, 500));
    }

    throw new Error("Plugin operation did not finish in time.");
  }

  function dispose(): void {
    consoleStream?.close();
    consoleStream = null;
    initialized = false;
  }

  function resolvePortfolioReconciliationStatusLabel(
    status: PortfolioReconciliationResponse["positions"][number]["status"],
  ): string {
    switch (status) {
      case "matched":
        return "已匹配";
      case "different":
        return "存在差异";
      case "missing-in-projection":
        return "内部缺失";
      case "missing-at-broker":
        return "券商缺失";
    }
  }

  function resolvePortfolioReconciliationTagType(
    status: PortfolioReconciliationResponse["positions"][number]["status"],
  ): "success" | "warning" | "danger" | "info" {
    switch (status) {
      case "matched":
        return "success";
      case "different":
        return "warning";
      case "missing-in-projection":
        return "danger";
      case "missing-at-broker":
        return "info";
    }
  }

  function resolveRealTradeHardStopScope(entry: {
    market: string | null;
    symbol: string | null;
    hardStopScope?: RealTradeHardStopScope | null;
  }): RealTradeHardStopScope {
    if (entry.hardStopScope != null) {
      return entry.hardStopScope;
    }

    if (entry.symbol != null) {
      return "SYMBOL";
    }

    if (entry.market != null) {
      return "MARKET";
    }

    return "ACCOUNT";
  }

  function formatRealTradeHardStopScope(entry: {
    market: string | null;
    symbol: string | null;
    hardStopScope?: RealTradeHardStopScope | null;
  }): string {
    const scope = resolveRealTradeHardStopScope(entry);

    switch (scope) {
      case "SYMBOL":
        return `SYMBOL / ${entry.market ?? "N/A"} / ${entry.symbol ?? "N/A"}`;
      case "MARKET":
        return `MARKET / ${entry.market ?? "N/A"}`;
      case "ACCOUNT":
        return "ACCOUNT";
    }
  }

  function resolveRealTradeHardStopScopeTagType(entry: {
    market: string | null;
    symbol: string | null;
    hardStopScope?: RealTradeHardStopScope | null;
  }): "info" | "warning" | "danger" {
    switch (resolveRealTradeHardStopScope(entry)) {
      case "ACCOUNT":
        return "info";
      case "MARKET":
        return "warning";
      case "SYMBOL":
        return "danger";
    }
  }

  function formatRealTradeKillSwitchSource(
    source: RealTradeKillSwitchStateResponse["killSwitchSource"],
  ): string {
    switch (source) {
      case "ENV":
        return "ENV";
      case "CONTROL_PLANE":
        return "CONTROL-PLANE";
      default:
        return "INACTIVE";
    }
  }

  function resolveRealTradeKillSwitchEventTagType(
    eventType: RealTradeKillSwitchEventsResponse["entries"][number]["eventType"],
  ): "success" | "warning" | "danger" {
    switch (eventType) {
      case "released":
        return "success";
      case "activated":
        return "warning";
      case "rejected":
        return "danger";
    }
  }

  function formatRealTradeRiskSource(
    source: RealTradeRiskStateResponse["riskConfigSource"],
  ): string {
    switch (source) {
      case "ENV":
        return "ENV";
      case "CONTROL_PLANE":
        return "CONTROL-PLANE";
      case "MERGED":
        return "MERGED";
      default:
        return "INACTIVE";
    }
  }

  function resolveRealTradeRiskEventTagType(
    eventType: RealTradeRiskEventsResponse["entries"][number]["eventType"],
  ): "success" | "warning" | "danger" {
    switch (eventType) {
      case "released":
        return "success";
      case "activated":
        return "warning";
      case "rejected":
        return "danger";
    }
  }

  function resolveWorkerBrokerSubscriptionTagType(
    status: WorkerBrokerOrderUpdatesResponse["subscriptions"][number]["status"],
  ): "success" | "warning" | "info" {
    switch (status) {
      case "active":
        return "success";
      case "retrying":
        return "warning";
      case "inactive":
        return "info";
    }
  }

  function formatDateTime(value: string | null | undefined): string {
    if (value == null || value === "") {
      return "N/A";
    }

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return value;
    }

    return new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
      timeZoneName: "short",
    }).format(date);
  }

  function formatDurationMs(value: number | null | undefined): string {
    if (value == null) {
      return "N/A";
    }

    if (value < 1000) {
      return `${value}ms`;
    }

    if (value < 60_000) {
      return `${Math.round(value / 1000)}s`;
    }

    if (value < 3_600_000) {
      return `${Math.round(value / 60_000)}m`;
    }

    return `${Math.round(value / 3_600_000)}h`;
  }

  function formatWorkerBrokerErrorContext(
    context: WorkerBrokerOrderUpdateErrorContext | null,
    fallback: string | null,
  ): string {
    return context?.summary ?? fallback ?? "No error context";
  }

  function resolveMarketInstrumentInput(
    value: string,
  ): { market: string; symbol: string } | null {
    return parseMarketInstrumentInput(value, prefs.value.market);
  }

  function resolveRealTradeApprovalDecisionTagType(
    decision: RealTradeApprovalsResponse["entries"][number]["decision"],
  ): "success" | "danger" {
    return decision === "approved" ? "success" : "danger";
  }

  return {
    availableBrokerAccounts,
    brokerCashFlows,
    brokerFunds,
    brokerOrderFees,
    brokerOrders,
    brokerPositions,
    brokerSettings,
    brokerRuntime,
    acquireMarketDataSubscription,
    applyMarketDataTickEvent,
    createManagedBrokerAccount,
    deleteManagedBrokerAccount,
    dispose,
    executionEventsError,
    executionOrderEvents,
    executionOrders,
    formatDateTime,
    formatDurationMs,
    formatRealTradeHardStopScope,
    formatRealTradeKillSwitchSource,
    formatRealTradeRiskSource,
    formatWorkerBrokerErrorContext,
    futuOpenDHealth,
    futuOpenDInstallGuide,
    initialize,
    heartbeatMarketDataConsumer,
    installPlugin,
    installingPluginIds,
    isLoading,
    isLoadingExecutionEvents,
    isLoadingMarketData,
    isLoadingMarketDataQuery,
    isLoadingOrderFees,
    liveStreamCheckedAt,
    liveStreamStatus,
    loadError,
    loadBrokerSettings,
    loadExecutionOrderDetails,
    loadMarketDataQuery,
    loadMarketInstrumentReferences,
    loadMarketDataSubscriptions,
    loadPluginUninstallGuidance,
    loadPlugins,
    loadSystemState,
    marketDataCandles,
    marketDataError,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    marketInstrumentReferences,
    marketInstrumentSearchOptions,
    marketDataSnapshot,
    marketDataSubscriptions,
    orderFeesError,
    portfolioCashBalances,
    portfolioCashReconciliation,
    portfolioPositions,
    portfolioReconciliation,
    pluginCatalog,
    pluginError,
    requestFutuOpenDManualRetry,
    realTradeApprovals,
    realTradeHardStopEvents,
    realTradeHardStops,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
    resolvePortfolioReconciliationStatusLabel,
    resolvePortfolioReconciliationTagType,
    resolveMarketInstrumentInput,
    resolveRealTradeApprovalDecisionTagType,
    resolveRealTradeHardStopScopeTagType,
    resolveRealTradeKillSwitchEventTagType,
    resolveRealTradeRiskEventTagType,
    resolveWorkerBrokerSubscriptionTagType,
    createStableWebConsumerId,
    saveBrokerIntegration,
    selectBrokerAccount,
    selectedBrokerAccount,
    selectedExecutionOrder,
    selectedExecutionOrderId,
    storageOverview,
    subscribeCurrentMarketData,
    systemStatus,
    releaseMarketDataSubscription,
    unsubscribeAllMarketData,
    uninstallPlugin,
    uninstallingPluginIds,
    updateManagedBrokerAccount,
    workerBrokerOrderUpdates,
  };
}

type ConsoleDataStore = ReturnType<typeof createConsoleDataStore>;

const consoleDataKey: InjectionKey<ConsoleDataStore> = Symbol("console-data");
let providedWorkspaceLayout: WorkspaceLayoutStore | null = null;

export const useConsoleDataStore = defineStore("console-data", () => {
  const workspaceLayout = providedWorkspaceLayout ?? useWorkspaceLayout();
  return {
    legacy: markRaw(createConsoleDataStore(workspaceLayout)),
  };
});

export function provideConsoleDataStore(
  workspaceLayout: WorkspaceLayoutStore,
): ConsoleDataStore {
  providedWorkspaceLayout = workspaceLayout;
  const store = useConsoleDataStore().legacy;
  provide(consoleDataKey, store);
  return store;
}

export function useConsoleData(): ConsoleDataStore {
  return inject(consoleDataKey, null) ?? useConsoleDataStore().legacy;
}
