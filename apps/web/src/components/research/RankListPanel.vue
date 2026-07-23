<script setup lang="ts">
import { computed, ref, watch } from "vue";

type SortOrder = "desc" | "asc";

const props = withDefaults(
  defineProps<{
    title: string;
    entries: Record<string, unknown>[];
    loading?: boolean;
    valueField?: string;
    valueLabel?: string;
    defaultSortOrder?: SortOrder;
  }>(),
  {
    loading: false,
    valueField: "changeRate",
    valueLabel: "涨跌幅",
    defaultSortOrder: "desc",
  },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

const sortOrder = ref<SortOrder>(props.defaultSortOrder);

watch(
  () => props.defaultSortOrder,
  (order) => {
    sortOrder.value = order;
  },
);

function pickString(
  entry: Record<string, unknown>,
  keys: readonly string[],
): string {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "string" && value.trim() !== "") return value;
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return "";
}

function pickNumber(
  entry: Record<string, unknown>,
  keys: readonly string[],
): number | null {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim() !== "") {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return null;
}

function entryCode(entry: Record<string, unknown>): string {
  return pickString(entry, ["symbol", "instrumentId"]);
}

function entryName(entry: Record<string, unknown>): string {
  return pickString(entry, ["name"]);
}

function entryValue(entry: Record<string, unknown>): number | null {
  return pickNumber(entry, [props.valueField]);
}

function entryPrice(entry: Record<string, unknown>): number | null {
  return pickNumber(entry, ["price", "lastPrice"]);
}

const sortedEntries = computed(() => {
  const direction = sortOrder.value === "desc" ? -1 : 1;
  return [...props.entries].sort((left, right) => {
    const leftValue = entryValue(left);
    const rightValue = entryValue(right);
    if (leftValue == null && rightValue == null) return 0;
    if (leftValue == null) return 1;
    if (rightValue == null) return -1;
    return (leftValue - rightValue) * direction;
  });
});

function toggleSort(): void {
  sortOrder.value = sortOrder.value === "desc" ? "asc" : "desc";
}

const valueIsPercent = computed(() =>
  /rate|yield|ratio|percent/i.test(props.valueField),
);

function formatValue(value: number | null): string {
  if (value == null) return "--";
  const formatted = `${Math.abs(value).toFixed(2)}${valueIsPercent.value ? "%" : ""}`;
  if (value > 0) return `+${formatted}`;
  if (value < 0) return `-${formatted}`;
  return formatted;
}

function formatPrice(value: number | null): string {
  if (value == null) return "--";
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 4 }).format(
    value,
  );
}

function valueClass(value: number | null): string {
  if (value == null || value === 0) return "";
  return value > 0 ? "tv-up" : "tv-down";
}
</script>

<template>
  <section class="rank-list-panel">
    <header class="rank-list-panel__head">
      <span class="rank-list-panel__title">{{ title }}</span>
    </header>
    <div v-if="loading" class="rank-list-panel__status">加载中…</div>
    <div v-else-if="sortedEntries.length === 0" class="rank-list-panel__status">
      暂无数据
    </div>
    <table v-else class="rank-list-panel__table">
      <thead>
        <tr>
          <th class="rank-list-panel__code">代码</th>
          <th>名称</th>
          <th
            class="rank-list-panel__value rank-list-panel__sortable"
            :aria-sort="sortOrder === 'desc' ? 'descending' : 'ascending'"
            @click="toggleSort"
          >
            {{ valueLabel }}
            <span class="rank-list-panel__sort-icon">{{
              sortOrder === "desc" ? "↓" : "↑"
            }}</span>
          </th>
          <th class="rank-list-panel__price">最新价</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="(entry, index) in sortedEntries"
          :key="`${entryCode(entry) || index}-${index}`"
          @click="emit('select', entry)"
        >
          <td class="rank-list-panel__code">{{ entryCode(entry) || "--" }}</td>
          <td class="rank-list-panel__name">{{ entryName(entry) || "--" }}</td>
          <td class="rank-list-panel__value tv-num" :class="valueClass(entryValue(entry))">
            {{ formatValue(entryValue(entry)) }}
          </td>
          <td class="rank-list-panel__price tv-num">
            {{ formatPrice(entryPrice(entry)) }}
          </td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.rank-list-panel {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-size: 12px;
}

.rank-list-panel__head {
  display: flex;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.rank-list-panel__title {
  font-size: 12px;
  font-weight: 600;
}

.rank-list-panel__status {
  display: grid;
  flex: 1;
  min-height: 96px;
  place-items: center;
  color: var(--tv-text-dim);
}

.rank-list-panel__table {
  width: 100%;
  border-collapse: collapse;
}

.rank-list-panel__table th,
.rank-list-panel__table td {
  height: 32px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  text-align: left;
  white-space: nowrap;
}

.rank-list-panel__table th {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 500;
}

.rank-list-panel__sortable {
  cursor: pointer;
  user-select: none;
}

.rank-list-panel__sortable:hover {
  color: var(--tv-text);
}

.rank-list-panel__sort-icon {
  margin-left: 2px;
  font-size: 10px;
}

.rank-list-panel__table tbody tr {
  cursor: pointer;
}

.rank-list-panel__table tbody tr:hover td {
  background: var(--tv-bg-elevated);
}

.rank-list-panel__code {
  font-variant-numeric: tabular-nums;
  color: var(--tv-text-muted);
}

.rank-list-panel__name {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.rank-list-panel__value,
.rank-list-panel__price {
  text-align: right;
}
</style>
