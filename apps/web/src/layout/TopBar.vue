<script setup lang="ts">
import { computed, ref, watch } from "vue";

import {
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import { resolveInstrumentRef } from "../composables/instrumentRef";
import { useCommandPalette } from "../composables/useCommandPalette";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";
import { useTheme } from "../composables/useTheme";
import {
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
} from "../composables/useWorkspaceLayout";

const {
  availableBrokerAccounts,
  marketInstrumentSearchOptions,
  selectWorkspaceInstrument,
  selectBrokerAccount,
  selectedBrokerAccount,
  systemStatus, } = useConsoleData();
const { theme, toggle: toggleTheme } = useTheme();
const { unreadCount } = useNotifications();
const { prefs, update } = useWorkspaceTradingPrefs();
const { update: updateViewState } = useWorkspaceViewState();
const palette = useCommandPalette();

const TOPBAR_MARKET_OPTIONS = ["HK", "US", "SH", "SZ", "CN", "SG", "JP", "AU", "MY", "CA", "CRYPTO"].map(
  (market) => ({ value: market, title: formatMarketLabel(market) }),
);

const selectedMarket = ref(prefs.value.market);
const codeInput = ref("");
const tradingEnvironmentFilter = ref<"REAL" | "SIMULATE">("SIMULATE");
const tradingEnvironmentFilterPinned = ref(false);
const brokerAccountFilterQuery = ref("");
const brokerAccountPickerOpen = ref(false);

const tradingEnvironmentFilterLabel = computed(() =>
  tradingEnvironmentFilter.value === "REAL" ? "实盘" : "模拟盘",
);

const favoriteBrokerAccountKeySet = computed(
  () => new Set(prefs.value.favoriteBrokerAccountKeys),
);

const normalizedBrokerAccountFilterQuery = computed(() =>
  brokerAccountFilterQuery.value.trim().toUpperCase(),
);

function isFavoriteBrokerAccount(selectionKey: string): boolean {
  return favoriteBrokerAccountKeySet.value.has(selectionKey);
}

function resolveBrokerAccountSearchText(account: {
  brokerId: string;
  displayName: string;
  accountId: string;
  market: string;
  securityFirm: string | null;
}): string {
  return [
    account.securityFirm ?? "",
    account.brokerId,
    account.displayName,
    account.accountId,
    account.market,
  ]
    .join(" ")
    .trim()
    .toUpperCase();
}

function filterAndSortBrokerAccounts(
  tradingEnvironment: "REAL" | "SIMULATE",
  filterQuery: string,
) {
  const normalizedQuery = filterQuery.trim().toUpperCase();
  return availableBrokerAccounts.value
    .filter((account) => account.tradingEnvironment === tradingEnvironment)
    .filter(
      (account) =>
        normalizedQuery === "" ||
        resolveBrokerAccountSearchText(account).includes(normalizedQuery),
    )
    .sort((left, right) => {
      const leftFavorite = isFavoriteBrokerAccount(left.selectionKey);
      const rightFavorite = isFavoriteBrokerAccount(right.selectionKey);
      if (leftFavorite === rightFavorite) {
        return 0;
      }
      return leftFavorite ? -1 : 1;
    });
}

const filteredBrokerAccounts = computed(() =>
  filterAndSortBrokerAccounts(
    tradingEnvironmentFilter.value,
    brokerAccountFilterQuery.value,
  ),
);

const selectedBrokerAccountSelectionKey = computed(() => {
  if (
    selectedBrokerAccount.value != null &&
    selectedBrokerAccount.value.tradingEnvironment ===
    tradingEnvironmentFilter.value
  ) {
    return selectedBrokerAccount.value.selectionKey;
  }
  return "";
});

function isBrokerAccountSelected(selectionKey: string): boolean {
  return selectedBrokerAccountSelectionKey.value === selectionKey;
}

const brokerAccountLabel = computed(() => {
  if (filteredBrokerAccounts.value.length === 0) {
    if (normalizedBrokerAccountFilterQuery.value !== "") {
      return `筛选后暂无${tradingEnvironmentFilterLabel.value}账户`;
    }
    return `暂无${tradingEnvironmentFilterLabel.value}账户`;
  }

  if (selectedBrokerAccount.value == null) {
    return `请选择${tradingEnvironmentFilterLabel.value}账户`;
  }

  if (
    selectedBrokerAccount.value.tradingEnvironment !==
    tradingEnvironmentFilter.value
  ) {
    return `请选择${tradingEnvironmentFilterLabel.value}账户`;
  }

  return `${selectedBrokerAccount.value.securityFirm ?? "未知券商"} / ${selectedBrokerAccount.value.brokerId.toUpperCase()} / ${selectedBrokerAccount.value.displayName} / ${formatMarketLabel(selectedBrokerAccount.value.market)}`;
});

const codeSuggestions = computed(() => {
  const market = selectedMarket.value.trim().toUpperCase();
  return marketInstrumentSearchOptions.value.filter(
    (option) => market === "" || option.market === market,
  );
});

watch(
  () => prefs.value.market,
  (market) => {
    const normalized = market.trim().toUpperCase();
    if (normalized === "") {
      return;
    }
    selectedMarket.value = normalized;
  },
  { immediate: true },
);

watch(codeInput, (value) => {
  const raw = value.trim();
  if (raw === "" || (!raw.includes(":") && !raw.includes("."))) {
    return;
  }
  const resolved = resolveInstrumentRef({ instrumentId: raw }, selectedMarket.value);
  if (resolved == null) {
    return;
  }
  if (selectedMarket.value !== resolved.market) {
    selectedMarket.value = resolved.market;
  }
  if (codeInput.value.trim().toUpperCase() !== resolved.code) {
    codeInput.value = resolved.code;
  }
});

watch(
  () => selectedBrokerAccount.value?.tradingEnvironment,
  (tradingEnvironment) => {
    if (tradingEnvironmentFilterPinned.value) {
      return;
    }
    tradingEnvironmentFilter.value = tradingEnvironment === "REAL" ? "REAL" : "SIMULATE";
  },
  { immediate: true },
);

watch(
  () => systemStatus.value.defaultTradingEnvironment,
  (tradingEnvironment) => {
    if (tradingEnvironmentFilterPinned.value) {
      return;
    }
    if (selectedBrokerAccount.value != null) {
      return;
    }
    tradingEnvironmentFilter.value = tradingEnvironment === "REAL" ? "REAL" : "SIMULATE";
  },
  { immediate: true },
);

function openRightDock(tab: "notifications" | "ai" | "context"): void {
  updateViewState({ rightDockOpen: true, rightDockTab: tab });
}

function submitSymbol(): void {
  const parsed = resolveInstrumentRef(
    {
      market: selectedMarket.value,
      code: codeInput.value,
    },
    selectedMarket.value,
  );
  if (parsed == null) return;
  selectWorkspaceInstrument({ market: parsed.market, symbol: parsed.code });
  selectedMarket.value = parsed.market;
  codeInput.value = "";
}

function onBrokerAccountChange(event: Event): void {
  const value = (event.target as HTMLSelectElement).value;
  void selectBrokerAccount(value === "" ? null : value);
}

function resolvePreferredBrokerAccountSelectionKey(
  accounts: ReadonlyArray<{ selectionKey: string }>,
): string | null {
  return accounts[0]?.selectionKey ?? null;
}

function applyPreferredBrokerAccountSelection(
  tradingEnvironment: "REAL" | "SIMULATE",
): void {
  const preferredSelectionKey = resolvePreferredBrokerAccountSelectionKey(
    filterAndSortBrokerAccounts(tradingEnvironment, ""),
  );

  if (preferredSelectionKey === selectedBrokerAccount.value?.selectionKey) {
    return;
  }

  void selectBrokerAccount(preferredSelectionKey);
}

function toggleBrokerAccountFavorite(selectionKey: string): void {
  const nextFavorites = prefs.value.favoriteBrokerAccountKeys.filter(
    (key) => key !== selectionKey,
  );

  if (!isFavoriteBrokerAccount(selectionKey)) {
    nextFavorites.unshift(selectionKey);
  }

  update({ favoriteBrokerAccountKeys: nextFavorites });
}

function openBrokerAccountPicker(): void {
  brokerAccountPickerOpen.value = true;
}

function closeBrokerAccountPicker(): void {
  brokerAccountPickerOpen.value = false;
}

function onBrokerAccountPickerVisibilityChange(nextOpen: boolean): void {
  brokerAccountPickerOpen.value = nextOpen;
}

function selectBrokerAccountFromPicker(selectionKey: string): void {
  void selectBrokerAccount(selectionKey);
  closeBrokerAccountPicker();
}

function onTradingEnvironmentSwitch(value: "REAL" | "SIMULATE" | null): void {
  if (value == null) {
    return;
  }
  tradingEnvironmentFilterPinned.value = true;
  tradingEnvironmentFilter.value = value;
  applyPreferredBrokerAccountSelection(value);
}
</script>

<template>
  <header class="tv-topbar">
    <div class="font-bold tracking-wider" style="letter-spacing: 0.18em; color: var(--tv-accent)">
      JFTRADE
    </div>

    <form class="tv-topbar-symbol" data-testid="topbar-instrument-form" @submit.prevent="submitSymbol">
      <span style="color: var(--tv-text-muted); font-size: 11px">⌕</span>
      <select v-model="selectedMarket" class="tv-topbar-symbol__market" data-testid="topbar-instrument-market">
        <option v-for="option in TOPBAR_MARKET_OPTIONS" :key="option.value" :value="option.value">
          {{ option.title }}
        </option>
      </select>
      <input
        v-model="codeInput"
        :placeholder="prefs.symbol"
        list="jftrade-symbol-search"
        spellcheck="false"
        autocomplete="off"
        enterkeyhint="search"
        type="search"
        data-testid="topbar-instrument-code"
        @keydown.enter.prevent="submitSymbol"
      />
      <datalist id="jftrade-symbol-search">
        <option v-for="option in codeSuggestions" :key="option.instrumentId" :value="option.symbol"
          :label="option.label" />
      </datalist>
      <span style="font-size: 10px; color: var(--tv-text-dim)"
        :title="`${codeSuggestions.length} 个可搜索代码，当前市场 ${formatMarketLabel(selectedMarket)}；来源于订阅、持仓、订单和查询缓存`">
        {{ codeSuggestions.length }}
      </span>
      <span style="font-size: 10px; color: var(--tv-text-dim)">⏎</span>
    </form>

    <button type="button" class="tv-btn tv-btn-ghost" style="height: 28px; padding: 0 8px; font-size: 11px"
      @click="palette.show()" title="命令面板（⌘K / Ctrl+K）">
      ⌘K
    </button>

    <div style="flex: 1"></div>

    <div style="display: inline-flex; align-items: center; gap: 8px; font-size: 13px; color: var(--tv-text-muted)">
      <div>
        <v-btn-toggle :model-value="tradingEnvironmentFilter" data-testid="topbar-trading-environment-switch"
          class="tv-topbar-env-toggle" color="teal" density="compact" divided mandatory variant="outlined"
          @update:modelValue="onTradingEnvironmentSwitch" style="width: max-content;">
          <v-btn value="SIMULATE" data-testid="topbar-trading-environment-simulate" size="small"
            class="tv-topbar-env-btn tv-topbar-env-btn--simulate"
            @click="onTradingEnvironmentSwitch('SIMULATE')">
            模拟盘
          </v-btn>
          <v-btn value="REAL" data-testid="topbar-trading-environment-real" size="small"
            class="tv-topbar-env-btn tv-topbar-env-btn--real"
            @click="onTradingEnvironmentSwitch('REAL')">
            实盘
          </v-btn>
        </v-btn-toggle>
      </div>

      <span style="white-space: nowrap;">选定账户</span>
      <button
        type="button"
        class="tv-btn tv-btn-ghost"
        style="height: 28px; padding: 0 10px; font-size: 11px; min-width: 360px; text-align: left"
        data-testid="topbar-broker-account-picker-open"
        @click="openBrokerAccountPicker"
      >
        {{ brokerAccountLabel }}
      </button>
    </div>

    <v-dialog
      :model-value="brokerAccountPickerOpen"
      max-width="760"
      @update:modelValue="onBrokerAccountPickerVisibilityChange"
    >
      <v-card class="tv-topbar-account-picker" data-testid="topbar-broker-account-picker-dialog">
        <v-card-title class="tv-topbar-account-picker__header">
          <span>选择账户</span>
          <button
            type="button"
            class="tv-btn tv-btn-ghost"
            style="height: 28px; padding: 0 8px; font-size: 11px"
            data-testid="topbar-broker-account-picker-close"
            @click="closeBrokerAccountPicker"
          >
            关闭
          </button>
        </v-card-title>

        <v-card-text class="tv-topbar-account-picker__body">
          <v-text-field
            v-model="brokerAccountFilterQuery"
            data-testid="topbar-broker-account-filter"
            placeholder="筛选券商 / 账户名 / 账号 / 市场"
            density="compact"
            variant="outlined"
            hide-details
            clearable
          />

          <div class="tv-topbar-account-picker__list" data-testid="topbar-broker-account-picker-list">
            <div
              v-if="filteredBrokerAccounts.length === 0"
              class="tv-topbar-account-picker__empty"
              data-testid="topbar-broker-account-picker-empty"
            >
              {{ brokerAccountLabel }}
            </div>

            <div
              v-for="account in filteredBrokerAccounts"
              :key="account.selectionKey"
              class="tv-topbar-account-picker__item"
              :class="{ 'is-selected': isBrokerAccountSelected(account.selectionKey) }"
              data-testid="topbar-broker-account-item"
            >
              <button
                type="button"
                class="tv-topbar-account-picker__item-main"
                :title="`${account.securityFirm ?? '未知券商'} / ${account.brokerId.toUpperCase()} / ${account.displayName}`"
                @click="selectBrokerAccountFromPicker(account.selectionKey)"
              >
                <span class="tv-topbar-account-picker__item-main-line">
                  {{ `${account.securityFirm ?? "未知券商"} / ${account.brokerId.toUpperCase()} / ${account.displayName}` }}
                </span>
                <span class="tv-topbar-account-picker__item-sub-line">
                  {{ `${account.accountId} / ${formatTradingEnvironment(account.tradingEnvironment)} / ${formatMarketLabel(account.market)}` }}
                </span>
              </button>

              <button
                type="button"
                class="tv-btn tv-btn-ghost tv-topbar-account-picker__favorite"
                :title="isFavoriteBrokerAccount(account.selectionKey) ? '取消收藏' : '收藏账户'"
                data-testid="topbar-broker-account-item-favorite"
                @click.stop="toggleBrokerAccountFavorite(account.selectionKey)"
              >
                {{ isFavoriteBrokerAccount(account.selectionKey) ? "★" : "☆" }}
              </button>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <button type="button" class="tv-icon-btn" :title="`主题：${theme === 'dark' ? '深色' : '浅色'}`" @click="toggleTheme">
      {{ theme === "dark" ? "☾" : "☀" }}
    </button>

    <button type="button" class="tv-icon-btn" title="通知" @click="openRightDock('notifications')">
      ◔
      <span v-if="unreadCount > 0" class="tv-badge">{{ unreadCount > 99 ? "99+" : unreadCount }}</span>
    </button>

    <button type="button" class="tv-icon-btn" title="AI 助手" @click="openRightDock('ai')">
      ✦
    </button>
  </header>
</template>

<style scoped>
.tv-topbar-symbol__market {
  color: var(--tv-text);
  background: var(--tv-bg-surface-2);
}

.tv-topbar-symbol__market option {
  color: var(--tv-text);
  background: var(--tv-bg-surface);
}

.tv-topbar-account-picker {
  max-width: min(760px, 92vw);
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.tv-topbar-account-picker__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.tv-topbar-account-picker__body {
  display: grid;
  gap: 10px;
  background: var(--tv-bg-surface);
}

.tv-topbar-account-picker__list {
  display: grid;
  gap: 8px;
  max-height: 360px;
  overflow: auto;
}

.tv-topbar-account-picker__empty {
  font-size: 12px;
  color: var(--tv-text-muted);
  padding: 10px;
  border: 1px dashed var(--tv-border);
  border-radius: 8px;
}

.tv-topbar-account-picker__item {
  display: flex;
  align-items: stretch;
  gap: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 8px;
  background: var(--tv-bg-surface-2);
}

.tv-topbar-account-picker__item.is-selected {
  border-color: var(--tv-accent);
}

.tv-topbar-account-picker__item-main {
  flex: 1;
  min-width: 0;
  background: transparent;
  border: none;
  color: inherit;
  text-align: left;
  padding: 8px 10px;
  cursor: pointer;
}

.tv-topbar-account-picker__item-main-line {
  display: block;
  font-size: 12px;
  color: var(--tv-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tv-topbar-account-picker__item-sub-line {
  display: block;
  font-size: 11px;
  color: var(--tv-text-muted);
  margin-top: 3px;
}

.tv-topbar-account-picker__favorite {
  width: 36px;
  min-width: 36px;
  padding: 0;
  font-size: 15px;
}

:deep(.tv-topbar-env-toggle .tv-topbar-env-btn) {
  opacity: 1;
  color: var(--tv-text-muted);
}

:deep(.tv-topbar-env-toggle .tv-topbar-env-btn--simulate.v-btn--active) {
  background: color-mix(in srgb, #facc15 75%, transparent);
  border-color: color-mix(
    in srgb,
    color-mix(in srgb, #facc15 75%, transparent),
    var(--tv-border)
  );
  color: var(--tv-bg-surface);
}

:deep(.tv-topbar-env-toggle .tv-topbar-env-btn--real.v-btn--active) {
  background: color-mix(in srgb, var(--tv-accent) 75%, transparent);
  border-color: color-mix(
    in srgb,
    color-mix(in srgb, var(--tv-accent) 75%, transparent),
    var(--tv-border)
  );
  color: rgba(255, 255, 255, 0.95);
}
</style>
