<script setup lang="ts">
interface DenseMetricItem {
  key?: string;
  label: string;
  value: string;
  tone?: "default" | "positive" | "negative" | "warning";
  title?: string;
}

withDefaults(defineProps<{
  items: DenseMetricItem[];
  emptyLabel?: string;
  minItemWidth?: string;
}>(), {
  emptyLabel: "暂无指标",
  minItemWidth: "80px",
});
</script>

<template>
  <div
    v-if="items.length > 0"
    class="dense-metric-strip"
    :style="{ '--dense-metric-min-width': minItemWidth }"
  >
    <div v-for="item in items" :key="item.key ?? item.label" class="dense-metric-strip__item">
      <div class="dense-metric-strip__label">{{ item.label }}</div>
      <div
        class="dense-metric-strip__value"
        :class="item.tone ? `dense-metric-strip__value--${item.tone}` : undefined"
        :title="item.title ?? item.value"
      >
        {{ item.value }}
      </div>
    </div>
  </div>
  <div v-else class="dense-metric-strip__empty">{{ emptyLabel }}</div>
</template>

<style scoped>
.dense-metric-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(var(--dense-metric-min-width), 1fr));
  gap: 8px;
  min-width: 0;
}

.dense-metric-strip__item {
  min-width: 0;
}

.dense-metric-strip__label {
  color: var(--tv-text-dim);
  font-size: 11px;
}

.dense-metric-strip__value {
  margin-top: 4px;
  overflow: hidden;
  color: var(--tv-text);
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dense-metric-strip__value--positive {
  color: var(--tv-price-up);
}

.dense-metric-strip__value--negative {
  color: var(--tv-price-down);
}

.dense-metric-strip__value--warning {
  color: var(--card-amber-text);
}

.dense-metric-strip__empty {
  color: var(--tv-text-dim);
  font-size: 12px;
}
</style>
