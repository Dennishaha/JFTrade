import type { SplitpanesResizedPayload } from "splitpanes";
import {
  computed,
  inject,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
  type InjectionKey,
} from "vue";
import { useRoute, useRouter } from "vue-router";

import {
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
  normalizeChartType,
  type ChartType,
} from "@/charting/kline";
import {
  type BacktestReportTab,
  formatBacktestRehabType,
  formatBacktestRunDate,
  formatBacktestTimestamp,
  isTerminalBacktestStatus,
} from "@/components/backtest/backtestRunPresentation";
import type {
  BacktestFeeRulePayload,
  InstrumentResolutionCandidate,
} from "@/types";
import { ApiClientError, apiGet, apiGetPath } from "@/composables/shared/apiClient";
import { formatGenericStatusLabel } from "@/composables/shared/consoleDataFormatting";
import {
  backtestInstrumentTypeForSecurityType,
  categoryMarketForUser,
} from "@/composables/market-data/instrumentPresentation";
import { useMarketProfiles } from "@/composables/market-data/marketProfiles";
import { queryClient, queryKeys } from "@/composables/settings/serverState";
import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
  type StrategyDefinitionVersionDocument,
  type StrategyDefinitionVersionSummary,
} from "@/composables/strategy/strategyDefinitionVersions";
import {
  useBacktestRuns,
  type BacktestFormState,
} from "@/composables/backtest/useBacktestRuns";
import { normalizeBacktestDateLabel } from "@/pages/backtestTimeWindow";
import dayjs from "dayjs";

export function useBacktestPage() {
const BACKTEST_FORM_STORAGE_KEY = "jftrade.backtest.form.v1";
const BACKTEST_RESULTS_PAGE_SIZE = 5;

const BACKTEST_RESULT_STATUS_OPTIONS = [
  { value: "all", title: "全部状态" },
  { value: "queued", title: "排队中" },
  { value: "running", title: "运行中" },
  { value: "completed", title: "已完成" },
  { value: "failed", title: "失败" },
  { value: "cancelled", title: "已取消" },
];

const BACKTEST_BROKER_FEE_MODE_OPTIONS = [
  { value: "market_preset", title: "市场预设" },
  { value: "script", title: "脚本" },
  { value: "custom", title: "自定义" },
  { value: "none", title: "关闭" },
];

const BACKTEST_MARKET_FEE_MODE_OPTIONS = [
  { value: "market_preset", title: "市场预设" },
  { value: "custom", title: "自定义" },
  { value: "none", title: "关闭" },
];

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

type BacktestReportMode = "single" | "compare";

function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) {
    return typeof value[0] === "string" ? value[0].trim() : "";
  }
  return typeof value === "string" ? value.trim() : "";
}
function reportModeFromQuery(value: unknown): BacktestReportMode {
  return firstQueryValue(value) === "compare" ? "compare" : "single";
}

// ── Backtest run DTOs ──
interface StrategyDefinition {
  id: string;
  name: string;
  version: string;
  symbol?: string;
  derivedWarmupBars?: number;
  derivedWarmupInterval?: string;
}

interface StoredBacktestFormPreferences {
  selectedDefinitionId: string;
  selectedMarket: string;
  codeInput: string;
  interval: string;
  chartType: ChartType;
  startDate: string;
  endDate: string;
  initialBalance: number;
  instrumentType: string;
  rehabType: string;
  useExtendedHours: boolean;
  brokerFeeMode: "market_preset" | "custom" | "script" | "none";
  marketFeeMode: "market_preset" | "custom" | "none";
  brokerFeeRulesText: string;
  marketFeeRulesText: string;
}

function readStoredBacktestFormPreferences(): StoredBacktestFormPreferences {
  const defaultStartDate = dayjs().subtract(3, "year").format("YYYY-MM-DD");
  const defaultEndDate = dayjs().format("YYYY-MM-DD");
  const defaults: StoredBacktestFormPreferences = {
    selectedDefinitionId: "",
    selectedMarket: "HK",
    codeInput: "00700",
    interval: "5m",
    chartType: "standard",
    startDate: defaultStartDate,
    endDate: defaultEndDate,
    initialBalance: 1000000,
    instrumentType: "stock",
    rehabType: "forward",
    useExtendedHours: false,
    brokerFeeMode: "market_preset",
    marketFeeMode: "market_preset",
    brokerFeeRulesText: "",
    marketFeeRulesText: "",
  };

  if (typeof window === "undefined" || window.localStorage == null) {
    return defaults;
  }

  try {
    const raw = window.localStorage.getItem(BACKTEST_FORM_STORAGE_KEY);
    if (raw == null || raw.trim() === "") {
      return defaults;
    }
    const parsed = JSON.parse(raw) as Partial<StoredBacktestFormPreferences>;
    const validIntervals = new Set<string>(
      KLINE_PERIODS.map((period) => period.value),
    );
    const validRehabTypes = new Set(["forward", "backward", "none"]);
    const validInstrumentTypes = new Set(["stock", "etf"]);
    const validBrokerFeeModes = new Set(["market_preset", "custom", "script", "none"]);
    const validMarketFeeModes = new Set(["market_preset", "custom", "none"]);
    const normalizeDate = (value: unknown, fallback: string) => {
      const normalized = normalizeBacktestDateLabel(typeof value === "string" ? value : "");
      return normalized === "" ? fallback : normalized;
    };
    const storedMarket =
      typeof parsed.selectedMarket === "string" &&
        parsed.selectedMarket.trim() !== ""
        ? parsed.selectedMarket.trim().toUpperCase()
        : defaults.selectedMarket;
    let storedCode =
      typeof parsed.codeInput === "string" && parsed.codeInput.trim() !== ""
        ? parsed.codeInput.trim().toUpperCase().replace(":", ".")
        : defaults.codeInput;
    if (
      (storedMarket === "SH" || storedMarket === "SZ") &&
      !storedCode.includes(".")
    ) {
      storedCode = `${storedMarket}.${storedCode}`;
    }

    return {
      selectedDefinitionId:
        typeof parsed.selectedDefinitionId === "string"
          ? parsed.selectedDefinitionId.trim()
          : defaults.selectedDefinitionId,
      selectedMarket: categoryMarketForUser(storedMarket),
      codeInput: storedCode,
      interval:
        typeof parsed.interval === "string" &&
          validIntervals.has(parsed.interval.trim())
          ? parsed.interval.trim()
          : defaults.interval,
      chartType: normalizeChartType(parsed.chartType),
      startDate: normalizeDate(parsed.startDate, defaults.startDate),
      endDate: normalizeDate(parsed.endDate, defaults.endDate),
      initialBalance:
        typeof parsed.initialBalance === "number" &&
          Number.isFinite(parsed.initialBalance) &&
          parsed.initialBalance > 0
          ? parsed.initialBalance
          : defaults.initialBalance,
      instrumentType:
        typeof parsed.instrumentType === "string" &&
          validInstrumentTypes.has(parsed.instrumentType.trim().toLowerCase())
          ? parsed.instrumentType.trim().toLowerCase()
          : defaults.instrumentType,
      rehabType:
        typeof parsed.rehabType === "string" &&
          validRehabTypes.has(parsed.rehabType.trim().toLowerCase())
          ? parsed.rehabType.trim().toLowerCase()
          : defaults.rehabType,
      useExtendedHours: parsed.useExtendedHours === true,
      brokerFeeMode:
        typeof parsed.brokerFeeMode === "string" &&
          validBrokerFeeModes.has(parsed.brokerFeeMode.trim().toLowerCase())
          ? (parsed.brokerFeeMode.trim().toLowerCase() as StoredBacktestFormPreferences["brokerFeeMode"])
          : defaults.brokerFeeMode,
      marketFeeMode:
        typeof parsed.marketFeeMode === "string" &&
          validMarketFeeModes.has(parsed.marketFeeMode.trim().toLowerCase())
          ? (parsed.marketFeeMode.trim().toLowerCase() as StoredBacktestFormPreferences["marketFeeMode"])
          : defaults.marketFeeMode,
      brokerFeeRulesText:
        typeof parsed.brokerFeeRulesText === "string"
          ? parsed.brokerFeeRulesText
          : defaults.brokerFeeRulesText,
      marketFeeRulesText:
        typeof parsed.marketFeeRulesText === "string"
          ? parsed.marketFeeRulesText
          : defaults.marketFeeRulesText,
    };
  } catch {
    return defaults;
  }
}

const storedBacktestFormPreferences = readStoredBacktestFormPreferences();

function canonicalBacktestInstrumentInput(market: string, code: string): string {
  const normalizedMarket = market.trim().toUpperCase();
  const normalizedCode = code.trim().toUpperCase().replace(":", ".");
  if (normalizedCode === "" || normalizedCode.includes(".")) {
    return normalizedCode;
  }
  return normalizedMarket === ""
    ? normalizedCode
    : `${normalizedMarket}.${normalizedCode}`;
}

// ── Reactive state ──
const definitions = ref<StrategyDefinition[]>([]);
const strategyDefinitionsReady = ref(false);
const warmupPreviewBars = ref<number | null>(null);
const warmupPreviewPending = ref(false);
const warmupPreviewInterval = ref("");
let warmupPreviewRequestId = 0;
const resultsPage = ref(1);
const resultsSearchQuery = ref("");
const resultsStatusFilter = ref("all");
const resultsStrategyFilter = ref("all");
const pendingDeleteRunId = ref("");
const deletingRunId = ref("");
const reportMode = ref<BacktestReportMode>(reportModeFromQuery(route.query.mode));
const comparisonDefinitionId = ref(firstQueryValue(route.query.definitionId));
const leftComparisonVersion = ref(firstQueryValue(route.query.leftVersion));
const rightComparisonVersion = ref(firstQueryValue(route.query.rightVersion));
const leftComparisonRunId = ref(firstQueryValue(route.query.leftRunId));
const rightComparisonRunId = ref(firstQueryValue(route.query.rightRunId));
const comparisonVersions = ref<StrategyDefinitionVersionSummary[]>([]);
const isLoadingComparisonVersions = ref(false);
const comparisonVersionsError = ref("");
const leftComparisonSnapshot = ref<StrategyDefinitionVersionDocument | null>(null);
const rightComparisonSnapshot = ref<StrategyDefinitionVersionDocument | null>(null);
const comparisonSnapshotErrors = ref({ left: "", right: "" });
const comparisonSnapshotLoading = ref({ left: false, right: false });
let comparisonVersionsRequestId = 0;
let leftComparisonSnapshotRequestId = 0;
let rightComparisonSnapshotRequestId = 0;
let applyingComparisonRoute = false;

// Form state
const selectedDefinitionId = ref(
  storedBacktestFormPreferences.selectedDefinitionId,
);
const selectedMarket = ref(storedBacktestFormPreferences.selectedMarket);
const codeInput = ref(storedBacktestFormPreferences.codeInput);
const instrumentSearchQuery = ref(
  canonicalBacktestInstrumentInput(
    storedBacktestFormPreferences.selectedMarket,
    storedBacktestFormPreferences.codeInput,
  ),
);
const interval = ref(storedBacktestFormPreferences.interval);
const chartType = ref<ChartType>(storedBacktestFormPreferences.chartType);
const startDate = ref(storedBacktestFormPreferences.startDate);
const endDate = ref(storedBacktestFormPreferences.endDate);
const initialBalance = ref(storedBacktestFormPreferences.initialBalance);
const instrumentType = ref(storedBacktestFormPreferences.instrumentType);
const rehabType = ref(storedBacktestFormPreferences.rehabType); // "forward" | "backward" | "none"
const useExtendedHours = ref(storedBacktestFormPreferences.useExtendedHours);
const brokerFeeMode = ref(storedBacktestFormPreferences.brokerFeeMode);
const marketFeeMode = ref(storedBacktestFormPreferences.marketFeeMode);
const brokerFeeRulesText = ref(storedBacktestFormPreferences.brokerFeeRulesText);
const marketFeeRulesText = ref(storedBacktestFormPreferences.marketFeeRulesText);

const EXTENDED_HOURS_INTERVALS = new Set([
  "1m",
  "5m",
  "15m",
  "30m",
  "1h",
  "2h",
  "4h",
  "6h",
  "12h",
  "1d",
  "1w",
  "1mo",
]);

// ── Derived ──
const selectedDefinition = computed(() =>
  definitions.value.find((d) => d.id === selectedDefinitionId.value),
);

const displayInstrumentId = computed(() => {
  const market = selectedMarket.value.trim().toUpperCase();
  const code = codeInput.value.trim().toUpperCase();
  if (code === "") {
    return "";
  }
  if (code.includes(".") || code.includes(":")) {
    return code.replace(":", ".");
  }
  return market === "" ? code : `${market}.${code}`;
});

const instrumentSelectionResolved = computed(() => {
  const draft = instrumentSearchQuery.value
    .trim()
    .toUpperCase()
    .replace(":", ".");
  if (draft === "" || displayInstrumentId.value === "") {
    return false;
  }
  const resolved = displayInstrumentId.value
    .trim()
    .toUpperCase()
    .replace(":", ".");
  return draft === resolved;
});

function handleResolvedBacktestInstrument(
  candidate: InstrumentResolutionCandidate,
): void {
  selectedMarket.value = categoryMarketForUser(candidate.market);
  codeInput.value = candidate.instrumentId;
  instrumentSearchQuery.value = candidate.instrumentId;
  instrumentType.value = backtestInstrumentTypeForSecurityType(
    candidate.securityType,
  );
}

const periodLabel = computed(
  () =>
    KLINE_PERIODS.find((p) => p.value === interval.value)?.label ??
    interval.value,
);

function supportsExtendedHoursForInterval(
  market: string,
  intervalValue: string,
) {
  if (!supportsExtendedHoursForMarket(market)) {
    return false;
  }
  return EXTENDED_HOURS_INTERVALS.has(
    (intervalValue ?? "").trim().toLowerCase(),
  );
}

const extendedHoursSupported = computed(() => {
  const market = selectedMarket.value.trim().toUpperCase();
  return supportsExtendedHoursForInterval(market, interval.value);
});

const extendedHoursHint = computed(() => {
  if (extendedHoursSupported.value) {
    return useExtendedHours.value
      ? "US 盘前、盘后与夜盘数据会写入 extended 版本，并参与本次回测回放/高周期合成。"
      : "仅使用 US regular session 数据；同步会写入 regular-only 版本，回测不会混入扩展时段 bar。";
  }
  return "当前市场或周期不支持扩展交易时段回放与对应同步版本。";
});

const quoteCurrency = computed(() => {
  return quoteCurrencyForMarket(selectedMarket.value);
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

function parseBacktestFeeRules(raw: string): BacktestFeeRulePayload[] {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return [];
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    return Array.isArray(parsed) ? (parsed as BacktestFeeRulePayload[]) : [];
  } catch {
    return [];
  }
}

const brokerFeeRules = computed(() => parseBacktestFeeRules(brokerFeeRulesText.value));
const marketFeeRules = computed(() => parseBacktestFeeRules(marketFeeRulesText.value));

const costModeSummary = computed(() => {
  const broker = BACKTEST_BROKER_FEE_MODE_OPTIONS.find((item) => item.value === brokerFeeMode.value)?.title ?? brokerFeeMode.value;
  const market = BACKTEST_MARKET_FEE_MODE_OPTIONS.find((item) => item.value === marketFeeMode.value)?.title ?? marketFeeMode.value;
  return `券商 ${broker} / 市场 ${market}`;
});

function quoteCurrencyFromInstrumentId(instrumentId: string | undefined) {
  const normalized = (instrumentId ?? "").trim().toUpperCase();
  const market = normalized.split(".")[0] ?? "";
  return quoteCurrencyForMarket(market);
}

function resolveRunQuoteCurrency(run: {
  request: { symbol: string };
  result?: { quoteCurrency?: string | undefined } | undefined;
}) {
  const resultCurrency = run.result?.quoteCurrency?.trim();
  if (resultCurrency) {
    return resultCurrency;
  }
  return quoteCurrencyFromInstrumentId(run.request.symbol);
}

function resolveRunSessionMode(run: {
  request: {
    symbol: string;
    interval: string;
    useExtendedHours?: boolean | undefined;
  };
}) {
  const normalizedSymbol = run.request.symbol.trim().toUpperCase();
  if (
    !supportsExtendedHoursForInterval(
      normalizedSymbol.split(".")[0] ?? "",
      run.request.interval,
    )
  ) {
    return "常规时段";
  }
  return run.request.useExtendedHours ? "含扩展时段" : "仅常规时段";
}

function resolveStrategyName(definitionId: string | undefined) {
  if (!definitionId) {
    return "未命名策略";
  }
  return (
    definitions.value.find((definition) => definition.id === definitionId)
      ?.name ?? definitionId
  );
}

function resolveStrategyDefinition(definitionId: string | undefined) {
  if (!definitionId) {
    return null;
  }
  return (
    definitions.value.find((definition) => definition.id === definitionId) ??
    null
  );
}

function formatStrategyVersion(version: string | undefined) {
  const normalized = (version ?? "").trim();
  if (normalized === "") {
    return "版本未知";
  }
  return `v${normalized}`;
}

function resolveBacktestStrategyVersionNotice(run: {
  request: { definitionId: string; definitionVersion?: string | undefined };
}) {
  const recordedVersion = (run.request.definitionVersion ?? "").trim();
  if (recordedVersion === "") {
    return "";
  }

  const currentDefinition = resolveStrategyDefinition(run.request.definitionId);
  if (currentDefinition == null) {
    return `历史策略回测结果：当前策略定义已不存在；该结果基于策略 ${formatStrategyVersion(recordedVersion)}。`;
  }

  const currentVersion = currentDefinition.version.trim();
  if (currentVersion === "" || currentVersion === recordedVersion) {
    return "";
  }

  return `旧版本策略回测结果：当时策略 ${formatStrategyVersion(recordedVersion)}，当前已更新到 ${formatStrategyVersion(currentVersion)}。`;
}

const backtestFormState = computed<BacktestFormState>(() => ({
  definitionId: selectedDefinitionId.value,
  definitionVersion: selectedDefinition.value?.version?.trim() ?? "",
  market: selectedMarket.value.trim().toUpperCase(),
  code: instrumentSelectionResolved.value
    ? codeInput.value.trim().toUpperCase()
    : "",
  instrumentId:
    instrumentSelectionResolved.value &&
      (codeInput.value.includes(".") || codeInput.value.includes(":"))
      ? codeInput.value.trim().toUpperCase()
      : "",
  instrumentType: instrumentType.value,
  interval: interval.value,
  chartType: interval.value === "tick" ? "standard" : chartType.value,
  startDate: startDate.value,
  endDate: endDate.value,
  initialBalance: initialBalance.value,
  rehabType: rehabType.value,
  useExtendedHours: useExtendedHours.value,
  brokerFeeMode: brokerFeeMode.value,
  marketFeeMode: marketFeeMode.value,
  brokerFeeRules: brokerFeeRules.value,
  marketFeeRules: marketFeeRules.value,
}));

watch(
  extendedHoursSupported,
  (supported) => {
    if (!supported) {
      useExtendedHours.value = false;
    }
  },
  { immediate: true },
);

watch(
  interval,
  (value) => {
    if (value === "tick") {
      chartType.value = "standard";
    }
  },
  { immediate: true },
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

type BacktestMobileSection = "setup" | "report";
type BacktestSidePanelId = "setup" | "history";

const BACKTEST_MEDIUM_WORKBENCH_QUERY = "(min-width: 769px) and (max-width: 1180px)";
let backtestMediumWorkbenchMediaQuery: MediaQueryList | null = null;

const activeReportTab = ref<BacktestReportTab>("chart");
const selectedRunId = ref("");
const showNewBacktestForm = ref(false);
const newBacktestFormTouched = ref(false);
const backtestPaneSizes = ref<[number, number]>([30, 70]);
const backtestMobileSection = ref<BacktestMobileSection>("setup");
const backtestSidebarOpen = ref(true);
const isMediumBacktestWorkbench = ref(false);
const errorExpanded = ref(false);
const expandedBacktestPanels = ref<BacktestSidePanelId[]>(["history"]);

const resultStrategyOptions = computed(() => {
  const options = [{ value: "all", title: "全部策略" }];
  const seenDefinitionIDs = new Set<string>();
  for (const run of sortedRuns.value) {
    const definitionID = run.request.definitionId.trim();
    if (definitionID === "" || seenDefinitionIDs.has(definitionID)) {
      continue;
    }
    seenDefinitionIDs.add(definitionID);
    options.push({
      value: definitionID,
      title: resolveStrategyName(definitionID),
    });
  }
  return options;
});

const hasResultsFilters = computed(
  () =>
    resultsSearchQuery.value.trim() !== "" ||
    resultsStatusFilter.value !== "all" ||
    resultsStrategyFilter.value !== "all",
);

const filteredRuns = computed(() => {
  const normalizedQuery = resultsSearchQuery.value.trim().toLowerCase();
  return sortedRuns.value.filter((run) => {
    if (
      resultsStatusFilter.value !== "all" &&
      run.status !== resultsStatusFilter.value
    ) {
      return false;
    }
    if (
      resultsStrategyFilter.value !== "all" &&
      run.request.definitionId !== resultsStrategyFilter.value
    ) {
      return false;
    }
    if (normalizedQuery === "") {
      return true;
    }

    const searchText = [
      run.id,
      run.request.symbol,
      run.request.market ?? "",
      run.request.code ?? "",
      run.request.interval,
      run.request.definitionId,
      run.request.definitionVersion ?? "",
      resolveStrategyName(run.request.definitionId),
      run.status,
    ]
      .join(" ")
      .toLowerCase();
    return searchText.includes(normalizedQuery);
  });
});

const emptyResultsMessage = computed(() => {
  if (sortedRuns.value.length === 0) {
    return "暂无回测记录。请在左侧配置参数并启动回测。";
  }
  return "没有匹配当前搜索或筛选条件的回测结果。";
});

const resultsPageCount = computed(() =>
  Math.max(
    1,
    Math.ceil(filteredRuns.value.length / BACKTEST_RESULTS_PAGE_SIZE),
  ),
);

const pagedRuns = computed(() => {
  const startIndex = (resultsPage.value - 1) * BACKTEST_RESULTS_PAGE_SIZE;
  return filteredRuns.value.slice(
    startIndex,
    startIndex + BACKTEST_RESULTS_PAGE_SIZE,
  );
});

const resultsPageSummary = computed(() => {
  if (filteredRuns.value.length === 0) {
    return "";
  }
  const startIndex = (resultsPage.value - 1) * BACKTEST_RESULTS_PAGE_SIZE;
  const visibleStart = startIndex + 1;
  const visibleEnd = Math.min(
    filteredRuns.value.length,
    startIndex + BACKTEST_RESULTS_PAGE_SIZE,
  );
  if (hasResultsFilters.value) {
    return `筛选后第 ${visibleStart}-${visibleEnd} 条，共 ${filteredRuns.value.length} 条；全部结果 ${sortedRuns.value.length} 条`;
  }
  return `第 ${visibleStart}-${visibleEnd} 条，共 ${filteredRuns.value.length} 条`;
});

type BacktestRunView = (typeof sortedRuns.value)[number];

type ComparisonSide = "left" | "right";

interface ComparisonMetric {
  label: string;
  kind: "currency" | "number" | "percent";
  left: number | undefined;
  right: number | undefined;
}

interface ComparisonConfigRow {
  label: string;
  left: string;
  right: string;
  same: boolean;
}

const comparisonDefinitionOptions = computed(() => {
  const items = definitions.value.map((definition) => ({
    value: definition.id,
    title: `${definition.name || definition.id} / ${formatStrategyVersion(definition.version)}`,
  }));
  if (
    comparisonDefinitionId.value !== "" &&
    !items.some((item) => item.value === comparisonDefinitionId.value)
  ) {
    items.unshift({
      value: comparisonDefinitionId.value,
      title: comparisonDefinitionId.value,
    });
  }
  return items;
});

const leftComparisonVersionOptions = computed(() =>
  comparisonVersions.value.filter(
    (version) => version.version !== rightComparisonVersion.value,
  ),
);
const rightComparisonVersionOptions = computed(() =>
  comparisonVersions.value.filter(
    (version) => version.version !== leftComparisonVersion.value,
  ),
);
const leftComparisonVersionSelectOptions = computed(() =>
  leftComparisonVersionOptions.value.map((version) => ({
    value: version.version,
    title: versionOptionTitle(version),
  })),
);
const rightComparisonVersionSelectOptions = computed(() =>
  rightComparisonVersionOptions.value.map((version) => ({
    value: version.version,
    title: versionOptionTitle(version),
  })),
);

function comparisonRunTimestamp(run: BacktestRunView): number {
  const updated = Date.parse(run.updatedAt);
  if (Number.isFinite(updated)) {
    return updated;
  }
  const created = Date.parse(run.createdAt);
  return Number.isFinite(created) ? created : 0;
}

function completedRunsForComparisonVersion(version: string): BacktestRunView[] {
  const normalizedVersion = version.trim();
  const definitionId = comparisonDefinitionId.value.trim();
  if (definitionId === "" || normalizedVersion === "") {
    return [];
  }
  return runs.value
    .filter(
      (run) =>
        run.status === "completed" &&
        run.request.definitionId === definitionId &&
        (run.request.definitionVersion ?? "").trim() === normalizedVersion,
    )
    .sort((left, right) => comparisonRunTimestamp(right) - comparisonRunTimestamp(left));
}

const leftComparisonRuns = computed(() =>
  completedRunsForComparisonVersion(leftComparisonVersion.value),
);
const rightComparisonRuns = computed(() =>
  completedRunsForComparisonVersion(rightComparisonVersion.value),
);
const leftComparisonRunOptions = computed(() =>
  leftComparisonRuns.value.map((run) => ({ value: run.id, title: comparisonRunOptionTitle(run) })),
);
const rightComparisonRunOptions = computed(() =>
  rightComparisonRuns.value.map((run) => ({ value: run.id, title: comparisonRunOptionTitle(run) })),
);
const leftComparisonRun = computed(() =>
  leftComparisonRuns.value.find((run) => run.id === leftComparisonRunId.value),
);
const rightComparisonRun = computed(() =>
  rightComparisonRuns.value.find((run) => run.id === rightComparisonRunId.value),
);
const comparisonRunsReady = computed(
  () => leftComparisonRun.value?.result != null && rightComparisonRun.value?.result != null,
);
const comparisonSourcesReady = computed(
  () => leftComparisonSnapshot.value != null && rightComparisonSnapshot.value != null,
);

function versionOptionTitle(version: StrategyDefinitionVersionSummary): string {
  const currentSuffix = version.isCurrent ? "（当前）" : "";
  return `v${version.version}${currentSuffix}`;
}

function comparisonRunOptionTitle(run: BacktestRunView): string {
  return `${run.id} · ${formatBacktestTimestamp(run.updatedAt)} · ${run.request.symbol}`;
}

function clearComparisonSnapshots(): void {
  leftComparisonSnapshotRequestId += 1;
  rightComparisonSnapshotRequestId += 1;
  leftComparisonSnapshot.value = null;
  rightComparisonSnapshot.value = null;
  comparisonSnapshotErrors.value = { left: "", right: "" };
  comparisonSnapshotLoading.value = { left: false, right: false };
}

function clearComparisonSelection(): void {
  comparisonVersionsRequestId += 1;
  comparisonVersions.value = [];
  comparisonVersionsError.value = "";
  isLoadingComparisonVersions.value = false;
  leftComparisonVersion.value = "";
  rightComparisonVersion.value = "";
  leftComparisonRunId.value = "";
  rightComparisonRunId.value = "";
  clearComparisonSnapshots();
}

function comparisonVersionExists(version: string): boolean {
  return comparisonVersions.value.some((candidate) => candidate.version === version);
}

function applyComparisonVersionDefaults(): void {
  const latest = comparisonVersions.value[0]?.version ?? "";
  const previous = comparisonVersions.value[1]?.version ?? "";
  let left = comparisonVersionExists(leftComparisonVersion.value)
    ? leftComparisonVersion.value
    : "";
  let right = comparisonVersionExists(rightComparisonVersion.value)
    ? rightComparisonVersion.value
    : "";

  if (left === "" && right === "" && previous !== "" && latest !== "") {
    left = previous;
    right = latest;
  } else if (left === "") {
    left = comparisonVersions.value.find((version) => version.version !== right)?.version ?? "";
  } else if (right === "") {
    right = comparisonVersions.value.find((version) => version.version !== left)?.version ?? "";
  }
  if (left === right) {
    right = comparisonVersions.value.find((version) => version.version !== left)?.version ?? "";
  }

  leftComparisonVersion.value = left;
  rightComparisonVersion.value = right;
  if (!leftComparisonRuns.value.some((run) => run.id === leftComparisonRunId.value)) {
    leftComparisonRunId.value = leftComparisonRuns.value[0]?.id ?? "";
  }
  if (!rightComparisonRuns.value.some((run) => run.id === rightComparisonRunId.value)) {
    rightComparisonRunId.value = rightComparisonRuns.value[0]?.id ?? "";
  }
  void loadComparisonSnapshot("left", left);
  void loadComparisonSnapshot("right", right);
}

async function loadComparisonVersions(
  definitionId = comparisonDefinitionId.value,
): Promise<void> {
  const normalizedDefinitionId = definitionId.trim();
  const requestId = ++comparisonVersionsRequestId;
  if (normalizedDefinitionId === "") {
    clearComparisonSelection();
    return;
  }
  isLoadingComparisonVersions.value = true;
  comparisonVersionsError.value = "";
  clearComparisonSnapshots();
  try {
    // A strategy can be saved in the design workspace immediately before this
    // view opens.  Always fetch the version list here so the compare selector
    // does not remain pinned to a fresh-but-outdated cache entry.
    const versions = await queryClient.fetchQuery({
      queryKey: strategyDefinitionVersionsQueryKey(normalizedDefinitionId),
      queryFn: () => fetchStrategyDefinitionVersions(normalizedDefinitionId),
      staleTime: 0,
    });
    if (requestId !== comparisonVersionsRequestId || normalizedDefinitionId !== comparisonDefinitionId.value) {
      return;
    }
    comparisonVersions.value = versions;
    applyComparisonVersionDefaults();
  } catch (cause) {
    if (requestId !== comparisonVersionsRequestId || normalizedDefinitionId !== comparisonDefinitionId.value) {
      return;
    }
    comparisonVersions.value = [];
    comparisonVersionsError.value = cause instanceof Error ? cause.message : String(cause);
    leftComparisonVersion.value = "";
    rightComparisonVersion.value = "";
    leftComparisonRunId.value = "";
    rightComparisonRunId.value = "";
  } finally {
    if (requestId === comparisonVersionsRequestId) {
      isLoadingComparisonVersions.value = false;
    }
  }
}

async function loadComparisonSnapshot(
  side: ComparisonSide,
  version: string,
): Promise<void> {
  const definitionId = comparisonDefinitionId.value.trim();
  const normalizedVersion = version.trim();
  const requestId = side === "left"
    ? ++leftComparisonSnapshotRequestId
    : ++rightComparisonSnapshotRequestId;
  const setSnapshot = (snapshot: StrategyDefinitionVersionDocument | null) => {
    if (side === "left") leftComparisonSnapshot.value = snapshot;
    else rightComparisonSnapshot.value = snapshot;
  };
  const currentRequestId = () => side === "left"
    ? leftComparisonSnapshotRequestId
    : rightComparisonSnapshotRequestId;
  if (definitionId === "" || normalizedVersion === "") {
    setSnapshot(null);
    comparisonSnapshotErrors.value = { ...comparisonSnapshotErrors.value, [side]: "" };
    comparisonSnapshotLoading.value = { ...comparisonSnapshotLoading.value, [side]: false };
    return;
  }
  comparisonSnapshotLoading.value = { ...comparisonSnapshotLoading.value, [side]: true };
  comparisonSnapshotErrors.value = { ...comparisonSnapshotErrors.value, [side]: "" };
  try {
    const snapshot = await queryClient.ensureQueryData({
      queryKey: strategyDefinitionVersionQueryKey(definitionId, normalizedVersion),
      queryFn: () => fetchStrategyDefinitionVersion(definitionId, normalizedVersion),
    });
    const selectedVersion = side === "left"
      ? leftComparisonVersion.value
      : rightComparisonVersion.value;
    if (
      requestId !== currentRequestId() ||
      definitionId !== comparisonDefinitionId.value ||
      normalizedVersion !== selectedVersion
    ) {
      return;
    }
    setSnapshot(snapshot);
  } catch (cause) {
    const selectedVersion = side === "left"
      ? leftComparisonVersion.value
      : rightComparisonVersion.value;
    if (
      requestId !== currentRequestId() ||
      definitionId !== comparisonDefinitionId.value ||
      normalizedVersion !== selectedVersion
    ) {
      return;
    }
    setSnapshot(null);
    comparisonSnapshotErrors.value = {
      ...comparisonSnapshotErrors.value,
      [side]: cause instanceof Error ? cause.message : String(cause),
    };
  } finally {
    if (requestId === currentRequestId()) {
      comparisonSnapshotLoading.value = { ...comparisonSnapshotLoading.value, [side]: false };
    }
  }
}

function nativeSelectValue(event: Event): string {
  return event.target instanceof HTMLSelectElement ? event.target.value : "";
}

function changeComparisonDefinition(value: unknown): void {
  const nextDefinitionId = typeof value === "string" ? value.trim() : "";
  if (nextDefinitionId === comparisonDefinitionId.value) {
    return;
  }
  comparisonDefinitionId.value = nextDefinitionId;
  clearComparisonSelection();
  void loadComparisonVersions(nextDefinitionId);
}

function changeComparisonVersion(side: ComparisonSide, value: unknown): void {
  const nextVersion = typeof value === "string" ? value.trim() : "";
  const otherVersion = side === "left"
    ? rightComparisonVersion.value
    : leftComparisonVersion.value;
  if (nextVersion === otherVersion) {
    return;
  }
  if (side === "left") {
    leftComparisonVersion.value = nextVersion;
    leftComparisonRunId.value = "";
  } else {
    rightComparisonVersion.value = nextVersion;
    rightComparisonRunId.value = "";
  }
  void loadComparisonSnapshot(side, nextVersion);
}

function changeComparisonRun(side: ComparisonSide, value: unknown): void {
  const runId = typeof value === "string" ? value : "";
  if (side === "left") leftComparisonRunId.value = runId;
  else rightComparisonRunId.value = runId;
}

function activateComparisonMode(): void {
  reportMode.value = "compare";
  const definitionId = comparisonDefinitionId.value || selectedDefinitionId.value || definitions.value[0]?.id || "";
  if (definitionId !== comparisonDefinitionId.value) {
    comparisonDefinitionId.value = definitionId;
    clearComparisonSelection();
  }
  if (definitionId !== "") {
    void loadComparisonVersions(definitionId);
  }
  backtestMobileSection.value = "report";
}

function activateSingleReportMode(): void {
  reportMode.value = "single";
  if (focusedRun.value != null) {
    backtestMobileSection.value = "report";
  }
}

function comparisonQueryMatchesRoute(): boolean {
  return (
    reportMode.value === reportModeFromQuery(route.query.mode) &&
    comparisonDefinitionId.value === firstQueryValue(route.query.definitionId) &&
    leftComparisonVersion.value === firstQueryValue(route.query.leftVersion) &&
    rightComparisonVersion.value === firstQueryValue(route.query.rightVersion) &&
    leftComparisonRunId.value === firstQueryValue(route.query.leftRunId) &&
    rightComparisonRunId.value === firstQueryValue(route.query.rightRunId)
  );
}

function syncComparisonRoute(): void {
  if (applyingComparisonRoute || comparisonQueryMatchesRoute()) {
    return;
  }
  const query = { ...route.query } as Record<string, string | string[] | undefined>;
  for (const key of ["mode", "definitionId", "leftVersion", "rightVersion", "leftRunId", "rightRunId"]) {
    delete query[key];
  }
  if (reportMode.value === "compare") {
    query.mode = "compare";
    if (comparisonDefinitionId.value !== "") query.definitionId = comparisonDefinitionId.value;
    if (leftComparisonVersion.value !== "") query.leftVersion = leftComparisonVersion.value;
    if (rightComparisonVersion.value !== "") query.rightVersion = rightComparisonVersion.value;
    if (leftComparisonRunId.value !== "") query.leftRunId = leftComparisonRunId.value;
    if (rightComparisonRunId.value !== "") query.rightRunId = rightComparisonRunId.value;
  }
  void router.replace({ path: route.path, query });
}

function formatComparisonCurrency(value: number | undefined, currency: string): string {
  if (value == null || !Number.isFinite(value)) {
    return "--";
  }
  const rendered = value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return currency === "" ? rendered : `${rendered} ${currency}`;
}

function formatComparisonMetric(value: number | undefined, kind: ComparisonMetric["kind"], currency = ""): string {
  if (value == null || !Number.isFinite(value)) {
    return "--";
  }
  if (kind === "percent") {
    return `${(value * 100).toFixed(2)}%`;
  }
  if (kind === "currency") {
    return formatComparisonCurrency(value, currency);
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 2 });
}

function comparisonMetricDelta(metric: ComparisonMetric): string {
  if (metric.left == null || metric.right == null || !Number.isFinite(metric.left) || !Number.isFinite(metric.right)) {
    return "--";
  }
  const leftCurrency = leftComparisonRun.value == null ? "" : resolveRunQuoteCurrency(leftComparisonRun.value);
  const rightCurrency = rightComparisonRun.value == null ? "" : resolveRunQuoteCurrency(rightComparisonRun.value);
  if (metric.kind === "currency" && leftCurrency !== rightCurrency) {
    return "币种不同";
  }
  const delta = metric.right - metric.left;
  const prefix = delta > 0 ? "+" : "";
  return `${prefix}${formatComparisonMetric(delta, metric.kind, rightCurrency)}`;
}

const comparisonMetrics = computed<ComparisonMetric[]>(() => {
  const left = leftComparisonRun.value?.result;
  const right = rightComparisonRun.value?.result;
  return [
    { label: "最终资金", kind: "currency", left: left?.finalBalance, right: right?.finalBalance },
    { label: "收益", kind: "currency", left: left?.pnl, right: right?.pnl },
    { label: "最大回撤", kind: "percent", left: left?.maxDrawdown, right: right?.maxDrawdown },
    { label: "当前回撤", kind: "percent", left: left?.currentDrawdown, right: right?.currentDrawdown },
    { label: "交易数", kind: "number", left: left?.totalTrades, right: right?.totalTrades },
    { label: "胜率", kind: "percent", left: left?.winRate, right: right?.winRate },
    { label: "总费用", kind: "currency", left: left?.totalFees, right: right?.totalFees },
  ];
});

function compareConfigValue(left: string, right: string): ComparisonConfigRow {
  return { label: "", left, right, same: left === right };
}

function comparisonFeeConfig(run: BacktestRunView): string {
  const costs = run.result?.tradingCosts ?? run.request.tradingCosts;
  const broker = costs?.brokerFees;
  const market = costs?.marketFees;
  const schedule = (entry: typeof broker) => {
    if (entry == null) return "market_preset";
    const mode = entry.mode ?? "market_preset";
    return entry.presetId ? `${mode}:${entry.presetId}` : mode;
  };
  return `券商 ${schedule(broker)} / 市场 ${schedule(market)}`;
}

function comparisonChartType(run: BacktestRunView): string {
  return (run.result?.chartType ?? run.request.chartType) === "heikinashi" ? "Heikin Ashi" : "标准K线";
}

const comparisonConfigRows = computed<ComparisonConfigRow[]>(() => {
  const left = leftComparisonRun.value;
  const right = rightComparisonRun.value;
  if (left == null || right == null) {
    return [];
  }
  const rows: Array<[string, string, string]> = [
    ["标的", left.request.symbol, right.request.symbol],
    ["周期", left.request.interval, right.request.interval],
    ["日期", `${formatBacktestRunDate(left.request.startDate)} → ${formatBacktestRunDate(left.request.endDate)}`, `${formatBacktestRunDate(right.request.startDate)} → ${formatBacktestRunDate(right.request.endDate)}`],
    ["初始资金", formatComparisonCurrency(left.request.initialBalance, resolveRunQuoteCurrency(left)), formatComparisonCurrency(right.request.initialBalance, resolveRunQuoteCurrency(right))],
    ["复权", formatBacktestRehabType(left.request.rehabType), formatBacktestRehabType(right.request.rehabType)],
    ["交易时段", resolveRunSessionMode(left), resolveRunSessionMode(right)],
    ["图表类型", comparisonChartType(left), comparisonChartType(right)],
    ["费用规则", comparisonFeeConfig(left), comparisonFeeConfig(right)],
    ["执行模型", left.result?.executionModel ?? left.request.executionModel ?? "默认", right.result?.executionModel ?? right.request.executionModel ?? "默认"],
  ];
  return rows.map(([label, leftValue, rightValue]) => ({
    ...compareConfigValue(leftValue, rightValue),
    label,
  }));
});

const comparisonConditionsMatch = computed(() =>
  comparisonConfigRows.value.length > 0 && comparisonConfigRows.value.every((row) => row.same),
);

const pendingDeleteRun = computed(() =>
  sortedRuns.value.find((run) => run.id === pendingDeleteRunId.value),
);
const pendingDeleteMessage = computed(() => {
  const run = pendingDeleteRun.value;
  if (run == null) return "";
  return `确认永久删除回测记录 ${run.id}（${resolveStrategyName(run.request.definitionId)} / ${run.request.symbol}）？此操作无法撤销。`;
});

function requestDeleteRun(runId: string): void {
  const run = sortedRuns.value.find((candidate) => candidate.id === runId);
  if (run == null || !isTerminalBacktestStatus(run.status)) return;
  pendingDeleteRunId.value = run.id;
}

async function confirmDeleteRun(): Promise<void> {
  const runId = pendingDeleteRunId.value;
  if (runId === "" || deletingRunId.value !== "") return;
  deletingRunId.value = runId;
  try {
    await deleteRun(runId);
  } finally {
    pendingDeleteRunId.value = "";
    deletingRunId.value = "";
  }
}

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

function setBacktestSetupPanelOpen(open: boolean): void {
  const nextPanels: BacktestSidePanelId[] = expandedBacktestPanels.value.filter(
    (panel) => panel !== "setup",
  );
  if (open) {
    nextPanels.unshift("setup");
  }
  if (!nextPanels.includes("history")) {
    nextPanels.push("history");
  }
  expandedBacktestPanels.value = nextPanels;
  showNewBacktestForm.value = open;
}

function handleBacktestPanelsUpdate(value: unknown): void {
  const panels = Array.isArray(value)
    ? value.filter((panel): panel is BacktestSidePanelId => panel === "setup" || panel === "history")
    : [];
  expandedBacktestPanels.value = panels;
  const setupOpen = panels.includes("setup");
  if (setupOpen !== showNewBacktestForm.value) {
    newBacktestFormTouched.value = true;
    showNewBacktestForm.value = setupOpen;
  }
}

function toggleBacktestPanel(panel: BacktestSidePanelId): void {
  const nextPanels = expandedBacktestPanels.value.includes(panel)
    ? expandedBacktestPanels.value.filter((item) => item !== panel)
    : [...expandedBacktestPanels.value, panel];
  handleBacktestPanelsUpdate(nextPanels);
}

function openNewBacktestForm(): void {
  newBacktestFormTouched.value = true;
  setBacktestSetupPanelOpen(true);
  backtestSidebarOpen.value = true;
  backtestMobileSection.value = "setup";
}

function toggleNewBacktestForm() {
  newBacktestFormTouched.value = true;
  setBacktestSetupPanelOpen(!showNewBacktestForm.value);
  backtestSidebarOpen.value = true;
  backtestMobileSection.value = "setup";
}

function toggleBacktestSidebar(): void {
  if (typeof window !== "undefined" && window.innerWidth <= 768) {
    backtestMobileSection.value = "setup";
    return;
  }
  backtestSidebarOpen.value = !backtestSidebarOpen.value;
}

function closeBacktestSidebar(): void {
  backtestSidebarOpen.value = false;
}

function syncMediumBacktestWorkbench(event: MediaQueryListEvent | MediaQueryList): void {
  const wasMedium = isMediumBacktestWorkbench.value;
  isMediumBacktestWorkbench.value = event.matches;
  if (event.matches) {
    backtestSidebarOpen.value = false;
    return;
  }
  if (wasMedium) {
    backtestSidebarOpen.value = true;
  }
}

function handleBacktestWorkbenchKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape" && isMediumBacktestWorkbench.value && backtestSidebarOpen.value) {
    closeBacktestSidebar();
  }
}

function installBacktestWorkbenchMediaQuery(): void {
  if (typeof window === "undefined") {
    return;
  }
  window.addEventListener("keydown", handleBacktestWorkbenchKeydown);
  if (typeof window.matchMedia !== "function") {
    return;
  }
  backtestMediumWorkbenchMediaQuery = window.matchMedia(BACKTEST_MEDIUM_WORKBENCH_QUERY);
  syncMediumBacktestWorkbench(backtestMediumWorkbenchMediaQuery);
  if (typeof backtestMediumWorkbenchMediaQuery.addEventListener === "function") {
    backtestMediumWorkbenchMediaQuery.addEventListener("change", syncMediumBacktestWorkbench);
  } else {
    backtestMediumWorkbenchMediaQuery.addListener(syncMediumBacktestWorkbench);
  }
}

function disposeBacktestWorkbenchMediaQuery(): void {
  if (typeof window !== "undefined") {
    window.removeEventListener("keydown", handleBacktestWorkbenchKeydown);
  }
  if (typeof backtestMediumWorkbenchMediaQuery?.removeEventListener === "function") {
    backtestMediumWorkbenchMediaQuery.removeEventListener("change", syncMediumBacktestWorkbench);
  } else {
    backtestMediumWorkbenchMediaQuery?.removeListener(syncMediumBacktestWorkbench);
  }
  backtestMediumWorkbenchMediaQuery = null;
}

function selectBacktestMobileSection(section: BacktestMobileSection): void {
  if (section === "report" && focusedRun.value == null && reportMode.value !== "compare") {
    backtestMobileSection.value = "setup";
    return;
  }
  backtestMobileSection.value = section;
}

function handleBacktestPaneResized(payload: SplitpanesResizedPayload): void {
  const sizes = payload.panes?.map((pane) => pane.size);
  if (
    sizes == null ||
    sizes.length !== 2 ||
    !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
  ) {
    return;
  }

  backtestPaneSizes.value = [sizes[0]!, sizes[1]!];
}

function resetResultsFilters() {
  resultsSearchQuery.value = "";
  resultsStatusFilter.value = "all";
  resultsStrategyFilter.value = "all";
  resultsPage.value = 1;
}

watch(
  [
    selectedDefinitionId,
    selectedMarket,
    codeInput,
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
  ],
  ([
    nextDefinitionId,
    nextMarket,
    nextCodeInput,
    nextInterval,
    nextChartType,
    nextStartDate,
    nextEndDate,
    nextInitialBalance,
    nextInstrumentType,
    nextRehabType,
    nextUseExtendedHours,
    nextBrokerFeeMode,
    nextMarketFeeMode,
    nextBrokerFeeRulesText,
    nextMarketFeeRulesText,
  ]) => {
    if (typeof window === "undefined" || window.localStorage == null) {
      return;
    }
    const storedPreferences: StoredBacktestFormPreferences = {
      selectedDefinitionId: nextDefinitionId.trim(),
      selectedMarket: nextMarket.trim().toUpperCase(),
      codeInput: nextCodeInput.trim().toUpperCase(),
      interval: nextInterval.trim(),
      chartType: normalizeChartType(nextChartType),
      startDate: nextStartDate,
      endDate: nextEndDate,
      initialBalance: nextInitialBalance,
      instrumentType: nextInstrumentType,
      rehabType: nextRehabType,
      useExtendedHours: nextUseExtendedHours,
      brokerFeeMode: nextBrokerFeeMode,
      marketFeeMode: nextMarketFeeMode,
      brokerFeeRulesText: nextBrokerFeeRulesText,
      marketFeeRulesText: nextMarketFeeRulesText,
    };
    window.localStorage.setItem(
      BACKTEST_FORM_STORAGE_KEY,
      JSON.stringify(storedPreferences),
    );
  },
  { immediate: true },
);

watch(
  () => [filteredRuns.value.length, resultsPageCount.value] as const,
  () => {
    if (resultsPage.value > resultsPageCount.value) {
      resultsPage.value = resultsPageCount.value;
    }
    if (resultsPage.value < 1) {
      resultsPage.value = 1;
    }
  },
  { immediate: true },
);

watch([resultsSearchQuery, resultsStatusFilter, resultsStrategyFilter], () => {
  resultsPage.value = 1;
});

watch(
  filteredRuns,
  (nextRuns) => {
    if (nextRuns.length === 0) {
      selectedRunId.value = "";
      return;
    }
    if (!nextRuns.some((run) => run.id === selectedRunId.value)) {
      selectedRunId.value = nextRuns[0]?.id ?? "";
    }
  },
  { immediate: true },
);

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

function ensureComparisonRunDefaults(): void {
  if (!leftComparisonRuns.value.some((run) => run.id === leftComparisonRunId.value)) {
    leftComparisonRunId.value = leftComparisonRuns.value[0]?.id ?? "";
  }
  if (!rightComparisonRuns.value.some((run) => run.id === rightComparisonRunId.value)) {
    rightComparisonRunId.value = rightComparisonRuns.value[0]?.id ?? "";
  }
}

function applyComparisonRouteState(): void {
  const nextMode = reportModeFromQuery(route.query.mode);
  const nextDefinitionId = firstQueryValue(route.query.definitionId);
  const definitionChanged = nextDefinitionId !== comparisonDefinitionId.value;
  applyingComparisonRoute = true;
  reportMode.value = nextMode;
  comparisonDefinitionId.value = nextDefinitionId;
  leftComparisonVersion.value = firstQueryValue(route.query.leftVersion);
  rightComparisonVersion.value = firstQueryValue(route.query.rightVersion);
  leftComparisonRunId.value = firstQueryValue(route.query.leftRunId);
  rightComparisonRunId.value = firstQueryValue(route.query.rightRunId);
  applyingComparisonRoute = false;
  if (nextMode === "compare" && nextDefinitionId !== "" && (definitionChanged || comparisonVersions.value.length === 0)) {
    void loadComparisonVersions(nextDefinitionId);
  }
}

watch(
  () => [
    route.query.mode,
    route.query.definitionId,
    route.query.leftVersion,
    route.query.rightVersion,
    route.query.leftRunId,
    route.query.rightRunId,
  ] as const,
  () => applyComparisonRouteState(),
);

watch(
  [
    reportMode,
    comparisonDefinitionId,
    leftComparisonVersion,
    rightComparisonVersion,
    leftComparisonRunId,
    rightComparisonRunId,
  ],
  () => syncComparisonRoute(),
);

watch(
  () => [
    leftComparisonVersion.value,
    rightComparisonVersion.value,
    runs.value,
  ] as const,
  () => ensureComparisonRunDefaults(),
  { deep: true },
);

watch(
  () => [leftComparisonRunId.value, rightComparisonRunId.value] as const,
  ([leftRunId, rightRunId]) => {
    if (leftRunId !== "") {
      void toggleRun(leftRunId);
    }
    if (rightRunId !== "") {
      void toggleRun(rightRunId);
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
  installBacktestWorkbenchMediaQuery();
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

onBeforeUnmount(() => {
  disposeBacktestWorkbenchMediaQuery();
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
  switch (status) {
    case "completed":
      return { color: "success", label: formatGenericStatusLabel(status) };
    case "failed":
      return { color: "error", label: formatGenericStatusLabel(status) };
    case "cancelled":
      return { color: "warning", label: formatGenericStatusLabel(status) };
    case "running":
      return { color: "info", label: formatGenericStatusLabel(status) };
    case "queued":
      return { color: "warning", label: formatGenericStatusLabel(status) };
    default:
      return { color: "", label: formatGenericStatusLabel(status) };
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
