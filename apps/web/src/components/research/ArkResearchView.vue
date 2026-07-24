<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import ResearchDataTable from "./ResearchDataTable.vue";
import {
  directionClass,
  formatCompactNumber,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

type ArkOperation = "ark_fund_holdings" | "ark_transactions";

const props = withDefaults(
  defineProps<{
    market?: string;
    brokerId?: string;
    operation?: ArkOperation;
  }>(),
  { market: "US", brokerId: "", operation: "ark_fund_holdings" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

const holdingType = ref(0);
const cycleType = ref(0);
watch(
  () => props.operation,
  () => {
    holdingType.value = 0;
    cycleType.value = 0;
  },
);
const path = computed(() => {
  const params = new URLSearchParams({
    market: props.market,
    operation: props.operation,
    pageSize: "50",
    holdingType: String(holdingType.value),
    cycleType: String(cycleType.value),
  });
  return `/api/v1/research/institutions?${params}`;
});
const feature = useResearchFeature(path, {
  expandCN: false,
  brokerId: () => props.brokerId,
});
const holdings = computed(() => props.operation === "ark_fund_holdings");

const holdingColumns: ResearchTableColumn[] = [
  {
    key: "instrument",
    label: "证券",
    value: (entry) =>
      pickString(entry, ["name", "instrumentId", "symbol"]),
  },
  {
    key: "shares",
    label: "持有股数",
    align: "right",
    value: (entry) => pickNumber(entry, ["shares"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "sharesChange",
    label: "股数变化",
    align: "right",
    value: (entry) => pickNumber(entry, ["sharesChange"]),
    format: (value) => formatSigned(value as number | null),
    className: (value) => directionClass(value as number | null),
  },
  {
    key: "marketValue",
    label: "持仓市值",
    align: "right",
    value: (entry) => pickNumber(entry, ["marketValue"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "weight",
    label: "权重",
    align: "right",
    value: (entry) => pickNumber(entry, ["weight"]),
    format: (value) => formatSigned(value as number | null, "%"),
  },
  {
    key: "weightChange",
    label: "权重变化",
    align: "right",
    value: (entry) => pickNumber(entry, ["weightChange"]),
    format: (value) => formatSigned(value as number | null, "%"),
    className: (value) => directionClass(value as number | null),
  },
];

const transactionColumns: ResearchTableColumn[] = [
  {
    key: "instrument",
    label: "证券",
    value: (entry) =>
      pickString(entry, ["name", "instrumentId", "symbol"]),
  },
  {
    key: "changeShares",
    label: "变动股数",
    align: "right",
    value: (entry) => pickNumber(entry, ["changeShares"]),
    format: (value) => formatSigned(value as number | null),
    className: (value) => directionClass(value as number | null),
  },
  {
    key: "changeAmount",
    label: "变动金额",
    align: "right",
    value: (entry) => pickNumber(entry, ["changeAmount"]),
    format: (value) => formatSigned(value as number | null),
    className: (value) => directionClass(value as number | null),
  },
];

function rowKey(
  entry: Record<string, unknown>,
  index: number,
): string {
  return [
    pickString(
      (entry.security as Record<string, unknown> | undefined) ?? entry,
      ["instrumentId", "symbol", "code"],
    ),
    index,
  ].join(":");
}
</script>

<template>
  <section class="ark-research">
    <header class="ark-research__toolbar">
      <div class="ark-research__toolbar-controls">
        <strong>{{ holdings ? "ARK 持仓" : "ARK 交易动态" }}</strong>
        <select v-model.number="holdingType" aria-label="持仓变化类型">
          <option v-if="holdings" :value="0">当前持仓</option>
          <option :value="holdings ? 1 : 0">增持</option>
          <option :value="holdings ? 2 : 1">减持</option>
          <option :value="holdings ? 3 : 2">建仓</option>
          <option :value="holdings ? 4 : 3">清仓</option>
        </select>
        <select
          v-if="!holdings || holdingType !== 0"
          v-model.number="cycleType"
          aria-label="统计周期"
        >
          <option :value="0">近 1 天</option>
          <option :value="1">近 5 天</option>
          <option :value="2">近 10 天</option>
          <option :value="3">近 30 天</option>
          <option :value="4">近 60 天</option>
        </select>
      </div>
      <div class="ark-research__toolbar-meta">
        <small v-if="feature.total.value">{{ feature.total.value }} 条</small>
        <small v-if="feature.asOf.value">更新 {{ feature.asOf.value }}</small>
        <button type="button" @click="feature.refresh">刷新</button>
      </div>
    </header>
    <div v-if="feature.loading.value" class="ark-research__status">加载中…</div>
    <div v-else-if="feature.error.value" class="ark-research__status">
      {{ feature.error.value }}
    </div>
    <ResearchDataTable
      v-else
      :entries="feature.entries.value"
      :columns="holdings ? holdingColumns : transactionColumns"
      :row-key="rowKey"
      empty-label="暂无 ARK 数据"
      @select="emit('select', $event)"
      @open="emit('open', $event)"
    />
    <button
      v-if="feature.hasMore.value"
      class="ark-research__more"
      type="button"
      :disabled="feature.loadingMore.value"
      @click="feature.loadMore"
    >
      {{ feature.loadingMore.value ? "加载中…" : "加载更多" }}
    </button>
  </section>
</template>

<style scoped>
.ark-research {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.ark-research__toolbar {
  display: flex;
  min-height: 32px;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.ark-research__toolbar-controls,
.ark-research__toolbar-meta {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.ark-research__toolbar-controls {
  flex: 1 1 240px;
}

.ark-research__toolbar-meta {
  flex: 0 1 auto;
  justify-content: flex-end;
}

.ark-research__toolbar small {
  color: var(--tv-text-dim);
  white-space: nowrap;
}

.ark-research__toolbar button,
.ark-research__toolbar select,
.ark-research__more {
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.ark-research__toolbar select {
  min-width: 92px;
}

.ark-research__status {
  display: grid;
  min-height: 120px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.ark-research__more {
  align-self: center;
}

@media (max-width: 520px) {
  .ark-research__toolbar-meta {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
