<script setup lang="ts">
import { computed } from "vue";

import {
  formatAccountTypeLabel,
  formatConnectivityLabel,
  formatTradingEnvironment,
} from "../../../composables/consoleDataFormatting";
import { formatUserMarketLabel } from "../../../composables/instrumentPresentation";
import { useConsoleData } from "../../../composables/useConsoleData";
import { formatMoney } from "../../../utils/numberFormat";

const {
  brokerFunds,
  brokerRuntime,
  portfolioCashBalances,
  selectedBrokerAccount,
  systemStatus,
} = useConsoleData();

const selectedRuntimeAccount = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return brokerRuntime.value.accounts[0] ?? null;
  }

  return (
    brokerRuntime.value.accounts.find(
      (account) =>
        account.accountId === selected.accountId &&
        account.tradingEnvironment === selected.tradingEnvironment,
    ) ?? null
  );
});

const accountTitle = computed(() => {
  if (selectedBrokerAccount.value != null) {
    return selectedBrokerAccount.value.displayName;
  }
  return selectedRuntimeAccount.value?.accountId ?? "未选择账户";
});

const summary = computed(() => brokerFunds.value.summary);
const currency = computed(() => summary.value?.currency ?? undefined);

const activeTradingEnvironment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    selectedRuntimeAccount.value?.tradingEnvironment ??
    summary.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment ??
    null,
);

function matchesActiveTradingEnvironment(tradingEnvironment: string): boolean {
  const active = activeTradingEnvironment.value;
  if (active == null || active.trim() === "") {
    return false;
  }
  return (
    tradingEnvironment.trim().toUpperCase() === active.trim().toUpperCase()
  );
}

const totalCash = computed(() => {
  if (summary.value?.cash != null) {
    return summary.value.cash;
  }
  const selected = selectedBrokerAccount.value;
  const balances = selected == null
    ? portfolioCashBalances.value.balances.filter((balance) =>
        matchesActiveTradingEnvironment(balance.tradingEnvironment),
      )
    : portfolioCashBalances.value.balances.filter(
        (balance) =>
          balance.brokerId === selected.brokerId &&
          balance.accountId === selected.accountId &&
          balance.tradingEnvironment === selected.tradingEnvironment,
      );
  if (balances.length === 0) {
    return null;
  }
  return balances.reduce(
    (sum, balance) => sum + (balance.cashBalance ?? 0),
    0,
  );
});

const overviewRows = computed(() => [
  { label: "现金", value: totalCash.value },
  { label: "证券市值", value: summary.value?.marketValue },
  { label: "冻结资金", value: summary.value?.frozenCash },
  { label: "购买力", value: summary.value?.purchasingPower },
]);

const accountFacts = computed(() => {
  const selected = selectedBrokerAccount.value;
  const runtimeAccount = selectedRuntimeAccount.value;

  return [
    {
      label: "券商",
      value:
        selected?.brokerId.toUpperCase() ??
        brokerRuntime.value.descriptor.displayName ??
        "未设置",
    },
    {
      label: "账户号",
      value: selected?.accountId ?? runtimeAccount?.accountId ?? "未设置",
    },
    {
      label: "交易环境",
      value: formatTradingEnvironment(
        selected?.tradingEnvironment ?? runtimeAccount?.tradingEnvironment,
      ),
    },
    {
      label: "市场",
      value: formatUserMarketLabel(
        selected?.market ?? runtimeAccount?.marketAuthorities[0],
      ),
    },
    {
      label: "账户类型",
      value: formatAccountTypeLabel(runtimeAccount?.accountType),
    },
    {
      label: "券商机构",
      value: runtimeAccount?.securityFirm ?? selected?.securityFirm ?? "未设置",
    },
  ];
});

const connectivityLabel = computed(() =>
  formatConnectivityLabel(brokerRuntime.value.session.connectivity),
);
const isConnected = computed(() => {
  const connectivity = String(
    brokerRuntime.value.session.connectivity ?? "",
  ).toLowerCase();
  return connectivity.includes("connect") && !connectivity.includes("disconnect");
});

function pnlClass(value: number | null | undefined): string {
  if (value == null || value === 0) return "is-flat";
  return value > 0 ? "tv-up" : "tv-down";
}

function formatPnl(value: number | null | undefined): string {
  if (value == null) return "--";
  const sign = value > 0 ? "+" : "";
  return `${sign}${formatMoney(value, currency.value, { maximumFractionDigits: 2 })}`;
}

function formatOverviewMoney(value: number | null | undefined): string {
  if (value == null) return "--";
  return formatMoney(value, currency.value, { maximumFractionDigits: 2 });
}
</script>

<template>
  <aside class="account-sidebar" aria-label="账户摘要">
    <div class="account-sidebar__head">
      <div class="account-sidebar__name" :title="accountTitle">{{ accountTitle }}</div>
      <span
        class="account-sidebar__connectivity"
        :class="isConnected ? 'tv-status--success' : 'tv-status--warning'"
      >
        <i class="tv-state-dot"></i>{{ connectivityLabel }}
      </span>
    </div>

    <div class="account-sidebar__total">
      <div class="account-sidebar__total-label">
        总资产 <span>{{ currency ?? "币种未设置" }}</span>
      </div>
      <div class="account-sidebar__total-value tv-num">
        {{ formatMoney(summary?.totalAssets, currency, { maximumFractionDigits: 2 }) }}
      </div>
      <div class="account-sidebar__pnl">
        <div>
          <span>持仓盈亏</span>
          <b class="tv-num" :class="pnlClass(summary?.unrealizedPnl)">
            {{ formatPnl(summary?.unrealizedPnl) }}
          </b>
        </div>
        <div>
          <span>已实现盈亏</span>
          <b class="tv-num" :class="pnlClass(summary?.realizedPnl)">
            {{ formatPnl(summary?.realizedPnl) }}
          </b>
        </div>
      </div>
    </div>

    <div class="account-sidebar__rows">
      <div v-for="row in overviewRows" :key="row.label" class="account-sidebar__row">
        <span>{{ row.label }}</span>
        <b class="tv-num">{{ formatOverviewMoney(row.value) }}</b>
      </div>
    </div>

    <div class="account-sidebar__facts">
      <div v-for="fact in accountFacts" :key="fact.label" class="account-sidebar__fact">
        <span>{{ fact.label }}</span>
        <b :title="fact.value">{{ fact.value }}</b>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.account-sidebar {
  display: flex;
  width: 264px;
  flex: 0 0 auto;
  flex-direction: column;
  overflow: hidden auto;
  border: 1px solid var(--tv-border);
  border-radius: 9px;
  background: var(--tv-bg-surface);
  box-shadow: 0 8px 24px color-mix(in srgb, #000 8%, transparent);
  scrollbar-width: thin;
}

.account-sidebar__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.account-sidebar__name {
  overflow: hidden;
  color: var(--tv-text);
  font-size: 13px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-sidebar__connectivity {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-dim));
  font-size: 10px;
}

.account-sidebar__total {
  padding: 14px;
  border-bottom: 1px solid var(--tv-border);
}

.account-sidebar__total-label {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.account-sidebar__total-label span {
  margin-left: 6px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.account-sidebar__total-value {
  margin-top: 4px;
  color: var(--tv-text);
  font-size: 26px;
  font-weight: 680;
  letter-spacing: -0.02em;
}

.account-sidebar__pnl {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-top: 10px;
}

.account-sidebar__pnl span {
  display: block;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.account-sidebar__pnl b {
  font-size: 12px;
  font-weight: 600;
}

.account-sidebar__pnl b.is-flat {
  color: var(--tv-text-muted);
}

.account-sidebar__rows {
  display: grid;
  gap: 2px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--tv-border);
}

.account-sidebar__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 0;
  font-size: 12px;
}

.account-sidebar__row span {
  color: var(--tv-text-muted);
}

.account-sidebar__row b {
  color: var(--tv-text);
  font-weight: 550;
}

.account-sidebar__facts {
  display: grid;
  gap: 2px;
  padding: 10px 14px 14px;
}

.account-sidebar__fact {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 3px 0;
  font-size: 11px;
}

.account-sidebar__fact span {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
}

.account-sidebar__fact b {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1180px) {
  .account-sidebar {
    width: 100%;
    flex: 0 0 auto;
  }

  .account-sidebar__facts {
    grid-template-columns: 1fr 1fr;
    column-gap: 16px;
  }
}
</style>
