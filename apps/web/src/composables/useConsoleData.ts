import { defineStore } from "pinia";
import {
  type InjectionKey,
  inject,
  markRaw,
  provide,
  ref,
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
  emptyPluginCatalog,
  emptyPortfolioCashBalances,
  emptyPortfolioCashReconciliation,
  emptyPortfolioPositions,
  emptyPortfolioReconciliation,
  emptyStorageOverview,
  emptySystemStatus,
  emptyWorkerBrokerOrderUpdates,
} from "@jftrade/ui-contracts";

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
import {
  useWorkspaceLayout,
  type WorkspaceLayoutStore,
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
  const executionOrders = ref<ExecutionOrdersResponse>(emptyExecutionOrders);
  const executionOrderEvents = ref<ExecutionOrderEventsResponse>(
    emptyExecutionOrderEvents,
  );
  const selectedExecutionOrderId = ref("");
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

  const {
    applyMarketDataTickEvent,
    isLoadingMarketDataQuery,
    loadMarketDataQuery,
    marketDataCandles,
    marketDataQueryError,
    marketDataQueryLimit,
    marketDataQueryMarket,
    marketDataQueryPeriod,
    marketDataQuerySymbol,
    marketSecurityDetails,
    marketDataSnapshot,
  } = createConsoleDataMarketDataQuerySlice();
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
    executionOrders,
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
    executionOrders,
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
      executionOrders,
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
  } = brokerLiveQueryController;
  systemStateController = createConsoleDataSystemStateController({
    prefs,
    update,
    systemStatus,
    storageOverview,
    brokerSettings,
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
    executionOrders,
    executionOrderEvents,
    selectedExecutionOrderId,
    executionEventsError,
    brokerOrderFees,
    orderFeesError,
    marketInstrumentReferences,
    isLoading,
    loadError,
    liveStreamStatus,
    fetchPluginCatalog,
    resolveActiveBrokerId,
    resolveBrokerAccountOptions,
    resolveSelectedBrokerAccountOption,
    resolveBrokerQuery,
    loadBrokerLiveData,
  });
  const { loadSystemState } = systemStateController;
  const consoleStreamController = createConsoleDataConsoleStreamController({
    liveStreamStatus,
    liveStreamCheckedAt,
    reloadSystemState,
  });
  const { dispose, initialize } = consoleStreamController;

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
    applyMarketDataTickEvent,
    createManagedBrokerAccount,
    deleteManagedBrokerAccount,
    dispose,
    executionEventsError,
    executionOrderEvents,
    executionOrders,
    futuOpenDHealth,
    futuOpenDInstallGuide,
    initialize,
    heartbeatMarketDataConsumer,
    installPlugin,
    installingPluginIds,
    isLoading,
    isLoadingBrokerFills,
    isLoadingBrokerMarginRatios,
    isLoadingBrokerMaxTradeQuantity,
    isLoadingExecutionEvents,
    isLoadingMarketData,
    isLoadingMarketDataQuery,
    isLoadingOrderFees,
    liveStreamCheckedAt,
    liveStreamStatus,
    loadError,
    loadBrokerSettings,
    loadBrokerMaxTradeQuantity,
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
    marketSecurityDetails,
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
let providedWorkspaceLayout: WorkspaceLayoutStore | null = null;

export const useConsoleDataStore = defineStore("console-data", () => {
  const workspaceLayout = providedWorkspaceLayout ?? useWorkspaceLayout();
  const legacy = markRaw(createConsoleDataStore(workspaceLayout)) as ConsoleDataStore;
  return {
    legacy,
  };
});

export function provideConsoleDataStore(
  workspaceLayout: WorkspaceLayoutStore,
): ConsoleDataStore {
  providedWorkspaceLayout = workspaceLayout;
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
