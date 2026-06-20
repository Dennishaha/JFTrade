<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";

import { KLINE_PERIODS } from "../charting/kline";
import BacktestChart from "../components/BacktestChart.vue";
import PageHeader from "../components/PageHeader.vue";
import { fetchEnvelope } from "../composables/apiClient";
import { formatGenericStatusLabel } from "../composables/consoleDataFormatting";
import { useMarketProfiles } from "../composables/marketProfiles";
import {
  useBacktestRuns,
  type BacktestFormState,
} from "../composables/useBacktestRuns";
import { useConsoleData } from "../composables/useConsoleData";
import { formatLocalDateTime } from "../utils/dateTime";
import { normalizeBacktestDateLabel } from "./backtestTimeWindow";
import dayjs from "dayjs";

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

// ── Console data (reuse existing symbol search infrastructure) ──
const { loadMarketInstrumentReferences, marketInstrumentSearchOptions } =
  useConsoleData();
const {
  marketOptions: backtestMarketOptions,
  defaultMarket,
  loadMarketProfiles,
  findMarketProfile,
  quoteCurrencyForMarket,
  supportsExtendedHoursForMarket,
  normalizeInstrumentRefWithMarketApi,
} = useMarketProfiles();

const controlPanelClass = "rounded-lg border bt-border bt-bg-surface";
const emptyStateClass =
  "rounded-lg border bt-border bt-bg-surface bt-text-muted";
const statCardClass = "rounded-2xl bt-bg-muted";
const cardBorderClass = "rounded-lg border bt-border";

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
  startDate: string;
  endDate: string;
  initialBalance: number;
  rehabType: string;
  useExtendedHours: boolean;
}

function readStoredBacktestFormPreferences(): StoredBacktestFormPreferences {
  const defaultStartDate = dayjs().subtract(3, "year").format("YYYY-MM-DD");
  const defaultEndDate = dayjs().format("YYYY-MM-DD");
  const defaults: StoredBacktestFormPreferences = {
    selectedDefinitionId: "",
    selectedMarket: "HK",
    codeInput: "00700",
    interval: "5m",
    startDate: defaultStartDate,
    endDate: defaultEndDate,
    initialBalance: 1000000,
    rehabType: "forward",
    useExtendedHours: false,
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
    const normalizeDate = (value: unknown, fallback: string) => {
      const normalized = normalizeBacktestDateLabel(typeof value === "string" ? value : "");
      return normalized === "" ? fallback : normalized;
    };

    return {
      selectedDefinitionId:
        typeof parsed.selectedDefinitionId === "string"
          ? parsed.selectedDefinitionId.trim()
          : defaults.selectedDefinitionId,
      selectedMarket:
        typeof parsed.selectedMarket === "string" &&
          parsed.selectedMarket.trim() !== ""
          ? parsed.selectedMarket.trim().toUpperCase()
          : defaults.selectedMarket,
      codeInput:
        typeof parsed.codeInput === "string" && parsed.codeInput.trim() !== ""
          ? parsed.codeInput.trim().toUpperCase()
          : defaults.codeInput,
      interval:
        typeof parsed.interval === "string" &&
          validIntervals.has(parsed.interval.trim())
          ? parsed.interval.trim()
          : defaults.interval,
      startDate: normalizeDate(parsed.startDate, defaults.startDate),
      endDate: normalizeDate(parsed.endDate, defaults.endDate),
      initialBalance:
        typeof parsed.initialBalance === "number" &&
          Number.isFinite(parsed.initialBalance) &&
          parsed.initialBalance > 0
          ? parsed.initialBalance
          : defaults.initialBalance,
      rehabType:
        typeof parsed.rehabType === "string" &&
          validRehabTypes.has(parsed.rehabType.trim().toLowerCase())
          ? parsed.rehabType.trim().toLowerCase()
          : defaults.rehabType,
      useExtendedHours: parsed.useExtendedHours === true,
    };
  } catch {
    return defaults;
  }
}

const storedBacktestFormPreferences = readStoredBacktestFormPreferences();

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

// Form state
const selectedDefinitionId = ref(
  storedBacktestFormPreferences.selectedDefinitionId,
);
const selectedMarket = ref(storedBacktestFormPreferences.selectedMarket);
const codeInput = ref(storedBacktestFormPreferences.codeInput);
const interval = ref(storedBacktestFormPreferences.interval);
const startDate = ref(storedBacktestFormPreferences.startDate);
const endDate = ref(storedBacktestFormPreferences.endDate);
const initialBalance = ref(storedBacktestFormPreferences.initialBalance);
const rehabType = ref(storedBacktestFormPreferences.rehabType); // "forward" | "backward" | "none"
const useExtendedHours = ref(storedBacktestFormPreferences.useExtendedHours);

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

const codeSuggestions = computed(() => {
  const market = selectedMarket.value.trim().toUpperCase();
  return marketInstrumentSearchOptions.value
    .filter((option) => option.market === market)
    .map((option) => ({
      value: option.symbol,
      title:
        option.name == null
          ? option.instrumentId
          : `${option.symbol} · ${option.name}`,
    }));
});

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
  code: codeInput.value.trim().toUpperCase(),
  instrumentId:
    codeInput.value.includes(".") || codeInput.value.includes(":")
      ? codeInput.value.trim().toUpperCase()
      : "",
  interval: interval.value,
  startDate: startDate.value,
  endDate: endDate.value,
  initialBalance: initialBalance.value,
  rehabType: rehabType.value,
  useExtendedHours: useExtendedHours.value,
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

const expandedPanels = ref<string[]>([]);

watch(expandedPanels, (nextPanels, previousPanels) => {
  const previous = new Set(previousPanels ?? []);
  for (const runID of nextPanels) {
    if (!previous.has(runID)) {
      void toggleRun(runID);
    }
  }
});

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
    startDate,
    endDate,
    initialBalance,
    rehabType,
    useExtendedHours,
  ],
  ([
    nextDefinitionId,
    nextMarket,
    nextCodeInput,
    nextInterval,
    nextStartDate,
    nextEndDate,
    nextInitialBalance,
    nextRehabType,
    nextUseExtendedHours,
  ]) => {
    if (typeof window === "undefined" || window.localStorage == null) {
      return;
    }
    const storedPreferences: StoredBacktestFormPreferences = {
      selectedDefinitionId: nextDefinitionId.trim(),
      selectedMarket: nextMarket.trim().toUpperCase(),
      codeInput: nextCodeInput.trim().toUpperCase(),
      interval: nextInterval.trim(),
      startDate: nextStartDate,
      endDate: nextEndDate,
      initialBalance: nextInitialBalance,
      rehabType: nextRehabType,
      useExtendedHours: nextUseExtendedHours,
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

const headerStats = computed(() => [
  {
    label: "策略数",
    value: definitions.value.length,
  },
  {
    label: "回测记录",
    value: runs.value.length,
  },
  {
    label: "运行中",
    value: runs.value.filter((r) => r.status === "running").length,
    tone: "info" as const,
  },
  {
    label: "已完成",
    value: runs.value.filter((r) => r.status === "completed").length,
    tone: "good" as const,
  },
]);

// ── Loaders ──
function ensureSelectedMarketProfile() {
  if (findMarketProfile(selectedMarket.value) != null) {
    return;
  }
  selectedMarket.value = defaultMarket.value.trim().toUpperCase() || "HK";
}

onMounted(async () => {
  await Promise.all([
    loadMarketProfiles(),
    loadDefinitions(),
    loadRuns(),
    loadMarketInstrumentReferences(),
  ]);
  ensureSelectedMarketProfile();
});

async function loadDefinitions() {
  try {
    const items = await fetchEnvelope<StrategyDefinition[]>(
      "/api/v1/strategy-definitions",
    );
    definitions.value = items;
    if (items.length > 0 && !selectedDefinitionId.value) {
      selectedDefinitionId.value = items[0]!.id;
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
    const detail = await fetchEnvelope<StrategyDefinition>(
      `/api/v1/strategy-definitions/${encodeURIComponent(definitionId)}?${params.toString()}`,
    );
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

function pnlPrefix(val: number) {
  return val >= 0 ? "+" : "";
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
  <div class="backtest-page grid gap-4">
    <PageHeader eyebrow="模拟回测" title="回测" description="选择策略定义、标的和时段，同步历史K线后运行回测。" :stats="headerStats" />

    <!-- Error banner -->
    <div v-if="error" class="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
      {{ error }}
      <button class="ml-3 underline" type="button" @click="error = ''">
        关闭
      </button>
    </div>

    <!-- Main layout -->
    <div class="grid grid-cols-12 gap-3">
      <!-- Left: control panel -->
      <div class="col-span-4 lg:col-span-3">
        <div :class="[controlPanelClass, 'sticky top-4 p-3']">
          <div class="space-y-2.5">
            <!-- Strategy -->
            <div class="grid gap-0.5">
              <label class="text-xs font-semibold bt-text-strong">策略定义</label>
              <v-select v-model="selectedDefinitionId" :items="definitions" item-title="name" item-value="id"
                density="compact" variant="outlined" placeholder="选择策略" />
            </div>

            <!-- Instrument -->
            <div class="grid gap-2">
              <div class="grid grid-cols-2 gap-2">
                <div class="grid gap-0.5">
                  <label class="text-xs font-semibold bt-text-strong">市场</label>
                  <v-select v-model="selectedMarket" :items="backtestMarketOptions" item-title="title"
                    item-value="value" density="compact" variant="outlined" />
                </div>
                <div class="grid gap-0.5">
                  <label class="text-xs font-semibold bt-text-strong">代码</label>
                  <v-combobox v-model="codeInput" :items="codeSuggestions" item-title="title" item-value="value"
                    density="compact" variant="outlined" placeholder="00700" clearable />
                </div>
              </div>
              <div class="text-xs bt-text-muted">
                {{ displayInstrumentId || "请先输入市场与代码" }}
              </div>
            </div>

            <!-- Period -->
            <div class="grid gap-0.5">
              <label class="text-xs font-semibold bt-text-strong">K线周期</label>
              <v-select v-model="interval" :items="KLINE_PERIODS" item-title="label" item-value="value"
                density="compact" variant="outlined" />
              <div class="text-xs bt-text-dim">
                默认 5m，可按本次回测需要单独调整。
              </div>
            </div>

            <!-- Rehab type -->
            <div class="grid gap-0.5">
              <label class="text-xs font-semibold bt-text-strong">复权方式</label>
              <v-select v-model="rehabType" :items="[
                { value: 'forward', title: '前复权' },
                { value: 'backward', title: '后复权' },
                { value: 'none', title: '不复权' },
              ]" item-title="title" item-value="value" density="compact" variant="outlined" />
              <div class="text-xs bt-text-dim">
                前复权适合回测，后复权适合分析。
              </div>
            </div>

            <div v-if="extendedHoursSupported" class="grid gap-1">
              <div class="flex items-start gap-3 rounded-lg border bt-border px-3 py-2">
                <v-switch v-model="useExtendedHours" color="teal" density="compact" hide-details class="self-center" />
                <div class="min-w-0 flex-1">
                  <div class="text-xs font-semibold bt-text-strong">
                    扩展交易时段
                  </div>
                  <div class="text-xs bt-text-dim">
                    {{ extendedHoursHint }}
                  </div>
                </div>
              </div>
            </div>

            <!-- Date range -->
            <div class="grid grid-cols-2 gap-2">
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold bt-text-strong">起始日期</label>
                <v-text-field v-model="startDate" type="date" density="compact" variant="outlined" />
              </div>
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold bt-text-strong">结束日期</label>
                <v-text-field v-model="endDate" type="date" density="compact" variant="outlined" />
              </div>
            </div>

            <!-- Initial balance & derived warmup -->
            <div class="grid grid-cols-2 gap-2">
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold bt-text-strong">初始资金</label>
                <v-text-field v-model.number="initialBalance" type="number" :min="1000" density="compact"
                  variant="outlined" />
                <div class="text-xs bt-text-dim">{{ quoteCurrency }}</div>
              </div>
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold bt-text-strong">预热K线</label>
                <div class="min-h-[40px] rounded-md border bt-border bt-bg-muted px-3 py-2 text-sm bt-text">
                  {{ warmupPreviewValue }}
                </div>
                <div class="text-xs bt-text-dim">{{ warmupPreviewNote }}</div>
              </div>
            </div>

            <!-- Sync section -->
            <div v-if="syncing && !syncProgress"
              class="rounded-xl border border-teal-200 bg-teal-50/50 px-3 py-3 text-center">
              <span class="text-sm text-teal-700">正在启动同步…</span>
            </div>
            <div v-else-if="syncing && syncProgress"
              class="rounded-xl border border-teal-200 bg-teal-50/50 px-3 py-3 space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs font-semibold text-teal-800">
                  同步中 · {{ syncProgress.currentInterval || "准备" }}
                </span>
                <button
                  class="rounded-full border border-red-200 px-2 py-0.5 text-xs text-red-600 hover:bg-red-50 transition"
                  type="button" @click="cancelSync">
                  取消
                </button>
              </div>
              <div class="h-2 rounded-full bg-teal-200 overflow-hidden">
                <div class="h-full rounded-full bg-teal-500 transition-all duration-500" :style="{
                  width:
                    syncProgress.totalIntervals > 0
                      ? (syncProgress.completedIntervals /
                        syncProgress.totalIntervals) *
                      100 +
                      '%'
                      : '10%',
                }" />
              </div>
              <div class="flex items-center justify-between text-xs text-teal-700">
                <span>{{ syncProgress.completedBatches }} 批</span>
                <span v-if="syncProgress.retries > 0" class="text-amber-600">重试 {{ syncProgress.retries }}</span>
              </div>
            </div>
            <div v-else-if="syncProgress?.status === 'cancelled'"
              class="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
              同步已取消 · {{ syncProgress.completedBatches }} 批已完成
            </div>
            <!-- Sync button -->
            <button v-else
              class="w-full rounded-xl border border-teal-300 bg-teal-50 px-3 py-1.5 text-xs font-semibold text-teal-700 shadow-sm transition hover:bg-teal-100 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="running" type="button" @click="syncKlines">
              ⬇ 同步历史K线
            </button>

            <!-- Run button -->
            <button
              class="w-full rounded-xl bg-teal-600 px-3 py-1.5 text-xs font-semibold text-white shadow-sm transition hover:bg-teal-700 disabled:cursor-not-allowed flex items-center justify-center gap-2"
              :class="{ 'bt-disabled-bg': running || !selectedDefinitionId }"
              :disabled="running || !selectedDefinitionId" type="button" @click="startBacktest">
              <v-progress-circular v-if="running" indeterminate :size="16" :width="2" color="white" />
              {{ running ? "启动中..." : "▶ 开始回测" }}
            </button>

            <div class="rounded-lg border border-teal-100 bg-teal-50 px-2 py-1.5 text-xs text-teal-800">
              ⚡ 先同步K线，再开始回测。
            </div>
          </div>
        </div>
      </div>

      <!-- Right: results list -->
      <div class="col-span-8 lg:col-span-9">
        <div class="grid gap-4">
          <!-- Results cards -->
          <div v-if="sortedRuns.length === 0" :class="[emptyStateClass, 'p-8 text-center text-sm']">
            {{ emptyResultsMessage }}
          </div>

          <div v-else class="grid gap-3 rounded-lg border bt-border bt-bg-surface px-3 py-3">
            <div class="grid gap-3 lg:grid-cols-[minmax(0,1.7fr)_180px_220px_auto]">
              <v-text-field v-model="resultsSearchQuery" density="compact" variant="outlined" hide-details clearable
                placeholder="搜索策略、标的、回测 ID" />
              <v-select v-model="resultsStatusFilter" :items="BACKTEST_RESULT_STATUS_OPTIONS" item-title="title"
                item-value="value" density="compact" variant="outlined" hide-details />
              <v-select v-model="resultsStrategyFilter" :items="resultStrategyOptions" item-title="title"
                item-value="value" density="compact" variant="outlined" hide-details />
              <div class="flex items-center justify-end">
                <v-btn variant="text" density="comfortable" :disabled="!hasResultsFilters" @click="resetResultsFilters">
                  清空筛选
                </v-btn>
              </div>
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3 text-xs bt-text-muted">
              <span>{{ resultsPageSummary }}</span>
              <span>回测结果由服务端提供。</span>
            </div>
          </div>

          <div v-if="sortedRuns.length > 0 && filteredRuns.length === 0"
            :class="[emptyStateClass, 'p-8 text-center text-sm']">
            {{ emptyResultsMessage }}
          </div>

          <v-expansion-panels v-model="expandedPanels" multiple variant="default" class="grid gap-3">
            <v-expansion-panel v-for="run in pagedRuns" :key="run.id" :value="run.id" :class="cardBorderClass">
              <v-expansion-panel-title class="pa-4">
                <template #default>
                  <div class="flex items-center gap-3 w-full min-w-0">
                    <v-chip :color="statusChip(run.status).color" size="small" variant="outlined" class="shrink-0">
                      {{ statusChip(run.status).label }}
                    </v-chip>
                    <div class="min-w-0 flex-1">
                      <div class="text-base bt-text-strong truncate">
                        {{ resolveStrategyName(run.request.definitionId) }} ·
                        {{ run.request.symbol }} ·
                        {{ formatBacktestRunDate(run.request.startDate) }} →
                        {{ formatBacktestRunDate(run.request.endDate) }} ·
                        {{ resolveRunSessionMode(run) }}
                      </div>
                      <div class="text-xs bt-text-muted mt-0.5">
                        {{ run.id }} · {{ run.request.interval }} ·
                        {{ formatBacktestRehabType(run.request.rehabType) }} ·
                        {{ run.request.initialBalance.toLocaleString() }}
                        {{ resolveRunQuoteCurrency(run)
                        }}<template v-if="run.request.definitionVersion">
                          ·
                          {{
                            formatStrategyVersion(run.request.definitionVersion)
                          }}</template>
                      </div>
                      <!-- Running / Queued progress -->
                      <div v-if="run.status === 'running' || run.status === 'queued'"
                        class="mt-2 flex items-center gap-3">
                        <v-progress-linear v-if="run.status === 'running'" color="teal" indeterminate rounded
                          :height="6" class="flex-1" />
                        <v-progress-linear v-else color="warning" indeterminate rounded :height="6" class="flex-1" />
                        <span class="text-xs whitespace-nowrap shrink-0" :class="run.status === 'running'
                          ? 'text-teal-600'
                          : 'text-amber-600'
                          ">
                          {{ run.status === "running" ? "回测运行中…" : "排队等待中…" }}
                        </span>
                      </div>
                    </div>
                    <v-btn v-if="isTerminalBacktestStatus(run.status)" icon="fa-solid fa-trash"
                      size="small" variant="text" color="error" title="删除回测结果" @click.stop="deleteRun(run.id)" />
                  </div>
                </template>
              </v-expansion-panel-title>

              <v-expansion-panel-text>
                <div v-if="resolveBacktestStrategyVersionNotice(run)"
                  class="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                  {{ resolveBacktestStrategyVersionNotice(run) }}
                </div>

                <div v-if="detailLoading[run.id]"
                  class="mb-3 rounded-lg border bt-border bt-bg-muted px-3 py-2 text-xs bt-text-muted">
                  正在加载完整回测详情…
                </div>
                <div v-if="detailErrors[run.id]"
                  class="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
                  {{ detailErrors[run.id] }}
                </div>

                <div v-if="
                  run.result &&
                  isTerminalBacktestStatus(run.status)
                ">
                  <div class="grid grid-cols-2 gap-3 lg:grid-cols-6">
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        最终资金
                      </div>
                      <div class="mt-1 text-lg font-semibold bt-text">
                        {{
                          run.result.finalBalance.toLocaleString(undefined, {
                            minimumFractionDigits: 2,
                          })
                        }}
                      </div>
                      <div class="text-xs bt-text-muted">
                        {{ resolveRunQuoteCurrency(run) }}
                      </div>
                    </div>
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        收益
                      </div>
                      <div class="mt-1 text-lg font-semibold" :class="pnlColor(run.result.pnl)">
                        {{ pnlPrefix(run.result.pnl)
                        }}{{
                          run.result.pnl.toLocaleString(undefined, {
                            minimumFractionDigits: 2,
                          })
                        }}
                      </div>
                      <div class="text-xs bt-text-muted">
                        {{ resolveRunQuoteCurrency(run) }}
                      </div>
                    </div>
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        交易次数
                      </div>
                      <div class="mt-1 text-lg font-semibold bt-text">
                        {{ run.result.totalTrades }}
                      </div>
                    </div>
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        胜率
                      </div>
                      <div class="mt-1 text-lg font-semibold bt-text">
                        {{ (run.result.winRate * 100).toFixed(1) }}%
                      </div>
                    </div>
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        最大回撤
                      </div>
                      <div class="mt-1 text-lg font-semibold" :class="drawdownColor(run.result.maxDrawdown)">
                        {{ formatPercentMetric(run.result.maxDrawdown) }}
                      </div>
                    </div>
                    <div :class="[statCardClass, 'px-3 py-3']">
                      <div class="text-xs uppercase tracking-[0.15em] bt-text-muted">
                        当前回撤
                      </div>
                      <div class="mt-1 text-lg font-semibold" :class="drawdownColor(run.result.currentDrawdown)">
                        {{ formatPercentMetric(run.result.currentDrawdown) }}
                      </div>
                    </div>
                  </div>
                  <div v-if="
                    run.result &&
                    run.result.totalTrades === 0 &&
                    !run.result.error
                  " class="mt-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                    未产生任何交易。可能原因：策略未调用
                    placeOrder()，或订阅的K线周期未同步。
                  </div>
                  <div class="mt-2 rounded border bt-border bt-bg-muted px-2 py-1 text-xs bt-text">
                    <div class="mt-1">{{ resolveBacktestPriceBasisNote(run) }}</div>
                    <div v-if="resolveQueriedCandleBounds(run.result?.candles)" class="mt-1">
                      查询到的周期边界：左边界
                      {{ resolveQueriedCandleBounds(run.result?.candles)?.left }} ｜
                      右边界
                      {{
                        resolveQueriedCandleBounds(run.result?.candles)?.right
                      }}
                      ｜ 共
                      {{
                        resolveQueriedCandleBounds(run.result?.candles)?.count
                      }}
                      根
                    </div>
                  </div>

                  <!-- Backtest chart -->
                  <div v-if="
                    run.status === 'completed' && run.result?.pnlCurve?.length
                  " class="mt-2">
                    <BacktestChart :candles="run.result.candles ?? []" :trades="run.result.trades ?? []"
                      :pnl-curve="run.result.pnlCurve" :drawdown-curve="run.result.drawdownCurve ?? []"
                      :initial-balance="run.request.initialBalance" :min-height="560"
                      :currency-unit="resolveRunQuoteCurrency(run)" empty-text="暂无权益曲线数据" />
                  </div>

                  <div v-if="run.result?.orderBook?.length" :class="[cardBorderClass, 'mt-3 overflow-hidden']">
                    <details>
                      <summary
                        class="flex cursor-pointer items-center justify-between px-4 py-3 text-sm font-semibold bt-text marker:content-none">
                        <span>订单簿</span>
                        <span class="text-xs font-medium bt-text-muted">
                          {{ run.result.orderBook.length }} 笔 · 默认收起
                        </span>
                      </summary>
                      <div class="border-t bt-border">
                        <div class="max-h-96 overflow-auto">
                          <table class="min-w-full divide-y bt-divide text-sm">
                            <thead
                              class="sticky top-0 bt-bg-muted text-left text-xs uppercase tracking-[0.14em] bt-text-muted">
                              <tr>
                                <th class="px-4 py-3 font-medium">下单</th>
                                <th class="px-4 py-3 font-medium">成交</th>
                                <th class="px-4 py-3 font-medium">方向</th>
                                <th class="px-4 py-3 font-medium">数量</th>
                                <th class="px-4 py-3 font-medium">委托价</th>
                                <th class="px-4 py-3 font-medium">成交价</th>
                                <th class="px-4 py-3 font-medium">状态</th>
                              </tr>
                            </thead>
                            <tbody class="divide-y bt-divide-soft bt-bg-surface">
                              <tr v-for="entry in run.result.orderBook"
                                :key="`${entry.orderId}-${entry.filledAt ?? entry.submittedAt ?? ''}`">
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  <div>
                                    {{ formatBacktestTimestamp(entry.submittedAt) }}
                                  </div>
                                  <div class="mt-1 text-xs bt-text-dim">
                                    #{{ entry.orderId
                                    }}<span v-if="entry.clientOrderId">
                                      · {{ entry.clientOrderId }}</span>
                                  </div>
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  {{ formatBacktestTimestamp(entry.filledAt) }}
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  {{ formatBacktestOrderSide(entry.side) }}
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  <div>
                                    {{
                                      formatBacktestQuantity(
                                        entry.quantity,
                                        entry.quantityText,
                                      )
                                    }}
                                  </div>
                                  <div v-if="entry.filledQuantity !== undefined" class="mt-1 text-xs bt-text-dim">
                                    成交
                                    {{
                                      formatBacktestQuantity(
                                        entry.filledQuantity,
                                        entry.filledQuantityText,
                                      )
                                    }}
                                  </div>
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  {{
                                    formatBacktestOrderPrice(
                                      entry.orderPrice,
                                      entry.orderType,
                                      entry.orderPriceText,
                                    )
                                  }}
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  {{
                                    formatBacktestOrderPrice(
                                      entry.filledPrice,
                                      undefined,
                                      entry.filledPriceText,
                                    )
                                  }}
                                </td>
                                <td class="px-4 py-3 align-top bt-text-strong">
                                  {{ formatBacktestOrderStatus(entry.status) }}
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                      </div>
                    </details>
                  </div>
                </div>

                <!-- Runtime errors (e.g. insufficient balance, order rejections) -->
                <div v-if="
                  run.result?.runtimeErrors && run.result.runtimeErrors.length > 0
                " class="mt-3">
                  <details class="rounded-lg border border-red-200 bg-red-50 px-3 py-2">
                    <summary class="cursor-pointer text-xs font-semibold text-red-700 select-none">
                      ⚡ {{ runtimeErrorSummary(run.result) }}
                    </summary>
                    <div class="mt-2 space-y-1 max-h-48 overflow-y-auto">
                      <div v-for="(err, i) in run.result.runtimeErrors" :key="i"
                        class="rounded border border-red-100 bt-bg-surface px-2 py-1 text-xs text-red-800 font-mono leading-relaxed">
                        <span v-if="runtimeErrorRepeatCount(run.result, err) > 1" class="font-semibold">
                          x{{ runtimeErrorRepeatCount(run.result, err) }}
                        </span>
                        {{ err }}
                      </div>
                    </div>
                  </details>
                </div>

                <!-- Diagnostic logs (always visible when present) -->
                <div v-if="run.result?.logs && run.result.logs.length > 0" class="mt-3">
                  <div
                    class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 space-y-1">
                    <div v-for="(log, i) in run.result.logs" :key="i">
                      ⚠ {{ log }}
                    </div>
                  </div>
                </div>
                <div v-if="run.result?.error" class="mt-3">
                  <div
                    class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 whitespace-pre-wrap">
                    {{ run.result.error }}
                  </div>
                </div>
              </v-expansion-panel-text>
            </v-expansion-panel>
          </v-expansion-panels>

          <div v-if="resultsPageCount > 1" class="flex justify-center pt-2">
            <v-pagination v-model="resultsPage" :length="resultsPageCount" :total-visible="6" density="comfortable" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backtest-page :deep(.v-field) {
  border-radius: 10px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 88%, transparent);
}

.backtest-page :deep(.v-field--variant-outlined .v-field__outline) {
  --v-field-border-opacity: 1;
  color: var(--tv-border);
}

.backtest-page :deep(.v-card) {
  background: var(--tv-bg-surface);
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-select .v-field__input),
.backtest-page :deep(.v-combobox .v-field__input),
.backtest-page :deep(.v-text-field .v-field__input) {
  color: var(--tv-text);
}

.backtest-page :deep(.v-input__details) {
  color: var(--tv-text-dim);
}

.backtest-page :deep(.v-expansion-panel) {
  background: var(--tv-bg-surface);
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-expansion-panel--active > .v-expansion-panel-title) {
  border-bottom: 1px solid var(--tv-border);
}

.backtest-page :deep(.v-expansion-panel-title) {
  color: var(--tv-text);
}

.backtest-page :deep(.v-expansion-panel-text) {
  color: var(--tv-text);
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

.backtest-page .bt-disabled-bg {
  background: color-mix(in srgb,
      var(--tv-bg-elevated) 60%,
      var(--tv-border) 40%);
}

.backtest-page .bt-divide> :not([hidden])~ :not([hidden]) {
  border-color: var(--tv-border);
}

.backtest-page .bt-divide-soft> :not([hidden])~ :not([hidden]) {
  border-color: color-mix(in srgb, var(--tv-border) 70%, transparent);
}

.backtest-page .bt-metric-negative {
  color: var(--tv-down);
}
</style>
