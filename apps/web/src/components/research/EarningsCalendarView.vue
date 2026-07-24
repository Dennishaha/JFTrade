<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import EarningsCalendarFilterDrawer from "./EarningsCalendarFilterDrawer.vue";
import {
  EARNINGS_CALENDAR_SORT_OPTIONS,
  buildEarningsCalendarPath,
  clearIncompatibleEarningsFilters,
  createEarningsCalendarFilters,
  earningsCalendarFilterCount,
  earningsCalendarPeriodLabel,
  earningsCalendarRange,
  isEarningsOptionMarket,
  moveEarningsCalendarAnchor,
  type EarningsCalendarFilters,
  type EarningsCalendarMode,
  type EarningsCalendarSort,
} from "./earningsCalendarModel";
import {
  dayKeyOf,
  entryDayKey,
  formatCompactNumber,
  formatPrice,
  hashColor,
  pickNumber,
  pickString,
} from "./researchEntry";
import { isResearchQuoteEntry } from "./researchQuote";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

const modes: ReadonlyArray<{ value: EarningsCalendarMode; label: string }> = [
  { value: "day", label: "日" },
  { value: "week", label: "周" },
  { value: "month", label: "月" },
];
const mode = ref<EarningsCalendarMode>("month");
const anchorKey = ref(dayKeyOf(new Date()));
const selectedSort = ref<EarningsCalendarSort>("hot");
const appliedFilters = ref<EarningsCalendarFilters>(createEarningsCalendarFilters());
const sortMenuOpen = ref(false);
const filterDrawerOpen = ref(false);
const sortRoot = ref<HTMLElement | null>(null);
const periodInput = ref<HTMLInputElement | null>(null);

const optionMarket = computed(() => isEarningsOptionMarket(props.market));
const range = computed(() => earningsCalendarRange(mode.value, anchorKey.value));
const periodLabel = computed(() =>
  earningsCalendarPeriodLabel(mode.value, anchorKey.value),
);
const periodInputType = computed(() => (mode.value === "month" ? "month" : "date"));
const periodInputValue = computed(() =>
  mode.value === "month" ? anchorKey.value.slice(0, 7) : anchorKey.value,
);
const availableSortOptions = computed(() =>
  EARNINGS_CALENDAR_SORT_OPTIONS.filter(
    (option) => !option.optionOnly || optionMarket.value,
  ),
);
const selectedSortLabel = computed(
  () =>
    EARNINGS_CALENDAR_SORT_OPTIONS.find(
      (option) => option.value === selectedSort.value,
    )?.label ?? "热门",
);
const activeFilterCount = computed(() =>
  earningsCalendarFilterCount(appliedFilters.value),
);

watch(
  () => props.market,
  (market) => {
    if (!isEarningsOptionMarket(market)) {
      const currentSort = EARNINGS_CALENDAR_SORT_OPTIONS.find(
        (option) => option.value === selectedSort.value,
      );
      if (currentSort?.optionOnly) selectedSort.value = "hot";
      appliedFilters.value = clearIncompatibleEarningsFilters(
        appliedFilters.value,
        market,
      );
    }
    sortMenuOpen.value = false;
    filterDrawerOpen.value = false;
  },
  { flush: "sync" },
);

const feature = useResearchFeature(
  () =>
    buildEarningsCalendarPath({
      market: props.market,
      range: range.value,
      sort: selectedSort.value,
      filters: appliedFilters.value,
    }),
  { brokerId: () => props.brokerId },
);

interface EarningsItem {
  entry: Record<string, unknown>;
  key: string;
  name: string;
  initial: string;
  color: string;
  quoteable: boolean;
  code: string;
  fiscalYear: string;
  marketCap: number | null;
  price: number | null;
  optionVolume: number | null;
  iv: number | null;
  ivRank: number | null;
  ivPercentile: number | null;
}

const earningsItems = computed<EarningsItem[]>(() =>
  feature.entries.value.map((entry, index) => {
    const name = pickString(entry, ["name", "symbol", "code"]) || "--";
    const instrumentID = pickString(entry, ["instrumentId"]);
    const code =
      pickString(entry, ["symbol", "code"]) ||
      instrumentID.split(".").at(-1) ||
      "--";
    return {
      entry,
      key: `${entryDayKey(entry, ["eventDate"])}:${instrumentID || code}:${index}`,
      name,
      initial: name.slice(0, 1),
      color: hashColor(name),
      quoteable: isResearchQuoteEntry(entry, props.market),
      code,
      fiscalYear: pickString(entry, [
        "periodText",
        "fiscalYear",
        "fiscalPeriod",
        "periodType",
      ]) || "--",
      marketCap: pickNumber(entry, ["marketCap", "marketValue", "marketVal"]),
      price: pickNumber(entry, ["price", "curPrice"]),
      optionVolume: pickNumber(entry, ["optionVolume"]),
      iv: pickNumber(entry, ["iv"]),
      ivRank: pickNumber(entry, ["ivRank"]),
      ivPercentile: pickNumber(entry, ["ivPercentile"]),
    };
  }),
);

const entriesByDay = computed(() => {
  const map = new Map<string, EarningsItem[]>();
  for (const item of earningsItems.value) {
    const dayKey = entryDayKey(item.entry, ["eventDate"]);
    if (dayKey === "") continue;
    const bucket = map.get(dayKey) ?? [];
    bucket.push(item);
    map.set(dayKey, bucket);
  }
  return map;
});

const dayItems = computed(
  () => entriesByDay.value.get(range.value.beginDate) ?? [],
);

function setMode(value: EarningsCalendarMode): void {
  mode.value = value;
}

function movePeriod(direction: -1 | 1): void {
  anchorKey.value = moveEarningsCalendarAnchor(
    mode.value,
    anchorKey.value,
    direction,
  );
}

function updatePeriod(event: Event): void {
  const value = (event.target as HTMLInputElement).value;
  if (mode.value === "month") {
    if (/^\d{4}-\d{2}$/.test(value)) anchorKey.value = `${value}-01`;
    return;
  }
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) anchorKey.value = value;
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

function selectSort(value: EarningsCalendarSort): void {
  selectedSort.value = value;
  sortMenuOpen.value = false;
}

function openFilters(): void {
  sortMenuOpen.value = false;
  filterDrawerOpen.value = true;
}

function applyFilters(filters: EarningsCalendarFilters): void {
  appliedFilters.value = clearIncompatibleEarningsFilters(filters, props.market);
  filterDrawerOpen.value = false;
}

function visibleItems(dayKey: string): EarningsItem[] {
  const items = entriesByDay.value.get(dayKey) ?? [];
  return mode.value === "month" ? items.slice(0, 4) : items;
}

function overflowCount(dayKey: string): number {
  if (mode.value !== "month") return 0;
  return Math.max(0, (entriesByDay.value.get(dayKey) ?? []).length - 4);
}

function formatPercentage(value: number | null): string {
  if (value == null) return "--";
  return `${new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 2,
  }).format(value)}%`;
}

async function handleModeKeydown(event: KeyboardEvent, index: number): Promise<void> {
  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
  event.preventDefault();
  const delta = event.key === "ArrowRight" ? 1 : -1;
  const nextIndex = (index + delta + modes.length) % modes.length;
  setMode(modes[nextIndex]!.value);
  await nextTick();
  document
    .querySelectorAll<HTMLButtonElement>(".earnings-calendar-view__mode")
    [nextIndex]?.focus();
}

function handleDocumentPointerDown(event: PointerEvent): void {
  if (
    sortMenuOpen.value &&
    event.target instanceof Node &&
    !sortRoot.value?.contains(event.target)
  ) {
    sortMenuOpen.value = false;
  }
}

function handleDocumentKeydown(event: KeyboardEvent): void {
  if (event.key !== "Escape") return;
  sortMenuOpen.value = false;
  filterDrawerOpen.value = false;
}

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
  <section
    class="earnings-calendar-view"
    :aria-busy="feature.loading.value"
  >
    <div class="earnings-calendar-view__toolbar">
      <div
        class="earnings-calendar-view__modes"
        role="tablist"
        aria-label="财报日历视图"
      >
        <button
          v-for="(item, index) in modes"
          :key="item.value"
          type="button"
          class="earnings-calendar-view__mode"
          :class="{ 'is-active': mode === item.value }"
          role="tab"
          :aria-selected="mode === item.value"
          :tabindex="mode === item.value ? 0 : -1"
          @click="setMode(item.value)"
          @keydown="handleModeKeydown($event, index)"
        >
          {{ item.label }}
        </button>
      </div>

      <div class="earnings-calendar-view__period">
        <button
          type="button"
          class="earnings-calendar-view__icon-button"
          :aria-label="`上一${mode === 'day' ? '日' : mode === 'week' ? '周' : '月'}`"
          @click="movePeriod(-1)"
        >
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="m14.5 6-6 6 6 6" />
          </svg>
        </button>
        <div class="earnings-calendar-view__period-picker-wrap">
          <button
            type="button"
            class="earnings-calendar-view__period-picker"
            :aria-label="mode === 'month' ? '打开月份选择器' : '打开日期选择器'"
            @click="openPeriodPicker"
          >
            <span>{{ periodLabel }}</span>
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <path d="m4 6 4 4 4-4" />
            </svg>
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
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="m9.5 6 6 6-6 6" />
          </svg>
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
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <path d="m4 6 4 4 4-4" />
            </svg>
          </button>
          <div
            v-if="sortMenuOpen"
            class="earnings-calendar-view__sort-menu"
            role="menu"
            aria-label="财报排序"
          >
            <button
              v-for="option in availableSortOptions"
              :key="option.value"
              type="button"
              role="menuitemradio"
              :aria-checked="selectedSort === option.value"
              :class="{ 'is-selected': selectedSort === option.value }"
              @click="selectSort(option.value)"
            >
              <svg viewBox="0 0 16 16" aria-hidden="true">
                <path d="m3 8 3 3 7-7" />
              </svg>
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
            <circle cx="7" cy="16" r="2" />
            <circle cx="17" cy="8" r="2" />
          </svg>
          <span v-if="activeFilterCount > 0">{{ activeFilterCount }}</span>
        </button>
      </div>
    </div>

    <div v-if="feature.error.value" class="earnings-calendar-view__error" role="alert">
      <span>{{ feature.error.value }}</span>
      <button type="button" class="tv-button" @click="feature.refresh">重试</button>
    </div>

    <template v-else-if="feature.loading.value">
      <div
        v-if="mode === 'day'"
        class="earnings-calendar-view__table-scroll earnings-calendar-view__skeleton-table"
        aria-label="正在加载日视图"
      >
        <span v-for="index in 6" :key="index" />
      </div>
      <div
        v-else
        class="earnings-calendar-view__calendar-scroll earnings-calendar-view__skeleton-grid"
        :class="{ 'is-month': mode === 'month' }"
        :aria-label="`正在加载${mode === 'week' ? '周' : '月'}视图`"
      >
        <span
          v-for="day in range.days"
          :key="day.key"
          class="earnings-calendar-view__skeleton-cell"
        />
      </div>
    </template>

    <div
      v-else-if="mode === 'day'"
      class="earnings-calendar-view__table-scroll"
      role="region"
      aria-label="单日财报列表"
      tabindex="0"
    >
      <table class="earnings-calendar-view__table">
        <thead>
          <tr>
            <th>代码</th>
            <th>名称</th>
            <th>财年</th>
            <th>市值</th>
            <th>价格</th>
            <th v-if="optionMarket">期权成交量</th>
            <th v-if="optionMarket">隐含波动率</th>
            <th v-if="optionMarket">IV 等级</th>
            <th v-if="optionMarket">IV 百分位数</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in dayItems" :key="item.key">
            <td>{{ item.code }}</td>
            <td>
              <component
                :is="item.quoteable ? 'button' : 'span'"
                :type="item.quoteable ? 'button' : undefined"
                class="earnings-calendar-view__table-company"
                :class="{ 'is-quoteable': item.quoteable }"
                @click="item.quoteable && emit('select', item.entry)"
              >
                <span
                  class="earnings-calendar-view__avatar"
                  :style="{ background: item.color }"
                >{{ item.initial }}</span>
                <span>{{ item.name }}</span>
              </component>
            </td>
            <td>{{ item.fiscalYear }}</td>
            <td>{{ formatCompactNumber(item.marketCap) }}</td>
            <td>{{ formatPrice(item.price) }}</td>
            <td v-if="optionMarket">{{ formatCompactNumber(item.optionVolume) }}</td>
            <td v-if="optionMarket">{{ formatPercentage(item.iv) }}</td>
            <td v-if="optionMarket">{{ formatPercentage(item.ivRank) }}</td>
            <td v-if="optionMarket">{{ formatPercentage(item.ivPercentile) }}</td>
          </tr>
          <tr v-if="dayItems.length === 0">
            <td :colspan="optionMarket ? 9 : 5" class="earnings-calendar-view__empty-row">
              当日暂无财报数据
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-else
      class="earnings-calendar-view__calendar-scroll earnings-calendar-view__week-scroll"
      :class="{ 'is-month': mode === 'month', 'is-week': mode === 'week' }"
      role="region"
      :aria-label="mode === 'month' ? '月度财报日历' : '本周财报日历'"
      tabindex="0"
    >
      <div
        v-if="mode === 'month'"
        class="earnings-calendar-view__weekday-row"
      >
        <span v-for="label in ['周日', '周一', '周二', '周三', '周四', '周五', '周六']" :key="label">
          {{ label }}
        </span>
      </div>
      <div class="earnings-calendar-view__grid">
        <article
          v-for="day in range.days"
          :key="day.key"
          class="earnings-calendar-view__day"
          :class="{
            'is-today': day.today,
            'is-adjacent': mode === 'month' && !day.currentMonth,
          }"
        >
          <header class="earnings-calendar-view__day-head">
            <span v-if="mode === 'week'">{{ day.weekday }}</span>
            <time :datetime="day.key" class="earnings-calendar-view__day-num">
              {{ day.dayOfMonth }}
            </time>
          </header>
          <div class="earnings-calendar-view__day-body">
            <component
              v-for="item in visibleItems(day.key)"
              :key="item.key"
              :is="item.quoteable ? 'button' : 'div'"
              :type="item.quoteable ? 'button' : undefined"
              class="earnings-calendar-view__item"
              :class="{ 'is-quoteable': item.quoteable }"
              :title="item.name"
              @click="item.quoteable && emit('select', item.entry)"
            >
              <span
                class="earnings-calendar-view__avatar"
                :style="{ background: item.color }"
              >{{ item.initial }}</span>
              <span class="earnings-calendar-view__item-name">
                {{ item.name }}
              </span>
            </component>
            <span
              v-if="visibleItems(day.key).length === 0"
              class="earnings-calendar-view__empty-day"
            >
              暂无数据
            </span>
            <span
              v-if="overflowCount(day.key) > 0"
              class="earnings-calendar-view__overflow"
            >
              另 {{ overflowCount(day.key) }} 家
            </span>
          </div>
        </article>
      </div>
    </div>

    <EarningsCalendarFilterDrawer
      :open="filterDrawerOpen"
      :market="market"
      :value="appliedFilters"
      @close="filterDrawerOpen = false"
      @apply="applyFilters"
    />
  </section>
</template>

<style scoped>
.earnings-calendar-view {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 460px;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow: hidden;
  color: var(--tv-text);
  font-size: 13px;
}

.earnings-calendar-view__toolbar {
  display: flex;
  min-height: 44px;
  flex: 0 0 auto;
  align-items: center;
  gap: 18px;
}

.earnings-calendar-view__modes {
  display: inline-flex;
  padding: 3px;
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.earnings-calendar-view__mode {
  min-width: 38px;
  height: 32px;
  padding: 0 11px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font: inherit;
  font-weight: 600;
}

.earnings-calendar-view__mode:hover {
  color: var(--tv-text);
}

.earnings-calendar-view__mode.is-active {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-border) 70%, transparent);
}

.earnings-calendar-view__period {
  display: flex;
  align-items: center;
  gap: 6px;
}

.earnings-calendar-view__icon-button,
.earnings-calendar-view__filter-button {
  display: inline-grid;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  place-items: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
}

.earnings-calendar-view__icon-button:hover,
.earnings-calendar-view__filter-button:hover {
  border-color: var(--tv-border);
  color: var(--tv-text);
}

.earnings-calendar-view__icon-button svg {
  width: 20px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.8;
}

.earnings-calendar-view__period-picker-wrap {
  position: relative;
}

.earnings-calendar-view__period-picker {
  display: flex;
  min-width: 132px;
  height: 34px;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 0 9px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  font-size: 15px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.earnings-calendar-view__period-picker:hover,
.earnings-calendar-view__period-picker:focus-visible {
  background: var(--tv-bg-surface-2);
}

.earnings-calendar-view__period-picker svg,
.earnings-calendar-view__sort-trigger svg {
  width: 14px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.8;
}

.earnings-calendar-view__period-input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  opacity: 0;
  pointer-events: none;
}

.earnings-calendar-view__actions {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: flex-end;
  gap: 7px;
}

.earnings-calendar-view__sort {
  position: relative;
}

.earnings-calendar-view__sort-trigger {
  display: flex;
  min-width: 132px;
  height: 34px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 11px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
  font-weight: 600;
}

.earnings-calendar-view__sort-menu {
  position: absolute;
  z-index: 20;
  top: calc(100% + 6px);
  right: 0;
  display: flex;
  width: 230px;
  flex-direction: column;
  padding: 6px;
  border: 1px solid var(--tv-border);
  border-radius: 7px;
  background: var(--tv-bg-elevated);
  box-shadow: 0 14px 35px rgb(0 0 0 / 28%);
}

.earnings-calendar-view__sort-menu button {
  display: grid;
  min-height: 34px;
  grid-template-columns: 20px 1fr;
  align-items: center;
  padding: 0 9px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.earnings-calendar-view__sort-menu button:hover,
.earnings-calendar-view__sort-menu button:focus-visible {
  background: var(--tv-bg-surface-2);
}

.earnings-calendar-view__sort-menu svg {
  width: 16px;
  visibility: hidden;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 2;
}

.earnings-calendar-view__sort-menu button.is-selected svg {
  visibility: visible;
}

.earnings-calendar-view__filter-button {
  position: relative;
  background: transparent;
}

.earnings-calendar-view__filter-button > svg {
  width: 23px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.5;
}

.earnings-calendar-view__filter-button > span {
  position: absolute;
  top: -3px;
  right: -3px;
  display: grid;
  min-width: 17px;
  height: 17px;
  place-items: center;
  padding: 0 4px;
  border: 2px solid var(--tv-bg);
  border-radius: 9px;
  background: var(--tv-accent);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
}

.earnings-calendar-view__error {
  display: flex;
  min-height: 140px;
  align-items: center;
  justify-content: center;
  gap: 14px;
  border: 1px solid var(--tv-border);
  border-radius: 7px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
}

.earnings-calendar-view__calendar-scroll,
.earnings-calendar-view__table-scroll {
  min-width: 0;
  min-height: 0;
  flex: 1;
  overflow: auto;
  overscroll-behavior-inline: contain;
  border: 1px solid var(--tv-border);
  border-radius: 7px;
  background: var(--tv-bg-surface);
}

.earnings-calendar-view__weekday-row,
.earnings-calendar-view__grid {
  display: grid;
  min-width: 980px;
  grid-template-columns: repeat(7, minmax(140px, 1fr));
}

.earnings-calendar-view__week-scroll.is-week .earnings-calendar-view__grid {
  min-width: 840px;
  grid-template-columns: repeat(7, minmax(120px, 1fr));
}

.earnings-calendar-view__weekday-row {
  position: sticky;
  z-index: 2;
  top: 0;
  height: 42px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.earnings-calendar-view__weekday-row span {
  display: flex;
  align-items: center;
  padding: 0 12px;
  border-right: 1px solid var(--tv-border);
  color: var(--tv-text-muted);
  font-size: 14px;
  font-weight: 650;
}

.earnings-calendar-view__weekday-row span:last-child {
  border-right: 0;
}

.earnings-calendar-view__day {
  min-width: 0;
  min-height: 154px;
  border-right: 1px solid var(--tv-border);
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.earnings-calendar-view__week-scroll.is-week .earnings-calendar-view__day {
  min-height: 280px;
  border-bottom: 0;
}

.earnings-calendar-view__day:nth-child(7n) {
  border-right: 0;
}

.earnings-calendar-view__day.is-adjacent {
  background: color-mix(in srgb, var(--tv-bg-surface-2) 48%, transparent);
}

.earnings-calendar-view__day-head {
  display: flex;
  height: 36px;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  color: var(--tv-text-muted);
}

.earnings-calendar-view__day-num {
  display: inline-grid;
  min-width: 26px;
  height: 26px;
  place-items: center;
  border-radius: 5px;
  color: var(--tv-text);
  font-size: 14px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.earnings-calendar-view__day.is-adjacent .earnings-calendar-view__day-num {
  color: var(--tv-text-dim);
}

.earnings-calendar-view__day.is-today .earnings-calendar-view__day-num {
  background: var(--tv-text);
  color: var(--tv-bg);
}

.earnings-calendar-view__day-body {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 3px;
  padding: 2px 7px 9px;
}

.earnings-calendar-view__item,
.earnings-calendar-view__table-company {
  display: flex;
  min-width: 0;
  height: 28px;
  align-items: center;
  gap: 7px;
  padding: 0 4px;
  overflow: hidden;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text);
  font: inherit;
  text-align: left;
}

.earnings-calendar-view__item.is-quoteable,
.earnings-calendar-view__table-company.is-quoteable {
  cursor: pointer;
}

.earnings-calendar-view__item.is-quoteable:hover,
.earnings-calendar-view__item.is-quoteable:focus-visible,
.earnings-calendar-view__table-company.is-quoteable:hover,
.earnings-calendar-view__table-company.is-quoteable:focus-visible {
  outline: none;
  background: var(--tv-bg-elevated);
}

.earnings-calendar-view__avatar {
  display: inline-grid;
  width: 22px;
  height: 22px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}

.earnings-calendar-view__item-name,
.earnings-calendar-view__table-company > span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.earnings-calendar-view__empty-day {
  padding: 8px 4px;
  color: var(--tv-text-dim);
}

.earnings-calendar-view__overflow {
  padding: 3px 5px;
  color: var(--tv-accent);
  font-size: 12px;
}

.earnings-calendar-view__table {
  width: 100%;
  min-width: 1100px;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}

.earnings-calendar-view__table th {
  position: sticky;
  z-index: 2;
  top: 0;
  height: 48px;
  padding: 0 14px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-weight: 600;
  text-align: right;
  white-space: nowrap;
}

.earnings-calendar-view__table th:nth-child(1),
.earnings-calendar-view__table th:nth-child(2),
.earnings-calendar-view__table td:nth-child(1),
.earnings-calendar-view__table td:nth-child(2) {
  text-align: left;
}

.earnings-calendar-view__table td {
  height: 54px;
  padding: 0 14px;
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text);
  text-align: right;
  white-space: nowrap;
}

.earnings-calendar-view__table-company {
  height: 38px;
  padding: 0 6px;
}

.earnings-calendar-view__empty-row {
  height: 150px !important;
  color: var(--tv-text-dim) !important;
  text-align: center !important;
}

.earnings-calendar-view__skeleton-grid {
  display: grid;
  min-width: 840px;
  grid-template-columns: repeat(7, minmax(120px, 1fr));
}

.earnings-calendar-view__skeleton-grid.is-month {
  min-width: 980px;
}

.earnings-calendar-view__skeleton-cell {
  min-height: 154px;
  border-right: 1px solid var(--tv-border);
  border-bottom: 1px solid var(--tv-border);
  background:
    linear-gradient(
      100deg,
      transparent 20%,
      color-mix(in srgb, var(--tv-text) 7%, transparent) 42%,
      transparent 64%
    ),
    var(--tv-bg-surface);
  background-size: 220% 100%;
  animation: earnings-calendar-shimmer 1.35s linear infinite;
}

.earnings-calendar-view__skeleton-table {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.earnings-calendar-view__skeleton-table span {
  height: 54px;
  background:
    linear-gradient(
      100deg,
      transparent 20%,
      color-mix(in srgb, var(--tv-text) 7%, transparent) 42%,
      transparent 64%
    ),
    var(--tv-bg-surface);
  background-size: 220% 100%;
  animation: earnings-calendar-shimmer 1.35s linear infinite;
}

@keyframes earnings-calendar-shimmer {
  to {
    background-position: -220% 0;
  }
}

:where(
  .earnings-calendar-view__mode,
  .earnings-calendar-view__icon-button,
  .earnings-calendar-view__sort-trigger,
  .earnings-calendar-view__filter-button
):focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: 2px;
}

@media (max-width: 780px) {
  .earnings-calendar-view__toolbar {
    flex-wrap: wrap;
    gap: 8px;
  }

  .earnings-calendar-view__actions {
    flex: 0 0 auto;
    margin-left: auto;
  }

  .earnings-calendar-view__period {
    order: 3;
    width: 100%;
    justify-content: center;
  }

  .earnings-calendar-view__sort-trigger {
    min-width: 112px;
  }
}
</style>
