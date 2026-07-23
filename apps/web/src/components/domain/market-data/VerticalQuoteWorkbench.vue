<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";

import KlineChart from "../../KlineChart.vue";
import DenseMetricStrip from "../shared/DenseMetricStrip.vue";
import WatchlistMembershipDialog from "../watchlist/WatchlistMembershipDialog.vue";
import CompactInstrumentNews from "./CompactInstrumentNews.vue";
import QuoteSummaryCard from "./QuoteSummaryCard.vue";
import {
  type QuoteWorkbenchPeriod,
  type QuoteWorkbenchTab,
  type QuoteWorkbenchTarget,
} from "./quoteWorkbench";
import { useVerticalQuoteWorkbench } from "./useVerticalQuoteWorkbench";

const props = withDefaults(
  defineProps<{
    target?: QuoteWorkbenchTarget | null;
    brokerId?: string;
    visible?: boolean;
    variant?: "rail" | "drawer";
    period?: QuoteWorkbenchPeriod;
    tab?: QuoteWorkbenchTab;
    emptyText?: string;
  }>(),
  {
    target: null,
    brokerId: "",
    visible: true,
    variant: "rail",
    period: "day",
    tab: "quote",
    emptyText: "选择标的后查看行情详情",
  },
);

const emit = defineEmits<{
  "update:period": [period: QuoteWorkbenchPeriod];
  "update:tab": [tab: QuoteWorkbenchTab];
  selectTarget: [target: QuoteWorkbenchTarget];
  openWorkspace: [target: QuoteWorkbenchTarget];
  close: [];
}>();

const {
  PERIOD_OPTIONS,
  selectedPeriod,
  resolvedTarget,
  instrumentParts,
  name,
  lastPrice,
  changeAmount,
  changeRate,
  statusLine,
  sourceLabel,
  metrics,
  security,
  extendedCards,
  candles,
  plateMembers,
  snapshotLoading,
  securityLoading,
  candlesLoading,
  plateMembersLoading,
  snapshotError,
  securityError,
  candlesError,
  plateMembersError,
  watchlistDialogOpen,
  favorite,
  handleWatchlistSaved,
  refresh,
} = useVerticalQuoteWorkbench(props);

const newsPanel = ref<InstanceType<typeof CompactInstrumentNews> | null>(null);
const refreshing = ref(false);
const isPlate = computed(() => resolvedTarget.value?.kind === "plate");
const activeTab = computed<QuoteWorkbenchTab>(() =>
  isPlate.value ? "quote" : props.tab,
);
const newsInstrumentId = computed(() => {
  const target = resolvedTarget.value;
  if (target == null || target.kind === "plate") return "";
  if (target.productClass === "warrant" || target.productClass === "cbbc") {
    return security.value?.warrant?.owner?.instrumentId?.trim().toUpperCase() ?? "";
  }
  return target.instrumentId;
});
const newsPending = computed(
  () =>
    (resolvedTarget.value?.productClass === "warrant" ||
      resolvedTarget.value?.productClass === "cbbc") &&
    securityLoading.value &&
    newsInstrumentId.value === "",
);

function selectPeriod(period: QuoteWorkbenchPeriod): void {
  if (selectedPeriod.value !== period) selectedPeriod.value = period;
  emit("update:period", period);
}

function selectTab(tab: QuoteWorkbenchTab): void {
  if (isPlate.value && tab === "news") return;
  emit("update:tab", tab);
}

async function handleRefresh(): Promise<void> {
  if (refreshing.value) return;
  refreshing.value = true;
  try {
    await refresh();
    if (activeTab.value === "news") {
      await nextTick();
      await newsPanel.value?.refresh();
    }
  } finally {
    refreshing.value = false;
  }
}

watch(
  () => props.period,
  (period) => {
    if (selectedPeriod.value !== period) selectedPeriod.value = period;
  },
  { immediate: true },
);
</script>

<template>
  <aside
    class="vertical-quote-workbench quote-detail-rail"
    :class="`vertical-quote-workbench--${variant}`"
    :aria-busy="snapshotLoading || securityLoading || refreshing"
  >
    <div v-if="resolvedTarget == null" class="quote-detail-rail__placeholder">
      {{ emptyText }}
    </div>

    <template v-else>
      <div class="vertical-quote-workbench__toolbar">
        <span>竖屏行情</span>
        <div>
          <button
            type="button"
            :disabled="refreshing"
            title="刷新当前标的"
            aria-label="刷新当前标的"
            @click="handleRefresh"
          >
            <span aria-hidden="true">↻</span>
          </button>
          <button
            v-if="!isPlate"
            type="button"
            class="vertical-quote-workbench__open is-outlined"
            title="在完整工作台中打开"
            @click="emit('openWorkspace', resolvedTarget)"
          >
            打开工作台
          </button>
          <button
            v-if="variant === 'drawer'"
            type="button"
            title="关闭行情详情"
            aria-label="关闭行情详情"
            @click="emit('close')"
          >
            ×
          </button>
        </div>
      </div>

      <QuoteSummaryCard
        v-if="instrumentParts != null"
        class="quote-detail-rail__summary"
        :market="instrumentParts.market"
        :code="instrumentParts.symbol"
        :instrument-id="resolvedTarget.instrumentId"
        :name="name"
        price-label="最新价"
        :price="lastPrice"
        :change-amount="changeAmount"
        :change-rate="changeRate"
        :show-change-amount="true"
        :status-text="
          statusLine || (snapshotLoading ? '行情加载中…' : '行情时间未知')
        "
        :source-text="sourceLabel"
        :loading="snapshotLoading"
        :favorite-visible="!isPlate"
        :favorite-active="favorite"
        favorite-test-id="quote-detail-rail-favorite"
        :extended-cards="extendedCards"
        @favorite="watchlistDialogOpen = true"
      />

      <div class="quote-detail-rail__metrics">
        <DenseMetricStrip :items="metrics" min-item-width="86px" />
      </div>

      <div
        v-if="snapshotError || securityError"
        class="quote-detail-rail__notice tv-status--warning"
      >
        {{ [snapshotError, securityError].filter(Boolean).join(" / ") }}
      </div>

      <nav
        v-if="!isPlate"
        class="vertical-quote-workbench__tabs"
        role="tablist"
        aria-label="竖屏行情内容"
      >
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'quote'"
          :class="{ 'is-active': activeTab === 'quote' }"
          @click="selectTab('quote')"
        >
          行情
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'news'"
          :class="{ 'is-active': activeTab === 'news' }"
          @click="selectTab('news')"
        >
          资讯
        </button>
      </nav>

      <section
        v-if="activeTab === 'quote' && !isPlate"
        class="quote-detail-rail__chart-section"
      >
        <div
          class="quote-detail-rail__chart-toolbar"
          role="tablist"
          aria-label="K 线周期"
        >
          <button
            v-for="option in PERIOD_OPTIONS"
            :key="option.value"
            type="button"
            role="tab"
            :aria-selected="selectedPeriod === option.value"
            :class="{ 'is-active': selectedPeriod === option.value }"
            @click="selectPeriod(option.value)"
          >
            {{ option.label }}
          </button>
        </div>
        <div class="quote-detail-rail__chart">
          <div
            v-if="candlesError && candles.length === 0"
            class="quote-detail-rail__chart-placeholder"
          >
            {{ candlesError }}
          </div>
          <div
            v-else-if="candlesLoading && candles.length === 0"
            class="quote-detail-rail__chart-placeholder"
          >
            K 线加载中…
          </div>
          <template v-else>
            <div
              v-if="candlesError"
              class="quote-detail-rail__chart-warning tv-status--warning"
            >
              K 线刷新失败：{{ candlesError }}
            </div>
            <KlineChart
              :candles="candles"
              :min-height="320"
              empty-text="暂无历史 K 线"
            />
          </template>
        </div>
      </section>

      <CompactInstrumentNews
        v-if="!isPlate"
        v-show="activeTab === 'news'"
        ref="newsPanel"
        :target="resolvedTarget"
        :query-instrument-id="newsInstrumentId"
        :broker-id="brokerId"
        :active="visible && activeTab === 'news'"
        :pending="newsPending"
        @select-target="emit('selectTarget', $event)"
      />

      <section v-if="isPlate" class="quote-detail-rail__members">
        <header>
          <strong>成分股</strong>
          <span v-if="plateMembers.length">前 {{ plateMembers.length }} 只</span>
        </header>
        <div v-if="plateMembersLoading" class="quote-detail-rail__members-state">
          成分股加载中…
        </div>
        <div
          v-else-if="plateMembersError"
          class="quote-detail-rail__members-state"
        >
          {{ plateMembersError }}
        </div>
        <div
          v-else-if="plateMembers.length === 0"
          class="quote-detail-rail__members-state"
        >
          暂无成分股
        </div>
        <button
          v-for="member in plateMembers"
          v-else
          :key="member.instrumentId"
          type="button"
          class="quote-detail-rail__member"
          @click="emit('selectTarget', member)"
        >
          <span>{{ member.name || member.instrumentId }}</span>
          <small>{{ member.instrumentId }}</small>
        </button>
      </section>

      <WatchlistMembershipDialog
        v-if="watchlistDialogOpen && instrumentParts != null && !isPlate"
        v-model="watchlistDialogOpen"
        :market="instrumentParts.market"
        :symbol="instrumentParts.symbol"
        :name="name"
        @saved="handleWatchlistSaved"
      />
    </template>
  </aside>
</template>

<style scoped>
.vertical-quote-workbench {
  display: flex;
  width: 100%;
  max-width: none;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow-y: auto;
  border-left: 1px solid var(--tv-border);
  background: var(--tv-bg-app);
  color: var(--tv-text);
  container: quote-workbench / inline-size;
  font-size: 12px;
}

.vertical-quote-workbench--drawer {
  box-shadow: var(--tv-shadow-lg, 0 18px 40px rgb(0 0 0 / 32%));
}

.vertical-quote-workbench__toolbar {
  display: flex;
  min-height: 38px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 12px;
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text-dim);
  font-size: 10px;
}

.vertical-quote-workbench__toolbar > div {
  display: flex;
  align-items: center;
  gap: 2px;
}

.vertical-quote-workbench__toolbar button,
.quote-detail-rail__chart-toolbar button,
.vertical-quote-workbench__tabs button {
  border: 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.vertical-quote-workbench__toolbar button {
  display: grid;
  min-width: 28px;
  height: 28px;
  place-items: center;
  padding: 0 7px;
  border-radius: 3px;
  font-size: 14px;
}

.vertical-quote-workbench__toolbar button:hover,
.vertical-quote-workbench__toolbar button:focus-visible {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.vertical-quote-workbench__toolbar button:disabled {
  cursor: wait;
  opacity: 0.5;
}

.vertical-quote-workbench__toolbar .vertical-quote-workbench__open {
  display: block;
  border: 1px solid var(--tv-border);
  background: transparent;
  font-size: 10px;
  transition:
    border-color 120ms ease,
    background-color 120ms ease,
    color 120ms ease,
    box-shadow 120ms ease;
}

.vertical-quote-workbench__toolbar .vertical-quote-workbench__open:hover,
.vertical-quote-workbench__toolbar .vertical-quote-workbench__open:focus-visible {
  border-color: var(--tv-accent);
  background: color-mix(in srgb, var(--tv-accent) 10%, transparent);
  color: var(--tv-accent);
}

.vertical-quote-workbench__toolbar .vertical-quote-workbench__open:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--tv-accent) 28%, transparent);
}

.quote-detail-rail__placeholder,
.quote-detail-rail__chart-placeholder,
.quote-detail-rail__members-state {
  display: grid;
  min-height: 120px;
  place-items: center;
  padding: 16px;
  color: var(--tv-text-dim);
  text-align: center;
}

.quote-detail-rail__placeholder { flex: 1; }

.quote-detail-rail__summary {
  flex: 0 0 auto;
  padding: 16px 18px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.quote-detail-rail__chart-toolbar,
.quote-detail-rail__members header,
.quote-detail-rail__member {
  display: flex;
  align-items: center;
}

.quote-detail-rail__members header,
.quote-detail-rail__member {
  justify-content: space-between;
}

.quote-detail-rail__metrics {
  flex: 0 0 auto;
  padding: 8px 14px 12px;
  border-bottom: 1px solid var(--tv-border);
}

.quote-detail-rail__notice {
  flex: 0 0 auto;
  padding: 8px 18px;
  border-bottom: 1px solid var(--tv-border);
  font-size: 11px;
}

.vertical-quote-workbench__tabs {
  display: flex;
  min-height: 43px;
  flex: 0 0 auto;
  align-items: stretch;
  gap: 14px;
  padding: 0 18px;
  border-bottom: 1px solid var(--tv-border);
}

.vertical-quote-workbench__tabs button {
  position: relative;
  padding: 0 3px;
  font-size: 12px;
}

.vertical-quote-workbench__tabs button:hover,
.vertical-quote-workbench__tabs button.is-active {
  color: var(--tv-text);
}

.vertical-quote-workbench__tabs button.is-active::after {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 3px;
  background: var(--tv-accent);
  content: "";
}

.quote-detail-rail__chart-section {
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tv-border);
}

.quote-detail-rail__chart-toolbar { gap: 4px; padding: 10px 14px 4px; }

.quote-detail-rail__chart-toolbar button {
  min-width: 42px;
  padding: 5px 8px;
  border-radius: 4px;
  font-size: 11px;
}

.quote-detail-rail__chart-toolbar button:hover,
.quote-detail-rail__chart-toolbar button.is-active {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.quote-detail-rail__chart-toolbar button.is-active {
  box-shadow: inset 0 -2px var(--tv-accent);
}

.quote-detail-rail__chart { position: relative; min-height: 320px; padding: 4px 8px 10px; }
.quote-detail-rail__chart-placeholder { min-height: 320px; }

.quote-detail-rail__chart-warning {
  margin: 4px 6px;
  padding: 5px 7px;
  font-size: 10px;
}
.quote-detail-rail__members { flex: 0 0 auto; padding: 12px 18px 18px; }
.quote-detail-rail__members header { height: 30px; color: var(--tv-text); }

.quote-detail-rail__members header span,
.quote-detail-rail__member small { color: var(--tv-text-dim); }

.quote-detail-rail__members-state { min-height: 72px; }

.quote-detail-rail__member {
  width: 100%;
  gap: 8px;
  padding: 8px 0;
  border: 0;
  border-top: 1px solid var(--tv-border);
  background: transparent;
  color: var(--tv-text);
  text-align: left;
  cursor: pointer;
}

.quote-detail-rail__member:hover { color: var(--tv-accent); }

@container quote-workbench (max-width: 399px) {
  .quote-detail-rail__metrics :deep(.dense-metric-strip) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
