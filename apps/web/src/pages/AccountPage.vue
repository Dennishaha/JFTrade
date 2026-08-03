<script setup lang="ts">
import AccountAssetStrip from "../components/domain/account/AccountAssetStrip.vue";
import AccountMoreSection from "../components/domain/account/AccountMoreSection.vue";
import AccountPageToolbar from "../components/domain/account/AccountPageToolbar.vue";
import AccountSummarySidebar from "../components/domain/account/AccountSummarySidebar.vue";
import ActiveOrdersTable from "../components/domain/account/ActiveOrdersTable.vue";
import OrderHistoryPanel from "../components/domain/account/OrderHistoryPanel.vue";
import PositionsTable from "../components/domain/account/PositionsTable.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import { useAccountPage } from "@/composables/trading/useAccountPage";

const {
  activeTab,
  isRefreshingAccount,
  pendingCancelOrder,
  accountDataError,
  pendingCancelMessage,
  pendingOrders,
  historicalOrders,
  displayedHistoricalOrders,
  hasMoreHistoricalOrders,
  accountPositions,
  marginRatioSymbols,
  supportsBrokerCashFlows,
  supportsBrokerMarginRatios,
  historicalOrdersError,
  isLoadingHistoricalOrders,
  selectedExecutionOrderId,
  orderMatchesTradingEnvironment,
  selectOrder,
  openOrderEvents,
  isCancellingOrder,
  canCancelOrder,
  refreshAccountData,
  loadMoreHistoricalOrders,
  requestCancelOrder,
  confirmCancelOrder,
  setActiveTab,
  requestedExecutionOrderId,
  cancellingOrderIds,
  historicalOrdersDisplayLimit,
  hasLoadedHistoricalOrders,
  selectedRuntimeAccount,
  scopedActiveExecutionOrders,
  scopedHistoricalExecutionOrders,
  accountProjectedPositions,
  accountBrokerPositions,
  activeTradingEnvironment,
  activeBrokerReadContext,
  executionOrdersUrl,
  refreshExecutionOrders,
  reloadHistoricalOrders,
  ensureHistoricalOrdersLoaded,
  cancelOrder,
} = useAccountPage();
</script>

<template>
  <div class="account-page">
    <AccountSummarySidebar />

    <section class="account-page__main">
      <AccountAssetStrip />

      <AccountPageToolbar
        :active-tab="activeTab"
        :pending-order-count="pendingOrders.length"
        :refreshing="isRefreshingAccount"
        :error="accountDataError"
        @select="setActiveTab"
        @refresh="refreshAccountData()"
      />

      <div class="account-page__content">
        <PositionsTable v-if="activeTab === 'positions'" :positions="accountPositions" />
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
  box-shadow: 0 8px 24px color-mix(in srgb, var(--jf-shadow-color) 8%, transparent);
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
