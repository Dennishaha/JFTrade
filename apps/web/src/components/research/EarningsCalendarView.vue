<script setup lang="ts">
import { computed, ref } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import { dayKeyOf, entryDayKey, hashColor, pickString } from "./researchEntry";
import { isResearchQuoteEntry } from "./researchQuote";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

const weekOffset = ref(0);

interface WeekDay {
  key: string;
  label: string;
  dayOfMonth: number;
  isToday: boolean;
}

const WEEK_LABELS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"] as const;

const weekDays = computed<WeekDay[]>(() => {
  const today = new Date();
  const monday = new Date(today);
  const distanceFromMonday = (today.getDay() + 6) % 7;
  monday.setDate(today.getDate() - distanceFromMonday + weekOffset.value * 7);
  const todayKey = dayKeyOf(today);
  return WEEK_LABELS.map((label, index) => {
    const day = new Date(monday);
    day.setDate(monday.getDate() + index);
    return {
      key: dayKeyOf(day),
      label,
      dayOfMonth: day.getDate(),
      isToday: dayKeyOf(day) === todayKey,
    };
  });
});

const monthLabel = computed(() => {
  const middle = weekDays.value[3] ?? weekDays.value[0];
  if (middle == null) return "";
  const [year, month] = middle.key.split("-");
  return `${year}年${Number(month)}月`;
});

const feature = useResearchFeature(() => {
  const beginDate = weekDays.value[0]?.key ?? dayKeyOf(new Date());
  const endDate = weekDays.value[6]?.key ?? beginDate;
  return `/api/v1/research/calendars?market=${encodeURIComponent(props.market)}&operation=earnings&beginDate=${beginDate}&endDate=${endDate}&pageSize=50`;
}, { brokerId: () => props.brokerId });

interface EarningsItem {
  entry: Record<string, unknown>;
  name: string;
  initial: string;
  color: string;
  quoteable: boolean;
}

const entriesByDay = computed(() => {
  const map = new Map<string, EarningsItem[]>();
  for (const entry of feature.entries.value) {
    const dayKey = entryDayKey(entry, ["eventDate"]);
    if (dayKey === "") continue;
    const name = pickString(entry, ["name", "symbol"]) || "--";
    const item: EarningsItem = {
      entry,
      name,
      initial: name.slice(0, 1),
      color: hashColor(name),
      quoteable: isResearchQuoteEntry(entry, props.market),
    };
    const bucket = map.get(dayKey) ?? [];
    bucket.push(item);
    map.set(dayKey, bucket);
  }
  return map;
});

const hasVisibleEntries = computed(() =>
  weekDays.value.some((day) => (entriesByDay.value.get(day.key) ?? []).length > 0),
);
</script>

<template>
  <div class="earnings-calendar-view">
    <div class="earnings-calendar-view__toolbar">
      <span class="tv-seg">
        <button type="button" disabled>日</button>
        <button type="button" class="is-active">周</button>
        <button type="button" disabled>月</button>
      </span>
      <button
        type="button"
        class="earnings-calendar-view__arrow"
        aria-label="上一周"
        @click="weekOffset -= 1"
      >
        ‹
      </button>
      <span class="earnings-calendar-view__month">{{ monthLabel }}</span>
      <button
        type="button"
        class="earnings-calendar-view__arrow"
        aria-label="下一周"
        @click="weekOffset += 1"
      >
        ›
      </button>
      <span class="earnings-calendar-view__spacer" />
      <select class="earnings-calendar-view__filter" disabled>
        <option>全部类型</option>
      </select>
    </div>

    <div v-if="feature.loading.value" class="earnings-calendar-view__status">加载中…</div>
    <div v-else-if="feature.error.value" class="earnings-calendar-view__status">
      {{ feature.error.value }}
    </div>
    <div v-else-if="!hasVisibleEntries" class="earnings-calendar-view__status">
      本周暂无财报数据
    </div>
    <div v-else class="earnings-calendar-view__grid">
      <div
        v-for="day in weekDays"
        :key="day.key"
        class="earnings-calendar-view__day"
        :class="{ 'is-today': day.isToday }"
      >
        <header class="earnings-calendar-view__day-head">
          <span>{{ day.label }}</span>
          <span class="earnings-calendar-view__day-num">{{ day.dayOfMonth }}</span>
        </header>
        <div class="earnings-calendar-view__day-body">
          <component
            v-for="item in entriesByDay.get(day.key) ?? []"
            :key="item.name"
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
            <span class="earnings-calendar-view__item-name">{{ item.name }}</span>
          </component>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.earnings-calendar-view {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.earnings-calendar-view__toolbar {
  display: flex;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

.earnings-calendar-view__arrow {
  width: 28px;
  height: 28px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}

.earnings-calendar-view__arrow:hover {
  border-color: var(--tv-accent);
}

.earnings-calendar-view__month {
  min-width: 76px;
  font-weight: 600;
  text-align: center;
}

.earnings-calendar-view__spacer {
  flex: 1;
}

.earnings-calendar-view__filter {
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 12px;
}

.earnings-calendar-view__status {
  display: grid;
  min-height: 120px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.earnings-calendar-view__grid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.earnings-calendar-view__day {
  min-height: 160px;
  border-right: 1px solid var(--tv-border);
}

.earnings-calendar-view__day:last-child {
  border-right: 0;
}

.earnings-calendar-view__day.is-today {
  background: color-mix(in srgb, var(--tv-accent) 8%, transparent);
}

.earnings-calendar-view__day-head {
  display: flex;
  height: 32px;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
}

.earnings-calendar-view__day.is-today .earnings-calendar-view__day-head {
  color: var(--tv-accent);
}

.earnings-calendar-view__day-num {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.earnings-calendar-view__day-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px;
}

.earnings-calendar-view__item {
  display: flex;
  height: 28px;
  align-items: center;
  gap: 6px;
  padding: 0 4px;
  overflow: hidden;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text);
  font-size: 12px;
  text-align: left;
}

.earnings-calendar-view__item.is-quoteable {
  cursor: pointer;
}

.earnings-calendar-view__item.is-quoteable:hover {
  background: var(--tv-bg-elevated);
}

.earnings-calendar-view__avatar {
  display: inline-grid;
  width: 20px;
  height: 20px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 50%;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
}

.earnings-calendar-view__item-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
