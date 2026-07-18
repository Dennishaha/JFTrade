<script setup lang="ts">
import { computed } from "vue";

import {
  type OptionComboDockTab,
  useOptionComboDraftStore,
} from "../../composables/optionComboDraft";
import { useConsoleData } from "../../composables/useConsoleData";
import PositionsPanel from "../workspace/PositionsPanel.vue";
import OptionComboBuilder from "./OptionComboBuilder.vue";

const draft = useOptionComboDraftStore();
const {
  activeExecutionOrders,
  historicalExecutionOrders,
  portfolioPositions,
  selectedBrokerAccount,
  systemStatus,
} = useConsoleData();
const collapsed = defineModel<boolean>("collapsed", { default: false });

const environment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment,
);
const accountLabel = computed(
  () =>
    selectedBrokerAccount.value?.displayName ||
    selectedBrokerAccount.value?.accountId ||
    "自动账户",
);
const activeComboOrders = computed(
  () =>
    activeExecutionOrders.value.orders.filter(
      (order) => order.orderKind === "option_combo",
    ).length,
);
const historicalComboOrders = computed(
  () =>
    historicalExecutionOrders.value.orders.filter(
      (order) => order.orderKind === "option_combo",
    ).length,
);
const tabs: Array<{ value: OptionComboDockTab; label: string }> = [
  { value: "trade", label: "期权交易" },
  { value: "positions", label: "持仓" },
  { value: "orders", label: "订单" },
  { value: "history", label: "历史" },
];

function count(tab: OptionComboDockTab): number | null {
  if (tab === "trade") return draft.legs.value.length;
  if (tab === "positions") return portfolioPositions.value.positions.length;
  if (tab === "orders") return activeComboOrders.value;
  return historicalComboOrders.value;
}

function selectTab(tab: OptionComboDockTab): void {
  draft.setDockTab(tab);
  collapsed.value = false;
}

function handleSubmitted(internalOrderId: string): void {
  draft.submittedOrderId.value = internalOrderId;
  draft.setDockTab("orders");
}
</script>

<template>
  <section
    class="option-trading-dock"
    :class="{ 'is-collapsed': collapsed }"
    data-capability-surface="workspace.option-combo-trading"
  >
    <header class="option-trading-dock__header">
      <div class="option-trading-dock__account">
        <span aria-hidden="true">▰</span>
        <strong>{{ accountLabel }}</strong>
        <small :class="{ 'is-real': environment === 'REAL' }">
          {{ environment }}
        </small>
      </div>
      <nav aria-label="期权交易区域">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          type="button"
          :class="{ 'is-active': draft.activeDockTab.value === tab.value }"
          @click="selectTab(tab.value)"
        >
          {{ tab.label }}
          <span v-if="count(tab.value)">{{ count(tab.value) }}</span>
        </button>
      </nav>
      <button
        type="button"
        class="option-trading-dock__collapse"
        :aria-label="collapsed ? '展开期权交易区' : '折叠期权交易区'"
        @click="collapsed = !collapsed"
      >
        {{ collapsed ? "⌃" : "⌄" }}
      </button>
    </header>

    <div v-show="!collapsed" class="option-trading-dock__body">
      <OptionComboBuilder
        v-if="draft.activeDockTab.value === 'trade'"
        :instrument-id="draft.underlyingInstrumentId.value"
        :market="draft.market.value"
        :contracts="draft.contracts.value"
        @submitted="handleSubmitted"
      />
      <PositionsPanel
        v-else-if="draft.activeDockTab.value === 'positions'"
        view="positions"
        hide-header
      />
      <PositionsPanel
        v-else-if="draft.activeDockTab.value === 'orders'"
        view="active"
        hide-header
        order-kind-filter="option_combo"
        :focus-order-id="draft.submittedOrderId.value"
      />
      <PositionsPanel
        v-else
        view="historical"
        hide-header
        order-kind-filter="option_combo"
        :focus-order-id="draft.submittedOrderId.value"
      />
    </div>
  </section>
</template>

<style scoped>
.option-trading-dock {
  container-name: option-trading-dock;
  container-type: inline-size;
  display: grid;
  width: 100%;
  height: 100%;
  grid-template-rows: 36px minmax(0, 1fr);
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--tv-bg-surface);
}

.option-trading-dock.is-collapsed {
  grid-template-rows: 36px 0;
}

.option-trading-dock__header {
  display: flex;
  min-width: 0;
  align-items: stretch;
  border-bottom: 1px solid var(--tv-border);
}

.option-trading-dock__account {
  display: flex;
  min-width: 160px;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border-right: 1px solid var(--tv-border);
}

.option-trading-dock__account > span {
  color: var(--tv-accent);
}

.option-trading-dock__account strong {
  overflow: hidden;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.option-trading-dock__account small {
  color: var(--tv-text-dim);
  font-size: 8px;
}

.option-trading-dock__account small.is-real {
  color: var(--tv-status-warning-fg);
}

.option-trading-dock nav {
  display: flex;
  min-width: 0;
  flex: 1;
}

.option-trading-dock nav button,
.option-trading-dock__collapse {
  position: relative;
  min-width: 68px;
  padding: 0 12px;
  border: 0;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: 10px;
}

.option-trading-dock nav button::after {
  position: absolute;
  right: 10px;
  bottom: 0;
  left: 10px;
  height: 2px;
  background: transparent;
  content: "";
}

.option-trading-dock nav button.is-active {
  color: var(--tv-text);
  font-weight: 700;
}

.option-trading-dock nav button.is-active::after {
  background: var(--tv-accent);
}

.option-trading-dock nav span {
  display: inline-grid;
  min-width: 14px;
  height: 14px;
  margin-left: 3px;
  place-items: center;
  border-radius: 7px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 8px;
}

.option-trading-dock__collapse {
  min-width: 36px;
  padding: 0;
}

.option-trading-dock__body {
  display: grid;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.option-trading-dock__body > * {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
}

@container option-trading-dock (max-width: 620px) {
  .option-trading-dock__account {
    display: none;
  }

  .option-trading-dock nav button {
    min-width: 0;
    flex: 1;
    padding: 0 5px;
  }
}
</style>
