<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from "vue";

import KlineChart from "../KlineChart.vue";
import MarketFeedStatus from "../domain/market-data/MarketFeedStatus.vue";
import {
  KLINE_PERIODS,
  formatKlinePeriodLabel,
  normalizeKlinePeriod,
  overlayRealtimeTickCandle,
  type KlineCandle,
} from "../../charting/kline";
import { formatMarketSessionLabel } from "../../composables/marketSessionDisplay";
import { useMarketDataProviderStatus } from "../../composables/marketDataProviderStatus";
import { getSharedLiveSocketHub } from "../../composables/sharedLiveSocket";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";

const { prefs, update } = useWorkspaceTradingPrefs();
const {
  loadMarketDataProviderStatus,
  providerCapabilitySummary,
  providerDisplayName,
} = useMarketDataProviderStatus();
const {
  currentMarketDataCandles: marketDataCandles,
  currentMarketDataSnapshot: marketDataSnapshot,
  marketDataQueryMarket,
  marketDataQuerySymbol,
  marketDataQueryPeriod,
  marketDataQueryLimit,
  marketDataQueryError,
  marketInstrumentSearchOptions,
  isLoadingMarketDataQuery,
  loadMarketDataQuery,
  selectWorkspaceInstrument,
  acquireMarketDataSubscription,
  createStableWebConsumerId,
  heartbeatMarketDataConsumer,
  releaseMarketDataSubscription,
  activeMarketDataInstrumentId,
  isMarketDataStale,
  isLiveStreamConnected,
} = useConsoleData();

const chartConsumerId = createStableWebConsumerId("workspace-chart");
const liveHub = getSharedLiveSocketHub();
let heldChartSubscription: {
  market: string;
  symbol: string;
  channel: "KLINE" | "TICK";
  interval: string;
} | null = null;
let heartbeatTimer: number | null = null;
let reloadInFlight: { key: string; promise: Promise<void> } | null = null;
let chartReloadSeq = 0;

const periods = KLINE_PERIODS;
const chartTarget = computed(() => {
  const period = normalizeKlinePeriod(prefs.value.period);
  return {
    market: prefs.value.market.trim().toUpperCase(),
    symbol: prefs.value.symbol.trim().toUpperCase(),
    period,
    instrumentId:
      prefs.value.market.trim() === "" || prefs.value.symbol.trim() === ""
        ? ""
        : `${prefs.value.market.trim().toUpperCase()}.${prefs.value.symbol.trim().toUpperCase()}`,
    channel: (period === "tick" ? "TICK" : "KLINE") as "TICK" | "KLINE",
    interval: period,
  };
});
const chartCandles = computed<KlineCandle[]>(() =>
  overlayRealtimeTickCandle(
    marketDataCandles.value?.candles ?? [],
    marketDataSnapshot.value?.snapshot ?? null,
    marketDataQueryPeriod.value,
  ),
);
const chartInstrumentTitle = computed(() => {
  const instrumentId = chartTarget.value.instrumentId;
  const option = marketInstrumentSearchOptions.value.find(
    (candidate) => candidate.instrumentId === instrumentId,
  );
  return option?.name == null || option.name === ""
    ? instrumentId
    : `${instrumentId} · ${option.name}`;
});
const chartSessionBadge = computed(() => {
  const snapshotSession = marketDataSnapshot.value?.snapshot?.session;
  if (typeof snapshotSession === "string" && snapshotSession !== "") {
    return formatMarketSessionLabel(snapshotSession);
  }
  const candleSession = marketDataCandles.value?.meta?.session;
  if (typeof candleSession === "string" && candleSession !== "") {
    return candleSession === "all" ? "盘前/盘后K线" : formatMarketSessionLabel(candleSession);
  }
  return "";
});
const chartSessionTitle = computed(() => {
  const extendedHours =
    marketDataSnapshot.value?.snapshot?.extendedHours === true ||
    marketDataCandles.value?.meta?.extendedHours === true;
  return extendedHours
    ? "美股扩展时段数据：历史K线请求盘前/盘后，实时快照含盘前/盘后/夜盘字段"
    : "常规交易时段数据";
});
const chartObservedAt = computed(() => {
  const snapshot = marketDataSnapshot.value?.snapshot;
  if (snapshot?.observedAt || snapshot?.at) return snapshot.observedAt ?? snapshot.at;
  const candle = marketDataCandles.value?.candles.at(-1);
  return candle?.displayAt ?? candle?.at ?? marketDataCandles.value?.meta.resolvedAt ?? null;
});
const chartConnectionState = computed(() =>
  liveHub.connectionState?.value ?? (isLiveStreamConnected.value ? "connected" : "disconnected"),
);
const chartTransportMode = computed(() =>
  liveHub.lastHeartbeatEvent?.value?.transport?.mode ?? null,
);
const chartSource = computed(() =>
  marketDataSnapshot.value?.meta.source ?? marketDataCandles.value?.meta.source ?? null,
);
const chartFromCache = computed(() =>
  marketDataSnapshot.value?.meta.fromCache ?? marketDataCandles.value?.meta.fromCache ?? false,
);

function resolveChartSubscriptionTarget() {
  return chartTarget.value;
}

function chartTargetKey(
  target: ReturnType<typeof resolveChartSubscriptionTarget>,
): string {
  return JSON.stringify(target);
}

async function reload(options: { preserveExisting?: boolean } = {}): Promise<void> {
  const target = resolveChartSubscriptionTarget();
  const reloadKey = chartTargetKey(target);
  if (reloadInFlight != null && reloadInFlight.key === reloadKey) {
    return reloadInFlight.promise;
  }

  const requestSeq = ++chartReloadSeq;
  const promise = (async () => {
    selectWorkspaceInstrument({
      market: target.market,
      symbol: target.symbol,
      period: target.period,
    });
    await syncChartSubscription(target, requestSeq);
    if (requestSeq !== chartReloadSeq) {
      return;
    }
    await loadMarketDataQuery(
      options.preserveExisting == null
        ? {}
        : { preserveExisting: options.preserveExisting },
    );
  })();
  reloadInFlight = { key: reloadKey, promise };

  try {
    await promise;
  } finally {
    if (reloadInFlight?.promise === promise) {
      reloadInFlight = null;
    }
  }
}

function handleChartVisibilityChange(): void {
  if (typeof document !== "undefined" && document.visibilityState === "hidden") {
    return;
  }
  const target = chartTarget.value;
  const hasLoadedCurrentTarget =
    target.instrumentId !== "" &&
    activeMarketDataInstrumentId.value === target.instrumentId &&
    marketDataCandles.value != null;

  // Wait for the WebSocket reconnect (triggered by AppShell) before deciding
  // the recovery path, so we don't trigger a full reload while reconnecting.
  void liveHub.waitForConnection(3_000).then((connected) => {
    // Smart recovery: if SSE is connected and data is fresh, only send heartbeat
    if (
      connected &&
      isLiveStreamConnected.value &&
      !isMarketDataStale(30_000) &&
      hasLoadedCurrentTarget
    ) {
      void heartbeatMarketDataConsumer(chartConsumerId);
      return;
    }

    // SSE connected but data stale → reload with preserveExisting to keep accumulated state
    if (
      connected &&
      isLiveStreamConnected.value &&
      hasLoadedCurrentTarget &&
      !isMarketDataStale(120_000)
    ) {
      void reload({ preserveExisting: true });
      return;
    }

    // SSE disconnected or data very stale → full reload
    if (hasLoadedCurrentTarget) {
      void heartbeatMarketDataConsumer(chartConsumerId);
      return;
    }

    void reload();
  });
}

function handleChartOnline(): void {
  void reload();
}

async function syncChartSubscription(
  next: ReturnType<typeof resolveChartSubscriptionTarget>,
  requestSeq = chartReloadSeq,
): Promise<void> {
  if (
    heldChartSubscription != null &&
    (heldChartSubscription.market !== next.market ||
      heldChartSubscription.symbol !== next.symbol ||
      heldChartSubscription.channel !== next.channel ||
      heldChartSubscription.interval !== next.interval)
  ) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      market: heldChartSubscription.market,
      symbol: heldChartSubscription.symbol,
      channel: heldChartSubscription.channel,
      ...(heldChartSubscription.channel === "KLINE"
        ? { interval: heldChartSubscription.interval }
        : {}),
    });
    heldChartSubscription = null;
  }

  if (next.market === "" || next.symbol === "") {
    return;
  }

  await acquireMarketDataSubscription({
    consumerId: chartConsumerId,
    market: next.market,
    symbol: next.symbol,
    channel: next.channel,
    ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
  });
  if (requestSeq !== chartReloadSeq) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      market: next.market,
      symbol: next.symbol,
      channel: next.channel,
      ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
    });
    return;
  }

  await heartbeatMarketDataConsumer(chartConsumerId);
  if (requestSeq !== chartReloadSeq) {
    await releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      market: next.market,
      symbol: next.symbol,
      channel: next.channel,
      ...(next.channel === "KLINE" ? { interval: next.interval } : {}),
    });
    return;
  }

  heldChartSubscription = next;
}

function setPeriod(p: string): void {
  update({ period: normalizeKlinePeriod(p) });
}

onMounted(() => {
  void loadMarketDataProviderStatus();
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", handleChartVisibilityChange);
  }
  if (typeof window !== "undefined") {
    window.addEventListener("online", handleChartOnline);
  }
  void reload();
  heartbeatTimer = window.setInterval(() => {
    void heartbeatMarketDataConsumer(chartConsumerId);
  }, 15_000);
});

onBeforeUnmount(() => {
  if (typeof document !== "undefined") {
    document.removeEventListener("visibilitychange", handleChartVisibilityChange);
  }
  if (typeof window !== "undefined") {
    window.removeEventListener("online", handleChartOnline);
  }
  if (heartbeatTimer != null) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
  if (heldChartSubscription != null) {
    void releaseMarketDataSubscription({
      consumerId: chartConsumerId,
      market: heldChartSubscription.market,
      symbol: heldChartSubscription.symbol,
      channel: heldChartSubscription.channel,
      ...(heldChartSubscription.channel === "KLINE"
        ? { interval: heldChartSubscription.interval }
        : {}),
      keepalive: true,
    });
  }
});

watch(
  () => chartTargetKey(chartTarget.value),
  () => {
    void reload();
  },
);
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <span class="tv-panel-title">图表</span>
      <span style="color: var(--tv-text); font-weight: 600">{{ chartInstrumentTitle }}</span>
      <div class="tv-seg" style="margin-left: 12px">
        <button
          v-for="p in periods"
          :key="p.value"
          :class="{ 'is-active': normalizeKlinePeriod(prefs.period) === p.value }"
          @click="setPeriod(p.value)"
        >
          {{ p.label }}
        </button>
      </div>
      <div style="flex: 1"></div>
      <MarketFeedStatus
        :connection-state="chartConnectionState"
        :observed-at="chartObservedAt"
        :transport-mode="chartTransportMode"
        :session="chartSessionBadge"
        :context-title="chartSessionTitle"
        :source="chartSource"
        :provider-name="providerDisplayName"
        :provider-capabilities="providerCapabilitySummary"
        :from-cache="chartFromCache"
        :loading="isLoadingMarketDataQuery"
        :error="marketDataQueryError"
      />
      <span style="color: var(--tv-text-dim); font-size: 11px">{{ marketDataCandles?.totalReturned ?? 0 }} 根 · {{ formatKlinePeriodLabel(prefs.period) }} · 上限 {{ marketDataQueryLimit }}</span>
      <button class="tv-icon-btn" title="刷新" @click="() => reload()">↻</button>
    </div>
    <div class="tv-panel-body is-flush">
      <div class="tv-chart-host">
        <KlineChart
          :candles="chartCandles"
          :min-height="320"
          show-indicator-selector
          indicator-storage-key="jftrade.workspace-chart.indicators"
          :default-indicators="['volume']"
          empty-text="暂无 K 线数据；确认 OpenD 行情权限后点击刷新。"
        />
      </div>
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

.tv-chart-host {
  flex: 1;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.tv-chart-host :deep(.kline-chart-shell) {
  height: 100%;
  min-height: 0;
}
</style>
