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
  isKlinePaneIndicator,
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
const isIndicatorPanelOpen = ref(false);
const triggerRef = ref<HTMLElement | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const panelTop = ref(0);
const panelRight = ref(0);
const { theme } = useTheme();
const paneIndicators = KLINE_INDICATORS.filter(
  (indicator) => indicator.kind === "pane",
);
const maIndicators = KLINE_INDICATORS.filter(
  (indicator) => indicator.family === "ma",
);
const emaIndicators = KLINE_INDICATORS.filter(
  (indicator) => indicator.family === "ema",
);
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
const paneIndicatorCount = computed(() =>
  selectedIndicators.value.filter(isKlinePaneIndicator).length,
);
const chartShellHeight = computed(() => {
  return props.minHeight + paneIndicatorCount.value * INDICATOR_PANE_HEIGHT;
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

function toggleIndicatorPanel(): void {
  isIndicatorPanelOpen.value = !isIndicatorPanelOpen.value;
  if (isIndicatorPanelOpen.value && triggerRef.value != null && typeof window !== "undefined") {
    const rect = triggerRef.value.getBoundingClientRect();
    panelTop.value = rect.bottom + 8;
    panelRight.value = window.innerWidth - rect.right;
  }
}

function closeIndicatorPanel(): void {
  isIndicatorPanelOpen.value = false;
}

function handleDocumentPointerDown(event: PointerEvent): void {
  const shellElement = shell.value;
  const panelElement = panelRef.value;
  const target = event.target;
  if (shellElement == null || !(target instanceof Node)) {
    return;
  }

  if (!shellElement.contains(target) && !(panelElement?.contains(target) ?? false)) {
    closeIndicatorPanel();
  }
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeIndicatorPanel();
  }
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

  document.addEventListener("pointerdown", handleDocumentPointerDown);
  document.addEventListener("keydown", handleDocumentKeydown);

  await nextTick();

  const storedIndicators = readStoredIndicators();
  if (storedIndicators != null) {
    selectedIndicators.value = storedIndicators;
    await nextTick();
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
        : "K 线图初始化失败。";
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
  document.removeEventListener("pointerdown", handleDocumentPointerDown);
  document.removeEventListener("keydown", handleDocumentKeydown);
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
    :style="{ '--kline-min-h': `${chartShellHeight}px` }"
  >
    <div v-if="showIndicatorSelector" class="kline-chart-toolbar">
      <button
        ref="triggerRef"
        class="kline-chart-trigger"
        type="button"
        :class="{ 'is-open': isIndicatorPanelOpen }"
        @click="toggleIndicatorPanel"
      >
        <span>指标</span>
        <span class="kline-chart-trigger-count">{{ selectedIndicators.length }}</span>
      </button>
      <Teleport to="body">
        <div
          v-if="isIndicatorPanelOpen"
          ref="panelRef"
          class="kline-chart-panel"
          :style="{ top: `${panelTop}px`, right: `${panelRight}px` }"
        >
          <div class="kline-chart-panel-header">
            <div>
              <div class="kline-chart-panel-title">主图 / 副图</div>
              <div class="kline-chart-panel-subtitle">勾选后立即叠加到当前 K 线图</div>
            </div>
            <button
              class="kline-chart-panel-close"
              type="button"
              @click="closeIndicatorPanel"
            >
              关闭
            </button>
          </div>

          <div class="kline-chart-panel-group">
            <div class="kline-chart-panel-group-title">主图 MA</div>
            <div class="kline-chart-panel-grid">
              <label
                v-for="indicator in maIndicators"
                :key="indicator.value"
                class="kline-chart-option"
              >
                <input
                  :checked="selectedIndicators.includes(indicator.value)"
                  :value="indicator.value"
                  type="checkbox"
                  @change="toggleIndicator(indicator.value)"
                />
                <span>{{ indicator.label }}</span>
              </label>
            </div>
          </div>

          <div class="kline-chart-panel-group">
            <div class="kline-chart-panel-group-title">主图 EMA</div>
            <div class="kline-chart-panel-grid">
              <label
                v-for="indicator in emaIndicators"
                :key="indicator.value"
                class="kline-chart-option"
              >
                <input
                  :checked="selectedIndicators.includes(indicator.value)"
                  :value="indicator.value"
                  type="checkbox"
                  @change="toggleIndicator(indicator.value)"
                />
                <span>{{ indicator.label }}</span>
              </label>
            </div>
          </div>

          <div class="kline-chart-panel-group">
            <div class="kline-chart-panel-group-title">副图</div>
            <div class="kline-chart-panel-grid">
              <label
                v-for="indicator in paneIndicators"
                :key="indicator.value"
                class="kline-chart-option"
              >
                <input
                  :checked="selectedIndicators.includes(indicator.value)"
                  :value="indicator.value"
                  type="checkbox"
                  @change="toggleIndicator(indicator.value)"
                />
                <span>{{ indicator.label }}</span>
              </label>
            </div>
          </div>
        </div>
      </Teleport>
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
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
}

.kline-chart-trigger {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  border-radius: 999px;
  padding: 8px 12px;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  transition:
    background 160ms ease,
    border-color 160ms ease,
    transform 160ms ease;
}

.kline-chart-trigger:hover,
.kline-chart-trigger.is-open {
  border-color: var(--tv-accent-strong);
  background: var(--tv-bg-elevated);
  transform: translateY(-1px);
}

.kline-chart-trigger-count {
  min-width: 1.5rem;
  height: 1.5rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-text) 16%, transparent);
  color: var(--tv-text);
  font-size: 11px;
  font-weight: 600;
}

.kline-chart-panel {
  position: fixed;
  z-index: 9999;
  width: min(420px, calc(100vw - 24px));
  max-height: min(72vh, 520px);
  overflow: auto;
  border: 1px solid var(--tv-border-strong);
  border-radius: 18px;
  padding: 14px;
  background: var(--tv-bg-surface);
  box-shadow: 0 24px 64px rgba(2, 6, 23, 0.42);
  backdrop-filter: blur(18px);
}

.kline-chart-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.kline-chart-panel-title {
  color: var(--tv-text);
  font-size: 14px;
  font-weight: 600;
}

.kline-chart-panel-subtitle {
  margin-top: 4px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.kline-chart-panel-close {
  border: 1px solid var(--tv-border);
  background: color-mix(in srgb, var(--tv-text) 8%, transparent);
  color: var(--tv-text);
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 11px;
  cursor: pointer;
}

.kline-chart-panel-group + .kline-chart-panel-group {
  margin-top: 14px;
}

.kline-chart-panel-group-title {
  margin-bottom: 8px;
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.kline-chart-panel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(84px, 1fr));
  gap: 8px;
}

.kline-chart-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 8px 10px;
  color: var(--tv-text-muted);
  background: var(--tv-bg-elevated);
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
  user-select: none;
}

.kline-chart-option input {
  margin: 0;
}

.kline-chart-option:has(input:checked) {
  border-color: var(--card-teal-border);
  background: var(--card-teal-surface);
  color: var(--tv-text);
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
