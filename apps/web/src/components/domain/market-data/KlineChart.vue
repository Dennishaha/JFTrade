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
  isKlinePaneIndicator,
  transformKlineCandles,
  normalizeKlineIndicators,
  type ChartType,
  type KlineCandle,
  type KlineChartAdapter,
  type KlineIndicatorKey,
} from "@/charting/kline";
import KlineIndicatorSelector from "@/components/domain/market-data/KlineIndicatorSelector.vue";
import { lightweightChartsKlineFactory } from "@/charting/lightweightChartsKline";
import {
  hexToRgba,
  resolveDirectionalColors,
  tryUseUIColorPreferences,
} from "@/composables/settings/useUIColorPreferences";
import { useTheme } from "@/composables/settings/useTheme";

const props = withDefaults(
  defineProps<{
    candles: readonly KlineCandle[];
    minHeight?: number;
    emptyText?: string;
    chartType?: ChartType | string;
    showIndicatorSelector?: boolean;
    indicatorStorageKey?: string;
    defaultIndicators?: readonly KlineIndicatorKey[];
    indicators?: readonly KlineIndicatorKey[];
  }>(),
  {
    minHeight: 220,
    emptyText: "暂无 K 线数据",
    chartType: "standard",
    showIndicatorSelector: false,
    defaultIndicators: () => ["volume"] as KlineIndicatorKey[],
  },
);
const emit = defineEmits<{
  "load-more": [];
  "update:indicators": [indicators: KlineIndicatorKey[]];
}>();

const shell = ref<HTMLElement | null>(null);
const host = ref<HTMLElement | null>(null);
const chartError = ref("");
const { theme } = useTheme();
const uiColorPreferences = tryUseUIColorPreferences();
const selectedIndicators = ref<KlineIndicatorKey[]>(
  normalizeKlineIndicators(props.defaultIndicators),
);
const activeIndicators = computed(() =>
  normalizeKlineIndicators(props.indicators ?? selectedIndicators.value),
);
const displayCandles = computed(() =>
  transformKlineCandles(props.candles, props.chartType),
);

let adapter: KlineChartAdapter | null = null;
let resizeObserver: ResizeObserver | null = null;
let scheduledFrame: number | null = null;
let resizeSettleTimer: number | null = null;
let loadMoreScheduled = false;

const RESIZE_SETTLE_DELAY_MS = 80;

const directionalColors = computed(() =>
  uiColorPreferences?.resolved.value ?? resolveDirectionalColors(theme.value, false),
);

const palette = computed(() =>
  theme.value === "light"
    ? {
        bg: "#ffffff",
        text: "#0f172a",
        grid: "rgba(15, 23, 42, 0.06)",
        border: "rgba(15, 23, 42, 0.12)",
        up: directionalColors.value.upColor,
        down: directionalColors.value.downColor,
        volumeUp: hexToRgba(directionalColors.value.upColor, 0.45),
        volumeDown: hexToRgba(directionalColors.value.downColor, 0.45),
        indicatorA: "#2563eb",
        indicatorB: "#f59e0b",
        indicatorC: "#8b5cf6",
        macdPositive: hexToRgba(directionalColors.value.upColor, 0.65),
        macdNegative: hexToRgba(directionalColors.value.downColor, 0.65),
      }
    : {
        bg: "#1a1a1a",
        text: "#cbd5e1",
        grid: "rgba(148, 163, 184, 0.08)",
        border: "rgba(148, 163, 184, 0.16)",
        up: directionalColors.value.upColor,
        down: directionalColors.value.downColor,
        volumeUp: hexToRgba(directionalColors.value.upColor, 0.45),
        volumeDown: hexToRgba(directionalColors.value.downColor, 0.45),
        indicatorA: "#60a5fa",
        indicatorB: "#fbbf24",
        indicatorC: "#c084fc",
        macdPositive: hexToRgba(directionalColors.value.upColor, 0.72),
        macdNegative: hexToRgba(directionalColors.value.downColor, 0.72),
      },
);

// Each indicator gets its own dedicated pane.  Keep this in sync with
// INDICATOR_PANE_HEIGHT in lightweightChartsKline.ts.
const INDICATOR_PANE_HEIGHT = 120;
const paneIndicatorCount = computed(() =>
  activeIndicators.value.filter(isKlinePaneIndicator).length,
);
const chartShellHeight = computed(() => {
  return props.minHeight + paneIndicatorCount.value * INDICATOR_PANE_HEIGHT;
});

function setSelectedIndicators(indicators: readonly KlineIndicatorKey[]): void {
  const normalized = normalizeKlineIndicators(indicators);
  if (props.indicators != null) {
    emit("update:indicators", normalized);
    return;
  }
  selectedIndicators.value = normalized;
}

function refreshChartData(): void {
  adapter?.setCandles(displayCandles.value);
  scheduleChartSync();
}

function firstPositiveDimension(
  ...values: Array<number | null | undefined>
): number | null {
  const value = values.find(
    (candidate) =>
      typeof candidate === "number" &&
      Number.isFinite(candidate) &&
      candidate > 0,
  );
  return value == null ? null : Math.floor(value);
}

function measureChartSize(): { width: number; height: number } | null {
  const target = host.value ?? shell.value;
  const fallbackTarget = target === host.value ? shell.value : host.value;
  const rect = target?.getBoundingClientRect();
  const fallbackRect = fallbackTarget?.getBoundingClientRect();
  const measuredWidth = firstPositiveDimension(
    rect?.width,
    target?.clientWidth,
    fallbackRect?.width,
    fallbackTarget?.clientWidth,
  );
  const measuredHeight = firstPositiveDimension(
    rect?.height,
    target?.clientHeight,
    fallbackRect?.height,
    fallbackTarget?.clientHeight,
  );

  if (measuredWidth == null || measuredHeight == null) {
    return null;
  }

  return {
    width: measuredWidth,
    height: measuredHeight,
  };
}

function syncChartSize(): void {
  if (adapter == null) {
    return;
  }

  const size = measureChartSize();
  if (size == null) {
    return;
  }
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

function scheduleChartLayoutSync(): void {
  scheduleChartSync();
  if (typeof window === "undefined") {
    return;
  }

  if (resizeSettleTimer != null) {
    window.clearTimeout(resizeSettleTimer);
  }
  resizeSettleTimer = window.setTimeout(() => {
    resizeSettleTimer = null;
    scheduleChartSync();
  }, RESIZE_SETTLE_DELAY_MS);
}

onMounted(async () => {
  if (host.value == null) {
    return;
  }

  window.addEventListener("resize", scheduleChartLayoutSync);

  await nextTick();

  if (typeof ResizeObserver === "undefined") {
    chartError.value = "K-line chart requires browser ResizeObserver support.";
    return;
  }

  try {
    adapter = lightweightChartsKlineFactory.create(host.value, {
      palette: palette.value,
      indicators: activeIndicators.value,
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
        : "K 线图初始化失败。";
    return;
  }

  resizeObserver = new ResizeObserver(() => {
    scheduleChartLayoutSync();
  });
  if (shell.value != null) {
    resizeObserver.observe(shell.value);
  }
  if (host.value != null && host.value !== shell.value) {
    resizeObserver.observe(host.value);
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", scheduleChartLayoutSync);
  if (scheduledFrame != null && typeof window !== "undefined") {
    window.cancelAnimationFrame(scheduledFrame);
    scheduledFrame = null;
  }
  if (resizeSettleTimer != null && typeof window !== "undefined") {
    window.clearTimeout(resizeSettleTimer);
    resizeSettleTimer = null;
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
  adapter?.remove();
  adapter = null;
});

watch(displayCandles, refreshChartData, { deep: true });
watch(
  activeIndicators,
  (indicators) => {
    adapter?.setIndicators(indicators);
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
    :style="{ '--kline-min-h': `${chartShellHeight}px` }"
  >
    <div v-if="showIndicatorSelector" class="kline-chart-toolbar">
      <KlineIndicatorSelector
        :model-value="activeIndicators"
        :storage-key="indicatorStorageKey"
        :default-indicators="defaultIndicators"
        @update:model-value="setSelectedIndicators"
      />
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
  width: 100%;
  min-width: 0;
  min-height: var(--kline-min-h, 220px);
}

.kline-chart-host {
  position: absolute;
  inset: 0;
}

.kline-chart-toolbar {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 100;
  display: flex;
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
  color: var(--jf-accent-red-strong);
}
</style>
