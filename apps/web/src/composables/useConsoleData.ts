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
  type PortfolioCashBalancesResponse,
  type PortfolioCashReconciliationResponse,
  type PortfolioPositionsResponse,
  type PortfolioReconciliationResponse,
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
    resolveMarketInstrumentInput,
    subscribeCurrentMarketData,
    unsubscribeAllMarketData,
  } = createConsoleDataMarketDataSlice({
    prefs,
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
      brokerFunds,
      brokerPositions,
      brokerOrders,
      executionOrders,
      loadPortfolioLiveData,
    });
  const { loadBrokerLiveData } = brokerLiveQueryController;
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
    resolveMarketInstrumentInput,
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
