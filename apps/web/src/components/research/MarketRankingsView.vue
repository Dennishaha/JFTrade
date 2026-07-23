<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import RankListPanel from "./RankListPanel.vue";
import { pickString } from "./researchEntry";
import {
  mergeResearchSnapshot,
  useResearchSnapshots,
} from "./researchSnapshots";

const props = withDefaults(
  defineProps<{
    market?: string;
    brokerId?: string;
    initialOperation?: string;
  }>(),
  { market: "US", brokerId: "", initialOperation: "top_gainers" },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

type ProductScope = "stock" | "fund";
const productScope = ref<ProductScope>(
  props.initialOperation === "fund_catalog" ? "fund" : "stock",
);
const operation = ref(props.initialOperation || "top_gainers");

interface OperationOption {
  value: string;
  label: string;
}

const STOCK_OPERATIONS: Record<string, OperationOption[]> = {
  US: [
    { value: "top_gainers", label: "领涨" },
    { value: "top_losers", label: "领跌" },
    { value: "hot", label: "热门" },
    { value: "pre_market", label: "盘前" },
    { value: "after_hours", label: "盘后" },
    { value: "overnight", label: "夜盘" },
  ],
  HK: [
    { value: "top_gainers", label: "领涨" },
    { value: "top_losers", label: "领跌" },
    { value: "hot", label: "热门" },
    { value: "high_dividend_state", label: "高股息" },
  ],
  CN: [],
};

const operationOptions = computed(
  () => STOCK_OPERATIONS[props.market] ?? STOCK_OPERATIONS.US!,
);

watch([() => props.market, productScope], () => {
  if (productScope.value === "fund") {
    operation.value = "fund_catalog";
    return;
  }
  if (!operationOptions.value.some((item) => item.value === operation.value)) {
    operation.value = operationOptions.value[0]?.value ?? "";
  }
}, { immediate: true });

function operationPath(): string {
  if (productScope.value === "fund") {
    return `/api/v1/research/rankings?market=${encodeURIComponent(props.market)}&operation=fund_catalog&pageSize=50`;
  }
  if (!operation.value) return "";
  const direction =
    operation.value === "top_gainers"
      ? "&direction=up"
      : operation.value === "top_losers"
        ? "&direction=down"
        : "";
  const backendOperation = operation.value.startsWith("top_")
    ? "top_movers"
    : operation.value;
  return `/api/v1/research/rankings?market=${encodeURIComponent(props.market)}&operation=${backendOperation}&pageSize=50${direction}`;
}

const feature = useResearchFeature(operationPath, {
  brokerId: () => props.brokerId,
});
const snapshots = useResearchSnapshots(
  () =>
    feature.entries.value
      .map((entry) => pickString(entry, ["instrumentId"]))
      .filter(Boolean),
  () => props.brokerId,
);

const assetClass = ref("");
const fundRanking = ref<"active" | "gainers" | "losers">("active");
const enrichedEntries = computed(() =>
  feature.entries.value.map((entry) =>
    mergeResearchSnapshot(
      entry,
      snapshots.byInstrumentId.value[
        pickString(entry, ["instrumentId"]).toUpperCase()
      ],
    ),
  ),
);

function assetClassLabel(entry: Record<string, unknown>): string {
  const value = pickString(entry, ["assetClass"]);
  return value === "" || /^unknown?$/i.test(value) ? "未分类" : value;
}

const assetClasses = computed(() => [
  ...new Set(
    enrichedEntries.value.map(assetClassLabel),
  ),
]);

watch(productScope, (scope) => {
  assetClass.value = "";
  operation.value =
    scope === "fund" ? "fund_catalog" : operationOptions.value[0]?.value ?? "";
});

const visibleEntries = computed(() =>
  enrichedEntries.value
    .filter((entry) => {
      if (productScope.value !== "fund" || !assetClass.value) return true;
      return assetClassLabel(entry) === assetClass.value;
    })
    .sort((left, right) => {
      if (productScope.value !== "fund") return 0;
      const leftValue = Number(
        fundRanking.value === "active"
          ? left.turnover ?? left.volume
          : left.changeRate,
      );
      const rightValue = Number(
        fundRanking.value === "active"
          ? right.turnover ?? right.volume
          : right.changeRate,
      );
      const safeLeft = Number.isFinite(leftValue) ? leftValue : -Infinity;
      const safeRight = Number.isFinite(rightValue) ? rightValue : -Infinity;
      return fundRanking.value === "losers"
        ? safeLeft - safeRight
        : safeRight - safeLeft;
    }),
);

const assetClassSummary = computed(() => {
  const summary = new Map<string, { count: number; turnover: number }>();
  for (const entry of enrichedEntries.value) {
    const name = assetClassLabel(entry);
    const current = summary.get(name) ?? { count: 0, turnover: 0 };
    current.count += 1;
    const turnover = Number(entry.turnover ?? 0);
    if (Number.isFinite(turnover)) current.turnover += turnover;
    summary.set(name, current);
  }
  return [...summary.entries()]
    .map(([name, value]) => ({ name, ...value }))
    .sort((left, right) => right.turnover - left.turnover || right.count - left.count);
});

const valuePresentation = computed(() => {
  if (operation.value === "hot") {
    return { field: "averageHeat", label: "综合热度", title: "热门榜" };
  }
  if (operation.value === "high_dividend_state") {
    return { field: "dividendYieldTTM", label: "股息率", title: "高股息" };
  }
  if (productScope.value === "fund") {
    return fundRanking.value === "active"
      ? { field: "turnover", label: "成交额", title: "ETF / 基金活跃榜" }
      : {
          field: "changeRate",
          label: "涨跌幅",
          title: fundRanking.value === "gainers" ? "ETF / 基金领涨榜" : "ETF / 基金领跌榜",
        };
  }
  return {
    field: "changeRate",
    label: "涨跌幅",
    title:
      operationOptions.value.find((item) => item.value === operation.value)
        ?.label ?? "市场榜单",
  };
});
</script>

<template>
  <div class="market-rankings-view">
    <div class="market-rankings-view__toolbar">
      <span class="tv-seg">
        <button type="button" :class="{ 'is-active': productScope === 'stock' }" @click="productScope = 'stock'">股票</button>
        <button type="button" :class="{ 'is-active': productScope === 'fund' }" @click="productScope = 'fund'">ETF / 基金</button>
      </span>
      <span v-if="productScope === 'stock'" class="tv-seg market-rankings-view__operations">
        <button
          v-for="item in operationOptions"
          :key="item.value"
          type="button"
          :class="{ 'is-active': operation === item.value }"
          @click="operation = item.value"
        >{{ item.label }}</button>
      </span>
      <select v-else v-model="assetClass" class="market-rankings-view__select">
        <option value="">全部资产类别</option>
        <option v-for="item in assetClasses" :key="item" :value="item">{{ item }}</option>
      </select>
      <span v-if="productScope === 'fund'" class="tv-seg">
        <button type="button" :class="{ 'is-active': fundRanking === 'active' }" @click="fundRanking = 'active'">活跃</button>
        <button type="button" :class="{ 'is-active': fundRanking === 'gainers' }" @click="fundRanking = 'gainers'">领涨</button>
        <button type="button" :class="{ 'is-active': fundRanking === 'losers' }" @click="fundRanking = 'losers'">领跌</button>
      </span>
      <span class="market-rankings-view__count">{{ feature.total.value }} 条</span>
    </div>

    <div v-for="warning in feature.warnings.value" :key="warning" class="market-rankings-view__warning">{{ warning }}</div>
    <div v-if="snapshots.error.value" class="market-rankings-view__warning">行情补充失败：{{ snapshots.error.value }}</div>
    <div v-for="item in feature.partialErrors.value" :key="`${item.scope}-${item.code}`" class="market-rankings-view__warning">
      {{ item.scope }} · {{ item.message }}
    </div>
    <div v-if="productScope === 'fund' && assetClassSummary.length" class="market-rankings-view__asset-map" aria-label="ETF 资产类别分布">
      <button
        v-for="item in assetClassSummary"
        :key="item.name"
        type="button"
        :class="{ 'is-active': assetClass === item.name }"
        @click="assetClass = assetClass === item.name ? '' : item.name"
      >
        <strong>{{ item.name }}</strong>
        <span>{{ item.count }} 只</span>
      </button>
    </div>
    <div v-if="productScope === 'stock' && market === 'CN'" class="market-rankings-view__status">
      OpenD 10.9.6908 的专用高股息协议不接受市场参数且实测仅返回港股；沪深股票榜单不展示错配数据。
    </div>
    <div v-else-if="feature.error.value" class="market-rankings-view__status">{{ feature.error.value }}</div>
    <RankListPanel
      v-else
      :title="valuePresentation.title"
      :entries="visibleEntries"
      :loading="feature.loading.value"
      :value-field="valuePresentation.field"
      :value-label="valuePresentation.label"
      :default-sort-order="operation === 'top_losers' || (productScope === 'fund' && fundRanking === 'losers') ? 'asc' : 'desc'"
      @select="emit('select', $event)"
    />
    <button
      v-if="feature.hasMore.value"
      type="button"
      class="market-rankings-view__more"
      :disabled="feature.loadingMore.value"
      @click="feature.loadMore()"
    >{{ feature.loadingMore.value ? "加载中…" : "加载更多" }}</button>
  </div>
</template>

<style scoped>
.market-rankings-view {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.market-rankings-view__toolbar {
  display: flex;
  min-height: 32px;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.market-rankings-view__operations {
  max-width: 100%;
  overflow-x: auto;
}

.market-rankings-view__select {
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}

.market-rankings-view__count {
  margin-left: auto;
  color: var(--tv-text-dim);
}

.market-rankings-view__warning,
.market-rankings-view__status {
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--tv-warn) 40%, var(--tv-border));
  border-radius: 4px;
  color: var(--tv-warn);
}

.market-rankings-view__asset-map {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 4px;
}

.market-rankings-view__asset-map button {
  display: flex;
  min-height: 52px;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  padding: 6px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tv-accent) 8%, var(--tv-bg-surface));
  color: var(--tv-text);
  cursor: pointer;
  text-align: left;
}

.market-rankings-view__asset-map button.is-active {
  border-color: var(--tv-accent);
}

.market-rankings-view__asset-map span {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.market-rankings-view__more {
  align-self: center;
  padding: 5px 18px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
}
</style>
