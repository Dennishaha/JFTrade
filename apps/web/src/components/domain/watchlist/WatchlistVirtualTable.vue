<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

import type {
  WatchlistItem,
  WatchlistQuote,
  WatchlistQuoteError,
} from "@/contracts";

import { formatDateTime } from "../../../composables/consoleDataFormatting";
import { formatUserMarketLabel } from "../../../composables/instrumentPresentation";
import { pricePrecisionForMarket } from "../../../composables/marketProfiles";
import { formatMarketSessionLabel } from "../../../composables/marketSessionDisplay";
import { formatMarketPrice, formatPercent } from "../../../utils/numberFormat";
import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";

const props = withDefaults(
  defineProps<{
    items: WatchlistItem[];
    quotes?: Map<string, WatchlistQuote>;
    quoteErrors?: Map<string, WatchlistQuoteError>;
    loading?: boolean;
    loadingMore?: boolean;
    hasMore?: boolean;
    compact?: boolean;
    emptyText?: string;
    activeInstrumentId?: string;
  }>(),
  {
    quotes: () => new Map<string, WatchlistQuote>(),
    quoteErrors: () => new Map<string, WatchlistQuoteError>(),
    loading: false,
    loadingMore: false,
    hasMore: false,
    compact: false,
    emptyText: "暂无自选标的",
    activeInstrumentId: "",
  },
);

const emit = defineEmits<{
  select: [item: WatchlistItem];
  "edit-membership": [item: WatchlistItem];
  "visible-instrument-ids": [instrumentIds: string[]];
  "end-reached": [];
}>();

const viewportRef = ref<HTMLElement | null>(null);
const scrollTop = ref(0);
const scrollLeft = ref(0);
const viewportHeight = ref(400);
const rowHeight = computed(() => (props.compact ? 46 : 52));
const overscan = computed(() => (props.compact ? 5 : 8));
const startIndex = computed(() =>
  Math.max(0, Math.floor(scrollTop.value / rowHeight.value) - overscan.value),
);
const endIndex = computed(() =>
  Math.min(
    props.items.length,
    Math.ceil((scrollTop.value + viewportHeight.value) / rowHeight.value) +
      overscan.value,
  ),
);
const visibleRows = computed(() =>
  props.items.slice(startIndex.value, endIndex.value).map((item, offset) => ({
    item,
    index: startIndex.value + offset,
  })),
);
const contentHeight = computed(() => props.items.length * rowHeight.value);
let resizeObserver: ResizeObserver | null = null;

function syncViewport(): void {
  const viewport = viewportRef.value;
  if (viewport == null) return;
  scrollTop.value = viewport.scrollTop;
  scrollLeft.value = viewport.scrollLeft;
  viewportHeight.value = Math.max(1, viewport.clientHeight);
}

function handleScroll(): void {
  syncViewport();
}

function quoteFor(item: WatchlistItem): WatchlistQuote | undefined {
  return props.quotes.get(item.instrumentId.toUpperCase());
}

function itemName(item: WatchlistItem): string {
  return item.name || quoteFor(item)?.name || "未命名标的";
}

function itemType(item: WatchlistItem): string {
  return item.securityType || quoteFor(item)?.securityType || "—";
}

function quoteErrorFor(item: WatchlistItem): WatchlistQuoteError | undefined {
  return props.quoteErrors.get(item.instrumentId.toUpperCase());
}

function formatPrice(value: number | undefined, market?: string): string {
  return formatMarketPrice(value, {
    market: market ?? null,
    precision: pricePrecisionForMarket(market),
  });
}

function quoteChange(quote: WatchlistQuote | undefined): number | undefined {
  if (quote?.change != null) return quote.change;
  if (quote?.price == null || quote.previousClose == null) return undefined;
  return quote.price - quote.previousClose;
}

function quoteChangePercent(
  quote: WatchlistQuote | undefined,
): number | undefined {
  if (quote?.changePercent != null) return quote.changePercent;
  if (
    quote?.price == null ||
    quote.previousClose == null ||
    quote.previousClose === 0
  ) {
    return undefined;
  }
  return ((quote.price - quote.previousClose) / quote.previousClose) * 100;
}

function formatChange(quote: WatchlistQuote | undefined, market?: string): string {
  const change = quoteChange(quote);
  const percent = quoteChangePercent(quote);
  if (change == null && percent == null) return "—";
  const sign = (percent ?? change ?? 0) > 0 ? "+" : "";
  if (props.compact) {
    return percent == null
      ? `${sign}${formatPrice(change, market)}`
      : formatPercent(percent, { showPositiveSign: true });
  }
  const changeLabel = change == null ? "" : `${sign}${formatPrice(change, market)}`;
  const percentLabel = percent == null
    ? ""
    : formatPercent(percent, { showPositiveSign: true });
  return [changeLabel, percentLabel].filter(Boolean).join("  ");
}

function changeClass(quote: WatchlistQuote | undefined): string {
  const value = quoteChangePercent(quote) ?? quoteChange(quote) ?? 0;
  return value > 0 ? "is-up" : value < 0 ? "is-down" : "";
}

function sourceLabel(item: WatchlistItem): string {
  const sources = item.sources ?? [];
  if (sources.length === 0) return "JFTrade";
  const names = sources.map((source) => source.sourceName || source.sourceId);
  return Array.from(new Set(names)).join("、");
}

function timeLabel(quote: WatchlistQuote | undefined): string {
  const value = quote?.updateTime || quote?.observedAt;
  return value ? formatDateTime(value) : "—";
}

watch(
  [visibleRows, () => props.hasMore, () => props.loadingMore],
  () => {
    emit(
      "visible-instrument-ids",
      visibleRows.value.map(({ item }) => item.instrumentId),
    );
    if (
      props.hasMore &&
      !props.loadingMore &&
      endIndex.value >= props.items.length - overscan.value
    ) {
      emit("end-reached");
    }
  },
  { immediate: true },
);

watch(
  () => props.items.length,
  async () => {
    await nextTick();
    syncViewport();
  },
);

onMounted(() => {
  syncViewport();
  if (typeof ResizeObserver !== "undefined" && viewportRef.value != null) {
    resizeObserver = new ResizeObserver(syncViewport);
    resizeObserver.observe(viewportRef.value);
  } else if (typeof window !== "undefined") {
    window.addEventListener("resize", syncViewport);
  }
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
  if (typeof window !== "undefined") {
    window.removeEventListener("resize", syncViewport);
  }
});
</script>

<template>
  <div
    class="watchlist-table"
    :class="{ 'watchlist-table--compact': compact }"
    role="grid"
    :aria-rowcount="items.length + 1"
    :aria-busy="loading"
  >
    <div class="watchlist-table__header-viewport">
      <div
        class="watchlist-table__header"
        role="row"
        :style="{ transform: `translateX(${-scrollLeft}px)` }"
      >
        <span role="columnheader">名称 / 代码</span>
        <span v-if="!compact" role="columnheader">市场 / 类型</span>
        <span role="columnheader" class="is-numeric">价格</span>
        <span role="columnheader" class="is-numeric">涨跌</span>
        <span v-if="!compact" role="columnheader">时段</span>
        <span v-if="!compact" role="columnheader">数据时间</span>
        <span v-if="!compact" role="columnheader">来源</span>
        <span role="columnheader" class="is-action">操作</span>
      </div>
    </div>

    <div
      ref="viewportRef"
      class="watchlist-table__viewport"
      data-testid="watchlist-virtual-viewport"
      @scroll.passive="handleScroll"
    >
      <div
        v-if="items.length > 0"
        class="watchlist-table__spacer"
        :style="{ height: `${contentHeight}px` }"
      >
        <div
          v-for="row in visibleRows"
          :key="row.item.instrumentId"
          class="watchlist-table__row"
          role="row"
          tabindex="0"
          :aria-label="`打开 ${itemName(row.item)}`"
          :class="{
            'is-active': row.item.instrumentId === activeInstrumentId,
          }"
          :style="{
            height: `${rowHeight}px`,
            transform: `translateY(${row.index * rowHeight}px)`,
          }"
          @click="emit('select', row.item)"
          @keydown.enter.prevent="emit('select', row.item)"
          @keydown.space.prevent="emit('select', row.item)"
        >
          <span class="watchlist-table__instrument" role="gridcell">
            <InstrumentIdentity
              :market="row.item.market"
              :code="row.item.symbol"
              :instrument-id="row.item.instrumentId"
              :name="itemName(row.item)"
              :compact="compact"
              layout="stacked"
            />
          </span>
          <span v-if="!compact" class="watchlist-table__muted" role="gridcell">
            {{ formatUserMarketLabel(row.item.market) }} · {{ itemType(row.item) }}
          </span>
          <span class="watchlist-table__price is-numeric" role="gridcell">
            <template v-if="quoteFor(row.item)">
              {{ formatPrice(quoteFor(row.item)?.price, row.item.market) }}
              <span
                v-if="quoteErrorFor(row.item)"
                class="watchlist-table__quote-stale"
                :title="`数据暂未更新：${quoteErrorFor(row.item)?.message ?? ''}`"
                aria-label="数据暂未更新"
              >!</span>
            </template>
            <span
              v-else-if="quoteErrorFor(row.item)"
              class="watchlist-table__quote-error"
              :title="quoteErrorFor(row.item)?.message"
            >不可用</span>
            <template v-else>—</template>
          </span>
          <span class="is-numeric" role="gridcell" :class="changeClass(quoteFor(row.item))">
            {{ formatChange(quoteFor(row.item), row.item.market) }}
          </span>
          <span v-if="!compact" class="watchlist-table__session" role="gridcell">
            {{ formatMarketSessionLabel(quoteFor(row.item)?.session) || "—" }}
          </span>
          <span v-if="!compact" class="watchlist-table__muted watchlist-table__time" role="gridcell">
            {{ timeLabel(quoteFor(row.item)) }}
          </span>
          <span v-if="!compact" class="watchlist-table__muted watchlist-table__source" role="gridcell">
            {{ sourceLabel(row.item) }}
          </span>
          <span class="is-action" role="gridcell">
            <button
              type="button"
              class="watchlist-table__star"
              title="管理自选分组"
              aria-label="管理自选分组"
              @click.stop="emit('edit-membership', row.item)"
            >★</button>
          </span>
        </div>
      </div>

      <div v-else class="watchlist-table__empty">
        <span v-if="loading" class="watchlist-table__spinner" aria-hidden="true"></span>
        <v-icon v-else>fa-regular fa-star</v-icon>
        <strong>{{ loading ? "正在加载自选股…" : emptyText }}</strong>
        <small v-if="!loading">点击图表标题旁的星号即可加入分组</small>
      </div>
      <div v-if="loadingMore" class="watchlist-table__loading-more">加载更多…</div>
    </div>
  </div>
</template>

<style scoped>
.watchlist-table {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  color: var(--tv-text);
}

.watchlist-table__header,
.watchlist-table__row {
  display: grid;
  grid-template-columns: minmax(180px, 1.55fr) minmax(104px, 0.8fr) minmax(84px, 0.72fr) minmax(120px, 0.9fr) minmax(72px, 0.58fr) minmax(132px, 0.95fr) minmax(110px, 0.8fr) 44px;
  align-items: center;
}

.watchlist-table__header-viewport {
  flex: 0 0 auto;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.watchlist-table--compact .watchlist-table__header,
.watchlist-table--compact .watchlist-table__row {
  grid-template-columns: minmax(104px, 1fr) minmax(68px, 0.55fr) minmax(68px, 0.55fr) 32px;
}

.watchlist-table__header {
  min-width: 840px;
  min-height: 34px;
  padding: 0 10px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-dim);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.watchlist-table--compact .watchlist-table__header {
  min-width: 0;
  min-height: 30px;
  padding: 0 6px;
  font-size: 9px;
}

.watchlist-table__viewport {
  position: relative;
  min-height: 0;
  flex: 1;
  overflow: auto;
}

.watchlist-table__spacer {
  position: relative;
  min-width: 840px;
}

.watchlist-table--compact .watchlist-table__spacer {
  min-width: 0;
}

.watchlist-table__row {
  position: absolute;
  inset: 0 0 auto 0;
  width: 100%;
  min-width: 840px;
  padding: 0 10px;
  border: 0;
  border-bottom: 1px solid color-mix(in srgb, var(--tv-border) 72%, transparent);
  background: var(--tv-bg-surface);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.watchlist-table--compact .watchlist-table__row {
  min-width: 0;
  padding: 0 6px;
}

.watchlist-table__row:hover,
.watchlist-table__row.is-active {
  background: color-mix(in srgb, var(--tv-accent) 9%, var(--tv-bg-surface));
}

.watchlist-table__row.is-active {
  box-shadow: inset 2px 0 0 var(--tv-accent);
}

.watchlist-table__row > span,
.watchlist-table__header > span {
  min-width: 0;
  padding-right: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.watchlist-table__instrument {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.watchlist-table__instrument strong {
  overflow: hidden;
  font-size: 12px;
  font-weight: 650;
  text-overflow: ellipsis;
}

.watchlist-table__instrument small,
.watchlist-table__muted {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.watchlist-table__price {
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.is-numeric {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.is-action {
  padding-right: 0 !important;
  text-align: center;
}

.is-up { color: var(--tv-price-up); }
.is-down { color: var(--tv-price-down); }

.watchlist-table__session {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.watchlist-table__quote-error {
  color: var(--tv-warning, #d97706);
  font-size: 10px;
}

.watchlist-table__quote-stale {
  display: inline-flex;
  width: 12px;
  height: 12px;
  align-items: center;
  justify-content: center;
  margin-left: 2px;
  border: 1px solid currentColor;
  border-radius: 999px;
  color: var(--tv-warning, #d97706);
  font-size: 8px;
  line-height: 1;
  vertical-align: 1px;
}

.watchlist-table__star {
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: #eab308;
  cursor: pointer;
}

.watchlist-table__star:hover {
  background: color-mix(in srgb, #eab308 14%, transparent);
}

.watchlist-table__empty {
  display: flex;
  height: 100%;
  min-height: 160px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text-dim);
  text-align: center;
}

.watchlist-table__empty strong { color: var(--tv-text-muted); font-size: 13px; }
.watchlist-table__empty small { max-width: 260px; font-size: 11px; }

.watchlist-table__spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--tv-border);
  border-top-color: var(--tv-accent);
  border-radius: 999px;
  animation: watchlist-spin 0.8s linear infinite;
}

.watchlist-table__loading-more {
  position: sticky;
  bottom: 4px;
  width: fit-content;
  margin: 0 auto;
  padding: 3px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text-dim);
  font-size: 10px;
}

@keyframes watchlist-spin { to { transform: rotate(360deg); } }
</style>
