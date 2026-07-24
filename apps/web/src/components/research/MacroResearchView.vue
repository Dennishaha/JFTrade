<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import ResearchDataTable from "./ResearchDataTable.vue";
import SparklineChart from "./SparklineChart.vue";
import {
  formatPrice,
  pickNumber,
  pickString,
} from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

type MacroOperation = "indicators" | "fed_target_rate" | "fed_dot_plot";

interface MacroIndicator {
  indicatorId: string;
  name: string;
  category: string;
}

const props = withDefaults(
  defineProps<{
    brokerId?: string;
    operation?: MacroOperation;
    indicatorId?: string;
  }>(),
  { brokerId: "", operation: "indicators", indicatorId: "" },
);
const emit = defineEmits<{
  "update:indicatorId": [indicatorId: string];
}>();

function asRecord(value: unknown): Record<string, unknown> | null {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function asRecords(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is Record<string, unknown> => asRecord(item) != null,
      )
    : [];
}

const listFeature = useResearchFeature(
  () =>
    props.operation === "indicators"
      ? "/api/v1/research/macro?market=US&operation=indicators&pageSize=100"
      : "",
  { expandCN: false, brokerId: () => props.brokerId },
);

const selectedIndicatorId = ref(props.indicatorId);
watch(
  () => props.indicatorId,
  (value) => {
    if (value && value !== selectedIndicatorId.value) {
      selectedIndicatorId.value = value;
    }
  },
);

const indicators = computed<MacroIndicator[]>(() => {
  const result: MacroIndicator[] = [];
  for (const categoryEntry of listFeature.entries.value) {
    const category =
      pickString(categoryEntry, ["categoryName", "name"]) || "其他";
    const nested = asRecords(
      categoryEntry.indicatorList ?? categoryEntry.indicators,
    );
    if (nested.length === 0) {
      const indicatorId = pickString(categoryEntry, ["indicatorId", "id"]);
      if (indicatorId) {
        result.push({
          indicatorId,
          name: pickString(categoryEntry, ["name", "indicatorName"]) || indicatorId,
          category,
        });
      }
      continue;
    }
    for (const indicator of nested) {
      const indicatorId = pickString(indicator, ["indicatorId", "id"]);
      if (!indicatorId) continue;
      result.push({
        indicatorId,
        name: pickString(indicator, ["name", "indicatorName"]) || indicatorId,
        category,
      });
    }
  }
  return result;
});

watch(
  indicators,
  (items) => {
    if (
      items.length > 0 &&
      !items.some((item) => item.indicatorId === selectedIndicatorId.value)
    ) {
      selectIndicator(items[0]!);
    }
  },
  { immediate: true },
);

function selectIndicator(indicator: MacroIndicator): void {
  selectedIndicatorId.value = indicator.indicatorId;
  emit("update:indicatorId", indicator.indicatorId);
}

const historyFeature = useResearchFeature(
  () =>
    props.operation === "indicators" && selectedIndicatorId.value
      ? `/api/v1/research/macro?market=US&operation=indicator_history&indicatorId=${encodeURIComponent(selectedIndicatorId.value)}&pageSize=100`
      : "",
  { expandCN: false, brokerId: () => props.brokerId },
);

const fedFeature = useResearchFeature(
  () =>
    props.operation !== "indicators"
      ? `/api/v1/research/macro?market=US&operation=${encodeURIComponent(props.operation)}&pageSize=100`
      : "",
  { expandCN: false, brokerId: () => props.brokerId },
);

const activeIndicator = computed(() =>
  indicators.value.find(
    (item) => item.indicatorId === selectedIndicatorId.value,
  ),
);
const historyPoints = computed(() =>
  historyFeature.entries.value
    .map((entry) => pickNumber(entry, ["value"]))
    .filter((value): value is number => value != null),
);

function formatMacroValue(
  value: number | null,
  entry: Record<string, unknown>,
): string {
  const formatted = formatPrice(value);
  if (value == null) return formatted;
  const explicitUnit = pickString(entry, ["unit"]);
  if (explicitUnit) return `${formatted}${explicitUnit}`;
  const unitType = pickNumber(entry, ["unitType"]);
  if (unitType === 1) return `${formatted}%`;
  if (unitType === 3) return `${formatted}（指数）`;
  return formatted;
}

const historyColumns: ResearchTableColumn[] = [
  {
    key: "time",
    label: "数据时间",
    value: (entry) => pickString(entry, ["dataTime", "date", "eventDate"]),
  },
  {
    key: "release",
    label: "发布时间",
    value: (entry) => pickString(entry, ["releaseTime"]),
  },
  {
    key: "value",
    label: "实际值",
    align: "right",
    value: (entry) => pickNumber(entry, ["value"]),
    format: (value, entry) =>
      formatMacroValue(value as number | null, entry),
  },
  {
    key: "predict",
    label: "预测值",
    align: "right",
    value: (entry) => pickNumber(entry, ["predictValue", "forecastValue"]),
    format: (value, entry) =>
      formatMacroValue(value as number | null, entry),
  },
  {
    key: "previous",
    label: "前值",
    align: "right",
    value: (entry) => pickNumber(entry, ["previousValue"]),
    format: (value, entry) =>
      formatMacroValue(value as number | null, entry),
  },
];

const targetRateRows = computed<Record<string, unknown>[]>(() => {
  const rows: Record<string, unknown>[] = [];
  for (const meeting of fedFeature.entries.value) {
    const meetingDate = pickString(meeting, ["meetingDate", "date"]);
    const rates = asRecords(meeting.targetRateList ?? meeting.rates);
    for (const rate of rates) {
      rows.push({ ...rate, meetingDate });
    }
  }
  return rows;
});
const targetRateColumns: ResearchTableColumn[] = [
  {
    key: "meeting",
    label: "会议日期",
    value: (entry) => pickString(entry, ["meetingDate"]),
  },
  {
    key: "range",
    label: "目标区间",
    value: (entry) => pickString(entry, ["targetRange", "rateRange"]),
  },
  {
    key: "probability",
    label: "概率",
    align: "right",
    value: (entry) => pickNumber(entry, ["probability"]),
    format: (value) => `${formatPrice(value as number | null)}%`,
  },
];

const dotYears = computed(() =>
  fedFeature.entries.value.map((entry) => ({
    year: pickString(entry, ["year"]) || String(entry.year ?? "--"),
    median: pickNumber(entry, ["medianRate"]),
    dots: asRecords(entry.dotList ?? entry.dots),
  })),
);

const status = computed(() => {
  if (props.operation === "indicators") {
    if (listFeature.loading.value || historyFeature.loading.value) return "加载中…";
    return listFeature.error.value || historyFeature.error.value;
  }
  if (fedFeature.loading.value) return "加载中…";
  return fedFeature.error.value;
});
</script>

<template>
  <section class="macro-research">
    <div v-if="status" class="macro-research__status">{{ status }}</div>
    <template v-else-if="operation === 'indicators'">
      <aside class="macro-research__catalog">
        <header>
          <strong>宏观指标</strong>
          <small>{{ indicators.length }} 项</small>
        </header>
        <nav>
          <button
            v-for="indicator in indicators"
            :key="indicator.indicatorId"
            type="button"
            :class="{ 'is-active': selectedIndicatorId === indicator.indicatorId }"
            @click="selectIndicator(indicator)"
          >
            <span>{{ indicator.name }}</span>
            <small>{{ indicator.category }}</small>
          </button>
        </nav>
      </aside>
      <div class="macro-research__detail">
        <header class="macro-research__detail-head">
          <div>
            <strong>{{ activeIndicator?.name ?? "请选择指标" }}</strong>
            <small>{{ activeIndicator?.category ?? "" }}</small>
          </div>
          <span class="macro-research__spacer" />
          <small v-if="historyFeature.asOf.value">
            更新 {{ historyFeature.asOf.value }}
          </small>
          <button type="button" @click="historyFeature.refresh">刷新</button>
        </header>
        <div v-if="historyPoints.length > 1" class="macro-research__chart">
          <SparklineChart
            :points="historyPoints"
            :width="560"
            :height="86"
            direction="flat"
          />
        </div>
        <ResearchDataTable
          :entries="historyFeature.entries.value"
          :columns="historyColumns"
          empty-label="暂无指标历史数据"
        />
      </div>
    </template>
    <template v-else-if="operation === 'fed_target_rate'">
      <header class="macro-research__toolbar">
        <strong>FedWatch 目标利率概率</strong>
        <span class="macro-research__spacer" />
        <small v-if="fedFeature.asOf.value">更新 {{ fedFeature.asOf.value }}</small>
        <button type="button" @click="fedFeature.refresh">刷新</button>
      </header>
      <div class="macro-research__probability">
        <div
          v-for="row in targetRateRows"
          :key="`${row.meetingDate}-${row.targetRange}`"
          class="macro-research__probability-row"
        >
          <span>{{ row.meetingDate }}</span>
          <strong>{{ row.targetRange }}</strong>
          <div>
            <i :style="{ width: `${Math.max(0, Math.min(100, Number(row.probability ?? 0)))}%` }" />
          </div>
          <b>{{ formatPrice(pickNumber(row, ['probability'])) }}%</b>
        </div>
      </div>
      <ResearchDataTable
        :entries="targetRateRows"
        :columns="targetRateColumns"
        empty-label="暂无目标利率概率"
      />
    </template>
    <template v-else>
      <header class="macro-research__toolbar">
        <strong>美联储点阵图</strong>
        <span class="macro-research__spacer" />
        <small v-if="fedFeature.metadata.value.currentRate">
          当前利率 {{ fedFeature.metadata.value.currentRate }}
        </small>
        <button type="button" @click="fedFeature.refresh">刷新</button>
      </header>
      <div class="macro-research__dots">
        <article v-for="item in dotYears" :key="item.year">
          <header>
            <strong>{{ item.year }}</strong>
            <span>中位数 {{ formatPrice(item.median) }}</span>
          </header>
          <div
            v-for="dot in item.dots"
            :key="String(dot.rate)"
            class="macro-research__dot-row"
          >
            <span>{{ dot.rate }}</span>
            <i
              v-for="vote in Math.max(0, Math.trunc(Number(dot.voteCount ?? 0)))"
              :key="vote"
            />
            <small>{{ dot.voteCount }} 票</small>
          </div>
        </article>
      </div>
    </template>
  </section>
</template>

<style scoped>
.macro-research {
  display: grid;
  min-height: 0;
  height: 100%;
  grid-template-columns: minmax(190px, 260px) minmax(0, 1fr);
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.macro-research__catalog,
.macro-research__detail,
.macro-research__probability,
.macro-research__dots {
  min-height: 0;
}

.macro-research__catalog {
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.macro-research__catalog > header,
.macro-research__detail-head,
.macro-research__toolbar {
  display: flex;
  height: 32px;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.macro-research__catalog > header {
  justify-content: space-between;
}

.macro-research__catalog small,
.macro-research__detail-head small,
.macro-research__toolbar small {
  color: var(--tv-text-dim);
}

.macro-research__catalog nav {
  height: calc(100% - 32px);
  overflow: auto;
}

.macro-research__catalog nav button {
  display: flex;
  width: 100%;
  min-height: 38px;
  flex-direction: column;
  justify-content: center;
  padding: 4px 8px;
  border: 0;
  border-bottom: 1px solid var(--tv-border);
  background: transparent;
  color: var(--tv-text);
  cursor: pointer;
  text-align: left;
}

.macro-research__catalog nav button:hover,
.macro-research__catalog nav button.is-active {
  background: var(--tv-bg-elevated);
}

.macro-research__catalog nav button.is-active {
  box-shadow: inset 2px 0 var(--tv-accent);
}

.macro-research__detail {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
}

.macro-research__detail-head {
  flex: 0 0 auto;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
}

.macro-research__detail-head > div {
  display: flex;
  flex-direction: column;
}

.macro-research__spacer {
  flex: 1;
}

.macro-research button {
  height: 24px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.macro-research__chart {
  min-height: 102px;
  padding: 8px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.macro-research__status {
  display: grid;
  min-height: 160px;
  grid-column: 1 / -1;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.macro-research__toolbar,
.macro-research__probability,
.macro-research__dots {
  grid-column: 1 / -1;
}

.macro-research > :deep(.research-data-table) {
  grid-column: 1 / -1;
}

.macro-research__probability {
  display: grid;
  gap: 4px;
  padding: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.macro-research__probability-row {
  display: grid;
  min-height: 28px;
  grid-template-columns: 100px 90px minmax(120px, 1fr) 64px;
  align-items: center;
  gap: 8px;
}

.macro-research__probability-row > div {
  height: 7px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--tv-bg-elevated);
}

.macro-research__probability-row i {
  display: block;
  height: 100%;
  background: var(--tv-accent);
}

.macro-research__probability-row b {
  text-align: right;
}

.macro-research__dots {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
}

.macro-research__dots article {
  padding: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.macro-research__dots article > header {
  display: flex;
  height: 28px;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--tv-border);
}

.macro-research__dot-row {
  display: flex;
  min-height: 28px;
  align-items: center;
  gap: 4px;
}

.macro-research__dot-row > span {
  width: 52px;
  font-variant-numeric: tabular-nums;
}

.macro-research__dot-row i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--tv-accent);
}

.macro-research__dot-row small {
  margin-left: auto;
  color: var(--tv-text-dim);
}

@media (max-width: 820px) {
  .macro-research {
    grid-template-columns: 1fr;
  }

  .macro-research__catalog {
    max-height: 230px;
  }
}
</style>
