<script setup lang="ts">
import { computed } from "vue";

type MarketStatusState = "live" | "stale" | "loading" | "empty" | "error" | "disabled";

const props = withDefaults(defineProps<{
  state: MarketStatusState;
  label?: string;
}>(), {
  label: "",
});

const defaultLabels: Record<MarketStatusState, string> = {
  live: "实时",
  stale: "陈旧",
  loading: "加载中",
  empty: "无数据",
  error: "异常",
  disabled: "不可用",
};

const displayLabel = computed(() => props.label || defaultLabels[props.state]);
</script>

<template>
  <span
    class="market-status-badge"
    :class="`market-status-badge--${state}`"
    :data-state="state"
    :aria-disabled="state === 'disabled' ? 'true' : undefined"
    :aria-live="state === 'loading' ? 'polite' : undefined"
  >
    <span class="market-status-badge__dot" aria-hidden="true"></span>
    <span class="market-status-badge__label">{{ displayLabel }}</span>
  </span>
</template>

<style scoped>
.market-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  max-width: 100%;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 3px 8px;
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 11px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.market-status-badge__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.market-status-badge__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.market-status-badge--live {
  color: var(--tv-accent);
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface-2));
}

.market-status-badge--loading {
  color: var(--card-amber-text);
  background: var(--card-amber-surface);
}

.market-status-badge--stale {
  color: var(--card-amber-text);
  background: var(--card-amber-surface);
}

.market-status-badge--error {
  color: var(--tv-down);
  background: color-mix(in srgb, var(--tv-down) 10%, var(--tv-bg-surface-2));
}

.market-status-badge--disabled {
  opacity: 0.65;
}
</style>
