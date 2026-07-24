<script setup lang="ts">
import { computed, ref } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import ResearchDataTable from "./ResearchDataTable.vue";
import {
  directionClass,
  formatCompactNumber,
  formatPrice,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import type { ResearchTableColumn } from "./researchTable";

type DerivativeScreenOperation = "option_screen" | "warrant";

const props = withDefaults(
  defineProps<{
    operation: DerivativeScreenOperation;
    brokerId?: string;
  }>(),
  { brokerId: "" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

function asRecord(value: unknown): Record<string, unknown> {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function securityLabel(
  entry: Record<string, unknown>,
  field = "security",
): string {
  const security = asRecord(entry[field]);
  const nestedInstrumentId = pickString(security, ["instrumentId"]);
  const market = pickString(security, ["market"]);
  const code = pickString(security, ["code", "symbol"]);
  return (
    nestedInstrumentId ||
    (market && code ? `${market}.${code}` : code)
    || (field === "security" ? pickString(entry, ["instrumentId"]) : "")
  );
}

function warrantTypeCode(entry: Record<string, unknown>): number | null {
  return pickNumber(entry, ["warrantType", "type"]);
}

function warrantTypeLabel(entry: Record<string, unknown>): string {
  const code = warrantTypeCode(entry);
  if (code != null) {
    return (
      {
        1: "认购",
        2: "认沽",
        3: "牛证",
        4: "熊证",
        5: "界内证",
      }[code] ?? `类型 ${code}`
    );
  }
  return pickString(entry, ["warrantType", "type"]) || "—";
}

function optionTypeLabel(entry: Record<string, unknown>): string {
  const code = pickNumber(entry, ["optionType"]);
  if (code != null) {
    return (
      {
        1: "看涨",
        2: "看跌",
      }[code] ?? `类型 ${code}`
    );
  }
  const value = pickString(entry, ["optionType"]).toLowerCase();
  if (value === "call") return "看涨";
  if (value === "put") return "看跌";
  return value || "—";
}

function optionUnderlyingLabel(entry: Record<string, unknown>): string {
  const underlying = asRecord(entry.underlyingInfo);
  const stockId = pickString(underlying, ["stockID", "stockId"]);
  return stockId ? `Stock ID ${stockId}` : "—";
}

function formatDerivativeDate(value: unknown): string {
  const raw =
    typeof value === "number" && Number.isFinite(value)
      ? String(Math.trunc(value))
      : typeof value === "string"
        ? value.trim()
        : "";
  const calendarDate = raw.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (calendarDate) {
    const [, year, month, day] = calendarDate;
    const date = new Date(
      Date.UTC(Number(year), Number(month) - 1, Number(day)),
    );
    if (
      date.getUTCFullYear() === Number(year) &&
      date.getUTCMonth() + 1 === Number(month) &&
      date.getUTCDate() === Number(day)
    ) {
      return `${year}-${month}-${day}`;
    }
    return raw;
  }

  const seconds =
    typeof value === "number"
      ? value
      : typeof value === "string" && value.trim() !== ""
        ? Number(value)
        : Number.NaN;
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return typeof value === "string" && value.trim() !== "" ? value : "—";
  }
  const date = new Date(seconds * 1000);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

const keyword = ref("");
const market = computed(() => (props.operation === "warrant" ? "HK" : "US"));
const path = computed(() =>
  props.operation === "warrant"
    ? "/api/v1/market-data/warrants?market=HK&operation=screen&pageSize=50"
    : "/api/v1/market-data/options/screens?market=US&operation=screen&pageSize=50",
);
const feature = useResearchFeature(path, {
  expandCN: false,
  brokerId: () => props.brokerId,
});
const visibleEntries = computed(() => {
  const value = keyword.value.trim().toLocaleLowerCase();
  if (!value) return feature.entries.value;
  return feature.entries.value.filter((entry) =>
    [
      securityLabel(
        entry,
        "security",
      ),
      securityLabel(entry, "owner"),
      securityLabel(entry, "ownerSecurity"),
      pickString(entry, ["optionName", "name", "ownerName"]),
      optionUnderlyingLabel(entry),
    ]
      .join(" ")
      .toLocaleLowerCase()
      .includes(value),
  );
});

const optionColumns: ResearchTableColumn[] = [
  {
    key: "contract",
    label: "期权合约",
    value: (entry) =>
      pickString(entry, ["optionName", "name"]) || securityLabel(entry),
  },
  {
    key: "underlying",
    label: "标的股票 ID",
    value: (entry) => optionUnderlyingLabel(entry),
  },
  {
    key: "type",
    label: "方向",
    value: (entry) => optionTypeLabel(entry),
  },
  {
    key: "expiry",
    label: "到期日",
    value: (entry) => entry.strikeDate,
    format: (value) => formatDerivativeDate(value),
  },
  {
    key: "strike",
    label: "行权价",
    align: "right",
    value: (entry) => pickNumber(entry, ["strikePrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "price",
    label: "最新价",
    align: "right",
    value: (entry) => pickNumber(entry, ["price", "currentPrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "change",
    label: "涨跌幅",
    align: "right",
    value: (entry) => pickNumber(entry, ["changeRate"]),
    format: (value) => formatSigned(value as number | null, "%"),
    className: (value) => directionClass(value as number | null),
  },
  {
    key: "volume",
    label: "成交量",
    align: "right",
    value: (entry) => pickNumber(entry, ["volume"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "openInterest",
    label: "持仓量",
    align: "right",
    value: (entry) => pickNumber(entry, ["openInterest"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "iv",
    label: "IV",
    align: "right",
    value: (entry) => pickNumber(entry, ["impliedVolatility", "IV", "iv"]),
    format: (value) => `${formatPrice(value as number | null)}%`,
  },
  {
    key: "delta",
    label: "Delta",
    align: "right",
    value: (entry) =>
      pickNumber(entry, ["delta"]) ??
      pickNumber(asRecord(entry.Greeks ?? entry.greeks), ["delta"]),
    format: (value) => formatPrice(value as number | null),
  },
];

const warrantColumns: ResearchTableColumn[] = [
  {
    key: "name",
    label: "轮证",
    value: (entry) =>
      pickString(entry, ["name"]) || securityLabel(entry, "security"),
  },
  {
    key: "underlying",
    label: "正股",
    value: (entry) =>
      pickString(entry, ["ownerName"]) ||
      securityLabel(entry, "owner") ||
      securityLabel(entry, "ownerSecurity"),
  },
  {
    key: "type",
    label: "类型",
    value: (entry) => warrantTypeLabel(entry),
  },
  {
    key: "price",
    label: "最新价",
    align: "right",
    value: (entry) => pickNumber(entry, ["curPrice", "currentPrice", "price"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "strike",
    label: "行权价",
    align: "right",
    value: (entry) => pickNumber(entry, ["strikePrice"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "maturity",
    label: "到期日",
    value: (entry) => entry.maturityTime ?? entry.maturityDate,
    format: (value) => formatDerivativeDate(value),
  },
  {
    key: "turnover",
    label: "成交额",
    align: "right",
    value: (entry) => pickNumber(entry, ["turnover"]),
    format: (value) => formatCompactNumber(value as number | null),
  },
  {
    key: "iv",
    label: "IV",
    align: "right",
    value: (entry) => pickNumber(entry, ["impliedVolatility", "IV", "iv"]),
    format: (value) => `${formatPrice(value as number | null)}%`,
  },
  {
    key: "premium",
    label: "溢价",
    align: "right",
    value: (entry) => pickNumber(entry, ["premium"]),
    format: (value) => `${formatPrice(value as number | null)}%`,
  },
  {
    key: "leverage",
    label: "实际杠杆",
    align: "right",
    value: (entry) => pickNumber(entry, ["leverage", "effectiveLeverage"]),
    format: (value) => formatPrice(value as number | null),
  },
  {
    key: "streetRate",
    label: "街货率",
    align: "right",
    value: (entry) => pickNumber(entry, ["streetRate"]),
    format: (value) => `${formatPrice(value as number | null)}%`,
  },
];

function rowKey(
  entry: Record<string, unknown>,
  index: number,
): string {
  return (
    securityLabel(
      entry,
      "security",
    ) || `${props.operation}:${index}`
  );
}

function normalizedEntry(
  entry: Record<string, unknown>,
): Record<string, unknown> {
  const instrumentId = securityLabel(entry, "security");
  const warrantType = pickString(entry, ["type", "warrantType"]).toLowerCase();
  const warrantTypeNumber = warrantTypeCode(entry);
  const productClass =
    props.operation === "option_screen"
      ? "option"
      : warrantTypeNumber === 3 ||
          warrantTypeNumber === 4 ||
          warrantType.includes("bull") ||
          warrantType.includes("bear")
        ? "cbbc"
        : "warrant";
  return {
    ...entry,
    ...(instrumentId ? { instrumentId } : {}),
    productClass,
  };
}

function selectEntry(entry: Record<string, unknown>): void {
  emit("select", normalizedEntry(entry));
}

function openEntry(entry: Record<string, unknown>): void {
  emit("open", normalizedEntry(entry));
}
</script>

<template>
  <section class="derivative-screen">
    <header class="derivative-screen__toolbar">
      <strong>{{ operation === "warrant" ? "港股轮证筛选" : "期权筛选" }}</strong>
      <input
        v-model="keyword"
        type="search"
        :placeholder="operation === 'warrant' ? '搜索轮证或正股' : '搜索合约或标的'"
      />
      <span />
      <small>{{ market }} · {{ visibleEntries.length }} 条</small>
      <small v-if="feature.asOf.value">更新 {{ feature.asOf.value }}</small>
      <button type="button" @click="feature.refresh">刷新</button>
    </header>
    <div v-if="feature.loading.value" class="derivative-screen__status">加载中…</div>
    <div v-else-if="feature.error.value" class="derivative-screen__status">
      {{ feature.error.value }}
    </div>
    <ResearchDataTable
      v-else
      :entries="visibleEntries"
      :columns="operation === 'warrant' ? warrantColumns : optionColumns"
      :row-key="rowKey"
      :empty-label="operation === 'warrant' ? '暂无符合条件的轮证' : '暂无符合条件的期权'"
      @select="selectEntry"
      @open="openEntry"
    />
    <button
      v-if="feature.hasMore.value"
      class="derivative-screen__more"
      type="button"
      :disabled="feature.loadingMore.value"
      @click="feature.loadMore"
    >
      {{ feature.loadingMore.value ? "加载中…" : "加载更多" }}
    </button>
  </section>
</template>

<style scoped>
.derivative-screen {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.derivative-screen__toolbar {
  display: flex;
  min-height: 32px;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.derivative-screen__toolbar strong {
  flex: 0 0 auto;
  white-space: nowrap;
}

.derivative-screen__toolbar input {
  width: min(280px, 100%);
  min-width: 120px;
  height: 28px;
  flex: 0 1 280px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  outline: 0;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font: inherit;
}

.derivative-screen__toolbar input:focus {
  border-color: var(--tv-accent);
}

.derivative-screen__toolbar > span {
  flex: 1;
}

.derivative-screen__toolbar small {
  min-width: 0;
  color: var(--tv-text-dim);
}

.derivative-screen__toolbar button,
.derivative-screen__more {
  height: 28px;
  padding: 0 9px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.derivative-screen__status {
  display: grid;
  min-height: 140px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.derivative-screen__more {
  align-self: center;
}

@media (max-width: 640px) {
  .derivative-screen__toolbar strong,
  .derivative-screen__toolbar input {
    flex-basis: 100%;
  }

  .derivative-screen__toolbar input {
    width: 100%;
    max-width: none;
  }

  .derivative-screen__toolbar > span {
    display: none;
  }
}
</style>
