<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { apiPostPath } from "@/composables/shared/apiClient";
import { useBrokerProviderSelection, withBrokerProvider } from "@/composables/trading/brokerProviderSelection";
import { productCompactMenuProps } from "@/composables/product/productControlDensity";
import {
  fetchProductFeature,
  prepareProductFeature,
  type ProductFeatureResult,
} from "@/composables/product/productFeatures";
import { type MarketFeatureRequest } from "@/composables/product/marketFeatureApi";
import { useConsoleData } from "@/composables/workspace/useConsoleData";
import {
  formatOptionResearchCell as formatCell,
  optionEntryInstrumentId as entryInstrumentId,
  optionResearchColumns,
  optionSecurityInstrumentId,
} from "../../features/optionResearch";
import type {
  OptionResearchDrilldownContext as DrilldownContext,
  OptionResearchEntry as Entry,
  OptionResearchOperation,
  OptionSellerStrategy as SellerStrategy,
} from "../../features/optionResearch";
import AsyncPanelState from "@/components/shared/AsyncPanelState.vue";
import ProductToolbarRefreshButton from "./ProductToolbarRefreshButton.vue";

const props = withDefaults(
  defineProps<{
    market: string;
    operation: OptionResearchOperation;
    scope?: "underlying" | "market";
    underlyingInstrumentId?: string;
    underlyingProductClass?: string;
    presentation?: "workspace" | "research";
  }>(),
  {
    scope: "market",
    underlyingInstrumentId: "",
    underlyingProductClass: "equity",
    presentation: "workspace",
  },
);
const emit = defineEmits<{
  openInstrument: [instrumentId: string, productClass: "option" | "equity"];
}>();

const loading = ref(false);
const error = ref("");
const result = ref<ProductFeatureResult | null>(null);
const cursor = ref("");
const cursorHistory = ref<string[]>([]);
const sellerStrategy = ref<SellerStrategy>("covered_call");
const drilldownSource = ref<Entry | null>(null);
const drilldownResult = ref<ProductFeatureResult | null>(null);
const { selectedBrokerId } = useBrokerProviderSelection();
const { selectedBrokerAccount } = useConsoleData();
let requestToken = 0;
const securityInstrumentId = optionSecurityInstrumentId;

const normalizedMarket = computed(() => props.market.trim().toUpperCase());
const columns = computed(() => optionResearchColumns[props.operation]);
const active = computed(
  () =>
    props.scope === "market" ||
    props.underlyingInstrumentId.trim().length > 0,
);
const sourceEntries = computed(
  () => drilldownResult.value?.entries ?? result.value?.entries ?? [],
);
const drilldownContext = computed<DrilldownContext | null>(() => {
  const value = drilldownSource.value?.drilldownContext;
  if (value == null || typeof value !== "object") return null;
  const context = value as Partial<DrilldownContext>;
  if (
    !context.underlyingInstrumentId ||
    !context.expiryTimestamp ||
    !context.chain?.productCode
  ) {
    return null;
  }
  return context as DrilldownContext;
});

function buildRequest(refresh = false): MarketFeatureRequest {
  return {
    scope: "market-feature",
    resource: "option-events",
    brokerId: selectedBrokerId.value,
    market: normalizedMarket.value,
    operation: props.operation,
    pageSize: 50,
    ...(cursor.value ? { cursor: cursor.value } : {}),
    ...(refresh ? { refresh: true } : {}),
    ...(props.scope === "underlying"
      ? {
          underlying: props.underlyingInstrumentId.trim().toUpperCase(),
          underlyingProductClass: props.underlyingProductClass,
        }
      : {}),
    ...(props.operation === "seller"
      ? { sellerStrategy: sellerStrategy.value }
      : {}),
  };
}

async function load(refresh = false): Promise<void> {
  const token = ++requestToken;
  drilldownSource.value = null;
  drilldownResult.value = null;
  if (!active.value) {
    result.value = null;
    error.value = "";
    loading.value = false;
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const response = await fetchProductFeature(prepareProductFeature(buildRequest(refresh)));
    if (token === requestToken) result.value = response;
  } catch (cause) {
    if (token !== requestToken) return;
    error.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    if (token === requestToken) loading.value = false;
  }
}

async function openDrilldown(entry: Entry): Promise<void> {
  drilldownSource.value = entry;
  drilldownResult.value = null;
  const context = drilldownContext.value;
  if (!context) {
    error.value = "该 0DTE 标的缺少期权链上下文，请刷新后重试。";
    return;
  }
  const token = ++requestToken;
  loading.value = true;
  error.value = "";
  try {
    const response = await apiPostPath(
      "/api/v1/market-data/options/events/zero-dte-contracts",
      withBrokerProvider(
        "/api/v1/market-data/options/events/zero-dte-contracts",
        selectedBrokerId.value,
      ),
      {
        brokerId: selectedBrokerId.value,
        accountId:
          selectedBrokerAccount.value?.brokerId === selectedBrokerId.value
            ? selectedBrokerAccount.value.accountId
            : "",
        tradingEnvironment:
          selectedBrokerAccount.value?.brokerId === selectedBrokerId.value
            ? selectedBrokerAccount.value.tradingEnvironment
            : "",
        market: "US",
        underlyingInstrumentId: context.underlyingInstrumentId,
        underlyingProductClass:
          props.underlyingProductClass === "option" ? "option" : "equity",
        expiryTimestamp: context.expiryTimestamp,
        chain: context.chain,
        sort: "volume",
        optionType: "all",
      },
    );
    if (token === requestToken) drilldownResult.value = response;
  } catch (cause) {
    if (token !== requestToken) return;
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    if (token === requestToken) loading.value = false;
  }
}

function nextPage(): void {
  const next = result.value?.nextCursor ?? "";
  if (!next) return;
  cursorHistory.value.push(cursor.value);
  cursor.value = next;
  void load();
}

function previousPage(): void {
  const previous = cursorHistory.value.pop();
  if (previous == null) return;
  cursor.value = previous;
  void load();
}

function resetAndLoad(): void {
  cursor.value = "";
  cursorHistory.value = [];
  void load();
}

watch(
  () => [
    props.market,
    props.operation,
    props.scope,
    props.underlyingInstrumentId,
    props.underlyingProductClass,
    sellerStrategy.value,
    selectedBrokerId.value,
  ],
  resetAndLoad,
  { immediate: true },
);
</script>

<template>
  <section
    class="option-research-panel"
    :class="`option-research-panel--${presentation}`"
  >
    <div class="option-research-panel__toolbar">
      <button
        v-if="drilldownResult"
        type="button"
        class="option-research-panel__back"
        @click="drilldownResult = null"
      >
        ← 返回 0DTE 标的
      </button>
      <span v-else class="option-research-panel__summary">
        {{
          scope === "underlying"
            ? `仅显示 ${underlyingInstrumentId}`
            : `${normalizedMarket} 全市场`
        }}
      </span>
      <select
        v-if="
          operation === 'seller' &&
          !drilldownResult &&
          presentation === 'research'
        "
        v-model="sellerStrategy"
        class="option-research-panel__seller-native"
        aria-label="卖方策略"
      >
        <option value="covered_call">备兑看涨</option>
        <option value="cash_secured_put">现金担保看跌</option>
      </select>
      <v-select
        v-else-if="operation === 'seller' && !drilldownResult"
        v-model="sellerStrategy"
        class="option-research-panel__seller product-compact-control"
        :items="[
          { title: '备兑看涨', value: 'covered_call' },
          { title: '现金担保看跌', value: 'cash_secured_put' },
        ]"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        aria-label="卖方策略"
      />
      <span class="option-research-panel__count">
        {{ sourceEntries.length }} 条
      </span>
      <button
        v-if="presentation === 'research'"
        type="button"
        class="option-research-panel__refresh-native"
        :disabled="loading"
        aria-label="刷新"
        @click="load(true)"
      >
        {{ loading ? "刷新中…" : "刷新" }}
      </button>
      <ProductToolbarRefreshButton
        v-else
        :loading="loading"
        @refresh="load(true)"
      />
    </div>

    <template v-if="presentation === 'research'">
      <div v-if="loading" class="option-research-panel__progress" />
      <div
        v-if="error"
        class="option-research-panel__notice tv-status--warning"
      >
        {{ error }}
      </div>
    </template>
    <AsyncPanelState v-else :loading="loading" :error="error" />
    <div v-if="!active" class="option-research-panel__empty">
      正在识别当前正股标的，识别完成后再加载期权事件。
    </div>
    <div
      v-else-if="sourceEntries.length === 0 && !loading && !error"
      class="option-research-panel__empty"
    >
      当前范围没有符合条件的期权数据。
    </div>
    <div v-else class="option-research-panel__table">
      <table
        v-if="presentation === 'research'"
        class="option-research-panel__native-table"
      >
        <thead>
          <tr>
            <template v-if="drilldownResult">
              <th>合约</th>
              <th>方向</th>
              <th>最新价</th>
              <th>涨跌幅</th>
              <th>成交量</th>
              <th>持仓量</th>
              <th>IV</th>
              <th>Delta</th>
            </template>
            <template v-else>
              <th v-for="column in columns" :key="column.key">
                {{ column.label }}
              </th>
            </template>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(entry, index) in sourceEntries" :key="index">
            <template v-if="drilldownResult">
              <td>{{ formatCell(entry.option) }}</td>
              <td>{{ formatCell(entry.optionType) }}</td>
              <td>{{ formatCell(entry.optionPrice) }}</td>
              <td>{{ formatCell(entry.changeRate) }}</td>
              <td>{{ formatCell(entry.volume) }}</td>
              <td>{{ formatCell(entry.openInterest) }}</td>
              <td>{{ formatCell(entry.iv) }}</td>
              <td>{{ formatCell(entry.delta) }}</td>
            </template>
            <template v-else>
              <td v-for="column in columns" :key="column.key">
                {{ formatCell(entry[column.key]) }}
              </td>
            </template>
            <td>
              <button
                v-if="!drilldownResult && operation === 'zero_dte'"
                type="button"
                class="option-research-panel__row-action"
                :disabled="!entry.drilldownContext"
                @click="openDrilldown(entry)"
              >
                查看合约
              </button>
              <button
                v-else-if="
                  entryInstrumentId(
                    entry,
                    operation === 'earnings' ? 'equity' : 'option',
                  )
                "
                type="button"
                class="option-research-panel__row-action"
                @click="
                  emit(
                    'openInstrument',
                    entryInstrumentId(
                      entry,
                      operation === 'earnings' ? 'equity' : 'option',
                    ),
                    operation === 'earnings' ? 'equity' : 'option',
                  )
                "
              >
                工作区
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <v-table v-else density="compact" fixed-header>
        <thead>
          <tr>
            <template v-if="drilldownResult">
              <th>合约</th>
              <th>方向</th>
              <th>最新价</th>
              <th>涨跌幅</th>
              <th>成交量</th>
              <th>持仓量</th>
              <th>IV</th>
              <th>Delta</th>
            </template>
            <template v-else>
              <th v-for="column in columns" :key="column.key">
                {{ column.label }}
              </th>
            </template>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(entry, index) in sourceEntries" :key="index">
            <template v-if="drilldownResult">
              <td>{{ formatCell(entry.option) }}</td>
              <td>{{ formatCell(entry.optionType) }}</td>
              <td>{{ formatCell(entry.optionPrice) }}</td>
              <td>{{ formatCell(entry.changeRate) }}</td>
              <td>{{ formatCell(entry.volume) }}</td>
              <td>{{ formatCell(entry.openInterest) }}</td>
              <td>{{ formatCell(entry.iv) }}</td>
              <td>{{ formatCell(entry.delta) }}</td>
            </template>
            <template v-else>
              <td v-for="column in columns" :key="column.key">
                {{ formatCell(entry[column.key]) }}
              </td>
            </template>
            <td>
              <v-btn
                v-if="!drilldownResult && operation === 'zero_dte'"
                size="x-small"
                variant="text"
                :disabled="!entry.drilldownContext"
                @click="openDrilldown(entry)"
              >
                查看合约
              </v-btn>
              <v-btn
                v-else-if="
                  entryInstrumentId(
                    entry,
                    operation === 'earnings' ? 'equity' : 'option',
                  )
                "
                size="x-small"
                variant="text"
                @click="
                  emit(
                    'openInstrument',
                    entryInstrumentId(
                      entry,
                      operation === 'earnings' ? 'equity' : 'option',
                    ),
                    operation === 'earnings' ? 'equity' : 'option',
                  )
                "
              >
                工作区
              </v-btn>
            </td>
          </tr>
        </tbody>
      </v-table>
    </div>

    <footer v-if="!drilldownResult && result" class="option-research-panel__pager">
      <template v-if="presentation === 'research'">
        <button
          type="button"
          class="option-research-panel__pager-action"
          :disabled="cursorHistory.length === 0"
          @click="previousPage"
        >
          上一页
        </button>
        <span>{{ result.total ?? result.entries.length }} 条结果</span>
        <button
          type="button"
          class="option-research-panel__pager-action"
          :disabled="!result.nextCursor"
          @click="nextPage"
        >
          下一页
        </button>
      </template>
      <v-btn
        v-else
        size="x-small"
        variant="text"
        :disabled="cursorHistory.length === 0"
        @click="previousPage"
      >
        上一页
      </v-btn>
      <span v-if="presentation !== 'research'">
        {{ result.total ?? result.entries.length }} 条结果
      </span>
      <v-btn
        v-if="presentation !== 'research'"
        size="x-small"
        variant="text"
        :disabled="!result.nextCursor"
        @click="nextPage"
      >
        下一页
      </v-btn>
    </footer>
  </section>
</template>

<style scoped>
.option-research-panel {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
}

.option-research-panel__toolbar {
  display: flex;
  min-height: 38px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.option-research-panel__summary,
.option-research-panel__count,
.option-research-panel__pager {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-2);
}

.option-research-panel__seller {
  width: 146px;
}

.option-research-panel__seller-native {
  width: 146px;
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.option-research-panel__count {
  margin-left: auto;
  white-space: nowrap;
}

.option-research-panel__back {
  border: 0;
  background: transparent;
  color: var(--tv-accent);
  cursor: pointer;
  font-size: var(--jf-text-3);
}

.option-research-panel__refresh-native,
.option-research-panel__row-action,
.option-research-panel__pager-action {
  border: 0;
  background: transparent;
  color: var(--tv-accent);
  cursor: pointer;
  font: inherit;
}

.option-research-panel__refresh-native {
  min-height: 26px;
  padding: 0 6px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
}

.option-research-panel__refresh-native:hover:not(:disabled),
.option-research-panel__pager-action:hover:not(:disabled) {
  background: var(--tv-bg-elevated);
}

.option-research-panel__refresh-native:disabled,
.option-research-panel__row-action:disabled,
.option-research-panel__pager-action:disabled {
  color: var(--tv-text-dim);
  cursor: not-allowed;
}

.option-research-panel__table {
  min-height: 0;
  flex: 1;
  overflow: auto;
}

.option-research-panel__native-table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}

.option-research-panel__native-table th {
  position: sticky;
  z-index: 1;
  top: 0;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  text-align: left;
}

.option-research-panel__native-table td {
  max-width: 280px;
  padding: 0 8px;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  text-overflow: ellipsis;
}

.option-research-panel__native-table tbody tr:hover {
  background: var(--tv-bg-elevated);
}

.option-research-panel__row-action {
  white-space: nowrap;
}

.option-research-panel :deep(table) {
  font-size: var(--jf-text-3);
  font-variant-numeric: tabular-nums;
}

.option-research-panel :deep(th) {
  height: 31px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-2);
  white-space: nowrap;
}

.option-research-panel :deep(td) {
  height: 34px;
  color: var(--tv-text-muted);
  white-space: nowrap;
}

.option-research-panel__empty {
  display: grid;
  min-height: 150px;
  flex: 1;
  place-items: center;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-3);
}

.option-research-panel__pager {
  display: flex;
  min-height: 34px;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 3px 8px;
  border-top: 1px solid var(--tv-border);
}

.option-research-panel :deep(.v-alert) {
  flex: 0 0 auto;
  font-size: var(--jf-text-3);
}

.option-research-panel__progress {
  position: relative;
  height: 2px;
  flex: 0 0 auto;
  overflow: hidden;
  background: var(--tv-bg-elevated);
}

.option-research-panel__progress::after {
  position: absolute;
  inset: 0;
  width: 35%;
  animation: option-research-progress 1s linear infinite;
  background: var(--tv-accent);
  content: "";
}

.option-research-panel__notice {
  flex: 0 0 auto;
  padding: 7px 8px;
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.option-research-panel--research {
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.option-research-panel--research .option-research-panel__toolbar {
  min-height: 32px;
  padding: 2px 8px;
}

.option-research-panel--research .option-research-panel__summary,
.option-research-panel--research .option-research-panel__count,
.option-research-panel--research .option-research-panel__pager {
  font-size: var(--jf-text-5);
}

.option-research-panel--research :deep(table) {
  font-size: var(--jf-text-6);
}

.option-research-panel--research :deep(th) {
  height: 32px;
  font-size: var(--jf-text-5);
}

.option-research-panel--research :deep(td) {
  height: 32px;
  color: var(--tv-text);
}

@keyframes option-research-progress {
  from {
    transform: translateX(-100%);
  }

  to {
    transform: translateX(390%);
  }
}
</style>
