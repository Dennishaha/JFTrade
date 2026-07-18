<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../composables/productFeatures";
import { useConsoleData } from "../../composables/useConsoleData";

type PredictionView = "contract" | "depth" | "chart" | "ticks" | "rules";
type Entry = Record<string, unknown>;

const props = defineProps<{
  instrumentId: string;
  view: PredictionView;
}>();

const { selectedBrokerAccount, systemStatus } = useConsoleData();
const { selectedBrokerId } = useBrokerProviderSelection();
const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const visible = ref(
  typeof document === "undefined" || document.visibilityState === "visible",
);
const lease = ref<{ leaseId: string; instrumentId: string } | null>(null);
let generation = 0;
let refreshTimer: ReturnType<typeof setInterval> | undefined;

const code = computed(() =>
  props.instrumentId.trim().toUpperCase().replace(/^US\./, ""),
);
const endpoint = computed(() => {
  const base = `/api/v1/market-data/prediction/contracts/${encodeURIComponent(code.value)}`;
  const suffix: Record<PredictionView, string> = {
    contract: "snapshot",
    depth: "order-book?pageSize=20",
    chart: "candles?pageSize=200",
    ticks: "ticks?pageSize=100",
    rules: "milestones?pageSize=100",
  };
  return withBrokerProvider(`${base}/${suffix[props.view]}`, selectedBrokerId.value);
});
const subscriptionType = computed(() => {
  if (props.view === "depth") return "ORDER_BOOK";
  if (props.view === "chart") return "KLINE";
  if (props.view === "ticks") return "TICKER";
  return "";
});
const firstEntry = computed<Entry>(() => result.value?.entries[0] ?? {});
const snapshotMetrics = computed(() => [
  ["状态", firstEntry.value.status],
  ["最新 YES", firstEntry.value.price],
  ["YES 买 / 卖", joinPrice(firstEntry.value.yesBid, firstEntry.value.yesAsk)],
  ["NO 买 / 卖", joinPrice(firstEntry.value.noBid, firstEntry.value.noAsk)],
  ["24h 成交", firstEntry.value.volume24h],
  ["持仓量", firstEntry.value.openInterest],
]);
const depthRows = computed(() => {
  const entry = firstEntry.value;
  const columns = [
    ["YES 买", entry.yesBids],
    ["YES 卖", entry.yesAsks],
    ["NO 买", entry.noBids],
    ["NO 卖", entry.noAsks],
  ] as const;
  const count = Math.max(
    0,
    ...columns.map(([, values]) => (Array.isArray(values) ? values.length : 0)),
  );
  return Array.from({ length: count }, (_, index) => ({
    level: index + 1,
    values: columns.map(([label, values]) => ({
      label,
      value: Array.isArray(values) ? asEntry(values[index]) : {},
    })),
  }));
});
const seriesRows = computed<Entry[]>(() => {
  const entry = firstEntry.value;
  const key = props.view === "chart" ? "klineList" : "tickerList";
  return Array.isArray(entry[key]) ? entry[key].map(asEntry) : [];
});

function asEntry(value: unknown): Entry {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? (value as Entry)
    : {};
}

function display(value: unknown): string {
  if (value == null || value === "") return "—";
  if (typeof value === "number") return value.toLocaleString("zh-CN");
  return String(value);
}

function joinPrice(bid: unknown, ask: unknown): string {
  return `${display(bid)} / ${display(ask)}`;
}

function accountQuery(): string {
  const params = new URLSearchParams();
  const brokerId =
    selectedBrokerId.value ||
    selectedBrokerAccount.value?.brokerId ||
    systemStatus.value.defaultBroker;
  if (brokerId) params.set("brokerId", brokerId);
  if (
    selectedBrokerAccount.value?.accountId &&
    selectedBrokerAccount.value.brokerId === brokerId
  ) {
    params.set("accountId", selectedBrokerAccount.value.accountId);
  }
  return params.toString() ? `?${params.toString()}` : "";
}

async function release(): Promise<void> {
  const current = lease.value;
  lease.value = null;
  if (current == null) return;
  try {
    await fetchEnvelopeWithInit(
      `/api/v1/market-data/prediction/contracts/${encodeURIComponent(current.instrumentId)}/subscriptions/${encodeURIComponent(current.leaseId)}`,
      { method: "DELETE" },
    );
  } catch {
    // Disconnecting OpenD releases its server-side subscriptions as well.
  }
}

async function synchronizeSubscription(): Promise<void> {
  const currentGeneration = ++generation;
  await release();
  if (!visible.value || !code.value || !subscriptionType.value) return;
  const acquired = await fetchEnvelopeWithInit<{
    leaseId: string;
    instrumentId: string;
  }>(
    `/api/v1/market-data/prediction/contracts/${encodeURIComponent(code.value)}/subscriptions${accountQuery()}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ dataTypes: [subscriptionType.value] }),
    },
  );
  if (currentGeneration !== generation) {
    lease.value = acquired;
    await release();
    return;
  }
  lease.value = acquired;
}

async function load(): Promise<void> {
  if (!code.value || !visible.value) return;
  loading.value = true;
  error.value = "";
  try {
    result.value = await fetchProductFeature(endpoint.value);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    loading.value = false;
  }
}

function handleVisibility(): void {
  visible.value =
    typeof document === "undefined" || document.visibilityState === "visible";
}

watch(
  [code, () => props.view, selectedBrokerId, visible],
  async () => {
    try {
      await synchronizeSubscription();
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : String(cause);
    }
    await load();
  },
);

onMounted(() => {
  document.addEventListener("visibilitychange", handleVisibility);
  void synchronizeSubscription().then(load);
  // Snapshot polling is only a fallback until a push is observed by the shared
  // connection; it is scoped to the one visible contract and current view.
  refreshTimer = setInterval(() => {
    if (visible.value && (lease.value != null || props.view === "contract")) {
      void load();
    }
  }, 3_000);
});

onUnmounted(() => {
  generation++;
  if (refreshTimer != null) clearInterval(refreshTimer);
  document.removeEventListener("visibilitychange", handleVisibility);
  void release();
});
</script>

<template>
  <section class="prediction-contract">
    <header class="prediction-contract__header">
      <strong>
        {{
          view === "depth"
            ? "预测盘口"
            : view === "chart"
              ? "预测图表"
              : view === "ticks"
                ? "预测逐笔"
                : view === "rules"
                  ? "合约规则"
                  : "合约行情"
        }}
      </strong>
    </header>
    <v-progress-linear v-if="loading" indeterminate />
    <v-alert v-if="error" type="warning" variant="tonal" density="compact">
      {{ error }}
    </v-alert>
    <v-alert
      v-for="warning in result?.warnings ?? []"
      :key="warning"
      type="info"
      variant="tonal"
      density="compact"
    >
      {{ warning }}
    </v-alert>

    <div v-if="props.view === 'contract'" class="prediction-contract__metrics">
      <div v-for="[label, value] in snapshotMetrics" :key="String(label)">
        <span>{{ label }}</span>
        <strong>{{ display(value) }}</strong>
      </div>
    </div>

    <div v-else-if="props.view === 'depth'" class="prediction-contract__table-wrap">
      <table class="prediction-contract__table">
        <thead>
          <tr>
            <th>档位</th>
            <th>YES 买</th>
            <th>YES 卖</th>
            <th>NO 买</th>
            <th>NO 卖</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in depthRows" :key="row.level">
            <td>{{ row.level }}</td>
            <td v-for="item in row.values" :key="item.label">
              {{ display(item.value.price) }}
              <small>{{ display(item.value.size) }}</small>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-else-if="props.view === 'chart' || props.view === 'ticks'"
      class="prediction-contract__table-wrap"
    >
      <table class="prediction-contract__table">
        <thead>
          <tr v-if="props.view === 'chart'">
            <th>时间</th><th>开</th><th>高</th><th>低</th><th>收</th><th>成交量</th>
          </tr>
          <tr v-else>
            <th>时间</th><th>方向</th><th>YES</th><th>NO</th><th>成交量</th><th>序号</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, index) in seriesRows" :key="String(row.sequence ?? row.timeKey ?? row.time ?? index)">
            <template v-if="props.view === 'chart'">
              <td>{{ display(row.timeKey) }}</td><td>{{ display(row.open) }}</td>
              <td>{{ display(row.high) }}</td><td>{{ display(row.low) }}</td>
              <td>{{ display(row.close) }}</td><td>{{ display(row.volume) }}</td>
            </template>
            <template v-else>
              <td>{{ display(row.time) }}</td><td>{{ display(row.side) }}</td>
              <td>{{ display(row.yesPrice) }}</td><td>{{ display(row.noPrice) }}</td>
              <td>{{ display(row.volume) }}</td><td>{{ display(row.sequence) }}</td>
            </template>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-else class="prediction-contract__rules">
      <article v-for="(entry, index) in result?.entries ?? []" :key="String(entry.instrumentId ?? index)">
        <strong>{{ display(entry.title) }}</strong>
        <span>{{ display(entry.type) }} · {{ display(entry.startDate) }} — {{ display(entry.endDate) }}</span>
        <p>{{ display(entry.notificationMessage) }}</p>
      </article>
    </div>

    <div v-if="!loading && !error && (result?.entries.length ?? 0) === 0" class="prediction-contract__empty">
      当前没有可展示的数据。
    </div>
  </section>
</template>

<style scoped>
.prediction-contract {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: auto;
  background: var(--tv-bg-surface);
}
.prediction-contract__header {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 12px;
  border-bottom: 1px solid var(--tv-border);
}
.prediction-contract__header > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
}
.prediction-contract__header strong { font-size: 13px; }
.prediction-contract__header span,
.prediction-contract__table small,
.prediction-contract__rules span {
  color: var(--tv-text-muted);
  font-size: 10px;
}
.prediction-contract__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(120px, 1fr));
  gap: 1px;
  background: var(--tv-border);
}
.prediction-contract__metrics > div {
  display: flex;
  min-height: 76px;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
  padding: 12px;
  background: var(--tv-bg-surface);
}
.prediction-contract__metrics span { color: var(--tv-text-muted); font-size: 10px; }
.prediction-contract__metrics strong { font-size: 17px; }
.prediction-contract__table-wrap { min-height: 0; overflow: auto; }
.prediction-contract__table { width: 100%; border-collapse: collapse; font-size: 11px; }
.prediction-contract__table th,
.prediction-contract__table td {
  padding: 7px 10px;
  border-bottom: 1px solid var(--tv-border);
  text-align: right;
  white-space: nowrap;
}
.prediction-contract__table th:first-child,
.prediction-contract__table td:first-child { text-align: left; }
.prediction-contract__table td small { display: block; }
.prediction-contract__rules {
  display: grid;
  gap: 8px;
  padding: 10px;
}
.prediction-contract__rules article {
  padding: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}
.prediction-contract__rules article > * { display: block; margin: 0 0 4px; }
.prediction-contract__rules p { color: var(--tv-text-muted); font-size: 11px; }
.prediction-contract__empty { padding: 24px; color: var(--tv-text-muted); text-align: center; }
@media (max-width: 760px) {
  .prediction-contract__metrics { grid-template-columns: repeat(2, minmax(100px, 1fr)); }
}
</style>
