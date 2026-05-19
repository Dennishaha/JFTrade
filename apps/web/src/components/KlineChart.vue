<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import type { KlineCandle, KlineChartAdapter } from "../charting/kline";
import { lightweightChartsKlineFactory } from "../charting/lightweightChartsKline";
import { useTheme } from "../composables/useTheme";

const props = withDefaults(
  defineProps<{
    candles: readonly KlineCandle[];
    minHeight?: number;
    emptyText?: string;
  }>(),
  {
    minHeight: 220,
    emptyText: "暂无 K 线数据",
  },
);
const emit = defineEmits<{
  "load-more": [];
}>();

const shell = ref<HTMLElement | null>(null);
const host = ref<HTMLElement | null>(null);
const chartError = ref("");
const { theme } = useTheme();

let adapter: KlineChartAdapter | null = null;
let resizeObserver: ResizeObserver | null = null;
let scheduledFrame: number | null = null;
let loadMoreScheduled = false;

const palette = computed(() =>
  theme.value === "light"
    ? {
        bg: "#ffffff",
        text: "#0f172a",
        grid: "rgba(15, 23, 42, 0.06)",
        border: "rgba(15, 23, 42, 0.12)",
        up: "#16c784",
        down: "#ea3943",
        volumeUp: "rgba(22, 199, 132, 0.45)",
        volumeDown: "rgba(234, 57, 67, 0.45)",
      }
    : {
        bg: "#0f172a",
        text: "#cbd5e1",
        grid: "rgba(148, 163, 184, 0.08)",
        border: "rgba(148, 163, 184, 0.16)",
        up: "#16c784",
        down: "#ea3943",
        volumeUp: "rgba(22, 199, 132, 0.45)",
        volumeDown: "rgba(234, 57, 67, 0.45)",
      },
);

function refreshChartData(): void {
  adapter?.setCandles(props.candles);
  scheduleChartSync();
}

function measureChartSize(): { width: number; height: number } {
  const target = shell.value ?? host.value;
  const rect = target?.getBoundingClientRect();
  return {
    width: Math.max(1, Math.floor(rect?.width ?? target?.clientWidth ?? 1)),
    height: Math.max(
      1,
      Math.floor(rect?.height ?? target?.clientHeight ?? props.minHeight),
    ),
  };
}

function syncChartSize(): void {
  if (adapter == null) {
    return;
  }

  const size = measureChartSize();
  adapter.resize(size.width, size.height);
}

function scheduleChartSync(): void {
  if (typeof window === "undefined") {
    return;
  }

  if (scheduledFrame != null) {
    window.cancelAnimationFrame(scheduledFrame);
  }

  scheduledFrame = window.requestAnimationFrame(() => {
    scheduledFrame = null;
    syncChartSize();
  });
}

onMounted(async () => {
  if (host.value == null) {
    return;
  }

  await nextTick();

  if (typeof ResizeObserver === "undefined") {
    chartError.value = "K-line chart requires browser ResizeObserver support.";
    return;
  }

  try {
    adapter = lightweightChartsKlineFactory.create(host.value, {
      palette: palette.value,
    });
    adapter.setLoadMoreHandler(() => {
      if (loadMoreScheduled) {
        return;
      }

      loadMoreScheduled = true;
      window.setTimeout(() => {
        loadMoreScheduled = false;
      }, 1000);
      emit("load-more");
    });
    chartError.value = "";
    refreshChartData();
    scheduleChartSync();
  } catch (error) {
    chartError.value =
      error instanceof Error
        ? error.message
        : "Failed to initialize K-line chart.";
    return;
  }

  if (shell.value != null) {
    resizeObserver = new ResizeObserver(() => {
      scheduleChartSync();
    });
    resizeObserver.observe(shell.value);
  }
});

onBeforeUnmount(() => {
  if (scheduledFrame != null && typeof window !== "undefined") {
    window.cancelAnimationFrame(scheduledFrame);
    scheduledFrame = null;
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
  adapter?.remove();
  adapter = null;
});

watch(() => props.candles, refreshChartData, { deep: true });
watch(palette, (next) => {
  adapter?.applyPalette(next);
  scheduleChartSync();
});
</script>

<template>
  <div
    ref="shell"
    class="kline-chart-shell"
    :style="{ height: `${minHeight}px`, minHeight: `${minHeight}px` }"
  >
    <div ref="host" class="kline-chart-host"></div>
    <div v-if="chartError" class="kline-chart-overlay is-error">
      {{ chartError }}
    </div>
    <div v-else-if="candles.length === 0" class="kline-chart-overlay">
      {{ emptyText }}
    </div>
  </div>
</template>
