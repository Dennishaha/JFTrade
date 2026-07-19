import type { Ref } from "vue";

import {
  type BrokerRuntimeResponse,
  type BrokerSettingsResponse,
  type ExecutionOrderEventsResponse,
  type ExecutionOrdersResponse,
  type FutuOpenDHealthResponse,
  type FutuOpenDInstallGuideResponse,
  type OnboardingStateResponse,
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
  emptyBrokerRuntime,
  emptyExecutionOrderEvents,
  emptyFutuOpenDHealth,
  emptyFutuOpenDInstallGuide,
  emptyOnboardingState,
  emptyRealTradeApprovals,
  emptyRealTradeHardStopEvents,
  emptyRealTradeHardStops,
  emptyRealTradeKillSwitchEvents,
  emptyRealTradeKillSwitchState,
  emptyRealTradeRiskEvents,
  emptySystemStatus,
} from "@/contracts";

import { apiGet, apiGetPath, fetchEnvelope } from "./apiClient";
import type { BrokerAccountSelectionOption } from "./consoleDataBrokerSettings";
import {
  resolveConsoleDataBrokerLiveSelection,
  resolveConsoleDataExecutionSelection,
} from "./consoleDataSystemStateDecisions";
import type { WorkspaceTradingPreferences } from "./useWorkspaceLayout";

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
  brokerMappings?: Array<{
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

function arrayOrEmpty<T>(items: T[] | null | undefined): T[] {
  return Array.isArray(items) ? items : [];
}

function arrayOrDefault<T>(items: T[] | null | undefined, fallback: T[]): T[] {
  return Array.isArray(items) ? items : [...fallback];
}

function normalizeRealTradeApprovals(
  response: RealTradeApprovalsResponse | null | undefined,
): RealTradeApprovalsResponse {
  const snapshot = response ?? emptyRealTradeApprovals;
  return {
    ...emptyRealTradeApprovals,
    ...snapshot,
    entries: arrayOrEmpty(snapshot.entries),
  };
}

function normalizeRealTradeHardStops(
  response: RealTradeHardStopsResponse | null | undefined,
): RealTradeHardStopsResponse {
  const snapshot = response ?? emptyRealTradeHardStops;
  return {
    ...emptyRealTradeHardStops,
    ...snapshot,
    blockedOperations: arrayOrDefault(
      snapshot.blockedOperations,
      emptyRealTradeHardStops.blockedOperations,
    ),
    entries: arrayOrEmpty(snapshot.entries),
  };
}

function normalizeRealTradeHardStopEvents(
  response: RealTradeHardStopEventsResponse | null | undefined,
): RealTradeHardStopEventsResponse {
  const snapshot = response ?? emptyRealTradeHardStopEvents;
  return {
    ...emptyRealTradeHardStopEvents,
    ...snapshot,
    blockedOperations: arrayOrDefault(
      snapshot.blockedOperations,
      emptyRealTradeHardStopEvents.blockedOperations,
    ),
    entries: arrayOrEmpty(snapshot.entries),
  };
}

function normalizeRealTradeKillSwitchState(
  response: RealTradeKillSwitchStateResponse | null | undefined,
): RealTradeKillSwitchStateResponse {
  const snapshot = response ?? emptyRealTradeKillSwitchState;
  return {
    ...emptyRealTradeKillSwitchState,
    ...snapshot,
    blockedOperations: arrayOrDefault(
      snapshot.blockedOperations,
      emptyRealTradeKillSwitchState.blockedOperations,
    ),
  };
}

function normalizeRealTradeKillSwitchEvents(
  response: RealTradeKillSwitchEventsResponse | null | undefined,
): RealTradeKillSwitchEventsResponse {
  const snapshot = response ?? emptyRealTradeKillSwitchEvents;
  return {
    ...emptyRealTradeKillSwitchEvents,
    ...snapshot,
    blockedOperations: arrayOrDefault(
      snapshot.blockedOperations,
      emptyRealTradeKillSwitchEvents.blockedOperations,
    ),
    entries: arrayOrEmpty(snapshot.entries),
  };
}

function normalizeRealTradeRiskEvents(
  response: RealTradeRiskEventsResponse | null | undefined,
): RealTradeRiskEventsResponse {
  const snapshot = response ?? emptyRealTradeRiskEvents;
  return {
    ...emptyRealTradeRiskEvents,
    ...snapshot,
    entries: arrayOrEmpty(snapshot.entries),
  };
}

function normalizeSystemStatus(
  response: SystemStatusResponse,
): SystemStatusResponse {
  const fallback = emptySystemStatus.observability.requests;
  const requests = response.observability?.requests ?? fallback;
  return {
    ...response,
    build: {
      ...emptySystemStatus.build,
      ...response.build,
    },
    observability: {
      ...response.observability,
      requests: {
        ...fallback,
        ...requests,
        recentErrors: arrayOrEmpty(requests.recentErrors),
        recentSlowRequests: arrayOrEmpty(requests.recentSlowRequests),
        openD: {
          ...fallback.openD,
          ...requests.openD,
        },
      },
    },
  };
}

interface CreateConsoleDataSystemStateControllerOptions {
  prefs: Ref<WorkspaceTradingPreferences>;
  update: (patch: Partial<WorkspaceTradingPreferences>) => void;
  systemStatus: Ref<SystemStatusResponse>;
  storageOverview: Ref<StorageOverviewResponse>;
  brokerSettings: Ref<BrokerSettingsResponse>;
  onboardingState: Ref<OnboardingStateResponse>;
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
  activeExecutionOrders: Ref<ExecutionOrdersResponse>;
  executionOrderEvents: Ref<ExecutionOrderEventsResponse>;
  selectedExecutionOrderId: Ref<string>;
  executionEventsError: Ref<string>;
  brokerOrderFees: Ref<typeof emptyBrokerOrderFees>;
  orderFeesError: Ref<string>;
  marketInstrumentReferences: Ref<MarketInstrumentReference[]>;
  isLoading: Ref<boolean>;
  loadError: Ref<string>;
  consoleRefreshError: Ref<string>;
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
      const referenceMarket = options.prefs.value.market.trim().toUpperCase();
      const referenceSymbol = options.prefs.value.symbol.trim().toUpperCase();
      const referenceParams = new URLSearchParams({
        query: `${referenceMarket}.${referenceSymbol}`,
        market: referenceMarket,
        limit: "5",
      });
      const [
        onboarding,
        statusPayload,
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
        plugins,
        opendInstallGuide,
        instrumentReferenceSnapshot,
      ] = await Promise.all([
        fetchEnvelope<OnboardingStateResponse>(
          "/api/v1/settings/onboarding",
        ).catch(() => emptyOnboardingState),
        fetchEnvelope<SystemStatusResponse>("/api/v1/system/status"),
        fetchEnvelope<StorageOverviewResponse>(
          "/api/v1/system/storage/overview",
        ),
        fetchEnvelope<BrokerSettingsResponse>("/api/v1/settings/brokers"),
        fetchEnvelope<RealTradeApprovalsResponse>(
          "/api/v1/system/real-trade-approvals",
        ),
        apiGet("/api/v1/system/real-trade-hard-stops"),
        apiGet("/api/v1/system/real-trade-hard-stop-events"),
        apiGet("/api/v1/system/real-trade-kill-switch"),
        apiGet("/api/v1/system/real-trade-kill-switch-events"),
        apiGet("/api/v1/system/real-trade-risk-limits"),
        apiGet("/api/v1/system/real-trade-risk-events"),
        fetchEnvelope<WorkerBrokerOrderUpdatesResponse>(
          "/api/v1/system/worker/broker-order-updates",
        ),
        options.fetchPluginCatalog(),
        fetchEnvelope<FutuOpenDInstallGuideResponse>(
          "/api/v1/system/futu-opend/install-guide",
        ).catch(() => emptyFutuOpenDInstallGuide),
        fetchEnvelope<MarketInstrumentReferenceResponse>(
          `/api/v1/market-data/instruments?${referenceParams.toString()}`,
        ).catch(() => ({
          query: `${referenceMarket}.${referenceSymbol}`,
          totalReturned: 0,
          entries: [],
        })),
      ]);
      const status = normalizeSystemStatus(statusPayload);
      const savedFutuIntegration =
        settingsSnapshot.brokers.find(
          (broker) => broker.descriptor.id === "futu",
        )?.integration ?? null;
      const activeBrokerId = options.resolveActiveBrokerId({
        settings: settingsSnapshot,
        status,
      });
      const shouldProbeFutu = savedFutuIntegration?.enabled === true;
      const opendHealth = shouldProbeFutu
        ? await fetchEnvelope<FutuOpenDHealthResponse>(
            "/api/v1/system/futu-opend",
          ).catch(() => emptyFutuOpenDHealth)
        : emptyFutuOpenDHealth;
      const broker =
        shouldProbeFutu && activeBrokerId === "futu"
          ? await apiGetPath<BrokerRuntimeResponse, "/api/v1/brokers/{brokerId}/runtime">(
              "/api/v1/brokers/{brokerId}/runtime",
              `/api/v1/brokers/${encodeURIComponent(activeBrokerId)}/runtime`,
            )
          : emptyBrokerRuntime;

      options.systemStatus.value = status;
      options.storageOverview.value = overview;
      options.brokerSettings.value = settingsSnapshot;
      options.onboardingState.value = onboarding;
      options.pluginCatalog.value = plugins;
      options.futuOpenDInstallGuide.value = opendInstallGuide;
      options.realTradeApprovals.value =
        normalizeRealTradeApprovals(realTradeApprovalSummary);
      options.realTradeHardStops.value =
        normalizeRealTradeHardStops(realTradeHardStopSummary);
      options.realTradeHardStopEvents.value =
        normalizeRealTradeHardStopEvents(realTradeHardStopEventSummary);
      options.realTradeKillSwitchState.value =
        normalizeRealTradeKillSwitchState(realTradeKillSwitchStateSummary);
      options.realTradeKillSwitchEvents.value =
        normalizeRealTradeKillSwitchEvents(realTradeKillSwitchSummary);
      options.realTradeRiskState.value = realTradeRiskStateSummary;
      options.realTradeRiskEvents.value =
        normalizeRealTradeRiskEvents(realTradeRiskSummary);
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

      const brokerQuery = options
        .resolveBrokerQuery(selectedAccount)
        .toString();
      if (shouldProbeFutu && activeBrokerId === "futu") {
        await options.loadBrokerLiveData({
          brokerId: brokerLiveSelection.brokerIdForQueries,
          brokerQuery,
          futuBrokerReadsPaused: brokerLiveSelection.futuBrokerReadsPaused,
        });
      }

      const executionSelection = resolveConsoleDataExecutionSelection({
        currentSelectedExecutionOrderId: options.selectedExecutionOrderId.value,
        executionOrders: options.activeExecutionOrders.value,
      });

      if (
        executionSelection.shouldResetExecutionDetails ||
        executionSelection.shouldClearExecutionDetails
      ) {
        resetExecutionDetails();
      }
      options.selectedExecutionOrderId.value =
        executionSelection.nextSelectedExecutionOrderId;
      if (background) {
        options.consoleRefreshError.value = "";
      }
    } catch (error) {
      if (background) {
        options.consoleRefreshError.value =
          error instanceof Error ? error.message : "控制台后台刷新失败。";
      } else {
        options.loadError.value =
          error instanceof Error ? error.message : "系统状态加载失败。";
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
