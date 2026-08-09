import {
  computed,
  inject,
  onMounted,
  ref,
  watch,
  type InjectionKey,
} from "vue";
import { useRoute, useRouter } from "vue-router";

import {
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
} from "@/charting/kline";
import {
  formatBacktestRehabType,
  formatBacktestRunDate,
  formatBacktestTimestamp,
  isTerminalBacktestStatus,
} from "@/components/backtest/backtestRunPresentation";
import { ApiClientError, apiGet, apiGetPath } from "@/composables/shared/apiClient";
import { statusTone } from "@/composables/shared/statusTone";
import { categoryMarketForUser } from "@/composables/market-data/instrumentPresentation";
import { useMarketProfiles } from "@/composables/market-data/marketProfiles";
import { queryClient, queryKeys } from "@/composables/settings/serverState";
import { useBacktestRuns } from "@/composables/backtest/useBacktestRuns";
import {
  BACKTEST_BROKER_FEE_MODE_OPTIONS,
  BACKTEST_FORM_STORAGE_KEY,
  BACKTEST_MARKET_FEE_MODE_OPTIONS,
  BACKTEST_RESULT_STATUS_OPTIONS,
  EXTENDED_HOURS_INTERVALS,
  canonicalBacktestInstrumentInput,
  parseBacktestFeeRules,
  readStoredBacktestFormPreferences,
} from "./backtestPagePreferences";
import {
  BACKTEST_MEDIUM_WORKBENCH_QUERY,
  useBacktestPageLayout,
  type BacktestReportMode,
} from "./useBacktestPageLayout";
import {
  firstQueryValue,
  formatStrategyVersion,
  reportModeFromQuery,
  useBacktestComparison,
  type BacktestStrategyDefinition,
} from "./useBacktestComparison";
import {
  BACKTEST_RESULTS_PAGE_SIZE,
  useBacktestResultList,
} from "./useBacktestResultList";
import { useBacktestForm } from "./useBacktestForm";

export function useBacktestPage() {
const {
  defaultMarket,
  loadMarketProfiles,
  findMarketProfile,
  quoteCurrencyForMarket,
  supportsExtendedHoursForMarket,
  normalizeInstrumentRefWithMarketApi,
} = useMarketProfiles();

const emptyStateClass =
  "rounded-lg border bt-border bt-bg-surface bt-text-muted";
const route = useRoute();
const router = useRouter();

const definitions = ref<BacktestStrategyDefinition[]>([]);
const strategyDefinitionsReady = ref(false);
const warmupPreviewBars = ref<number | null>(null);
const warmupPreviewPending = ref(false);
const warmupPreviewInterval = ref("");
let warmupPreviewRequestId = 0;

const {
  backtestFormState,
  brokerFeeMode,
  brokerFeeRules,
  brokerFeeRulesText,
  chartType,
  codeInput,
  costModeSummary,
  displayInstrumentId,
  endDate,
  extendedHoursHint,
  extendedHoursSupported,
  handleResolvedBacktestInstrument,
  initialBalance,
  instrumentSearchQuery,
  instrumentSelectionResolved,
  instrumentType,
  interval,
  marketFeeMode,
  marketFeeRules,
  marketFeeRulesText,
  periodLabel,
  quoteCurrency,
  quoteCurrencyFromInstrumentId,
  rehabType,
  resolveBacktestStrategyVersionNotice,
  resolveRunQuoteCurrency,
  resolveRunSessionMode,
  resolveStrategyDefinition,
  resolveStrategyName,
  selectedDefinition,
  selectedDefinitionId,
  selectedMarket,
  startDate,
  storedBacktestFormPreferences,
  supportsExtendedHoursForInterval,
  useExtendedHours,
} = useBacktestForm({
  definitions,
  quoteCurrencyForMarket,
  supportsExtendedHoursForMarket,
});

const warmupPreviewValue = computed(() => {
  if (!selectedDefinitionId.value) {
    return "--";
  }
  if (warmupPreviewPending.value) {
    return "计算中...";
  }
  if (warmupPreviewBars.value === null) {
    return "自动推导";
  }
  return `${warmupPreviewBars.value} 根`;
});

const warmupPreviewNote = computed(() => {
  const previewInterval = warmupPreviewInterval.value || interval.value || "5m";
  const sessionMode =
    extendedHoursSupported.value && useExtendedHours.value
      ? "扩展时段"
      : "当前时段口径";
  return `按当前标的与回测周期 ${previewInterval} 的${sessionMode}推导策略依赖的最大历史 bars。`;
});

const warmupPreviewSymbol = computed(
  () =>
    displayInstrumentId.value ||
    selectedDefinition.value?.symbol?.trim() ||
    "",
);

const {
  runs,
  running,
  syncing,
  syncProgress,
  error,
  detailLoading,
  detailErrors,
  filteredRuns: sortedRuns,
  toggleRun,
  deleteRun,
  loadRuns,
  syncKlines,
  cancelSync,
  startBacktest,
} = useBacktestRuns({
  formState: backtestFormState,
  normalizeInstrument: async (input) => {
    const candidate = (input.instrumentId || input.code).trim();
    const request =
      candidate.includes(".") || candidate.includes(":")
        ? { instrumentId: candidate }
        : {
          market: input.market,
          code: input.code,
        };
    const normalized = await normalizeInstrumentRefWithMarketApi(request);
    return {
      market: normalized.market,
      prefix: normalized.prefix,
      code: normalized.code,
      instrumentId: normalized.instrumentId,
    };
  },
});

const {
  activeReportTab,
  backtestMobileSection,
  backtestPaneSizes,
  backtestSidebarOpen,
  closeBacktestSidebar,
  disposeBacktestWorkbenchMediaQuery,
  errorExpanded,
  expandedBacktestPanels,
  handleBacktestPaneResized,
  handleBacktestPanelsUpdate,
  handleBacktestWorkbenchKeydown,
  installBacktestWorkbenchMediaQuery,
  isMediumBacktestWorkbench,
  newBacktestFormTouched,
  openNewBacktestForm,
  selectedRunId,
  selectBacktestMobileSection,
  setBacktestSetupPanelOpen,
  showNewBacktestForm,
  syncMediumBacktestWorkbench,
  toggleBacktestPanel,
  toggleBacktestSidebar,
  toggleNewBacktestForm,
} = useBacktestPageLayout({
  canShowReport: () =>
    focusedRun.value != null || reportMode.value === "compare",
});

const {
  confirmDeleteRun,
  deletingRunId,
  emptyResultsMessage,
  filteredRuns,
  hasResultsFilters,
  pagedRuns,
  pendingDeleteMessage,
  pendingDeleteRun,
  pendingDeleteRunId,
  requestDeleteRun,
  resetResultsFilters,
  resultStrategyOptions,
  resultsPage,
  resultsPageCount,
  resultsPageSummary,
  resultsSearchQuery,
  resultsStatusFilter,
  resultsStrategyFilter,
} = useBacktestResultList({
  deleteRun,
  resolveStrategyName,
  selectedRunId,
  sortedRuns,
});

const {
  activateComparisonMode,
  activateSingleReportMode,
  applyComparisonRouteState,
  applyComparisonVersionDefaults,
  changeComparisonDefinition,
  changeComparisonRun,
  changeComparisonVersion,
  clearComparisonSelection,
  clearComparisonSnapshots,
  comparisonConditionsMatch,
  comparisonChartType,
  comparisonConfigRows,
  comparisonDefinitionId,
  comparisonDefinitionOptions,
  comparisonMetricDelta,
  comparisonMetrics,
  comparisonFeeConfig,
  comparisonQueryMatchesRoute,
  comparisonRunsReady,
  comparisonSnapshotErrors,
  comparisonSnapshotLoading,
  comparisonSourcesReady,
  comparisonVersionExists,
  comparisonVersions,
  comparisonVersionsError,
  completedRunsForComparisonVersion,
  compareConfigValue,
  ensureComparisonRunDefaults,
  formatComparisonCurrency,
  formatComparisonMetric,
  isLoadingComparisonVersions,
  leftComparisonRun,
  leftComparisonRunId,
  leftComparisonRunOptions,
  leftComparisonRuns,
  leftComparisonSnapshot,
  leftComparisonVersion,
  leftComparisonVersionOptions,
  leftComparisonVersionSelectOptions,
  loadComparisonSnapshot,
  loadComparisonVersions,
  nativeSelectValue,
  reportMode,
  rightComparisonRun,
  rightComparisonRunId,
  rightComparisonRunOptions,
  rightComparisonRuns,
  rightComparisonSnapshot,
  rightComparisonVersion,
  rightComparisonVersionOptions,
  rightComparisonVersionSelectOptions,
  syncComparisonRoute,
  versionOptionTitle,
  comparisonRunOptionTitle,
  comparisonRunTimestamp,
} = useBacktestComparison({
  backtestMobileSection,
  definitions,
  getFocusedRun: () => focusedRun.value,
  resolveRunQuoteCurrency,
  resolveRunSessionMode,
  route,
  router,
  runs,
  selectedDefinitionId,
  toggleRun,
});

type BacktestRunView = (typeof sortedRuns.value)[number];

const focusedRun = computed<BacktestRunView | undefined>(() => {
  if (filteredRuns.value.length === 0) {
    return undefined;
  }
  const selected = filteredRuns.value.find((run) => run.id === selectedRunId.value);
  return selected ?? filteredRuns.value[0];
});

const focusedRunResultReady = computed(() => {
  const run = focusedRun.value;
  return run?.result != null && isTerminalBacktestStatus(run.status);
});

watch(focusedRun, (run) => {
  if (run == null && backtestMobileSection.value === "report" && reportMode.value !== "compare") {
    backtestMobileSection.value = "setup";
  }
});

const focusedRunHasChartData = computed(() => {
  const run = focusedRun.value;
  return run?.status === "completed" && (run.result?.pnlCurve?.length ?? 0) > 0;
});

function selectFocusedRun(runId: string) {
  reportMode.value = "single";
  selectedRunId.value = runId;
  activeReportTab.value = "chart";
  backtestMobileSection.value = "report";
}

watch(
  sortedRuns,
  (nextRuns) => {
    if (newBacktestFormTouched.value) {
      return;
    }
    const hasRuns = nextRuns.length > 0;
    showNewBacktestForm.value = !hasRuns;
    expandedBacktestPanels.value = hasRuns ? ["history"] : ["setup"];
  },
  { immediate: true },
);

watch(
  () => focusedRun.value?.id,
  (runId) => {
    if (runId) {
      void toggleRun(runId);
    }
  },
  { immediate: true },
);

// ── Loaders ──
function ensureSelectedMarketProfile() {
  const categoryMarket = categoryMarketForUser(selectedMarket.value);
  if (findMarketProfile(categoryMarket) != null) {
    selectedMarket.value = categoryMarket;
    return;
  }
  selectedMarket.value = defaultMarket.value.trim().toUpperCase() || "HK";
}

onMounted(async () => {
  await Promise.all([
    loadMarketProfiles(),
    loadDefinitions(),
    loadRuns(),
  ]);
  ensureSelectedMarketProfile();
  if (reportMode.value === "compare") {
    const definitionId = comparisonDefinitionId.value || selectedDefinitionId.value || definitions.value[0]?.id || "";
    if (definitionId !== "") {
      comparisonDefinitionId.value = definitionId;
      void loadComparisonVersions(definitionId);
    }
  }
});

async function loadDefinitions() {
  strategyDefinitionsReady.value = false;
  try {
    const items = await queryClient.fetchQuery({
      queryKey: queryKeys.strategyDefinitions(),
      queryFn: () => apiGet("/api/v1/strategy-definitions"),
      retry: false,
      staleTime: 0,
    });
    const nextDefinitions = items.map((item) => ({
      id: item.id ?? "",
      name: item.name ?? "",
      version: item.version ?? "",
      ...(item.symbol == null ? {} : { symbol: item.symbol }),
    })).filter((definition) => definition.id.trim() !== "");
    definitions.value = nextDefinitions;
    const selectedId = selectedDefinitionId.value.trim();
    if (nextDefinitions.length === 0) {
      selectedDefinitionId.value = "";
    } else if (!nextDefinitions.some((definition) => definition.id === selectedId)) {
      selectedDefinitionId.value = nextDefinitions[0]!.id;
    }
    strategyDefinitionsReady.value = true;
    void loadWarmupPreview();
  } catch {
    // definitions not critical for sync
  }
}

async function loadWarmupPreview() {
  const definitionId = selectedDefinitionId.value.trim();
  const requestedInterval = (interval.value || "5m").trim();
  const requestedSymbol = warmupPreviewSymbol.value.trim();
  const requestId = ++warmupPreviewRequestId;

  if (!strategyDefinitionsReady.value) {
    warmupPreviewBars.value = null;
    warmupPreviewInterval.value = requestedInterval;
    warmupPreviewPending.value = false;
    return;
  }

  if (!definitionId) {
    warmupPreviewBars.value = null;
    warmupPreviewInterval.value = requestedInterval;
    warmupPreviewPending.value = false;
    return;
  }

  warmupPreviewPending.value = true;
  try {
    const params = new URLSearchParams({ interval: requestedInterval });
    if (requestedSymbol !== "") {
      params.set("symbol", requestedSymbol);
    }
    params.set(
      "useExtendedHours",
      String(extendedHoursSupported.value && useExtendedHours.value),
    );
    const detail = await queryClient.ensureQueryData({
      queryKey: queryKeys.strategyDefinition(definitionId, {
        interval: requestedInterval,
        symbol: requestedSymbol,
        useExtendedHours: extendedHoursSupported.value && useExtendedHours.value,
      }),
      queryFn: () => apiGetPath(
        "/api/v1/strategy-definitions/{definitionId}",
        `/api/v1/strategy-definitions/${encodeURIComponent(definitionId)}?${params.toString()}`,
      ),
      retry: false,
    });
    if (requestId !== warmupPreviewRequestId) {
      return;
    }
    warmupPreviewBars.value = Number.isFinite(detail.derivedWarmupBars)
      ? (detail.derivedWarmupBars ?? null)
      : null;
    warmupPreviewInterval.value =
      detail.derivedWarmupInterval?.trim() || requestedInterval;
  } catch (error) {
    if (requestId !== warmupPreviewRequestId) {
      return;
    }
    const errorStatus =
      error instanceof ApiClientError
        ? error.status
        : (error as { status?: unknown } | null)?.status;
    if (errorStatus === 404 && selectedDefinitionId.value.trim() === definitionId) {
      selectedDefinitionId.value = "";
    }
    warmupPreviewBars.value = null;
    warmupPreviewInterval.value = requestedInterval;
  } finally {
    if (requestId === warmupPreviewRequestId) {
      warmupPreviewPending.value = false;
    }
  }
}

// ── Formatters ──
const statusChip = (status: string) => {
  const tone = statusTone(status);
  switch (status) {
    case "completed":
    case "failed":
    case "running":
      return { color: tone.color, label: tone.label };
    case "cancelled":
    case "queued":
      return { color: "warning", label: tone.label };
    default:
      return { color: "", label: tone.label };
  }
};

// When definition changes, fill defaults only if user hasn't manually overridden
watch(
  [selectedDefinitionId, interval],
  () => {
    if (!strategyDefinitionsReady.value) {
      return;
    }
    void loadWarmupPreview();
  },
);

  return {
    BACKTEST_FORM_STORAGE_KEY,
    BACKTEST_RESULTS_PAGE_SIZE,
    BACKTEST_RESULT_STATUS_OPTIONS,
    BACKTEST_BROKER_FEE_MODE_OPTIONS,
    BACKTEST_MARKET_FEE_MODE_OPTIONS,
    emptyStateClass,
    route,
    router,
    storedBacktestFormPreferences,
    definitions,
    strategyDefinitionsReady,
    warmupPreviewBars,
    warmupPreviewPending,
    warmupPreviewInterval,
    resultsPage,
    resultsSearchQuery,
    resultsStatusFilter,
    resultsStrategyFilter,
    pendingDeleteRunId,
    deletingRunId,
    reportMode,
    comparisonDefinitionId,
    leftComparisonVersion,
    rightComparisonVersion,
    leftComparisonRunId,
    rightComparisonRunId,
    comparisonVersions,
    isLoadingComparisonVersions,
    comparisonVersionsError,
    leftComparisonSnapshot,
    rightComparisonSnapshot,
    comparisonSnapshotErrors,
    comparisonSnapshotLoading,
    selectedDefinitionId,
    selectedMarket,
    codeInput,
    instrumentSearchQuery,
    interval,
    chartType,
    startDate,
    endDate,
    initialBalance,
    instrumentType,
    rehabType,
    useExtendedHours,
    brokerFeeMode,
    marketFeeMode,
    brokerFeeRulesText,
    marketFeeRulesText,
    EXTENDED_HOURS_INTERVALS,
    selectedDefinition,
    displayInstrumentId,
    instrumentSelectionResolved,
    periodLabel,
    extendedHoursSupported,
    extendedHoursHint,
    quoteCurrency,
    warmupPreviewValue,
    warmupPreviewNote,
    warmupPreviewSymbol,
    brokerFeeRules,
    marketFeeRules,
    costModeSummary,
    backtestFormState,
    BACKTEST_MEDIUM_WORKBENCH_QUERY,
    activeReportTab,
    selectedRunId,
    showNewBacktestForm,
    newBacktestFormTouched,
    backtestPaneSizes,
    backtestMobileSection,
    backtestSidebarOpen,
    isMediumBacktestWorkbench,
    errorExpanded,
    expandedBacktestPanels,
    resultStrategyOptions,
    hasResultsFilters,
    filteredRuns,
    emptyResultsMessage,
    resultsPageCount,
    pagedRuns,
    resultsPageSummary,
    comparisonDefinitionOptions,
    leftComparisonVersionOptions,
    rightComparisonVersionOptions,
    leftComparisonVersionSelectOptions,
    rightComparisonVersionSelectOptions,
    leftComparisonRuns,
    rightComparisonRuns,
    leftComparisonRunOptions,
    rightComparisonRunOptions,
    leftComparisonRun,
    rightComparisonRun,
    comparisonRunsReady,
    comparisonSourcesReady,
    comparisonMetrics,
    comparisonConfigRows,
    comparisonConditionsMatch,
    pendingDeleteRun,
    pendingDeleteMessage,
    focusedRun,
    focusedRunResultReady,
    focusedRunHasChartData,
    statusChip,
    firstQueryValue,
    reportModeFromQuery,
    readStoredBacktestFormPreferences,
    canonicalBacktestInstrumentInput,
    handleResolvedBacktestInstrument,
    supportsExtendedHoursForInterval,
    parseBacktestFeeRules,
    quoteCurrencyFromInstrumentId,
    resolveRunQuoteCurrency,
    resolveRunSessionMode,
    resolveStrategyName,
    resolveStrategyDefinition,
    formatStrategyVersion,
    resolveBacktestStrategyVersionNotice,
    comparisonRunTimestamp,
    completedRunsForComparisonVersion,
    versionOptionTitle,
    comparisonRunOptionTitle,
    clearComparisonSnapshots,
    clearComparisonSelection,
    comparisonVersionExists,
    applyComparisonVersionDefaults,
    loadComparisonVersions,
    loadComparisonSnapshot,
    nativeSelectValue,
    changeComparisonDefinition,
    changeComparisonVersion,
    changeComparisonRun,
    activateComparisonMode,
    activateSingleReportMode,
    comparisonQueryMatchesRoute,
    syncComparisonRoute,
    formatComparisonCurrency,
    formatComparisonMetric,
    comparisonMetricDelta,
    compareConfigValue,
    comparisonFeeConfig,
    comparisonChartType,
    requestDeleteRun,
    confirmDeleteRun,
    selectFocusedRun,
    setBacktestSetupPanelOpen,
    handleBacktestPanelsUpdate,
    toggleBacktestPanel,
    openNewBacktestForm,
    toggleNewBacktestForm,
    toggleBacktestSidebar,
    closeBacktestSidebar,
    syncMediumBacktestWorkbench,
    handleBacktestWorkbenchKeydown,
    installBacktestWorkbenchMediaQuery,
    disposeBacktestWorkbenchMediaQuery,
    selectBacktestMobileSection,
    handleBacktestPaneResized,
    resetResultsFilters,
    ensureComparisonRunDefaults,
    applyComparisonRouteState,
    ensureSelectedMarketProfile,
    loadDefinitions,
    loadWarmupPreview,
    defaultMarket,
    loadMarketProfiles,
    findMarketProfile,
    quoteCurrencyForMarket,
    supportsExtendedHoursForMarket,
    normalizeInstrumentRefWithMarketApi,
    runs,
    running,
    syncing,
    syncProgress,
    error,
    detailLoading,
    detailErrors,
    sortedRuns,
    toggleRun,
    deleteRun,
    loadRuns,
    syncKlines,
    cancelSync,
    startBacktest,
    KLINE_CHART_TYPES,
    KLINE_PERIODS,
    formatBacktestRehabType,
    formatBacktestRunDate,
    formatBacktestTimestamp,
    isTerminalBacktestStatus,
  };
}

export type BacktestPageContext = ReturnType<typeof useBacktestPage>;

export const backtestPageContextKey: InjectionKey<BacktestPageContext> = Symbol(
  "backtest-page-context",
);

export function useBacktestPageContext(): BacktestPageContext {
  const context = inject(backtestPageContextKey);
  if (context == null) {
    throw new Error("Backtest page context is unavailable");
  }
  return context;
}
