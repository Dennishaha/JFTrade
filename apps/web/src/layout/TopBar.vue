<script setup lang="ts">
import { computed, ref, watch } from "vue";

import TradingEnvironmentBadge from "../components/TradingEnvironmentBadge.vue";
import {
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import { resolveInstrumentRef } from "../composables/instrumentRef";
import { useCommandPalette } from "../composables/useCommandPalette";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";
import { useTheme } from "../composables/useTheme";
import { useWorkspaceLayout } from "../composables/useWorkspaceLayout";

const {
  availableBrokerAccounts,
  marketInstrumentSearchOptions,
  selectBrokerAccount,
  selectedBrokerAccount,
  systemStatus,} = useConsoleData();
const { theme, toggle: toggleTheme } = useTheme();
const { unreadCount } = useNotifications();
const { prefs, update } = useWorkspaceLayout();
const palette = useCommandPalette();

const TOPBAR_MARKET_OPTIONS = ["HK", "US", "SH", "SZ", "CN", "SG", "JP", "AU", "MY", "CA", "CRYPTO"].map(
  (market) => ({ value: market, title: formatMarketLabel(market) }),
);

const selectedMarket = ref(prefs.value.market);
const codeInput = ref("");

const env = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment ??
    "SIMULATE",
);

const brokerAccountLabel = computed(() => {
  if (selectedBrokerAccount.value == null) {
    return "未选择账号";
  }

  return `${selectedBrokerAccount.value.brokerId.toUpperCase()} / ${selectedBrokerAccount.value.displayName} / ${formatMarketLabel(selectedBrokerAccount.value.market)}`;
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

function openRightDock(tab: "notifications" | "ai" | "context"): void {
  update({ rightDockOpen: true, rightDockTab: tab });
}

function onSymbolSubmit(event: Event): void {
  event.preventDefault();
  const parsed = resolveInstrumentRef(
    {
      market: selectedMarket.value,
      code: codeInput.value,
    },
    selectedMarket.value,
  );
  if (parsed == null) return;
  update({ market: parsed.market, symbol: parsed.code });
  selectedMarket.value = parsed.market;
  codeInput.value = "";
}

function onBrokerAccountChange(event: Event): void {
  const value = (event.target as HTMLSelectElement).value;
  void selectBrokerAccount(value === "" ? null : value);
}
</script>

<template>
  <header class="tv-topbar">
    <div class="font-bold tracking-wider" style="letter-spacing: 0.18em; color: var(--tv-accent)">
      JFTRADE
    </div>

    <form class="tv-topbar-symbol" data-testid="topbar-instrument-form" @submit="onSymbolSubmit">
      <span style="color: var(--tv-text-muted); font-size: 11px">⌕</span>
      <select
        v-model="selectedMarket"
        class="tv-topbar-symbol__market"
        data-testid="topbar-instrument-market"
      >
        <option
          v-for="option in TOPBAR_MARKET_OPTIONS"
          :key="option.value"
          :value="option.value"
        >
          {{ option.title }}
        </option>
      </select>
      <input
        v-model="codeInput"
        :placeholder="prefs.symbol"
        list="jftrade-symbol-search"
        spellcheck="false"
        autocomplete="off"
        data-testid="topbar-instrument-code"
      />
      <datalist id="jftrade-symbol-search">
        <option
          v-for="option in codeSuggestions"
          :key="option.instrumentId"
          :value="option.symbol"
          :label="option.label"
        />
      </datalist>
      <span
        style="font-size: 10px; color: var(--tv-text-dim)"
        :title="`${codeSuggestions.length} 个可搜索代码，当前市场 ${formatMarketLabel(selectedMarket)}；来源于订阅、持仓、订单和查询缓存`"
      >
        {{ codeSuggestions.length }}
      </span>
      <span style="font-size: 10px; color: var(--tv-text-dim)">⏎</span>
    </form>

    <button
      type="button"
      class="tv-btn tv-btn-ghost"
      style="height: 28px; padding: 0 8px; font-size: 11px"
      @click="palette.show()"
      title="命令面板（⌘K / Ctrl+K）"
    >
      ⌘K
    </button>

    <div style="flex: 1"></div>

    <label
      style="display: inline-flex; align-items: center; gap: 8px; font-size: 11px; color: var(--tv-text-muted)"
    >
      <span style="white-space: nowrap;">选定账户</span>
      <select
        :value="selectedBrokerAccount?.selectionKey ?? ''"
        class="tv-select"
        style="height: 28px; min-width: 260px"
        @change="onBrokerAccountChange"
      >
        <option value="">{{ brokerAccountLabel }}</option>
        <option
          v-for="account in availableBrokerAccounts"
          :key="account.selectionKey"
          :value="account.selectionKey"
        >
          {{ `${account.brokerId.toUpperCase()} / ${account.displayName} / ${account.accountId} / ${formatTradingEnvironment(account.tradingEnvironment)} / ${formatMarketLabel(account.market)}` }}
        </option>
      </select>
    </label>

    <TradingEnvironmentBadge :env="env" />

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
