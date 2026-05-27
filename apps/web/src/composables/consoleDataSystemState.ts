import type { Ref } from "vue";

import {
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  type FutuOpenDHealthResponse,
  type FutuOpenDInstallGuideResponse,
  type PluginCatalogResponse,
  type RealTradeApprovalsResponse,
  type RealTradeHardStopEventsResponse,
  type RealTradeHardStopsResponse,
  type RealTradeKillSwitchEventsResponse,
  type RealTradeKillSwitchStateResponse,
  type RealTradeRiskEventsResponse,
  type RealTradeRiskStateResponse,
  type StorageOverviewResponse,
  type SystemStatusResponse,
  type WorkerBrokerOrderUpdatesResponse,
  emptyBrokerOrderFees,
  emptyExecutionOrderEvents,
  emptyFutuOpenDHealth,
  emptyFutuOpenDInstallGuide,
} from "@jftrade/ui-contracts";

import { fetchEnvelope } from "./apiClient";
import type { BrokerAccountSelectionOption } from "./consoleDataBrokerSettings";
import {
  resolveConsoleDataBrokerLiveSelection,
  resolveConsoleDataExecutionSelection,
} from "./consoleDataSystemStateDecisions";
import type { WorkspacePreferences } from "./useWorkspaceLayout";

export interface MarketInstrumentReference {
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

export interface MarketInstrumentReferenceResponse {
  query: string;
  totalReturned: number;
  entries: MarketInstrumentReference[];
}

interface CreateConsoleDataSystemStateControllerOptions {
  prefs: Ref<WorkspacePreferences>;
  update: (patch: Partial<WorkspacePreferences>) => void;
  systemStatus: Ref<SystemStatusResponse>;
  storageOverview: Ref<StorageOverviewResponse>;
  brokerSettings: Ref<BrokerSettingsResponse>;
  pluginCatalog: Ref<PluginCatalogResponse>;
  futuOpenDHealth: Ref<FutuOpenDHealthResponse>;
  futuOpenDInstallGuide: Ref<FutuOpenDInstallGuideResponse>;
  realTradeApprovals: Ref<RealTradeApprovalsResponse>;
  realTradeHardStopEvents: Ref<RealTradeHardStopEventsResponse>;
  realTradeHardStops: Ref<RealTradeHardStopsResponse>;
  realTradeKillSwitchState: Ref<RealTradeKillSwitchStateResponse>;
  realTradeKillSwitchEvents: Ref<RealTradeKillSwitchEventsResponse>;
  realTradeRiskState: Ref<RealTradeRiskStateResponse>;
  realTradeRiskEvents: Ref<RealTradeRiskEventsResponse>;
  workerBrokerOrderUpdates: Ref<WorkerBrokerOrderUpdatesResponse>;
  brokerRuntime: Ref<BrokerRuntimeResponse>;
  executionOrders: Ref<ExecutionOrdersResponse>;
  executionOrderEvents: Ref<ExecutionOrderEventsResponse>;
  selectedExecutionOrderId: Ref<string>;
  executionEventsError: Ref<string>;
  brokerOrderFees: Ref<typeof emptyBrokerOrderFees>;
  orderFeesError: Ref<string>;
  marketInstrumentReferences: Ref<MarketInstrumentReference[]>;
  isLoading: Ref<boolean>;
  loadError: Ref<string>;
  liveStreamStatus: Ref<"disconnected" | "connected" | "degraded">;
  fetchPluginCatalog: () => Promise<PluginCatalogResponse>;
  resolveActiveBrokerId: (options?: {
    settings?: BrokerSettingsResponse;
    status?: SystemStatusResponse;
  }) => string;
  resolveBrokerAccountOptions: (options: {
    activeBrokerId: string;
    settings: BrokerSettingsResponse;
    runtime: BrokerRuntimeResponse;
    fallbackMarket: string;
  }) => BrokerAccountSelectionOption[];
  resolveSelectedBrokerAccountOption: (
    options: readonly BrokerAccountSelectionOption[],
    activeBrokerId: string,
    defaultTradingEnvironment: string,
  ) => BrokerAccountSelectionOption | null;
  resolveBrokerQuery: (
    selection: BrokerAccountSelectionOption | null,
  ) => URLSearchParams;
  loadBrokerLiveData: (options: {
    brokerId: string;
    brokerQuery: string;
    futuBrokerReadsPaused: boolean;
  }) => Promise<void>;
}

export function createConsoleDataSystemStateController(
  options: CreateConsoleDataSystemStateControllerOptions,
) {
  let isBackgroundRefreshing = false;
  let activeSystemStatePromise: Promise<void> | null = null;
  let lastForegroundSystemStateRefreshAt = 0;

  function resetExecutionDetails(): void {
    options.executionOrderEvents.value = emptyExecutionOrderEvents;
    options.executionEventsError.value = "";
    options.brokerOrderFees.value = emptyBrokerOrderFees;
    options.orderFeesError.value = "";
  }

  async function loadSystemState(
    loadOptions: { background?: boolean; bypassCooldown?: boolean } = {},
  ): Promise<void> {
    const background = loadOptions.background === true;
    const now = Date.now();

    if (activeSystemStatePromise != null) {
      return activeSystemStatePromise;
    }

    if (
      !background &&
      loadOptions.bypassCooldown !== true &&
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
      options.isLoading.value = true;
      options.loadError.value = "";
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
        options.fetchPluginCatalog(),
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

      const activeBrokerId = options.resolveActiveBrokerId({
        settings: settingsSnapshot,
        status,
      });
      const broker = await fetchEnvelope<BrokerRuntimeResponse>(
        `/api/v1/brokers/${encodeURIComponent(activeBrokerId)}/runtime`,
      );

      options.systemStatus.value = status;
      options.storageOverview.value = overview;
      options.brokerSettings.value = settingsSnapshot;
      options.pluginCatalog.value = plugins;
      options.futuOpenDInstallGuide.value = opendInstallGuide;
      options.realTradeApprovals.value = realTradeApprovalSummary;
      options.realTradeHardStops.value = realTradeHardStopSummary;
      options.realTradeHardStopEvents.value = realTradeHardStopEventSummary;
      options.realTradeKillSwitchState.value = realTradeKillSwitchStateSummary;
      options.realTradeKillSwitchEvents.value = realTradeKillSwitchSummary;
      options.realTradeRiskState.value = realTradeRiskStateSummary;
      options.realTradeRiskEvents.value = realTradeRiskSummary;
      options.workerBrokerOrderUpdates.value = workerBrokerUpdates;
      options.futuOpenDHealth.value = opendHealth;
      options.brokerRuntime.value = broker;
      options.marketInstrumentReferences.value =
        instrumentReferenceSnapshot.entries;

      const brokerAccountOptions = options.resolveBrokerAccountOptions({
        activeBrokerId,
        settings: settingsSnapshot,
        runtime: broker,
        fallbackMarket:
          broker.descriptor.capabilities[0]?.market ??
          status.broker.capabilities[0]?.market ??
          "HK",
      });
      const selectedAccount = options.resolveSelectedBrokerAccountOption(
        brokerAccountOptions,
        activeBrokerId,
        status.defaultTradingEnvironment,
      );
      const brokerLiveSelection = resolveConsoleDataBrokerLiveSelection({
        activeBrokerId,
        selectedAccount,
        runtime: broker,
        opendHealth,
      });

      if (
        brokerLiveSelection.nextSelectedBrokerAccountKey !==
        options.prefs.value.selectedBrokerAccountKey
      ) {
        options.update({
          selectedBrokerAccountKey:
            brokerLiveSelection.nextSelectedBrokerAccountKey,
        });
      }

      const brokerQuery = options.resolveBrokerQuery(selectedAccount).toString();
      await options.loadBrokerLiveData({
        brokerId: brokerLiveSelection.brokerIdForQueries,
        brokerQuery,
        futuBrokerReadsPaused: brokerLiveSelection.futuBrokerReadsPaused,
      });

      const executionSelection = resolveConsoleDataExecutionSelection({
        currentSelectedExecutionOrderId: options.selectedExecutionOrderId.value,
        executionOrders: options.executionOrders.value,
      });

      if (
        executionSelection.shouldResetExecutionDetails ||
        executionSelection.shouldClearExecutionDetails
      ) {
        resetExecutionDetails();
      }
      options.selectedExecutionOrderId.value =
        executionSelection.nextSelectedExecutionOrderId;
    } catch (error) {
      if (background) {
        options.liveStreamStatus.value = "degraded";
      } else {
        options.loadError.value =
          error instanceof Error
            ? error.message
            : "系统状态加载失败。";
      }
    } finally {
      if (background) {
        isBackgroundRefreshing = false;
      } else {
        options.isLoading.value = false;
      }
      resolveActiveSystemStatePromise();
      activeSystemStatePromise = null;
    }
  }

  return {
    loadSystemState,
  };
}