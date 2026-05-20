<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import {
  KLINE_INDICATORS,
  normalizeKlineIndicators,
  type KlineCandle,
  type KlineChartAdapter,
  type KlineIndicatorKey,
} from "../charting/kline";
import { lightweightChartsKlineFactory } from "../charting/lightweightChartsKline";
import { useTheme } from "../composables/useTheme";

const props = withDefaults(
  defineProps<{
    candles: readonly KlineCandle[];
    minHeight?: number;
    emptyText?: string;
    showIndicatorSelector?: boolean;
    indicatorStorageKey?: string;
    defaultIndicators?: readonly KlineIndicatorKey[];
  }>(),
  {
    minHeight: 220,
    emptyText: "暂无 K 线数据",
    showIndicatorSelector: false,
    defaultIndicators: () => ["volume"] as KlineIndicatorKey[],
  },
);
const emit = defineEmits<{
  "load-more": [];
}>();

const shell = ref<HTMLElement | null>(null);
const host = ref<HTMLElement | null>(null);
const chartError = ref("");
const { theme } = useTheme();
const availableIndicators = KLINE_INDICATORS;
const selectedIndicators = ref<KlineIndicatorKey[]>(
  normalizeKlineIndicators(props.defaultIndicators),
);

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
        indicatorA: "#2563eb",
        indicatorB: "#f59e0b",
        indicatorC: "#8b5cf6",
        macdPositive: "rgba(22, 199, 132, 0.65)",
        macdNegative: "rgba(234, 57, 67, 0.65)",
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
        indicatorA: "#60a5fa",
        indicatorB: "#fbbf24",
        indicatorC: "#c084fc",
        macdPositive: "rgba(22, 199, 132, 0.72)",
        macdNegative: "rgba(248, 113, 113, 0.72)",
      },
);

// Each indicator gets its own dedicated pane.  Keep this in sync with
// INDICATOR_PANE_HEIGHT in lightweightChartsKline.ts.
const INDICATOR_PANE_HEIGHT = 120;
const chartShellHeight = computed(() => {
  const nIndicators = props.showIndicatorSelector
    ? selectedIndicators.value.length
    : selectedIndicators.value.length; // always allocate space even without selector
  return props.minHeight + nIndicators * INDICATOR_PANE_HEIGHT;
});

function readStoredIndicators(): KlineIndicatorKey[] | null {
  if (
    props.indicatorStorageKey == null ||
    props.indicatorStorageKey.trim() === "" ||
    typeof window === "undefined" ||
    window.localStorage == null
  ) {
    return null;
  }

  try {
    const raw = window.localStorage.getItem(props.indicatorStorageKey);
    if (raw == null || raw.trim() === "") {
      return null;
    }

    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return null;
    }

    return normalizeKlineIndicators(parsed);
  } catch {
    return null;
  }
}

function persistIndicators(next: readonly KlineIndicatorKey[]): void {
  if (
    props.indicatorStorageKey == null ||
    props.indicatorStorageKey.trim() === "" ||
    typeof window === "undefined" ||
    window.localStorage == null
  ) {
    return;
  }

  window.localStorage.setItem(
    props.indicatorStorageKey,
    JSON.stringify(next),
  );
}

function toggleIndicator(indicator: KlineIndicatorKey): void {
  const exists = selectedIndicators.value.includes(indicator);
  selectedIndicators.value = normalizeKlineIndicators(
    exists
      ? selectedIndicators.value.filter((value) => value !== indicator)
      : [...selectedIndicators.value, indicator],
  );
}

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

  const storedIndicators = readStoredIndicators();
  if (storedIndicators != null) {
    selectedIndicators.value = storedIndicators;
  }

  if (typeof ResizeObserver === "undefined") {
    chartError.value = "K-line chart requires browser ResizeObserver support.";
    return;
  }

  try {
    adapter = lightweightChartsKlineFactory.create(host.value, {
      palette: palette.value,
      indicators: selectedIndicators.value,
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
watch(
  selectedIndicators,
  (next) => {
    persistIndicators(next);
    adapter?.setIndicators(next);
    scheduleChartSync();
  },
  { deep: true },
);
watch(palette, (next) => {
  adapter?.applyPalette(next);
  scheduleChartSync();
});
</script>

<template>
  <div
    ref="shell"
    class="kline-chart-shell"
    :style="{ height: `${chartShellHeight}px`, minHeight: `${chartShellHeight}px` }"
  >
    <div v-if="showIndicatorSelector" class="kline-chart-toolbar">
      <button
        v-for="indicator in availableIndicators"
        :key="indicator.value"
        class="kline-chart-chip"
        :class="{ 'is-active': selectedIndicators.includes(indicator.value) }"
        type="button"
        @click="toggleIndicator(indicator.value)"
      >
        {{ indicator.label }}
      </button>
    </div>
    <div ref="host" class="kline-chart-host"></div>
    <div v-if="chartError" class="kline-chart-overlay is-error">
      {{ chartError }}
    </div>
    <div v-else-if="candles.length === 0" class="kline-chart-overlay">
      {{ emptyText }}
    </div>
  </div>
</template>

<style scoped>
.kline-chart-shell {
  position: relative;
}

.kline-chart-host {
  height: 100%;
}

.kline-chart-toolbar {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 2;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.kline-chart-chip {
  border: 1px solid rgba(148, 163, 184, 0.3);
  background: rgba(15, 23, 42, 0.72);
  color: #e2e8f0;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
  transition: all 160ms ease;
}

.kline-chart-chip.is-active {
  border-color: rgba(13, 148, 136, 0.72);
  background: rgba(13, 148, 136, 0.18);
  color: #f8fafc;
}

.kline-chart-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  text-align: center;
}

.kline-chart-overlay.is-error {
  color: #dc2626;
}
</style>
