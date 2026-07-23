<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

import { squarifiedLayout } from "./heatmapLayout";

const WEIGHT_FIELD_CANDIDATES = ["marketValue", "turnover", "volume"] as const;

const props = withDefaults(
  defineProps<{
    entries: Record<string, unknown>[];
    weightField?: string;
    height?: number;
    width?: number;
  }>(),
  { weightField: "", height: 320, width: 0 },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

const host = ref<HTMLElement | null>(null);
const measuredWidth = ref(0);
let resizeObserver: ResizeObserver | null = null;

onMounted(() => {
  measuredWidth.value = host.value?.clientWidth ?? 0;
  if (typeof ResizeObserver !== "undefined" && host.value != null) {
    resizeObserver = new ResizeObserver((observerEntries) => {
      const next = observerEntries[0]?.contentRect.width;
      if (typeof next === "number" && next > 0) measuredWidth.value = next;
    });
    resizeObserver.observe(host.value);
  }
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});

const containerWidth = computed(() =>
  props.width > 0 ? props.width : measuredWidth.value,
);

function pickNumber(
  entry: Record<string, unknown>,
  keys: readonly string[],
): number | null {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
  }
  return null;
}

interface HeatmapItem {
  entry: Record<string, unknown>;
  name: string;
  changeRate: number | null;
  value: number;
}

function resolveWeight(entry: Record<string, unknown>): number | null {
  const explicit = props.weightField.trim();
  if (explicit !== "") return pickNumber(entry, [explicit]);
  return pickNumber(entry, WEIGHT_FIELD_CANDIDATES);
}

const items = computed<HeatmapItem[]>(() =>
  props.entries
    .map((entry) => {
      const name =
        typeof entry.name === "string" && entry.name.trim() !== ""
          ? entry.name
          : typeof entry.code === "string"
            ? entry.code
            : "";
      return {
        entry,
        name,
        changeRate: pickNumber(entry, ["changeRate"]),
        value: Math.abs(resolveWeight(entry) ?? 0),
      };
    })
    .filter((item) => item.value > 0),
);

const layout = computed(() =>
  squarifiedLayout(items.value, containerWidth.value, props.height),
);

/** 涨跌幅映射着色强度：|changeRate| 到 3% 封顶 */
function blockStyle(item: HeatmapItem): Record<string, string> {
  const change = item.changeRate ?? 0;
  const intensity = Math.min(1, Math.abs(change) / 3);
  const percent = Math.round(14 + intensity * 54);
  const directionVar =
    change > 0
      ? "var(--tv-price-up)"
      : change < 0
        ? "var(--tv-price-down)"
        : "var(--tv-text-dim)";
  return {
    background: `color-mix(in srgb, ${directionVar} ${percent}%, var(--tv-bg-surface-2))`,
  };
}

function blockTextLevel(widthPx: number, heightPx: number): "full" | "name" | "none" {
  if (widthPx >= 72 && heightPx >= 40) return "full";
  if (widthPx >= 44 && heightPx >= 22) return "name";
  return "none";
}

function formatChangeRate(value: number | null): string {
  if (value == null) return "--";
  const formatted = `${Math.abs(value).toFixed(2)}%`;
  if (value > 0) return `+${formatted}`;
  if (value < 0) return `-${formatted}`;
  return formatted;
}

function tooltip(item: HeatmapItem): string {
  return `${item.name || "未命名"} ${formatChangeRate(item.changeRate)}`;
}
</script>

<template>
  <div ref="host" class="sector-heatmap" :style="{ height: `${height}px` }">
    <div v-if="items.length === 0" class="sector-heatmap__empty">暂无数据</div>
    <button
      v-for="cell in layout"
      :key="cell.index"
      type="button"
      class="sector-heatmap__block"
      :class="`sector-heatmap__block--${blockTextLevel(cell.rect.width, cell.rect.height)}`"
      :style="{
        left: `${cell.rect.x}px`,
        top: `${cell.rect.y}px`,
        width: `${cell.rect.width}px`,
        height: `${cell.rect.height}px`,
        ...blockStyle(cell.item),
      }"
      :title="tooltip(cell.item)"
      @click="emit('select', cell.item.entry)"
    >
      <template v-if="blockTextLevel(cell.rect.width, cell.rect.height) !== 'none'">
        <span class="sector-heatmap__name">{{ cell.item.name }}</span>
        <span
          v-if="blockTextLevel(cell.rect.width, cell.rect.height) === 'full'"
          class="sector-heatmap__change tv-num"
          >{{ formatChangeRate(cell.item.changeRate) }}</span
        >
      </template>
    </button>
  </div>
</template>

<style scoped>
.sector-heatmap {
  position: relative;
  width: 100%;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.sector-heatmap__empty {
  display: grid;
  height: 100%;
  place-items: center;
  color: var(--tv-text-dim);
  font-size: 12px;
}

.sector-heatmap__block {
  position: absolute;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  box-sizing: border-box;
  padding: 2px 6px;
  overflow: hidden;
  border: 1px solid var(--tv-bg-app);
  border-radius: 2px;
  color: var(--tv-text);
  cursor: pointer;
  font-size: 12px;
  line-height: 1.3;
  text-align: left;
}

.sector-heatmap__block:hover {
  filter: brightness(1.15);
}

.sector-heatmap__name {
  max-width: 100%;
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sector-heatmap__change {
  font-size: 11px;
  opacity: 0.9;
}

.sector-heatmap__block--name .sector-heatmap__name {
  font-size: 10px;
  font-weight: 500;
}
</style>
