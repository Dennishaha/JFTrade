<script setup lang="ts">
import { provide } from "vue";

import BacktestRunReport from "../components/backtest/BacktestRunReport.vue";
import BacktestSidebar from "../components/backtest/BacktestSidebar.vue";
import BacktestVersionComparison from "../components/backtest/BacktestVersionComparison.vue";
import BacktestWorkbenchHeader from "../components/backtest/BacktestWorkbenchHeader.vue";
import ActionConfirmDialog from "../components/shared/ActionConfirmDialog.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import {
  backtestPageContextKey,
  useBacktestPage,
} from "../composables/useBacktestPage";

const backtestPage = useBacktestPage();
provide(backtestPageContextKey, backtestPage);

const {
  BACKTEST_FORM_STORAGE_KEY,
  BACKTEST_RESULTS_PAGE_SIZE,
  BACKTEST_RESULT_STATUS_OPTIONS,
  BACKTEST_BROKER_FEE_MODE_OPTIONS,
  BACKTEST_MARKET_FEE_MODE_OPTIONS,
  emptyStateClass,
  route,
  router,
  storedBacktestFormPreferences,
  definitions,
  strategyDefinitionsReady,
  warmupPreviewBars,
  warmupPreviewPending,
  warmupPreviewInterval,
  resultsPage,
  resultsSearchQuery,
  resultsStatusFilter,
  resultsStrategyFilter,
  pendingDeleteRunId,
  deletingRunId,
  reportMode,
  comparisonDefinitionId,
  leftComparisonVersion,
  rightComparisonVersion,
  leftComparisonRunId,
  rightComparisonRunId,
  comparisonVersions,
  isLoadingComparisonVersions,
  comparisonVersionsError,
  leftComparisonSnapshot,
  rightComparisonSnapshot,
  comparisonSnapshotErrors,
  comparisonSnapshotLoading,
  selectedDefinitionId,
  selectedMarket,
  codeInput,
  instrumentSearchQuery,
  interval,
  chartType,
  startDate,
  endDate,
  initialBalance,
  instrumentType,
  rehabType,
  useExtendedHours,
  brokerFeeMode,
  marketFeeMode,
  brokerFeeRulesText,
  marketFeeRulesText,
  EXTENDED_HOURS_INTERVALS,
  selectedDefinition,
  displayInstrumentId,
  instrumentSelectionResolved,
  periodLabel,
  extendedHoursSupported,
  extendedHoursHint,
  quoteCurrency,
  warmupPreviewValue,
  warmupPreviewNote,
  warmupPreviewSymbol,
  brokerFeeRules,
  marketFeeRules,
  costModeSummary,
  backtestFormState,
  BACKTEST_MEDIUM_WORKBENCH_QUERY,
  activeReportTab,
  selectedRunId,
  showNewBacktestForm,
  newBacktestFormTouched,
  backtestPaneSizes,
  backtestMobileSection,
  backtestSidebarOpen,
  isMediumBacktestWorkbench,
  errorExpanded,
  expandedBacktestPanels,
  resultStrategyOptions,
  hasResultsFilters,
  filteredRuns,
  emptyResultsMessage,
  resultsPageCount,
  pagedRuns,
  resultsPageSummary,
  comparisonDefinitionOptions,
  leftComparisonVersionOptions,
  rightComparisonVersionOptions,
  leftComparisonVersionSelectOptions,
  rightComparisonVersionSelectOptions,
  leftComparisonRuns,
  rightComparisonRuns,
  leftComparisonRunOptions,
  rightComparisonRunOptions,
  leftComparisonRun,
  rightComparisonRun,
  comparisonRunsReady,
  comparisonSourcesReady,
  comparisonMetrics,
  comparisonConfigRows,
  comparisonConditionsMatch,
  pendingDeleteRun,
  pendingDeleteMessage,
  focusedRun,
  focusedRunResultReady,
  focusedRunHasChartData,
  statusChip,
  firstQueryValue,
  reportModeFromQuery,
  readStoredBacktestFormPreferences,
  canonicalBacktestInstrumentInput,
  handleResolvedBacktestInstrument,
  supportsExtendedHoursForInterval,
  parseBacktestFeeRules,
  quoteCurrencyFromInstrumentId,
  resolveRunQuoteCurrency,
  resolveRunSessionMode,
  resolveStrategyName,
  resolveStrategyDefinition,
  formatStrategyVersion,
  resolveBacktestStrategyVersionNotice,
  comparisonRunTimestamp,
  completedRunsForComparisonVersion,
  versionOptionTitle,
  comparisonRunOptionTitle,
  clearComparisonSnapshots,
  clearComparisonSelection,
  comparisonVersionExists,
  applyComparisonVersionDefaults,
  loadComparisonVersions,
  loadComparisonSnapshot,
  nativeSelectValue,
  changeComparisonDefinition,
  changeComparisonVersion,
  changeComparisonRun,
  activateComparisonMode,
  activateSingleReportMode,
  comparisonQueryMatchesRoute,
  syncComparisonRoute,
  formatComparisonCurrency,
  formatComparisonMetric,
  comparisonMetricDelta,
  compareConfigValue,
  comparisonFeeConfig,
  comparisonChartType,
  requestDeleteRun,
  confirmDeleteRun,
  selectFocusedRun,
  setBacktestSetupPanelOpen,
  handleBacktestPanelsUpdate,
  toggleBacktestPanel,
  openNewBacktestForm,
  toggleNewBacktestForm,
  toggleBacktestSidebar,
  closeBacktestSidebar,
  syncMediumBacktestWorkbench,
  handleBacktestWorkbenchKeydown,
  installBacktestWorkbenchMediaQuery,
  disposeBacktestWorkbenchMediaQuery,
  selectBacktestMobileSection,
  handleBacktestPaneResized,
  resetResultsFilters,
  ensureComparisonRunDefaults,
  applyComparisonRouteState,
  ensureSelectedMarketProfile,
  loadDefinitions,
  loadWarmupPreview,
  defaultMarket,
  loadMarketProfiles,
  findMarketProfile,
  quoteCurrencyForMarket,
  supportsExtendedHoursForMarket,
  normalizeInstrumentRefWithMarketApi,
  runs,
  running,
  syncing,
  syncProgress,
  error,
  detailLoading,
  detailErrors,
  sortedRuns,
  toggleRun,
  deleteRun,
  loadRuns,
  syncKlines,
  cancelSync,
  startBacktest,
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
  formatBacktestRehabType,
  formatBacktestRunDate,
  formatBacktestTimestamp,
  isTerminalBacktestStatus,
} = backtestPage;
</script>

<template>
  <div
    class="backtest-page"
    :class="[
      `backtest-page--mobile-${backtestMobileSection}`,
      backtestSidebarOpen ? 'backtest-page--sidebar-open' : 'backtest-page--sidebar-closed',
      { 'backtest-page--medium': isMediumBacktestWorkbench },
    ]"
  >
    <BacktestWorkbenchHeader />

    <button
      v-if="isMediumBacktestWorkbench && backtestSidebarOpen"
      type="button"
      class="backtest-sidebar-backdrop"
      aria-label="关闭回测配置与历史"
      data-testid="backtest-sidebar-backdrop"
      @click="closeBacktestSidebar"
    />

    <SplitPane
      class="backtest-page__split"
      :pane-min-size="18"
      @resized="handleBacktestPaneResized"
    >
      <SplitPaneItem :size="backtestPaneSizes[0]" :min-size="22" :max-size="55">
        <BacktestSidebar />
      </SplitPaneItem>

      <SplitPaneItem :size="backtestPaneSizes[1]" :min-size="45">
        <main class="backtest-page__pane">
          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <BacktestVersionComparison v-if="reportMode === 'compare'" />
            <div
              v-else-if="!focusedRun"
              :class="[emptyStateClass, 'p-6 text-center text-sm']"
            >
              {{ emptyResultsMessage }}
            </div>
            <BacktestRunReport
              v-else
              v-model:active-tab="activeReportTab"
              :run="focusedRun"
              :result-ready="focusedRunResultReady"
              :has-chart-data="focusedRunHasChartData"
              :strategy-name="resolveStrategyName(focusedRun.request.definitionId)"
              :strategy-version-label="formatStrategyVersion(focusedRun.request.definitionVersion)"
              :status-label="statusChip(focusedRun.status).label"
              :quote-currency="resolveRunQuoteCurrency(focusedRun)"
              :session-mode="resolveRunSessionMode(focusedRun)"
              :rehab-label="formatBacktestRehabType(focusedRun.request.rehabType)"
              :version-notice="resolveBacktestStrategyVersionNotice(focusedRun)"
              :detail-loading="detailLoading[focusedRun.id] === true"
              :detail-error="detailErrors[focusedRun.id] ?? ''"
            />
          </div>
        </main>
      </SplitPaneItem>
    </SplitPane>

    <ActionConfirmDialog
      :open="pendingDeleteRun != null"
      title="删除回测记录"
      :message="pendingDeleteMessage"
      confirm-label="确认删除"
      :busy="deletingRunId !== ''"
      @close="pendingDeleteRunId = ''"
      @confirm="confirmDeleteRun"
    />
  </div>
</template>

<style scoped>
.backtest-page {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: 0;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.backtest-page__split {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.backtest-page__split :deep(.splitpanes__pane) {
  min-width: 0;
  overflow: hidden;
}

.backtest-page__pane {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.backtest-page__pane > * {
  min-width: 0;
}

.backtest-sidebar-backdrop {
  position: absolute;
  z-index: 20;
  inset: 44px 0 0;
  border: 0;
  border-radius: 0;
  background: rgba(2, 6, 23, 0.38);
  padding: 0;
}

@media (min-width: 1181px) {
  .backtest-page--sidebar-closed .backtest-page__split > :deep(.splitpanes__pane:first-of-type),
  .backtest-page--sidebar-closed .backtest-page__split > :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }

  .backtest-page--sidebar-closed .backtest-page__split > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }
}

@media (min-width: 769px) and (max-width: 1180px) {
  .backtest-page__split > :deep(.splitpanes__pane:first-of-type) {
    position: absolute !important;
    z-index: 30;
    inset: 0 auto 0 0;
    width: min(380px, calc(100% - 48px)) !important;
    max-width: min(380px, calc(100% - 48px)) !important;
    min-width: min(300px, calc(100% - 48px)) !important;
    flex: 0 0 min(380px, calc(100% - 48px)) !important;
    transform: translateX(0);
    transition: transform 160ms ease;
    box-shadow: 16px 0 36px rgba(2, 6, 23, 0.3);
  }

  .backtest-page__split > :deep(.splitpanes__splitter) {
    display: none;
  }

  .backtest-page__split > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .backtest-page--sidebar-closed .backtest-page__split > :deep(.splitpanes__pane:first-of-type) {
    pointer-events: none;
    transform: translateX(-105%);
    box-shadow: none;
  }
}

@media (max-width: 768px) {
  .backtest-page__split.tv-splitpanes {
    display: block !important;
    width: 100%;
    height: 100%;
    min-height: 0;
    overflow: hidden;
  }

  .backtest-page__split :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .backtest-page__split > :deep(.splitpanes__pane) {
    width: 100% !important;
    max-width: 100% !important;
    min-width: 0 !important;
    height: 100% !important;
    max-height: 100% !important;
    min-height: 0 !important;
    flex: none !important;
    transform: none !important;
  }

  .backtest-page--mobile-setup .backtest-page__split > :deep(.splitpanes__pane:last-of-type),
  .backtest-page--mobile-report .backtest-page__split > :deep(.splitpanes__pane:first-of-type) {
    display: none !important;
  }

  .backtest-page :deep(.v-chip) {
    max-width: 100%;
  }

  .backtest-page :deep(.v-chip__content) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

.backtest-page :deep(.v-card) {
  background: var(--tv-bg-surface);
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-chip) {
  border-color: var(--tv-border);
}

.backtest-page :deep(.v-pagination .v-btn) {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  border: 1px solid var(--tv-border);
}

.backtest-page :deep(.v-pagination .v-btn.v-btn--active) {
  color: var(--tv-accent);
  border-color: var(--tv-accent);
}

.backtest-page :deep(.bt-accent-action.v-btn) {
  border: 1px solid color-mix(in srgb, var(--tv-accent) 34%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent);
  color: var(--tv-accent);
}

.backtest-page :deep(.bt-accent-action.v-btn:hover) {
  background: color-mix(in srgb, var(--tv-accent) 20%, transparent);
  border-color: color-mix(in srgb, var(--tv-accent) 54%, var(--tv-border));
}

.backtest-page :deep(.bt-bg-surface) {
  background: var(--tv-bg-surface);
}

.backtest-page :deep(.bt-bg-muted) {
  background: var(--tv-bg-surface-2);
}

.backtest-page :deep(.bt-border) {
  border-color: var(--tv-border);
}

.backtest-page :deep(.bt-border-soft) {
  border-color: color-mix(in srgb, var(--tv-border) 70%, transparent);
}

.backtest-page :deep(.bt-text) {
  color: var(--tv-text);
}

.backtest-page :deep(.bt-text-strong) {
  color: var(--tv-text);
}

.backtest-page :deep(.bt-text-muted) {
  color: var(--tv-text-muted);
}

.backtest-page :deep(.bt-text-dim) {
  color: var(--tv-text-dim);
}

.backtest-page :deep(.bt-divide > :not([hidden]) ~ :not([hidden])) {
  border-color: var(--tv-border);
}

.backtest-page :deep(.bt-divide-soft > :not([hidden]) ~ :not([hidden])) {
  border-color: color-mix(in srgb, var(--tv-border) 70%, transparent);
}
</style>
