<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRouter } from "vue-router";

import type { WatchlistItem } from "@/contracts";

import {
  useWatchlistGroups,
  useWatchlistItems,
  useWatchlistQuotes,
} from "../../../composables/useWatchlist";
import {
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
} from "../../../composables/useWorkspaceLayout";
import WatchlistMembershipDialog from "./WatchlistMembershipDialog.vue";
import WatchlistVirtualTable from "./WatchlistVirtualTable.vue";

defineProps<{ compact?: boolean }>();
const emit = defineEmits<{ close: []; selected: [item: WatchlistItem] }>();

const router = useRouter();
const { prefs: viewPrefs, update: updateView } = useWorkspaceViewState();
const { prefs: tradingPrefs, update: updateTrading } = useWorkspaceTradingPrefs();
const visibleInstrumentIds = ref<string[]>([]);
const membershipTarget = ref<WatchlistItem | null>(null);
const membershipDialogOpen = ref(false);
const groupsQuery = useWatchlistGroups();
const groups = computed(() => groupsQuery.data.value ?? []);
const selectedGroupId = computed({
  get: () => viewPrefs.value.watchlistGroupId,
  set: (value: string | null) => updateView({ watchlistGroupId: value || null }),
});
const itemsQuery = useWatchlistItems(
  computed(() => ({ groupId: selectedGroupId.value, limit: 160 })),
);
const items = computed(() => {
  const seen = new Set<string>();
  return (itemsQuery.data.value?.pages ?? []).flatMap((page) =>
    (page.items ?? []).filter((item) => {
      const id = item.instrumentId.toUpperCase();
      if (seen.has(id)) return false;
      seen.add(id);
      return true;
    }),
  );
});
const quotesQuery = useWatchlistQuotes(visibleInstrumentIds, true);
const activeInstrumentId = computed(
  () => `${tradingPrefs.value.market}.${tradingPrefs.value.symbol}`.toUpperCase(),
);
const errorMessage = computed(() => {
  const error = groupsQuery.error.value ?? itemsQuery.error.value;
  return error instanceof Error ? error.message : error ? String(error) : "";
});

function selectItem(item: WatchlistItem): void {
  updateTrading({ market: item.market, symbol: item.symbol });
  emit("selected", item);
}

function editMembership(item: WatchlistItem): void {
  membershipTarget.value = item;
  membershipDialogOpen.value = true;
}

function loadMore(): void {
  if (itemsQuery.hasNextPage.value && !itemsQuery.isFetchingNextPage.value) {
    void itemsQuery.fetchNextPage();
  }
}

function openFullPage(): void {
  void router.push("/watchlist");
}

watch(groups, (next) => {
  if (
    selectedGroupId.value != null &&
    !next.some((group) => group.id === selectedGroupId.value)
  ) {
    selectedGroupId.value = next.find((group) => group.isDefault)?.id ?? null;
  }
});
</script>

<template>
  <aside class="workspace-watchlist" :class="{ 'workspace-watchlist--compact': compact }">
    <header class="workspace-watchlist__header">
      <div>
        <span>自选股</span>
        <small>可见行情实时刷新</small>
      </div>
      <button type="button" title="隐藏自选栏" aria-label="隐藏自选栏" @click="emit('close')">
        <v-icon>{{ compact ? "fa-solid fa-xmark" : "fa-solid fa-chevron-left" }}</v-icon>
      </button>
    </header>

    <div class="workspace-watchlist__group-row">
      <select v-model="selectedGroupId" aria-label="选择自选分组">
        <option :value="null">全部分组</option>
        <option v-for="group in groups" :key="group.id" :value="group.id">
          {{ group.name }} · {{ group.itemCount ?? 0 }}
        </option>
      </select>
      <button type="button" title="打开完整自选页" aria-label="打开完整自选页" @click="openFullPage">
        <v-icon size="12">fa-solid fa-ellipsis</v-icon>
      </button>
    </div>

    <div v-if="errorMessage" class="workspace-watchlist__error tv-status--error tv-status-surface">
      <span>{{ errorMessage }}</span>
      <button type="button" @click="itemsQuery.refetch()">重试</button>
    </div>

    <WatchlistVirtualTable
      compact
      :items="items"
      :quotes="quotesQuery.quotesByInstrument.value"
      :quote-errors="quotesQuery.errorsByInstrument.value"
      :loading="itemsQuery.isLoading.value"
      :loading-more="itemsQuery.isFetchingNextPage.value"
      :has-more="itemsQuery.hasNextPage.value === true"
      :active-instrument-id="activeInstrumentId"
      empty-text="当前分组暂无自选"
      @select="selectItem"
      @edit-membership="editMembership"
      @visible-instrument-ids="visibleInstrumentIds = $event"
      @end-reached="loadMore"
    />

    <footer>
      <button type="button" @click="openFullPage">
        管理全部自选 <v-icon>fa-solid fa-arrow-right</v-icon>
      </button>
    </footer>

    <WatchlistMembershipDialog
      v-if="membershipTarget"
      v-model="membershipDialogOpen"
      :market="membershipTarget.market"
      :symbol="membershipTarget.symbol"
      :title="membershipTarget.name ? `${membershipTarget.instrumentId} · ${membershipTarget.name}` : membershipTarget.instrumentId"
    />
  </aside>
</template>

<style scoped>
.workspace-watchlist { display: flex; width: 100%; height: 100%; min-width: 0; min-height: 0; flex-direction: column; overflow: hidden; border-right: 1px solid var(--tv-border); background: var(--tv-bg-surface); color: var(--tv-text); }
.workspace-watchlist__header { display: flex; height: 45px; flex: 0 0 auto; align-items: center; justify-content: space-between; padding: 0 8px 0 11px; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface-2); }
.workspace-watchlist__header > div { display: flex; min-width: 0; flex-direction: column; }
.workspace-watchlist__header span { font-size: 12px; font-weight: 700; }
.workspace-watchlist__header small { color: var(--tv-text-dim); font-size: 9px; }
.workspace-watchlist__header button,
.workspace-watchlist__group-row button { width: 28px; height: 28px; border: 0; border-radius: 5px; background: transparent; color: var(--tv-text-muted); }
.workspace-watchlist__header button:hover,
.workspace-watchlist__group-row button:hover { background: var(--tv-bg-elevated); color: var(--tv-text); }
.workspace-watchlist__group-row { display: flex; flex: 0 0 auto; align-items: center; gap: 4px; padding: 6px; border-bottom: 1px solid var(--tv-border); }
.workspace-watchlist__group-row select { min-width: 0; height: 29px; flex: 1; padding: 0 7px; border: 1px solid var(--tv-border); border-radius: 5px; outline: 0; background: var(--tv-bg-surface-2); color: var(--tv-text); font-size: 10px; }
.workspace-watchlist__error { display: flex; align-items: center; gap: 5px; padding: 6px 8px; font-size: 9px; }
.workspace-watchlist__error span { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.workspace-watchlist__error button { border: 0; background: transparent; color: inherit; text-decoration: underline; }
.workspace-watchlist footer { display: flex; min-height: 33px; flex: 0 0 auto; align-items: center; justify-content: center; border-top: 1px solid var(--tv-border); background: var(--tv-bg-surface-2); }
.workspace-watchlist footer button { display: inline-flex; align-items: center; gap: 6px; border: 0; background: transparent; color: var(--tv-text-muted); font-size: 10px; }
.workspace-watchlist footer button:hover { color: var(--tv-accent); }
.workspace-watchlist--compact { border-right: 0; box-shadow: 18px 0 42px rgba(2, 6, 23, .3); }
</style>
