<script setup lang="ts">
import { computed, ref } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import {
  entryDayKey,
  pickNumber,
  pickString,
} from "./researchEntry";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);

// ---- 筛选：日期快捷段 ----
type RangeFilter = "upcoming" | "week" | "month";
const RANGE_FILTERS: Array<{ value: RangeFilter; label: string }> = [
  { value: "upcoming", label: "今天往后" },
  { value: "week", label: "本周" },
  { value: "month", label: "本月" },
];
const rangeFilter = ref<RangeFilter>("upcoming");

function shanghaiToday(): Date {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const value = (type: Intl.DateTimeFormatPartTypes): string =>
    parts.find((part) => part.type === type)?.value ?? "";
  return new Date(`${value("year")}-${value("month")}-${value("day")}T00:00:00Z`);
}

function utcDayKey(date: Date): string {
  return [
    date.getUTCFullYear(),
    String(date.getUTCMonth() + 1).padStart(2, "0"),
    String(date.getUTCDate()).padStart(2, "0"),
  ].join("-");
}

const rangeBounds = computed(() => {
  const today = shanghaiToday();
  if (rangeFilter.value === "week") {
    const begin = new Date(today);
    begin.setUTCDate(today.getUTCDate() - ((today.getUTCDay() + 6) % 7));
    const end = new Date(begin);
    end.setUTCDate(begin.getUTCDate() + 6);
    return { beginDate: utcDayKey(begin), endDate: utcDayKey(end) };
  }
  if (rangeFilter.value === "month") {
    const begin = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), 1));
    const end = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() + 1, 0));
    return { beginDate: utcDayKey(begin), endDate: utcDayKey(end) };
  }
  const end = new Date(today);
  end.setUTCDate(today.getUTCDate() + 6);
  return { beginDate: utcDayKey(today), endDate: utcDayKey(end) };
});

// OpenD economic-calendar filtering supports CNSH but not CNSZ. Keep CN as the
// UI scope while sending the single concrete SH branch used by the adapter.
const queryMarket = computed(() =>
  props.market.trim().toUpperCase() === "CN" ? "SH" : props.market,
);
const feature = useResearchFeature(
  () =>
    `/api/v1/research/calendars?market=${encodeURIComponent(queryMarket.value)}&operation=economic&beginDate=${rangeBounds.value.beginDate}&endDate=${rangeBounds.value.endDate}&pageSize=50`,
  { brokerId: () => props.brokerId, expandCN: false },
);

function inRange(dayKey: string): boolean {
  if (dayKey === "") return false;
  return dayKey >= rangeBounds.value.beginDate && dayKey <= rangeBounds.value.endDate;
}

// ---- 筛选：地区 ----
const regionFilter = ref("");
const regionOptions = computed(() => {
  const regions = new Set<string>();
  for (const entry of feature.entries.value) {
    const region = pickString(entry, ["region"]);
    if (region !== "") regions.add(region);
  }
  return [...regions];
});

function shanghaiParts(entry: Record<string, unknown>): {
  dayKey: string;
  time: string;
} | null {
  const unixSeconds = pickNumber(entry, ["eventTimestamp"]);
  const raw = pickString(entry, ["eventTime"]);
  const timestamp = unixSeconds != null
    ? unixSeconds * 1_000
    : raw.includes("T") || /[zZ]|[+-]\d{2}:?\d{2}$/.test(raw)
      ? Date.parse(raw)
      : Number.NaN;
  if (!Number.isFinite(timestamp)) return null;
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(timestamp));
  const value = (type: Intl.DateTimeFormatPartTypes): string =>
    parts.find((part) => part.type === type)?.value ?? "";
  return {
    dayKey: `${value("year")}-${value("month")}-${value("day")}`,
    time: `${value("hour")}:${value("minute")}`,
  };
}

function eventDayKey(entry: Record<string, unknown>): string {
  return shanghaiParts(entry)?.dayKey ?? entryDayKey(entry, ["eventDate"]);
}

function timeLabel(entry: Record<string, unknown>): string {
  const localized = shanghaiParts(entry);
  if (localized != null) return localized.time;
  const raw = pickString(entry, ["eventTime"]);
  const match = raw.match(/(\d{2}:\d{2})/);
  return match?.[1] ?? "--";
}

const filteredEntries = computed(() => {
  const seen = new Set<string>();
  return feature.entries.value.filter((entry) => {
    if (!inRange(eventDayKey(entry))) return false;
    if (
      regionFilter.value !== "" &&
      pickString(entry, ["region"]) !== regionFilter.value
    ) {
      return false;
    }
    const key =
      pickString(entry, ["eventId"]) ||
      [
        eventDayKey(entry),
        timeLabel(entry),
        pickString(entry, ["title"]),
        pickString(entry, ["region"]),
      ].join("\u0000");
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
});

interface DayGroup {
  dayKey: string;
  entries: Record<string, unknown>[];
}

const groups = computed<DayGroup[]>(() => {
  const map = new Map<string, Record<string, unknown>[]>();
  for (const entry of filteredEntries.value) {
    const dayKey = eventDayKey(entry) || "未定日期";
    const bucket = map.get(dayKey) ?? [];
    bucket.push(entry);
    map.set(dayKey, bucket);
  }
  return [...map.entries()]
    .sort((left, right) => left[0].localeCompare(right[0]))
    .map(([dayKey, entries]) => ({ dayKey, entries }));
});

function importance(entry: Record<string, unknown>): number {
  const value = pickNumber(entry, ["importance"]);
  if (value == null) return 0;
  return Math.max(1, Math.min(3, Math.round(value)));
}
</script>

<template>
  <div class="econ-calendar-view">
    <div class="econ-calendar-view__toolbar">
      <span class="tv-seg">
        <button
          v-for="item in RANGE_FILTERS"
          :key="item.value"
          type="button"
          :class="{ 'is-active': rangeFilter === item.value }"
          @click="rangeFilter = item.value"
        >{{ item.label }}</button>
      </span>
      <select v-model="regionFilter" class="econ-calendar-view__region">
        <option value="">全部地区</option>
        <option v-for="region in regionOptions" :key="region" :value="region">
          {{ region }}
        </option>
      </select>
      <span class="econ-calendar-view__spacer" />
      <span class="econ-calendar-view__count">共 {{ filteredEntries.length }} 条</span>
    </div>

    <div v-if="feature.loading.value" class="econ-calendar-view__status">加载中…</div>
    <div v-else-if="feature.error.value" class="econ-calendar-view__status">
      {{ feature.error.value }}
    </div>
    <div v-else-if="groups.length === 0" class="econ-calendar-view__status">
      暂无数据
    </div>
    <div v-else class="econ-calendar-view__list">
      <section
        v-for="group in groups"
        :key="group.dayKey"
        class="econ-calendar-view__group"
      >
        <header class="econ-calendar-view__group-head">{{ group.dayKey }}</header>
        <div
          v-for="(entry, index) in group.entries"
          :key="index"
          class="econ-calendar-view__item"
        >
          <span class="econ-calendar-view__time tv-num">
            {{ timeLabel(entry) }}
          </span>
          <span class="econ-calendar-view__headline">
            <span
              v-if="importance(entry) > 0"
              class="econ-calendar-view__stars"
              :aria-label="`重要性 ${importance(entry)}`"
            >{{ "★".repeat(importance(entry)) }}</span>
            <span class="econ-calendar-view__title">
              {{ pickString(entry, ["title"]) || "--" }}
            </span>
            <span
              v-if="pickString(entry, ['region'])"
              class="econ-calendar-view__region-tag"
            >{{ pickString(entry, ["region"]) }}</span>
          </span>
          <span class="econ-calendar-view__values tv-num">
            前值 {{ pickString(entry, ["previousValue"]) || "--" }}
            · 预测 {{ pickString(entry, ["forecastValue"]) || "--" }}
            · 公布 {{ pickString(entry, ["actualValue"]) || "--" }}
          </span>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.econ-calendar-view {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.econ-calendar-view__toolbar {
  display: flex;
  min-height: 32px;
  flex: 0 0 auto;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.econ-calendar-view__region {
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font-size: 12px;
}

.econ-calendar-view__spacer {
  flex: 1;
}

.econ-calendar-view__count {
  color: var(--tv-text-dim);
  font-variant-numeric: tabular-nums;
}

.econ-calendar-view__status {
  display: grid;
  min-height: 120px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.econ-calendar-view__list {
  overflow: hidden auto;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.econ-calendar-view__group-head {
  display: flex;
  height: 32px;
  align-items: center;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-weight: 600;
}

.econ-calendar-view__item {
  display: grid;
  grid-template-columns: 44px minmax(160px, 1fr) auto;
  min-height: 32px;
  align-items: center;
  gap: 10px;
  padding: 4px 10px;
  border-bottom: 1px solid var(--tv-border);
}

.econ-calendar-view__item:hover {
  background: var(--tv-bg-elevated);
}

.econ-calendar-view__time {
  color: var(--tv-text-muted);
}

.econ-calendar-view__headline {
  display: grid;
  min-width: 0;
  align-items: center;
  gap: 10px;
  grid-template-areas: "stars title region";
  grid-template-columns: auto minmax(0, 1fr) auto;
}

.econ-calendar-view__stars {
  grid-area: stars;
  color: var(--tv-warn);
  font-size: 10px;
}

.econ-calendar-view__title {
  grid-area: title;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.econ-calendar-view__region-tag {
  grid-area: region;
  padding: 1px 6px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.econ-calendar-view__values {
  color: var(--tv-text-muted);
  font-size: 11px;
  white-space: nowrap;
}

@media (max-width: 700px) {
  .econ-calendar-view__item {
    grid-template-columns: 44px minmax(0, 1fr);
    align-items: start;
  }

  .econ-calendar-view__headline {
    grid-template-areas:
      "title title"
      "stars region";
    grid-template-columns: minmax(0, 1fr) auto;
    row-gap: 4px;
  }

  .econ-calendar-view__title {
    min-width: 96px;
  }

  .econ-calendar-view__values {
    grid-column: 1 / -1;
    padding-left: 54px;
    white-space: normal;
  }
}
</style>
