<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";

import InstrumentIdentity from "../components/domain/market-data/InstrumentIdentity.vue";

import { formatTradingEnvironment } from "../composables/consoleDataFormatting";
import {
  categoryMarketForUser,
  formatInstrumentExchangeTag,
  formatUserMarketLabel,
} from "../composables/instrumentPresentation";
import { useInstrumentResolver } from "../composables/instrumentResolver";
import { useMarketProfiles } from "../composables/marketProfiles";
import { useCommandPalette } from "../composables/useCommandPalette";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";
import { useTheme } from "../composables/useTheme";
import { webLogout } from "../composables/apiClient";
import { resolveDesktopMode } from "../runtimeConfig";
import {
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
} from "../composables/useWorkspaceLayout";

const props = defineProps<{
  compact?: boolean;
}>();

defineEmits<{
  "toggle-nav": [];
}>();

const {
  availableBrokerAccounts,
  marketInstrumentSearchOptions,
  selectWorkspaceInstrument,
  selectBrokerAccount,
  selectedBrokerAccount,
  systemStatus, } = useConsoleData();
const { theme, toggle: toggleTheme } = useTheme();
const notifications = useNotifications();
const { unreadCount } = notifications;
const { prefs, update } = useWorkspaceTradingPrefs();
const { update: updateViewState } = useWorkspaceViewState();
const palette = useCommandPalette();
const {
  marketOptions: topbarMarketOptions,
  loadMarketProfiles,
} = useMarketProfiles();

const selectedMarket = ref(categoryMarketForUser(prefs.value.market));
const codeInput = ref("");
const tradingEnvironmentFilter = ref<"REAL" | "SIMULATE">("SIMULATE");
const tradingEnvironmentFilterPinned = ref(false);
const brokerAccountFilterQuery = ref("");
const brokerAccountPickerOpen = ref(false);
const desktopMode = resolveDesktopMode();
const loggingOut = ref(false);

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

  return `${selectedBrokerAccount.value.securityFirm ?? "未知券商"} / ${selectedBrokerAccount.value.brokerId.toUpperCase()} / ${selectedBrokerAccount.value.displayName} / ${formatUserMarketLabel(selectedBrokerAccount.value.market)}`;
});

const compactBrokerAccountLabel = computed(() => {
  const environmentLabel = tradingEnvironmentFilterLabel.value;
  if (
    selectedBrokerAccount.value == null ||
    selectedBrokerAccount.value.tradingEnvironment !==
      tradingEnvironmentFilter.value
  ) {
    return `${environmentLabel} · 选择账户`;
  }

  return `${environmentLabel} · ${selectedBrokerAccount.value.brokerId.toUpperCase()} / ${selectedBrokerAccount.value.accountId}`;
});

const codeSuggestions = computed(() => {
  const market = categoryMarketForUser(selectedMarket.value);
  return marketInstrumentSearchOptions.value.filter(
    (option) =>
      market === "" || categoryMarketForUser(option.market) === market,
  );
});

watch(
  () => prefs.value.market,
  (market) => {
    const normalized = market.trim().toUpperCase();
    if (normalized === "") {
      return;
    }
    selectedMarket.value = categoryMarketForUser(normalized);
  },
  { immediate: true },
);

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

function openRightDock(tab: "notifications" | "ai"): void {
  updateViewState({ rightDockOpen: true, rightDockTab: tab });
}

function handleResolvedInstrument(candidate: {
  market: string;
  code: string;
}): void {
  selectWorkspaceInstrument({ market: candidate.market, symbol: candidate.code });
  selectedMarket.value = categoryMarketForUser(candidate.market);
  codeInput.value = "";
}

const {
  loading: instrumentResolving,
  panelOpen: instrumentResolutionOpen,
  candidates: instrumentCandidates,
  failures: instrumentFailures,
  resolutionStatus: instrumentResolutionStatus,
  resolutionError: instrumentResolutionError,
  statusMessage: instrumentResolutionMessage,
  activeCandidateIndex,
  resolve: resolveInstrument,
  closePanel: closeInstrumentResolution,
  selectCandidate: selectInstrumentCandidate,
  handleKeydown: handleInstrumentKeydown,
} = useInstrumentResolver({
  market: selectedMarket,
  query: codeInput,
  onResolved: handleResolvedInstrument,
});

function submitSymbol(): void {
  void resolveInstrument();
}

function failureMarketLabel(market: string): string {
  return formatInstrumentExchangeTag(market) ?? formatUserMarketLabel(market);
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

async function logoutWebSession(): Promise<void> {
  if (desktopMode || loggingOut.value) return;
  loggingOut.value = true;
  try {
    await webLogout();
  } catch (error) {
    notifications.push({
      level: "error",
      title: "退出 Web 登录失败",
      message: error instanceof Error ? error.message : "请稍后重试。",
      source: "web-auth",
      category: "system.auth",
    });
  } finally {
    loggingOut.value = false;
  }
}

onMounted(() => {
  void loadMarketProfiles();
});
</script>

<template>
  <header class="tv-topbar" :class="{ 'tv-topbar--compact': compact }">
    <button
      v-if="compact"
      type="button"
      class="tv-icon-btn tv-topbar-nav-button"
      title="导航"
      aria-label="打开导航"
      data-testid="topbar-compact-nav-toggle"
      @click="$emit('toggle-nav')"
    >
      ☰
    </button>

    <div class="tv-topbar-brand font-bold tracking-wider">
      JFTRADE
    </div>

    <form class="tv-topbar-symbol" data-testid="topbar-instrument-form" @submit.prevent="submitSymbol">
      <span style="color: var(--tv-text-muted); font-size: 11px">⌕</span>
      <select v-model="selectedMarket" class="tv-topbar-symbol__market" data-testid="topbar-instrument-market">
        <option v-for="option in topbarMarketOptions" :key="option.value" :value="option.value">
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
        role="combobox"
        :aria-expanded="instrumentResolutionOpen"
        data-testid="topbar-instrument-code"
        @keydown="handleInstrumentKeydown"
      />
      <datalist id="jftrade-symbol-search">
        <option v-for="option in codeSuggestions" :key="option.instrumentId" :value="option.symbol"
          :label="option.label" />
      </datalist>
      <span style="font-size: 10px; color: var(--tv-text-dim)"
        :title="`${codeSuggestions.length} 个可搜索代码，当前市场 ${formatUserMarketLabel(selectedMarket)}；来源于订阅、持仓、订单和查询缓存`">
        {{ codeSuggestions.length }}
      </span>
      <button
        type="button"
        class="tv-topbar-symbol__submit"
        :disabled="instrumentResolving"
        :aria-label="instrumentResolving ? '正在查询标的' : '查询标的'"
        data-testid="topbar-instrument-submit"
        @click="submitSymbol"
      >
        <span class="tv-topbar-symbol__submit-shortcut" aria-hidden="true">
          {{ instrumentResolving ? "···" : "⏎" }}
        </span>
        <span class="tv-topbar-symbol__submit-label">
          {{ instrumentResolving ? "查询中" : "查询" }}
        </span>
      </button>
      <div
        v-if="instrumentResolutionOpen"
        class="tv-topbar-symbol__resolution-panel"
        :class="{ 'tv-topbar-symbol__resolution-panel--warning': instrumentResolutionStatus === 'incomplete' }"
      >
        <div v-if="instrumentResolutionMessage" class="tv-topbar-symbol__resolution-message" role="status">
          {{ instrumentResolutionMessage }}
        </div>
        <div v-if="instrumentFailures.length" class="tv-topbar-symbol__resolution-failures">
          <div v-for="failure in instrumentFailures" :key="`${failure.market}:${failure.code}`">
            {{ failureMarketLabel(failure.market) }}：{{ failure.message }}
          </div>
        </div>
        <button
          v-for="(candidate, index) in instrumentCandidates"
          :key="candidate.instrumentId"
          type="button"
          role="option"
          class="tv-topbar-symbol__resolution-option"
          :class="{ 'tv-topbar-symbol__resolution-option--active': index === activeCandidateIndex }"
          :aria-selected="index === activeCandidateIndex"
          @mouseenter="activeCandidateIndex = index"
          @keydown="handleInstrumentKeydown"
          @click="selectInstrumentCandidate(candidate)"
        >
          <span>{{ formatUserMarketLabel(candidate.market) }}</span>
          <InstrumentIdentity
            :market="candidate.market"
            :code="candidate.code"
            :instrument-id="candidate.instrumentId"
            :name="candidate.name"
            compact
          />
          <span>{{ candidate.securityType || "类型未知" }}</span>
        </button>
        <div class="tv-topbar-symbol__resolution-actions">
          <button
            v-if="instrumentResolutionStatus === 'incomplete' || instrumentResolutionStatus === 'not_found' || instrumentResolutionError"
            type="button"
            :disabled="instrumentResolving"
            @click="submitSymbol"
          >
            重试
          </button>
          <button type="button" @click="closeInstrumentResolution">关闭</button>
        </div>
      </div>
    </form>

    <button type="button" class="tv-btn tv-btn-ghost tv-topbar-command"
      @click="palette.show()" title="命令面板（⌘K / Ctrl+K）">
      ⌘K
    </button>

    <div class="tv-topbar-spacer"></div>

    <div class="tv-topbar-account-group">
      <div v-if="!props.compact" class="tv-topbar-env-control">
        <v-btn-toggle :model-value="tradingEnvironmentFilter" data-testid="topbar-trading-environment-switch"
          class="tv-topbar-env-toggle" color="teal" density="compact" divided mandatory variant="outlined"
          @update:modelValue="onTradingEnvironmentSwitch">
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

      <span class="tv-topbar-account-label">选定账户</span>
      <button
        type="button"
        class="tv-btn tv-btn-ghost tv-topbar-account-trigger"
        data-testid="topbar-broker-account-picker-open"
        :title="brokerAccountLabel"
        @click="openBrokerAccountPicker"
      >
        {{ props.compact ? compactBrokerAccountLabel : brokerAccountLabel }}
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
          <div class="tv-topbar-account-picker__env">
            <span class="tv-topbar-account-picker__env-label">交易环境</span>
            <v-btn-toggle
              :model-value="tradingEnvironmentFilter"
              data-testid="topbar-account-picker-trading-environment-switch"
              class="tv-topbar-env-toggle"
              color="teal"
              density="compact"
              divided
              mandatory
              variant="outlined"
              @update:modelValue="onTradingEnvironmentSwitch"
            >
              <v-btn
                value="SIMULATE"
                data-testid="topbar-account-picker-trading-environment-simulate"
                size="small"
                class="tv-topbar-env-btn tv-topbar-env-btn--simulate"
                @click="onTradingEnvironmentSwitch('SIMULATE')"
              >
                模拟盘
              </v-btn>
              <v-btn
                value="REAL"
                data-testid="topbar-account-picker-trading-environment-real"
                size="small"
                class="tv-topbar-env-btn tv-topbar-env-btn--real"
                @click="onTradingEnvironmentSwitch('REAL')"
              >
                实盘
              </v-btn>
            </v-btn-toggle>
          </div>

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
                  {{ `${account.accountId} / ${formatTradingEnvironment(account.tradingEnvironment)} / ${formatUserMarketLabel(account.market)}` }}
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

    <div class="tv-topbar-actions">
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

    <button
      v-if="!desktopMode"
      type="button"
      class="tv-btn tv-btn-ghost"
      data-testid="web-logout"
      :disabled="loggingOut"
      title="退出当前 Web 登录"
      @click="logoutWebSession"
    >
      {{ loggingOut ? "退出中…" : "退出 Web" }}
    </button>
    </div>
  </header>
</template>

<style scoped>
.tv-topbar-brand {
  flex: 0 0 auto;
  letter-spacing: 0.18em;
  color: var(--tv-accent);
}

.tv-topbar-command {
  flex: 0 0 auto;
  height: 28px;
  padding: 0 8px;
  font-size: 11px;
}

.tv-topbar-spacer {
  flex: 1 1 auto;
  min-width: 0;
}

.tv-topbar-account-group {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--tv-text-muted);
  font-size: 13px;
}

.tv-topbar-env-control {
  flex: 0 0 auto;
  min-width: 0;
}

.tv-topbar-env-toggle {
  width: max-content;
}

.tv-topbar-account-label {
  flex: 0 0 auto;
  white-space: nowrap;
}

.tv-topbar-account-trigger {
  height: 28px;
  min-width: 0;
  max-width: min(360px, 28vw);
  padding: 0 10px;
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
}

.tv-topbar-actions {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 2px;
}

.tv-topbar-nav-button {
  flex: 0 0 auto;
}

.tv-topbar-symbol__market {
  color: var(--tv-text);
  background: var(--tv-bg-surface-2);
}

.tv-topbar-symbol__market option {
  color: var(--tv-text);
  background: var(--tv-bg-surface);
}

:global(.tv-topbar-symbol) {
  position: relative;
}

.tv-topbar-symbol__submit {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 22px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-dim);
  padding: 0 5px;
  font: inherit;
  font-size: 10px;
  line-height: 1;
  cursor: pointer;
  transition:
    border-color 120ms ease,
    background-color 120ms ease,
    color 120ms ease;
}

.tv-topbar-symbol__submit-label {
  display: none;
}

.tv-topbar-symbol__submit:hover,
.tv-topbar-symbol__submit:focus-visible {
  border-color: var(--tv-border-strong);
  background: var(--tv-bg-hover);
  color: var(--tv-text);
  outline: none;
}

.tv-topbar-symbol__submit:hover .tv-topbar-symbol__submit-shortcut,
.tv-topbar-symbol__submit:focus-visible .tv-topbar-symbol__submit-shortcut {
  display: none;
}

.tv-topbar-symbol__submit:hover .tv-topbar-symbol__submit-label,
.tv-topbar-symbol__submit:focus-visible .tv-topbar-symbol__submit-label {
  display: inline;
}

.tv-topbar-symbol__submit:disabled {
  cursor: wait;
  opacity: 0.7;
}

.tv-topbar-symbol__resolution-panel {
  position: absolute;
  z-index: 80;
  top: calc(100% + 6px);
  right: 0;
  left: 0;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  box-shadow: var(--tv-shadow-lg);
  color: var(--tv-text);
}

.tv-topbar-symbol__resolution-panel--warning {
  border-color: var(--tv-warn);
}

.tv-topbar-symbol__resolution-message,
.tv-topbar-symbol__resolution-failures {
  padding: 7px 10px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.tv-topbar-symbol__resolution-failures {
  padding-top: 0;
  color: var(--tv-warn);
}

.tv-topbar-symbol__resolution-option {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: 0;
  border-top: 1px solid var(--tv-border);
  background: transparent;
  color: inherit;
  padding: 7px 10px;
  text-align: left;
  cursor: pointer;
}

.tv-topbar-symbol__resolution-option > span:first-child,
.tv-topbar-symbol__resolution-option > span:last-child {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.tv-topbar-symbol__resolution-option--active,
.tv-topbar-symbol__resolution-option:hover {
  background: var(--tv-bg-hover);
}

.tv-topbar-symbol__resolution-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  border-top: 1px solid var(--tv-border);
  padding: 6px 8px;
}

.tv-topbar-symbol__resolution-actions button {
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  padding: 2px 7px;
  font-size: 10px;
  cursor: pointer;
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

.tv-topbar-account-picker__env {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.tv-topbar-account-picker__env-label {
  flex: 0 0 auto;
  color: var(--tv-text-muted);
  font-size: 12px;
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

@media (max-width: 900px) {
  :global(.tv-topbar-symbol) {
    min-width: min(300px, 42vw);
  }

  .tv-topbar-account-trigger {
    max-width: min(280px, 24vw);
  }
}

@media (max-width: 1180px) {
  .tv-topbar--compact {
    box-sizing: border-box;
    display: grid;
    grid-template-columns: auto auto minmax(76px, 1fr) auto;
    grid-template-areas:
      "nav brand account actions"
      "search search search search";
    gap: 5px 6px;
    align-items: center;
    width: 100%;
    max-width: 100vw;
    min-width: 0;
    overflow: visible;
    padding: 5px 6px;
  }

  .tv-topbar--compact .tv-topbar-nav-button {
    grid-area: nav;
  }

  .tv-topbar-brand {
    min-width: 0;
    letter-spacing: 0.06em;
    font-size: 11px;
  }

  .tv-topbar--compact .tv-topbar-brand {
    grid-area: brand;
  }

  .tv-topbar--compact :global(.tv-topbar-symbol) {
    box-sizing: border-box;
    grid-area: search;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    gap: 4px;
    padding: 3px 6px;
  }

  .tv-topbar--compact .tv-topbar-symbol__market {
    flex: 0 1 74px;
    width: 74px;
    min-width: 74px;
    max-width: 86px;
  }

  .tv-topbar--compact :global(.tv-topbar-symbol select),
  .tv-topbar--compact :global(.tv-topbar-symbol input) {
    min-width: 0;
  }

  .tv-topbar--compact .tv-topbar-command,
  .tv-topbar--compact .tv-topbar-spacer {
    display: none;
  }

  .tv-topbar--compact .tv-topbar-account-group {
    grid-area: account;
    justify-self: stretch;
    width: auto;
    min-width: 0;
  }

  .tv-topbar--compact .tv-topbar-account-label {
    display: none;
  }

  .tv-topbar--compact .tv-topbar-account-trigger {
    width: 100%;
    max-width: none;
    padding: 0 6px;
    text-align: center;
    font-size: 10px;
  }

  .tv-topbar--compact .tv-topbar-actions {
    grid-area: actions;
    justify-self: end;
    gap: 0;
  }

  .tv-topbar--compact .tv-icon-btn {
    width: 30px;
    height: 30px;
  }

  .tv-topbar--compact .tv-topbar-command {
    display: none;
  }
}
</style>
