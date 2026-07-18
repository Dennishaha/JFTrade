<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import type {
  MarketDataDepthResponse,
} from "@/contracts";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import {
  getSharedLiveSocketHub,
  type MarketDepthLiveStreamEvent,
} from "../../composables/sharedLiveSocket";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import OrderBookDepthTable from "../domain/market-data/OrderBookDepthTable.vue";

const {
  currentMarketDataSnapshot: marketDataSnapshot,
  currentMarketSecurityDetails: marketSecurityDetails,
  acquireMarketDataSubscription,
  createStableWebConsumerId,
  heartbeatMarketDataConsumer,
  releaseMarketDataSubscription,
} = useConsoleData();
const { prefs } = useWorkspaceTradingPrefs();
const { selectedBrokerId } = useBrokerProviderSelection();

// --- Depth presets ---
const DEPTH_PRESETS = [5, 10, 20, 50] as const;
const DEFAULT_DEPTH_NUM = 10;

// --- State ---
const depthNum = ref(DEFAULT_DEPTH_NUM);
const depthData = ref<MarketDataDepthResponse | null>(null);
const isLoadingDepth = ref(false);
const depthError = ref("");
let depthRequestSeq = 0;
let depthAbortController: AbortController | null = null;
let lastDepthDataRefreshedAt = 0;
let depthLifecycleSeq = 0;
let heartbeatTimer = 0;
let isUnmounted = false;
const depthConsumerId = createStableWebConsumerId("workspace-depth");
let heldDepthSubscription: {
  brokerId: string;
  market: string;
  symbol: string;
  instrumentId: string;
} | null = null;
const liveHub = getSharedLiveSocketHub();
const depthSubscriptionOwnerId = liveHub.createOwnerId("order-book-depth");
const removeDepthListener = liveHub.addEventListener((event) => {
  if (!isMarketDepthEvent(event)) {
    return;
  }
  if (event.meta.instrumentId.trim().toUpperCase() !== currentInstrumentId.value) {
    return;
  }
  if (Math.trunc(event.request.num) !== depthNum.value) {
    return;
  }
  depthData.value = event as unknown as MarketDataDepthResponse;
  depthError.value = "";
  isLoadingDepth.value = false;
  lastDepthDataRefreshedAt = Date.now();
});

function isMarketDepthEvent(
  event: { type: string },
): event is MarketDepthLiveStreamEvent {
  return event.type === "market.depth";
}

interface DepthLevel {
  bidPrice: number | null;
  askPrice: number | null;
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
      bidPrice: bid?.price ?? null,
      askPrice: ask?.price ?? null,
      bidSize: bid?.volume ?? 0,
      askSize: ask?.volume ?? 0,
    });
  }
  return levels;
});

const snapshot = computed(() => {
  const s = marketDataSnapshot.value;
  if (!s || !s.snapshot) return null;
  return s.snapshot;
});

const security = computed(() => marketSecurityDetails.value?.security ?? null);
const currentInstrumentId = computed(() => {
  const market = prefs.value?.market?.trim().toUpperCase() ?? "";
  const symbol = prefs.value?.symbol?.trim().toUpperCase() ?? "";
  return market === "" || symbol === "" ? "" : `${market}.${symbol}`;
});

function resolveDepthSubscriptionTarget() {
  const market = prefs.value?.market?.trim().toUpperCase() ?? "";
  const symbol = prefs.value?.symbol?.trim().toUpperCase() ?? "";
  return {
    brokerId: selectedBrokerId.value,
    market,
    symbol,
    instrumentId: market === "" || symbol === "" ? "" : `${market}.${symbol}`,
  };
}

function isSameDepthSubscription(
  left: ReturnType<typeof resolveDepthSubscriptionTarget>,
  right: ReturnType<typeof resolveDepthSubscriptionTarget>,
): boolean {
  return (
    left.brokerId === right.brokerId &&
    left.market === right.market &&
    left.symbol === right.symbol
  );
}

const bidPrice = computed(() => security.value?.bidPrice ?? snapshot.value?.bid ?? null);
const askPrice = computed(() => security.value?.askPrice ?? snapshot.value?.ask ?? null);
const bidVolume = computed(() => security.value?.bidVolume ?? null);
const askVolume = computed(() => security.value?.askVolume ?? null);
const lastPrice = computed(() => security.value?.currentPrice ?? snapshot.value?.price ?? null);
const depthObservedAt = computed(() => depthData.value?.meta.resolvedAt ?? null);
const depthConnectionState = computed(() => liveHub.connectionState?.value ?? "idle");
const depthTransportMode = computed(() => liveHub.lastHeartbeatEvent?.value?.transport?.mode ?? null);

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

function fmtPrice(v: number | null): string {
  if (v == null) return "--";
  return v.toFixed(v < 1 ? 4 : v < 10 ? 3 : 2);
}

function fmtSize(v: number | null): string {
  if (v == null) return "--";
  if (v >= 1_000_000_000) return (v / 1_000_000_000).toFixed(2) + "B";
  if (v >= 1_000_000) return (v / 1_000_000).toFixed(2) + "M";
  if (v >= 1_000) return (v / 1_000).toFixed(1) + "K";
  return v.toFixed(0);
}

// --- API calls ---

function buildDepthUrl(): string | null {
  const market = prefs.value?.market;
  const symbol = prefs.value?.symbol;
  if (!market || !symbol) return null;
  return withBrokerProvider(
    `/api/v1/market-data/depth/${market}/${symbol}?num=${depthNum.value}`,
    selectedBrokerId.value,
  );
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
    if (data.meta.instrumentId.trim().toUpperCase() !== currentInstrumentId.value) {
      return;
    }
    depthData.value = data;
    lastDepthDataRefreshedAt = Date.now();
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

function closeDepthStream(): void {
  liveHub.setDepthTarget(depthSubscriptionOwnerId, null);
}

function clearDepthData(): void {
  depthRequestSeq += 1;
  depthAbortController?.abort();
  depthAbortController = null;
  depthData.value = null;
  depthError.value = "";
  isLoadingDepth.value = false;
  lastDepthDataRefreshedAt = 0;
}

async function releaseDepthSubscription(
  target: ReturnType<typeof resolveDepthSubscriptionTarget>,
  keepalive = false,
): Promise<void> {
  await releaseMarketDataSubscription({
    consumerId: depthConsumerId,
    ...(target.brokerId ? { brokerId: target.brokerId } : {}),
    market: target.market,
    symbol: target.symbol,
    channel: "ORDER_BOOK",
    keepalive,
  });
}

async function syncDepthSubscription(
  target: ReturnType<typeof resolveDepthSubscriptionTarget>,
  lifecycleSeq: number,
  forceAcquire = false,
): Promise<boolean> {
  if (heldDepthSubscription != null && !isSameDepthSubscription(heldDepthSubscription, target)) {
    const previous = heldDepthSubscription;
    heldDepthSubscription = null;
    await releaseDepthSubscription(previous);
  }
  if (isUnmounted || lifecycleSeq !== depthLifecycleSeq) {
    return false;
  }

  if (heldDepthSubscription != null && !forceAcquire) {
    void heartbeatDepthSubscription(target.brokerId);
    return !isUnmounted && lifecycleSeq === depthLifecycleSeq;
  }

  let acquired = false;
  try {
    acquired = await acquireMarketDataSubscription({
      consumerId: depthConsumerId,
      ...(target.brokerId ? { brokerId: target.brokerId } : {}),
      market: target.market,
      symbol: target.symbol,
      channel: "ORDER_BOOK",
    });
  } catch (error) {
    depthError.value = error instanceof Error ? error.message : "盘口订阅申请失败";
  }
  if (!acquired) {
    if (depthError.value === "") {
      depthError.value = "盘口订阅申请失败";
    }
    isLoadingDepth.value = false;
    return false;
  }

  if (
    isUnmounted ||
    lifecycleSeq !== depthLifecycleSeq ||
    !isSameDepthSubscription(target, resolveDepthSubscriptionTarget())
  ) {
    await releaseDepthSubscription(target, isUnmounted);
    return false;
  }

  heldDepthSubscription = target;
  void heartbeatDepthSubscription(target.brokerId);
  return true;
}

function heartbeatDepthSubscription(brokerId: string): Promise<void> {
  return brokerId
    ? heartbeatMarketDataConsumer(depthConsumerId, brokerId)
    : heartbeatMarketDataConsumer(depthConsumerId);
}

async function connectDepthStream(
  preserveData = false,
  forceAcquire = false,
): Promise<void> {
  if (isUnmounted) {
    return;
  }
  const lifecycleSeq = ++depthLifecycleSeq;
  const target = resolveDepthSubscriptionTarget();
  if (target.instrumentId === "") {
    closeDepthStream();
    clearDepthData();
    if (heldDepthSubscription != null) {
      const previous = heldDepthSubscription;
      heldDepthSubscription = null;
      await releaseDepthSubscription(previous);
    }
    return;
  }

  closeDepthStream();
  if (!preserveData) {
    clearDepthData();
  }
  isLoadingDepth.value = true;
  depthError.value = "";
  if (!await syncDepthSubscription(target, lifecycleSeq, forceAcquire)) {
    return;
  }
  if (isUnmounted || lifecycleSeq !== depthLifecycleSeq) {
    return;
  }
  liveHub.setDepthTarget(depthSubscriptionOwnerId, {
    market: target.market,
    symbol: target.symbol,
    instrumentId: target.instrumentId,
    num: depthNum.value,
  });
  await fetchDepth();
}

function isDepthDataStale(maxAgeMs = 30_000): boolean {
  if (lastDepthDataRefreshedAt === 0) return true;
  return Date.now() - lastDepthDataRefreshedAt > maxAgeMs;
}

function isLiveHubConnected(): boolean {
  return liveHub.connectionState.value === "connected";
}

function handleDepthVisibilityChange(): void {
  if (typeof document !== "undefined" && document.visibilityState === "hidden") {
    return;
  }

  // Smart recovery: if SSE is connected and depth data is fresh, skip reconnection
  if (isLiveHubConnected() && !isDepthDataStale(30_000)) {
    return;
  }

  // Wait for the WebSocket reconnect (triggered by AppShell) before deciding
  // the recovery path, so we don't tear down depth data while reconnecting.
  void liveHub.waitForConnection(3_000).then((connected) => {
    if (connected && !isDepthDataStale(30_000)) {
      // Connection recovered and data still fresh — no action needed
      return;
    }
    // SSE still disconnected or depth data stale → reconnect
    void connectDepthStream(true, true);
  });
}

function handleDepthOnline(): void {
  void connectDepthStream(false, true);
}

function setDepthNum(num: number): void {
  if (depthNum.value === num) return;
  depthNum.value = num;
  void connectDepthStream();
}

// --- Lifecycle ---
onMounted(() => {
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", handleDepthVisibilityChange);
  }
  if (typeof window !== "undefined") {
    window.addEventListener("online", handleDepthOnline);
  }
  void connectDepthStream();
  heartbeatTimer = window.setInterval(() => {
    if (heldDepthSubscription != null) {
      void heartbeatDepthSubscription(heldDepthSubscription.brokerId);
    }
  }, 15_000);
});

onUnmounted(() => {
  isUnmounted = true;
  depthLifecycleSeq += 1;
  if (typeof document !== "undefined") {
    document.removeEventListener("visibilitychange", handleDepthVisibilityChange);
  }
  if (typeof window !== "undefined") {
    window.removeEventListener("online", handleDepthOnline);
  }
  closeDepthStream();
  removeDepthListener();
  if (heartbeatTimer !== 0) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = 0;
  }
  if (heldDepthSubscription != null) {
    const previous = heldDepthSubscription;
    heldDepthSubscription = null;
    void releaseDepthSubscription(previous, true);
  }
  depthAbortController?.abort();
  depthAbortController = null;
});

// Re-fetch only when the instrument changes. Period changes update workspace
// prefs too, but depth data is independent from the chart interval.
watch(
  () => `${selectedBrokerId.value}|${currentInstrumentId.value}`,
  () => {
    clearDepthData();
    void connectDepthStream();
  },
);
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">盘口</span>
      <div style="flex: 1"></div>
      <MarketFeedStatus
        :connection-state="depthConnectionState"
        :observed-at="depthObservedAt"
        :transport-mode="depthTransportMode"
        :source="depthData?.meta.source ?? null"
        :from-cache="depthData?.meta.fromCache ?? false"
        :loading="isLoadingDepth"
        :error="depthError"
      />
    </div>

    <div class="tv-panel-body is-flush">
      <!-- Depth preset selector -->
      <div class="tv-ob-presets">
        <button v-for="preset in DEPTH_PRESETS" :key="preset" class="tv-ob-preset-btn"
          :class="{ 'is-active': depthNum === preset }" @click="setDepthNum(preset)">
          {{ preset }}
        </button>
        <span v-if="isLoadingDepth" class="tv-ob-preset-spinner fa-solid fa-spinner fa-spin"></span>
      </div>

      <!-- BBO ratio bar -->
      <div v-if="bidAskRatio != null" class="tv-ob-ratio-bar">
        <div class="tv-ob-ratio-bid" :style="{ width: bidRatioPercent + '%' }">
          <span v-if="bidRatioPercent && parseFloat(bidRatioPercent) > 10">Bid {{ bidRatioPercent }}%</span>
        </div>
        <div class="tv-ob-ratio-ask" :style="{ width: askRatioPercent + '%' }">
          <span v-if="askRatioPercent && parseFloat(askRatioPercent) > 10">Ask {{ askRatioPercent }}%</span>
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

      <OrderBookDepthTable
        :levels="depthLevels"
        :loading="isLoadingDepth"
        :error="depthError"
        :disabled="currentInstrumentId === ''"
      />

    </div>
  </section>
</template>

<style scoped>
.tv-panel-body.is-flush {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

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
  background: color-mix(in srgb, var(--tv-price-up) 18%, transparent);
  color: var(--tv-price-up);
  text-align: left;
  padding-left: 6px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  transition: width 200ms ease;
}

.tv-ob-ratio-ask {
  background: color-mix(in srgb, var(--tv-price-down) 18%, transparent);
  color: var(--tv-price-down);
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
  grid-template-columns: 1fr auto auto 1fr;
  gap: 1px;
  background: var(--tv-border);
  box-sizing: border-box;
}

.tv-ob-bbo::before,
.tv-ob-bbo::after {
  content: "";
  background: var(--tv-bg-surface);
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

</style>
