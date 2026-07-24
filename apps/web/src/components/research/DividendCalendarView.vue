<script setup lang="ts">
import { computed, ref } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import ResearchDataTable from "./ResearchDataTable.vue";
import { dayKeyOf, pickString } from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

const selectedDate = ref(dayKeyOf(new Date()));
const feature = useResearchFeature(
  () =>
    `/api/v1/research/calendars?market=${encodeURIComponent(props.market)}&operation=dividends&date=${selectedDate.value}&pageSize=50`,
  { brokerId: () => props.brokerId },
);

const columns: ResearchTableColumn[] = [
  {
    key: "instrument",
    label: "证券",
    width: "150px",
    value: (entry) =>
      pickString(entry, ["name", "instrumentId", "symbol", "code"]),
  },
  {
    key: "statement",
    label: "派息公告",
    value: (entry) =>
      pickString(entry, ["statement", "title", "description"]),
  },
  {
    key: "exDate",
    label: "除权日",
    width: "110px",
    value: (entry) => pickString(entry, ["exDate", "eventDate"]),
  },
  {
    key: "recordDate",
    label: "登记日",
    width: "110px",
    value: (entry) => pickString(entry, ["recordDate"]),
  },
  {
    key: "payDate",
    label: "派息日",
    width: "110px",
    value: (entry) =>
      pickString(entry, ["dividendPayableDate", "payDate"]),
  },
];

const statusLabel = computed(() => {
  if (feature.loading.value) return "派息日历加载中…";
  if (feature.error.value) return feature.error.value;
  return "";
});

function shiftDate(days: number): void {
  const value = new Date(`${selectedDate.value}T12:00:00`);
  value.setDate(value.getDate() + days);
  selectedDate.value = dayKeyOf(value);
}

function rowKey(
  entry: Record<string, unknown>,
  index: number,
): string {
  return [
    pickString(entry, ["instrumentId", "symbol", "code"]),
    pickString(entry, ["exDate", "eventDate"]),
    index,
  ].join(":");
}
</script>

<template>
  <section class="dividend-calendar">
    <header class="dividend-calendar__toolbar">
      <button type="button" aria-label="前一天" @click="shiftDate(-1)">‹</button>
      <input v-model="selectedDate" type="date" />
      <button type="button" aria-label="后一天" @click="shiftDate(1)">›</button>
      <span class="dividend-calendar__spacer" />
      <small v-if="feature.asOf.value">更新 {{ feature.asOf.value }}</small>
      <button type="button" @click="feature.refresh">刷新</button>
    </header>
    <div v-if="statusLabel" class="dividend-calendar__status">
      {{ statusLabel }}
    </div>
    <ResearchDataTable
      v-else
      :entries="feature.entries.value"
      :columns="columns"
      :row-key="rowKey"
      empty-label="当日暂无派息安排"
      @select="emit('select', $event)"
      @open="emit('open', $event)"
    />
  </section>
</template>

<style scoped>
.dividend-calendar {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.dividend-calendar__toolbar {
  display: flex;
  height: 32px;
  align-items: center;
  gap: 6px;
}

.dividend-calendar__toolbar button,
.dividend-calendar__toolbar input {
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font: inherit;
}

.dividend-calendar__toolbar button {
  cursor: pointer;
}

.dividend-calendar__toolbar button:hover {
  border-color: var(--tv-accent);
}

.dividend-calendar__spacer {
  flex: 1;
}

.dividend-calendar__toolbar small {
  color: var(--tv-text-dim);
}

.dividend-calendar__status {
  display: grid;
  min-height: 120px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}
</style>
