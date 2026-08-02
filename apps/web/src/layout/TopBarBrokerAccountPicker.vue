<script setup lang="ts">
import type { BrokerAccountSelectionOption } from "@/composables/trading/consoleDataBrokerAccountSelection";
import { formatTradingEnvironment } from "@/composables/shared/consoleDataFormatting";
import { formatUserMarketLabel } from "@/composables/market-data/instrumentPresentation";

type TradingEnvironment = "REAL" | "SIMULATE";

const props = defineProps<{
  open: boolean;
  tradingEnvironment: TradingEnvironment;
  filterQuery: string;
  accounts: readonly BrokerAccountSelectionOption[];
  emptyLabel: string;
  selectedSelectionKey: string;
  favoriteSelectionKeys: readonly string[];
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  "update:filter-query": [value: string];
  "switch-environment": [value: TradingEnvironment];
  select: [selectionKey: string];
  "toggle-favorite": [selectionKey: string];
}>();

function isFavorite(selectionKey: string): boolean {
  return props.favoriteSelectionKeys.includes(selectionKey);
}

function updateFilterQuery(value: unknown): void {
  emit("update:filter-query", typeof value === "string" ? value : "");
}

function switchEnvironment(value: unknown): void {
  if (value === "REAL" || value === "SIMULATE") {
    emit("switch-environment", value);
  }
}
</script>

<template>
  <v-dialog
    :model-value="open"
    max-width="760"
    @update:model-value="$emit('update:open', $event)"
  >
    <v-card
      class="tv-topbar-account-picker"
      data-testid="topbar-broker-account-picker-dialog"
    >
      <v-card-title class="tv-topbar-account-picker__header">
        <span>选择账户</span>
        <button
          type="button"
          class="tv-btn tv-btn-ghost jf-btn-sm"
          data-testid="topbar-broker-account-picker-close"
          @click="$emit('update:open', false)"
        >
          关闭
        </button>
      </v-card-title>

      <v-card-text class="tv-topbar-account-picker__body">
        <div class="tv-topbar-account-picker__env">
          <span class="tv-topbar-account-picker__env-label">交易环境</span>
          <v-btn-toggle
            :model-value="tradingEnvironment"
            data-testid="topbar-account-picker-trading-environment-switch"
            class="tv-topbar-env-toggle"
            color="teal"
            density="compact"
            divided
            mandatory
            variant="outlined"
            @update:model-value="switchEnvironment"
          >
            <v-btn
              value="SIMULATE"
              data-testid="topbar-account-picker-trading-environment-simulate"
              size="small"
              class="tv-topbar-env-btn tv-topbar-env-btn--simulate"
              @click="switchEnvironment('SIMULATE')"
            >
              模拟盘
            </v-btn>
            <v-btn
              value="REAL"
              data-testid="topbar-account-picker-trading-environment-real"
              size="small"
              class="tv-topbar-env-btn tv-topbar-env-btn--real"
              @click="switchEnvironment('REAL')"
            >
              实盘
            </v-btn>
          </v-btn-toggle>
        </div>

        <v-text-field
          :model-value="filterQuery"
          data-testid="topbar-broker-account-filter"
          placeholder="筛选券商 / 账户名 / 账号 / 市场"
          density="compact"
          variant="outlined"
          hide-details
          clearable
          @update:model-value="updateFilterQuery"
        />

        <div
          class="tv-topbar-account-picker__list"
          data-testid="topbar-broker-account-picker-list"
        >
          <div
            v-if="accounts.length === 0"
            class="tv-topbar-account-picker__empty"
            data-testid="topbar-broker-account-picker-empty"
          >
            {{ emptyLabel }}
          </div>

          <div
            v-for="account in accounts"
            :key="account.selectionKey"
            class="tv-topbar-account-picker__item"
            :class="{
              'is-selected': selectedSelectionKey === account.selectionKey,
            }"
            data-testid="topbar-broker-account-item"
          >
            <button
              type="button"
              class="tv-topbar-account-picker__item-main"
              :title="`${account.securityFirm ?? '未知券商'} / ${account.brokerId.toUpperCase()} / ${account.displayName}`"
              @click="$emit('select', account.selectionKey)"
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
              :title="isFavorite(account.selectionKey) ? '取消收藏' : '收藏账户'"
              data-testid="topbar-broker-account-item-favorite"
              @click.stop="$emit('toggle-favorite', account.selectionKey)"
            >
              {{ isFavorite(account.selectionKey) ? "★" : "☆" }}
            </button>
          </div>
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style scoped>
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
  font-size: var(--jf-text-6);
}

.tv-topbar-account-picker__list {
  display: grid;
  gap: 8px;
  max-height: 360px;
  overflow: auto;
}

.tv-topbar-account-picker__empty {
  padding: 10px;
  border: 1px dashed var(--tv-border);
  border-radius: 8px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-6);
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
  padding: 8px 10px;
  border: none;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.tv-topbar-account-picker__item-main-line {
  display: block;
  overflow: hidden;
  color: var(--tv-text);
  font-size: var(--jf-text-6);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tv-topbar-account-picker__item-sub-line {
  display: block;
  margin-top: 3px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.tv-topbar-account-picker__favorite {
  width: 36px;
  min-width: 36px;
  padding: 0;
  font-size: var(--jf-text-9);
}
</style>
