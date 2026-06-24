<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";

type ExchangeCalendarSessionWindow = {
  kind: string;
  startMinute: number;
  endMinute: number;
};

type ExchangeCalendarManualOverride = {
  market: string;
  date: string;
  status: string;
  sessions?: ExchangeCalendarSessionWindow[];
  reason?: string;
  observed?: boolean;
};

type ExchangeCalendarSourcePolicy = {
  market: string;
  preferredSourceIds?: string[];
  enabledSourceIds?: string[];
  fallbackToBuiltin: boolean;
  requireOfficial?: boolean;
  staleAfterHours?: number;
};

type ExchangeCalendarSettings = {
  autoRefreshEnabled: boolean;
  errorNotificationsEnabled: boolean;
  refreshIntervalHours: number;
  warmupMarkets?: string[];
  sourcePolicies?: ExchangeCalendarSourcePolicy[];
  manualOverrides?: ExchangeCalendarManualOverride[];
};

type ExchangeCalendarSettingsResponse = { exchangeCalendars: ExchangeCalendarSettings };

type CalendarMarketStatus = {
  market: string;
  effectiveSource: string;
  effectiveMode: string;
  effectiveReason: string;
  fallbackChain: string[];
  checkedAt: string;
};

type CalendarSourceStatus = {
  id: string;
  kind: string;
  authority: string;
  markets: string[];
  enabled: boolean;
  availabilityNote?: string;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  lastError?: string;
  consecutiveFailures?: number;
  nextRefreshAt?: string;
  lastSnapshotFetchedAt?: string;
  lastProbeAt?: string;
  lastProbeSuccessAt?: string;
  lastProbeFailureAt?: string;
  lastProbeStatus?: string;
  lastProbeError?: string;
  lastProbeMarket?: string;
  lastProbeSchedules?: number;
  healthState?: string;
  lastAlertAt?: string;
  lastAlertStatus?: string;
};

type CalendarSampleSchedule = {
  market: string;
  date: string;
  status: string;
  reason?: string;
  sourceId?: string;
  observed?: boolean;
  sessions?: ExchangeCalendarSessionWindow[];
};

type CalendarSnapshotSummary = {
  market: string;
  sourceId: string;
  from: string;
  to: string;
  fetchedAt: string;
  validUntil?: string;
  schedulesParsed: number;
  checksum?: string;
  sampleSchedules?: CalendarSampleSchedule[];
};

type CalendarStatusResponse = {
  autoRefreshEnabled: boolean;
  refreshIntervalHours: number;
  warmupMarkets?: string[];
  markets?: CalendarMarketStatus[];
  sources?: CalendarSourceStatus[];
  snapshots?: CalendarSnapshotSummary[];
};

type CalendarProbeResult = {
  sourceId: string;
  market: string;
  status: "healthy" | "unhealthy" | string;
  error?: string;
  fetchedAt?: string;
  validUntil?: string;
  schedulesParsed?: number;
  checksum?: string;
};

type CalendarProbeResponse = {
  accepted: boolean;
  market?: string;
  checkedAt?: string;
  healthy?: number;
  failures?: number;
  probeScope?: string[];
  results?: CalendarProbeResult[];
};

type CalendarRefreshResponse = {
  accepted: boolean;
  market?: string;
  updated?: number;
  failures?: number;
  requestedAt?: string;
  warmupMarkets?: string[];
};

const settings = ref<ExchangeCalendarSettings | null>(null);
const status = ref<CalendarStatusResponse | null>(null);
const probeResult = ref<CalendarProbeResponse | null>(null);
const refreshResults = ref<CalendarRefreshResponse[]>([]);
const selectedSourceId = ref("");
const loading = ref(true);
const saving = ref(false);
const probing = ref(false);
const refreshing = ref(false);
const statusMessage = ref("");
const errorMessage = ref("");

const markets = computed(() => status.value?.markets ?? []);
const sources = computed(() => status.value?.sources ?? []);
const snapshots = computed(() => status.value?.snapshots ?? []);
const warmupMarkets = computed(() => settings.value?.warmupMarkets ?? status.value?.warmupMarkets ?? []);
const activeSourceIDs = computed(() => new Set(markets.value.map((market) => market.effectiveSource)));

const sortedSources = computed(() =>
  [...sources.value].sort((left, right) => sourceRank(left) - sourceRank(right) || left.id.localeCompare(right.id)),
);

const selectedSource = computed(() =>
  sortedSources.value.find((source) => source.id === selectedSourceId.value) ?? sortedSources.value[0] ?? null,
);

const selectedSourceMarkets = computed(() => selectedSource.value?.markets ?? []);
const selectedNetworkMarkets = computed(() =>
  selectedSourceMarkets.value.filter((market) => market !== "" && !isLocalSource(selectedSource.value)),
);
const probeDisabled = computed(() => probing.value || selectedNetworkMarkets.value.length === 0);
const refreshDisabled = computed(() => refreshing.value || selectedNetworkMarkets.value.length === 0);
const selectedSnapshots = computed(() =>
  snapshots.value.filter((snapshot) => snapshot.sourceId === selectedSource.value?.id),
);
const selectedProbeResults = computed(() => {
  const source = selectedSource.value;
  if (source == null || probeResult.value == null) return [];
  const marketSet = new Set(source.markets);
  return (probeResult.value.results ?? []).filter(
    (result) => result.sourceId === source.id || marketSet.has(result.market),
  );
});
const selectedRefreshResults = computed(() => {
  const source = selectedSource.value;
  if (source == null) return [];
  const marketSet = new Set(source.markets);
  return refreshResults.value.filter(
    (result) => result.market == null || result.market === "" || marketSet.has(result.market),
  );
});

onMounted(() => {
  void loadAll();
});

async function loadAll(): Promise<void> {
  loading.value = true;
  errorMessage.value = "";
  try {
    const [settingsResponse, statusResponse] = await Promise.all([
      fetchEnvelope<ExchangeCalendarSettingsResponse>("/api/v1/settings/exchange-calendars"),
      fetchEnvelope<CalendarStatusResponse>("/api/v1/system/exchange-calendars/status"),
    ]);
    settings.value = settingsResponse.exchangeCalendars;
    status.value = statusResponse;
    ensureSelectedSource();
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "读取交易所日历状态失败";
  } finally {
    loading.value = false;
  }
}

async function reloadStatus(): Promise<void> {
  errorMessage.value = "";
  try {
    status.value = await fetchEnvelope<CalendarStatusResponse>("/api/v1/system/exchange-calendars/status");
    ensureSelectedSource();
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "刷新交易所日历状态失败";
  }
}

function ensureSelectedSource(): void {
  if (sources.value.some((source) => source.id === selectedSourceId.value)) return;
  selectedSourceId.value =
    sortedSources.value.find((source) => source.healthState === "unhealthy")?.id ??
    sortedSources.value.find((source) => activeSourceIDs.value.has(source.id))?.id ??
    sortedSources.value.find((source) => source.enabled)?.id ??
    sortedSources.value[0]?.id ??
    "";
}

function selectSource(sourceID: string): void {
  selectedSourceId.value = sourceID;
}

async function updateErrorNotifications(event: Event): Promise<void> {
  const target = event.target as HTMLInputElement | null;
  if (settings.value == null || target == null || saving.value) return;
  const previous = settings.value;
  const next: ExchangeCalendarSettings = { ...previous, errorNotificationsEnabled: target.checked };
  settings.value = next;
  saving.value = true;
  statusMessage.value = "";
  errorMessage.value = "";
  try {
    const response = await fetchEnvelopeWithInit<ExchangeCalendarSettingsResponse>(
      "/api/v1/settings/exchange-calendars",
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ exchangeCalendars: next }),
      },
    );
    settings.value = response.exchangeCalendars;
    statusMessage.value = response.exchangeCalendars.errorNotificationsEnabled ? "错误推送已开启。" : "错误推送已关闭。";
  } catch (error) {
    settings.value = previous;
    errorMessage.value = error instanceof Error ? error.message : "保存交易所日历设置失败";
  } finally {
    saving.value = false;
  }
}

async function probeSelectedSource(): Promise<void> {
  if (probing.value || selectedSource.value == null || selectedNetworkMarkets.value.length === 0) return;
  probing.value = true;
  errorMessage.value = "";
  statusMessage.value = "";
  try {
    probeResult.value = await runMarketProbe(selectedNetworkMarkets.value);
    statusMessage.value = `${selectedSource.value.id} 支持市场探测已完成。`;
    await reloadStatus();
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "执行交易所日历探测失败";
  } finally {
    probing.value = false;
  }
}

async function refreshSelectedSource(): Promise<void> {
  if (refreshing.value || selectedSource.value == null || selectedNetworkMarkets.value.length === 0) return;
  refreshing.value = true;
  errorMessage.value = "";
  statusMessage.value = "";
  try {
    refreshResults.value = [];
    for (const market of selectedNetworkMarkets.value) {
      refreshResults.value.push(
        await fetchEnvelopeWithInit<CalendarRefreshResponse>(
          `/api/v1/system/exchange-calendars/refresh/${encodeURIComponent(market)}`,
          { method: "POST" },
        ),
      );
    }
    statusMessage.value = `${selectedSource.value.id} 支持市场刷新已完成。`;
    await reloadStatus();
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "刷新交易所日历失败";
  } finally {
    refreshing.value = false;
  }
}

async function runMarketProbe(targetMarkets: string[]): Promise<CalendarProbeResponse> {
  const results: CalendarProbeResult[] = [];
  let healthy = 0;
  let failures = 0;
  let checkedAt = "";
  for (const market of targetMarkets) {
    const response = await fetchEnvelopeWithInit<CalendarProbeResponse>(
      `/api/v1/system/exchange-calendars/probe/${encodeURIComponent(market)}`,
      { method: "POST" },
    );
    results.push(...(response.results ?? []));
    healthy += response.healthy ?? 0;
    failures += response.failures ?? 0;
    checkedAt = response.checkedAt ?? checkedAt;
  }
  return { accepted: true, checkedAt, healthy, failures, probeScope: targetMarkets, results };
}

function sourceRank(source: CalendarSourceStatus): number {
  if (source.healthState === "unhealthy") return 0;
  if (activeSourceIDs.value.has(source.id)) return 10;
  if (source.enabled) return 20;
  return 30;
}

function isLocalSource(source: CalendarSourceStatus | null): boolean {
  return source?.id === "manual_override" || source?.id === "builtin_rules";
}

function marketModeLabel(mode: string): string {
  return {
    manual_override: "人工覆盖",
    remote_override: "使用外部日历",
    remote_covered_day: "使用外部日历",
    builtin_fallback: "内置兜底",
  }[mode] ?? mode;
}

function marketModeDescription(market: CalendarMarketStatus): string {
  if (market.effectiveMode === "builtin_fallback") return "当前未使用外部日历，正在使用内置规则。";
  if (market.effectiveMode === "remote_override") return "外部日历对当前日期给出了特殊交易安排。";
  if (market.effectiveMode === "remote_covered_day") return "外部快照覆盖当前日期，标准时段由内置模板补齐。";
  if (market.effectiveMode === "manual_override") return "设置中的人工覆盖正在生效。";
  return market.effectiveReason;
}

function marketModeClass(mode: string): string {
  if (mode === "builtin_fallback") {
    return "border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-400/60 dark:bg-amber-950/40 dark:text-amber-100";
  }
  if (mode === "manual_override") {
    return "border-sky-300 bg-sky-50 text-sky-800 dark:border-sky-400/60 dark:bg-sky-950/40 dark:text-sky-100";
  }
  return "border-emerald-300 bg-emerald-50 text-emerald-800 dark:border-emerald-400/60 dark:bg-emerald-950/40 dark:text-emerald-100";
}

function sourceUsageLabel(source: CalendarSourceStatus): string {
  const activeMarkets = markets.value.filter((market) => market.effectiveSource === source.id).map((market) => market.market);
  return activeMarkets.length > 0 ? `正在服务 ${activeMarkets.join(" / ")}` : "当前未被市场使用";
}

function sourceMarketUsageText(market: CalendarMarketStatus, source: CalendarSourceStatus): string {
  if (market.effectiveSource === source.id) return `当前使用 ${source.id}`;
  if (market.effectiveMode === "builtin_fallback") return "当前未使用外部日历，正在使用内置规则";
  return `当前使用 ${market.effectiveSource}`;
}

function formatDateTime(value?: string): string {
  if (value == null || value === "" || value.startsWith("0001-")) return "未记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatDate(value?: string): string {
  if (value == null || value === "") return "未记录";
  return value.slice(0, 10);
}

function healthLabel(source: CalendarSourceStatus): string {
  if (source.healthState === "healthy") return "健康";
  if (source.healthState === "unhealthy") return "异常";
  if (source.enabled) return "待检测";
  return "未启用";
}

function healthClass(source: CalendarSourceStatus): string {
  if (source.healthState === "healthy") return "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/50 dark:bg-emerald-950/60 dark:text-emerald-200";
  if (source.healthState === "unhealthy") return "border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-500/50 dark:bg-rose-950/60 dark:text-rose-200";
  return source.enabled
    ? "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/50 dark:bg-amber-950/60 dark:text-amber-200"
    : "border-slate-200 bg-slate-50 text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400";
}

function probeClass(result: CalendarProbeResult): string {
  return result.status === "healthy"
    ? "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/50 dark:bg-emerald-950/60 dark:text-emerald-200"
    : "border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-500/50 dark:bg-rose-950/60 dark:text-rose-200";
}

function currentSourceError(source: CalendarSourceStatus): string {
  if (source.healthState !== "unhealthy") return "";
  return source.lastProbeError || source.lastError || "";
}
</script>

<template>
  <section class="grid gap-5">
    <header class="rounded-lg border border-slate-200 bg-white px-5 py-5 dark:border-slate-700 dark:bg-slate-900">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold text-slate-900 dark:text-slate-100">交易所日历</h2>
          <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">先看市场正在使用什么，再定位对应数据源的健康、缓存和错误。</p>
        </div>
        <button
          data-testid="calendar-reload-status"
          type="button"
          class="rounded-md border border-slate-200 px-3 py-2 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
          :disabled="loading"
          @click="reloadStatus"
        >
          刷新状态
        </button>
      </div>

      <div class="mt-5 grid gap-3 md:grid-cols-4">
        <div class="rounded-md border border-slate-200 px-4 py-3 dark:border-slate-700 dark:bg-slate-950/40">
          <div class="text-xs text-slate-400 dark:text-slate-500">自动刷新</div>
          <div class="mt-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{{ (settings?.autoRefreshEnabled ?? status?.autoRefreshEnabled) ? "已开启" : "已关闭" }}</div>
        </div>
        <div class="rounded-md border border-slate-200 px-4 py-3 dark:border-slate-700 dark:bg-slate-950/40">
          <div class="text-xs text-slate-400 dark:text-slate-500">刷新间隔</div>
          <div class="mt-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{{ settings?.refreshIntervalHours ?? status?.refreshIntervalHours ?? "-" }} 小时</div>
        </div>
        <div class="rounded-md border border-slate-200 px-4 py-3 dark:border-slate-700 dark:bg-slate-950/40">
          <div class="text-xs text-slate-400 dark:text-slate-500">预热市场</div>
          <div class="mt-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{{ warmupMarkets.length > 0 ? warmupMarkets.join(" / ") : "-" }}</div>
        </div>
        <label class="flex items-center justify-between gap-4 rounded-md border border-slate-200 px-4 py-3 dark:border-slate-700 dark:bg-slate-950/40">
          <span>
            <span class="block text-xs text-slate-400 dark:text-slate-500">错误推送</span>
            <span class="mt-1 block text-sm font-semibold text-slate-900 dark:text-slate-100">{{ settings?.errorNotificationsEnabled ? "已开启" : "已关闭" }}</span>
          </span>
          <input
            data-testid="calendar-error-notifications-toggle"
            type="checkbox"
            class="h-5 w-5 cursor-pointer accent-slate-900 disabled:cursor-not-allowed dark:accent-slate-100"
            :checked="settings?.errorNotificationsEnabled ?? true"
            :disabled="loading || saving || settings == null"
            @change="updateErrorNotifications"
          />
        </label>
      </div>

      <p v-if="statusMessage" class="mt-4 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs font-medium text-emerald-700 dark:border-emerald-500/50 dark:bg-emerald-950/60 dark:text-emerald-200">{{ statusMessage }}</p>
      <p v-if="errorMessage" class="mt-4 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs font-medium text-rose-700 dark:border-rose-500/50 dark:bg-rose-950/60 dark:text-rose-200">{{ errorMessage }}</p>
    </header>

    <div v-if="loading" class="rounded-lg border border-slate-200 bg-white px-5 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-400">
      正在读取交易所日历状态…
    </div>

    <template v-else>
      <section class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        <article
          v-for="market in markets"
          :key="market.market"
          class="rounded-lg border px-5 py-4 shadow-sm transition-colors dark:shadow-none"
          :class="marketModeClass(market.effectiveMode)"
          :data-testid="`calendar-market-${market.market}`"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-lg font-semibold">{{ market.market }}</div>
              <div class="mt-1 text-xs font-semibold">{{ marketModeLabel(market.effectiveMode) }}</div>
            </div>
            <button
              type="button"
              class="rounded border border-current bg-white/60 px-2 py-1 text-xs font-semibold transition hover:bg-white/90 focus:outline-none focus:ring-2 focus:ring-current focus:ring-offset-2 focus:ring-offset-white dark:bg-slate-950/30 dark:hover:bg-slate-950/60 dark:focus:ring-offset-slate-950"
              @click="selectSource(market.effectiveSource)"
            >
              {{ market.effectiveSource }}
            </button>
          </div>
          <p class="mt-3 text-xs leading-5 opacity-90 dark:opacity-95">{{ marketModeDescription(market) }}</p>
          <div class="mt-3 text-[11px] opacity-75 dark:opacity-80">检查时间 {{ formatDateTime(market.checkedAt) }}</div>
        </article>
      </section>

      <section class="grid gap-4 xl:grid-cols-[320px_1fr]">
        <aside class="rounded-lg border border-slate-200 bg-white px-3 py-3 dark:border-slate-700 dark:bg-slate-900">
          <div class="px-2 pb-2 text-sm font-semibold text-slate-900 dark:text-slate-100">数据源</div>
          <div class="grid gap-2">
            <button
              v-for="source in sortedSources"
              :key="source.id"
              type="button"
              class="rounded-md border px-3 py-3 text-left transition hover:bg-slate-50 dark:hover:bg-slate-800"
              :class="selectedSource?.id === source.id ? 'border-slate-900 bg-slate-50 dark:border-slate-400 dark:bg-slate-800' : 'border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900'"
              :data-testid="`calendar-source-nav-${source.id}`"
              @click="selectSource(source.id)"
            >
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <div class="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{{ source.id }}</div>
                  <div class="mt-1 text-[11px] text-slate-500 dark:text-slate-400">{{ source.kind }} / {{ source.authority }}</div>
                </div>
                <span class="shrink-0 rounded border px-2 py-0.5 text-[11px] font-medium" :class="healthClass(source)">{{ healthLabel(source) }}</span>
              </div>
              <div class="mt-2 flex flex-wrap gap-1">
                <span v-for="market in source.markets" :key="`${source.id}-${market}`" class="rounded border border-slate-200 px-1.5 py-0.5 text-[11px] text-slate-500 dark:border-slate-700 dark:text-slate-400">{{ market }}</span>
              </div>
              <div class="mt-2 text-[11px]" :class="activeSourceIDs.has(source.id) ? 'font-semibold text-emerald-700 dark:text-emerald-300' : 'text-slate-500 dark:text-slate-400'">
                {{ sourceUsageLabel(source) }}
              </div>
              <div v-if="currentSourceError(source)" class="mt-1 line-clamp-2 text-[11px] text-rose-600 dark:text-rose-300">
                {{ currentSourceError(source) }}
              </div>
            </button>
          </div>
        </aside>

        <article v-if="selectedSource" class="rounded-lg border border-slate-200 bg-white px-5 py-5 dark:border-slate-700 dark:bg-slate-900">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-base font-semibold text-slate-900 dark:text-slate-100">{{ selectedSource.id }}</h3>
                <span class="rounded border px-2 py-0.5 text-xs font-medium" :class="healthClass(selectedSource)">{{ healthLabel(selectedSource) }}</span>
                <span v-if="activeSourceIDs.has(selectedSource.id)" class="rounded border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:border-emerald-500/50 dark:bg-emerald-950/60 dark:text-emerald-200">当前生效</span>
              </div>
              <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">{{ selectedSource.kind }} / {{ selectedSource.authority }}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <button
                data-testid="calendar-probe"
                type="button"
                class="rounded-md border px-3 py-2 text-xs font-semibold transition"
                :class="probeDisabled ? 'cursor-not-allowed border-slate-200 text-slate-400 dark:border-slate-700 dark:text-slate-500' : 'border-slate-900 text-slate-900 hover:bg-slate-50 dark:border-slate-300 dark:text-slate-100 dark:hover:bg-slate-800'"
                :disabled="probeDisabled"
                @click="probeSelectedSource"
              >
                {{ probing ? "探测中" : "在线探测" }}
              </button>
              <button
                data-testid="calendar-refresh"
                type="button"
                class="rounded-md border px-3 py-2 text-xs font-semibold transition"
                :class="refreshDisabled ? 'cursor-not-allowed border-slate-200 text-slate-400 dark:border-slate-700 dark:text-slate-500' : 'border-slate-900 text-slate-900 hover:bg-slate-50 dark:border-slate-300 dark:text-slate-100 dark:hover:bg-slate-800'"
                :disabled="refreshDisabled"
                @click="refreshSelectedSource"
              >
                {{ refreshing ? "刷新中" : "立即刷新" }}
              </button>
            </div>
          </div>

          <div v-if="selectedNetworkMarkets.length === 0" class="mt-4 rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
            {{ selectedSource.id }} 是本地规则源，不需要在线探测或刷新。
          </div>

          <dl class="mt-5 grid gap-3 text-xs md:grid-cols-4">
            <div class="rounded-md border border-slate-200 px-3 py-2 dark:border-slate-700 dark:bg-slate-950/40">
              <dt class="text-slate-400 dark:text-slate-500">启用状态</dt>
              <dd class="mt-1 font-semibold text-slate-800 dark:text-slate-200">{{ selectedSource.enabled ? "已启用" : "未启用" }}</dd>
            </div>
            <div class="rounded-md border border-slate-200 px-3 py-2 dark:border-slate-700 dark:bg-slate-950/40">
              <dt class="text-slate-400 dark:text-slate-500">上次成功</dt>
              <dd class="mt-1 font-semibold text-slate-800 dark:text-slate-200">{{ formatDateTime(selectedSource.lastSuccessAt) }}</dd>
            </div>
            <div class="rounded-md border border-slate-200 px-3 py-2 dark:border-slate-700 dark:bg-slate-950/40">
              <dt class="text-slate-400 dark:text-slate-500">上次失败</dt>
              <dd class="mt-1 font-semibold text-slate-800 dark:text-slate-200">{{ formatDateTime(selectedSource.lastFailureAt) }}</dd>
            </div>
            <div class="rounded-md border border-slate-200 px-3 py-2 dark:border-slate-700 dark:bg-slate-950/40">
              <dt class="text-slate-400 dark:text-slate-500">下次刷新</dt>
              <dd class="mt-1 font-semibold text-slate-800 dark:text-slate-200">{{ formatDateTime(selectedSource.nextRefreshAt) }}</dd>
            </div>
          </dl>

          <div v-if="currentSourceError(selectedSource)" class="mt-4 rounded-md border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-500/50 dark:bg-rose-950/60 dark:text-rose-200">
            {{ currentSourceError(selectedSource) }}
          </div>
          <div v-if="!currentSourceError(selectedSource) && selectedSource.availabilityNote" class="mt-4 rounded-md border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
            {{ selectedSource.availabilityNote }}
          </div>

          <section class="mt-6">
            <h4 class="text-sm font-semibold text-slate-900 dark:text-slate-100">当前市场使用情况</h4>
            <div class="mt-3 grid gap-2">
              <div v-for="market in markets" :key="`${selectedSource.id}-${market.market}`" class="grid gap-2 rounded-md border border-slate-200 px-3 py-3 text-xs md:grid-cols-[80px_160px_1fr] dark:border-slate-700 dark:bg-slate-950/40">
                <div class="font-semibold text-slate-900 dark:text-slate-100">{{ market.market }}</div>
                <div>
                  <span class="rounded border px-2 py-0.5 font-medium" :class="marketModeClass(market.effectiveMode)">{{ marketModeLabel(market.effectiveMode) }}</span>
                </div>
                <div class="text-slate-600 dark:text-slate-300">{{ sourceMarketUsageText(market, selectedSource) }}</div>
              </div>
            </div>
          </section>

          <section class="mt-6">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <h4 class="text-sm font-semibold text-slate-900 dark:text-slate-100">缓存日历数据</h4>
              <span class="text-xs text-slate-400 dark:text-slate-500">{{ selectedSnapshots.length }} 个快照</span>
            </div>
            <div v-if="selectedSnapshots.length === 0" class="mt-3 rounded-md border border-dashed border-slate-200 px-4 py-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
              该源暂无外部日历缓存。
            </div>
            <div v-else class="mt-3 grid gap-3">
              <article v-for="snapshot in selectedSnapshots" :key="`${snapshot.market}-${snapshot.sourceId}-${snapshot.from}`" class="rounded-md border border-slate-200 px-4 py-4 dark:border-slate-700 dark:bg-slate-950/40" :data-testid="`calendar-snapshot-${snapshot.market}-${snapshot.sourceId}`">
                <div class="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div class="text-sm font-semibold text-slate-900 dark:text-slate-100">{{ snapshot.market }} / {{ snapshot.sourceId }}</div>
                    <div class="mt-1 text-xs text-slate-500 dark:text-slate-400">{{ formatDate(snapshot.from) }} 至 {{ formatDate(snapshot.to) }}</div>
                  </div>
                  <span class="rounded border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:border-emerald-500/50 dark:bg-emerald-950/60 dark:text-emerald-200">{{ snapshot.schedulesParsed }} 条</span>
                </div>
                <dl class="mt-4 grid gap-3 text-xs sm:grid-cols-3">
                  <div>
                    <dt class="text-slate-400 dark:text-slate-500">获取时间</dt>
                    <dd class="mt-1 font-medium text-slate-700 dark:text-slate-300">{{ formatDateTime(snapshot.fetchedAt) }}</dd>
                  </div>
                  <div>
                    <dt class="text-slate-400 dark:text-slate-500">有效至</dt>
                    <dd class="mt-1 font-medium text-slate-700 dark:text-slate-300">{{ formatDateTime(snapshot.validUntil) }}</dd>
                  </div>
                  <div>
                    <dt class="text-slate-400 dark:text-slate-500">校验值</dt>
                    <dd class="mt-1 break-all font-mono text-[11px] text-slate-600 dark:text-slate-400">{{ snapshot.checksum || "-" }}</dd>
                  </div>
                </dl>
                <div v-if="snapshot.sampleSchedules?.length" class="mt-4 flex flex-wrap gap-2">
                  <span v-for="sample in snapshot.sampleSchedules" :key="`${snapshot.sourceId}-${sample.date}-${sample.status}`" class="rounded border border-amber-200 bg-amber-50 px-2 py-1 text-xs text-amber-800 dark:border-amber-500/50 dark:bg-amber-950/60 dark:text-amber-200">
                    {{ sample.date }} {{ sample.status }}{{ sample.reason ? ` / ${sample.reason}` : "" }}
                  </span>
                </div>
              </article>
            </div>
          </section>

          <section v-if="probeResult || selectedRefreshResults.length > 0" class="mt-6 grid gap-4 lg:grid-cols-2">
            <div v-if="probeResult" class="rounded-md border border-slate-200 px-4 py-4 dark:border-slate-700 dark:bg-slate-950/40">
              <div class="flex items-center justify-between gap-3">
                <h4 class="text-sm font-semibold text-slate-900 dark:text-slate-100">在线探测结果</h4>
                <span class="text-xs text-slate-400 dark:text-slate-500">{{ selectedProbeResults.length }} 条</span>
              </div>
              <div class="mt-3 grid gap-2">
                <div v-for="result in selectedProbeResults" :key="`${result.market}-${result.sourceId}`" class="rounded border px-3 py-2 text-xs" :class="probeClass(result)" :data-testid="`calendar-probe-result-${result.market}-${result.sourceId}`">
                  <div class="font-semibold">{{ result.market }} / {{ result.sourceId }}</div>
                  <div class="mt-1">
                    <template v-if="result.status === 'healthy'">解析 {{ result.schedulesParsed ?? 0 }} 条，有效至 {{ formatDateTime(result.validUntil) }}</template>
                    <template v-else>{{ result.error || "探测失败，未返回错误原因" }}</template>
                  </div>
                </div>
                <div v-if="selectedProbeResults.length === 0" class="text-xs text-slate-500 dark:text-slate-400">最近探测没有返回该源相关结果。</div>
              </div>
            </div>
            <div v-if="selectedRefreshResults.length > 0" class="rounded-md border border-slate-200 px-4 py-4 dark:border-slate-700 dark:bg-slate-950/40">
              <h4 class="text-sm font-semibold text-slate-900 dark:text-slate-100">刷新结果</h4>
              <div class="mt-3 grid gap-2 text-xs text-slate-600 dark:text-slate-300">
                <div v-for="result in selectedRefreshResults" :key="`${result.market}-${result.requestedAt}`" class="rounded border border-slate-200 px-3 py-2 dark:border-slate-700 dark:bg-slate-900">
                  {{ result.market || "全部市场" }}：更新 {{ result.updated ?? 0 }} 个源，失败 {{ result.failures ?? 0 }} 个源
                </div>
              </div>
            </div>
          </section>
        </article>
      </section>
    </template>
  </section>
</template>
