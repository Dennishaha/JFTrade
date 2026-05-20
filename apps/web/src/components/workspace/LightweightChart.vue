<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from "vue";

import KlineChart from "../KlineChart.vue";
import {
  KLINE_PERIODS,
  formatKlinePeriodLabel,
  normalizeKlinePeriod,
  overlayRealtimeTickCandle,
  type KlineCandle,
} from "../../charting/kline";
import { useConsoleData } from "../../composables/useConsoleData";
import { useWorkspaceLayout } from "../../composables/useWorkspaceLayout";

const { prefs, update } = useWorkspaceLayout();
const {
  marketDataCandles,
  marketDataSnapshot,
  marketDataQueryMarket,
  marketDataQuerySymbol,
  marketDataQueryPeriod,
  marketDataQueryLimit,
  marketDataQueryError,
  marketInstrumentSearchOptions,
  isLoadingMarketDataQuery,
  loadMarketDataQuery,
  acquireMarketDataSubscription,
  createStableWebConsumerId,
  heartbeatMarketDataConsumer,
  releaseMarketDataSubscription,
} = useConsoleData();

const chartConsumerId = createStableWebConsumerId("workspace-chart");
let heldChartSubscription: {
  market: string;
  symbol: string;
  channel: "KLINE" | "TICK";
  interval: string;
} | null = null;
let heartbeatTimer: number | null = null;

const periods = KLINE_PERIODS;
const chartCandles = computed<KlineCandle[]>(() =>
  overlayRealtimeTickCandle(
    marketDataCandles.value?.candles ?? [],
    marketDataSnapshot.value?.snapshot ?? null,
    marketDataQueryPeriod.value,
  ),
);
const chartInstrumentTitle = computed(() => {
  const instrumentId = `${prefs.value.market}.${prefs.value.symbol}`;
  const option = marketInstrumentSearchOptions.value.find(
    (candidate) => candidate.instrumentId === instrumentId,
  );
  return option?.name == null || option.name === ""
    ? instrumentId
    : `${instrumentId} · ${option.name}`;
});
const sessionLabels: Record<string, string> = {
  regular: "盘中",
  pre: "盘前",
  after: "盘后",
  overnight: "夜盘",
  closed: "休市",
  unknown: "未知时段",
  all: "盘前/盘后K线",
};
const chartSessionBadge = computed(() => {
  const snapshotSession = marketDataSnapshot.value?.snapshot?.session;
  if (typeof snapshotSession === "string" && snapshotSession !== "") {
    return sessionLabels[snapshotSession] ?? snapshotSession;
  }
  const candleSession = marketDataCandles.value?.meta?.session;
  if (typeof candleSession === "string" && candleSession !== "") {
    return sessionLabels[candleSession] ?? candleSession;
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

async function reload(): Promise<void> {
  marketDataQueryMarket.value = prefs.value.market;
  marketDataQuerySymbol.value = prefs.value.symbol;
  marketDataQueryPeriod.value = normalizeKlinePeriod(prefs.value.period);
  await syncChartSubscription();
  await loadMarketDataQuery();
}

async function syncChartSubscription(): Promise<void> {
  const interval = normalizeKlinePeriod(prefs.value.period);
  const channel: "TICK" | "KLINE" = interval === "tick" ? "TICK" : "KLINE";
  const next = {
    market: prefs.value.market.trim().toUpperCase(),
    symbol: prefs.value.symbol.trim().toUpperCase(),
    channel,
    interval,
  };

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
  await heartbeatMarketDataConsumer(chartConsumerId);
  heldChartSubscription = next;
}

function setPeriod(p: string): void {
  update({ period: normalizeKlinePeriod(p) });
  void reload();
}

onMounted(() => {
  void reload();
  heartbeatTimer = window.setInterval(() => {
    void heartbeatMarketDataConsumer(chartConsumerId);
  }, 15_000);
});

onBeforeUnmount(() => {
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
  () => [prefs.value.market, prefs.value.symbol] as const,
  () => {
    void reload();
  },
);
</script>

<template>
  <section class="tv-panel tv-grid-area-chart">
    <div class="tv-panel-head">
      <span class="tv-panel-title">Chart</span>
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
      <span v-if="chartSessionBadge" :title="chartSessionTitle" style="border: 1px solid var(--tv-border); border-radius: 999px; padding: 3px 8px; color: var(--tv-text); background: var(--card-teal-surface); font-size: 11px; white-space: nowrap">
        {{ chartSessionBadge }}
      </span>
      <span v-if="isLoadingMarketDataQuery" style="color: var(--tv-text-dim); font-size: 11px">loading…</span>
      <span v-else-if="marketDataQueryError" style="color: var(--tv-down); font-size: 11px" :title="marketDataQueryError">{{ marketDataQueryError }}</span>
      <span v-else style="color: var(--tv-text-dim); font-size: 11px">{{ marketDataCandles?.totalReturned ?? 0 }} bars · {{ formatKlinePeriodLabel(prefs.period) }} · limit {{ marketDataQueryLimit }}</span>
      <button class="tv-icon-btn" title="Reload" @click="reload">↻</button>
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
