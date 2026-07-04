<script setup lang="ts">
import { computed } from "vue";

import {
  formatConnectivityLabel,
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../composables/useWorkspaceLayout";

const { brokerRuntime, selectedBrokerAccount, systemStatus } = useConsoleData();
const { prefs } = useWorkspaceTradingPrefs();

const tradingEnvironment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment,
);
const isRealTradingEnvironment = computed(
  () => tradingEnvironment.value.trim().toUpperCase() === "REAL",
);
const accountLabel = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return "未选择账户";
  }
  return selected.displayName.trim() || selected.accountId;
});
const market = computed(
  () =>
    prefs.value.market ||
    selectedBrokerAccount.value?.market ||
    systemStatus.value.broker.capabilities[0]?.market ||
    "",
);
const brokerLabel = computed(
  () =>
    brokerRuntime.value.session.displayName.trim() ||
    brokerRuntime.value.descriptor.displayName.trim() ||
    systemStatus.value.broker.displayName.trim() ||
    systemStatus.value.defaultBroker,
);
const connectivity = computed(() => brokerRuntime.value.session.connectivity);
const symbolLabel = computed(() => {
  const symbol = prefs.value.symbol.trim().toUpperCase();
  const marketCode = market.value.trim().toUpperCase();
  if (symbol === "") {
    return "未设置";
  }
  if (marketCode === "") {
    return symbol;
  }
  return `${marketCode}.${symbol}`;
});
const realTradingStatus = computed(() => {
  if (!isRealTradingEnvironment.value) {
    return "非实盘";
  }
  if (systemStatus.value.realTradingKillSwitch.active) {
    return "实盘已熔断";
  }
  if (!systemStatus.value.realTradingEnabled) {
    return "实盘未启用";
  }
  return "实盘可下单";
});
</script>

<template>
  <section
    class="trading-scope-bar"
    :class="{ 'trading-scope-bar--real': isRealTradingEnvironment }"
    aria-label="交易作用域"
    data-testid="trading-scope-bar"
  >
    <div class="trading-scope-bar__group trading-scope-bar__group--primary">
      <span
        class="trading-scope-bar__chip trading-scope-bar__chip--env"
        :class="{ 'trading-scope-bar__chip--real': isRealTradingEnvironment }"
        data-testid="trading-scope-env"
      >
        {{ formatTradingEnvironment(tradingEnvironment) }}
      </span>
      <span
        class="trading-scope-bar__status"
        :class="{ 'trading-scope-bar__status--real': isRealTradingEnvironment }"
        data-testid="trading-scope-real-status"
      >
        {{ realTradingStatus }}
      </span>
    </div>

    <dl class="trading-scope-bar__items">
      <div class="trading-scope-bar__item" data-testid="trading-scope-account">
        <dt>账户</dt>
        <dd>{{ accountLabel }}</dd>
      </div>
      <div class="trading-scope-bar__item" data-testid="trading-scope-market">
        <dt>市场</dt>
        <dd>{{ formatMarketLabel(market) }}</dd>
      </div>
      <div class="trading-scope-bar__item" data-testid="trading-scope-symbol">
        <dt>标的</dt>
        <dd>{{ symbolLabel }}</dd>
      </div>
      <div class="trading-scope-bar__item" data-testid="trading-scope-broker">
        <dt>券商</dt>
        <dd>{{ brokerLabel }}</dd>
      </div>
      <div class="trading-scope-bar__item" data-testid="trading-scope-connectivity">
        <dt>连接</dt>
        <dd>{{ formatConnectivityLabel(connectivity) }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.trading-scope-bar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-height: 2.75rem;
  padding: 0.45rem 0.7rem;
  border: 1px solid var(--tv-border);
  background: color-mix(in srgb, var(--tv-bg-surface) 94%, var(--tv-bg-elevated));
  color: var(--tv-text);
}

.trading-scope-bar--real {
  border-color: color-mix(in srgb, #dc2626 44%, var(--tv-border));
  background: color-mix(in srgb, #fee2e2 16%, var(--tv-bg-surface));
}

.trading-scope-bar__group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.trading-scope-bar__group--primary {
  flex-shrink: 0;
}

.trading-scope-bar__chip,
.trading-scope-bar__status {
  display: inline-flex;
  align-items: center;
  height: 1.75rem;
  padding: 0 0.55rem;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font-size: 0.76rem;
  font-weight: 700;
  white-space: nowrap;
}

.trading-scope-bar__chip--real,
.trading-scope-bar__status--real {
  border-color: color-mix(in srgb, #dc2626 58%, var(--tv-border));
  background: color-mix(in srgb, #dc2626 13%, var(--tv-bg-surface));
  color: color-mix(in srgb, #dc2626 86%, var(--tv-text));
}

.trading-scope-bar__items {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, max-content));
  align-items: center;
  gap: 0.35rem 0.8rem;
  min-width: 0;
  margin: 0;
}

.trading-scope-bar__item {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  align-items: baseline;
  gap: 0.35rem;
  min-width: 0;
}

.trading-scope-bar__item dt {
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 700;
}

.trading-scope-bar__item dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: var(--tv-text);
  font-size: 0.78rem;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 960px) {
  .trading-scope-bar {
    align-items: flex-start;
    flex-direction: column;
  }

  .trading-scope-bar__items {
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
