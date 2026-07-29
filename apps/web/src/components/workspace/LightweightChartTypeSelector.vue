<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";

import { KLINE_CHART_TYPES, type ChartType } from "../../charting/kline";

const props = defineProps<{
  activeChartType: ChartType;
  activeChartTypeLabel: string;
  tickPeriod: boolean;
}>();

const emit = defineEmits<{
  select: [chartType: ChartType];
}>();

const triggerRef = ref<HTMLElement | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const optionRefs = ref<Array<HTMLButtonElement | null>>([]);
const open = ref(false);
const panelTop = ref(0);
const panelLeft = ref(0);
const PANEL_GAP = 4;
const VIEWPORT_GAP = 8;

function syncPanelPosition(): void {
  const trigger = triggerRef.value;
  const panel = panelRef.value;
  if (trigger == null || panel == null || typeof window === "undefined") return;

  const triggerRect = trigger.getBoundingClientRect();
  const panelRect = panel.getBoundingClientRect();
  const maxLeft = Math.max(VIEWPORT_GAP, window.innerWidth - panelRect.width - VIEWPORT_GAP);
  panelLeft.value = Math.min(Math.max(triggerRect.left, VIEWPORT_GAP), maxLeft);

  const below = triggerRect.bottom + PANEL_GAP;
  const above = triggerRect.top - panelRect.height - PANEL_GAP;
  const maxTop = Math.max(VIEWPORT_GAP, window.innerHeight - panelRect.height - VIEWPORT_GAP);
  panelTop.value = below + panelRect.height <= window.innerHeight - VIEWPORT_GAP
    ? below
    : above >= VIEWPORT_GAP
      ? above
      : Math.min(Math.max(below, VIEWPORT_GAP), maxTop);
}

function enabledOptions(): HTMLButtonElement[] {
  return optionRefs.value.filter(
    (option): option is HTMLButtonElement => option != null && !option.disabled,
  );
}

function focusOption(chartType: ChartType): void {
  const options = enabledOptions();
  const selected = options.find((option) => option.dataset.chartType === chartType);
  (selected ?? options[0])?.focus();
}

async function toggle(): Promise<void> {
  open.value = !open.value;
  if (!open.value) return;
  await nextTick();
  syncPanelPosition();
  focusOption(props.activeChartType);
}

function close(options?: { restoreTriggerFocus?: boolean }): void {
  open.value = false;
  if (options?.restoreTriggerFocus) triggerRef.value?.focus();
}

function setOptionRef(index: number, element: unknown): void {
  optionRefs.value[index] = element instanceof HTMLButtonElement ? element : null;
}

function select(chartType: ChartType): void {
  if (chartType === "heikinashi" && props.tickPeriod) return;
  emit("select", chartType);
  close({ restoreTriggerFocus: true });
}

function handleDocumentPointerDown(event: PointerEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (
    !(triggerRef.value?.contains(target) ?? false) &&
    !(panelRef.value?.contains(target) ?? false)
  ) {
    close();
  }
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (!open.value) return;
  if (event.key === "Escape") {
    event.preventDefault();
    close({ restoreTriggerFocus: true });
    return;
  }
  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;

  const options = enabledOptions();
  if (options.length === 0) return;
  event.preventDefault();
  const activeIndex = options.findIndex((option) => option === document.activeElement);
  let nextIndex = 0;
  if (event.key === "ArrowDown") {
    nextIndex = activeIndex < 0 ? 0 : (activeIndex + 1) % options.length;
  } else if (event.key === "ArrowUp") {
    nextIndex = activeIndex < 0 ? options.length - 1 : (activeIndex - 1 + options.length) % options.length;
  } else if (event.key === "End") {
    nextIndex = options.length - 1;
  }
  options[nextIndex]?.focus();
}

onMounted(() => {
  document.addEventListener("pointerdown", handleDocumentPointerDown);
  document.addEventListener("keydown", handleDocumentKeydown);
  window.addEventListener("resize", syncPanelPosition);
  window.addEventListener("scroll", syncPanelPosition, true);
});

onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", handleDocumentPointerDown);
  document.removeEventListener("keydown", handleDocumentKeydown);
  window.removeEventListener("resize", syncPanelPosition);
  window.removeEventListener("scroll", syncPanelPosition, true);
});
</script>

<template>
  <div class="kline-chart-type-selector">
    <button
      ref="triggerRef"
      class="kline-chart-type-selector__trigger"
      type="button"
      :class="{ 'is-open': open }"
      :title="`图表类型：${activeChartTypeLabel}`"
      aria-label="选择图表类型"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="toggle"
    >
      <span class="fa-solid fa-chart-column" aria-hidden="true" />
    </button>
    <Teleport to="body">
      <div
        v-if="open"
        ref="panelRef"
        class="kline-chart-type-selector__panel"
        role="menu"
        aria-label="图表类型"
        :style="{ top: `${panelTop}px`, left: `${panelLeft}px` }"
      >
        <button
          v-for="(option, index) in KLINE_CHART_TYPES"
          :key="option.value"
          :ref="(element) => setOptionRef(index, element)"
          class="kline-chart-type-selector__option"
          type="button"
          role="menuitemradio"
          :data-chart-type="option.value"
          :aria-checked="activeChartType === option.value"
          :class="{ 'is-active': activeChartType === option.value }"
          :disabled="option.value === 'heikinashi' && tickPeriod"
          :title="option.value === 'heikinashi' && tickPeriod
            ? 'Tick 周期不支持平均K线图'
            : option.label"
          @click="select(option.value)"
        >
          <span>{{ option.label }}</span>
          <span
            v-if="activeChartType === option.value"
            class="fa-solid fa-check"
            aria-hidden="true"
          />
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.kline-chart-type-selector {
  display: inline-flex;
  flex: 0 0 auto;
}

.kline-chart-type-selector__trigger {
  display: inline-grid;
  width: 26px;
  height: 26px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 12px;
  line-height: 1;
}

.kline-chart-type-selector__trigger:hover,
.kline-chart-type-selector__trigger.is-open,
.kline-chart-type-selector__trigger:focus-visible {
  border-color: var(--tv-border-strong);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-chart-type-selector__panel {
  position: fixed;
  z-index: 9999;
  display: grid;
  width: min(188px, calc(100vw - 16px));
  padding: 4px;
  border: 1px solid var(--tv-border-strong);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  box-shadow: 0 14px 36px rgb(0 0 0 / 36%);
}

.kline-chart-type-selector__option {
  display: flex;
  min-height: 30px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 8px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 12px;
  text-align: left;
}

.kline-chart-type-selector__option:hover,
.kline-chart-type-selector__option:focus-visible,
.kline-chart-type-selector__option.is-active {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-chart-type-selector__option:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
</style>
