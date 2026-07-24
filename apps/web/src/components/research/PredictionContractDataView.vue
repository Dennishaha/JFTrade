<script setup lang="ts">
import { computed } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import ResearchDataTable from "./ResearchDataTable.vue";
import {
  formatCompactNumber,
  formatPrice,
  pickNumber,
  pickString,
} from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

type PredictionContractView =
  | "snapshot"
  | "depth"
  | "candles"
  | "ticks"
  | "milestones";

const props = defineProps<{
  path: string;
  view: PredictionContractView;
}>();

function asRecord(value: unknown): Record<string, unknown> | null {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function asRecords(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter(
        (entry): entry is Record<string, unknown> => asRecord(entry) != null,
      )
    : [];
}

const feature = useResearchFeature(() => props.path, { expandCN: false });
const snapshot = computed(() => feature.entries.value[0] ?? {});

function flattenNestedRows(
  keys: readonly string[],
): Record<string, unknown>[] {
  const rows: Record<string, unknown>[] = [];
  for (const wrapper of feature.entries.value) {
    const nested = keys
      .map((key) => asRecords(wrapper[key]))
      .find((items) => items.length > 0);
    if (nested == null) {
      rows.push(wrapper);
      continue;
    }
    const contract = asRecord(wrapper.code);
    for (const item of nested) {
      rows.push({
        ...item,
        contract: contract?.instrumentId ?? contract?.code ?? "",
        predictionSide: wrapper.preSide ?? wrapper.predictionSide,
      });
    }
  }
  return rows;
}

const depthRows = computed<Record<string, unknown>[]>(() => {
  const direct = feature.entries.value.filter(
    (entry) =>
      pickString(entry, ["side", "predictionSide"]) ||
      pickNumber(entry, ["price"]) != null,
  );
  if (direct.length > 1) return direct;
  const source = feature.entries.value[0] ?? {};
  const rows: Record<string, unknown>[] = [];
  const groups: Array<[string, unknown]> = [
    ["YES 买盘", source.yesBidList ?? source.yesBids],
    ["YES 卖盘", source.yesAskList ?? source.yesAsks],
    ["NO 买盘", source.noBidList ?? source.noBids],
    ["NO 卖盘", source.noAskList ?? source.noAsks],
    ["买盘", source.bidList ?? source.bids],
    ["卖盘", source.askList ?? source.asks],
  ];
  for (const [side, value] of groups) {
    for (const item of asRecords(value)) rows.push({ ...item, side });
  }
  return rows;
});

const depthColumns: ResearchTableColumn[] = [
  {
    key: "side",
    label: "方向",
    value: (entry) => pickString(entry, ["side", "predictionSide"]),
  },
  {
    key: "price",
    label: "价格",
    align: "right",
    value: (entry) => pickNumber(entry, ["price", "orderPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "size",
    label: "数量",
    align: "right",
    value: (entry) => pickNumber(entry, ["volume", "size", "quantity"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "orders",
    label: "委托数",
    align: "right",
    value: (entry) => pickNumber(entry, ["orderCount", "orders"]),
  },
];
const candleColumns: ResearchTableColumn[] = [
  {
    key: "time",
    label: "时间",
    value: (entry) =>
      pickString(entry, ["timeKey", "time", "timestamp", "dateTime", "eventTime"]),
  },
  {
    key: "open",
    label: "开",
    align: "right",
    value: (entry) => pickNumber(entry, ["open", "openPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "high",
    label: "高",
    align: "right",
    value: (entry) => pickNumber(entry, ["high", "highPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "low",
    label: "低",
    align: "right",
    value: (entry) => pickNumber(entry, ["low", "lowPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "close",
    label: "收",
    align: "right",
    value: (entry) => pickNumber(entry, ["close", "closePrice", "price"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "volume",
    label: "成交量",
    align: "right",
    value: (entry) => pickNumber(entry, ["volume"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
];
const tickColumns: ResearchTableColumn[] = [
  candleColumns[0]!,
  {
    key: "side",
    label: "方向",
    value: (entry) => pickString(entry, ["side", "predictionSide"]),
  },
  {
    key: "yesPrice",
    label: "YES 价格",
    align: "right",
    value: (entry) => pickNumber(entry, ["yesPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "noPrice",
    label: "NO 价格",
    align: "right",
    value: (entry) => pickNumber(entry, ["noPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "volume",
    label: "数量",
    align: "right",
    value: (entry) => pickNumber(entry, ["volume", "quantity"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
];
const milestoneColumns: ResearchTableColumn[] = [
  {
    key: "time",
    label: "开始时间",
    value: (entry) =>
      pickString(entry, ["startDate", "eventTime", "date", "time", "timestamp"]),
  },
  {
    key: "title",
    label: "里程碑",
    value: (entry) =>
      pickString(entry, ["title", "name", "description"]),
  },
  {
    key: "endDate",
    label: "结束时间",
    value: (entry) => pickString(entry, ["endDate"]),
  },
  {
    key: "type",
    label: "类型",
    value: (entry) => pickString(entry, ["type", "category", "status"]),
  },
];

const columns = computed(() => {
  switch (props.view) {
    case "depth":
      return depthColumns;
    case "candles":
      return candleColumns;
    case "ticks":
      return tickColumns;
    default:
      return milestoneColumns;
  }
});
const rows = computed(() => {
  switch (props.view) {
    case "depth":
      return depthRows.value;
    case "candles":
      return flattenNestedRows(["klineList", "klines"]);
    case "ticks":
      return flattenNestedRows(["tickerList", "ticks"]);
    default:
      return feature.entries.value;
  }
});
</script>

<template>
  <section class="prediction-contract-data">
    <header>
      <strong>
        {{
          {
            snapshot: "合约快照",
            depth: "YES/NO 盘口",
            candles: "价格走势",
            ticks: "逐笔成交",
            milestones: "事件里程碑",
          }[view]
        }}
      </strong>
      <span />
      <small v-if="feature.asOf.value">更新 {{ feature.asOf.value }}</small>
      <button type="button" @click="feature.refresh">刷新</button>
    </header>
    <div v-if="feature.loading.value" class="prediction-contract-data__status">
      加载中…
    </div>
    <div v-else-if="feature.error.value" class="prediction-contract-data__status">
      {{ feature.error.value }}
    </div>
    <div v-else-if="view === 'snapshot'" class="prediction-contract-data__snapshot">
      <div>
        <span>状态</span>
        <strong>{{ pickString(snapshot, ["status", "contractStatus"]) || "--" }}</strong>
      </div>
      <div>
        <span>最新价</span>
        <strong>{{ formatPrice(pickNumber(snapshot, ["price", "lastPrice"])) }}</strong>
      </div>
      <div>
        <span>YES 买一 / 卖一</span>
        <strong>
          {{ formatPrice(pickNumber(snapshot, ["yesBid", "yesPrice"])) }}
          /
          {{ formatPrice(pickNumber(snapshot, ["yesAsk"])) }}
        </strong>
      </div>
      <div>
        <span>NO 买一 / 卖一</span>
        <strong>
          {{ formatPrice(pickNumber(snapshot, ["noBid", "noPrice"])) }}
          /
          {{ formatPrice(pickNumber(snapshot, ["noAsk"])) }}
        </strong>
      </div>
      <div>
        <span>成交量</span>
        <strong>
          {{
            formatCompactNumber(
              pickNumber(snapshot, ["cumulativeVolume", "volume24h", "volume"]),
            )
          }}
        </strong>
      </div>
      <div>
        <span>未平仓</span>
        <strong>{{ formatCompactNumber(pickNumber(snapshot, ["openInterest"])) }}</strong>
      </div>
    </div>
    <ResearchDataTable
      v-else
      :entries="rows"
      :columns="columns"
      empty-label="暂无合约数据"
    />
  </section>
</template>

<style scoped>
.prediction-contract-data {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 8px;
}

.prediction-contract-data > header {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.prediction-contract-data > header > span {
  flex: 1;
}

.prediction-contract-data header small {
  color: var(--tv-text-dim);
}

.prediction-contract-data header button {
  height: 24px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.prediction-contract-data__status {
  display: grid;
  min-height: 120px;
  flex: 1;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.prediction-contract-data__snapshot {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-border);
}

.prediction-contract-data__snapshot > div {
  display: flex;
  min-height: 72px;
  flex-direction: column;
  justify-content: center;
  gap: 5px;
  padding: 8px;
  background: var(--tv-bg-surface);
}

.prediction-contract-data__snapshot span {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.prediction-contract-data__snapshot strong {
  font-size: 16px;
  font-variant-numeric: tabular-nums;
}
</style>
