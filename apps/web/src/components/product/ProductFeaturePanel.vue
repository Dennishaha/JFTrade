<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  featureEntryTitle,
  fetchProductFeature,
  instrumentIDFromFeatureEntry,
  type ProductFeatureResult,
} from "../../composables/productFeatures";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import ProductPanelToolbar from "./ProductPanelToolbar.vue";
import ProductToolbarRefreshButton from "./ProductToolbarRefreshButton.vue";

const props = withDefaults(
  defineProps<{
    title: string;
    description?: string;
    path: string;
    active?: boolean;
  }>(),
  { description: "", active: true },
);

const emit = defineEmits<{
  openInstrument: [instrumentId: string];
}>();

const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const filter = ref("");
const { selectedBrokerId } = useBrokerProviderSelection();
let requestToken = 0;

const preferredColumns = [
  "name",
  "title",
  "instrumentId",
  "code",
  "eventName",
  "seriesName",
  "curPrice",
  "lastPrice",
  "changeRate",
  "strikePrice",
  "maturityTime",
  "volume",
  "turnover",
  "status",
];
const columnLabels: Record<string, string> = {
  name: "名称",
  title: "标题",
  instrumentId: "标的",
  code: "代码",
  eventName: "事件",
  seriesName: "系列",
  curPrice: "最新价",
  lastPrice: "最新价",
  changeRate: "涨跌幅",
  strikePrice: "行权价",
  maturityTime: "到期日",
  volume: "成交量",
  turnover: "成交额",
  status: "状态",
};

const visibleEntries = computed(() => {
  const entries = result.value?.entries ?? [];
  const keyword = filter.value.trim().toLocaleLowerCase();
  if (!keyword) return entries;
  return entries.filter((entry) =>
    JSON.stringify(entry).toLocaleLowerCase().includes(keyword),
  );
});

const tableColumns = computed(() => {
  const keys = new Set<string>();
  for (const entry of visibleEntries.value.slice(0, 30)) {
    for (const [key, value] of Object.entries(entry)) {
      if (
        value == null ||
        ["string", "number", "boolean"].includes(typeof value)
      ) {
        keys.add(key);
      }
    }
  }
  return [...keys]
    .sort((left, right) => {
      const leftIndex = preferredColumns.indexOf(left);
      const rightIndex = preferredColumns.indexOf(right);
      if (leftIndex >= 0 || rightIndex >= 0) {
        if (leftIndex < 0) return 1;
        if (rightIndex < 0) return -1;
        return leftIndex - rightIndex;
      }
      return left.localeCompare(right);
    })
    .slice(0, 8);
});

function formatCell(value: unknown): string {
  if (value == null || value === "") return "—";
  if (typeof value === "number") {
    return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 4 }).format(
      value,
    );
  }
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string") return value;
  return Array.isArray(value) ? `${value.length} 项` : "查看详情";
}

const requestPath = computed(() =>
  withBrokerProvider(props.path, selectedBrokerId.value),
);
const entryCountLabel = computed(
  () => `${visibleEntries.value.length} / ${result.value?.entries.length ?? 0}`,
);

async function load(refresh = false): Promise<void> {
  if (!props.active || !props.path) return;
  const token = ++requestToken;
  loading.value = true;
  error.value = "";
  try {
    const path = requestPath.value;
    const separator = path.includes("?") ? "&" : "?";
    const response = await fetchProductFeature(
      refresh ? `${path}${separator}refresh=true` : path,
    );
    if (token === requestToken) result.value = response;
  } catch (cause) {
    if (token !== requestToken) return;
    error.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    if (token === requestToken) loading.value = false;
  }
}

watch(
  [() => props.path, selectedBrokerId],
  () => {
    requestToken++;
    result.value = null;
    error.value = "";
  },
);

watch(
  () => [props.path, props.active, selectedBrokerId.value] as const,
  ([, active]) => {
    if (active) {
      void load();
      return;
    }
    requestToken++;
    loading.value = false;
  },
  { immediate: true },
);
</script>

<template>
  <section class="product-feature-panel">
    <ProductPanelToolbar :title="title" :description="description">
      <slot name="controls" />
      <v-text-field
        v-if="result?.entries.length"
        v-model="filter"
        class="product-feature-panel__filter product-compact-control"
        density="compact"
        variant="outlined"
        hide-details
        clearable
        prepend-inner-icon="fa-solid fa-magnifying-glass"
        placeholder="筛选结果"
      />
      <span v-if="result?.entries.length" class="product-feature-panel__count">
        {{ entryCountLabel }} 条
      </span>
      <ProductToolbarRefreshButton :loading="loading" @refresh="load(true)" />
    </ProductPanelToolbar>

    <v-progress-linear v-if="loading" indeterminate />
    <v-alert v-if="error" type="warning" variant="tonal" density="compact">
      {{ error }}
    </v-alert>
    <template v-else-if="result != null">
      <v-alert
        v-for="warning in result.warnings ?? []"
        :key="warning"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ warning }}
      </v-alert>
      <v-alert
        v-for="partialError in result.partialErrors ?? []"
        :key="`${partialError.scope}-${partialError.code}`"
        type="warning"
        variant="outlined"
        density="compact"
      >
        {{ partialError.scope }} · {{ partialError.code }} ·
        {{ partialError.message }}
      </v-alert>
      <div
        v-if="result.entries.length === 0 && !loading"
        class="product-feature-panel__empty"
      >
        <span>◇</span>
        <strong>当前账户与权限下暂无数据</strong>
        <small>可切换数据视图或检查券商行情权限后重试。</small>
      </div>
      <template v-else>
        <div class="product-feature-panel__table-wrap">
          <v-table density="compact" fixed-header>
            <thead>
              <tr>
                <th v-if="tableColumns.length === 0">结果</th>
                <th v-for="column in tableColumns" :key="column">
                  {{ columnLabels[column] ?? column }}
                </th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(entry, index) in visibleEntries"
                :key="`${featureEntryTitle(entry, index)}-${index}`"
              >
                <td v-if="tableColumns.length === 0">
                  {{ featureEntryTitle(entry, index) }}
                </td>
                <td v-for="column in tableColumns" :key="column">
                  {{ formatCell(entry[column]) }}
                </td>
                <td class="product-feature-panel__actions">
                  <v-btn
                    v-if="instrumentIDFromFeatureEntry(entry)"
                    size="x-small"
                    variant="text"
                    @click="
                      emit(
                        'openInstrument',
                        instrumentIDFromFeatureEntry(entry)!,
                      )
                    "
                  >
                    工作区
                  </v-btn>
                  <details>
                    <summary>详情</summary>
                    <pre>{{ JSON.stringify(entry, null, 2) }}</pre>
                  </details>
                </td>
              </tr>
            </tbody>
          </v-table>
        </div>
      </template>
      <footer class="product-feature-panel__footer">
        <span>数据时间 {{ result.asOf }}</span>
        <span v-if="result.nextCursor">还有下一页</span>
      </footer>
    </template>
  </section>
</template>

<style scoped>
.product-feature-panel {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.product-feature-panel__footer {
  color: var(--tv-text-muted);
}

.product-feature-panel__empty {
  display: grid;
  flex: 1;
  min-height: 180px;
  place-content: center;
  justify-items: center;
  color: var(--tv-text-muted);
  text-align: center;
}

.product-feature-panel__empty > span {
  display: grid;
  width: 40px;
  height: 40px;
  margin-bottom: 9px;
  place-items: center;
  border: 1px solid var(--tv-border-strong);
  border-radius: 50%;
  color: var(--tv-accent);
  font-size: 18px;
}

.product-feature-panel__empty strong {
  color: var(--tv-text);
  font-size: 11px;
}

.product-feature-panel__empty small {
  margin-top: 4px;
  font-size: 9px;
}

.product-feature-panel__filter {
  max-width: 190px;
  flex: 0 1 190px;
}

.product-feature-panel__count {
  color: var(--tv-text-dim);
  font-size: 9px;
  font-variant-numeric: tabular-nums;
}

.product-feature-panel__table-wrap {
  min-height: 0;
  flex: 1;
  overflow: auto;
}

.product-feature-panel__table-wrap :deep(table) {
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}

.product-feature-panel__table-wrap :deep(th) {
  height: 33px !important;
  background: var(--tv-bg-surface-2) !important;
  color: var(--tv-text-dim) !important;
  font-size: 9px;
  font-weight: 650 !important;
  white-space: nowrap;
}

.product-feature-panel__table-wrap :deep(td) {
  height: 38px !important;
  border-color: var(--tv-border) !important;
  color: var(--tv-text);
}

.product-feature-panel__table-wrap :deep(tbody tr:hover td) {
  background: color-mix(in srgb, var(--tv-accent) 5%, transparent);
}

.product-feature-panel__actions {
  display: flex;
  align-items: center;
  white-space: nowrap;
}
.product-feature-panel__actions details {
  position: relative;
}
.product-feature-panel__actions summary {
  padding: 4px 8px;
  color: var(--tv-accent);
  cursor: pointer;
  list-style: none;
  font-size: 9px;
}

.product-feature-panel__actions pre {
  position: absolute;
  z-index: 10;
  right: 0;
  width: min(520px, 70vw);
  max-height: 360px;
  padding: 10px;
  margin: 0;
  overflow: auto;
  border: 1px solid var(--tv-border-strong);
  border-radius: 6px;
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
  box-shadow: 0 8px 24px rgb(0 0 0 / 20%);
  font:
    11px/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
  white-space: pre-wrap;
}
.product-feature-panel__footer {
  display: flex;
  min-height: 30px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  padding: 0 11px;
  border-top: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-dim);
  font-size: 8px;
}
</style>
