<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

import { resolveMarketDataFeedQuality } from "../../../composables/marketDataFeedQuality";
import type { LiveSocketConnectionState } from "../../../composables/sharedLiveSocket";
import MarketStatusBadge from "./MarketStatusBadge.vue";

const props = withDefaults(defineProps<{
  connectionState: LiveSocketConnectionState;
  observedAt?: string | null;
  transportMode?: string | null;
  source?: string | null;
  fromCache?: boolean;
  loading?: boolean;
  error?: string | null;
}>(), {
  observedAt: null,
  transportMode: null,
  source: null,
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
const feedQualityInput = computed(() => ({
  connectionState: props.connectionState,
  transportMode: props.transportMode,
  fromCache: props.fromCache,
  hasUsableData: observedTime.value != null,
  error: props.error,
}));
const feedQuality = computed(() =>
  resolveMarketDataFeedQuality(feedQualityInput.value),
);
const feedQualityLabel = computed(() => {
  if (feedQuality.value === "healthy") return "实时推送正常";
  if (feedQuality.value === "unavailable") return "数据源不可用";
  if (feedQuality.value === "idle") return "等待行情订阅";
  if (props.fromCache) return "正在使用缓存数据";
  const transportMode = props.transportMode?.trim().toLowerCase() ?? "";
  if (transportMode === "snapshot-poll-fallback") return "已降级到轮询行情";
  if (props.connectionState === "unsupported") return "不支持推送，使用快照行情";
  if (
    props.connectionState === "disconnected" ||
    props.connectionState === "error"
  ) {
    return "连接不可用，显示最近一次行情";
  }
  return transportMode ? `行情传输已降级：${transportMode}` : "行情传输已降级";
});
const issue = computed<FeedIssue | null>(() => {
  const error = props.error?.trim() ?? "";
  if (error !== "") {
    return { kind: "error", state: "error", label: "行情异常", detail: error };
  }
  if (ageMs.value != null && ageMs.value > staleAfterMs) {
    return {
      kind: "stale",
      state: "stale",
      label: "数据陈旧",
      detail: `行情已 ${formatAge(ageMs.value)} 未更新`,
    };
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
  if (feedQuality.value === "degraded") {
    return {
      kind: "degraded",
      state: "stale",
      label: "行情已降级",
      detail: feedQualityLabel.value,
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
  return null;
});
const issueTitle = computed(() => {
  const current = issue.value;
  if (current == null) return "";
  return [
    current.detail,
    `数据源质量：${feedQualityLabel.value}`,
    props.source?.trim() ? `来源：${props.source.trim()}` : "",
    props.observedAt?.trim() ? `更新时间：${props.observedAt.trim()}` : "",
  ].filter(Boolean).join("\n");
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
    :label="issue.label"
    :data-quality="feedQuality"
    :data-issue="issue.kind"
    :title="issueTitle"
  />
</template>

<style scoped>
.market-feed-issue-badge {
  max-width: 148px;
}
</style>
