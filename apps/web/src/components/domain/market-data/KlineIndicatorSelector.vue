<script setup lang="ts">
import {
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import {
  KLINE_INDICATORS,
  normalizeKlineIndicators,
  type KlineIndicatorKey,
} from "@/charting/kline";
import {
  readLocalStorage,
  writeLocalStorage,
} from "@/composables/shared/safeStorage";

const props = withDefaults(
  defineProps<{
    modelValue: readonly KlineIndicatorKey[];
    storageKey?: string | undefined;
    defaultIndicators?: readonly KlineIndicatorKey[];
    disabled?: boolean;
  }>(),
  {
    defaultIndicators: () => ["volume"] as KlineIndicatorKey[],
    disabled: false,
  },
);

const emit = defineEmits<{
  "update:modelValue": [indicators: KlineIndicatorKey[]];
}>();

const triggerRef = ref<HTMLElement | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const isOpen = ref(false);
const panelTop = ref(0);
const panelLeft = ref(0);
const selectedIndicators = ref<KlineIndicatorKey[]>(
  normalizeKlineIndicators(props.modelValue ?? props.defaultIndicators),
);
const indicatorGroups = [
  {
    label: "MA",
    indicators: KLINE_INDICATORS.filter(
      (indicator) => indicator.family === "ma",
    ),
  },
  {
    label: "EMA",
    indicators: KLINE_INDICATORS.filter(
      (indicator) => indicator.family === "ema",
    ),
  },
  {
    label: "副图",
    indicators: KLINE_INDICATORS.filter(
      (indicator) => indicator.kind === "pane",
    ),
  },
] as const;

const VIEWPORT_GAP = 8;
const PANEL_GAP = 4;

function storageKey(): string | null {
  const key = props.storageKey?.trim() ?? "";
  return key === "" ? null : key;
}

function sameIndicators(
  left: readonly KlineIndicatorKey[],
  right: readonly KlineIndicatorKey[],
): boolean {
  return (
    left.length === right.length &&
    left.every((indicator, index) => right[index] === indicator)
  );
}

function readStoredIndicators(): KlineIndicatorKey[] | null {
  const key = storageKey();
  if (key == null) return null;

  try {
    const raw = readLocalStorage(key);
    if (raw == null || raw.trim() === "") return null;
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? normalizeKlineIndicators(parsed) : null;
  } catch {
    return null;
  }
}

function persistIndicators(indicators: readonly KlineIndicatorKey[]): void {
  const key = storageKey();
  if (key == null) return;
  writeLocalStorage(
    key,
    JSON.stringify(normalizeKlineIndicators(indicators)),
  );
}

function setIndicators(indicators: readonly KlineIndicatorKey[]): void {
  const normalized = normalizeKlineIndicators(indicators);
  selectedIndicators.value = normalized;
  emit("update:modelValue", normalized);
}

function toggleIndicator(indicator: KlineIndicatorKey): void {
  const exists = selectedIndicators.value.includes(indicator);
  setIndicators(
    exists
      ? selectedIndicators.value.filter((value) => value !== indicator)
      : [...selectedIndicators.value, indicator],
  );
}

function syncPanelPosition(): void {
  const trigger = triggerRef.value;
  const panel = panelRef.value;
  if (trigger == null || panel == null || typeof window === "undefined") return;

  const triggerRect = trigger.getBoundingClientRect();
  const panelRect = panel.getBoundingClientRect();
  const maxLeft = Math.max(
    VIEWPORT_GAP,
    window.innerWidth - panelRect.width - VIEWPORT_GAP,
  );
  panelLeft.value = Math.min(
    Math.max(triggerRect.left, VIEWPORT_GAP),
    maxLeft,
  );

  const below = triggerRect.bottom + PANEL_GAP;
  const above = triggerRect.top - panelRect.height - PANEL_GAP;
  const maxTop = Math.max(
    VIEWPORT_GAP,
    window.innerHeight - panelRect.height - VIEWPORT_GAP,
  );
  panelTop.value =
    below + panelRect.height <= window.innerHeight - VIEWPORT_GAP
      ? below
      : above >= VIEWPORT_GAP
        ? above
        : Math.min(Math.max(below, VIEWPORT_GAP), maxTop);
}

async function togglePanel(): Promise<void> {
  if (props.disabled) return;
  isOpen.value = !isOpen.value;
  if (!isOpen.value) return;
  await nextTick();
  syncPanelPosition();
}

function closePanel(): void {
  isOpen.value = false;
}

function handleDocumentPointerDown(event: PointerEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (
    !(triggerRef.value?.contains(target) ?? false) &&
    !(panelRef.value?.contains(target) ?? false)
  ) {
    closePanel();
  }
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape") closePanel();
}

onMounted(() => {
  const stored = readStoredIndicators();
  const initial =
    stored ??
    normalizeKlineIndicators(props.modelValue ?? props.defaultIndicators);
  if (!sameIndicators(initial, selectedIndicators.value)) {
    setIndicators(initial);
  }

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

watch(
  () => props.modelValue,
  (indicators) => {
    const normalized = normalizeKlineIndicators(indicators);
    if (!sameIndicators(normalized, selectedIndicators.value)) {
      selectedIndicators.value = normalized;
    }
  },
  { deep: true },
);

watch(
  selectedIndicators,
  (indicators) => persistIndicators(indicators),
  { deep: true },
);
</script>

<template>
  <div class="kline-indicator-selector">
    <button
      ref="triggerRef"
      class="kline-indicator-selector__trigger"
      type="button"
      title="选择技术指标"
      aria-haspopup="dialog"
      :aria-expanded="isOpen"
      :disabled="disabled"
      :class="{ 'is-open': isOpen }"
      @click="togglePanel"
    >
      <span>指标</span>
      <span class="kline-indicator-selector__count">
        {{ selectedIndicators.length }}
      </span>
    </button>

    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="panelRef"
        class="kline-indicator-selector__panel"
        role="dialog"
        aria-label="技术指标"
        :style="{ top: `${panelTop}px`, left: `${panelLeft}px` }"
      >
        <header class="kline-indicator-selector__header">
          <strong>指标</strong>
          <button
            class="kline-indicator-selector__close"
            type="button"
            title="关闭"
            aria-label="关闭指标选择"
            @click="closePanel"
          >
            <span class="fa-solid fa-xmark" aria-hidden="true" />
          </button>
        </header>

        <section
          v-for="group in indicatorGroups"
          :key="group.label"
          class="kline-indicator-selector__group"
        >
          <div class="kline-indicator-selector__group-title">
            {{ group.label }}
          </div>
          <div class="kline-indicator-selector__grid">
            <label
              v-for="indicator in group.indicators"
              :key="indicator.value"
              class="kline-indicator-selector__option"
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
        </section>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.kline-indicator-selector {
  display: inline-flex;
  flex: 0 0 auto;
}

.kline-indicator-selector__trigger {
  display: inline-flex;
  height: 26px;
  align-items: center;
  gap: 5px;
  padding: 0 7px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-5);
  line-height: 1;
}

.kline-indicator-selector__trigger:hover,
.kline-indicator-selector__trigger.is-open {
  border-color: var(--tv-border-strong);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-indicator-selector__trigger:disabled {
  cursor: default;
  opacity: 0.55;
}

.kline-indicator-selector__count {
  display: inline-grid;
  min-width: 16px;
  height: 16px;
  place-items: center;
  border-radius: 8px;
  background: color-mix(in srgb, var(--tv-text) 12%, transparent);
  color: var(--tv-text);
  font-size: var(--jf-text-4);
  font-weight: 600;
}

.kline-indicator-selector__panel {
  position: fixed;
  z-index: 9999;
  width: min(360px, calc(100vw - 16px));
  max-height: min(70vh, 420px);
  padding: 9px;
  overflow: auto;
  border: 1px solid var(--tv-border-strong);
  border-radius: 8px;
  background: var(--tv-bg-surface);
  box-shadow: 0 14px 36px rgb(0 0 0 / 36%);
}

.kline-indicator-selector__header {
  display: flex;
  height: 24px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
  color: var(--tv-text);
  font-size: var(--jf-text-6);
}

.kline-indicator-selector__close {
  display: inline-grid;
  width: 24px;
  height: 24px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.kline-indicator-selector__close:hover {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.kline-indicator-selector__group + .kline-indicator-selector__group {
  margin-top: 8px;
}

.kline-indicator-selector__group-title {
  margin-bottom: 4px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-4);
  font-weight: 650;
}

.kline-indicator-selector__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(96px, 1fr));
  gap: 4px;
}

.kline-indicator-selector__option {
  display: inline-flex;
  min-width: 0;
  height: 26px;
  align-items: center;
  gap: 5px;
  padding: 0 6px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-4);
  line-height: 1;
  user-select: none;
}

.kline-indicator-selector__option input {
  width: 13px;
  height: 13px;
  flex: 0 0 auto;
  margin: 0;
}

.kline-indicator-selector__option span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kline-indicator-selector__option:has(input:checked) {
  border-color: var(--card-teal-border);
  background: var(--card-teal-surface);
  color: var(--tv-text);
}
</style>
