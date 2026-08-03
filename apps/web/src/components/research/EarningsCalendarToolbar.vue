<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import SegmentedControl from "@/components/shared/SegmentedControl.vue";

import {
  earningsCalendarPeriodLabel,
  moveEarningsCalendarAnchor,
} from "./earningsCalendarModel";
import type {
  EarningsCalendarMode,
  EarningsCalendarSort,
} from "./earningsCalendarModel";

const props = defineProps<{
  mode: EarningsCalendarMode;
  anchorKey: string;
  availableSortOptions: ReadonlyArray<{
    value: EarningsCalendarSort;
    label: string;
    optionOnly: boolean;
  }>;
  selectedSort: EarningsCalendarSort;
  selectedSortLabel: string;
  activeFilterCount: number;
}>();

const emit = defineEmits<{
  "update:mode": [value: EarningsCalendarMode];
  "update:anchorKey": [value: string];
  "update:selectedSort": [value: EarningsCalendarSort];
  "open-filters": [];
  dismiss: [];
}>();

const modes: ReadonlyArray<{ value: EarningsCalendarMode; label: string }> = [
  { value: "day", label: "日" },
  { value: "week", label: "周" },
  { value: "month", label: "月" },
];
const sortMenuOpen = ref(false);
const sortRoot = ref<HTMLElement | null>(null);
const periodInput = ref<HTMLInputElement | null>(null);
const periodLabel = computed(() =>
  earningsCalendarPeriodLabel(props.mode, props.anchorKey),
);
const periodInputType = computed(() => props.mode === "month" ? "month" : "date");
const periodInputValue = computed(() =>
  props.mode === "month" ? props.anchorKey.slice(0, 7) : props.anchorKey,
);

function movePeriod(direction: -1 | 1): void {
  emit(
    "update:anchorKey",
    moveEarningsCalendarAnchor(props.mode, props.anchorKey, direction),
  );
}

function updatePeriod(event: Event): void {
  const value = (event.target as HTMLInputElement).value;
  if (props.mode === "month" && /^\d{4}-\d{2}$/.test(value)) {
    emit("update:anchorKey", `${value}-01`);
  } else if (props.mode !== "month" && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
    emit("update:anchorKey", value);
  }
}

function openPeriodPicker(): void {
  const input = periodInput.value;
  if (!input) return;
  input.focus({ preventScroll: true });
  try {
    input.showPicker();
  } catch {
    input.click();
  }
}

function openFilters(): void {
  sortMenuOpen.value = false;
  emit("open-filters");
}

function selectMode(value: string): void {
  if (!modes.some((mode) => mode.value === value)) return;
  emit("update:mode", value as EarningsCalendarMode);
}

function handleDocumentPointerDown(event: PointerEvent): void {
  if (
    sortMenuOpen.value &&
    event.target instanceof Node &&
    !sortRoot.value?.contains(event.target)
  ) sortMenuOpen.value = false;
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (event.key !== "Escape") return;
  sortMenuOpen.value = false;
  emit("dismiss");
}

watch(() => props.availableSortOptions, () => {
  sortMenuOpen.value = false;
});
onMounted(() => {
  document.addEventListener("pointerdown", handleDocumentPointerDown);
  document.addEventListener("keydown", handleDocumentKeydown);
});
onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", handleDocumentPointerDown);
  document.removeEventListener("keydown", handleDocumentKeydown);
});
</script>

<template>
  <div class="earnings-calendar-view__toolbar">
    <SegmentedControl class="earnings-calendar-view__modes" size="small" :model-value="mode"
      :items="modes" label="财报日历视图" @update:model-value="selectMode" />

    <div class="earnings-calendar-view__period">
      <button
        type="button"
        class="earnings-calendar-view__icon-button"
        :aria-label="`上一${mode === 'day' ? '日' : mode === 'week' ? '周' : '月'}`"
        @click="movePeriod(-1)"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14.5 6-6 6 6 6" /></svg>
      </button>
      <div class="earnings-calendar-view__period-picker-wrap">
        <button
          type="button"
          class="earnings-calendar-view__period-picker"
          :aria-label="mode === 'month' ? '打开月份选择器' : '打开日期选择器'"
          @click="openPeriodPicker"
        >
          <span>{{ periodLabel }}</span>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
        </button>
        <input
          ref="periodInput"
          class="earnings-calendar-view__period-input"
          :type="periodInputType"
          :value="periodInputValue"
          :aria-label="mode === 'month' ? '选择月份' : '选择日期'"
          @change="updatePeriod"
        >
      </div>
      <button
        type="button"
        class="earnings-calendar-view__icon-button"
        :aria-label="`下一${mode === 'day' ? '日' : mode === 'week' ? '周' : '月'}`"
        @click="movePeriod(1)"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9.5 6 6 6-6 6" /></svg>
      </button>
    </div>

    <div class="earnings-calendar-view__actions">
      <div ref="sortRoot" class="earnings-calendar-view__sort">
        <button
          type="button"
          class="earnings-calendar-view__sort-trigger"
          aria-haspopup="menu"
          :aria-expanded="sortMenuOpen"
          @click="sortMenuOpen = !sortMenuOpen"
        >
          <span>{{ selectedSortLabel }}</span>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
        </button>
        <div v-if="sortMenuOpen" class="earnings-calendar-view__sort-menu" role="menu" aria-label="财报排序">
          <button
            v-for="option in availableSortOptions"
            :key="option.value"
            type="button"
            role="menuitemradio"
            :aria-checked="selectedSort === option.value"
            :class="{ 'is-selected': selectedSort === option.value }"
            @click="emit('update:selectedSort', option.value); sortMenuOpen = false"
          >
            <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8 3 3 7-7" /></svg>
            <span>{{ option.label }}</span>
          </button>
        </div>
      </div>
      <button
        type="button"
        class="earnings-calendar-view__filter-button"
        :aria-label="activeFilterCount > 0 ? `筛选，已生效 ${activeFilterCount} 项` : '筛选'"
        @click="openFilters"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="M7 4v10M7 18v2M17 4v2M17 10v10M4 14h6M14 7h6" />
          <circle cx="7" cy="16" r="2" /><circle cx="17" cy="8" r="2" />
        </svg>
        <span v-if="activeFilterCount > 0">{{ activeFilterCount }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.earnings-calendar-view__toolbar { display: flex; min-height: 44px; flex: 0 0 auto; align-items: center; gap: 18px; }
.earnings-calendar-view__modes { display: inline-flex; padding: 3px; border: 0; border-radius: 6px; background: var(--tv-bg-surface-2); }
.earnings-calendar-view__modes :deep(.segmented-control__item) {
  min-width: 38px; height: 32px; padding: 0 11px; border: 0; border-radius: 4px;
  background: transparent; color: var(--tv-text-muted); cursor: pointer; font: inherit; font-weight: 600;
}
.earnings-calendar-view__modes :deep(.segmented-control__item:hover) { color: var(--tv-text); }
.earnings-calendar-view__modes :deep(.segmented-control__item.is-active) {
  background: var(--tv-bg-elevated); color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-border) 70%, transparent);
}
.earnings-calendar-view__period { display: flex; align-items: center; gap: 6px; }
.earnings-calendar-view__icon-button,
.earnings-calendar-view__filter-button {
  display: inline-grid; width: 34px; height: 34px; flex: 0 0 auto; place-items: center; padding: 0;
  border: 1px solid transparent; border-radius: 5px; background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted); cursor: pointer;
}
.earnings-calendar-view__icon-button:hover,
.earnings-calendar-view__filter-button:hover { border-color: var(--tv-border); color: var(--tv-text); }
.earnings-calendar-view__icon-button svg {
  width: 20px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8;
}
.earnings-calendar-view__period-picker-wrap { position: relative; }
.earnings-calendar-view__period-picker {
  display: flex; min-width: 132px; height: 34px; align-items: center; justify-content: center; gap: 5px;
  padding: 0 9px; border: 0; border-radius: 5px; background: transparent; color: inherit;
  cursor: pointer; font: inherit; font-size: var(--jf-text-9); font-weight: 650; font-variant-numeric: tabular-nums;
}
.earnings-calendar-view__period-picker:hover,
.earnings-calendar-view__period-picker:focus-visible { background: var(--tv-bg-surface-2); }
.earnings-calendar-view__period-picker svg,
.earnings-calendar-view__sort-trigger svg {
  width: 14px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.8;
}
.earnings-calendar-view__period-input {
  position: absolute; width: 1px; height: 1px; overflow: hidden; opacity: 0; pointer-events: none;
}
.earnings-calendar-view__actions { display: flex; flex: 1; align-items: center; justify-content: flex-end; gap: 7px; }
.earnings-calendar-view__sort { position: relative; }
.earnings-calendar-view__sort-trigger {
  display: flex; min-width: 132px; height: 34px; align-items: center; justify-content: space-between; gap: 12px;
  padding: 0 11px; border: 1px solid var(--tv-border); border-radius: 5px;
  background: var(--tv-bg-surface-2); color: var(--tv-text); cursor: pointer; font: inherit; font-weight: 600;
}
.earnings-calendar-view__sort-menu {
  position: absolute; z-index: 20; top: calc(100% + 6px); right: 0; display: flex; width: 230px;
  flex-direction: column; padding: 6px; border: 1px solid var(--tv-border); border-radius: 7px;
  background: var(--tv-bg-elevated); box-shadow: 0 14px 35px rgb(0 0 0 / 28%);
}
.earnings-calendar-view__sort-menu button {
  display: grid; min-height: 34px; grid-template-columns: 20px 1fr; align-items: center; padding: 0 9px;
  border: 0; border-radius: 5px; background: transparent; color: var(--tv-text); cursor: pointer; font: inherit; text-align: left;
}
.earnings-calendar-view__sort-menu button:hover,
.earnings-calendar-view__sort-menu button:focus-visible { background: var(--tv-bg-surface-2); }
.earnings-calendar-view__sort-menu svg {
  width: 16px; visibility: hidden; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 2;
}
.earnings-calendar-view__sort-menu button.is-selected svg { visibility: visible; }
.earnings-calendar-view__filter-button { position: relative; background: transparent; }
.earnings-calendar-view__filter-button > svg {
  width: 23px; fill: none; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.5;
}
.earnings-calendar-view__filter-button > span {
  position: absolute; top: -3px; right: -3px; display: grid; min-width: 17px; height: 17px;
  place-items: center; padding: 0 4px; border: 2px solid var(--tv-bg); border-radius: 9px;
  background: var(--tv-accent); color: var(--jf-white); font-size: var(--jf-text-4); font-weight: 700;
}
:where(
  .earnings-calendar-view__mode,
  .earnings-calendar-view__icon-button,
  .earnings-calendar-view__sort-trigger,
  .earnings-calendar-view__filter-button
):focus-visible { outline: 2px solid var(--tv-accent); outline-offset: 2px; }
@media (max-width: 780px) {
  .earnings-calendar-view__toolbar { flex-wrap: wrap; gap: 8px; }
  .earnings-calendar-view__actions { flex: 0 0 auto; margin-left: auto; }
  .earnings-calendar-view__period { order: 3; width: 100%; justify-content: center; }
  .earnings-calendar-view__sort-trigger { min-width: 112px; }
}
</style>
