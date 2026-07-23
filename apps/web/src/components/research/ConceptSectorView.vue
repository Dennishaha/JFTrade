<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import {
  directionClass,
  formatCompactNumber,
  formatPrice,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import {
  mergeResearchSnapshot,
  useResearchSnapshots,
} from "./researchSnapshots";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

function industriesPath(operation: string): string {
  return `/api/v1/research/industries?market=${encodeURIComponent(props.market)}&operation=${operation}&pageSize=50`;
}

const plateType = ref<"industry" | "concept" | "region">("concept");
const connectOnly = ref(false);
const plates = useResearchFeature(
  () => `${industriesPath("plate_list")}&plateType=${plateType.value}`,
  { brokerId: () => props.brokerId },
);

const selectedPlate = ref<Record<string, unknown> | null>(null);
const plateSnapshots = useResearchSnapshots(
  () =>
    plates.entries.value
      .map((entry) => pickString(entry, ["instrumentId"]))
      .filter(Boolean),
  () => props.brokerId,
);
const plateEntries = computed(() =>
  plates.entries.value
    .map((entry) =>
      mergeResearchSnapshot(
        entry,
        plateSnapshots.byInstrumentId.value[
          pickString(entry, ["instrumentId"]).toUpperCase()
        ],
      ),
    )
    .filter(
      (entry) =>
        !connectOnly.value ||
        /港股通|沪股通|深股通/i.test(pickString(entry, ["name"])),
    ),
);

// 默认选中排行榜第一条
watch([plateEntries, () => props.market, plateType], ([entries]) => {
  const selectedId = selectedPlate.value?.instrumentId;
  const stillExists = entries.some((entry) => entry.instrumentId === selectedId);
  if (!stillExists) selectedPlate.value = entries[0] ?? null;
}, { immediate: true });

function choosePlate(entry: Record<string, unknown>): void {
  selectedPlate.value = entry;
  emit("select", entry);
}

function plateKey(entry: Record<string, unknown>): string {
  return pickString(entry, ["instrumentId", "name"]);
}

const stocksPath = computed(() => {
  const plate = selectedPlate.value;
  if (plate == null) return "";
  const instrumentId = pickString(plate, ["instrumentId"]);
  if (!instrumentId.includes(".")) return "";
  const concreteMarket = instrumentId.split(".")[0]!;
  return `/api/v1/research/industries?market=${encodeURIComponent(concreteMarket)}&operation=plate_members&instrumentId=${encodeURIComponent(instrumentId)}&pageSize=50`;
});
const stocks = useResearchFeature(() => stocksPath.value, {
  brokerId: () => props.brokerId,
});
const stockSnapshots = useResearchSnapshots(
  () =>
    stocks.entries.value
      .map((entry) => pickString(entry, ["instrumentId"]))
      .filter(Boolean),
  () => props.brokerId,
);

type SortKey = "price" | "changeAmount" | "changeRate" | "volume" | "turnover";
const sortKey = ref<SortKey>("changeRate");
const sortAsc = ref(false);

function toggleSort(key: SortKey): void {
  if (sortKey.value === key) {
    sortAsc.value = !sortAsc.value;
  } else {
    sortKey.value = key;
    sortAsc.value = false;
  }
}

const SORT_FIELDS: Record<SortKey, readonly string[]> = {
  price: ["price", "lastPrice"],
  changeAmount: ["changeAmount"],
  changeRate: ["changeRate"],
  volume: ["volume"],
  turnover: ["turnover"],
};

const sortedStocks = computed(() => {
  const fields = SORT_FIELDS[sortKey.value];
  const direction = sortAsc.value ? 1 : -1;
  return stocks.entries.value
    .map((entry) =>
      mergeResearchSnapshot(
        entry,
        stockSnapshots.byInstrumentId.value[
          pickString(entry, ["instrumentId"]).toUpperCase()
        ],
      ),
    )
    .sort((left, right) => {
    const leftValue = pickNumber(left, fields);
    const rightValue = pickNumber(right, fields);
    if (leftValue == null && rightValue == null) return 0;
    if (leftValue == null) return 1;
    if (rightValue == null) return -1;
    return (leftValue - rightValue) * direction;
    });
});

function sortIndicator(key: SortKey): string {
  if (sortKey.value !== key) return "";
  return sortAsc.value ? "↑" : "↓";
}

const stockColumns: Array<{ key: SortKey; label: string }> = [
  { key: "price", label: "最新价" },
  { key: "changeAmount", label: "涨跌额" },
  { key: "changeRate", label: "涨跌幅" },
  { key: "volume", label: "成交量" },
  { key: "turnover", label: "成交额" },
];
</script>

<template>
  <div class="concept-sector-view">
    <div v-if="plateSnapshots.error.value || stockSnapshots.error.value" class="concept-sector-view__warning">
      行情补充失败：{{ plateSnapshots.error.value || stockSnapshots.error.value }}
    </div>
    <section class="concept-sector-view__plates">
      <header class="concept-sector-view__head">
        <span>板块排行</span>
        <span class="tv-seg concept-sector-view__types">
          <button type="button" :class="{ 'is-active': plateType === 'industry' }" @click="plateType = 'industry'">行业</button>
          <button type="button" :class="{ 'is-active': plateType === 'concept' }" @click="plateType = 'concept'">概念</button>
          <button type="button" :class="{ 'is-active': plateType === 'region' }" @click="plateType = 'region'">地域</button>
        </span>
        <button
          v-if="market === 'HK' || market === 'CN'"
          type="button"
          class="concept-sector-view__connect"
          :class="{ 'is-active': connectOnly }"
          @click="connectOnly = !connectOnly"
        >港股通</button>
      </header>
      <div v-if="plates.loading.value" class="concept-sector-view__status">加载中…</div>
      <div v-else-if="plates.error.value" class="concept-sector-view__status">
        {{ plates.error.value }}
      </div>
      <div v-else-if="plateEntries.length === 0" class="concept-sector-view__status">
        {{ connectOnly ? "当前 OpenD 未返回港股通相关板块" : "暂无数据" }}
      </div>
      <table v-else class="concept-sector-view__table">
        <thead>
          <tr>
            <th class="concept-sector-view__index">#</th>
            <th>名称</th>
            <th class="concept-sector-view__num">最新价</th>
            <th class="concept-sector-view__num">涨跌幅</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(entry, index) in plateEntries"
            :key="plateKey(entry) || index"
            :class="{
              'is-selected': selectedPlate != null && plateKey(selectedPlate) === plateKey(entry),
            }"
            @click="choosePlate(entry)"
          >
            <td class="concept-sector-view__index">{{ index + 1 }}</td>
            <td class="concept-sector-view__name">
              {{ pickString(entry, ["name"]) || "--" }}
            </td>
            <td class="concept-sector-view__num tv-num">
              {{ formatPrice(pickNumber(entry, ["price", "lastPrice"])) }}
            </td>
            <td
              class="concept-sector-view__num tv-num"
              :class="directionClass(pickNumber(entry, ['changeRate']))"
            >
              {{ formatSigned(pickNumber(entry, ["changeRate"]), "%") }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="concept-sector-view__stocks">
      <header class="concept-sector-view__head">
        成分股<span v-if="selectedPlate" class="concept-sector-view__head-sub">
          {{ pickString(selectedPlate, ["name"]) }}</span
        >
      </header>
      <div v-if="stocks.loading.value" class="concept-sector-view__status">加载中…</div>
      <div v-else-if="stocks.error.value" class="concept-sector-view__status">
        {{ stocks.error.value }}
      </div>
      <div v-else-if="sortedStocks.length === 0" class="concept-sector-view__status">
        暂无数据
      </div>
      <table v-else class="concept-sector-view__table">
        <thead>
          <tr>
            <th class="concept-sector-view__index">#</th>
            <th>代码</th>
            <th>名称</th>
            <th
              v-for="column in stockColumns"
              :key="column.key"
              class="concept-sector-view__num concept-sector-view__sortable"
              @click="toggleSort(column.key)"
            >
              {{ column.label }}{{ sortIndicator(column.key) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(entry, index) in sortedStocks"
            :key="pickString(entry, ['instrumentId']) || index"
            @click="emit('select', entry)"
          >
            <td class="concept-sector-view__index">{{ index + 1 }}</td>
            <td>{{ pickString(entry, ["symbol", "instrumentId"]) || "--" }}</td>
            <td class="concept-sector-view__name">
              {{ pickString(entry, ["name"]) || "--" }}
            </td>
            <td class="concept-sector-view__num tv-num">
              {{ formatPrice(pickNumber(entry, ["price", "lastPrice"])) }}
            </td>
            <td
              class="concept-sector-view__num tv-num"
              :class="directionClass(pickNumber(entry, ['changeAmount']))"
            >
              {{ formatSigned(pickNumber(entry, ["changeAmount"])) }}
            </td>
            <td
              class="concept-sector-view__num tv-num"
              :class="directionClass(pickNumber(entry, ['changeRate']))"
            >
              {{ formatSigned(pickNumber(entry, ["changeRate"]), "%") }}
            </td>
            <td class="concept-sector-view__num tv-num">
              {{ formatCompactNumber(pickNumber(entry, ["volume"])) }}
            </td>
            <td class="concept-sector-view__num tv-num">
              {{ formatCompactNumber(pickNumber(entry, ["turnover"])) }}
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.concept-sector-view {
  display: grid;
  min-height: 0;
  grid-template-columns: minmax(240px, 1fr) minmax(320px, 2fr);
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.concept-sector-view__plates,
.concept-sector-view__stocks {
  display: flex;
  min-height: 0;
  flex-direction: column;
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.concept-sector-view__warning {
  grid-column: 1 / -1;
  padding: 7px 10px;
  border: 1px solid color-mix(in srgb, var(--tv-warn) 40%, var(--tv-border));
  border-radius: 4px;
  color: var(--tv-warn);
}

.concept-sector-view__head {
  display: flex;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  font-weight: 600;
}

.concept-sector-view__head-sub {
  color: var(--tv-text-dim);
  font-weight: 400;
}

.concept-sector-view__types {
  margin-left: auto;
}

.concept-sector-view__connect {
  height: 24px;
  padding: 0 7px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.concept-sector-view__connect.is-active {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}

.concept-sector-view__status {
  display: grid;
  min-height: 96px;
  place-items: center;
  color: var(--tv-text-dim);
}

.concept-sector-view__table {
  width: 100%;
  border-collapse: collapse;
}

.concept-sector-view__table th,
.concept-sector-view__table td {
  height: 32px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  text-align: left;
  white-space: nowrap;
}

.concept-sector-view__table th {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 500;
}

.concept-sector-view__table tbody tr {
  cursor: pointer;
}

.concept-sector-view__table tbody tr:hover td {
  background: var(--tv-bg-elevated);
}

.concept-sector-view__table tbody tr.is-selected td {
  background: color-mix(in srgb, var(--tv-accent) 12%, transparent);
}

.concept-sector-view__index {
  width: 32px;
  color: var(--tv-text-dim);
  font-variant-numeric: tabular-nums;
}

.concept-sector-view__num {
  text-align: right;
}

.concept-sector-view__name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.concept-sector-view__sortable {
  cursor: pointer;
  user-select: none;
}

.concept-sector-view__sortable:hover {
  color: var(--tv-text);
}

@media (max-width: 720px) {
  .concept-sector-view {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
