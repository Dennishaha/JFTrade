<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import type { LiveSocketConnectionState } from "../../../composables/sharedLiveSocket";
import { formatMarketSessionLabel } from "../../../composables/marketSessionDisplay";
import MarketStatusBadge from "./MarketStatusBadge.vue";

const props = withDefaults(defineProps<{
  connectionState: LiveSocketConnectionState;
  observedAt?: string | null;
  comparisonObservedAt?: string | null;
  comparisonLabel?: string;
  transportMode?: string | null;
  session?: string | null;
  source?: string | null;
  providerName?: string | null;
  providerCapabilities?: string | null;
  contextTitle?: string;
  emptyLabel?: string;
  fromCache?: boolean;
  loading?: boolean;
  error?: string | null;
  staleAfterMs?: number;
}>(), {
  observedAt: null,
  comparisonObservedAt: null,
  comparisonLabel: "数据",
  transportMode: null,
  session: null,
  source: null,
  providerName: null,
  providerCapabilities: null,
  contextTitle: "",
  emptyLabel: "无数据",
  fromCache: false,
  loading: false,
  error: null,
  staleAfterMs: 30_000,
});

const now = ref(Date.now());
let clock: ReturnType<typeof setInterval> | null = null;

const observedTime = computed(() => parseTimestamp(props.observedAt));
const ageMs = computed(() => {
  if (observedTime.value == null) return null;
  return Math.max(0, now.value - observedTime.value);
});
const badgeState = computed<"live" | "stale" | "loading" | "empty" | "error">(() => {
  if (props.error && observedTime.value == null) return "error";
  if (props.loading && observedTime.value == null) return "loading";
  if (ageMs.value == null) return "empty";
  return ageMs.value > props.staleAfterMs ? "stale" : "live";
});
const freshnessLabel = computed(() => {
  if (badgeState.value === "error") return props.error?.trim() || "异常";
  if (badgeState.value === "loading") return "加载中";
  if (badgeState.value === "empty") return props.emptyLabel;
  return `${badgeState.value === "stale" ? "陈旧" : "新鲜"} ${formatAge(ageMs.value ?? 0)}`;
});
const connectionLabel = computed(() => {
  switch (props.connectionState) {
    case "connected": return "流已连";
    case "connecting": return "连接中";
    case "error": return "流异常";
    case "disconnected": return "流中断";
    case "unsupported": return "无推送";
    default: return "流空闲";
  }
});
const transportLabel = computed(() => {
  if (props.fromCache) return "缓存";
  switch (props.transportMode?.trim().toLowerCase()) {
    case "push-stream": return "推送";
    case "snapshot-poll-fallback": return "轮询回退";
    case "idle": return "空闲";
    default: return props.connectionState === "connected" ? "推送" : "快照";
  }
});
const sessionLabel = computed(() => formatMarketSessionLabel(props.session));
const comparisonText = computed(() => {
  const primary = observedTime.value;
  const comparison = parseTimestamp(props.comparisonObservedAt);
  if (primary == null || comparison == null) return "";
  const difference = Math.abs(primary - comparison);
  return difference < 1_000
    ? `${props.comparisonLabel}同步`
    : `${props.comparisonLabel}差 ${formatAge(difference)}`;
});
const statusTitle = computed(() => [
  props.contextTitle.trim(),
  props.error?.trim() || "",
  props.providerName?.trim() ? `Provider：${props.providerName.trim()}` : "",
  props.providerCapabilities?.trim() ? `能力：${props.providerCapabilities.trim()}` : "",
  props.source?.trim() ? `来源：${props.source.trim()}` : "",
  props.observedAt?.trim() ? `更新时间：${props.observedAt.trim()}` : "",
  props.comparisonObservedAt?.trim() ? `${props.comparisonLabel}时间：${props.comparisonObservedAt.trim()}` : "",
].filter(Boolean).join("\n"));

function parseTimestamp(value: string | null | undefined): number | null {
  const parsed = Date.parse(value?.trim() ?? "");
  return Number.isFinite(parsed) ? parsed : null;
}

function formatAge(value: number): string {
  if (value < 1_000) return "刚刚";
  if (value < 60_000) return `${Math.floor(value / 1_000)}秒`;
  if (value < 3_600_000) return `${Math.floor(value / 60_000)}分`;
  return `${Math.floor(value / 3_600_000)}时`;
}

onMounted(() => {
  clock = setInterval(() => {
    now.value = Date.now();
  }, 1_000);
});

onUnmounted(() => {
  if (clock != null) clearInterval(clock);
});
</script>

<template>
  <div class="market-feed-status" :title="statusTitle">
    <MarketStatusBadge :state="badgeState" :label="freshnessLabel" />
    <span class="market-feed-status__item">{{ connectionLabel }}</span>
    <span class="market-feed-status__item">{{ transportLabel }}</span>
    <span v-if="providerName?.trim()" class="market-feed-status__item">{{ providerName.trim() }}</span>
    <span v-if="sessionLabel" class="market-feed-status__item">{{ sessionLabel }}</span>
    <span v-if="comparisonText" class="market-feed-status__item">{{ comparisonText }}</span>
  </div>
</template>

<style scoped>
.market-feed-status {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  color: var(--tv-text-dim);
  font-size: 10px;
  white-space: nowrap;
}

.market-feed-status__item + .market-feed-status__item::before {
  margin-right: 6px;
  color: var(--tv-border-strong);
  content: "·";
}

@media (max-width: 900px) {
  .market-feed-status__item {
    display: none;
  }
}
</style>
