<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import {
  formatOptionMetric,
  type OptionChainSideModel,
} from "../../composables/optionChainModel";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../composables/productFeatures";

type Entry = Record<string, unknown>;
type AnalysisOperation = "quote" | "volatility" | "exercise_probability";

const props = defineProps<{
  contract: OptionChainSideModel | null;
  market: string;
}>();
const emit = defineEmits<{
  close: [];
  openWorkspace: [instrumentId: string];
}>();

const loading = ref(false);
const results = ref<Partial<Record<AnalysisOperation, ProductFeatureResult>>>(
  {},
);
const errors = ref<Partial<Record<AnalysisOperation, string>>>({});
const { selectedBrokerId } = useBrokerProviderSelection();
let requestToken = 0;

const quote = computed<Entry>(
  () => results.value.quote?.entries[0] ?? {},
);
const volatility = computed<Entry>(
  () => results.value.volatility?.entries[0] ?? {},
);
const exercise = computed<Entry>(
  () => results.value.exercise_probability?.entries[0] ?? {},
);
const metrics = computed(() => [
  {
    label: "买价",
    value: formatOptionMetric(props.contract?.bidPrice ?? null),
  },
  {
    label: "卖价",
    value: formatOptionMetric(props.contract?.askPrice ?? null),
  },
  { label: "最新价", value: metric(quote.value, "price", "markPrice", "mid") },
  { label: "行权价", value: metric(quote.value, "strike") },
  {
    label: "IV",
    value: metric(
      quote.value,
      "IV",
      "iv",
      "impliedVolatility",
      volatility.value.impliedVolatility,
      results.value.volatility?.metadata?.averageImpvol,
    ),
  },
  {
    label: "HV",
    value: metric(volatility.value, "historyVolatility"),
  },
  {
    label: "行权概率",
    value: metric(exercise.value, "strikeProbability"),
  },
  { label: "Delta", value: metric(quote.value, "delta") },
  { label: "Gamma", value: metric(quote.value, "gamma") },
  { label: "Theta", value: metric(quote.value, "theta") },
  { label: "Vega", value: metric(quote.value, "vega") },
  { label: "到期日", value: textMetric(quote.value, "expireTime") },
]);

function finiteValue(value: unknown): number | null {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function metric(
  entry: Entry,
  ...keysOrValues: Array<string | unknown>
): string {
  for (const item of keysOrValues) {
    const value =
      typeof item === "string" && Object.hasOwn(entry, item)
        ? entry[item]
        : item;
    const parsed = finiteValue(value);
    if (parsed != null) return formatOptionMetric(parsed);
  }
  return "—";
}

function textMetric(entry: Entry, ...keys: string[]): string {
  for (const key of keys) {
    const value = String(entry[key] ?? "").trim();
    if (value) return value;
  }
  return "—";
}

async function load(): Promise<void> {
  const instrumentId = props.contract?.instrumentId.trim().toUpperCase() ?? "";
  const token = ++requestToken;
  results.value = {};
  errors.value = {};
  if (!instrumentId) {
    loading.value = false;
    return;
  }
  loading.value = true;
  const operations: AnalysisOperation[] = [
    "quote",
    "volatility",
    "exercise_probability",
  ];
  const settled = await Promise.allSettled(
    operations.map((operation) =>
      fetchProductFeature(
        withBrokerProvider(
          `/api/v1/market-data/options/analysis/${encodeURIComponent(instrumentId)}?market=${encodeURIComponent(props.market)}&operation=${operation}&pageSize=30`,
          selectedBrokerId.value,
        ),
      ),
    ),
  );
  if (token !== requestToken) return;
  const nextResults: Partial<Record<AnalysisOperation, ProductFeatureResult>> =
    {};
  const nextErrors: Partial<Record<AnalysisOperation, string>> = {};
  settled.forEach((outcome, index) => {
    const operation = operations[index]!;
    if (outcome.status === "fulfilled") {
      nextResults[operation] = outcome.value;
    } else {
      nextErrors[operation] =
        outcome.reason instanceof Error
          ? outcome.reason.message
          : String(outcome.reason);
    }
  });
  results.value = nextResults;
  errors.value = nextErrors;
  loading.value = false;
}

watch(
  () => [
    props.contract?.instrumentId,
    props.market,
    selectedBrokerId.value,
  ],
  () => void load(),
  { immediate: true },
);
</script>

<template>
  <aside
    v-if="contract"
    class="option-contract-drawer"
    aria-label="期权合约分析"
  >
    <header>
      <div>
        <span>合约分析</span>
        <strong>{{ contract.code }}</strong>
        <small>{{ contract.name }}</small>
      </div>
      <button type="button" aria-label="关闭合约分析" @click="emit('close')">
        ×
      </button>
    </header>

    <v-progress-linear v-if="loading" indeterminate />
    <div class="option-contract-drawer__metrics">
      <div v-for="item in metrics" :key="item.label">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </div>
    </div>

    <v-alert
      v-for="(message, operation) in errors"
      :key="operation"
      type="warning"
      variant="tonal"
      density="compact"
    >
      {{ operation }} 暂不可用：{{ message }}
    </v-alert>

    <footer>
      <v-btn
        size="small"
        variant="tonal"
        @click="emit('openWorkspace', contract.instrumentId)"
      >
        打开合约工作区
      </v-btn>
    </footer>
  </aside>
</template>

<style scoped>
.option-contract-drawer {
  position: absolute;
  z-index: 12;
  top: 40px;
  right: 0;
  bottom: 0;
  width: min(360px, 92%);
  overflow: auto;
  border-left: 1px solid var(--tv-border-strong);
  background: color-mix(in srgb, var(--tv-bg-surface) 97%, transparent);
  box-shadow: -16px 0 34px rgb(0 0 0 / 24%);
  backdrop-filter: blur(12px);
}

header {
  display: flex;
  min-height: 58px;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--tv-border);
}

header > div {
  display: grid;
  gap: 2px;
}

header span,
header small,
.option-contract-drawer__metrics span {
  color: var(--tv-text-dim);
  font-size: 8px;
}

header strong {
  color: var(--tv-text);
  font-size: 12px;
}

header button {
  width: 26px;
  height: 26px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 18px;
}

.option-contract-drawer__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  padding: 1px;
  background: var(--tv-border);
}

.option-contract-drawer__metrics > div {
  display: grid;
  min-height: 58px;
  align-content: center;
  gap: 4px;
  padding: 8px 10px;
  background: var(--tv-bg-surface);
}

.option-contract-drawer__metrics strong {
  color: var(--tv-text);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.option-contract-drawer :deep(.v-alert) {
  margin: 8px 10px 0;
  font-size: 9px;
}

footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px;
  border-top: 1px solid var(--tv-border);
}
</style>
