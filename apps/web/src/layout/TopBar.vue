<script setup lang="ts">
import { computed } from "vue";

import TradingEnvironmentBadge from "../components/TradingEnvironmentBadge.vue";
import { useCommandPalette } from "../composables/useCommandPalette";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";
import { useSharedLiveSocket } from "../composables/useSharedLiveSocket";
import { useTheme } from "../composables/useTheme";
import { useWorkspaceLayout } from "../composables/useWorkspaceLayout";

const {
  availableBrokerAccounts,
  liveStreamStatus,
  marketInstrumentSearchOptions,
  resolveMarketInstrumentInput,
  selectBrokerAccount,
  selectedBrokerAccount,
  systemStatus,
} = useConsoleData();
const { connectionState } = useSharedLiveSocket();
const { theme, toggle: toggleTheme } = useTheme();
const { unreadCount } = useNotifications();
const { prefs, update } = useWorkspaceLayout();
const palette = useCommandPalette();

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

  return `${selectedBrokerAccount.value.brokerId.toUpperCase()} / ${selectedBrokerAccount.value.displayName} / ${selectedBrokerAccount.value.market}`;
});

function openRightDock(tab: "notifications" | "ai" | "context"): void {
  update({ rightDockOpen: true, rightDockTab: tab });
}

function onSymbolSubmit(event: Event): void {
  event.preventDefault();
  const input = (event.target as HTMLFormElement).querySelector("input");
  if (!input) return;
  const parsed = resolveMarketInstrumentInput(input.value);
  if (parsed == null) return;
  update({ market: parsed.market, symbol: parsed.symbol });
  input.value = "";
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

    <form class="tv-topbar-symbol" @submit="onSymbolSubmit">
      <span style="color: var(--tv-text-muted); font-size: 11px">⌕</span>
      <input
        :placeholder="`${prefs.market}:${prefs.symbol}`"
        list="jftrade-symbol-search"
        spellcheck="false"
        autocomplete="off"
      />
      <datalist id="jftrade-symbol-search">
        <option
          v-for="option in marketInstrumentSearchOptions"
          :key="option.instrumentId"
          :value="option.lookupValue"
          :label="option.label"
        />
      </datalist>
      <span
        style="font-size: 10px; color: var(--tv-text-dim)"
        :title="`${marketInstrumentSearchOptions.length} searchable code(s) from subscriptions, positions, orders and query cache`"
      >
        {{ marketInstrumentSearchOptions.length }}
      </span>
      <span style="font-size: 10px; color: var(--tv-text-dim)">⏎</span>
    </form>

    <button
      type="button"
      class="tv-btn tv-btn-ghost"
      style="height: 28px; padding: 0 8px; font-size: 11px"
      @click="palette.show()"
      title="Command palette (⌘K / Ctrl+K)"
    >
      ⌘K
    </button>

    <div style="flex: 1"></div>

    <label
      style="display: inline-flex; align-items: center; gap: 8px; font-size: 11px; color: var(--tv-text-muted)"
    >
      <span>Scope</span>
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
          {{ `${account.brokerId.toUpperCase()} / ${account.displayName} / ${account.accountId} / ${account.tradingEnvironment} / ${account.market}` }}
        </option>
      </select>
    </label>

    <TradingEnvironmentBadge :env="env" />

    <div style="display: flex; gap: 12px; font-size: 11px; color: var(--tv-text-muted)">
      <span>
        <span class="tv-status-dot" :class="liveStreamStatus === 'connected' ? 'tv-dot-ok' : liveStreamStatus === 'degraded' ? 'tv-dot-warn' : 'tv-dot-idle'"></span>
        SSE
      </span>
      <span>
        <span class="tv-status-dot" :class="connectionState === 'connected' ? 'tv-dot-ok' : connectionState === 'error' ? 'tv-dot-err' : 'tv-dot-idle'"></span>
        WS
      </span>
    </div>

    <button type="button" class="tv-icon-btn" :title="`Theme: ${theme}`" @click="toggleTheme">
      {{ theme === "dark" ? "☾" : "☀" }}
    </button>

    <button type="button" class="tv-icon-btn" title="Notifications" @click="openRightDock('notifications')">
      ◔
      <span v-if="unreadCount > 0" class="tv-badge">{{ unreadCount > 99 ? "99+" : unreadCount }}</span>
    </button>

    <button type="button" class="tv-icon-btn" title="AI Assistant" @click="openRightDock('ai')">
      ✦
    </button>
  </header>
</template>
