<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import {
  KLINE_PERIODS,
  overlayRealtimeTickCandle,
  resolveKlineCandleDisplayAt,
  type KlineCandle,
} from "../charting/kline";
import KlineChart from "../components/KlineChart.vue";
import PageHeader from "../components/PageHeader.vue";
import SectionHeader from "../components/SectionHeader.vue";
import { createMarketDataSubscriptionScope } from "../composables/consoleDataMarketSubscriptionScope";
import {
  formatConnectivityLabel,
  formatDateTime,
  formatGenericStatusLabel,
  formatMarketDataChannelLabel,
  formatMarketLabel,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import { useSharedLiveSocket } from "../composables/useSharedLiveSocket";

const {
  isLoadingMarketData,
  isLoadingMarketDataQuery,
  loadMarketDataQuery,
  loadMarketInstrumentReferences,
  loadMarketDataSubscriptions,
  marketDataCandles,
  marketDataError,
  marketDataQueryError,
  marketDataQueryLimit,
  marketDataQueryMarket,
  marketDataQueryPeriod,
  marketDataQuerySymbol,
  marketDataSnapshot,
  marketDataSubscriptions,
  marketInstrumentSearchOptions,
  acquireMarketDataSubscription,
  createStableWebConsumerId,
  heartbeatMarketDataConsumer,
  releaseMarketDataSubscription,
  resolveMarketInstrumentInput,
} = useConsoleData();
const live = useSharedLiveSocket();

const marketOptions = ["HK", "US", "CN", "SG", "JP", "AU", "MY", "CA", "CRYPTO"].map(
  (market) => ({ value: market, title: formatMarketLabel(market) }),
);
const marketPageConsumerId = createStableWebConsumerId("market-page");
const symbolSearchText = ref(marketDataQuerySymbol.value);
const activeResultPanels = ref(["query-results"]);
const isLoadingOlderCandles = ref(false);
let queryRefreshTimer: number | null = null;
const marketDataSubscriptionScope = createMarketDataSubscriptionScope({
  consumerId: marketPageConsumerId,
  acquireMarketDataSubscription,
  releaseMarketDataSubscription,
  heartbeatMarketDataConsumer,
});

function currentVisibleSubscription() {
  const rawSymbol = marketDataQuerySymbol.value.trim().toUpperCase();
  const separator = rawSymbol.includes(":") ? ":" : ".";
  if (rawSymbol.includes(separator)) {
    const [market, symbol] = rawSymbol.split(separator, 2);
    return {
      market: (market ?? "").trim().toUpperCase(),
      symbol: (symbol ?? "").trim().toUpperCase(),
    };
  }

  return {
    market: marketDataQueryMarket.value.trim().toUpperCase(),
    symbol: rawSymbol,
  };
}

async function syncVisibleSubscription(): Promise<void> {
  await marketDataSubscriptionScope.syncTarget(currentVisibleSubscription());
}

onMounted(() => {
  void (async () => {
    symbolSearchText.value = marketDataQuerySymbol.value;
    await loadMarketInstrumentReferences();
    await loadMarketDataSubscriptions();
    await syncVisibleSubscription();
    await loadMarketDataQuery();
    scheduleMarketDataAutoRefresh();
    marketDataSubscriptionScope.startHeartbeat();
  })();
});

watch([marketDataQueryMarket, marketDataQuerySymbol], () => {
  void syncVisibleSubscription();
});

watch(
  [
    marketDataQueryMarket,
    marketDataQuerySymbol,
    marketDataQueryPeriod,
    marketDataQueryLimit,
  ],
  () => {
    symbolSearchText.value = marketDataQuerySymbol.value;
    void loadMarketDataQuery();
    scheduleMarketDataAutoRefresh();
  },
);

onUnmounted(() => {
  if (queryRefreshTimer != null) {
    window.clearTimeout(queryRefreshTimer);
    queryRefreshTimer = null;
  }

  void marketDataSubscriptionScope.cleanup({ keepalive: true });
});

const marketHeaderStats = computed(() => [
  {
    label: "活跃订阅",
    value: marketDataSubscriptions.value.totalActiveSubscriptions,
  },
  {
    label: "配额",
    value: `${marketDataSubscriptions.value.quota.totalUsed} / ${marketDataSubscriptions.value.quota.totalLimit ?? "∞"}`,
  },
  {
    label: "快照",
    value: formatGenericStatusLabel(marketDataSnapshot.value?.snapshot ? "READY" : "EMPTY"),
    tone: marketDataSnapshot.value?.snapshot ? "good" : "warn",
  },
  {
    label: "K线",
    value: marketDataCandles.value?.totalReturned ?? 0,
    hint: `${marketDataQueryMarket.value}.${marketDataQuerySymbol.value}`,
  },
]);

const chartCandles = computed<KlineCandle[]>(() =>
  overlayRealtimeTickCandle(
    marketDataCandles.value?.candles ?? [],
    marketDataSnapshot.value?.snapshot ?? null,
    marketDataQueryPeriod.value,
  ),
);
const periods = KLINE_PERIODS;

const selectedInstrumentParts = computed(() => currentVisibleSubscription());
const selectedMarketInstrument = computed(() => {
  const instrumentId = `${selectedInstrumentParts.value.market}.${selectedInstrumentParts.value.symbol}`;
  return (
    marketInstrumentSearchOptions.value.find(
      (option) => option.instrumentId === instrumentId,
    ) ?? null
  );
});

const selectedInstrumentTitle = computed(() => {
  const instrumentId = `${selectedInstrumentParts.value.market}.${selectedInstrumentParts.value.symbol}`;
  const name = selectedMarketInstrument.value?.name;
  return name == null || name === ""
    ? instrumentId
    : `${instrumentId} · ${name}`;
});

const latestQuoteSnapshot = computed(
  () => marketDataSnapshot.value?.snapshot ?? null,
);
const latestQuoteReferencePrice = computed(() => {
  const snapshot = latestQuoteSnapshot.value;
  if (snapshot == null) {
    return null;
  }

  return snapshot.previousClosePrice ?? snapshot.openPrice ?? null;
});
const latestQuoteChange = computed(() => {
  const snapshot = latestQuoteSnapshot.value;
  const referencePrice = latestQuoteReferencePrice.value;
  if (snapshot == null || referencePrice == null) {
    return null;
  }

  return snapshot.price - referencePrice;
});
const latestQuoteChangePercent = computed(() => {
  const referencePrice = latestQuoteReferencePrice.value;
  const change = latestQuoteChange.value;
  if (referencePrice == null || referencePrice === 0 || change == null) {
    return null;
  }

  return (change / referencePrice) * 100;
});
const latestQuoteToneClass = computed(() => {
  const change = latestQuoteChange.value;
  if (change == null || change === 0) {
    return "text-slate-900";
  }

  return change > 0 ? "tv-up" : "tv-down";
});
const latestQuoteChangeLabel = computed(() => {
  const change = latestQuoteChange.value;
  const percent = latestQuoteChangePercent.value;
  if (change == null || percent == null) {
    return "暂无";
  }

  const prefix = change > 0 ? "+" : "";
  return `${prefix}${change.toFixed(3)} (${prefix}${percent.toFixed(2)}%)`;
});
const liveQuoteStateLabel = computed(() => {
  return `实时通道：${formatConnectivityLabel(live.connectionState.value)}`;
});

function formatMarketDataSourceLabel(source: string | null | undefined): string {
  switch ((source ?? "").trim().toLowerCase()) {
    case "cache":
      return "缓存";
    case "live":
      return "实时";
    case "provider":
      return "数据源";
    default:
      return source == null || source === "" ? "缓存" : source;
  }
}

const historicalPriceSummary = computed(() => {
  const candles = chartCandles.value;
  if (candles.length === 0) {
    return null;
  }

  const first = candles[0];
  const last = candles[candles.length - 1];
  if (first == null || last == null) {
    return null;
  }

  const high = Math.max(...candles.map((candle) => candle.high));
  const low = Math.min(...candles.map((candle) => candle.low));
  const change = last.close - first.open;
  const changePct = first.open === 0 ? 0 : (change / first.open) * 100;
  const volume = candles.reduce((total, candle) => total + candle.volume, 0);

  return {
    firstOpen: first.open,
    lastClose: last.close,
    high,
    low,
    change,
    changePct,
    volume,
    from: resolveKlineCandleDisplayAt(first),
    to: resolveKlineCandleDisplayAt(last),
  };
});

async function queryInstrumentSuggestions(
  query: string,
  cb: (suggestions: Array<{ value: string; label: string }>) => void,
): Promise<void> {
  await loadMarketInstrumentReferences(query);
  const normalized = query.trim().toUpperCase();
  cb(
    marketInstrumentSearchOptions.value
      .filter((option) => {
        if (normalized === "") {
          return true;
        }
        return (
          option.instrumentId.includes(normalized) ||
          option.symbol.includes(normalized) ||
          option.lookupValue.includes(normalized) ||
          (option.name ?? "").toUpperCase().includes(normalized)
        );
      })
      .slice(0, 20)
      .map((option) => ({
        value: option.lookupValue,
        label: option.label,
      })),
  );
}

function normalizeManualSymbolInput(value: string): string {
  const input = value.trim().toUpperCase();
  if (/^\d{1,5}$/.test(input) && marketDataQueryMarket.value === "HK") {
    return input.padStart(5, "0");
  }

  return input;
}

function selectInstrumentSuggestion(item: {
  value: string;
  label?: string;
}): void {
  const [market, symbol] = item.value.split(":", 2);
  if (market == null || symbol == null || market === "" || symbol === "") {
    return;
  }

  marketDataQueryMarket.value = market;
  marketDataQuerySymbol.value = symbol;
  symbolSearchText.value = symbol;
}

async function runMarketDataQuery(): Promise<void> {
  const parsed = resolveMarketInstrumentInput(
    normalizeManualSymbolInput(symbolSearchText.value),
  );
  if (parsed != null) {
    marketDataQueryMarket.value = parsed.market;
    marketDataQuerySymbol.value = parsed.symbol;
    symbolSearchText.value = parsed.symbol;
  }

  await syncVisibleSubscription();
  await loadMarketDataQuery();
  scheduleMarketDataAutoRefresh();
}

async function loadOlderCandles(): Promise<void> {
  const oldest = chartCandles.value[0];
  if (oldest == null || isLoadingOlderCandles.value) {
    return;
  }

  isLoadingOlderCandles.value = true;
  try {
    await loadMarketDataQuery({
      appendOlder: true,
      toTime: oldest.at,
    });
  } finally {
    isLoadingOlderCandles.value = false;
  }
}

function resolveMarketDataAutoRefreshMs(): number {
  switch (marketDataQueryPeriod.value) {
    case "tick":
      return 1000;
    case "1m":
      return 10_000;
    default:
      return 60_000;
  }
}

function scheduleMarketDataAutoRefresh(): void {
  if (queryRefreshTimer != null) {
    window.clearTimeout(queryRefreshTimer);
    queryRefreshTimer = null;
  }

  queryRefreshTimer = window.setTimeout(() => {
    void (async () => {
      await loadMarketDataQuery({ appendOlder: true });
      scheduleMarketDataAutoRefresh();
    })();
  }, resolveMarketDataAutoRefreshMs());
}
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="行情工作台"
      title="行情 / 数据"
      description="用更接近交易终端的布局收纳订阅、配额、快照和 K 线查询，避免行情状态散落在不同卡片里。"
      :stats="marketHeaderStats"
    />

    <!-- Section Header with Page Title and Description -->
    <div>
      <SectionHeader
        title="行情数据"
        description="配置查询参数并监控活跃订阅。"
      />
    </div>

    <!-- Main Two-Column Layout -->
    <div class="grid grid-cols-12 gap-4">
      <!-- Left Column: Filter Panel -->
      <div class="col-span-4 lg:col-span-3">
        <div class="sticky top-4 rounded-lg border border-slate-200 bg-white p-4">
          <div class="space-y-4">
            <!-- Market Select -->
            <div class="grid gap-1">
              <label class="text-sm font-medium text-slate-700">市场</label>
              <v-select
                v-model="marketDataQueryMarket"
                class="w-full"
                density="compact"
                variant="outlined"
                :items="marketOptions"
                item-title="title"
                item-value="value"
              />
            </div>

            <!-- Symbol Input -->
            <div class="grid gap-1">
              <label class="text-sm font-medium text-slate-700">标的</label>
              <v-autocomplete
                v-model="symbolSearchText"
                class="w-full"
                :items="marketInstrumentSearchOptions.map(o => ({ value: o.lookupValue, title: o.label }))"
                item-title="title"
                item-value="value"
                placeholder="00700"
                clearable
                density="compact"
                variant="outlined"
                @update:model-value="(val: string | null | undefined) => val && selectInstrumentSuggestion({ value: String(val) })"
              />
              <div class="mt-1 text-xs text-slate-500">
                当前标的：<span class="font-medium text-slate-700">{{ selectedInstrumentTitle }}</span>
              </div>
            </div>

            <!-- Period Select -->
            <div class="grid gap-1">
              <label class="text-sm font-medium text-slate-700">周期</label>
              <v-select
                v-model="marketDataQueryPeriod"
                class="w-full"
                density="compact"
                variant="outlined"
                :items="periods"
                item-title="label"
                item-value="value"
              />
            </div>

            <!-- Limit Input -->
            <div class="grid gap-1">
              <label class="text-sm font-medium text-slate-700">查询条数</label>
              <v-text-field
                v-model.number="marketDataQueryLimit"
                type="number"
                :min="1"
                class="w-full"
                density="compact"
                variant="outlined"
              />
            </div>

            <!-- Query Button -->
            <button
              class="w-full rounded-2xl bg-teal-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-teal-700 disabled:cursor-not-allowed disabled:bg-slate-300"
              :disabled="isLoadingMarketDataQuery"
              type="button"
                @click="runMarketDataQuery()"
            >
              {{ isLoadingMarketDataQuery ? '查询中...' : '查询' }}
            </button>

            <!-- Info Note -->
            <div class="rounded-lg border border-teal-100 bg-teal-50 px-3 py-2 text-xs text-teal-900">
              实时订阅已改为动态池：当前查看的 {{ selectedInstrumentTitle }}
              会自动申请订阅。
            </div>
          </div>
        </div>
      </div>

      <!-- Right Column: Primary Content -->
      <div class="col-span-8 lg:col-span-9">
        <div class="grid gap-4">
          <!-- Subscriptions Table -->
          <div class="rounded-lg border border-slate-200 bg-white p-4">
            <div class="mb-4">
              <h3 class="text-base font-semibold text-slate-900">
                行情订阅
              </h3>
              <p class="mt-1 text-sm text-slate-500">
                活跃：{{ marketDataSubscriptions.totalActiveSubscriptions }}
              </p>
            </div>

            <div v-if="isLoadingMarketData" class="py-6 text-center text-sm text-slate-500">
              正在加载市场数据订阅...
            </div>

            <v-alert
              v-else-if="marketDataError"
              class="mb-4"
              type="warning"
              :closable="false"
              title="行情数据提示"
            >{{ marketDataError }}</v-alert>

            <div v-else class="overflow-x-auto">
              <table class="min-w-full text-left text-sm">
                <thead class="text-xs uppercase tracking-[0.18em] text-slate-500">
                  <tr>
                    <th class="whitespace-nowrap px-3 py-2">代码</th>
                    <th class="whitespace-nowrap px-3 py-2">名称</th>
                    <th class="whitespace-nowrap px-3 py-2">市场</th>
                    <th class="whitespace-nowrap px-3 py-2">通道</th>
                    <th class="whitespace-nowrap px-3 py-2">标的ID</th>
                    <th class="whitespace-nowrap px-3 py-2 text-center">引用数</th>
                    <th class="whitespace-nowrap px-3 py-2 text-center">消费者</th>
                    <th class="whitespace-nowrap px-3 py-2">更新时间</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="row in marketDataSubscriptions.entries"
                    :key="row.key"
                    class="border-t border-slate-100"
                  >
                    <td class="whitespace-nowrap px-3 py-2 font-medium text-slate-900">{{ row.symbol }}</td>
                    <td class="px-3 py-2 text-slate-600">{{ marketInstrumentSearchOptions.find(o => o.instrumentId === row.instrumentId)?.name ?? '暂无' }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-slate-600">{{ formatMarketLabel(row.market) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-slate-600">{{ formatMarketDataChannelLabel(row.channel) }}</td>
                    <td class="px-3 py-2 text-slate-600">{{ row.instrumentId }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-center text-slate-600">{{ row.refCount }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-center text-slate-600">{{ row.consumers.length }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-slate-600">{{ formatDateTime(row.updatedAt) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <v-empty-state v-if="!isLoadingMarketData && !marketDataError && !marketDataSubscriptions.entries.length" text="当前没有活跃的市场数据订阅。策略或券商数据源建立订阅后会显示消费者、配额与更新时间。" />
          </div>

          <!-- Secondary Panels in Collapse -->
          <v-expansion-panels v-model="activeResultPanels" variant="accordion" multiple>
            <!-- Quota Section -->
            <v-expansion-panel value="subscription-quota">
              <template #title>
                <div class="flex items-center justify-between gap-3 flex-1 pr-4">
                  <span>订阅配额</span>
                  <v-chip variant="outlined" size="small">{{ marketDataSubscriptions.quota.totalUsed }} / {{ marketDataSubscriptions.quota.totalLimit ?? '∞' }}</v-chip>
                </div>
              </template>

              <div v-if="marketDataSubscriptions.quota.byMarket.length" class="grid gap-3">
                <div
                  v-for="bucket in marketDataSubscriptions.quota.byMarket"
                  :key="bucket.market"
                  class="rounded-lg border border-slate-200 bg-white px-4 py-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="text-base font-semibold text-slate-900">{{ formatMarketLabel(bucket.market) }}</div>
                    <v-chip variant="outlined" size="small">{{ bucket.used }} / {{ bucket.limit ?? '∞' }}</v-chip>
                  </div>
                  <div class="mt-3 rounded-lg bg-slate-50 px-3 py-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">剩余</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900">
                      {{ bucket.remaining ?? '∞' }}
                    </div>
                  </div>
                </div>
              </div>

              <v-empty-state v-else text="暂无各市场配额信息。配额读模型会随订阅注册表接入后自动填充。" />
            </v-expansion-panel>

            <!-- Snapshot and Candles Section -->
            <v-expansion-panel value="query-results">
              <template #title>
                <div class="flex items-center justify-between gap-3 flex-1 pr-4">
                  <span>行情查询结果</span>
                  <v-chip variant="outlined" size="small">{{ formatMarketDataSourceLabel(marketDataSnapshot?.meta.source) }}</v-chip>
                </div>
              </template>

              <v-alert
                v-if="marketDataQueryError"
                type="warning"
                class="mb-4"
                :closable="false"
                title="行情查询提示"
              >{{ marketDataQueryError }}</v-alert>

              <div class="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
                <!-- Snapshot Card -->
                <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">快照</div>
                      <div class="mt-1 text-sm font-medium text-slate-900">
                        {{ selectedInstrumentTitle }}
                      </div>
                    </div>
                    <v-chip :color="marketDataSnapshot?.snapshot ? 'success' : 'info'" variant="outlined" size="small">
                      {{ formatGenericStatusLabel(marketDataSnapshot?.snapshot ? 'FOUND' : 'EMPTY') }}
                    </v-chip>
                  </div>

                  <div v-if="latestQuoteSnapshot" class="mt-4 grid gap-3 sm:grid-cols-4">
                    <div class="rounded-lg bg-slate-50 px-3 py-3 sm:col-span-2">
                      <div class="flex items-center justify-between gap-3">
                        <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最新报价</div>
                        <v-chip variant="outlined" size="small">{{ liveQuoteStateLabel }}</v-chip>
                      </div>
                      <div class="mt-2 flex flex-wrap items-end gap-3">
                        <div class="text-3xl font-semibold" :class="latestQuoteToneClass">
                          {{ latestQuoteSnapshot.price }}
                        </div>
                        <div class="pb-1 text-sm font-semibold" :class="latestQuoteToneClass">
                          {{ latestQuoteChangeLabel }}
                        </div>
                      </div>
                      <div class="mt-2 text-xs text-slate-500">
                        参考价：{{ latestQuoteReferencePrice ?? '暂无' }} · {{ formatDateTime(latestQuoteSnapshot.at) }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">买一 / 卖一</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ latestQuoteSnapshot.bid }} / {{ latestQuoteSnapshot.ask }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">成交量</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">{{ latestQuoteSnapshot.volume }}</div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最高 / 最低</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ latestQuoteSnapshot.highPrice ?? '暂无' }} / {{ latestQuoteSnapshot.lowPrice ?? '暂无' }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">更新时间</div>
                      <div class="mt-2 text-sm font-semibold text-slate-900">
                        {{ formatDateTime(latestQuoteSnapshot.at) }}
                      </div>
                    </div>
                  </div>
                  <v-empty-state v-else text="未命中快照缓存；请确认市场 / 标的，或等待行情数据源写入。" class="mt-4" />
                </div>

                <!-- Candles Card -->
                <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">近期K线</div>
                      <div class="mt-1 text-sm font-medium text-slate-900">
                        {{ selectedInstrumentTitle }} · {{ marketDataCandles?.request.period ?? marketDataQueryPeriod }} / {{ marketDataCandles?.totalReturned ?? 0 }} 条
                      </div>
                    </div>
                  </div>

                  <div class="mt-4 rounded-xl border border-slate-100 bg-slate-50 p-2">
                    <KlineChart
                      :candles="chartCandles"
                      :min-height="260"
                      show-indicator-selector
                      indicator-storage-key="jftrade.market-chart.indicators"
                      :default-indicators="['volume']"
                      empty-text="暂无 K 线图数据；点击查询后会在这里渲染最近 K 线数据。"
                      @load-more="loadOlderCandles"
                    />
                    <div v-if="isLoadingOlderCandles" class="px-2 pt-2 text-xs text-slate-500">
                      正在加载更早的历史 K 线...
                    </div>
                  </div>

                  <div
                    v-if="historicalPriceSummary"
                    class="mt-4 grid gap-3 sm:grid-cols-4 lg:grid-cols-4"
                  >
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">历史区间</div>
                      <div class="mt-2 text-xs font-semibold text-slate-900">
                        {{ formatDateTime(historicalPriceSummary.from) }} → {{ formatDateTime(historicalPriceSummary.to) }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">开盘 → 收盘</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ historicalPriceSummary.firstOpen }} → {{ historicalPriceSummary.lastClose }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">最高 / 最低</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ historicalPriceSummary.high }} / {{ historicalPriceSummary.low }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">涨跌 / 成交量</div>
                      <div
                        class="mt-2 text-lg font-semibold"
                        :class="historicalPriceSummary.change >= 0 ? 'tv-up' : 'tv-down'"
                      >
                        {{ historicalPriceSummary.change.toFixed(3) }}
                        ({{ historicalPriceSummary.changePct.toFixed(2) }}%) · {{ historicalPriceSummary.volume }}
                      </div>
                    </div>
                  </div>

                  <div v-if="marketDataCandles?.candles.length" class="mt-4 overflow-x-auto">
                    <div class="mb-2 text-xs uppercase tracking-[0.18em] text-slate-500">
                      历史K线价格
                    </div>
                    <table class="min-w-full text-left text-sm">
                      <thead class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        <tr>
                          <th class="whitespace-nowrap px-3 py-2">时间</th>
                          <th class="whitespace-nowrap px-3 py-2">开盘</th>
                          <th class="whitespace-nowrap px-3 py-2">最高</th>
                          <th class="whitespace-nowrap px-3 py-2">最低</th>
                          <th class="whitespace-nowrap px-3 py-2">收盘</th>
                          <th class="whitespace-nowrap px-3 py-2">成交量</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr
                          v-for="candle in marketDataCandles.candles"
                          :key="`${candle.period}:${candle.at}`"
                          class="border-t border-slate-100"
                        >
                          <td class="whitespace-nowrap px-3 py-2 text-slate-600">{{ formatDateTime(resolveKlineCandleDisplayAt(candle)) }}</td>
                          <td class="px-3 py-2 font-medium text-slate-900">{{ candle.open }}</td>
                          <td class="px-3 py-2 font-medium text-slate-900">{{ candle.high }}</td>
                          <td class="px-3 py-2 font-medium text-slate-900">{{ candle.low }}</td>
                          <td class="px-3 py-2 font-semibold text-teal-700">{{ candle.close }}</td>
                          <td class="px-3 py-2 text-slate-600">{{ candle.volume }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </v-expansion-panel>
          </v-expansion-panels>
        </div>
      </div>
    </div>
  </div>
</template>
