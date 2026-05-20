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
import { useConsoleData } from "../composables/useConsoleData";
import { useSharedLiveSocket } from "../composables/useSharedLiveSocket";

const {
  formatDateTime,
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

const marketPageConsumerId = createStableWebConsumerId("market-page");
const heldVisibleSubscription = ref<{ market: string; symbol: string } | null>(
  null,
);
const symbolSearchText = ref(marketDataQuerySymbol.value);
const activeResultPanels = ref(["query-results"]);
const isLoadingOlderCandles = ref(false);
let heartbeatTimer: number | null = null;
let queryRefreshTimer: number | null = null;

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
  const next = currentVisibleSubscription();
  const previous = heldVisibleSubscription.value;

  if (
    previous != null &&
    (previous.market !== next.market || previous.symbol !== next.symbol)
  ) {
    await Promise.all([
      releaseMarketDataSubscription({
        consumerId: marketPageConsumerId,
        market: previous.market,
        symbol: previous.symbol,
        channel: "SNAPSHOT",
      }),
      releaseMarketDataSubscription({
        consumerId: marketPageConsumerId,
        market: previous.market,
        symbol: previous.symbol,
        channel: "TICK",
      }),
    ]);
    heldVisibleSubscription.value = null;
  }

  if (next.market === "" || next.symbol === "") {
    return;
  }

  await Promise.all([
    acquireMarketDataSubscription({
      consumerId: marketPageConsumerId,
      market: next.market,
      symbol: next.symbol,
      channel: "SNAPSHOT",
    }),
    acquireMarketDataSubscription({
      consumerId: marketPageConsumerId,
      market: next.market,
      symbol: next.symbol,
      channel: "TICK",
    }),
  ]);
  await heartbeatMarketDataConsumer(marketPageConsumerId);
  heldVisibleSubscription.value = next;
}

onMounted(() => {
  void (async () => {
    symbolSearchText.value = marketDataQuerySymbol.value;
    await loadMarketInstrumentReferences();
    await loadMarketDataSubscriptions();
    await syncVisibleSubscription();
    await loadMarketDataQuery();
    scheduleMarketDataAutoRefresh();
    heartbeatTimer = window.setInterval(() => {
      void heartbeatMarketDataConsumer(marketPageConsumerId);
    }, 15_000);
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
  if (heartbeatTimer != null) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
  if (queryRefreshTimer != null) {
    window.clearTimeout(queryRefreshTimer);
    queryRefreshTimer = null;
  }
  const previous = heldVisibleSubscription.value;
  if (previous == null) {
    return;
  }

  void releaseMarketDataSubscription({
    consumerId: marketPageConsumerId,
    market: previous.market,
    symbol: previous.symbol,
    channel: "SNAPSHOT",
    keepalive: true,
  });
  void releaseMarketDataSubscription({
    consumerId: marketPageConsumerId,
    market: previous.market,
    symbol: previous.symbol,
    channel: "TICK",
    keepalive: true,
  });
});

const marketHeaderStats = computed(() => [
  {
    label: "Active Subs",
    value: marketDataSubscriptions.value.totalActiveSubscriptions,
  },
  {
    label: "Quota",
    value: `${marketDataSubscriptions.value.quota.totalUsed} / ${marketDataSubscriptions.value.quota.totalLimit ?? "∞"}`,
  },
  {
    label: "Snapshot",
    value: marketDataSnapshot.value?.snapshot ? "READY" : "EMPTY",
    tone: marketDataSnapshot.value?.snapshot ? "good" : "warn",
  },
  {
    label: "Candles",
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

  return change > 0 ? "text-emerald-600" : "text-red-600";
});
const latestQuoteChangeLabel = computed(() => {
  const change = latestQuoteChange.value;
  const percent = latestQuoteChangePercent.value;
  if (change == null || percent == null) {
    return "N/A";
  }

  const prefix = change > 0 ? "+" : "";
  return `${prefix}${change.toFixed(3)} (${prefix}${percent.toFixed(2)}%)`;
});
const liveQuoteStateLabel = computed(() => {
  switch (live.connectionState.value) {
    case "connected":
      return "WS LIVE";
    case "connecting":
      return "WS CONNECTING";
    case "unsupported":
      return "WS UNSUPPORTED";
    case "error":
      return "WS ERROR";
    case "disconnected":
      return "WS DISCONNECTED";
    default:
      return "WS IDLE";
  }
});

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
      eyebrow="Market workstation"
      title="Market / Data"
      description="用更接近交易终端的布局收纳订阅、配额、快照和 K 线查询，避免行情状态散落在不同卡片里。"
      :stats="marketHeaderStats"
    />

    <!-- Section Header with Page Title and Description -->
    <div>
      <SectionHeader
        title="Market Data"
        description="Configure query parameters and monitor active subscriptions"
      />
    </div>

    <!-- Main Two-Column Layout -->
    <div class="grid grid-cols-12 gap-4">
      <!-- Left Column: Filter Panel -->
      <div class="col-span-4 lg:col-span-3">
        <div class="sticky top-4 rounded-lg border border-slate-200 bg-white p-4">
          <el-form label-position="top" class="space-y-4">
            <!-- Market Select -->
            <el-form-item label="市场 (Market)">
              <el-select v-model="marketDataQueryMarket" class="w-full">
                <el-option value="HK" label="HK" />
                <el-option value="US" label="US" />
                <el-option value="CN" label="CN" />
                <el-option value="SG" label="SG" />
                <el-option value="JP" label="JP" />
                <el-option value="AU" label="AU" />
                <el-option value="MY" label="MY" />
                <el-option value="CA" label="CA" />
                <el-option value="CRYPTO" label="CRYPTO" />
              </el-select>
            </el-form-item>

            <!-- Symbol Input -->
            <el-form-item label="标的 (Symbol)">
              <el-autocomplete
                v-model="symbolSearchText"
                class="w-full"
                :fetch-suggestions="queryInstrumentSuggestions"
                placeholder="00700"
                value-key="value"
                clearable
                @select="selectInstrumentSuggestion"
              >
                <template #default="{ item }">
                  <div class="font-medium text-slate-900">{{ item.value }}</div>
                  <div class="text-xs text-slate-500">{{ item.label }}</div>
                </template>
              </el-autocomplete>
              <div class="mt-2 text-xs text-slate-500">
                当前标的：<span class="font-medium text-slate-700">{{ selectedInstrumentTitle }}</span>
              </div>
            </el-form-item>

            <!-- Period Select -->
            <el-form-item label="周期 (Period)">
              <el-select v-model="marketDataQueryPeriod" class="w-full">
                <el-option
                  v-for="period in periods"
                  :key="period.value"
                  :value="period.value"
                  :label="period.label"
                />
              </el-select>
            </el-form-item>

            <!-- Limit Input -->
            <el-form-item label="Limit">
              <el-input-number
                v-model="marketDataQueryLimit"
                :min="1"
                class="w-full"
              />
            </el-form-item>

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
          </el-form>
        </div>
      </div>

      <!-- Right Column: Primary Content -->
      <div class="col-span-8 lg:col-span-9">
        <div class="grid gap-4">
          <!-- Subscriptions Table -->
          <div class="rounded-lg border border-slate-200 bg-white p-4">
            <div class="mb-4">
              <h3 class="text-base font-semibold text-slate-900">
                Market Data Subscriptions
              </h3>
              <p class="mt-1 text-sm text-slate-500">
                Active: {{ marketDataSubscriptions.totalActiveSubscriptions }}
              </p>
            </div>

            <div v-if="isLoadingMarketData" class="py-6 text-center text-sm text-slate-500">
              正在加载市场数据订阅...
            </div>

            <el-alert
              v-else-if="marketDataError"
              class="mb-4"
              type="warning"
              :closable="false"
              show-icon
              title="Market Data Warning"
            >
              <template #default>{{ marketDataError }}</template>
            </el-alert>

            <el-table
              v-else
              :data="marketDataSubscriptions.entries"
              class="w-full"
              stripe
            >
              <el-table-column prop="symbol" label="Symbol" width="100" />
              <el-table-column label="Name" min-width="160">
                <template #default="{ row }">
                  {{
                    marketInstrumentSearchOptions.find((option) => option.instrumentId === row.instrumentId)?.name ?? 'N/A'
                  }}
                </template>
              </el-table-column>
              <el-table-column prop="market" label="Market" width="80" />
              <el-table-column prop="channel" label="Channel" width="100" />
              <el-table-column prop="instrumentId" label="Instrument ID" min-width="150" />
              <el-table-column prop="refCount" label="Ref Count" width="80" align="center" />
              <el-table-column label="Consumers" width="100" align="center">
                <template #default="{ row }">
                  {{ row.consumers.length }}
                </template>
              </el-table-column>
              <el-table-column label="Updated" min-width="180">
                <template #default="{ row }">
                  {{ formatDateTime(row.updatedAt) }}
                </template>
              </el-table-column>
            </el-table>

            <el-empty v-if="!isLoadingMarketData && !marketDataError && !marketDataSubscriptions.entries.length" description="当前没有活跃的市场数据订阅。策略或 broker provider 建立订阅后会显示消费者、配额与更新时间。" :image-size="80" />
          </div>

          <!-- Secondary Panels in Collapse -->
          <el-collapse v-model="activeResultPanels">
            <!-- Quota Section -->
            <el-collapse-item title="Subscription Quota" name="subscription-quota">
              <template #title>
                <div class="flex items-center justify-between gap-3 flex-1 pr-4">
                  <span>Subscription Quota</span>
                  <el-tag effect="plain">{{ marketDataSubscriptions.quota.totalUsed }} / {{ marketDataSubscriptions.quota.totalLimit ?? '∞' }}</el-tag>
                </div>
              </template>

              <div v-if="marketDataSubscriptions.quota.byMarket.length" class="grid gap-3">
                <div
                  v-for="bucket in marketDataSubscriptions.quota.byMarket"
                  :key="bucket.market"
                  class="rounded-lg border border-slate-200 bg-white px-4 py-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div class="text-base font-semibold text-slate-900">{{ bucket.market }}</div>
                    <el-tag effect="plain">{{ bucket.used }} / {{ bucket.limit ?? '∞' }}</el-tag>
                  </div>
                  <div class="mt-3 rounded-lg bg-slate-50 px-3 py-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Remaining</div>
                    <div class="mt-2 text-xl font-semibold text-slate-900">
                      {{ bucket.remaining ?? '∞' }}
                    </div>
                  </div>
                </div>
              </div>

              <el-empty v-else description="暂无各市场配额信息。配额 read-model 会随订阅 registry 接入后自动填充。" :image-size="80" />
            </el-collapse-item>

            <!-- Snapshot and Candles Section -->
            <el-collapse-item title="Market Data Query Results" name="query-results">
              <template #title>
                <div class="flex items-center justify-between gap-3 flex-1 pr-4">
                  <span>Market Data Query Results</span>
                  <el-tag effect="plain">{{ marketDataSnapshot?.meta.source ?? 'cache' }}</el-tag>
                </div>
              </template>

              <el-alert
                v-if="marketDataQueryError"
                type="warning"
                class="mb-4"
                :closable="false"
                show-icon
                title="Market Data Query Warning"
              >
                <template #default>{{ marketDataQueryError }}</template>
              </el-alert>

              <div class="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
                <!-- Snapshot Card -->
                <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Snapshot</div>
                      <div class="mt-1 text-sm font-medium text-slate-900">
                        {{ selectedInstrumentTitle }}
                      </div>
                    </div>
                    <el-tag :type="marketDataSnapshot?.snapshot ? 'success' : 'info'" effect="plain">
                      {{ marketDataSnapshot?.snapshot ? 'FOUND' : 'EMPTY' }}
                    </el-tag>
                  </div>

                  <div v-if="latestQuoteSnapshot" class="mt-4 grid gap-3 sm:grid-cols-2">
                    <div class="rounded-lg bg-slate-50 px-3 py-3 sm:col-span-2">
                      <div class="flex items-center justify-between gap-3">
                        <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Latest Quote</div>
                        <el-tag effect="plain" size="small">{{ liveQuoteStateLabel }}</el-tag>
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
                        参考价：{{ latestQuoteReferencePrice ?? 'N/A' }} · {{ formatDateTime(latestQuoteSnapshot.at) }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Bid / Ask</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ latestQuoteSnapshot.bid }} / {{ latestQuoteSnapshot.ask }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Volume</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">{{ latestQuoteSnapshot.volume }}</div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">High / Low</div>
                      <div class="mt-2 text-xl font-semibold text-slate-900">
                        {{ latestQuoteSnapshot.highPrice ?? 'N/A' }} / {{ latestQuoteSnapshot.lowPrice ?? 'N/A' }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Updated</div>
                      <div class="mt-2 text-sm font-semibold text-slate-900">
                        {{ formatDateTime(latestQuoteSnapshot.at) }}
                      </div>
                    </div>
                  </div>
                  <el-empty v-else description="未命中快照缓存；请确认 market / symbol 或等待行情 provider 写入。" :image-size="80" class="mt-4" />
                </div>

                <!-- Candles Card -->
                <div class="rounded-lg border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Recent Candles</div>
                      <div class="mt-1 text-sm font-medium text-slate-900">
                        {{ selectedInstrumentTitle }} · {{ marketDataCandles?.request.period ?? marketDataQueryPeriod }} / {{ marketDataCandles?.totalReturned ?? 0 }} returned
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
                      empty-text="暂无 K 线图数据；点击查询后会在这里渲染最近 candles。"
                      @load-more="loadOlderCandles"
                    />
                    <div v-if="isLoadingOlderCandles" class="px-2 pt-2 text-xs text-slate-500">
                      正在加载更早的历史 K 线...
                    </div>
                  </div>

                  <div
                    v-if="historicalPriceSummary"
                    class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
                  >
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">History Range</div>
                      <div class="mt-2 text-xs font-semibold text-slate-900">
                        {{ formatDateTime(historicalPriceSummary.from) }} → {{ formatDateTime(historicalPriceSummary.to) }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Open → Close</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ historicalPriceSummary.firstOpen }} → {{ historicalPriceSummary.lastClose }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">High / Low</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">
                        {{ historicalPriceSummary.high }} / {{ historicalPriceSummary.low }}
                      </div>
                    </div>
                    <div class="rounded-lg bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Change / Volume</div>
                      <div
                        class="mt-2 text-lg font-semibold"
                        :class="historicalPriceSummary.change >= 0 ? 'text-teal-700' : 'text-rose-700'"
                      >
                        {{ historicalPriceSummary.change.toFixed(3) }}
                        ({{ historicalPriceSummary.changePct.toFixed(2) }}%) · {{ historicalPriceSummary.volume }}
                      </div>
                    </div>
                  </div>

                  <div v-if="marketDataCandles?.candles.length" class="mt-4 overflow-x-auto">
                    <div class="mb-2 text-xs uppercase tracking-[0.18em] text-slate-500">
                      Historical K-line Prices
                    </div>
                    <table class="min-w-full text-left text-sm">
                      <thead class="text-xs uppercase tracking-[0.18em] text-slate-500">
                        <tr>
                          <th class="whitespace-nowrap px-3 py-2">Time</th>
                          <th class="whitespace-nowrap px-3 py-2">Open</th>
                          <th class="whitespace-nowrap px-3 py-2">High</th>
                          <th class="whitespace-nowrap px-3 py-2">Low</th>
                          <th class="whitespace-nowrap px-3 py-2">Close</th>
                          <th class="whitespace-nowrap px-3 py-2">Volume</th>
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
            </el-collapse-item>
          </el-collapse>
        </div>
      </div>
    </div>
  </div>
</template>
