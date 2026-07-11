<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useRouter } from "vue-router";

import type { WatchlistItem } from "@/contracts";

import WatchlistGroupManagerDialog from "../components/domain/watchlist/WatchlistGroupManagerDialog.vue";
import WatchlistImportDialog from "../components/domain/watchlist/WatchlistImportDialog.vue";
import WatchlistMembershipDialog from "../components/domain/watchlist/WatchlistMembershipDialog.vue";
import WatchlistVirtualTable from "../components/domain/watchlist/WatchlistVirtualTable.vue";
import {
  useWatchlistGroups,
  useWatchlistItems,
  useWatchlistQuotes,
} from "../composables/useWatchlist";
import { useWorkspaceTradingPrefs } from "../composables/useWorkspaceLayout";

const router = useRouter();
const { prefs: tradingPrefs, update: updateTradingPrefs } =
  useWorkspaceTradingPrefs();
const selectedGroupId = ref<string | null>(null);
const searchInput = ref("");
const appliedSearch = ref("");
const selectedMarket = ref("");
const visibleInstrumentIds = ref<string[]>([]);
const groupManagerOpen = ref(false);
const importDialogOpen = ref(false);
const membershipDialogOpen = ref(false);
const membershipTarget = ref<WatchlistItem | null>(null);
let searchTimer: number | null = null;

const groupsQuery = useWatchlistGroups();
const groups = computed(() => groupsQuery.data.value ?? []);
const itemFilters = computed(() => ({
  groupId: selectedGroupId.value,
  query: appliedSearch.value,
  market: selectedMarket.value,
  limit: 200,
}));
const itemsQuery = useWatchlistItems(itemFilters);
const items = computed(() => {
  const seen = new Set<string>();
  const result: WatchlistItem[] = [];
  for (const page of itemsQuery.data.value?.pages ?? []) {
    for (const item of page.items ?? []) {
      const key = item.instrumentId.toUpperCase();
      if (seen.has(key)) continue;
      seen.add(key);
      result.push(item);
    }
  }
  return result;
});
const quotesQuery = useWatchlistQuotes(visibleInstrumentIds, true);
const activeInstrumentId = computed(
  () => `${tradingPrefs.value.market}.${tradingPrefs.value.symbol}`.toUpperCase(),
);
const pageError = computed(() => {
  const error = groupsQuery.error.value ?? itemsQuery.error.value;
  return error instanceof Error ? error.message : error ? String(error) : "";
});
const loadedCount = computed(() => items.value.length);
const selectedGroupName = computed(
  () =>
    selectedGroupId.value == null
      ? "全部"
      : groups.value.find((group) => group.id === selectedGroupId.value)?.name ??
        "分组",
);

function selectGroup(groupId: string | null): void {
  selectedGroupId.value = groupId;
  visibleInstrumentIds.value = [];
}

function selectInstrument(item: WatchlistItem): void {
  updateTradingPrefs({ market: item.market, symbol: item.symbol });
  void router.push("/workspace");
}

function editMembership(item: WatchlistItem): void {
  membershipTarget.value = item;
  membershipDialogOpen.value = true;
}

function loadMore(): void {
  if (
    itemsQuery.hasNextPage.value &&
    !itemsQuery.isFetchingNextPage.value
  ) {
    void itemsQuery.fetchNextPage();
  }
}

watch(searchInput, (value) => {
  if (searchTimer != null) window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(() => {
    appliedSearch.value = value.trim();
    searchTimer = null;
  }, 220);
});

watch(groups, (next) => {
  if (
    selectedGroupId.value != null &&
    !next.some((group) => group.id === selectedGroupId.value)
  ) {
    selectedGroupId.value = null;
  }
});

onBeforeUnmount(() => {
  if (searchTimer != null) window.clearTimeout(searchTimer);
});
</script>

<template>
  <div class="watchlist-page">
    <header class="watchlist-page__header">
      <div>
        <span class="watchlist-page__eyebrow">JFTrade Watchlist</span>
        <div class="watchlist-page__title-row">
          <h1>自选股</h1>
          <span>{{ selectedGroupName }} · 已加载 {{ loadedCount }}</span>
        </div>
        <p>本地统一管理多分组自选，可从券商预览导入；点击标的回到交易工作台。</p>
      </div>
      <div class="watchlist-page__header-actions">
        <button type="button" class="tv-btn tv-btn-ghost" @click="groupManagerOpen = true">
          <v-icon>fa-solid fa-layer-group</v-icon> 创建 / 管理分组
        </button>
        <button type="button" class="tv-btn watchlist-page__import-button" @click="importDialogOpen = true">
          <v-icon>fa-solid fa-file-import</v-icon> 券商导入
        </button>
      </div>
    </header>

    <section class="watchlist-page__surface">
      <div class="watchlist-page__tabs-row">
        <div class="watchlist-page__tabs" role="tablist" aria-label="自选分组">
          <button type="button" role="tab" :aria-selected="selectedGroupId == null" :class="{ 'is-active': selectedGroupId == null }" @click="selectGroup(null)">
            全部
          </button>
          <button v-for="group in groups" :key="group.id" type="button" role="tab" :aria-selected="selectedGroupId === group.id" :class="{ 'is-active': selectedGroupId === group.id }" @click="selectGroup(group.id)">
            {{ group.name }} <span>{{ group.itemCount ?? 0 }}</span>
          </button>
        </div>
        <button type="button" class="watchlist-page__add-group" title="新建分组" @click="groupManagerOpen = true">＋</button>
      </div>

      <div class="watchlist-page__filters">
        <div class="watchlist-page__search" role="search">
          <v-icon>fa-solid fa-magnifying-glass</v-icon>
          <input v-model="searchInput" type="search" placeholder="搜索名称或代码" aria-label="搜索自选股名称或代码" />
          <button v-if="searchInput" type="button" aria-label="清除搜索" @click="searchInput = ''">×</button>
        </div>
        <label class="watchlist-page__market-filter">
          <span>市场</span>
          <select v-model="selectedMarket">
            <option value="">全部市场</option>
            <option value="HK">港股</option>
            <option value="US">美股</option>
            <option value="CN">A 股</option>
            <option value="SH">沪市</option>
            <option value="SZ">深市</option>
            <option value="SG">新加坡</option>
            <option value="JP">日本</option>
            <option value="AU">澳大利亚</option>
            <option value="MY">马来西亚</option>
            <option value="CA">加拿大</option>
          </select>
        </label>
        <span class="watchlist-page__polling-state" title="行情仅刷新当前可见行">
          <i class="tv-state-dot tv-status--success"></i> 可见行情 3 秒刷新
        </span>
      </div>

      <div v-if="pageError" class="watchlist-page__error tv-status--error tv-status-surface">
        <v-icon>fa-solid fa-circle-exclamation</v-icon>
        <span>{{ pageError }}</span>
        <button type="button" @click="itemsQuery.refetch()">重试</button>
      </div>

      <WatchlistVirtualTable
        :items="items"
        :quotes="quotesQuery.quotesByInstrument.value"
        :quote-errors="quotesQuery.errorsByInstrument.value"
        :loading="itemsQuery.isLoading.value"
        :loading-more="itemsQuery.isFetchingNextPage.value"
        :has-more="itemsQuery.hasNextPage.value === true"
        :active-instrument-id="activeInstrumentId"
        :empty-text="appliedSearch || selectedMarket ? '没有符合筛选条件的自选标的' : '这个分组还没有标的'"
        @select="selectInstrument"
        @edit-membership="editMembership"
        @visible-instrument-ids="visibleInstrumentIds = $event"
        @end-reached="loadMore"
      />
    </section>

    <WatchlistGroupManagerDialog v-model="groupManagerOpen" />
    <WatchlistImportDialog v-model="importDialogOpen" />
    <WatchlistMembershipDialog
      v-if="membershipTarget"
      v-model="membershipDialogOpen"
      :market="membershipTarget.market"
      :symbol="membershipTarget.symbol"
      :title="membershipTarget.name ? `${membershipTarget.instrumentId} · ${membershipTarget.name}` : membershipTarget.instrumentId"
    />
  </div>
</template>

<style scoped>
.watchlist-page {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  overflow: hidden;
  background:
    radial-gradient(circle at 92% -20%, color-mix(in srgb, var(--tv-accent) 9%, transparent), transparent 36%),
    var(--tv-bg-app);
}

.watchlist-page__header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 2px 2px 0;
}

.watchlist-page__eyebrow { color: var(--tv-accent); font-size: 9px; font-weight: 750; letter-spacing: .18em; text-transform: uppercase; }
.watchlist-page__title-row { display: flex; align-items: baseline; gap: 12px; }
.watchlist-page__title-row h1 { margin: 2px 0 0; color: var(--tv-text); font-size: 24px; font-weight: 680; letter-spacing: -.02em; }
.watchlist-page__title-row span { color: var(--tv-text-dim); font-size: 11px; }
.watchlist-page__header p { margin: 2px 0 0; color: var(--tv-text-dim); font-size: 11px; }
.watchlist-page__header-actions { display: flex; gap: 8px; }
.watchlist-page__header-actions .tv-btn { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.watchlist-page__import-button { border-color: var(--tv-accent); background: var(--tv-accent); color: #fff; }

.watchlist-page__surface {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, #000 8%, transparent);
}

.watchlist-page__tabs-row { display: flex; flex: 0 0 auto; align-items: stretch; min-width: 0; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface-2); }
.watchlist-page__tabs { display: flex; min-width: 0; flex: 1; gap: 2px; padding: 5px 7px 0; overflow-x: auto; scrollbar-width: thin; }
.watchlist-page__tabs button { position: relative; flex: 0 0 auto; padding: 8px 11px 9px; border: 0; border-radius: 6px 6px 0 0; background: transparent; color: var(--tv-text-muted); font-size: 11px; cursor: pointer; }
.watchlist-page__tabs button span { margin-left: 4px; color: var(--tv-text-dim); font-size: 9px; }
.watchlist-page__tabs button.is-active { background: var(--tv-bg-surface); color: var(--tv-text); font-weight: 650; }
.watchlist-page__tabs button.is-active::after { position: absolute; right: 8px; bottom: -1px; left: 8px; height: 2px; background: var(--tv-accent); content: ""; }
.watchlist-page__add-group { width: 40px; flex: 0 0 auto; border: 0; border-left: 1px solid var(--tv-border); background: transparent; color: var(--tv-text-muted); font-size: 18px; }

.watchlist-page__filters { display: flex; min-height: 43px; flex: 0 0 auto; align-items: center; gap: 10px; padding: 6px 9px; border-bottom: 1px solid var(--tv-border); }
.watchlist-page__search { display: flex; width: min(360px, 42vw); height: 30px; align-items: center; gap: 7px; padding: 0 9px; border: 1px solid var(--tv-border); border-radius: 6px; background: var(--tv-bg-surface-2); color: var(--tv-text-dim); }
.watchlist-page__search input { min-width: 0; flex: 1; border: 0; outline: 0; background: transparent; color: var(--tv-text); font-size: 11px; }
.watchlist-page__search button { border: 0; background: transparent; color: var(--tv-text-dim); }
.watchlist-page__market-filter { display: flex; height: 30px; align-items: center; gap: 5px; padding: 0 8px; border: 1px solid var(--tv-border); border-radius: 6px; color: var(--tv-text-dim); font-size: 10px; }
.watchlist-page__market-filter select { border: 0; outline: 0; background: transparent; color: var(--tv-text); font-size: 11px; }
.watchlist-page__polling-state { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; color: var(--tv-text-dim); font-size: 10px; white-space: nowrap; }
.watchlist-page__error { display: flex; align-items: center; gap: 8px; padding: 8px 11px; border-bottom-width: 1px; border-bottom-style: solid; font-size: 11px; }
.watchlist-page__error button { margin-left: auto; border: 0; background: transparent; color: inherit; text-decoration: underline; }

@media (max-width: 780px) {
  .watchlist-page { gap: 8px; padding: 6px; }
  .watchlist-page__header { align-items: flex-start; flex-direction: column; gap: 8px; }
  .watchlist-page__header p { display: none; }
  .watchlist-page__header-actions { width: 100%; }
  .watchlist-page__header-actions .tv-btn { flex: 1; justify-content: center; }
  .watchlist-page__filters { flex-wrap: wrap; }
  .watchlist-page__search { width: 100%; flex: 1 1 100%; }
  .watchlist-page__polling-state { margin-left: 0; }
}
</style>
