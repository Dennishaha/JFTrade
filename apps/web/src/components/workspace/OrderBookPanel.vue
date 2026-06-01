<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import type {
  MarketDataDepthResponse,
} from "@jftrade/ui-contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "../../composables/apiClient";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceLayout } from "../../composables/useWorkspaceLayout";

const { marketDataSnapshot, marketSecurityDetails } = useConsoleData();
const { prefs } = useWorkspaceLayout();

// --- Depth presets ---
const DEPTH_PRESETS = [5, 10, 20, 50] as const;
const DEFAULT_DEPTH_NUM = 10;

// --- State ---
const depthNum = ref(DEFAULT_DEPTH_NUM);
const depthData = ref<MarketDataDepthResponse | null>(null);
const isLoadingDepth = ref(false);
const depthError = ref("");
let pollTimer: ReturnType<typeof setInterval> | null = null;
let depthRequestSeq = 0;
let depthAbortController: AbortController | null = null;

interface DepthLevel {
  price: number;
  bidSize: number;
  askSize: number;
}

// --- Derived: depth levels from API response ---
const depthLevels = computed<DepthLevel[]>(() => {
  const data = depthData.value;
  if (!data?.depth) return [];

  const bids = data.depth.bids ?? [];
  const asks = data.depth.asks ?? [];
  const maxLen = Math.max(bids.length, asks.length);
  if (maxLen === 0) return [];

  const levels: DepthLevel[] = [];
  for (let i = 0; i < maxLen; i++) {
    const bid = bids[i] ?? null;
    const ask = asks[i] ?? null;
    levels.push({
      price: bid?.price ?? ask?.price ?? 0,
      bidSize: bid?.volume ?? 0,
      askSize: ask?.volume ?? 0,
    });
  }
  return levels;
});

const maxBidSize = computed(() => {
  const levels = depthLevels.value;
  if (levels.length === 0) return 1;
  return Math.max(...levels.map((l) => l.bidSize), 1);
});

const maxAskSize = computed(() => {
  const levels = depthLevels.value;
  if (levels.length === 0) return 1;
  return Math.max(...levels.map((l) => l.askSize), 1);
});

const snapshot = computed(() => {
  const s = marketDataSnapshot.value;
  if (!s || !s.snapshot) return null;
  return s.snapshot;
});

const security = computed(() => marketSecurityDetails.value?.security ?? null);

const bidPrice = computed(() => security.value?.bidPrice ?? snapshot.value?.bid ?? null);
const askPrice = computed(() => security.value?.askPrice ?? snapshot.value?.ask ?? null);
const bidVolume = computed(() => security.value?.bidVolume ?? null);
const askVolume = computed(() => security.value?.askVolume ?? null);
const lastPrice = computed(() => security.value?.currentPrice ?? snapshot.value?.price ?? null);

const changeFromClose = computed(() => {
  const lp = lastPrice.value;
  const prev = security.value?.lastClosePrice ?? snapshot.value?.previousClosePrice ?? null;
  if (lp == null || prev == null || prev === 0) return null;
  return lp - prev;
});

const changePercent = computed(() => {
  const lp = lastPrice.value;
  const prev = security.value?.lastClosePrice ?? snapshot.value?.previousClosePrice ?? null;
  if (lp == null || prev == null || prev === 0) return null;
  return ((lp - prev) / prev) * 100;
});

const bidAskRatio = computed(() => {
  const bv = bidVolume.value;
  const av = askVolume.value;
  if (typeof bv === "number" && typeof av === "number" && bv + av > 0) {
    return bv / (bv + av);
  }
  return null;
});

const bidRatioPercent = computed(() => {
  const r = bidAskRatio.value;
  return r != null ? (r * 100).toFixed(2) : null;
});

const askRatioPercent = computed(() => {
  const r = bidAskRatio.value;
  if (r == null) return null;
  return ((1 - r) * 100).toFixed(2);
});

const sideClass = (val: number | null): string => {
  if (val == null) return "";
  return val >= 0 ? "tv-up" : "tv-down";
};

function fmtPrice(v: number | null): string {
  if (v == null) return "—";
  return v.toFixed(v < 1 ? 4 : v < 10 ? 3 : 2);
}

function fmtSize(v: number | null): string {
  if (v == null) return "—";
  if (v >= 1_000_000_000) return (v / 1_000_000_000).toFixed(2) + "B";
  if (v >= 1_000_000) return (v / 1_000_000).toFixed(2) + "M";
  if (v >= 1_000) return (v / 1_000).toFixed(1) + "K";
  return v.toFixed(0);
}

function barWidth(max: number, v: number): string {
  if (max <= 0) return "0%";
  return ((v / max) * 100).toFixed(1) + "%";
}

// --- API calls ---

function buildDepthUrl(): string | null {
  const market = prefs.value?.market;
  const symbol = prefs.value?.symbol;
  if (!market || !symbol) return null;
  return `/api/v1/market-data/depth/${market}/${symbol}?num=${depthNum.value}`;
}

async function loadBrokerCapability(): Promise<void> {
  try {
    const runtime = await fetchEnvelope<any>("/api/v1/brokers/futu/runtime");
    const caps = runtime?.descriptor?.capabilities;
    if (caps && caps.length > 0) {
      const orderBook = caps[0]?.readFeatures?.orderBook;
      if (orderBook?.numPresets) {
        // Use broker default if available
        if (orderBook.defaultNum && DEPTH_PRESETS.includes(orderBook.defaultNum as typeof DEPTH_PRESETS[number])) {
          depthNum.value = orderBook.defaultNum;
        }
      }
    }
  } catch {
    // Silently fall back to defaults
  }
}

async function fetchDepth(): Promise<void> {
  const url = buildDepthUrl();
  if (!url) {
    depthAbortController?.abort();
    depthAbortController = null;
    depthData.value = null;
    depthError.value = "";
    isLoadingDepth.value = false;
    return;
  }

  depthAbortController?.abort();
  const controller = new AbortController();
  depthAbortController = controller;
  const requestSeq = ++depthRequestSeq;
  isLoadingDepth.value = true;
  depthError.value = "";
  try {
    const data = await fetchEnvelopeWithInit<MarketDataDepthResponse>(url, {
      signal: controller.signal,
    });
    if (requestSeq !== depthRequestSeq) return;
    depthData.value = data;
  } catch (err: any) {
    if (controller.signal.aborted) {
      return;
    }
    if (requestSeq !== depthRequestSeq) return;
    depthError.value = err?.message ?? "获取盘口深度失败";
  } finally {
    if (requestSeq === depthRequestSeq) {
      isLoadingDepth.value = false;
    }
    if (depthAbortController === controller) {
      depthAbortController = null;
    }
  }
}

function startPolling(): void {
  stopPolling();
  fetchDepth();
  // Poll every 3 seconds for depth updates
  pollTimer = setInterval(() => {
    fetchDepth();
  }, 3000);
}

function stopPolling(): void {
  if (pollTimer != null) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

function setDepthNum(num: number): void {
  if (depthNum.value === num) return;
  depthNum.value = num;
  fetchDepth();
}

// --- Lifecycle ---
onMounted(() => {
  loadBrokerCapability().then(() => startPolling());
});

onUnmounted(() => {
  stopPolling();
  depthAbortController?.abort();
  depthAbortController = null;
});

// Re-fetch when symbol changes
watch(
  () => [prefs.value?.market, prefs.value?.symbol],
  () => {
    fetchDepth();
  },
);
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">盘口</span>
      <span style="color: var(--tv-text); font-weight: 600">{{ prefs.market }}:{{ prefs.symbol }}</span>
      <div style="flex: 1"></div>
      <span
        v-if="lastPrice"
        style="font-size: 12px; font-weight: 700"
        :class="sideClass(changeFromClose)"
      >
        {{ fmtPrice(lastPrice) }}
      </span>
    </div>

    <div class="tv-panel-body is-flush">
      <!-- Depth preset selector -->
      <div class="tv-ob-presets">
        <button
          v-for="preset in DEPTH_PRESETS"
          :key="preset"
          class="tv-ob-preset-btn"
          :class="{ 'is-active': depthNum === preset }"
          @click="setDepthNum(preset)"
        >
          {{ preset }}档
        </button>
        <span v-if="isLoadingDepth" class="tv-ob-preset-spinner fa-solid fa-spinner fa-spin"></span>
        <span v-if="depthError" class="tv-ob-preset-error" :title="depthError">
          <span class="fa-solid fa-triangle-exclamation"></span>
        </span>
      </div>

      <!-- BBO ratio bar -->
      <div v-if="bidAskRatio != null" class="tv-ob-ratio-bar">
        <div class="tv-ob-ratio-bid" :style="{ width: bidRatioPercent + '%' }">
          <span v-if="bidRatioPercent && parseFloat(bidRatioPercent) > 10">买 {{ bidRatioPercent }}%</span>
        </div>
        <div class="tv-ob-ratio-ask" :style="{ width: askRatioPercent + '%' }">
          <span v-if="askRatioPercent && parseFloat(askRatioPercent) > 10">卖 {{ askRatioPercent }}%</span>
        </div>
      </div>

      <!-- BBO cards -->
      <div class="tv-ob-bbo">
        <div class="tv-ob-bbo-card tv-ob-bbo-bid">
          <div class="tv-ob-bbo-label">买一</div>
          <div class="tv-ob-bbo-price tv-up">{{ fmtPrice(bidPrice) }}</div>
          <div class="tv-ob-bbo-size">{{ fmtSize(bidVolume) }}</div>
        </div>
        <div class="tv-ob-bbo-card tv-ob-bbo-ask">
          <div class="tv-ob-bbo-label">卖一</div>
          <div class="tv-ob-bbo-price tv-down">{{ fmtPrice(askPrice) }}</div>
          <div class="tv-ob-bbo-size">{{ fmtSize(askVolume) }}</div>
        </div>
      </div>

      <!-- Depth levels -->
      <div v-if="depthLevels.length > 0" class="tv-ob-depth-table-wrap">
        <div class="tv-ob-depth-table">
          <div class="tv-ob-depth-col tv-ob-depth-bid-col">
            <div
              v-for="(row, idx) in depthLevels"
              :key="'b' + idx"
              class="tv-ob-depth-row tv-ob-depth-row-bid"
            >
              <div class="tv-ob-depth-bar" :style="{ width: barWidth(maxBidSize, row.bidSize) }"></div>
              <span class="tv-ob-depth-size">{{ fmtSize(row.bidSize) }}</span>
            </div>
          </div>
          <div class="tv-ob-depth-col tv-ob-depth-price-col">
            <div
              v-for="(row, idx) in depthLevels"
              :key="'p' + idx"
              class="tv-ob-depth-row tv-ob-depth-price"
            >
              {{ fmtPrice(row.price) }}
            </div>
          </div>
          <div class="tv-ob-depth-col tv-ob-depth-ask-col">
            <div
              v-for="(row, idx) in depthLevels"
              :key="'a' + idx"
              class="tv-ob-depth-row tv-ob-depth-row-ask"
            >
              <span class="tv-ob-depth-size">{{ fmtSize(row.askSize) }}</span>
              <div class="tv-ob-depth-bar" :style="{ width: barWidth(maxAskSize, row.askSize) }"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- Loading / empty / error placeholder -->
      <div v-else class="tv-ob-depth-placeholder">
        <template v-if="isLoadingDepth && !depthError">
          <div class="tv-ob-depth-placeholder-icon">
            <span class="fa-solid fa-spinner fa-spin"></span>
          </div>
          <div class="tv-ob-depth-placeholder-text">加载盘口数据…</div>
        </template>
        <template v-else-if="depthError">
          <div class="tv-ob-depth-placeholder-icon">
            <span class="fa-solid fa-triangle-exclamation"></span>
          </div>
          <div class="tv-ob-depth-placeholder-text">数据获取失败</div>
          <div class="tv-ob-depth-placeholder-hint">{{ depthError }}</div>
        </template>
        <template v-else>
          <div class="tv-ob-depth-placeholder-icon">
            <span class="fa-solid fa-chart-bar"></span>
          </div>
          <div class="tv-ob-depth-placeholder-text">暂无深度数据</div>
          <div class="tv-ob-depth-placeholder-hint">请确保已连接行情并选择有效证券。</div>
        </template>
      </div>

      <!-- Bottom summary -->
      <div class="tv-ob-footer">
        <div class="tv-ob-footer-item">
          <span class="tv-ob-footer-label">涨跌</span>
          <span :class="sideClass(changeFromClose)" style="font-weight: 600">
            {{ changeFromClose != null ? (changeFromClose >= 0 ? '+' : '') + changeFromClose.toFixed(3) : '—' }}
          </span>
        </div>
        <div class="tv-ob-footer-item">
          <span class="tv-ob-footer-label">涨幅</span>
          <span :class="sideClass(changePercent)" style="font-weight: 600">
            {{ changePercent != null ? (changePercent >= 0 ? '+' : '') + changePercent.toFixed(2) + '%' : '—' }}
          </span>
        </div>
        <div class="tv-ob-footer-item">
          <span class="tv-ob-footer-label">最新价</span>
          <span style="font-weight: 600">{{ fmtPrice(lastPrice) }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
/* ---------- Preset buttons ---------- */
.tv-ob-presets {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 6px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.tv-ob-preset-btn {
  padding: 2px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 3px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
  font-size: 10px;
  font-weight: 500;
  cursor: pointer;
  transition: all 120ms ease;
}

.tv-ob-preset-btn:hover {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}

.tv-ob-preset-btn.is-active {
  background: var(--tv-accent);
  border-color: var(--tv-accent);
  color: #fff;
}

.tv-ob-preset-spinner {
  font-size: 11px;
  color: var(--tv-text-dim);
  margin-left: 4px;
}

.tv-ob-preset-error {
  font-size: 11px;
  color: var(--tv-down);
  margin-left: 4px;
  cursor: help;
}

/* ---------- Ratio bar ---------- */
.tv-ob-ratio-bar {
  display: flex;
  height: 18px;
  margin: 0;
  border-bottom: 1px solid var(--tv-border);
  font-size: 10px;
  line-height: 18px;
}

.tv-ob-ratio-bid {
  background: rgba(22, 199, 132, 0.18);
  color: var(--tv-up);
  text-align: left;
  padding-left: 6px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  transition: width 200ms ease;
}

.tv-ob-ratio-ask {
  background: rgba(234, 57, 67, 0.15);
  color: var(--tv-down);
  text-align: right;
  padding-right: 6px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  transition: width 200ms ease;
}

/* ---------- BBO cards ---------- */
.tv-ob-bbo {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1px;
  background: var(--tv-border);
}

.tv-ob-bbo-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 10px 8px;
  background: var(--tv-bg-surface);
}

.tv-ob-bbo-label {
  font-size: 10px;
  color: var(--tv-text-dim);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.tv-ob-bbo-price {
  font-size: 18px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.tv-ob-bbo-size {
  font-size: 11px;
  color: var(--tv-text-muted);
  font-variant-numeric: tabular-nums;
}

/* ---------- Depth table ---------- */
.tv-ob-depth-table-wrap {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.tv-ob-depth-table {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.tv-ob-depth-col {
  display: flex;
  flex-direction: column;
}

.tv-ob-depth-row {
  display: flex;
  align-items: center;
  height: 20px;
  padding: 0 6px;
  position: relative;
}

.tv-ob-depth-row-bid {
  justify-content: flex-end;
  color: var(--tv-up);
}

.tv-ob-depth-row-bid .tv-ob-depth-bar {
  position: absolute;
  right: 0;
  top: 2px;
  bottom: 2px;
  background: rgba(22, 199, 132, 0.12);
  border-radius: 0 2px 2px 0;
}

.tv-ob-depth-row-ask {
  justify-content: flex-start;
  color: var(--tv-down);
}

.tv-ob-depth-row-ask .tv-ob-depth-bar {
  position: absolute;
  left: 0;
  top: 2px;
  bottom: 2px;
  background: rgba(234, 57, 67, 0.10);
  border-radius: 2px 0 0 2px;
}

.tv-ob-depth-price {
  justify-content: center;
  color: var(--tv-text);
  font-weight: 600;
  padding: 0 10px;
  min-width: 64px;
  border-left: 1px solid var(--tv-border);
  border-right: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.tv-ob-depth-size {
  position: relative;
  z-index: 1;
}

/* ---------- Placeholder ---------- */
.tv-ob-depth-placeholder {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: var(--tv-text-dim);
}

.tv-ob-depth-placeholder-icon {
  font-size: 28px;
  opacity: 0.4;
}

.tv-ob-depth-placeholder-text {
  font-size: 13px;
  font-weight: 500;
}

.tv-ob-depth-placeholder-hint {
  font-size: 11px;
  max-width: 180px;
  text-align: center;
  line-height: 1.4;
}

/* ---------- Footer ---------- */
.tv-ob-footer {
  display: flex;
  justify-content: space-around;
  border-top: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  padding: 6px 0;
  font-size: 11px;
  flex-shrink: 0;
}

.tv-ob-footer-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
}

.tv-ob-footer-label {
  font-size: 10px;
  color: var(--tv-text-dim);
  text-transform: uppercase;
}
</style>
