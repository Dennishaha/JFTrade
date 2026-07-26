<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import {
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
  normalizeChartType,
  type ChartType,
} from "../charting/kline";
import BacktestChart from "../components/BacktestChart.vue";
import StrategySourceDiff from "../components/StrategySourceDiff.vue";
import InstrumentIdentity from "../components/domain/market-data/InstrumentIdentity.vue";
import InstrumentSearchBox from "../components/domain/market-data/InstrumentSearchBox.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import type {
  BacktestFeeRulePayload,
  InstrumentResolutionCandidate,
} from "../contracts";
import { apiGet, fetchEnvelope } from "../composables/apiClient";
import { formatGenericStatusLabel } from "../composables/consoleDataFormatting";
import {
  backtestInstrumentTypeForSecurityType,
  categoryMarketForUser,
} from "../composables/instrumentPresentation";
import { useMarketProfiles } from "../composables/marketProfiles";
import { queryClient, queryKeys } from "../composables/serverState";
import {
  fetchStrategyDefinitionVersion,
  fetchStrategyDefinitionVersions,
  strategyDefinitionVersionQueryKey,
  strategyDefinitionVersionsQueryKey,
  type StrategyDefinitionVersionDocument,
  type StrategyDefinitionVersionSummary,
} from "../composables/strategyDefinitionVersions";
import {
  useBacktestRuns,
  type BacktestFormState,
} from "../composables/useBacktestRuns";
import { formatLocalDateTime } from "../utils/dateTime";
import { normalizeBacktestDateLabel } from "./backtestTimeWindow";
import dayjs from "dayjs";

const BACKTEST_FORM_STORAGE_KEY = "jftrade.backtest.form.v1";
const BACKTEST_RESULTS_PAGE_SIZE = 5;
const BACKTEST_ORDER_BOOK_RENDER_WINDOW = 200;
const BACKTEST_RUNTIME_ERROR_RENDER_WINDOW = 120;
const BACKTEST_WARNING_RENDER_WINDOW = 120;
const BACKTEST_LOG_RENDER_WINDOW = 120;

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
const cardBorderClass = "rounded-lg border bt-border";
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

function formatBacktestRehabType(rehabType: string | undefined) {
  switch ((rehabType ?? "forward").trim().toLowerCase()) {
    case "none":
      return "不复权";
    case "backward":
      return "后复权";
    case "forward":
    default:
      return "前复权";
  }
}

function resolveBacktestPriceBasisNote(run: {
  request: { rehabType?: string; interval: string };
}) {
  const rehabLabel = formatBacktestRehabType(run.request.rehabType);
  const intervalLabel = run.request.interval.trim() || "当前周期";
  if ((run.request.rehabType ?? "forward").trim().toLowerCase() === "none") {
    return `价格口径：图表显示的是 ${intervalLabel} 已闭合历史 K 线；若和当前盘后/夜盘快照不同，通常是因为快照展示的是最新成交，而不是最后一根已闭合 bar。`;
  }
  return `价格口径：图表显示的是${rehabLabel}${intervalLabel}已闭合历史 K 线；不要直接和实时盘后/夜盘快照比较，后者通常是不复权的最新成交。`;
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

type BacktestReportTab = "chart" | "orders" | "properties";
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

function visibleBacktestOrderBook(run: BacktestRunView) {
  return (run.result?.orderBook ?? []).slice(0, BACKTEST_ORDER_BOOK_RENDER_WINDOW);
}

function hiddenBacktestOrderBookCount(run: BacktestRunView): number {
  return Math.max(
    0,
    (run.result?.orderBook?.length ?? 0) - BACKTEST_ORDER_BOOK_RENDER_WINDOW,
  );
}

function visibleBacktestRuntimeErrors(run: BacktestRunView): string[] {
  return (run.result?.runtimeErrors ?? []).slice(
    0,
    BACKTEST_RUNTIME_ERROR_RENDER_WINDOW,
  );
}

function hiddenBacktestRuntimeErrorCount(run: BacktestRunView): number {
  return Math.max(
    0,
    (run.result?.runtimeErrors?.length ?? 0) -
    BACKTEST_RUNTIME_ERROR_RENDER_WINDOW,
  );
}

function visibleBacktestWarnings(run: BacktestRunView): string[] {
  return (run.result?.warnings ?? []).slice(0, BACKTEST_WARNING_RENDER_WINDOW);
}

function hiddenBacktestWarningCount(run: BacktestRunView): number {
  return Math.max(
    0,
    (run.result?.warnings?.length ?? 0) - BACKTEST_WARNING_RENDER_WINDOW,
  );
}

function visibleBacktestLogs(run: BacktestRunView): string[] {
  return (run.result?.logs ?? []).slice(0, BACKTEST_LOG_RENDER_WINDOW);
}

function hiddenBacktestLogCount(run: BacktestRunView): number {
  return Math.max(0, (run.result?.logs?.length ?? 0) - BACKTEST_LOG_RENDER_WINDOW);
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
  try {
    const items = await queryClient.ensureQueryData({
      queryKey: queryKeys.strategyDefinitions(),
      queryFn: () => apiGet("/api/v1/strategy-definitions"),
    });
    definitions.value = items.map((item) => ({
      id: item.id ?? "",
      name: item.name ?? "",
      version: item.version ?? "",
      ...(item.symbol == null ? {} : { symbol: item.symbol }),
    }));
    if (definitions.value.length > 0 && !selectedDefinitionId.value) {
      selectedDefinitionId.value = definitions.value[0]!.id;
    }
  } catch {
    // definitions not critical for sync
  }
}

async function loadWarmupPreview() {
  const definitionId = selectedDefinitionId.value.trim();
  const requestedInterval = (interval.value || "5m").trim();
  const requestedSymbol = warmupPreviewSymbol.value.trim();
  const requestId = ++warmupPreviewRequestId;

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
      queryFn: () => fetchEnvelope<StrategyDefinition>(
        `/api/v1/strategy-definitions/${encodeURIComponent(definitionId)}?${params.toString()}`,
      ),
    });
    if (requestId !== warmupPreviewRequestId) {
      return;
    }
    warmupPreviewBars.value = Number.isFinite(detail.derivedWarmupBars)
      ? (detail.derivedWarmupBars ?? null)
      : null;
    warmupPreviewInterval.value =
      detail.derivedWarmupInterval?.trim() || requestedInterval;
  } catch {
    if (requestId !== warmupPreviewRequestId) {
      return;
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

function pnlColor(val: number) {
  if (val >= 0) {
    return "tv-up";
  }
  return "tv-down";
}

function isTerminalBacktestStatus(status: string) {
  return status === "completed" || status === "failed" || status === "cancelled";
}

function runtimeErrorTotal(result: {
  runtimeErrors?: string[] | undefined;
  runtimeErrorTotal?: number | undefined;
}) {
  return result.runtimeErrorTotal ?? result.runtimeErrors?.length ?? 0;
}

function runtimeErrorRepeatCount(
  result: { runtimeErrorCounts?: Record<string, number> | undefined },
  message: string,
) {
  return result.runtimeErrorCounts?.[message] ?? 1;
}

function runtimeErrorSummary(result: {
  runtimeErrors?: string[] | undefined;
  runtimeErrorTotal?: number | undefined;
  runtimeErrorsTruncated?: boolean | undefined;
}) {
  const shown = result.runtimeErrors?.length ?? 0;
  const total = runtimeErrorTotal(result);
  if (result.runtimeErrorsTruncated || total > shown) {
    return `运行时错误 ${total} 次，仅显示 ${shown} 条样本`;
  }
  return `运行时错误 (${total})`;
}

function warningTotal(result: {
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
}) {
  return result.warningTotal ?? result.warnings?.length ?? 0;
}

function warningSummary(result: {
  warnings?: string[] | undefined;
  warningTotal?: number | undefined;
  warningsTruncated?: boolean | undefined;
  ignoredOrders?: number | undefined;
}) {
  const shown = result.warnings?.length ?? 0;
  const total = warningTotal(result);
  const ignoredOrders = result.ignoredOrders ?? 0;
  const prefix = ignoredOrders > 0 ? `回测警告 ${total} 条，忽略订单 ${ignoredOrders} 笔` : `回测警告 (${total})`;
  if (result.warningsTruncated || total > shown) {
    return `${prefix}，仅显示 ${shown} 条样本`;
  }
  return prefix;
}

function pnlPrefix(val: number) {
  return val >= 0 ? "+" : "";
}

function usesClosedTradeStats(result: {
  tradeStatsVersion?: number | undefined;
}): boolean {
  return result.tradeStatsVersion === 2;
}

function backtestFillCount(result: {
  totalFills?: number | undefined;
  totalTrades: number;
  trades?: readonly unknown[] | undefined;
}): number {
  if (Number.isFinite(result.totalFills) && (result.totalFills ?? 0) >= 0) {
    return Math.trunc(result.totalFills ?? 0);
  }
  if (result.trades != null) {
    return result.trades.length;
  }
  return Number.isFinite(result.totalTrades) && result.totalTrades > 0
    ? Math.trunc(result.totalTrades)
    : 0;
}

function drawdownColor(value: number | undefined) {
  if ((value ?? 0) > 0) {
    return "bt-metric-negative";
  }
  return "bt-text";
}

function formatPercentMetric(value: number | undefined) {
  const normalized = Number.isFinite(value) ? (value ?? 0) : 0;
  return `${(normalized * 100).toFixed(2)}%`;
}

function formatBacktestTimestamp(value?: string) {
  if (!value) {
    return "--";
  }

  return formatLocalDateTime(value, "--");
}

function formatBacktestRunDate(date: string | undefined) {
  return normalizeBacktestDateLabel(date ?? "") || "--";
}

function formatBacktestOrderSide(side: string) {
  switch (side) {
    case "BUY":
      return "买入";
    case "SELL":
      return "卖出";
    default:
      return side;
  }
}

function formatBacktestOrderStatus(status: string) {
  switch (status) {
    case "NEW":
      return "已下单";
    case "FILLED":
      return "已成交";
    case "CANCELED":
      return "已撤单";
    case "REJECTED":
      return "已拒绝";
    default:
      return status;
  }
}

function formatBacktestOrderPrice(
  value: number | undefined,
  orderType?: string,
  raw?: string,
) {
  if (raw && raw.trim() !== "" && raw !== "0") {
    return raw;
  }
  if (value !== undefined && Number.isFinite(value) && value > 0) {
    return value.toLocaleString(undefined, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    });
  }
  if (orderType === "MARKET") {
    return "市价";
  }
  return "--";
}

function formatBacktestQuantity(value: number | undefined, raw?: string) {
  if (raw && raw.trim() !== "") {
    return raw;
  }
  if (value === undefined || !Number.isFinite(value)) {
    return "--";
  }
  return value.toLocaleString(undefined, {
    minimumFractionDigits: Number.isInteger(value) ? 0 : 2,
    maximumFractionDigits: 4,
  });
}

function formatBacktestFee(value: number | undefined, currency?: string) {
  if (value === undefined || !Number.isFinite(value) || value <= 0) {
    return "--";
  }
  const amount = value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  });
  return currency && currency.trim() !== "" ? `${amount} ${currency}` : amount;
}

function resolveQueriedCandleBounds(
  candles: Array<{ time: string }> | undefined,
) {
  if (!candles || candles.length === 0) {
    return null;
  }

  const sorted = [...candles]
    .filter((candle) => {
      const at = new Date(candle.time).getTime();
      return Number.isFinite(at);
    })
    .sort(
      (left, right) =>
        new Date(left.time).getTime() - new Date(right.time).getTime(),
    );

  if (sorted.length === 0) {
    return null;
  }

  const first = sorted[0];
  const last = sorted[sorted.length - 1];
  if (!first || !last) {
    return null;
  }

  return {
    left: formatBacktestTimestamp(first.time),
    right: formatBacktestTimestamp(last.time),
    count: sorted.length,
  };
}

// When definition changes, fill defaults only if user hasn't manually overridden
watch(
  [selectedDefinitionId, interval],
  () => {
    void loadWarmupPreview();
  },
  { immediate: true },
);
</script>

<template>
  <div class="backtest-page" :class="[
    `backtest-page--mobile-${backtestMobileSection}`,
    backtestSidebarOpen ? 'backtest-page--sidebar-open' : 'backtest-page--sidebar-closed',
    { 'backtest-page--medium': isMediumBacktestWorkbench },
  ]">
    <header class="backtest-workbench-header">
      <div class="backtest-workbench-header__identity">
        <button type="button" class="backtest-sidebar-toggle"
          :class="{ 'is-active': backtestSidebarOpen || backtestMobileSection === 'setup' }"
          :aria-expanded="backtestSidebarOpen" aria-controls="backtest-sidebar" data-testid="backtest-sidebar-toggle"
          title="显示或隐藏回测配置与历史" @click="toggleBacktestSidebar">
          <v-icon size="14">fa-solid fa-table-columns</v-icon>
          <span>配置与历史</span>
        </button>
        <div class="backtest-workbench-title">
          <h1>回测工作台</h1>
        </div>
      </div>
      <div class="backtest-workbench-header__actions">
        <div class="backtest-report-mode-switch" aria-label="回测报告视图">
          <button type="button" class="backtest-report-mode-switch__button"
            :class="{ 'is-active': reportMode === 'single' }" data-testid="backtest-report-mode-single"
            @click="activateSingleReportMode">
            单次报告
          </button>
          <button type="button" class="backtest-report-mode-switch__button"
            :class="{ 'is-active': reportMode === 'compare' }" data-testid="backtest-open-version-comparison"
            @click="activateComparisonMode">
            版本对比
          </button>
        </div>
        <button v-if="reportMode === 'single' && focusedRun && isTerminalBacktestStatus(focusedRun.status)"
          type="button" class="backtest-header-icon-button backtest-header-icon-button--danger" title="删除回测结果"
          aria-label="删除当前回测结果" @click="requestDeleteRun(focusedRun.id)">
          <v-icon size="13">fa-solid fa-trash</v-icon>
        </button>
        <button type="button" class="backtest-header-action backtest-header-action--primary"
          data-testid="backtest-open-new-form" @click="openNewBacktestForm">
          <v-icon size="13">fa-solid fa-plus</v-icon>
          新建回测
        </button>
      </div>
    </header>

    <div v-if="error" class="backtest-error-banner" :class="{ 'is-expanded': errorExpanded }" :title="error">
      <button type="button" class="backtest-error-banner__content" :aria-expanded="errorExpanded"
        @click="errorExpanded = !errorExpanded">
        <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
        <span>{{ error }}</span>
        <v-icon size="12">{{ errorExpanded ? "fa-solid fa-chevron-up" : "fa-solid fa-chevron-down" }}</v-icon>
      </button>
      <button type="button" class="backtest-error-banner__close" aria-label="关闭错误提示" @click="error = ''">
        <v-icon size="12">fa-solid fa-xmark</v-icon>
      </button>
    </div>

    <nav class="backtest-page__mobile-switch" aria-label="回测移动端工作区">
      <button class="backtest-page__mobile-switch-button" :class="{ 'is-active': backtestMobileSection === 'setup' }"
        data-testid="backtest-mobile-section-setup" type="button" @click="selectBacktestMobileSection('setup')">
        配置与历史
      </button>
      <button class="backtest-page__mobile-switch-button" :class="{ 'is-active': backtestMobileSection === 'report' }"
        data-testid="backtest-mobile-section-report" :disabled="focusedRun == null && reportMode !== 'compare'"
        type="button" @click="selectBacktestMobileSection('report')">
        报告
      </button>
    </nav>

    <button v-if="isMediumBacktestWorkbench && backtestSidebarOpen" type="button" class="backtest-sidebar-backdrop"
      aria-label="关闭回测配置与历史" data-testid="backtest-sidebar-backdrop" @click="closeBacktestSidebar" />

    <SplitPane class="backtest-page__split" :pane-min-size="18" @resized="handleBacktestPaneResized">
      <SplitPaneItem :size="backtestPaneSizes[0]" :min-size="22" :max-size="55">
        <aside id="backtest-sidebar" class="backtest-page__pane backtest-page__pane--sidebar">
          <div class="bt-sidebar-shell">
            <div class="bt-sidebar-drawer-head">
              <div>
                <strong>配置与历史</strong>
                <span>{{ resultsPageSummary || "回测结果由服务端提供。" }}</span>
              </div>
              <button type="button" aria-label="关闭回测配置与历史" @click="closeBacktestSidebar">
                <v-icon size="14">fa-solid fa-xmark</v-icon>
              </button>
            </div>

            <div class="bt-sidebar-panels">
              <section class="bt-sidebar-panel bt-sidebar-panel--setup"
                :class="{ 'is-expanded': expandedBacktestPanels.includes('setup') }">
                <button type="button" class="bt-sidebar-panel__title" data-testid="backtest-side-panel-setup-title"
                  :aria-expanded="expandedBacktestPanels.includes('setup')" @click="toggleBacktestPanel('setup')">
                  <v-icon size="11">fa-solid fa-chevron-right</v-icon>
                  <span>回测配置</span>
                  <em>{{ selectedDefinitionId ? "已选择策略" : "等待策略" }}</em>
                </button>
                <div v-if="showNewBacktestForm" class="bt-sidebar-panel__body bt-sidebar-panel__body--setup">
                  <div class="bt-new-backtest-form">
                    <div class="bt-new-backtest-fields">
                      <section class="grid gap-1.5">
                        <div class="flex items-center justify-between gap-2">
                          <div class="text-sm font-semibold bt-text-strong">策略与标的</div>
                          <div class="truncate text-xs bt-text-muted">
                            <InstrumentIdentity v-if="displayInstrumentId" :instrument-id="displayInstrumentId"
                              compact />
                            <template v-else>等待标的</template>
                          </div>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-definition">策略定义</label>
                          <select id="bt-field-definition" v-model="selectedDefinitionId" class="bt-native-select">
                            <option value="" disabled>选择策略</option>
                            <option v-for="definition in definitions" :key="definition.id" :value="definition.id">
                              {{ definition.name }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label">代码或名称</label>
                          <InstrumentSearchBox v-model="instrumentSearchQuery" action-label="查询"
                            input-test-id="backtest-instrument-code" placeholder="输入代码或名称"
                            root-test-id="backtest-instrument-search" submit-test-id="backtest-instrument-submit"
                            variant="backtest" @select="handleResolvedBacktestInstrument" />
                        </div>
                        <div v-if="!instrumentSelectionResolved" class="bt-inline-warning"
                          data-testid="backtest-instrument-unresolved">
                          当前输入尚未解析。请查询并选择标的后再同步或运行；未解析内容不会覆盖已保存标的。
                        </div>
                      </section>

                      <section class="grid gap-1.5 border-t bt-border pt-2">
                        <div class="text-sm font-semibold bt-text-strong">数据范围</div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-interval">K线周期</label>
                          <select id="bt-field-interval" v-model="interval" class="bt-native-select">
                            <option v-for="period in KLINE_PERIODS" :key="period.value" :value="period.value">
                              {{ period.label }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-chart-type">图表类型</label>
                          <select id="bt-field-chart-type" v-model="chartType" class="bt-native-select"
                            :disabled="interval === 'tick'">
                            <option v-for="type in KLINE_CHART_TYPES" :key="type.value" :value="type.value">
                              {{ type.label }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-start-date">起始日期</label>
                          <input id="bt-field-start-date" v-model="startDate" type="date" class="bt-native-input" />
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-end-date">结束日期</label>
                          <input id="bt-field-end-date" v-model="endDate" type="date" class="bt-native-input" />
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-rehab">复权方式</label>
                          <select id="bt-field-rehab" v-model="rehabType" class="bt-native-select">
                            <option value="forward">前复权</option>
                            <option value="backward">后复权</option>
                            <option value="none">不复权</option>
                          </select>
                        </div>
                        <div v-if="extendedHoursSupported"
                          class="bt-extended-hours rounded-md border bt-border px-2 py-1.5">
                          <label class="bt-form-check">
                            <input v-model="useExtendedHours" type="checkbox" class="bt-form-check__input" />
                            <span class="min-w-0 flex-1">
                              <span class="bt-form-check__title">扩展交易时段</span>
                              <span class="bt-form-check__hint">{{ extendedHoursHint }}</span>
                            </span>
                          </label>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label">预热K线</label>
                          <div class="bt-warmup-preview" :title="warmupPreviewNote">
                            <span class="bt-warmup-preview__value">{{ warmupPreviewValue }}</span>
                            <span class="bt-warmup-preview__note">{{ warmupPreviewNote }}</span>
                          </div>
                        </div>
                      </section>

                      <section class="grid gap-1.5 border-t bt-border pt-2">
                        <div class="flex items-center justify-between gap-2">
                          <div class="text-sm font-semibold bt-text-strong">资金与成本</div>
                          <div class="text-xs bt-text-muted">{{ costModeSummary }}</div>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-initial-balance">初始资金</label>
                          <span class="bt-input-suffix">
                            <input id="bt-field-initial-balance" v-model.number="initialBalance" type="number"
                              :min="1000" class="bt-native-input" />
                            <span class="bt-input-suffix__text">{{ quoteCurrency }}</span>
                          </span>
                        </div>
                        <div class="grid grid-cols-2 gap-2">
                          <div class="bt-form-row bt-form-row--compact">
                            <label class="bt-form-row__label" for="bt-field-broker-fee">券商费用</label>
                            <select id="bt-field-broker-fee" v-model="brokerFeeMode" class="bt-native-select">
                              <option v-for="option in BACKTEST_BROKER_FEE_MODE_OPTIONS" :key="option.value"
                                :value="option.value">
                                {{ option.title }}
                              </option>
                            </select>
                          </div>
                          <div class="bt-form-row bt-form-row--compact">
                            <label class="bt-form-row__label" for="bt-field-market-fee">市场费用</label>
                            <select id="bt-field-market-fee" v-model="marketFeeMode" class="bt-native-select">
                              <option v-for="option in BACKTEST_MARKET_FEE_MODE_OPTIONS" :key="option.value"
                                :value="option.value">
                                {{ option.title }}
                              </option>
                            </select>
                          </div>
                        </div>
                        <div v-if="brokerFeeMode === 'custom'" class="grid gap-1">
                          <label class="bt-form-row__label" for="bt-field-broker-rules">券商费用规则 JSON</label>
                          <textarea id="bt-field-broker-rules" v-model="brokerFeeRulesText" class="bt-native-textarea"
                            rows="3" />
                        </div>
                        <div v-if="marketFeeMode === 'custom'" class="grid gap-1">
                          <label class="bt-form-row__label" for="bt-field-market-rules">市场费用规则 JSON</label>
                          <textarea id="bt-field-market-rules" v-model="marketFeeRulesText" class="bt-native-textarea"
                            rows="3" />
                        </div>
                      </section>
                    </div>

                    <section class="bt-new-backtest-run grid gap-1.5">
                      <div class="bt-run-actions">

                        <!-- Sync section -->
                        <div v-if="syncing && !syncProgress" class="bt-sync-block bt-sync-block--pending">
                          <span>正在启动同步…</span>
                        </div>
                        <div v-else-if="syncing && syncProgress" class="bt-sync-block">
                          <div class="bt-sync-block__head">
                            <span class="bt-sync-block__title">
                              同步中 · {{ syncProgress.currentInterval || "准备" }}
                            </span>
                            <button class="bt-sync-block__cancel" type="button" @click="cancelSync">
                              取消
                            </button>
                          </div>
                          <div class="bt-sync-block__bar">
                            <div class="bt-sync-block__bar-fill" :style="{
                              width:
                                syncProgress.totalIntervals > 0
                                  ? (syncProgress.completedIntervals /
                                    syncProgress.totalIntervals) *
                                  100 +
                                  '%'
                                  : '10%',
                            }" />
                          </div>
                          <div class="bt-sync-block__meta">
                            <span>{{ syncProgress.completedBatches }} 批</span>
                            <span v-if="syncProgress.retries > 0" class="bt-text-queued">重试 {{ syncProgress.retries
                              }}</span>
                          </div>
                        </div>
                        <div v-else-if="syncProgress?.status === 'cancelled'"
                          class="bt-sync-block bt-sync-block--cancelled">
                          同步已取消 · {{ syncProgress.completedBatches }} 批已完成
                        </div>
                        <!-- Sync button -->
                        <button v-else class="bt-run-btn" :disabled="running || !instrumentSelectionResolved"
                          type="button" @click="syncKlines">
                          <v-icon size="13">fa-solid fa-cloud-arrow-down</v-icon>
                          同步K线
                        </button>

                        <!-- Run button -->
                        <button class="bt-run-btn bt-run-btn--primary"
                          :disabled="running || !selectedDefinitionId || !instrumentSelectionResolved" type="button"
                          @click="startBacktest">
                          <v-progress-circular v-if="running" indeterminate :size="16" :width="2" color="white" />
                          <v-icon v-else size="13">fa-solid fa-play</v-icon>
                          {{ running ? "启动中..." : "开始回测" }}
                        </button>
                      </div>

                    </section>
                  </div>
                </div>
              </section>

              <section class="bt-sidebar-panel bt-sidebar-panel--history"
                :class="{ 'is-expanded': expandedBacktestPanels.includes('history') }">
                <button type="button" class="bt-sidebar-panel__title" data-testid="backtest-side-panel-history-title"
                  :aria-expanded="expandedBacktestPanels.includes('history')" @click="toggleBacktestPanel('history')">
                  <v-icon size="11">fa-solid fa-chevron-right</v-icon>
                  <span>历史回测</span>
                  <em>{{ resultsPageSummary || `${filteredRuns.length} 条` }}</em>
                </button>
                <div v-if="expandedBacktestPanels.includes('history')"
                  class="bt-sidebar-panel__body bt-sidebar-panel__body--history">
                  <div class="bt-backtest-results-filters">
                    <input v-model="resultsSearchQuery" type="search" class="bt-native-input"
                      placeholder="搜索策略、标的、回测 ID" aria-label="搜索回测记录" />
                    <div class="grid grid-cols-2 gap-2">
                      <select v-model="resultsStatusFilter" class="bt-native-select" aria-label="按状态筛选">
                        <option v-for="option in BACKTEST_RESULT_STATUS_OPTIONS" :key="option.value"
                          :value="option.value">
                          {{ option.title }}
                        </option>
                      </select>
                      <select v-model="resultsStrategyFilter" class="bt-native-select" aria-label="按策略筛选">
                        <option v-for="option in resultStrategyOptions" :key="option.value" :value="option.value">
                          {{ option.title }}
                        </option>
                      </select>
                    </div>
                    <button type="button" class="bt-filter-reset" :disabled="!hasResultsFilters"
                      @click="resetResultsFilters">
                      清空筛选
                    </button>
                  </div>

                  <div v-if="filteredRuns.length === 0" :class="[emptyStateClass, 'p-6 text-center text-sm']">
                    {{ emptyResultsMessage }}
                  </div>
                  <div v-else class="bt-history-list">
                    <div v-for="run in pagedRuns" :key="run.id" class="bt-history-run cursor-pointer transition" :class="focusedRun && focusedRun.id === run.id
                      ? 'bt-history-run--selected'
                      : 'bt-history-run--idle bt-border bt-bg-surface'" role="button" tabindex="0"
                      @click="selectFocusedRun(run.id)" @keydown.enter.prevent="selectFocusedRun(run.id)"
                      @keydown.space.prevent="selectFocusedRun(run.id)">
                      <div class="flex items-start gap-2">
                        <div class="min-w-0 flex-1">
                          <div class="bt-history-run__title">
                            <span class="truncate">
                              {{ resolveStrategyName(run.request.definitionId) }} ·
                              <InstrumentIdentity :instrument-id="run.request.symbol" compact />
                            </span>
                            <span class="bt-history-run__status" :class="`is-${run.status}`">
                              {{ statusChip(run.status).label }}
                            </span>
                          </div>
                          <div class="bt-history-run__meta">
                            <span>{{ run.request.interval }}</span>
                            <span>{{ formatBacktestRunDate(run.request.startDate) }} → {{
                              formatBacktestRunDate(run.request.endDate)
                              }}</span>
                            <span v-if="run.request.definitionVersion">{{
                              formatStrategyVersion(run.request.definitionVersion)
                              }}</span>
                          </div>
                          <div class="bt-history-run__id" :title="run.id">
                            {{ run.id }} · {{ resolveRunSessionMode(run) }} · {{
                              formatBacktestRehabType(run.request.rehabType) }}
                          </div>
                          <div v-if="run.status === 'running' || run.status === 'queued'"
                            class="mt-2 flex items-center gap-3">
                            <v-progress-linear :color="run.status === 'running' ? 'teal' : 'warning'" indeterminate
                              rounded :height="6" class="flex-1" />
                            <span class="text-xs whitespace-nowrap shrink-0" :class="run.status === 'running'
                              ? 'bt-text-running'
                              : 'bt-text-queued'">
                              {{ run.status === "running" ? "回测运行中…" : "排队等待中…" }}
                            </span>
                          </div>
                        </div>
                        <v-btn v-if="isTerminalBacktestStatus(run.status)" icon="fa-solid fa-trash"
                          class="bt-history-run__delete" size="x-small" variant="text" color="error" title="删除回测结果"
                          @click.stop="requestDeleteRun(run.id)" />
                      </div>
                    </div>
                  </div>

                  <div v-if="resultsPageCount > 1" class="flex justify-center p-2">
                    <v-pagination v-model="resultsPage" class="bt-sidebar-pagination" :length="resultsPageCount"
                      :total-visible="3" density="comfortable" />
                  </div>
                </div>
              </section>
            </div>
          </div>
        </aside>
      </SplitPaneItem>

      <SplitPaneItem :size="backtestPaneSizes[1]" :min-size="45">
        <main class="backtest-page__pane">
          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <template v-if="reportMode === 'compare'">
              <section class="bt-version-comparison">
                <div class="bt-report-topbar">
                  <span class="bt-report-topbar__title">策略版本对比</span>
                </div>
                <div class="bt-version-comparison__body">
                  <div class="bt-version-compare-definition">
                    <div>
                      <label class="text-xs font-semibold bt-text-strong">策略定义</label>
                      <span>基线在左，候选在右；只使用各版本已完成的回测。</span>
                    </div>
                    <select :value="comparisonDefinitionId" class="bt-native-select"
                      data-testid="backtest-comparison-definition" aria-label="对比策略定义"
                      @change="changeComparisonDefinition(nativeSelectValue($event))">
                      <option value="" disabled>选择策略</option>
                      <option v-for="option in comparisonDefinitionOptions" :key="option.value" :value="option.value">
                        {{ option.title }}
                      </option>
                    </select>
                  </div>

                  <div v-if="isLoadingComparisonVersions" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    正在加载版本历史…
                  </div>
                  <div v-else-if="comparisonVersionsError"
                    class="bt-version-compare-notice bt-version-compare-notice--warning">
                    版本历史暂不可用：{{ comparisonVersionsError }}
                  </div>
                  <div v-else-if="comparisonDefinitionId === ''" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    请选择拥有版本历史的策略。
                  </div>
                  <div v-else-if="comparisonVersions.length < 2" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    至少需要两个已保存策略版本才能比较。
                  </div>
                  <template v-else>
                    <div class="bt-version-compare-selectors">
                      <section class="bt-version-compare-selector" data-testid="backtest-comparison-left">
                        <div class="bt-version-compare-selector__eyebrow">基线（较早版本）</div>
                        <select :value="leftComparisonVersion" class="bt-native-select"
                          data-testid="backtest-comparison-left-version" aria-label="选择基线版本"
                          @change="changeComparisonVersion('left', nativeSelectValue($event))">
                          <option value="" disabled>选择基线版本</option>
                          <option v-for="option in leftComparisonVersionSelectOptions" :key="option.value"
                            :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                        <div v-if="leftComparisonRuns.length === 0" class="bt-version-compare-selector__empty">
                          该版本暂无已完成回测。
                        </div>
                        <select v-else :value="leftComparisonRunId" class="bt-native-select"
                          data-testid="backtest-comparison-left-run" aria-label="关联回测"
                          @change="changeComparisonRun('left', nativeSelectValue($event))">
                          <option value="" disabled>关联回测</option>
                          <option v-for="option in leftComparisonRunOptions" :key="option.value" :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                      </section>
                      <section class="bt-version-compare-selector" data-testid="backtest-comparison-right">
                        <div class="bt-version-compare-selector__eyebrow">候选（较新版本）</div>
                        <select :value="rightComparisonVersion" class="bt-native-select"
                          data-testid="backtest-comparison-right-version" aria-label="选择候选版本"
                          @change="changeComparisonVersion('right', nativeSelectValue($event))">
                          <option value="" disabled>选择候选版本</option>
                          <option v-for="option in rightComparisonVersionSelectOptions" :key="option.value"
                            :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                        <div v-if="rightComparisonRuns.length === 0" class="bt-version-compare-selector__empty">
                          该版本暂无已完成回测。
                        </div>
                        <select v-else :value="rightComparisonRunId" class="bt-native-select"
                          data-testid="backtest-comparison-right-run" aria-label="关联回测"
                          @change="changeComparisonRun('right', nativeSelectValue($event))">
                          <option value="" disabled>关联回测</option>
                          <option v-for="option in rightComparisonRunOptions" :key="option.value" :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                      </section>
                    </div>

                    <div v-if="comparisonRunsReady && leftComparisonRun && rightComparisonRun"
                      class="bt-version-compare-results">
                      <div class="bt-version-compare-notice"
                        :class="comparisonConditionsMatch ? 'bt-version-compare-notice--ok' : 'bt-version-compare-notice--warning'">
                        <template v-if="comparisonConditionsMatch">
                          两次回测的配置一致，可将指标差异作为策略版本变化的参考。
                        </template>
                        <template v-else>
                          两次回测存在配置差异，结果不可直接归因于策略代码。请结合下方配置表评估。
                        </template>
                      </div>

                      <section class="bt-version-compare-section">
                        <div class="bt-version-compare-section__title">绩效指标</div>
                        <div class="bt-version-compare-metrics">
                          <div class="bt-version-compare-metrics__head">指标</div>
                          <div class="bt-version-compare-metrics__head">基线 v{{ leftComparisonVersion }}</div>
                          <div class="bt-version-compare-metrics__head">候选 v{{ rightComparisonVersion }}</div>
                          <div class="bt-version-compare-metrics__head">候选 − 基线</div>
                          <template v-for="metric in comparisonMetrics" :key="metric.label">
                            <div class="bt-version-compare-metrics__label">{{ metric.label }}</div>
                            <div>{{ formatComparisonMetric(metric.left, metric.kind,
                              resolveRunQuoteCurrency(leftComparisonRun)) }}</div>
                            <div>{{ formatComparisonMetric(metric.right, metric.kind,
                              resolveRunQuoteCurrency(rightComparisonRun)) }}</div>
                            <div>{{ comparisonMetricDelta(metric) }}</div>
                          </template>
                        </div>
                      </section>

                      <section class="bt-version-compare-section">
                        <div class="bt-version-compare-section__title">回测配置</div>
                        <div class="bt-version-compare-config">
                          <div class="bt-version-compare-config__head">字段</div>
                          <div class="bt-version-compare-config__head">基线</div>
                          <div class="bt-version-compare-config__head">候选</div>
                          <template v-for="row in comparisonConfigRows" :key="row.label">
                            <div class="bt-version-compare-config__label" :class="{ 'is-different': !row.same }">{{
                              row.label }}</div>
                            <div :class="{ 'is-different': !row.same }">{{ row.left }}</div>
                            <div :class="{ 'is-different': !row.same }">{{ row.right }}</div>
                          </template>
                        </div>
                      </section>
                    </div>
                    <div v-else class="bt-version-compare-notice bt-version-compare-notice--warning">
                      请选择两个版本各自的已完成回测后查看指标与配置对比。
                    </div>

                    <section class="bt-version-compare-section">
                      <div class="bt-version-compare-section__title">Pine 源码差异</div>
                      <StrategySourceDiff
                        v-if="comparisonSourcesReady && leftComparisonSnapshot && rightComparisonSnapshot"
                        :left-label="`基线 v${leftComparisonVersion}`" :right-label="`候选 v${rightComparisonVersion}`"
                        :left-source="leftComparisonSnapshot.script || ''"
                        :right-source="rightComparisonSnapshot.script || ''" />
                      <div v-else class="bt-version-compare-notice bt-version-compare-notice--warning">
                        <template v-if="comparisonSnapshotLoading.left || comparisonSnapshotLoading.right">
                          正在加载历史源码快照…
                        </template>
                        <template v-else-if="comparisonSnapshotErrors.left || comparisonSnapshotErrors.right">
                          策略版本快照不可用：{{ comparisonSnapshotErrors.left || comparisonSnapshotErrors.right
                          }}。升级前回测可能只保留指标和配置，无法伪造源码差异。
                        </template>
                        <template v-else>
                          选择两个不同版本后可查看只读源码差异。
                        </template>
                      </div>
                    </section>
                  </template>
                </div>
              </section>
            </template>
            <div v-else-if="!focusedRun" :class="[emptyStateClass, 'p-6 text-center text-sm']">
              {{ emptyResultsMessage }}
            </div>

            <template v-else>
              <section v-if="focusedRun" class="bt-report-workspace">
                <div class="bt-report-topbar">
                  <span class="bt-report-topbar__title">
                    {{ resolveStrategyName(focusedRun.request.definitionId) }} ·
                    <InstrumentIdentity :instrument-id="focusedRun.request.symbol" compact />
                  </span>
                  <span v-if="focusedRun.request.definitionVersion" class="bt-report-topbar__chip">
                    {{ formatStrategyVersion(focusedRun.request.definitionVersion) }}
                  </span>
                  <span class="bt-report-topbar__chip">{{ focusedRun.request.interval }}</span>
                  <span class="bt-report-topbar__chip bt-report-topbar__chip--status"
                    :class="`is-${focusedRun.status}`">
                    {{ statusChip(focusedRun.status).label }}
                  </span>
                </div>
                <div class="bt-report-context-bar">
                  <span class="bt-report-context-bar__id" :title="focusedRun.id">{{ focusedRun.id }}</span>
                  <span>{{ formatBacktestRunDate(focusedRun.request.startDate) }} → {{
                    formatBacktestRunDate(focusedRun.request.endDate) }}</span>
                  <span>{{ resolveRunSessionMode(focusedRun) }}</span>
                  <span>{{ formatBacktestRehabType(focusedRun.request.rehabType) }}</span>
                  <span>{{ focusedRun.request.initialBalance.toLocaleString() }} {{ resolveRunQuoteCurrency(focusedRun)
                    }}</span>
                </div>

                <div v-if="
                  focusedRun.status === 'running' ||
                  focusedRun.status === 'queued' ||
                  resolveBacktestStrategyVersionNotice(focusedRun) ||
                  detailLoading[focusedRun.id] ||
                  detailErrors[focusedRun.id]
                " class="bt-report-notices">
                  <div v-if="focusedRun.status === 'running' || focusedRun.status === 'queued'"
                    class="bt-report-notice flex items-center gap-3">
                    <v-progress-linear :color="focusedRun.status === 'running' ? 'teal' : 'warning'" indeterminate
                      rounded :height="4" class="flex-1" />
                    <span class="text-xs whitespace-nowrap shrink-0" :class="focusedRun.status === 'running'
                      ? 'bt-text-running'
                      : 'bt-text-queued'">
                      {{ focusedRun.status === "running" ? "回测运行中…" : "排队等待中…" }}
                    </span>
                  </div>
                  <div v-if="resolveBacktestStrategyVersionNotice(focusedRun)"
                    class="bt-report-notice bt-report-notice--warning">
                    {{ resolveBacktestStrategyVersionNotice(focusedRun) }}
                  </div>
                  <div v-if="detailLoading[focusedRun.id]" class="bt-report-notice">
                    正在加载完整回测详情…
                  </div>
                  <div v-if="detailErrors[focusedRun.id]" class="bt-report-notice bt-report-notice--error">
                    {{ detailErrors[focusedRun.id] }}
                  </div>
                </div>

                <div class="bt-report-summary">
                  <div v-if="focusedRunResultReady && focusedRun.result" class="bt-report-stats-grid">
                    <div class="bt-report-stat" data-testid="backtest-kpi-final-balance">
                      <div class="bt-report-stat__label">最终资金</div>
                      <div class="bt-report-stat__value bt-text">
                        {{ focusedRun.result.finalBalance.toLocaleString(undefined, { minimumFractionDigits: 2 }) }}
                      </div>
                      <div class="bt-report-stat__meta">{{ resolveRunQuoteCurrency(focusedRun) }}</div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-pnl">
                      <div class="bt-report-stat__label">收益</div>
                      <div class="bt-report-stat__value" :class="pnlColor(focusedRun.result.pnl)">
                        {{ pnlPrefix(focusedRun.result.pnl) }}{{ focusedRun.result.pnl.toLocaleString(undefined, {
                        minimumFractionDigits: 2 }) }}
                      </div>
                      <div class="bt-report-stat__meta">{{ resolveRunQuoteCurrency(focusedRun) }}</div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-trades">
                      <div class="bt-report-stat__label">
                        {{ usesClosedTradeStats(focusedRun.result) ? "已平仓交易" : "历史成交数" }}
                      </div>
                      <div class="bt-report-stat__value bt-text">{{ focusedRun.result.totalTrades }}</div>
                      <div v-if="usesClosedTradeStats(focusedRun.result)" class="bt-report-stat__meta">
                        成交 {{ backtestFillCount(focusedRun.result) }} 笔
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-win-rate">
                      <div class="bt-report-stat__label">
                        {{ usesClosedTradeStats(focusedRun.result) ? "已平仓胜率" : "历史胜率" }}
                      </div>
                      <div class="bt-report-stat__value bt-text">
                        {{ usesClosedTradeStats(focusedRun.result) ? `${(focusedRun.result.winRate * 100).toFixed(1)}%`
                        : "--" }}
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-max-drawdown">
                      <div class="bt-report-stat__label">最大回撤</div>
                      <div class="bt-report-stat__value" :class="drawdownColor(focusedRun.result.maxDrawdown)">
                        {{ formatPercentMetric(focusedRun.result.maxDrawdown) }}
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-current-drawdown">
                      <div class="bt-report-stat__label">当前回撤</div>
                      <div class="bt-report-stat__value" :class="drawdownColor(focusedRun.result.currentDrawdown)">
                        {{ formatPercentMetric(focusedRun.result.currentDrawdown) }}
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-broker-fees">
                      <div class="bt-report-stat__label">券商费用</div>
                      <div class="bt-report-stat__value bt-text">
                        {{ formatBacktestFee(focusedRun.result.totalBrokerFees, resolveRunQuoteCurrency(focusedRun)) }}
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-market-fees">
                      <div class="bt-report-stat__label">市场费用</div>
                      <div class="bt-report-stat__value bt-text">
                        {{ formatBacktestFee(focusedRun.result.totalMarketFees, resolveRunQuoteCurrency(focusedRun)) }}
                      </div>
                    </div>
                    <div class="bt-report-stat" data-testid="backtest-kpi-total-fees">
                      <div class="bt-report-stat__label">总费用</div>
                      <div class="bt-report-stat__value bt-text">
                        {{ formatBacktestFee(focusedRun.result.totalFees, resolveRunQuoteCurrency(focusedRun)) }}
                      </div>
                    </div>
                  </div>
                  <div v-else class="bt-report-summary__empty">
                    当前回测尚未生成完整报告。
                  </div>
                  <div
                    v-if="focusedRun.result && backtestFillCount(focusedRun.result) === 0 && !focusedRun.result.error"
                    class="bt-report-zero-trades">
                    未产生任何交易。可能原因：策略未调用 placeOrder()，或订阅的K线周期未同步。
                  </div>
                </div>

                <v-tabs v-model="activeReportTab" bg-color="transparent" density="compact"
                  class="bt-report-tabs shrink-0">
                  <v-tab value="chart">
                    <v-icon size="13" class="mr-1">fa-solid fa-chart-line</v-icon>
                    图表
                  </v-tab>
                  <v-tab value="orders">
                    <v-icon size="13" class="mr-1">fa-solid fa-list-check</v-icon>
                    订单
                  </v-tab>
                  <v-tab value="properties">
                    <v-icon size="13" class="mr-1">fa-solid fa-sliders</v-icon>
                    属性
                  </v-tab>
                </v-tabs>

                <v-window v-model="activeReportTab" class="bt-report-window min-h-0 flex-1 overflow-hidden">
                  <v-window-item value="chart" class="bt-report-window-item bt-report-window-item--chart">
                    <div class="bt-report-chart-tab flex h-full min-h-0 flex-col">
                      <div v-if="focusedRunResultReady && focusedRun.result"
                        class="bt-report-chart-stage min-h-0 flex-1">
                        <BacktestChart v-if="focusedRunHasChartData" :candles="focusedRun.result.candles ?? []"
                          :trades="focusedRun.result.trades ?? []" :pnl-curve="focusedRun.result.pnlCurve ?? []"
                          :drawdown-curve="focusedRun.result.drawdownCurve ?? []"
                          :initial-balance="focusedRun.request.initialBalance"
                          :chart-type="focusedRun.result.chartType ?? focusedRun.request.chartType"
                          :heikin-ashi-seed="focusedRun.result.heikinAshiSeed"
                          :currency-unit="resolveRunQuoteCurrency(focusedRun)" fit-container empty-text="暂无权益曲线数据" />
                        <div v-else
                          :class="[emptyStateClass, 'flex h-full min-h-[220px] items-center justify-center p-6 text-center text-sm']">
                          暂无权益曲线数据。
                        </div>
                      </div>
                      <div v-else
                        :class="[emptyStateClass, 'flex h-full min-h-[220px] items-center justify-center p-6 text-center text-sm']">
                        当前回测尚未生成完整报告。
                      </div>
                    </div>
                  </v-window-item>

                  <v-window-item value="orders" class="bt-report-window-item">
                    <div class="h-full min-h-0 overflow-auto p-2">
                      <div v-if="focusedRun.result?.orderBook?.length" :class="[cardBorderClass, 'overflow-hidden']">
                        <div
                          class="flex items-center justify-between border-b bt-border px-3 py-2 text-sm font-semibold bt-text">
                          <span>订单</span>
                          <span class="text-xs font-medium bt-text-muted">
                            {{ focusedRun.result.orderBook.length }} 笔
                          </span>
                        </div>
                        <div class="bt-order-table-scroll max-h-[520px] overflow-auto">
                          <table class="bt-order-table min-w-full divide-y bt-divide text-sm">
                            <thead
                              class="sticky top-0 bt-bg-muted text-left text-xs uppercase tracking-[0.14em] bt-text-muted">
                              <tr>
                                <th class="px-3 py-1.5 font-medium">下单</th>
                                <th class="px-3 py-1.5 font-medium">成交</th>
                                <th class="px-3 py-1.5 font-medium">方向</th>
                                <th class="px-3 py-1.5 font-medium">数量</th>
                                <th class="px-3 py-1.5 font-medium">委托价</th>
                                <th class="px-3 py-1.5 font-medium">成交价</th>
                                <th class="px-3 py-1.5 font-medium">费用</th>
                                <th class="px-3 py-1.5 font-medium">状态</th>
                              </tr>
                            </thead>
                            <tbody class="divide-y bt-divide-soft bt-bg-surface">
                              <tr v-for="(entry, index) in visibleBacktestOrderBook(focusedRun)"
                                :key="`${entry.orderId || index}-${entry.filledAt ?? entry.submittedAt ?? ''}`">
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  <div>{{ formatBacktestTimestamp(entry.submittedAt) }}</div>
                                  <div class="mt-0.5 text-xs bt-text-dim">
                                    #{{ entry.orderId }}<span v-if="entry.clientOrderId"> · {{ entry.clientOrderId
                                      }}</span>
                                  </div>
                                </td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">{{
                                  formatBacktestTimestamp(entry.filledAt) }}</td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">{{ formatBacktestOrderSide(entry.side)
                                  }}</td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  <div>{{ formatBacktestQuantity(entry.quantity, entry.quantityText) }}</div>
                                  <div v-if="entry.filledQuantity !== undefined" class="mt-0.5 text-xs bt-text-dim">
                                    成交 {{ formatBacktestQuantity(entry.filledQuantity, entry.filledQuantityText) }}
                                  </div>
                                </td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  {{ formatBacktestOrderPrice(entry.orderPrice, entry.orderType, entry.orderPriceText)
                                  }}
                                </td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  {{ formatBacktestOrderPrice(entry.filledPrice, undefined, entry.filledPriceText) }}
                                </td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  <div>{{ formatBacktestFee(entry.totalFee, entry.feeCurrency) }}</div>
                                  <div v-if="entry.totalFee" class="mt-0.5 text-xs bt-text-dim">
                                    券商 {{ formatBacktestFee(entry.brokerFee, entry.feeCurrency) }} ｜ 市场
                                    {{ formatBacktestFee(entry.marketFee, entry.feeCurrency) }}
                                  </div>
                                </td>
                                <td class="px-3 py-1.5 align-top bt-text-strong">
                                  {{ formatBacktestOrderStatus(entry.status) }}
                                  <span v-if="entry.warmup" class="bt-warmup-label">
                                    预热
                                  </span>
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                        <div v-if="hiddenBacktestOrderBookCount(focusedRun) > 0"
                          class="border-t bt-border px-4 py-2 text-xs bt-text-muted">
                          另有 {{ hiddenBacktestOrderBookCount(focusedRun) }} 笔订单。
                        </div>
                      </div>
                      <div v-else :class="[emptyStateClass, 'p-6 text-center text-sm']">
                        暂无订单记录。
                      </div>
                    </div>
                  </v-window-item>

                  <v-window-item value="properties" class="bt-report-window-item">
                    <div class="grid h-full min-h-0 gap-2 overflow-auto p-2">
                      <div v-if="focusedRun.result"
                        class="rounded-lg border bt-border bt-bg-muted px-2.5 py-1.5 text-xs bt-text">
                        <div>{{ resolveBacktestPriceBasisNote(focusedRun) }}</div>
                        <div class="mt-1">
                          费用口径：券商 {{ focusedRun.result.tradingCosts?.brokerFees?.mode ?? "market_preset" }} ｜ 市场
                          {{ focusedRun.result.tradingCosts?.marketFees?.mode ?? "market_preset" }}
                        </div>
                        <div v-if="resolveQueriedCandleBounds(focusedRun.result?.candles)" class="mt-1">
                          查询到的周期边界：左边界
                          {{ resolveQueriedCandleBounds(focusedRun.result?.candles)?.left }} ｜
                          右边界 {{ resolveQueriedCandleBounds(focusedRun.result?.candles)?.right }} ｜
                          共 {{ resolveQueriedCandleBounds(focusedRun.result?.candles)?.count }} 根
                        </div>
                      </div>

                      <details v-if="focusedRun.result?.runtimeErrors && focusedRun.result.runtimeErrors.length > 0"
                        class="bt-prop-block bt-prop-block--error">
                        <summary class="bt-prop-block__summary">
                          <v-icon size="13">fa-solid fa-circle-exclamation</v-icon>
                          {{ runtimeErrorSummary(focusedRun.result) }}
                        </summary>
                        <div class="mt-1.5 space-y-1 max-h-48 overflow-y-auto">
                          <div v-for="(err, i) in visibleBacktestRuntimeErrors(focusedRun)" :key="i"
                            class="bt-prop-block__item">
                            <span v-if="runtimeErrorRepeatCount(focusedRun.result, err) > 1" class="font-semibold">
                              x{{ runtimeErrorRepeatCount(focusedRun.result, err) }}
                            </span>
                            {{ err }}
                          </div>
                          <div v-if="hiddenBacktestRuntimeErrorCount(focusedRun) > 0" class="bt-prop-block__more">
                            另有 {{ hiddenBacktestRuntimeErrorCount(focusedRun) }} 条错误。
                          </div>
                        </div>
                      </details>

                      <details v-if="focusedRun.result?.warnings && focusedRun.result.warnings.length > 0"
                        class="bt-prop-block bt-prop-block--warning">
                        <summary class="bt-prop-block__summary">
                          <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
                          {{ warningSummary(focusedRun.result) }}
                        </summary>
                        <div class="mt-1.5 space-y-1 max-h-48 overflow-y-auto">
                          <div v-for="(warning, i) in visibleBacktestWarnings(focusedRun)" :key="i"
                            class="bt-prop-block__item">
                            {{ warning }}
                          </div>
                          <div v-if="hiddenBacktestWarningCount(focusedRun) > 0" class="bt-prop-block__more">
                            另有 {{ hiddenBacktestWarningCount(focusedRun) }} 条警告。
                          </div>
                        </div>
                      </details>

                      <div v-if="focusedRun.result?.logs && focusedRun.result.logs.length > 0"
                        class="bt-prop-block bt-prop-block--warning space-y-1">
                        <div v-for="(log, i) in visibleBacktestLogs(focusedRun)" :key="i" class="flex gap-2">
                          <v-icon size="12" class="mt-0.5">fa-solid fa-circle-info</v-icon>
                          <span>{{ log }}</span>
                        </div>
                        <div v-if="hiddenBacktestLogCount(focusedRun) > 0">
                          另有 {{ hiddenBacktestLogCount(focusedRun) }} 条日志。
                        </div>
                      </div>

                      <div v-if="focusedRun.result?.error"
                        class="bt-prop-block bt-prop-block--error whitespace-pre-wrap">
                        {{ focusedRun.result.error }}
                      </div>

                      <div v-if="!focusedRun.result" :class="[emptyStateClass, 'p-6 text-center text-sm']">
                        暂无属性。
                      </div>
                    </div>
                  </v-window-item>
                </v-window>
              </section>

            </template>
          </div>
        </main>
      </SplitPaneItem>
    </SplitPane>
    <ActionConfirmDialog :open="pendingDeleteRun != null" title="删除回测记录" :message="pendingDeleteMessage"
      confirm-label="确认删除" :busy="deletingRunId !== ''" @close="pendingDeleteRunId = ''" @confirm="confirmDeleteRun" />
  </div>
</template>

<style scoped>
.backtest-page {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 0;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.backtest-workbench-header {
  display: flex;
  min-width: 0;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.backtest-workbench-header__identity,
.backtest-workbench-title,
.backtest-workbench-header__actions {
  display: flex;
  min-width: 0;
  align-items: center;
}

.backtest-workbench-header__identity {
  flex: 1 1 auto;
  gap: 6px;
  overflow: hidden;
}

.backtest-workbench-title {
  flex: 0 1 auto;
  gap: 7px;
  overflow: hidden;
}

.backtest-workbench-title h1 {
  flex: 0 0 auto;
  margin: 0;
  font-size: 0.92rem;
  line-height: 1;
  white-space: nowrap;
}

.backtest-workbench-header__actions {
  flex: 0 0 auto;
  justify-content: flex-end;
  gap: 4px;
}

.backtest-sidebar-toggle,
.backtest-header-action,
.backtest-header-icon-button,
.backtest-report-mode-switch__button {
  min-height: 30px;
  height: 30px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-size: 0.77rem;
  font-weight: 750;
}

.backtest-sidebar-toggle,
.backtest-header-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 0 8px;
}

.backtest-sidebar-toggle {
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.backtest-sidebar-toggle:is(:hover, .is-active) {
  border-color: var(--tv-border);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.backtest-header-action--primary {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface));
  color: var(--tv-accent);
}

.backtest-header-icon-button {
  display: inline-grid;
  width: 30px;
  place-items: center;
  padding: 0;
}

.backtest-header-icon-button--danger {
  border-color: transparent;
  background: transparent;
  color: var(--tv-status-error-fg);
}

.backtest-report-mode-switch {
  display: inline-grid;
  grid-template-columns: repeat(2, minmax(4rem, 1fr));
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 2px;
}

.backtest-report-mode-switch__button {
  min-width: 4rem;
  min-height: 26px;
  height: 26px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  padding: 0 7px;
  color: var(--tv-text-muted);
}

.backtest-report-mode-switch__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 22%, var(--tv-bg-surface));
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 36%, transparent);
}

.backtest-error-banner {
  display: flex;
  min-width: 0;
  min-height: 30px;
  flex: 0 0 30px;
  align-items: center;
  gap: 7px;
  border: 0;
  border-bottom: 1px solid color-mix(in srgb, #ef4444 42%, var(--tv-border));
  border-radius: 0;
  background: color-mix(in srgb, #ef4444 9%, var(--tv-bg-surface));
  padding: 0 4px 0 8px;
  color: color-mix(in srgb, #fca5a5 78%, var(--tv-text));
  font-size: 0.76rem;
  text-align: left;
}

.backtest-error-banner__content {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 7px;
  align-self: stretch;
  border: 0;
  background: transparent;
  color: inherit;
  padding: 5px 0;
  font-size: inherit;
  text-align: left;
}

.backtest-error-banner__content>span {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.backtest-error-banner.is-expanded {
  min-height: 30px;
  flex-basis: auto;
}

.backtest-error-banner.is-expanded .backtest-error-banner__content>span {
  overflow: visible;
  white-space: normal;
}

.backtest-error-banner__close {
  display: inline-grid;
  width: 24px;
  min-height: 24px;
  flex: 0 0 24px;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
}

.backtest-page__mobile-switch {
  display: none;
}

.backtest-page__split {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.backtest-page__split :deep(.splitpanes__pane) {
  min-width: 0;
  overflow: hidden;
}

.backtest-page__pane {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.backtest-page__pane>* {
  min-width: 0;
}

.backtest-page__pane--sidebar {
  container-type: inline-size;
  background: var(--tv-bg-surface);
}

.bt-sidebar-shell,
.bt-sidebar-panels {
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  overflow: hidden;
}

.bt-sidebar-drawer-head {
  display: none;
  min-height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.bt-sidebar-drawer-head>div {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.bt-sidebar-drawer-head strong {
  color: var(--tv-text);
  font-size: 0.8rem;
}

.bt-sidebar-drawer-head span {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sidebar-drawer-head button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
}

.bt-sidebar-panel {
  display: flex;
  min-width: 0;
  min-height: 34px;
  flex: 0 0 34px;
  flex-direction: column;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.bt-sidebar-panel.is-expanded {
  min-height: 96px;
  flex: 1 1 0;
}

.bt-sidebar-panel--setup.is-expanded+.bt-sidebar-panel--history.is-expanded {
  flex-grow: 1.25;
}

.bt-sidebar-panel__title {
  display: grid;
  width: 100%;
  min-width: 0;
  min-height: 34px;
  flex: 0 0 34px;
  grid-template-columns: 12px minmax(0, 1fr) auto;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-surface);
  padding: 0 8px;
  color: var(--tv-text-muted);
  text-align: left;
}

.bt-sidebar-panel__title:hover {
  background: var(--tv-bg-elevated);
}

.bt-sidebar-panel__title>.v-icon {
  transform: rotate(0deg);
  transition: transform 120ms ease;
}

.bt-sidebar-panel.is-expanded>.bt-sidebar-panel__title>.v-icon {
  transform: rotate(90deg);
}

.bt-sidebar-panel__title span {
  font-size: 0.78rem;
  font-weight: 750;
  letter-spacing: 0.04em;
}

.bt-sidebar-panel__title em {
  max-width: 12rem;
  overflow: hidden;
  font-size: 0.67rem;
  font-style: normal;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sidebar-panel__body {
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.bt-sidebar-panel__body--setup {
  overflow: hidden;
}

.bt-new-backtest-form {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
}

.bt-new-backtest-fields {
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.bt-new-backtest-fields>section,
.bt-new-backtest-run {
  gap: 6px !important;
  padding: 8px;
}

.bt-new-backtest-fields>section+section,
.bt-new-backtest-run {
  border-top: 1px solid var(--tv-border);
}

.bt-new-backtest-fields>section> :is(.text-sm, .flex:first-child) {
  min-height: 24px;
}

.bt-new-backtest-run {
  flex: 0 0 auto;
  background: var(--tv-bg-surface);
  box-shadow: 0 -8px 18px color-mix(in srgb, var(--tv-bg-app) 42%, transparent);
}

.bt-run-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.bt-run-actions>div {
  grid-column: 1 / -1;
}

.bt-run-actions>div+button:last-child {
  grid-column: 1 / -1;
}

.bt-run-btn {
  display: inline-flex;
  min-width: 0;
  min-height: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid color-mix(in srgb, #14b8a6 45%, var(--tv-border));
  border-radius: 5px;
  background: color-mix(in srgb, #14b8a6 12%, var(--tv-bg-surface));
  color: #2dd4bf;
  padding: 0 10px;
  font-size: 0.75rem;
  font-weight: 750;
  cursor: pointer;
  transition: background 120ms ease, border-color 120ms ease;
}

.bt-run-btn:hover:not(:disabled) {
  border-color: color-mix(in srgb, #14b8a6 62%, var(--tv-border));
  background: color-mix(in srgb, #14b8a6 18%, var(--tv-bg-surface));
}

.bt-run-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.bt-run-btn--primary {
  border-color: color-mix(in srgb, #14b8a6 62%, var(--tv-border));
  background: color-mix(in srgb, #14b8a6 68%, var(--tv-bg-surface));
  color: #f0fdfa;
}

.bt-run-btn--primary:hover:not(:disabled) {
  background: color-mix(in srgb, #14b8a6 78%, var(--tv-bg-surface));
}

.bt-run-btn--primary:disabled {
  border-color: var(--tv-border);
  background: color-mix(in srgb, var(--tv-bg-elevated) 60%, var(--tv-border) 40%);
  color: var(--tv-text-dim);
  opacity: 1;
}

.bt-sync-block {
  display: grid;
  min-width: 0;
  gap: 6px;
  border: 1px solid color-mix(in srgb, #14b8a6 40%, var(--tv-border));
  border-radius: 6px;
  background: color-mix(in srgb, #14b8a6 8%, var(--tv-bg-surface));
  padding: 6px 8px;
  color: #2dd4bf;
  font-size: 0.72rem;
}

.bt-sync-block--pending {
  place-items: center;
  text-align: center;
}

.bt-sync-block__head,
.bt-sync-block__meta {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.bt-sync-block__title {
  min-width: 0;
  overflow: hidden;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sync-block__cancel {
  flex: 0 0 auto;
  border: 1px solid color-mix(in srgb, #ef4444 45%, var(--tv-border));
  border-radius: 999px;
  background: transparent;
  color: color-mix(in srgb, #fca5a5 78%, var(--tv-text));
  padding: 1px 8px;
  font-size: 0.68rem;
  cursor: pointer;
}

.bt-sync-block__cancel:hover {
  background: color-mix(in srgb, #ef4444 10%, var(--tv-bg-surface));
}

.bt-sync-block__bar {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: color-mix(in srgb, #14b8a6 22%, var(--tv-bg-surface));
}

.bt-sync-block__bar-fill {
  height: 100%;
  border-radius: 999px;
  background: #14b8a6;
  transition: width 500ms ease;
}

.bt-sync-block--cancelled {
  border-color: color-mix(in srgb, #f59e0b 44%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
}

.bt-backtest-results-filters {
  display: grid;
  gap: 6px;
  padding: 8px;
  border-bottom: 1px solid var(--tv-border);
}

.bt-filter-reset {
  justify-self: end;
  min-height: 24px;
  border: 0;
  background: transparent;
  color: var(--tv-accent);
  padding: 0 4px;
  font-size: 0.7rem;
  font-weight: 700;
}

.bt-filter-reset:disabled {
  color: var(--tv-text-dim);
  opacity: 0.5;
}

.bt-history-list {
  display: grid;
  gap: 0;
}

.bt-history-run {
  min-width: 0;
  border-width: 0 0 1px;
  border-style: solid;
  border-radius: 0;
  padding: 8px;
}

.bt-history-run__title {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  color: var(--tv-text);
  font-size: 0.78rem;
  font-weight: 750;
}

.bt-history-run__status {
  display: inline-flex;
  min-height: 18px;
  flex: 0 0 auto;
  align-items: center;
  border-radius: 999px;
  background: var(--tv-bg-elevated);
  padding: 0 6px;
  color: var(--tv-text-muted);
  font-size: 0.64rem;
  font-weight: 750;
}

.bt-history-run__status.is-running {
  color: #2dd4bf;
}

.bt-history-run__status.is-failed,
.bt-history-run__status.is-cancelled {
  color: #f87171;
}

.bt-history-run__meta,
.bt-history-run__id {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.67rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-history-run__meta {
  display: flex;
  gap: 7px;
  margin-top: 3px;
}

.bt-history-run__id {
  margin-top: 2px;
  color: var(--tv-text-dim);
}

.bt-history-run__delete {
  flex: 0 0 auto;
  opacity: 0;
  transition: opacity 120ms ease;
}

.bt-history-run:is(:hover, :focus-within) .bt-history-run__delete {
  opacity: 1;
}

.backtest-page__pane--sidebar input {
  min-width: 0;
  max-width: 100%;
}

.backtest-page__pane--sidebar .grid {
  min-width: 0;
}

.backtest-page__pane--sidebar .grid>*,
.backtest-page__pane--sidebar .flex>* {
  min-width: 0;
}

.bt-form-row {
  display: grid;
  min-width: 0;
  grid-template-columns: 76px minmax(0, 1fr);
  align-items: center;
  gap: 6px;
}

.bt-form-row--compact {
  grid-template-columns: auto minmax(0, 1fr);
}

.bt-form-row__label {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-overflow: ellipsis;
  text-transform: uppercase;
  white-space: nowrap;
}

.bt-native-input,
.bt-native-select,
.bt-native-textarea {
  width: 100%;
  min-width: 0;
  min-height: 32px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 5px 8px;
  font-size: 0.83rem;
  line-height: 1.25;
  outline: none;
}

.bt-native-input:focus,
.bt-native-select:focus,
.bt-native-textarea:focus {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

.bt-native-input:disabled,
.bt-native-select:disabled,
.bt-native-textarea:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.bt-native-textarea {
  min-height: 56px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  resize: vertical;
}

.bt-input-suffix {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.bt-input-suffix__text {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
  font-size: 0.7rem;
}

.bt-inline-warning {
  border: 1px solid color-mix(in srgb, #f59e0b 44%, var(--tv-border));
  border-radius: 4px;
  background: color-mix(in srgb, #f59e0b 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
  padding: 4px 8px;
  font-size: 0.72rem;
  line-height: 1.35;
}

.bt-form-check {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
  cursor: pointer;
}

.bt-form-check__input {
  width: auto;
  min-height: 0;
  margin-top: 2px;
  accent-color: #14b8a6;
  cursor: pointer;
}

.bt-form-check__title {
  display: block;
  color: var(--tv-text);
  font-size: 0.72rem;
  font-weight: 700;
}

.bt-form-check__hint {
  display: block;
  color: var(--tv-text-dim);
  font-size: 0.68rem;
  line-height: 1.35;
}

.bt-warmup-preview {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  padding: 4px 8px;
}

.bt-warmup-preview__value {
  flex: 0 0 auto;
  color: var(--tv-text);
  font-size: 0.78rem;
}

.bt-warmup-preview__note {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 0.68rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-text-running {
  color: #2dd4bf;
}

.bt-text-queued {
  color: #fbbf24;
}

.bt-sidebar-header {
  min-width: 0;
}

.bt-sidebar-create-action {
  flex: 0 0 auto;
}

.bt-sidebar-pagination {
  max-width: 100%;
  min-width: 0;
}

.bt-sidebar-pagination :deep(.v-pagination__list) {
  flex-wrap: wrap;
  justify-content: center;
  max-width: 100%;
  min-width: 0;
}

.bt-sidebar-pagination :deep(.v-btn) {
  height: 30px;
  min-width: 30px;
  width: 30px;
}

.backtest-page__pane--sidebar .bt-text-muted,
.backtest-page__pane--sidebar .bt-text-dim,
.backtest-page__pane--sidebar .bt-text,
.backtest-page__pane--sidebar .bt-text-strong,
.backtest-page .bt-text-muted,
.backtest-page .bt-text-dim,
.backtest-page .bt-text,
.backtest-page .bt-text-strong {
  overflow-wrap: anywhere;
}

.bt-report-workspace {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-app);
  container-type: inline-size;
}

.bt-report-topbar {
  display: flex;
  min-width: 0;
  min-height: 34px;
  flex: 0 0 34px;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  padding: 0 8px;
  white-space: nowrap;
}

.bt-report-topbar__title {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 4px;
  overflow: hidden;
  color: var(--tv-text);
  font-size: 0.8rem;
  font-weight: 650;
  text-overflow: ellipsis;
}

.bt-report-topbar__chip {
  display: inline-flex;
  min-height: 20px;
  flex: 0 0 auto;
  align-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 1px 6px;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 650;
  line-height: 1;
  white-space: nowrap;
}

.bt-report-topbar__chip--status.is-running {
  border-color: color-mix(in srgb, #14b8a6 52%, var(--tv-border));
  color: #2dd4bf;
}

.bt-report-topbar__chip--status.is-failed,
.bt-report-topbar__chip--status.is-cancelled {
  border-color: color-mix(in srgb, #ef4444 52%, var(--tv-border));
  color: #f87171;
}

.bt-report-context-bar {
  display: flex;
  min-width: 0;
  min-height: 30px;
  flex: 0 0 30px;
  align-items: center;
  gap: 10px;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  padding: 0 8px;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  white-space: nowrap;
}

.bt-report-context-bar__id {
  max-width: 15rem;
  overflow: hidden;
  color: var(--tv-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  text-overflow: ellipsis;
}

.bt-report-notices {
  display: grid;
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tv-border);
}

.bt-report-notice {
  min-width: 0;
  min-height: 28px;
  padding: 5px 8px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  line-height: 1.35;
}

.bt-report-notice+.bt-report-notice {
  border-top: 1px solid var(--tv-border);
}

.bt-report-notice--warning,
.bt-report-zero-trades {
  background: color-mix(in srgb, #f59e0b 8%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
}

.bt-report-notice--error {
  background: color-mix(in srgb, #ef4444 8%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fca5a5 76%, var(--tv-text));
}

.bt-prop-block {
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 0.72rem;
  line-height: 1.45;
}

.bt-prop-block--error {
  border-color: color-mix(in srgb, #ef4444 42%, var(--tv-border));
  background: color-mix(in srgb, #ef4444 8%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fca5a5 78%, var(--tv-text));
}

.bt-prop-block--warning {
  border-color: color-mix(in srgb, #f59e0b 42%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 8%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 74%, var(--tv-text));
}

.bt-prop-block__summary {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.72rem;
  font-weight: 700;
  cursor: pointer;
  user-select: none;
}

.bt-prop-block__item {
  border: 1px solid color-mix(in srgb, currentColor 22%, transparent);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  padding: 4px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.7rem;
  line-height: 1.5;
}

.bt-prop-block__more {
  font-size: 0.7rem;
}

.bt-warmup-label {
  margin-left: 4px;
  color: #fbbf24;
  font-size: 0.72rem;
  font-weight: 500;
}

.bt-report-summary {
  flex: 0 0 auto;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.bt-report-summary__empty {
  min-height: 52px;
  padding: 16px;
  color: var(--tv-text-muted);
  font-size: 0.76rem;
  text-align: center;
}

.bt-report-zero-trades {
  min-height: 28px;
  border-top: 1px solid color-mix(in srgb, #f59e0b 30%, var(--tv-border));
  padding: 5px 8px;
  font-size: 0.7rem;
}

.bt-report-window,
.bt-report-window-item,
.bt-report-chart-tab {
  min-width: 0;
  min-height: 0;
}

.bt-report-chart-tab {
  height: 100%;
  padding: 4px;
}

.bt-report-chart-stage {
  min-width: 0;
  min-height: 0;
}

.bt-report-stats-grid {
  display: grid;
  grid-template-columns: repeat(9, minmax(0, 1fr));
  min-width: 0;
}

.bt-report-stat {
  display: grid;
  min-width: 0;
  min-height: 50px;
  align-content: center;
  gap: 2px;
  border-right: 1px solid var(--tv-border);
  padding: 4px 8px;
}

.bt-report-stat:last-child {
  border-right: 0;
}

.bt-report-stat__label {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.61rem;
  font-weight: 750;
  letter-spacing: 0.06em;
  text-overflow: ellipsis;
  text-transform: uppercase;
  white-space: nowrap;
}

.bt-report-stat__value {
  overflow: hidden;
  font-size: 0.88rem;
  font-weight: 500;
  line-height: 1.15;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-report-stat__meta {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 0.62rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@container (max-width: 900px) {
  .bt-report-stats-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }

  .bt-report-stat {
    border-bottom: 1px solid var(--tv-border);
  }

  .bt-report-stat:nth-child(5n) {
    border-right: 0;
  }

  .bt-report-stat:nth-child(n + 6) {
    border-bottom: 0;
  }
}

@container (max-width: 560px) {
  .bt-report-stats-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .bt-report-stat:nth-child(5n) {
    border-right: 1px solid var(--tv-border);
  }

  .bt-report-stat:nth-child(3n) {
    border-right: 0;
  }

  .bt-report-stat:nth-child(n + 6) {
    border-bottom: 1px solid var(--tv-border);
  }

  .bt-report-stat:nth-child(n + 7) {
    border-bottom: 0;
  }
}

@container (max-width: 360px) {
  .bt-report-stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .bt-report-stat:nth-child(3n) {
    border-right: 1px solid var(--tv-border);
  }

  .bt-report-stat:nth-child(2n) {
    border-right: 0;
  }

  .bt-report-stat:nth-child(n + 7) {
    border-bottom: 1px solid var(--tv-border);
  }

  .bt-report-stat:nth-child(n + 9) {
    border-bottom: 0;
  }
}

.bt-report-window :deep(.v-window__container),
.bt-report-window :deep(.v-window-item),
.bt-report-window :deep(.v-window-item--active) {
  min-width: 0;
  height: 100%;
  min-height: 0;
}

.bt-report-tabs {
  min-height: 32px;
  height: 32px;
  max-width: 100%;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.bt-report-tabs :deep(.v-slide-group__content) {
  height: 32px;
}

.bt-report-tabs :deep(.v-tab) {
  min-height: 32px;
  height: 32px;
  padding-inline: 10px;
  font-size: 0.73rem;
}

.bt-report-tabs :deep(.v-slide-group__container) {
  min-width: 0;
  overflow-x: auto;
}

.bt-order-table-scroll {
  max-width: 100%;
  min-width: 0;
  overscroll-behavior: contain;
}

.bt-order-table {
  min-width: 48rem;
}

.bt-version-comparison {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  overflow: auto;
  background: var(--tv-bg-app);
}

.bt-version-comparison__body {
  display: grid;
  gap: 8px;
  padding: 8px;
}

.bt-version-compare-definition {
  display: grid;
  grid-template-columns: minmax(14rem, 1fr) minmax(16rem, 24rem);
  align-items: center;
  gap: 8px;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding-bottom: 8px;
}

.bt-version-compare-definition>div {
  display: grid;
  gap: 2px;
}

.bt-version-compare-definition>div>span {
  color: var(--tv-text-muted);
  font-size: 0.68rem;
}

.bt-version-compare-selectors {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.bt-version-compare-selector {
  display: grid;
  align-content: start;
  gap: 6px;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 62%, transparent);
  padding: 8px;
}

.bt-version-compare-selector__eyebrow,
.bt-version-compare-section__title {
  color: var(--tv-text-muted);
  font-size: 0.75rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.bt-version-compare-selector__empty {
  border: 1px dashed var(--tv-border);
  border-radius: 0.4rem;
  color: var(--tv-text-muted);
  padding: 0.6rem;
  font-size: 0.78rem;
}

.bt-version-compare-results,
.bt-version-compare-section {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.bt-version-compare-notice {
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 70%, transparent);
  color: var(--tv-text-muted);
  padding: 6px 8px;
  font-size: 0.73rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.bt-version-compare-notice--warning {
  border-color: color-mix(in srgb, #f59e0b 46%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
}

.bt-version-compare-notice--ok {
  border-color: color-mix(in srgb, #22c55e 48%, var(--tv-border));
  background: color-mix(in srgb, #22c55e 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #86efac 72%, var(--tv-text));
}

.bt-version-compare-metrics {
  display: grid;
  grid-template-columns: minmax(7rem, 1fr) repeat(3, minmax(8rem, 1fr));
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  font-size: 0.76rem;
}

.bt-version-compare-metrics>div {
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding: 4px 6px;
  overflow-wrap: anywhere;
}

.bt-version-compare-metrics>div:nth-last-child(-n + 4) {
  border-bottom: 0;
}

.bt-version-compare-metrics__head,
.bt-version-compare-config__head {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 800;
}

.bt-version-compare-metrics__label,
.bt-version-compare-config__label {
  color: var(--tv-text);
  font-weight: 700;
}

.bt-version-compare-config {
  display: grid;
  grid-template-columns: minmax(6rem, 0.7fr) repeat(2, minmax(10rem, 1fr));
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  font-size: 0.74rem;
}

.bt-version-compare-config>div {
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding: 4px 6px;
  overflow-wrap: anywhere;
}

.bt-version-compare-config>div:nth-last-child(-n + 3) {
  border-bottom: 0;
}

.bt-version-compare-config .is-different {
  background: color-mix(in srgb, #f59e0b 8%, var(--tv-bg-surface));
  color: var(--tv-text);
}

@container (max-width: 360px) {
  .backtest-page__pane--sidebar .grid-cols-2 {
    grid-template-columns: minmax(0, 1fr) !important;
  }

  .bt-sidebar-header {
    align-items: stretch;
    flex-direction: column;
  }

  .bt-sidebar-create-action {
    width: 100%;
  }
}

@container (max-width: 320px) {
  .bt-sidebar-pagination :deep(.v-btn) {
    height: 28px;
    min-width: 28px;
    width: 28px;
  }
}

@media (min-width: 1181px) {

  .backtest-page--sidebar-closed .backtest-page__split> :deep(.splitpanes__pane:first-of-type),
  .backtest-page--sidebar-closed .backtest-page__split> :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }

  .backtest-page--sidebar-closed .backtest-page__split> :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }
}

.backtest-sidebar-backdrop {
  position: absolute;
  z-index: 20;
  inset: 44px 0 0;
  border: 0;
  border-radius: 0;
  background: rgba(2, 6, 23, 0.38);
  padding: 0;
}

@media (min-width: 769px) and (max-width: 1180px) {
  .backtest-workbench-header {
    gap: 4px;
    padding-inline: 6px;
  }

  .backtest-page__split> :deep(.splitpanes__pane:first-of-type) {
    position: absolute !important;
    z-index: 30;
    inset: 0 auto 0 0;
    width: min(380px, calc(100% - 48px)) !important;
    max-width: min(380px, calc(100% - 48px)) !important;
    min-width: min(300px, calc(100% - 48px)) !important;
    flex: 0 0 min(380px, calc(100% - 48px)) !important;
    transform: translateX(0);
    transition: transform 160ms ease;
    box-shadow: 16px 0 36px rgba(2, 6, 23, 0.3);
  }

  .backtest-page__split> :deep(.splitpanes__splitter) {
    display: none;
  }

  .backtest-page__split> :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .backtest-page--sidebar-closed .backtest-page__split> :deep(.splitpanes__pane:first-of-type) {
    pointer-events: none;
    transform: translateX(-105%);
    box-shadow: none;
  }

  .bt-sidebar-drawer-head {
    display: flex;
  }
}

@media (max-width: 920px) and (min-width: 769px) {
  .backtest-sidebar-toggle span {
    display: none;
  }
}

@media (max-width: 768px) {
  .backtest-workbench-header {
    min-height: 44px;
    height: auto;
    flex: 0 0 auto;
    flex-flow: row wrap;
    gap: 4px 8px;
    padding: 5px 6px;
  }

  .backtest-workbench-header__identity {
    flex-basis: 100%;
  }

  .backtest-sidebar-toggle span {
    display: none;
  }

  .backtest-workbench-title {
    flex: 1 1 auto;
  }

  .backtest-workbench-header__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .backtest-page__mobile-switch {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    min-height: 40px;
    flex: 0 0 40px;
    gap: 1px;
    min-width: 0;
    border-bottom: 1px solid var(--tv-border);
    background: var(--tv-bg-surface);
    padding: 3px 6px;
  }

  .backtest-page__mobile-switch-button {
    min-width: 0;
    min-height: 34px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--tv-text-muted);
    font-size: 0.77rem;
    font-weight: 800;
    line-height: 1.2;
  }

  .backtest-page__mobile-switch-button.is-active {
    background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
    color: var(--tv-text);
  }

  .backtest-page__mobile-switch-button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .backtest-page__split.tv-splitpanes {
    display: block !important;
    width: 100%;
    height: 100%;
    min-height: 0;
    overflow: hidden;
  }

  .backtest-page__split :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .backtest-page__split> :deep(.splitpanes__pane) {
    width: 100% !important;
    max-width: 100% !important;
    min-width: 0 !important;
    height: 100% !important;
    max-height: 100% !important;
    min-height: 0 !important;
    flex: none !important;
    transform: none !important;
  }

  .backtest-page--mobile-setup .backtest-page__split> :deep(.splitpanes__pane:last-of-type),
  .backtest-page--mobile-report .backtest-page__split> :deep(.splitpanes__pane:first-of-type) {
    display: none !important;
  }

  .bt-sidebar-drawer-head {
    display: none;
  }

  .bt-report-topbar,
  .bt-report-context-bar {
    gap: 7px;
    overflow-x: auto;
  }

  .bt-report-window {
    overflow: hidden;
  }

  .bt-report-window :deep(.v-window__container),
  .bt-report-window :deep(.v-window-item),
  .bt-report-window :deep(.v-window-item--active) {
    height: 100%;
  }

  .bt-report-tabs :deep(.v-tab) {
    min-width: 0;
    padding-inline: 10px;
  }

  .bt-order-table {
    min-width: 42rem;
  }

  .bt-version-compare-selectors {
    grid-template-columns: minmax(0, 1fr);
  }

  .bt-version-compare-metrics {
    grid-template-columns: minmax(6.5rem, 1fr) repeat(3, minmax(7rem, 1fr));
  }

  .bt-version-compare-definition {
    grid-template-columns: minmax(0, 1fr);
  }

  .backtest-page :deep(.v-chip) {
    max-width: 100%;
  }

  .backtest-page :deep(.v-chip__content) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

.backtest-page :deep(.v-card) {
  background: var(--tv-bg-surface);
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-chip) {
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-pagination .v-btn) {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  border: 1px solid var(--tv-border);
}

.backtest-page :deep(.v-pagination .v-btn.v-btn--active) {
  color: var(--tv-accent);
  border-color: var(--tv-accent);
}

.backtest-page :deep(.bt-accent-action.v-btn) {
  border: 1px solid color-mix(in srgb, var(--tv-accent) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: var(--tv-accent);
}

.backtest-page :deep(.bt-accent-action.v-btn:hover) {
  background: color-mix(in srgb, var(--tv-accent) 20%, transparent);
  border-color: color-mix(in srgb, var(--tv-accent) 54%, var(--tv-border));
}

.backtest-page .bt-history-run--selected {
  border-color: color-mix(in srgb, var(--tv-accent) 54%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 12%, var(--tv-bg-surface));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 18%, transparent);
}

.backtest-page .bt-history-run--idle:hover {
  border-color: color-mix(in srgb, var(--tv-accent) 42%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 6%, var(--tv-bg-surface));
}

.backtest-page .bt-bg-surface {
  background: var(--tv-bg-surface);
}

.backtest-page .bt-bg-muted {
  background: var(--tv-bg-surface-2);
}

.backtest-page .bt-border {
  border-color: var(--tv-border);
}

.backtest-page .bt-border-soft {
  border-color: color-mix(in srgb, var(--tv-border) 70%, transparent);
}

.backtest-page .bt-text {
  color: var(--tv-text);
}

.backtest-page .bt-text-strong {
  color: var(--tv-text);
}

.backtest-page .bt-text-muted {
  color: var(--tv-text-muted);
}

.backtest-page .bt-text-dim {
  color: var(--tv-text-dim);
}

.backtest-page .bt-divide> :not([hidden])~ :not([hidden]) {
  border-color: var(--tv-border);
}

.backtest-page .bt-divide-soft> :not([hidden])~ :not([hidden]) {
  border-color: color-mix(in srgb, var(--tv-border) 70%, transparent);
}

.backtest-page .bt-metric-negative {
  color: var(--tv-price-down);
}
</style>
