<script setup lang="ts">
import type { ResearchTableColumn } from "./researchTable";
import { formatResearchCell } from "./researchTable";

const props = withDefaults(
  defineProps<{
    entries: Record<string, unknown>[];
    columns: ResearchTableColumn[];
    rowKey?: (
      entry: Record<string, unknown>,
      index: number,
    ) => string | number;
    selectedKey?: string | number | null;
    emptyLabel?: string;
    compact?: boolean;
  }>(),
  {
    selectedKey: null,
    emptyLabel: "暂无数据",
    compact: false,
  },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

function keyFor(
  entry: Record<string, unknown>,
  index: number,
): string | number {
  return props.rowKey?.(entry, index) ?? index;
}

function display(
  column: ResearchTableColumn,
  entry: Record<string, unknown>,
): string {
  const value = column.value(entry);
  return column.format?.(value, entry) ?? formatResearchCell(value);
}
</script>

<template>
  <div class="research-data-table" :class="{ 'is-compact': compact }">
    <table v-if="entries.length > 0">
      <thead>
        <tr>
          <th
            v-for="column in columns"
            :key="column.key"
            :class="`is-${column.align ?? 'left'}`"
            :style="{ width: column.width }"
          >
            {{ column.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="(entry, index) in entries"
          :key="keyFor(entry, index)"
          :class="{
            'is-selected':
              selectedKey != null && selectedKey === keyFor(entry, index),
          }"
          tabindex="0"
          @click="emit('select', entry)"
          @dblclick="emit('open', entry)"
          @keydown.enter="emit('select', entry)"
        >
          <td
            v-for="column in columns"
            :key="column.key"
            :class="[
              `is-${column.align ?? 'left'}`,
              column.className?.(column.value(entry), entry),
            ]"
            :title="display(column, entry)"
          >
            {{ display(column, entry) }}
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="research-data-table__empty">{{ emptyLabel }}</div>
  </div>
</template>

<style scoped>
.research-data-table {
  min-width: 0;
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-size: 12px;
}

.research-data-table table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.research-data-table th {
  position: sticky;
  z-index: 2;
  top: 0;
  height: 32px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 600;
}

.research-data-table td {
  max-width: 320px;
  height: 32px;
  padding: 0 8px;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  text-overflow: ellipsis;
}

.research-data-table tr:last-child td {
  border-bottom: 0;
}

.research-data-table tbody tr {
  cursor: default;
  outline: none;
}

.research-data-table tbody tr:hover,
.research-data-table tbody tr:focus-visible {
  background: var(--tv-bg-elevated);
}

.research-data-table tbody tr.is-selected {
  background: color-mix(in srgb, var(--tv-accent) 10%, transparent);
}

.research-data-table .is-right {
  text-align: right;
}

.research-data-table .is-center {
  text-align: center;
}

.research-data-table.is-compact td {
  height: 28px;
}

.research-data-table__empty {
  display: grid;
  min-height: 120px;
  place-items: center;
  color: var(--tv-text-dim);
}
</style>
