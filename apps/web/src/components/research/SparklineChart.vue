<script setup lang="ts">
import { computed, useId } from "vue";

type SparklineDirection = "up" | "down" | "flat";

const props = withDefaults(
  defineProps<{
    points: number[];
    width?: number;
    height?: number;
    direction?: SparklineDirection;
  }>(),
  { width: 120, height: 48, direction: "flat" },
);

const gradientId = `sparkline-gradient-${useId()}`;

const validPoints = computed(() =>
  props.points.filter((point) => Number.isFinite(point)),
);

const geometry = computed(() => {
  const points = validPoints.value;
  const { width, height } = props;
  if (points.length < 2 || width <= 0 || height <= 0) return null;
  const min = Math.min(...points);
  const max = Math.max(...points);
  const range = max - min;
  const padding = 2;
  const usableWidth = width - padding * 2;
  const usableHeight = height - padding * 2;
  const coords = points.map((point, index) => {
    const x = padding + (index / (points.length - 1)) * usableWidth;
    // 数值全相同时画一条水平中线
    const ratio = range === 0 ? 0.5 : (point - min) / range;
    const y = padding + (1 - ratio) * usableHeight;
    return [x, y] as const;
  });
  const linePath = coords
    .map(
      ([x, y], index) =>
        `${index === 0 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)}`,
    )
    .join(" ");
  const first = coords[0]!;
  const last = coords[coords.length - 1]!;
  const areaPath = `${linePath} L${last[0].toFixed(2)} ${height} L${first[0].toFixed(2)} ${height} Z`;
  return { linePath, areaPath };
});
</script>

<template>
  <svg
    class="sparkline-chart"
    :class="`sparkline-chart--${direction}`"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    role="img"
    aria-hidden="true"
  >
    <template v-if="geometry != null">
      <defs>
        <linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" class="sparkline-chart__stop--from" />
          <stop offset="100%" class="sparkline-chart__stop--to" />
        </linearGradient>
      </defs>
      <path
        class="sparkline-chart__area"
        :d="geometry.areaPath"
        :fill="`url(#${gradientId})`"
      />
      <path
        class="sparkline-chart__line"
        :d="geometry.linePath"
        fill="none"
        stroke-width="1.5"
        stroke-linejoin="round"
        stroke-linecap="round"
      />
    </template>
    <rect
      v-else
      class="sparkline-chart__placeholder"
      x="0"
      y="0"
      :width="width"
      :height="height"
      rx="4"
    />
  </svg>
</template>

<style scoped>
.sparkline-chart {
  display: block;
}

.sparkline-chart--up .sparkline-chart__line {
  stroke: var(--tv-price-up);
}
.sparkline-chart--down .sparkline-chart__line {
  stroke: var(--tv-price-down);
}
.sparkline-chart--flat .sparkline-chart__line {
  stroke: var(--tv-text-dim);
}

.sparkline-chart--up .sparkline-chart__stop--from {
  stop-color: var(--tv-price-up);
  stop-opacity: 0.32;
}
.sparkline-chart--up .sparkline-chart__stop--to {
  stop-color: var(--tv-price-up);
  stop-opacity: 0;
}
.sparkline-chart--down .sparkline-chart__stop--from {
  stop-color: var(--tv-price-down);
  stop-opacity: 0.32;
}
.sparkline-chart--down .sparkline-chart__stop--to {
  stop-color: var(--tv-price-down);
  stop-opacity: 0;
}
.sparkline-chart--flat .sparkline-chart__stop--from {
  stop-color: var(--tv-text-dim);
  stop-opacity: 0.2;
}
.sparkline-chart--flat .sparkline-chart__stop--to {
  stop-color: var(--tv-text-dim);
  stop-opacity: 0;
}

.sparkline-chart__placeholder {
  fill: var(--tv-bg-surface-2);
  stroke: var(--tv-border);
  stroke-width: 1;
  stroke-dasharray: 3 3;
}
</style>
