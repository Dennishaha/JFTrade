<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import type { ExecutionOrdersResponse } from "@/contracts";

import AccountAssetStrip from "../components/domain/account/AccountAssetStrip.vue";
import AccountMoreSection from "../components/domain/account/AccountMoreSection.vue";
import AccountSummarySidebar from "../components/domain/account/AccountSummarySidebar.vue";
import ActiveOrdersTable, {
  type AccountExecutionOrder,
} from "../components/domain/account/ActiveOrdersTable.vue";
import OrderHistoryPanel from "../components/domain/account/OrderHistoryPanel.vue";
import PositionsTable, {
  type AccountPositionRow,
} from "../components/domain/account/PositionsTable.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import { fetchEnvelope, fetchEnvelopeWithInit } from "../composables/apiClient";
import { isFinalExecutionOrderStatus } from "../composables/consoleDataFormatting";
import { formatInstrumentIdentityText } from "../composables/instrumentPresentation";
import { useConsoleData } from "../composables/useConsoleData";
import { useNotifications } from "../composables/useNotifications";

type AccountTab = "positions" | "orders" | "history" | "funds";

const ACCOUNT_TABS: ReadonlyArray<{ value: AccountTab; label: string }> = [
  { value: "positions", label: "持仓" },
  { value: "orders", label: "订单" },
  { value: "history", label: "历史" },
  { value: "funds", label: "资金" },
];

const {
  activeExecutionOrders,
  historicalExecutionOrders,
  brokerFunds,
  brokerPositions,
  brokerRuntime,
  executionOrderEvents,
  historicalOrdersError,
  isLoadingHistoricalOrders,
  loadExecutionOrderDetails,
  loadHistoricalExecutionOrders,
  portfolioPositions,
  portfolioReconciliation,
  selectedBrokerAccount,
  selectedExecutionOrderId,
  supportsBrokerReadFeature,
  systemStatus,
} = useConsoleData();
const notifications = useNotifications();
const route = useRoute();
const router = useRouter();

const requestedExecutionOrderId = initialExecutionOrderIdFromLocation();
const activeTab = ref<AccountTab>(initialAccountTabFromLocation(requestedExecutionOrderId));
const cancellingOrderIds = ref<Set<string>>(new Set());
const historicalOrdersDisplayLimit = ref(50);
const hasLoadedHistoricalOrders = ref(false);

const pendingCancelOrder = ref<AccountExecutionOrder | null>(null);
const pendingCancelMessage = computed(() => {
  const order = pendingCancelOrder.value;
  if (order == null) return "";
  const instrument = order.symbol
    ? formatInstrumentIdentityText({
        market: order.market,
        instrumentId: order.symbol,
      })
    : order.internalOrderId;
  const kind = order.orderKind === "option_combo" || order.orderKind === "event_parlay"
    ? "组合订单"
    : "订单";
  return `确认撤销${kind} ${instrument}？撤单请求提交后仍以券商最终处理结果为准。`;
});

interface ExecutionOrderCommandResult {
  accepted: boolean;
  operation: string;
  internalOrderId?: string | null;
  brokerOrderId?: string | null;
  brokerOrderIdEx?: string | null;
  orderStatus?: string | null;
  brokerErrorCode?: string | null;
  message: string;
  checkedAt: string;
}

function initialExecutionOrderIdFromLocation(): string {
  if (typeof window === "undefined") {
    return "";
  }
  return new URLSearchParams(window.location.search).get("orderId")?.trim() ?? "";
}

function normalizeAccountTab(raw: string | null | undefined): AccountTab | null {
  switch (raw) {
    case "positions":
    case "orders":
    case "history":
    case "funds":
      return raw;
    // 兼容旧版 URL：/account?tab=account|pending
    case "account":
      return "positions";
    case "pending":
      return "orders";
    default:
      return null;
  }
}

function initialAccountTabFromLocation(orderId: string): AccountTab {
  if (typeof window === "undefined") {
    return "positions";
  }
  const requestedTab = normalizeAccountTab(
    new URLSearchParams(window.location.search).get("tab")?.trim(),
  );
  if (requestedTab != null) {
    return requestedTab;
  }
  return orderId === "" ? "positions" : "history";
}

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

const scopedActiveExecutionOrders = computed(() => {
  const selected = selectedBrokerAccount.value;
  const scoped = activeExecutionOrders.value.orders.filter((order) => {
    if (selected == null) {
      return orderMatchesTradingEnvironment(order.tradingEnvironment);
    }
    return (
      order.brokerId === selected.brokerId &&
      order.accountId === selected.accountId &&
      order.tradingEnvironment === selected.tradingEnvironment &&
      order.market === selected.market
    );
  });

  return dedupeExecutionOrders(scoped);
});

const scopedHistoricalExecutionOrders = computed(() => {
  const selected = selectedBrokerAccount.value;
  const scoped = historicalExecutionOrders.value.orders.filter((order) => {
    if (selected == null) {
      return orderMatchesTradingEnvironment(order.tradingEnvironment);
    }
    return (
      order.brokerId === selected.brokerId &&
      order.accountId === selected.accountId &&
      order.tradingEnvironment === selected.tradingEnvironment &&
      order.market === selected.market
    );
  });

  return dedupeExecutionOrders(scoped);
});

const pendingOrders = computed(() =>
  scopedActiveExecutionOrders.value.filter(
    (order) => !isFinalExecutionOrderStatus(order.status),
  ),
);

const historicalOrders = computed(() =>
  scopedHistoricalExecutionOrders.value.filter((order) =>
    isFinalExecutionOrderStatus(order.status),
  ),
);

const displayedHistoricalOrders = computed(() =>
  historicalOrders.value.slice(0, historicalOrdersDisplayLimit.value),
);

const hasMoreHistoricalOrders = computed(
  () => historicalOrdersDisplayLimit.value < historicalOrders.value.length,
);

const accountProjectedPositions = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return portfolioPositions.value.positions.filter((position) =>
      orderMatchesTradingEnvironment(position.tradingEnvironment),
    );
  }

  return portfolioPositions.value.positions.filter(
    (position) =>
      position.brokerId === selected.brokerId &&
      position.accountId === selected.accountId &&
      position.tradingEnvironment === selected.tradingEnvironment &&
      position.market === selected.market,
  );
});

const accountBrokerPositions = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return brokerPositions.value.positions.filter((position) =>
      orderMatchesTradingEnvironment(position.tradingEnvironment),
    );
  }

  return brokerPositions.value.positions.filter(
    (position) =>
      position.accountId === selected.accountId &&
      position.tradingEnvironment === selected.tradingEnvironment &&
      position.market === selected.market,
  );
});

const accountPositions = computed<AccountPositionRow[]>(() => {
  if (accountBrokerPositions.value.length > 0) {
    return accountBrokerPositions.value.map((position) => ({
      symbol: position.symbol,
      name: position.symbolName ?? null,
      market: position.market,
      quantity: position.quantity,
      averagePrice: position.averageCostPrice ?? position.costPrice ?? null,
      lastPrice: position.lastPrice ?? null,
      marketValue: position.marketValue,
      unrealizedPnl: position.unrealizedPnl ?? null,
      pnlRatio: position.pnlRatio ?? null,
      currency: position.currency,
      productClass: null,
      strategyType: null,
      positionType: null,
      payoutIfWin: null,
      source: "券商",
      updatedAt: brokerPositions.value.checkedAt,
    }));
  }

  return accountProjectedPositions.value.map((position) => ({
    symbol: position.symbol,
    name: null,
    market: position.market,
    quantity: position.quantity,
    averagePrice: position.averagePrice,
    lastPrice: null,
    marketValue: position.marketValue,
    unrealizedPnl: null,
    pnlRatio: null,
    currency: null,
    productClass: "equity",
    strategyType: null,
    positionType: null,
    payoutIfWin: null,
    source: "投影",
    updatedAt: position.updatedAt,
  }));
});

const accountReconciliation = computed(() => {
  const selected = selectedBrokerAccount.value;
  const entries = portfolioReconciliation.value.positions;
  if (selected == null) {
    return entries.filter((entry) =>
      orderMatchesTradingEnvironment(entry.tradingEnvironment),
    );
  }

  return entries.filter(
    (entry) =>
      entry.accountId === selected.accountId &&
      entry.tradingEnvironment === selected.tradingEnvironment &&
      entry.market === selected.market,
  );
});

const activeTradingEnvironment = computed(
  () =>
    selectedBrokerAccount.value?.tradingEnvironment ??
    selectedRuntimeAccount.value?.tradingEnvironment ??
    brokerFunds.value.summary?.tradingEnvironment ??
    systemStatus.value.defaultTradingEnvironment ??
    null,
);

function orderMatchesTradingEnvironment(tradingEnvironment: string): boolean {
  const activeEnvironment = activeTradingEnvironment.value;
  if (activeEnvironment == null || activeEnvironment.trim() === "") {
    return false;
  }
  return (
    tradingEnvironment.trim().toUpperCase() ===
    activeEnvironment.trim().toUpperCase()
  );
}

const activeBrokerReadContext = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected != null) {
    return {
      brokerId: selected.brokerId,
      accountId: selected.accountId,
      tradingEnvironment: selected.tradingEnvironment,
      market: selected.market,
    };
  }

  const summary = brokerFunds.value.summary;
  if (summary != null) {
    return {
      brokerId: brokerRuntime.value.descriptor.id,
      accountId: summary.accountId,
      tradingEnvironment: summary.tradingEnvironment,
      market: summary.market,
    };
  }

  const runtimeAccount = selectedRuntimeAccount.value;
  if (runtimeAccount == null) {
    return null;
  }

  return {
    brokerId: brokerRuntime.value.descriptor.id,
    accountId: runtimeAccount.accountId,
    tradingEnvironment: runtimeAccount.tradingEnvironment,
    market:
      runtimeAccount.marketAuthorities[0] ??
      brokerRuntime.value.descriptor.capabilities[0]?.market ??
      "HK",
  };
});

const marginRatioSymbols = computed(() =>
  Array.from(
    new Set(
      accountPositions.value
        .map((position) => position.symbol?.trim())
        .filter((symbol): symbol is string => symbol != null && symbol !== ""),
    ),
  ).slice(0, 24),
);

const supportsBrokerCashFlows = computed(() =>
  supportsBrokerReadFeature("cashFlows", {
    market: activeBrokerReadContext.value?.market ?? null,
    tradingEnvironment:
      activeBrokerReadContext.value?.tradingEnvironment ?? activeTradingEnvironment.value,
  }),
);

const supportsBrokerMarginRatios = computed(() =>
  supportsBrokerReadFeature("marginRatios", {
    market: activeBrokerReadContext.value?.market ?? null,
    tradingEnvironment:
      activeBrokerReadContext.value?.tradingEnvironment ?? activeTradingEnvironment.value,
  }),
);

function executionOrderDisplayKey(order: AccountExecutionOrder): string {
  const brokerOrderIdentity =
    order.brokerOrderId?.trim() ||
    order.brokerOrderIdEx?.trim() ||
    order.internalOrderId;
  return [
    order.brokerId,
    order.accountId,
    order.tradingEnvironment,
    order.market,
    brokerOrderIdentity,
  ].join("|");
}

function dedupeExecutionOrders(
  orders: AccountExecutionOrder[],
): AccountExecutionOrder[] {
  const seen = new Set<string>();
  const deduped: AccountExecutionOrder[] = [];
  for (const order of orders) {
    const key = executionOrderDisplayKey(order);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    deduped.push(order);
  }
  return deduped;
}

function selectOrder(internalOrderId: string): void {
  void loadExecutionOrderDetails(internalOrderId);
}

function openOrderEvents(order: AccountExecutionOrder): void {
  setActiveTab("history");
  void loadExecutionOrderDetails(order.internalOrderId);
  ensureHistoricalOrdersLoaded();
}

function isCancellingOrder(internalOrderId: string): boolean {
  return cancellingOrderIds.value.has(internalOrderId);
}

function canCancelOrder(order: AccountExecutionOrder): boolean {
  if (isFinalExecutionOrderStatus(order.status)) {
    return false;
  }
  if (isCancellingOrder(order.internalOrderId)) {
    return false;
  }

  const normalized = order.status.trim().toUpperCase();
  if (
    normalized === "CANCELING" ||
    normalized === "PENDING_CANCEL" ||
    normalized === "CANCEL_REQUESTED"
  ) {
    return false;
  }

  return true;
}

function executionOrdersUrl(): string {
  const params = new URLSearchParams();
  const context = activeBrokerReadContext.value;
  if (context != null) {
    params.set("brokerId", context.brokerId);
    params.set("tradingEnvironment", context.tradingEnvironment);
    params.set("accountId", context.accountId);
    params.set("market", context.market);
  } else if (activeTradingEnvironment.value != null) {
    params.set("tradingEnvironment", activeTradingEnvironment.value);
  }
  const query = params.toString();
  return query === "" ? "/api/v1/execution/orders" : `/api/v1/execution/orders?${query}`;
}

async function refreshExecutionOrders(): Promise<void> {
  activeExecutionOrders.value = await fetchEnvelope<ExecutionOrdersResponse>(
    executionOrdersUrl(),
  );
}

function ensureHistoricalOrdersLoaded(): void {
  if (hasLoadedHistoricalOrders.value) return;
  hasLoadedHistoricalOrders.value = true;
  const context = activeBrokerReadContext.value;
  if (context == null) return;
  const params = new URLSearchParams();
  params.set("brokerId", context.brokerId);
  params.set("tradingEnvironment", context.tradingEnvironment);
  if (context.accountId) params.set("accountId", context.accountId);
  if (context.market) params.set("market", context.market);
  void loadHistoricalExecutionOrders({
    brokerId: context.brokerId,
    brokerQuery: params.toString(),
  });
}

function loadMoreHistoricalOrders(): void {
  historicalOrdersDisplayLimit.value += 50;
}

async function cancelOrder(order: AccountExecutionOrder): Promise<void> {
  if (!canCancelOrder(order)) {
    return;
  }

  const nextCancelling = new Set(cancellingOrderIds.value);
  nextCancelling.add(order.internalOrderId);
  cancellingOrderIds.value = nextCancelling;

  try {
    const cancelPath =
      order.orderKind === "option_combo" || order.orderKind === "event_parlay"
        ? `/api/v1/execution/combos/${encodeURIComponent(order.internalOrderId)}/cancel`
        : `/api/v1/execution/orders/${encodeURIComponent(order.internalOrderId)}/cancel`;
    const result = await fetchEnvelopeWithInit<ExecutionOrderCommandResult>(
      cancelPath,
      {
        method: "POST",
      },
    );

    await refreshExecutionOrders();
    await loadExecutionOrderDetails(order.internalOrderId);

    notifications.push({
      level: "success",
      title: `已提交撤单 ${
        order.symbol
          ? formatInstrumentIdentityText({
              market: order.market,
              instrumentId: order.symbol,
            })
          : order.internalOrderId
      }`,
      message: result.message,
      source: "account-page",
    });
  } catch (error) {
    const message = error instanceof Error && error.message.trim() !== ""
      ? error.message
      : "撤单请求失败。";
    notifications.push({
      level: "error",
      title: `撤单失败 ${
        order.symbol
          ? formatInstrumentIdentityText({
              market: order.market,
              instrumentId: order.symbol,
            })
          : order.internalOrderId
      }`,
      message,
      source: "account-page",
    });
  } finally {
    const nextCancellingDone = new Set(cancellingOrderIds.value);
    nextCancellingDone.delete(order.internalOrderId);
    cancellingOrderIds.value = nextCancellingDone;
  }
}

function requestCancelOrder(order: AccountExecutionOrder): void {
  if (!canCancelOrder(order)) return;
  pendingCancelOrder.value = order;
}

function confirmCancelOrder(): void {
  const order = pendingCancelOrder.value;
  pendingCancelOrder.value = null;
  if (order != null) void cancelOrder(order);
}

function setActiveTab(tab: AccountTab): void {
  if (activeTab.value === tab) return;
  activeTab.value = tab;
}

// Tab 与 URL 双向同步：入口带 ?tab= 跳转时切换视图。
// 旧版 /account?tab=risk 链接重定向到独立风控页 /risk。
// 测试环境可能未安装路由，route/router 需要可缺省。
watch(
  () => route?.query.tab,
  (raw) => {
    if (raw === "risk") {
      void router?.replace("/risk");
      return;
    }
    const normalized = normalizeAccountTab(
      typeof raw === "string" ? raw : null,
    );
    if (normalized != null && normalized !== activeTab.value) {
      activeTab.value = normalized;
    }
  },
  { immediate: true },
);

watch(activeTab, (tab) => {
  if (tab === "history") {
    ensureHistoricalOrdersLoaded();
  }
  if (route == null || router == null) return;
  const currentQueryTab = normalizeAccountTab(
    typeof route.query.tab === "string" ? route.query.tab : null,
  );
  if (currentQueryTab !== tab) {
    void router.replace({
      path: route.path,
      query: { ...route.query, tab },
    });
  }
});

watch(
  [pendingOrders, historicalOrders],
  ([nextPendingOrders, nextHistoricalOrders]) => {
    const visibleOrders = [...nextPendingOrders, ...nextHistoricalOrders];
    const selectedStillVisible = visibleOrders.some(
      (order) => order.internalOrderId === selectedExecutionOrderId.value,
    );
    const requestedStillVisible =
      requestedExecutionOrderId !== "" &&
      visibleOrders.some((order) => order.internalOrderId === requestedExecutionOrderId);
    const nextOrderId =
      requestedStillVisible
        ? requestedExecutionOrderId
        : selectedStillVisible ? selectedExecutionOrderId.value : visibleOrders[0]?.internalOrderId;

    if (
      nextOrderId == null ||
      nextOrderId === "" ||
      executionOrderEvents.value.internalOrderId === nextOrderId
    ) {
      return;
    }

    void loadExecutionOrderDetails(nextOrderId);
  },
  { immediate: true },
);

if (requestedExecutionOrderId !== "") {
  ensureHistoricalOrdersLoaded();
  void loadExecutionOrderDetails(requestedExecutionOrderId);
}
</script>

<template>
  <div class="account-page">
    <AccountSummarySidebar />

    <section class="account-page__main">
      <AccountAssetStrip />

      <div class="account-page__tabs-row">
        <div class="account-page__tabs" role="tablist" aria-label="账户视图">
          <button
            v-for="tab in ACCOUNT_TABS"
            :key="tab.value"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.value"
            :class="{ 'is-active': activeTab === tab.value }"
            @click="setActiveTab(tab.value)"
          >
            {{ tab.label }}
            <span v-if="tab.value === 'orders' && pendingOrders.length">
              {{ pendingOrders.length }}
            </span>
          </button>
        </div>
      </div>

      <div class="account-page__content">
        <PositionsTable
          v-if="activeTab === 'positions'"
          :positions="accountPositions"
          :reconciliation="accountReconciliation"
        />
        <ActiveOrdersTable
          v-else-if="activeTab === 'orders'"
          :orders="pendingOrders"
          :selected-order-id="selectedExecutionOrderId"
          :can-cancel="canCancelOrder"
          :is-cancelling="isCancellingOrder"
          @cancel="requestCancelOrder"
          @view-events="openOrderEvents"
        />
        <OrderHistoryPanel
          v-else-if="activeTab === 'history'"
          :orders="displayedHistoricalOrders"
          :total-count="historicalOrders.length"
          :is-loading="isLoadingHistoricalOrders"
          :error="historicalOrdersError"
          :has-more="hasMoreHistoricalOrders"
          :selected-order-id="selectedExecutionOrderId"
          @select="selectOrder"
          @load-more="loadMoreHistoricalOrders"
        />
        <AccountMoreSection
          v-else-if="activeTab === 'funds'"
          :margin-ratio-symbols="marginRatioSymbols"
          :supports-cash-flows="supportsBrokerCashFlows"
          :supports-margin-ratios="supportsBrokerMarginRatios"
          :matches-trading-environment="orderMatchesTradingEnvironment"
        />
      </div>

      <ActionConfirmDialog
        :open="pendingCancelOrder != null"
        title="确认撤单"
        :message="pendingCancelMessage"
        confirm-label="确认撤单"
        :busy="pendingCancelOrder != null && isCancellingOrder(pendingCancelOrder.internalOrderId)"
        @close="pendingCancelOrder = null"
        @confirm="confirmCancelOrder"
      />
    </section>
  </div>
</template>

<style scoped>
.account-page {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  gap: 12px;
  padding: 14px;
  overflow: hidden;
  background:
    radial-gradient(circle at 92% -20%, color-mix(in srgb, var(--tv-accent) 9%, transparent), transparent 36%),
    var(--tv-bg-app);
}

.account-page__main {
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

.account-page__tabs-row {
  display: flex;
  flex: 0 0 auto;
  align-items: stretch;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.account-page__tabs {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 2px;
  padding: 5px 7px 0;
  overflow-x: auto;
  scrollbar-width: thin;
}

.account-page__tabs button {
  position: relative;
  flex: 0 0 auto;
  padding: 8px 14px 9px;
  border: 0;
  border-radius: 6px 6px 0 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: 11px;
}

.account-page__tabs button span {
  margin-left: 4px;
  color: var(--tv-text-dim);
  font-size: 9px;
}

.account-page__tabs button.is-active {
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 650;
}

.account-page__tabs button.is-active::after {
  position: absolute;
  right: 8px;
  bottom: -1px;
  left: 8px;
  height: 2px;
  background: var(--tv-accent);
  content: "";
}

.account-page__content {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
}

@media (max-width: 1180px) {
  .account-page {
    flex-direction: column;
    overflow: auto;
  }

  .account-page__main {
    flex: 1 0 auto;
    min-height: 480px;
  }
}
</style>
