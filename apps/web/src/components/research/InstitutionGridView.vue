<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { useResearchFeature } from "../../composables/useResearchFeature";
import {
  formatCompactNumber,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import { isResearchQuoteEntry } from "./researchQuote";

type InstitutionOperation = "list" | "holding_changes";

const props = withDefaults(
  defineProps<{
    market?: string;
    brokerId?: string;
    operation?: InstitutionOperation;
  }>(),
  { market: "US", brokerId: "", operation: "list" },
);
const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
}>();

const feature = useResearchFeature(
  () =>
    `/api/v1/research/institutions?market=${encodeURIComponent(props.market)}&operation=list&pageSize=50`,
  { expandCN: false, brokerId: () => props.brokerId },
);

const keyword = ref("");
const selectedInstitution = ref<Record<string, unknown> | null>(null);
const isHoldingChanges = computed(() => props.operation === "holding_changes");

function institutionKey(entry: Record<string, unknown>): string {
  const value = pickNumber(entry, ["institutionId"]);
  return value == null ? "" : String(Math.trunc(value));
}

const institutionId = computed(() =>
  institutionKey(selectedInstitution.value ?? {}),
);
function detailPath(operation: string, enabled = true): string {
  if (!enabled || !institutionId.value) return "";
  return `/api/v1/research/institutions?market=${encodeURIComponent(props.market)}&operation=${operation}&institutionId=${encodeURIComponent(institutionId.value)}&pageSize=50`;
}
const profile = useResearchFeature(() => detailPath("profile", !isHoldingChanges.value), {
  expandCN: false,
  brokerId: () => props.brokerId,
});
const holdings = useResearchFeature(() => detailPath("holdings", !isHoldingChanges.value), {
  expandCN: false,
  brokerId: () => props.brokerId,
});
const distribution = useResearchFeature(() => detailPath("distribution", !isHoldingChanges.value), {
  expandCN: false,
  brokerId: () => props.brokerId,
});
const holdingChanges = useResearchFeature(
  () => detailPath("holding_changes", isHoldingChanges.value),
  {
    expandCN: false,
    brokerId: () => props.brokerId,
  },
);

watch(
  () => [props.market, props.operation] as const,
  () => {
    selectedInstitution.value = null;
  },
);

const selectedInstitutionName = computed(() =>
  pickString(selectedInstitution.value ?? {}, ["name", "institutionName"]),
);

function closeDetails(): void {
  selectedInstitution.value = null;
}

function selectInstitution(entry: Record<string, unknown>): void {
  selectedInstitution.value = entry;
}

function selectHolding(entry: Record<string, unknown>): void {
  if (isResearchQuoteEntry(entry, props.market)) {
    emit("select", entry);
  }
}

function holdingIsQuoteable(entry: Record<string, unknown>): boolean {
  return isResearchQuoteEntry(entry, props.market);
}

const activeDetail = computed(() =>
  isHoldingChanges.value ? holdingChanges : holdings,
);

const activeDetailError = computed(() => {
  if (isHoldingChanges.value) return holdingChanges.error.value;
  return (
    profile.error.value ||
    holdings.error.value ||
    distribution.error.value
  );
});

const activeDetailEmptyLabel = computed(() =>
  isHoldingChanges.value ? "暂无持仓变化" : "暂无持仓明细",
);

const activeLoadMoreLabel = computed(() =>
  isHoldingChanges.value ? "加载更多变化" : "加载更多持仓",
);

const activeLoadingMoreLabel = computed(() =>
  isHoldingChanges.value ? "正在加载变化…" : "加载中…",
);

const activeEntries = computed(() => activeDetail.value.entries.value);
const activeHasMore = computed(() => activeDetail.value.hasMore.value);
const activeLoading = computed(() => activeDetail.value.loading.value);
const activeLoadingMore = computed(
  () => activeDetail.value.loadingMore.value,
);

function loadMoreDetails(): void {
  void activeDetail.value.loadMore();
}

const toolbarTitle = computed(() => {
  const marketLabel = props.market === "HK" ? "港股机构" : "美股机构";
  return isHoldingChanges.value ? `${marketLabel}持仓变化` : marketLabel;
});

const selectionHint = computed(() =>
  isHoldingChanges.value ? "请选择机构查看持仓变化" : "",
);

const holdingChangesTotal = computed(() => holdingChanges.total.value);

const holdingChangesWarnings = computed(() => [
  ...holdingChanges.warnings.value,
  ...holdingChanges.partialErrors.value.map((item) => item.message),
]);

const hasHoldingChangesWarnings = computed(
  () => holdingChangesWarnings.value.length > 0,
);

function rowKey(entry: Record<string, unknown>, index: number): string {
  return (
    pickString(entry, ["instrumentId", "symbol"]) ||
    `${pickString(entry, ["holdingDate"])}-${index}`
  );
}

interface InstitutionCard {
  entry: Record<string, unknown>;
  name: string;
  initial: string;
  marketValue: number | null;
  marketValueChange: number | null;
  holdingCount: number | null;
  holdingCountChange: number | null;
  disclosureDate: string;
}

const cards = computed<InstitutionCard[]>(() =>
  feature.entries.value.map((entry) => {
    const name =
      pickString(entry, ["name", "institutionName"]) || "--";
    return {
      entry,
      name,
      initial: name.slice(0, 1),
      marketValue: pickNumber(entry, ["marketValue"]),
      marketValueChange: pickNumber(entry, ["marketValueChange"]),
      holdingCount: pickNumber(entry, ["holdingCount"]),
      holdingCountChange: pickNumber(entry, ["holdingCountChange"]),
      disclosureDate: pickString(entry, ["asOfDate", "disclosureDate"]),
    };
  }),
);

const visibleCards = computed(() => {
  const filter = keyword.value.trim().toLocaleLowerCase();
  if (filter === "") return cards.value;
  return cards.value.filter((card) =>
    card.name.toLocaleLowerCase().includes(filter),
  );
});

</script>

<template>
  <div class="institution-grid-view">
    <div class="institution-grid-view__toolbar">
      <strong>{{ toolbarTitle }}</strong>
      <input
        v-model="keyword"
        class="institution-grid-view__search"
        type="search"
        placeholder="搜索机构名称"
      />
      <span class="institution-grid-view__spacer" />
      <span class="institution-grid-view__currency">
        货币单位：{{ String(feature.metadata.value.currency ?? (market === "HK" ? "HKD" : "USD")) }}
      </span>
    </div>

    <div v-if="feature.loading.value" class="institution-grid-view__status">加载中…</div>
    <div v-else-if="feature.error.value" class="institution-grid-view__status">
      {{ feature.error.value }}
    </div>
    <div v-else-if="visibleCards.length === 0" class="institution-grid-view__status">
      暂无数据
    </div>
    <div v-else class="institution-grid-view__grid">
      <div
        v-for="card in visibleCards"
        :key="card.name"
        class="institution-grid-view__card"
        :class="{ 'is-selected': institutionId === institutionKey(card.entry) }"
        @click="selectInstitution(card.entry)"
      >
        <span class="institution-grid-view__mark">{{ card.initial }}</span>
        <div class="institution-grid-view__info">
          <div class="institution-grid-view__name" :title="card.name">{{ card.name }}</div>
          <div class="institution-grid-view__row">
            <span class="institution-grid-view__label">持仓市值</span>
            <span class="institution-grid-view__value tv-num">
              {{ formatCompactNumber(card.marketValue) }}
            </span>
          </div>
          <div class="institution-grid-view__row">
            <span class="institution-grid-view__label">持仓数量</span>
            <span class="institution-grid-view__value tv-num">
              {{ formatCompactNumber(card.holdingCount) }}
            </span>
          </div>
          <div class="institution-grid-view__row">
            <span class="institution-grid-view__label">市值变化</span>
            <span class="institution-grid-view__value tv-num">
              {{ formatSigned(card.marketValueChange) }}
            </span>
          </div>
          <div class="institution-grid-view__row">
            <span class="institution-grid-view__label">数量变化</span>
            <span class="institution-grid-view__value tv-num">
              {{ formatSigned(card.holdingCountChange) }}
            </span>
          </div>
          <small v-if="card.disclosureDate" class="institution-grid-view__date">披露 {{ card.disclosureDate }}</small>
        </div>
      </div>
    </div>
    <button
      v-if="feature.hasMore.value"
      type="button"
      class="institution-grid-view__load-more"
      :disabled="feature.loadingMore.value"
      @click="feature.loadMore()"
    >{{ feature.loadingMore.value ? "加载中…" : "加载更多机构" }}</button>

    <div
      v-if="selectionHint && !selectedInstitution"
      class="institution-grid-view__detail-empty institution-grid-view__selection-hint"
    >
      {{ selectionHint }}
    </div>

    <section v-if="selectedInstitution" class="institution-grid-view__details">
      <header class="institution-grid-view__details-head">
        <strong>{{ selectedInstitutionName }}</strong>
        <button type="button" @click="closeDetails">关闭</button>
      </header>
      <div v-if="!isHoldingChanges" class="institution-grid-view__summary">
        <span>资料 {{ profile.entries.value.length }} 项</span>
        <span>持仓 {{ holdings.total.value }} 项</span>
        <span>分布 {{ distribution.entries.value.length }} 项</span>
      </div>
      <div v-else class="institution-grid-view__summary">
        <span>持仓变化 {{ holdingChangesTotal }} 项</span>
      </div>
      <div v-if="activeDetailError" class="institution-grid-view__detail-error">
        {{ activeDetailError }}
      </div>
      <div
        v-if="isHoldingChanges && hasHoldingChangesWarnings"
        class="institution-grid-view__detail-warning"
      >
        {{ holdingChangesWarnings.join("；") }}
      </div>
      <table v-if="activeEntries.length" class="institution-grid-view__holdings">
        <thead v-if="isHoldingChanges">
          <tr>
            <th>代码</th><th>名称</th><th>机构仓位</th><th>变动股数</th><th>变动比例</th><th>持仓日期</th><th>来源</th>
          </tr>
        </thead>
        <thead v-else>
          <tr><th>代码</th><th>名称</th><th>持仓市值</th><th>持股比例</th><th>上期比例</th><th>变动股数</th><th>机构仓位</th></tr>
        </thead>
        <tbody>
          <tr
            v-for="(entry, index) in activeEntries"
            :key="rowKey(entry, index)"
            :class="{ 'is-quoteable': holdingIsQuoteable(entry) }"
            @click="selectHolding(entry)"
          >
            <td>{{ pickString(entry, ["symbol", "instrumentId"]) || "--" }}</td>
            <td>{{ pickString(entry, ["name"]) || "--" }}</td>
            <template v-if="isHoldingChanges">
              <td class="tv-num">{{ formatSigned(pickNumber(entry, ["portfolioPct"]), "%") }}</td>
              <td class="tv-num">{{ formatCompactNumber(pickNumber(entry, ["changeShares"])) }}</td>
              <td class="tv-num">{{ formatSigned(pickNumber(entry, ["changePct"]), "%") }}</td>
              <td>{{ pickString(entry, ["holdingDate"]) || "--" }}</td>
              <td>{{ pickString(entry, ["source"]) || "--" }}</td>
            </template>
            <template v-else>
              <td class="tv-num">{{ formatCompactNumber(pickNumber(entry, ["holdingValue"])) }}</td>
              <td class="tv-num">{{ formatSigned(pickNumber(entry, ["holdingPct"]), "%") }}</td>
              <td class="tv-num">{{ formatSigned(pickNumber(entry, ["lastHoldingPct"]), "%") }}</td>
              <td class="tv-num">{{ formatCompactNumber(pickNumber(entry, ["changeShares"])) }}</td>
              <td class="tv-num">{{ formatSigned(pickNumber(entry, ["portfolioPct"]), "%") }}</td>
            </template>
          </tr>
        </tbody>
      </table>
      <div v-else-if="activeLoading" class="institution-grid-view__detail-empty">加载中…</div>
      <div v-else class="institution-grid-view__detail-empty">{{ activeDetailEmptyLabel }}</div>
      <button
        v-if="activeHasMore"
        type="button"
        class="institution-grid-view__load-more"
        :disabled="activeLoadingMore"
        @click="loadMoreDetails"
      >{{ activeLoadingMore ? activeLoadingMoreLabel : activeLoadMoreLabel }}</button>
    </section>
  </div>
</template>

<style scoped>
.institution-grid-view {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.institution-grid-view__toolbar {
  display: flex;
  min-height: 32px;
  flex: 0 0 auto;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.institution-grid-view__search {
  width: 200px;
  height: 28px;
  padding: 0 8px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  outline: none;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font-size: 12px;
}

.institution-grid-view__search:focus {
  border-color: var(--tv-accent);
}

.institution-grid-view__spacer {
  flex: 1;
}

.institution-grid-view__currency {
  color: var(--tv-text-dim);
}

.institution-grid-view__status {
  display: grid;
  min-height: 120px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.institution-grid-view__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
}

.institution-grid-view__card {
  display: flex;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  cursor: pointer;
}

.institution-grid-view__card:hover {
  border-color: var(--tv-border-strong);
}

.institution-grid-view__card.is-selected {
  border-color: var(--tv-accent);
}

.institution-grid-view__mark {
  display: inline-grid;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid var(--tv-border-strong);
  border-radius: 50%;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 16px;
  font-weight: 600;
}

.institution-grid-view__info {
  min-width: 0;
  flex: 1;
}

.institution-grid-view__name {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.institution-grid-view__row {
  display: flex;
  height: 22px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.institution-grid-view__label {
  color: var(--tv-text-dim);
}

.institution-grid-view__value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.institution-grid-view__date {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.institution-grid-view__details {
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.institution-grid-view__details-head,
.institution-grid-view__summary {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 16px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
}

.institution-grid-view__details-head button {
  margin-left: auto;
  border: 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
}

.institution-grid-view__summary,
.institution-grid-view__detail-error,
.institution-grid-view__detail-warning,
.institution-grid-view__detail-empty {
  color: var(--tv-text-dim);
}

.institution-grid-view__detail-error,
.institution-grid-view__detail-warning,
.institution-grid-view__detail-empty {
  padding: 16px;
}

.institution-grid-view__detail-warning {
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-warning, #d6a34a);
}

.institution-grid-view__selection-hint {
  border: 1px dashed var(--tv-border);
  border-radius: 6px;
  text-align: center;
}

.institution-grid-view__holdings {
  width: 100%;
  border-collapse: collapse;
}

.institution-grid-view__holdings th,
.institution-grid-view__holdings td {
  height: 32px;
  padding: 0 10px;
  border-bottom: 1px solid var(--tv-border);
  text-align: left;
}

.institution-grid-view__holdings th {
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 500;
}

.institution-grid-view__holdings tr.is-quoteable {
  cursor: pointer;
}

.institution-grid-view__holdings tr.is-quoteable:hover td {
  background: var(--tv-bg-elevated);
}

.institution-grid-view__load-more {
  align-self: center;
  margin: 8px;
  padding: 5px 14px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  cursor: pointer;
}
</style>
