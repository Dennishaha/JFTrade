<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import {
  resolveMarketDataFeedPresentation,
  resolveMarketDataFeedQuality,
} from "@/composables/market-data/marketDataFeedQuality";
import type { LiveSocketConnectionState } from "@/composables/market-data/sharedLiveSocket";
import MarketStatusBadge from "./MarketStatusBadge.vue";

const props = withDefaults(defineProps<{
  connectionState: LiveSocketConnectionState;
  observedAt?: string | null;
  transportMode?: string | null;
  source?: string | null;
  providerName?: string | null;
  fromCache?: boolean;
  loading?: boolean;
  error?: string | null;
}>(), {
  observedAt: null,
  transportMode: null,
  source: null,
  providerName: null,
  fromCache: false,
  loading: false,
  error: null,
});

type FeedIssue = {
  kind: "error" | "stale" | "unavailable" | "cache" | "degraded" | "empty";
  state: "error" | "stale";
  label: string;
  detail: string;
};

const staleAfterMs = 30_000;
const now = ref(Date.now());
let clock: ReturnType<typeof setInterval> | null = null;

const observedTime = computed(() => parseTimestamp(props.observedAt));
const ageMs = computed(() => {
  if (observedTime.value == null) return null;
  return Math.max(0, now.value - observedTime.value);
});
const effectiveTransportMode = computed(() => {
  const source = props.source?.trim().toLowerCase() ?? "";
  if (source === "yfinance" || source === "yahoo-finance") {
    return "snapshot-poll-delayed";
  }
  return props.transportMode;
});
const providerLabel = computed(() => {
  const explicit = props.providerName?.trim();
  if (explicit) return explicit;
  const source = props.source?.trim().toLowerCase() ?? "";
  if (source === "yfinance" || source === "yahoo-finance") {
    return "Yahoo";
  }
  if (
    source === "futu" ||
    source === "futu-opend" ||
    source.startsWith("futu:") ||
    source.startsWith("bbgo:futu")
  ) {
    return "Futu OpenD";
  }
  return "";
});
const feedQualityInput = computed(() => ({
  connectionState: props.connectionState,
  transportMode: effectiveTransportMode.value,
  fromCache: props.fromCache,
  hasUsableData: observedTime.value != null,
  error: props.error,
}));
const feedQuality = computed(() =>
  resolveMarketDataFeedQuality(feedQualityInput.value),
);
const feedPresentation = computed(() =>
  resolveMarketDataFeedPresentation(feedQualityInput.value),
);
const feedQualityLabel = computed(() => feedPresentation.value.qualityLabel);
const expectedDelayedSnapshot = computed(
  () =>
    effectiveTransportMode.value?.trim().toLowerCase() ===
    "snapshot-poll-delayed",
);
const feedIssueLabel = computed(() => {
  const transportMode = effectiveTransportMode.value?.trim().toLowerCase() ?? "";
  if (transportMode === "snapshot-poll-fallback") return "推送回退";
  switch (props.connectionState) {
    case "disconnected":
      return "实时连接中断";
    case "error":
      return "实时连接异常";
    case "unsupported":
      return "不支持实时推送";
    default:
      return "查询受限";
  }
});
const issue = computed<FeedIssue | null>(() => {
  const error = props.error?.trim() ?? "";
  if (error !== "") {
    return { kind: "error", state: "error", label: "行情异常", detail: error };
  }
  if (props.loading || props.connectionState === "connecting") {
    return null;
  }
  if (
    observedTime.value == null &&
    (props.connectionState === "disconnected" || props.connectionState === "error")
  ) {
    return {
      kind: "unavailable",
      state: "error",
      label: "行情不可用",
      detail: "行情连接不可用，且没有可显示的数据",
    };
  }
  if (props.fromCache) {
    return {
      kind: "cache",
      state: "stale",
      label: "缓存行情",
      detail: "当前显示缓存数据",
    };
  }
  if (
    ageMs.value != null &&
    ageMs.value > staleAfterMs &&
    !expectedDelayedSnapshot.value
  ) {
    return {
      kind: "stale",
      state: "stale",
      label: "数据陈旧",
      detail: `行情已 ${formatAge(ageMs.value)} 未更新`,
    };
  }
  if (observedTime.value == null) {
    return {
      kind: "empty",
      state: "stale",
      label: "暂无行情数据",
      detail: "当前栏位还没有可显示的行情数据",
    };
  }
  if (feedQuality.value === "degraded" && !feedPresentation.value.expected) {
    const isPushFallback =
      effectiveTransportMode.value?.trim().toLowerCase() ===
      "snapshot-poll-fallback";
    return {
      kind: "degraded",
      state: "stale",
      label: isPushFallback ? "推送回退" : feedIssueLabel.value,
      detail: feedQualityLabel.value,
    };
  }
  return null;
});
const issueTitle = computed(() => {
  const current = issue.value;
  if (current == null) return "";
  return [
    current.detail,
    providerLabel.value ? `供应商：${providerLabel.value}` : "",
    `连接方式：${feedPresentation.value.connectionLabel}`,
    `数据质量：${feedPresentation.value.qualityLabel}`,
    props.source?.trim() ? `来源：${props.source.trim()}` : "",
    props.observedAt?.trim() ? `更新时间：${props.observedAt.trim()}` : "",
  ].filter(Boolean).join("\n");
});
const normalTitle = computed(() => [
  providerLabel.value ? `供应商：${providerLabel.value}` : "",
  `连接方式：${feedPresentation.value.connectionLabel}`,
  `数据质量：${feedPresentation.value.qualityLabel}`,
  props.source?.trim() ? `来源：${props.source.trim()}` : "",
  props.observedAt?.trim() ? `更新时间：${props.observedAt.trim()}` : "",
].filter(Boolean).join("\n"));

const issueLabel = computed(() => {
  const current = issue.value;
  if (current == null || !providerLabel.value) return current?.label ?? "";
  return `${providerLabel.value} · ${current.label}`;
});

function parseTimestamp(value: string | null | undefined): number | null {
  const parsed = Date.parse(value?.trim() ?? "");
  return Number.isFinite(parsed) ? parsed : null;
}

function formatAge(value: number): string {
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
  <MarketStatusBadge
    v-if="issue"
    class="market-feed-issue-badge"
    :state="issue.state"
    :label="issueLabel"
    :data-quality="feedQuality"
    :data-issue="issue.kind"
    :title="issueTitle"
    :aria-label="issueTitle"
  />
  <MarketStatusBadge
    v-else-if="providerLabel && feedPresentation.state === 'live' && observedTime != null"
    class="market-feed-provider-badge"
    state="live"
    :label="providerLabel"
    :data-quality="feedQuality"
    data-issue="none"
    :title="normalTitle"
    :aria-label="normalTitle"
  />
</template>

<style scoped>
.market-feed-issue-badge {
  max-width: 148px;
}
</style>
