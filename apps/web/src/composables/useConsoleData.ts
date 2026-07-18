import { defineStore } from "pinia";
import {
  type InjectionKey,
  computed,
  inject,
  markRaw,
  provide,
  ref,
  watch,
} from "vue";

import {
  type BrokerCashFlowsResponse,
  type BrokerFillsResponse,
  type BrokerFundsResponse,
  type BrokerMarginRatiosResponse,
  type BrokerMaxTradeQuantityResponse,
  type BrokerOrderFeesResponse,
  type BrokerOrdersResponse,
  type BrokerPositionsResponse,
  type BrokerReadFeatureCapability,
  type BrokerReadFeatureKey,
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  type FutuOpenDHealthResponse,
  type FutuOpenDInstallGuideResponse,
  type MarketDataSubscriptionsResponse,
  type OnboardingStateResponse,
  type PluginCatalogResponse,
  type PortfolioCashBalancesResponse,
  type PortfolioCashReconciliationResponse,
  type PortfolioPositionsResponse,
  type PortfolioReconciliationResponse,
  type StorageOverviewResponse,
  type SystemStatusResponse,
  type WorkerBrokerOrderUpdateErrorContext,
  type WorkerBrokerOrderUpdatesResponse,
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
  emptyMarketDataSubscriptions,
  emptyOnboardingState,
  emptyPluginCatalog,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@/contracts";

import { fetchEnvelopeWithInit } from "./apiClient";
import {
  createConsoleDataBrokerSettingsController,
} from "./consoleDataBrokerSettings";
import {
  createConsoleDataBrokerLiveQueryController,
} from "./consoleDataBrokerLiveQuery";
import {
  createConsoleDataConsoleStreamController,
} from "./consoleDataConsoleStream";
import {
  createConsoleDataExecutionOrdersController,
} from "./consoleDataExecutionOrders";
import {
  createConsoleDataMarketDataSlice,
} from "./consoleDataMarketData";
import {
  createConsoleDataMarketDataQuerySlice,
} from "./consoleDataMarketDataQuery";
import { createStableWebConsumerId } from "./consoleDataMarketSubscriptions";
import {
  createConsoleDataPluginController,
} from "./consoleDataPlugins";
import {
  createConsoleDataPortfolioLiveQueryController,
} from "./consoleDataPortfolioLiveQuery";
import {
  createConsoleDataSystemStateController,
  type MarketInstrumentReference,
} from "./consoleDataSystemState";
import {
  createConsoleDataRealTradeController,
} from "./consoleDataRealTrade";
import {
  type LoadMarketDataQueryOptions,
} from "./marketDataQuery";
import { getSharedLiveSocketHub } from "./sharedLiveSocket";
import { normalizeKlinePeriod } from "../charting/kline";
import {
  useWorkspaceTradingPrefs,
  type WorkspaceTradingPreferencesStore,
} from "./useWorkspaceLayout";

export type {
  MarketInstrumentSearchOption,
} from "./consoleDataMarketInstruments";

interface BrokerReadFeatureQueryRequirements {
  capability: BrokerReadFeatureCapability | null;
  supported: boolean;
  supportsHistory: boolean;
  requiresSymbols: boolean;
  requiresClearingDate: boolean;
  requiresPrice: boolean;
  requiresOrderIdEx: boolean;
}

function createConsoleDataStore(
  workspaceTradingPrefs: WorkspaceTradingPreferencesStore,
) {
  const liveHub = getSharedLiveSocketHub();
  const activeInstrumentOwnerId = liveHub.createOwnerId("active-market-instrument");
  const { prefs, update } = workspaceTradingPrefs;
  const systemStatus = ref<SystemStatusResponse>(emptySystemStatus);
  const storageOverview = ref<StorageOverviewResponse>(emptyStorageOverview);
  const brokerSettings = ref<BrokerSettingsResponse>(emptyBrokerSettings);
  const onboardingState = ref<OnboardingStateResponse>(emptyOnboardingState);
  const pluginCatalog = ref<PluginCatalogResponse>(emptyPluginCatalog);
  const futuOpenDHealth = ref<FutuOpenDHealthResponse>(emptyFutuOpenDHealth);
  const futuOpenDInstallGuide = ref<FutuOpenDInstallGuideResponse>(
    emptyFutuOpenDInstallGuide,
  );
  const pluginError = ref("");
  const installingPluginIds = ref<string[]>([]);
  const uninstallingPluginIds = ref<string[]>([]);
  const workerBrokerOrderUpdates = ref<WorkerBrokerOrderUpdatesResponse>(
    emptyWorkerBrokerOrderUpdates,
  );
  const brokerRuntime = ref<BrokerRuntimeResponse>(emptyBrokerRuntime);
  const brokerCashFlows = ref<BrokerCashFlowsResponse>(emptyBrokerCashFlows);
  const brokerFills = ref<BrokerFillsResponse>(emptyBrokerFills);
  const brokerOrderFees = ref<BrokerOrderFeesResponse>(emptyBrokerOrderFees);
  const brokerFunds = ref<BrokerFundsResponse>(emptyBrokerFunds);
  const brokerMarginRatios = ref<BrokerMarginRatiosResponse>(
    emptyBrokerMarginRatios,
  );
  const brokerMaxTradeQuantity = ref<BrokerMaxTradeQuantityResponse>(
    emptyBrokerMaxTradeQuantity,
  );
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
  const activeExecutionOrders = ref<ExecutionOrdersResponse>(emptyExecutionOrders);
  const historicalExecutionOrders = ref<ExecutionOrdersResponse>(emptyExecutionOrders);
  const executionOrderEvents = ref<ExecutionOrderEventsResponse>(
    emptyExecutionOrderEvents,
  );
  const selectedExecutionOrderId = ref("");
  const isLoadingBrokerOrders = ref(false);
  const isLoadingHistoricalOrders = ref(false);
  const historicalOrdersError = ref("");
  const isLoadingExecutionEvents = ref(false);
  const isLoadingBrokerFills = ref(false);
  const isLoadingBrokerMarginRatios = ref(false);
  const isLoadingBrokerMaxTradeQuantity = ref(false);
  const isLoadingOrderFees = ref(false);
  const executionEventsError = ref("");
  const orderFeesError = ref("");
  const isLoading = ref(true);
  const loadError = ref("");
  const liveStreamStatus = ref<"disconnected" | "connected" | "degraded">(
    "disconnected",
  );
  const liveStreamCheckedAt = ref("");
  const consoleRefreshError = ref("");

  const {
    applyMarketDataTickEvent,
    activeMarketDataInstrumentId,
    currentMarketDataCandles,
    currentMarketDataSnapshot,
    currentMarketSecurityDetails,
    disposeMarketDataQuery,
    isMarketDataStale,
    isLoadingMarketDataQuery,
    isMarketDataSwitching,
    lastDataRefreshedAt,
    loadMarketDataQuery,
    marketDataCandles,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    marketSecurityDetails,
    marketDataSnapshot,
    selectMarketDataInstrument,
  } = createConsoleDataMarketDataQuerySlice();
  selectMarketDataInstrument({
    market: prefs.value.market,
    symbol: prefs.value.symbol,
    period: prefs.value.period,
  });
  let systemStateController!: ReturnType<
    typeof createConsoleDataSystemStateController
  >;
  const reloadSystemState = (options?: {
    background?: boolean;
    bypassCooldown?: boolean;
  }) => systemStateController.loadSystemState(options);
  const brokerSettingsController =
    createConsoleDataBrokerSettingsController({
      prefs,
      update,
      brokerSettings,
      brokerRuntime,
      systemStatus,
      reloadSystemState,
    });
  const {
    availableBrokerAccounts,
    createManagedBrokerAccount,
    deleteManagedBrokerAccount,
    loadBrokerSettings,
    requestFutuOpenDManualRetry,
    resolveActiveBrokerId,
    resolveBrokerAccountOptions,
    resolveBrokerQuery,
    resolveSelectedBrokerAccountOption,
    saveBrokerIntegration,
    selectBrokerAccount,
    selectedBrokerAccount,
    updateManagedBrokerAccount,
  } = brokerSettingsController;

  function resolveBrokerMarketCapability(market?: string | null) {
    const normalizedMarket = market?.trim().toUpperCase() ?? "";
    const descriptors = [brokerRuntime.value.descriptor, systemStatus.value.broker];

    for (const descriptor of descriptors) {
      if (descriptor.capabilities.length === 0) {
        continue;
      }
      if (normalizedMarket !== "") {
        const matched = descriptor.capabilities.find(
          (capability) => capability.market.trim().toUpperCase() === normalizedMarket,
        );
        if (matched != null) {
          return matched;
        }
      }
      return descriptor.capabilities[0] ?? null;
    }

    return null;
  }

  function resolveBrokerReadFeatureCapability(
    feature: BrokerReadFeatureKey,
    context?: {
      market?: string | null;
      tradingEnvironment?: string | null;
    },
  ): BrokerReadFeatureCapability | null {
    const capability = resolveBrokerMarketCapability(context?.market);
    const featureCapability = capability?.readFeatures?.[feature];
    if (featureCapability == null) {
      return null;
    }

    const normalizedEnvironment =
      context?.tradingEnvironment?.trim().toUpperCase() ?? "";
    if (normalizedEnvironment === "") {
      return featureCapability;
    }

    return featureCapability.supportedEnvironments.some(
      (environment) => environment.trim().toUpperCase() === normalizedEnvironment,
    )
      ? featureCapability
      : null;
  }

  function resolveBrokerReadFeatureQueryRequirements(
    feature: BrokerReadFeatureKey,
    context?: {
      market?: string | null;
      tradingEnvironment?: string | null;
    },
  ): BrokerReadFeatureQueryRequirements {
    const capability = resolveBrokerReadFeatureCapability(feature, context);

    return {
      capability,
      supported: capability != null,
      supportsHistory: capability?.supportsHistory === true,
      requiresSymbols:
        capability?.requiresSymbols ?? feature === "marginRatios",
      requiresClearingDate:
        capability?.requiresClearingDate ?? feature === "cashFlows",
      requiresPrice:
        capability?.requiresPrice ?? feature === "maxTradeQuantity",
      requiresOrderIdEx:
        capability?.requiresOrderIdEx ?? feature === "orderFees",
    };
  }

  function supportsBrokerReadFeature(
    feature: BrokerReadFeatureKey,
    context?: {
      market?: string | null;
      tradingEnvironment?: string | null;
    },
  ): boolean {
    return resolveBrokerReadFeatureCapability(feature, context) != null;
  }

  const {
    acquireMarketDataSubscription,
    heartbeatMarketDataConsumer,
    isLoadingMarketData,
    loadMarketDataSubscriptions,
    loadMarketInstrumentReferences,
    marketDataError,
    marketDataSubscriptions,
    marketInstrumentReferences,
    marketInstrumentSearchOptions,
    releaseMarketDataSubscription,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  } = createConsoleDataMarketDataSlice({
    marketDataQueryMarket,
    marketDataQuerySymbol,
    selectedBrokerAccount,
    portfolioPositions,
    brokerPositions,
    brokerOrders,
    activeExecutionOrders,
  });
  const pluginController = createConsoleDataPluginController({
    pluginCatalog,
    pluginError,
    installingPluginIds,
    uninstallingPluginIds,
  });
  const {
    fetchPluginCatalog,
    installPlugin,
    loadPluginUninstallGuidance,
    loadPlugins,
    uninstallPlugin,
  } = pluginController;
  const executionOrdersController = createConsoleDataExecutionOrdersController({
    activeExecutionOrders,
    historicalExecutionOrders,
    executionOrderEvents,
    selectedExecutionOrderId,
    isLoadingExecutionEvents,
    isLoadingOrderFees,
    executionEventsError,
    brokerOrderFees,
    orderFeesError,
    resolveBrokerReadFeatureQueryRequirements,
    supportsBrokerReadFeature,
  });
  const { loadExecutionOrderDetails, selectedExecutionOrder } =
    executionOrdersController;
  const realTradeController = createConsoleDataRealTradeController();
  const {
    realTradeApprovals,
    realTradeHardStopEvents,
    realTradeHardStops,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
  } = realTradeController;
  const portfolioLiveQueryController =
    createConsoleDataPortfolioLiveQueryController({
      portfolioCashBalances,
      portfolioCashReconciliation,
      portfolioPositions,
      portfolioReconciliation,
    });
  const { loadPortfolioLiveData } = portfolioLiveQueryController;
  const brokerLiveQueryController =
    createConsoleDataBrokerLiveQueryController({
      systemStatus,
      brokerCashFlows,
      brokerFills,
      brokerFunds,
      brokerMarginRatios,
      brokerMaxTradeQuantity,
      brokerPositions,
      brokerOrders,
      activeExecutionOrders,
      historicalExecutionOrders,
      isLoadingBrokerOrders,
      isLoadingHistoricalOrders,
      historicalOrdersError,
      isLoadingBrokerFills,
      isLoadingBrokerMarginRatios,
      isLoadingBrokerMaxTradeQuantity,
      resolveBrokerReadFeatureQueryRequirements,
      supportsBrokerReadFeature,
      loadPortfolioLiveData,
    });
  const {
    clearBrokerMaxTradeQuantity,
    loadBrokerLiveData,
    loadBrokerMaxTradeQuantity,
    loadHistoricalExecutionOrders,
  } = brokerLiveQueryController;
  systemStateController = createConsoleDataSystemStateController({
    prefs,
    update,
    systemStatus,
    storageOverview,
    brokerSettings,
    onboardingState,
    pluginCatalog,
    futuOpenDHealth,
    futuOpenDInstallGuide,
    realTradeApprovals,
    realTradeHardStopEvents,
    realTradeHardStops,
    realTradeKillSwitchState,
    realTradeKillSwitchEvents,
    realTradeRiskState,
    realTradeRiskEvents,
    workerBrokerOrderUpdates,
    brokerRuntime,
    activeExecutionOrders,
    executionOrderEvents,
    selectedExecutionOrderId,
    executionEventsError,
    brokerOrderFees,
    orderFeesError,
    marketInstrumentReferences,
    isLoading,
    loadError,
    consoleRefreshError,
    fetchPluginCatalog,
    resolveActiveBrokerId,
    resolveBrokerAccountOptions,
    resolveSelectedBrokerAccountOption,
    resolveBrokerQuery,
    loadBrokerLiveData,
  });
  const { loadSystemState } = systemStateController;
  async function loadOnboardingState(): Promise<OnboardingStateResponse> {
    const response = await fetchEnvelopeWithInit<OnboardingStateResponse>(
      "/api/v1/settings/onboarding",
      { method: "GET" },
    );
    onboardingState.value = response;
    return response;
  }

  async function saveOnboardingState(payload: {
    completed: boolean;
    dismissed?: boolean;
    lastBrokerId?: string;
  }): Promise<void> {
    const response = await fetchEnvelopeWithInit<OnboardingStateResponse>(
      "/api/v1/settings/onboarding",
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      },
    );
    onboardingState.value = response;
  }
  const consoleStreamController = createConsoleDataConsoleStreamController({
    liveStreamStatus,
    liveStreamCheckedAt,
    reloadSystemState,
  });
  const { dispose: disposeConsoleStream, initialize } = consoleStreamController;

  function dispose(): void {
    disposeMarketDataQuery();
    disposeConsoleStream();
  }

  watch(
    activeMarketDataInstrumentId,
    (instrumentId) => {
      liveHub.setActiveInstrument(activeInstrumentOwnerId, instrumentId || null);
    },
    { immediate: true },
  );

  function selectWorkspaceInstrument(input: {
    market: string;
    symbol: string;
    period?: string;
  }): void {
    const period =
      input.period == null ? prefs.value.period : normalizeKlinePeriod(input.period);
    selectMarketDataInstrument({
      market: input.market,
      symbol: input.symbol,
      period,
    });
    update({
      market: marketDataQueryMarket.value,
      symbol: marketDataQuerySymbol.value,
      period,
      marketSegment: "securities",
      productClass: "unknown",
    });
  }

  return {
    availableBrokerAccounts,
    brokerCashFlows,
    brokerFills,
    brokerFunds,
    brokerMarginRatios,
    brokerMaxTradeQuantity,
    brokerOrderFees,
    brokerOrders,
    brokerPositions,
    brokerSettings,
    brokerRuntime,
    acquireMarketDataSubscription,
    activeMarketDataInstrumentId,
    applyMarketDataTickEvent,
    createManagedBrokerAccount,
    deleteManagedBrokerAccount,
    dispose,
    executionEventsError,
    executionOrderEvents,
    activeExecutionOrders,
    historicalExecutionOrders,
    futuOpenDHealth,
    futuOpenDInstallGuide,
    initialize,
    heartbeatMarketDataConsumer,
    installPlugin,
    installingPluginIds,
    isLiveStreamConnected: computed(() => liveStreamStatus.value === "connected"),
    isMarketDataStale,
    isLoading,
    lastDataRefreshedAt,
    isLoadingBrokerOrders,
    isLoadingHistoricalOrders,
    historicalOrdersError,
    isLoadingBrokerFills,
    isLoadingBrokerMarginRatios,
    isLoadingBrokerMaxTradeQuantity,
    isLoadingExecutionEvents,
    isLoadingMarketData,
    isLoadingMarketDataQuery,
    isMarketDataSwitching,
    isLoadingOrderFees,
    consoleRefreshError,
    liveStreamCheckedAt,
    liveStreamStatus,
    loadError,
    loadBrokerSettings,
    loadBrokerMaxTradeQuantity,
    loadExecutionOrderDetails,
    loadHistoricalExecutionOrders,
    loadMarketDataQuery,
    loadMarketInstrumentReferences,
    loadMarketDataSubscriptions,
    loadOnboardingState,
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
    marketSecurityDetails,
    currentMarketDataCandles,
    currentMarketDataSnapshot,
    currentMarketSecurityDetails,
    marketInstrumentReferences,
    marketInstrumentSearchOptions,
    marketDataSnapshot,
    marketDataSubscriptions,
    onboardingState,
    orderFeesError,
    portfolioCashBalances,
    portfolioCashReconciliation,
    portfolioPositions,
    portfolioReconciliation,
    pluginCatalog,
    pluginError,
    requestFutuOpenDManualRetry,
    clearBrokerMaxTradeQuantity,
    realTradeApprovals,
    realTradeHardStopEvents,
    realTradeHardStops,
    realTradeKillSwitchEvents,
    realTradeKillSwitchState,
    realTradeRiskEvents,
    realTradeRiskState,
    createStableWebConsumerId,
    saveBrokerIntegration,
    saveOnboardingState,
    selectWorkspaceInstrument,
    selectBrokerAccount,
    selectedBrokerAccount,
    selectedExecutionOrder,
    selectedExecutionOrderId,
    resolveBrokerReadFeatureCapability,
    resolveBrokerReadFeatureQueryRequirements,
    storageOverview,
    supportsBrokerReadFeature,
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
let providedWorkspaceTradingPrefs: WorkspaceTradingPreferencesStore | null = null;

export const useConsoleDataStore = defineStore("console-data", () => {
  const workspaceTradingPrefs =
    providedWorkspaceTradingPrefs ?? useWorkspaceTradingPrefs();
  const legacy = markRaw(
    createConsoleDataStore(workspaceTradingPrefs),
  ) as ConsoleDataStore;
  return {
    legacy,
  };
});

export function provideConsoleDataStore(
  workspaceTradingPrefs: WorkspaceTradingPreferencesStore,
): ConsoleDataStore {
  providedWorkspaceTradingPrefs = workspaceTradingPrefs;
  const store = useConsoleDataStore().legacy as unknown as ConsoleDataStore;
  provide(consoleDataKey, store);
  return store;
}

export function useConsoleData(): ConsoleDataStore {
  return (
    inject(consoleDataKey, null) ??
    (useConsoleDataStore().legacy as unknown as ConsoleDataStore)
  );
}
