<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";

import { KLINE_PERIODS } from "../charting/kline";
import BacktestChart from "../components/BacktestChart.vue";
import PageHeader from "../components/PageHeader.vue";
import { fetchEnvelope } from "../composables/apiClient";
import { formatGenericStatusLabel } from "../composables/consoleDataFormatting";
import { useBacktestRuns, type BacktestFormState } from "../composables/useBacktestRuns";
import { useConsoleData } from "../composables/useConsoleData";
import { useTheme } from "../composables/useTheme";
import { buildBacktestDayInclusiveEndTime, buildBacktestDayStartTime } from "./backtestTimeWindow";
import dayjs from "dayjs";

// ── Console data (reuse existing symbol search infrastructure) ──
const {
  loadMarketInstrumentReferences,
  marketInstrumentSearchOptions,
} = useConsoleData();

// ── Theme ──
const { theme } = useTheme();

const controlPanelClass = computed(() =>
  theme.value === "light"
    ? "rounded-lg border border-slate-200 bg-white"
    : "rounded-lg border border-slate-700 bg-slate-900",
);

const emptyStateClass = computed(() =>
  theme.value === "light"
    ? "rounded-lg border border-slate-200 bg-white text-slate-500"
    : "rounded-lg border border-slate-700 bg-slate-900 text-slate-400",
);

const statCardClass = computed(() =>
  theme.value === "light"
    ? "rounded-2xl bg-slate-50"
    : "rounded-2xl bg-slate-800",
);

const cardBorderClass = computed(() =>
  theme.value === "light"
    ? "rounded-lg border border-slate-200"
    : "rounded-lg border border-slate-700",
);

// ── Backtest run DTOs ──
interface StrategyDefinition {
  id: string;
  name: string;
  symbol: string;
  interval: string;
  derivedWarmupBars?: number;
  derivedWarmupInterval?: string;
}

// ── Reactive state ──
const definitions = ref<StrategyDefinition[]>([]);
const warmupPreviewBars = ref<number | null>(null);
const warmupPreviewPending = ref(false);
const warmupPreviewInterval = ref("");
let warmupPreviewRequestId = 0;

// Form state
const selectedDefinitionId = ref("");
const symbolInput = ref("HK:00700");
const interval = ref("1m");
const startDate = ref(dayjs().subtract(3, "year").format("YYYY-MM-DD"));
const endDate = ref(dayjs().format("YYYY-MM-DD"));
const initialBalance = ref(1000000);
const rehabType = ref("forward"); // "forward" | "backward" | "none"

// Track whether the user has manually overridden the strategy defaults
const symbolManuallySet = ref(false);
const intervalManuallySet = ref(false);

// Strategy's configured symbol in "MARKET:CODE" display format
const strategyDefaultSymbol = computed(() => {
  const d = selectedDefinition.value;
  if (!d?.symbol) return "";
  return d.symbol.includes(".")
    ? d.symbol.replace(".", ":")
    : `HK:${d.symbol}`;
});

// Whether the current symbol differs from the strategy's default
const symbolDiffersFromStrategy = computed(() => {
  if (!strategyDefaultSymbol.value) return false;
  return symbolInput.value !== strategyDefaultSymbol.value;
});

// Whether the current interval differs from the strategy's default
const intervalDiffersFromStrategy = computed(() => {
  const d = selectedDefinition.value;
  if (!d?.interval) return false;
  return interval.value !== d.interval;
});

// Reset symbol/interval to strategy defaults
function resetToStrategyDefaults() {
  const d = selectedDefinition.value;
  if (!d) return;
  if (d.symbol) {
    symbolInput.value = strategyDefaultSymbol.value;
    symbolManuallySet.value = false;
  }
  if (d.interval) {
    interval.value = d.interval;
    intervalManuallySet.value = false;
  }
}

// Sync form (start/end time)
const syncStartTime = computed(() => buildBacktestDayStartTime(startDate.value));
const syncEndTime = computed(() => buildBacktestDayInclusiveEndTime(endDate.value));
const backtestStartTime = computed(() => buildBacktestDayStartTime(startDate.value));
const backtestEndTime = computed(() => buildBacktestDayInclusiveEndTime(endDate.value));

// ── Derived ──
const selectedDefinition = computed(() =>
  definitions.value.find((d) => d.id === selectedDefinitionId.value),
);

const parsedSymbol = computed(() => {
  const raw = symbolInput.value ?? "";
  const parts = raw.includes(":") ? raw.split(":") : ["HK", "00700"];
  return {
    market: parts[0] || "HK",
    symbol: parts[1] || "00700",
    instrumentId: `${parts[0] || "HK"}.${parts[1] || "00700"}`,
  };
});

const periodLabel = computed(() =>
  KLINE_PERIODS.find((p) => p.value === interval.value)?.label ?? interval.value,
);

const quoteCurrency = computed(() => {
  const market = parsedSymbol.value.market.toUpperCase();
  if (market === "US") return "USD";
  return "HKD";
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
  const previewInterval = warmupPreviewInterval.value || interval.value || selectedDefinition.value?.interval || "1m";
  return `按当前回测周期 ${previewInterval} 推导策略依赖的最大历史 bars。`;
});

function inferQuoteCurrencyFromInstrumentId(instrumentId: string | undefined) {
  const normalized = (instrumentId ?? "").trim().toUpperCase();
  if (normalized.startsWith("US.")) {
    return "USD";
  }
  if (normalized.startsWith("HK.")) {
    return "HKD";
  }
  if (normalized.startsWith("CN.") || normalized.startsWith("SH.") || normalized.startsWith("SZ.")) {
    return "CNY";
  }
  return "HKD";
}

function resolveRunQuoteCurrency(run: { request: { symbol: string }; result?: { quoteCurrency?: string | undefined } | undefined }) {
  const resultCurrency = run.result?.quoteCurrency?.trim();
  if (resultCurrency) {
    return resultCurrency;
  }
  return inferQuoteCurrencyFromInstrumentId(run.request.symbol);
}

function resolveStrategyName(definitionId: string | undefined) {
  if (!definitionId) {
    return "未命名策略";
  }
  return definitions.value.find((definition) => definition.id === definitionId)?.name ?? definitionId;
}

const backtestFormState = computed<BacktestFormState>(() => ({
  definitionId: selectedDefinitionId.value,
  instrumentId: parsedSymbol.value.instrumentId,
  interval: interval.value,
  syncStartTime: syncStartTime.value,
  syncEndTime: syncEndTime.value,
  backtestStartTime: backtestStartTime.value,
  backtestEndTime: backtestEndTime.value,
  initialBalance: initialBalance.value,
  rehabType: rehabType.value,
}));

const {
  runs,
  running,
  syncing,
  syncProgress,
  error,
  expandedRuns,
  filteredRuns,
  toggleRun,
  loadRuns,
  syncKlines,
  cancelSync,
  startBacktest,
} = useBacktestRuns({
  formState: backtestFormState,
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
onMounted(async () => {
  await Promise.all([
    loadDefinitions(),
    loadRuns(),
    loadMarketInstrumentReferences(),
  ]);
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
  const requestedInterval = (interval.value || selectedDefinition.value?.interval || "1m").trim();
  const requestId = ++warmupPreviewRequestId;

  if (!definitionId) {
    warmupPreviewBars.value = null;
    warmupPreviewInterval.value = requestedInterval;
    warmupPreviewPending.value = false;
    return;
  }

  warmupPreviewPending.value = true;
  try {
    const detail = await fetchEnvelope<StrategyDefinition>(
      `/api/v1/strategy-definitions/${encodeURIComponent(definitionId)}?interval=${encodeURIComponent(requestedInterval)}`,
    );
    if (requestId !== warmupPreviewRequestId) {
      return;
    }
    warmupPreviewBars.value = Number.isFinite(detail.derivedWarmupBars)
      ? detail.derivedWarmupBars ?? null
      : null;
    warmupPreviewInterval.value = detail.derivedWarmupInterval?.trim() || requestedInterval;
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

function pnlPrefix(val: number) {
  return val >= 0 ? "+" : "";
}

function drawdownColor(value: number | undefined) {
  if ((value ?? 0) > 0) {
    return "text-red-600 dark:text-red-400";
  }
  return "text-slate-900 dark:text-slate-100";
}

function formatPercentMetric(value: number | undefined) {
  const normalized = Number.isFinite(value) ? value ?? 0 : 0;
  return `${(normalized * 100).toFixed(2)}%`;
}

function formatBacktestTimestamp(value?: string) {
  if (!value) {
    return "--";
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  return parsed.toLocaleString("zh-CN", {
    hour12: false,
  });
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

function formatBacktestOrderPrice(value: number | undefined, orderType?: string, raw?: string) {
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

function resolveQueriedCandleBounds(candles: Array<{ time: string }> | undefined) {
  if (!candles || candles.length === 0) {
    return null;
  }

  const sorted = [...candles]
    .filter((candle) => {
      const at = new Date(candle.time).getTime();
      return Number.isFinite(at);
    })
    .sort((left, right) => new Date(left.time).getTime() - new Date(right.time).getTime());

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
watch(selectedDefinitionId, () => {
  const d = selectedDefinition.value;
  if (d) {
    if (d.symbol && !symbolManuallySet.value) {
      symbolInput.value = strategyDefaultSymbol.value;
    }
    if (d.interval && !intervalManuallySet.value) {
      interval.value = d.interval;
    }
  }
});

// Track manual user edits on symbol and interval
watch(symbolInput, (val, old) => {
  if (old !== undefined && val !== strategyDefaultSymbol.value) {
    symbolManuallySet.value = true;
  }
});
watch(interval, (val, old) => {
  const d = selectedDefinition.value;
  if (old !== undefined && d && val !== d.interval) {
    intervalManuallySet.value = true;
  }
});

watch([selectedDefinitionId, interval], () => {
  void loadWarmupPreview();
}, { immediate: true });
</script>

<template>
  <div class="grid gap-4">
    <PageHeader eyebrow="模拟回测" title="回测" description="选择策略定义、标的和时段，同步历史K线后运行回测。" :stats="headerStats" />

    <!-- Error banner -->
    <div v-if="error"
      class="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-200">
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
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">策略定义</label>
              <v-select v-model="selectedDefinitionId" :items="definitions" item-title="name" item-value="id"
                density="compact" variant="outlined" placeholder="选择策略" />
            </div>

            <!-- Symbol -->
            <div class="grid gap-0.5">
              <div class="flex items-center justify-between">
                <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">标的</label>
                <button v-if="symbolDiffersFromStrategy || intervalDiffersFromStrategy"
                  class="text-xs text-teal-600 underline hover:text-teal-800 transition dark:text-teal-400 dark:hover:text-teal-200"
                  type="button" @click="resetToStrategyDefaults">
                  使用策略默认
                </button>
              </div>
              <v-combobox v-model="symbolInput"
                :items="marketInstrumentSearchOptions.map(o => ({ value: o.lookupValue, title: o.label }))"
                item-title="title" item-value="value" density="compact" variant="outlined" placeholder="HK:00700"
                clearable />
              <div v-if="symbolDiffersFromStrategy" class="text-xs text-amber-600 dark:text-amber-400">
                策略默认标的：{{ strategyDefaultSymbol }}（{{ parsedSymbol.instrumentId }}）
              </div>
              <div v-else class="text-xs text-slate-500 dark:text-slate-400">
                {{ parsedSymbol.instrumentId }}
              </div>
            </div>

            <!-- Period -->
            <div class="grid gap-0.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">K线周期</label>
              <v-select v-model="interval" :items="KLINE_PERIODS" item-title="label" item-value="value"
                density="compact" variant="outlined" />
              <div v-if="intervalDiffersFromStrategy" class="text-xs text-amber-600 dark:text-amber-400">
                策略默认周期：{{ selectedDefinition?.interval }}
              </div>
            </div>

            <!-- Rehab type -->
            <div class="grid gap-0.5">
              <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">复权方式</label>
              <v-select v-model="rehabType" :items="[
                { value: 'forward', title: '前复权' },
                { value: 'backward', title: '后复权' },
                { value: 'none', title: '不复权' },
              ]" item-title="title" item-value="value" density="compact" variant="outlined" />
              <div class="text-xs text-slate-400 dark:text-slate-500">前复权适合回测，后复权适合分析。</div>
            </div>

            <!-- Date range -->
            <div class="grid grid-cols-2 gap-2">
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">起始日期</label>
                <v-text-field v-model="startDate" type="date" density="compact" variant="outlined" />
              </div>
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">结束日期</label>
                <v-text-field v-model="endDate" type="date" density="compact" variant="outlined" />
              </div>
            </div>

            <!-- Initial balance & derived warmup -->
            <div class="grid grid-cols-2 gap-2">
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">初始资金</label>
                <v-text-field v-model.number="initialBalance" type="number" :min="1000" density="compact"
                  variant="outlined" />
                <div class="text-xs text-slate-400 dark:text-slate-500">{{ quoteCurrency }}</div>
              </div>
              <div class="grid gap-0.5">
                <label class="text-xs font-semibold text-slate-700 dark:text-slate-200">预热K线</label>
                <div
                  class="min-h-[40px] rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
                  {{ warmupPreviewValue }}
                </div>
                <div class="text-xs text-slate-400 dark:text-slate-500">{{ warmupPreviewNote }}</div>
              </div>
            </div>

            <!-- Sync section -->
            <div v-if="syncing && !syncProgress"
              class="rounded-xl border border-teal-200 bg-teal-50/50 px-3 py-3 text-center dark:border-teal-800 dark:bg-teal-950/50">
              <span class="text-sm text-teal-700 dark:text-teal-300">正在启动同步…</span>
            </div>
            <div v-else-if="syncing && syncProgress"
              class="rounded-xl border border-teal-200 bg-teal-50/50 px-3 py-3 space-y-2 dark:border-teal-800 dark:bg-teal-950/50">
              <div class="flex items-center justify-between">
                <span class="text-xs font-semibold text-teal-800 dark:text-teal-200">
                  同步中 · {{ syncProgress.currentInterval || "准备" }}
                </span>
                <button
                  class="rounded-full border border-red-200 px-2 py-0.5 text-xs text-red-600 hover:bg-red-50 transition dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
                  type="button" @click="cancelSync">
                  取消
                </button>
              </div>
              <div class="h-2 rounded-full bg-teal-200 overflow-hidden dark:bg-teal-800">
                <div class="h-full rounded-full bg-teal-500 transition-all duration-500 dark:bg-teal-400"
                  :style="{ width: syncProgress.totalIntervals > 0 ? (syncProgress.completedIntervals / syncProgress.totalIntervals * 100) + '%' : '10%' }" />
              </div>
              <div class="flex items-center justify-between text-xs text-teal-700 dark:text-teal-300">
                <span>{{ syncProgress.completedBatches }} 批</span>
                <span v-if="syncProgress.retries > 0" class="text-amber-600 dark:text-amber-400">重试 {{
                  syncProgress.retries }}</span>
              </div>
            </div>
            <div v-else-if="syncProgress?.status === 'cancelled'"
              class="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300">
              同步已取消 · {{ syncProgress.completedBatches }} 批已完成
            </div>
            <!-- Sync button -->
            <button v-else
              class="w-full rounded-xl border border-teal-300 bg-teal-50 px-3 py-1.5 text-xs font-semibold text-teal-700 shadow-sm transition hover:bg-teal-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-teal-700 dark:bg-teal-950 dark:text-teal-300 dark:hover:bg-teal-900"
              :disabled="running" type="button" @click="syncKlines">
              ⬇ 同步历史K线
            </button>

            <!-- Run button -->
            <button
              class="w-full rounded-xl bg-teal-600 px-3 py-1.5 text-xs font-semibold text-white shadow-sm transition hover:bg-teal-700 disabled:cursor-not-allowed disabled:bg-slate-300 flex items-center justify-center gap-2 dark:bg-teal-500 dark:hover:bg-teal-600 dark:disabled:bg-slate-600"
              :disabled="running || !selectedDefinitionId" type="button" @click="startBacktest">
              <v-progress-circular v-if="running" indeterminate :size="16" :width="2" color="white" />
              {{ running ? "启动中..." : "▶ 开始回测" }}
            </button>

            <div
              class="rounded-lg border border-teal-100 bg-teal-50 px-2 py-1.5 text-xs text-teal-800 dark:border-teal-900 dark:bg-teal-950 dark:text-teal-200">
              ⚡ 先同步K线，再开始回测。
            </div>
          </div>
        </div>
      </div>

      <!-- Right: results list -->
      <div class="col-span-8 lg:col-span-9">
        <div class="grid gap-4">
          <!-- Results cards -->
          <div v-if="filteredRuns.length === 0" :class="[emptyStateClass, 'p-8 text-center text-sm']">
            暂无回测记录。请在左侧配置参数并启动回测。
          </div>

          <v-card v-for="run in filteredRuns" :key="run.id" flat :class="cardBorderClass">
            <v-card-item>
              <template #prepend>
                <v-chip :color="statusChip(run.status).color" size="small" variant="outlined">
                  {{ statusChip(run.status).label }}
                </v-chip>
              </template>
              <v-card-title>
                {{ resolveStrategyName(run.request.definitionId) }} · {{ run.request.symbol }} · {{ dayjs(run.request.startTime).format('YYYY-MM-DD') }} → {{
                  dayjs(run.request.endTime).format('YYYY-MM-DD') }}
              </v-card-title>
              <v-card-subtitle>
                {{ run.id }} · {{ run.request.interval }} · {{ run.request.initialBalance.toLocaleString() }} {{
                  resolveRunQuoteCurrency(run) }}
              </v-card-subtitle>
              <template #append>
                <v-btn v-if="run.result && (run.status === 'completed' || run.status === 'failed')"
                  :icon="expandedRuns[run.id] ? 'fa-solid fa-chevron-up' : 'fa-solid fa-chevron-down'" size="small"
                  variant="text" :title="expandedRuns[run.id] ? '收起结果' : '展开结果'" @click="toggleRun(run.id)" />
              </template>
            </v-card-item>

            <!-- Running / Queued progress -->
            <v-card-text v-if="run.status === 'running' || run.status === 'queued'" class="pb-0">
              <div class="flex items-center gap-3">
                <v-progress-linear v-if="run.status === 'running'" color="teal" indeterminate rounded :height="6"
                  class="flex-1" />
                <v-progress-linear v-else color="warning" indeterminate rounded :height="6" class="flex-1" />
                <span class="text-xs whitespace-nowrap"
                  :class="run.status === 'running' ? 'text-teal-600 dark:text-teal-400' : 'text-amber-600 dark:text-amber-400'">
                  {{ run.status === 'running' ? '回测运行中…' : '排队等待中…' }}
                </span>
              </div>
            </v-card-text>

            <v-card-text
              v-if="expandedRuns[run.id] && run.result && (run.status === 'completed' || run.status === 'failed')">
              <div class="grid grid-cols-2 gap-3 lg:grid-cols-6">
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">最终资金</div>
                  <div class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">
                    {{ run.result.finalBalance.toLocaleString(undefined, { minimumFractionDigits: 2 }) }}
                  </div>
                  <div class="text-xs text-slate-500 dark:text-slate-400">{{ resolveRunQuoteCurrency(run)
                  }}</div>
                </div>
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">收益</div>
                  <div class="mt-1 text-lg font-semibold" :class="pnlColor(run.result.pnl)">
                    {{ pnlPrefix(run.result.pnl) }}{{ run.result.pnl.toLocaleString(undefined, {
                      minimumFractionDigits:
                        2
                    }) }}
                  </div>
                  <div class="text-xs text-slate-500 dark:text-slate-400">{{ resolveRunQuoteCurrency(run)
                  }}</div>
                </div>
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">交易次数</div>
                  <div class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">
                    {{ run.result.totalTrades }}
                  </div>
                </div>
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">胜率</div>
                  <div class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">
                    {{ (run.result.winRate * 100).toFixed(1) }}%
                  </div>
                </div>
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">最大回撤</div>
                  <div class="mt-1 text-lg font-semibold" :class="drawdownColor(run.result.maxDrawdown)">
                    {{ formatPercentMetric(run.result.maxDrawdown) }}
                  </div>
                </div>
                <div :class="[statCardClass, 'px-3 py-3']">
                  <div class="text-xs uppercase tracking-[0.15em] text-slate-500 dark:text-slate-400">当前回撤</div>
                  <div class="mt-1 text-lg font-semibold" :class="drawdownColor(run.result.currentDrawdown)">
                    {{ formatPercentMetric(run.result.currentDrawdown) }}
                  </div>
                </div>
              </div>
              <div v-if="run.result && run.result.totalTrades === 0 && !run.result.error"
                class="mt-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300">
                未产生任何交易。可能原因：策略未调用 placeOrder()，或订阅的K线周期未同步。
              </div>
              <div v-if="resolveQueriedCandleBounds(run.result?.candles)"
                class="mt-2 rounded border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
                查询到的周期边界：左边界 {{ resolveQueriedCandleBounds(run.result?.candles)?.left }} ｜ 右边界 {{
                  resolveQueriedCandleBounds(run.result?.candles)?.right }} ｜ 共 {{
                  resolveQueriedCandleBounds(run.result?.candles)?.count }} 根
              </div>

              <!-- Backtest chart -->
              <div v-if="run.status === 'completed' && run.result?.pnlCurve?.length" class="mt-2">
                <BacktestChart :candles="run.result.candles ?? []" :trades="run.result.trades ?? []"
                  :pnl-curve="run.result.pnlCurve" :drawdown-curve="run.result.drawdownCurve ?? []"
                  :initial-balance="run.request.initialBalance" :min-height="560"
                  :currency-unit="resolveRunQuoteCurrency(run)"
                  empty-text="暂无权益曲线数据" />
              </div>

              <div v-if="run.result?.orderBook?.length" :class="[cardBorderClass, 'mt-3 overflow-hidden']">
                <details>
                  <summary
                    class="flex cursor-pointer items-center justify-between px-4 py-3 text-sm font-semibold text-slate-900 marker:content-none dark:text-slate-100">
                    <span>订单簿</span>
                    <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
                      {{ run.result.orderBook.length }} 笔 · 默认收起
                    </span>
                  </summary>
                  <div class="border-t border-slate-200 dark:border-slate-700">
                    <div class="max-h-96 overflow-auto">
                      <table class="min-w-full divide-y divide-slate-200 text-sm dark:divide-slate-700">
                        <thead
                          class="sticky top-0 bg-slate-50 text-left text-xs uppercase tracking-[0.14em] text-slate-500 dark:bg-slate-800/95 dark:text-slate-400">
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
                        <tbody class="divide-y divide-slate-100 bg-white dark:divide-slate-800 dark:bg-slate-900/50">
                          <tr v-for="entry in run.result.orderBook"
                            :key="`${entry.orderId}-${entry.filledAt ?? entry.submittedAt ?? ''}`">
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              <div>{{ formatBacktestTimestamp(entry.submittedAt) }}</div>
                              <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                                #{{ entry.orderId }}<span v-if="entry.clientOrderId"> · {{ entry.clientOrderId }}</span>
                              </div>
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              {{ formatBacktestTimestamp(entry.filledAt) }}
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              {{ formatBacktestOrderSide(entry.side) }}
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              <div>{{ formatBacktestQuantity(entry.quantity, entry.quantityText) }}</div>
                              <div v-if="entry.filledQuantity !== undefined"
                                class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                                成交 {{ formatBacktestQuantity(entry.filledQuantity, entry.filledQuantityText) }}
                              </div>
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              {{ formatBacktestOrderPrice(entry.orderPrice, entry.orderType, entry.orderPriceText) }}
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              {{ formatBacktestOrderPrice(entry.filledPrice, undefined, entry.filledPriceText) }}
                            </td>
                            <td class="px-4 py-3 align-top text-slate-700 dark:text-slate-200">
                              {{ formatBacktestOrderStatus(entry.status) }}
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </details>
              </div>
            </v-card-text>

            <!-- Runtime errors (e.g. insufficient balance, order rejections) -->
            <v-card-text v-if="run.result?.runtimeErrors && run.result.runtimeErrors.length > 0" class="pb-0">
              <details class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 dark:border-red-800 dark:bg-red-950">
                <summary class="cursor-pointer text-xs font-semibold text-red-700 select-none dark:text-red-300">
                  ⚡ 运行时错误 ({{ run.result.runtimeErrors.length }})
                </summary>
                <div class="mt-2 space-y-1 max-h-48 overflow-y-auto">
                  <div v-for="(err, i) in run.result.runtimeErrors" :key="i"
                    class="rounded border border-red-100 bg-white px-2 py-1 text-xs text-red-800 font-mono leading-relaxed dark:border-red-900 dark:bg-slate-800 dark:text-red-200">
                    {{ err }}
                  </div>
                </div>
              </details>
            </v-card-text>

            <!-- Diagnostic logs (always visible when present) -->
            <v-card-text v-if="run.result?.logs && run.result.logs.length > 0" class="pb-0">
              <div
                class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 space-y-1 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300">
                <div v-for="(log, i) in run.result.logs" :key="i">⚠ {{ log }}</div>
              </div>
            </v-card-text>
            <v-card-text v-if="run.result?.error" class="pb-0">
              <div
                class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 whitespace-pre-wrap dark:border-red-800 dark:bg-red-950 dark:text-red-300">
                {{ run.result.error }}
              </div>
            </v-card-text>
          </v-card>
        </div>
      </div>
    </div>
  </div>
</template>
