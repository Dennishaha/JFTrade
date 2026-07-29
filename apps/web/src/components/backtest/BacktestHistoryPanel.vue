<script setup lang="ts">
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import { useBacktestPageContext } from "../../composables/useBacktestPage";

const {
  BACKTEST_RESULT_STATUS_OPTIONS,
  emptyResultsMessage,
  emptyStateClass,
  expandedBacktestPanels,
  filteredRuns,
  focusedRun,
  formatBacktestRehabType,
  formatBacktestRunDate,
  formatStrategyVersion,
  hasResultsFilters,
  isTerminalBacktestStatus,
  pagedRuns,
  requestDeleteRun,
  resetResultsFilters,
  resolveRunSessionMode,
  resolveStrategyName,
  resultStrategyOptions,
  resultsPage,
  resultsPageCount,
  resultsPageSummary,
  resultsSearchQuery,
  resultsStatusFilter,
  resultsStrategyFilter,
  selectFocusedRun,
  statusChip,
  toggleBacktestPanel,
} = useBacktestPageContext();
</script>

<template>
<section class="bt-sidebar-panel bt-sidebar-panel--history"
                :class="{ 'is-expanded': expandedBacktestPanels.includes('history') }">
                <button type="button" class="bt-sidebar-panel__title" data-testid="backtest-side-panel-history-title"
                  :aria-expanded="expandedBacktestPanels.includes('history')" @click="toggleBacktestPanel('history')">
                  <v-icon size="11">fa-solid fa-chevron-right</v-icon>
                  <span>历史回测</span>
                  <em>{{ resultsPageSummary || `${filteredRuns.length} 条` }}</em>
                </button>
                <div v-if="expandedBacktestPanels.includes('history')"
                  class="bt-sidebar-panel__body bt-sidebar-panel__body--history">
                  <div class="bt-backtest-results-filters">
                    <input v-model="resultsSearchQuery" type="search" class="bt-native-input"
                      placeholder="搜索策略、标的、回测 ID" aria-label="搜索回测记录" />
                    <div class="grid grid-cols-2 gap-2">
                      <select v-model="resultsStatusFilter" class="bt-native-select" aria-label="按状态筛选">
                        <option v-for="option in BACKTEST_RESULT_STATUS_OPTIONS" :key="option.value"
                          :value="option.value">
                          {{ option.title }}
                        </option>
                      </select>
                      <select v-model="resultsStrategyFilter" class="bt-native-select" aria-label="按策略筛选">
                        <option v-for="option in resultStrategyOptions" :key="option.value" :value="option.value">
                          {{ option.title }}
                        </option>
                      </select>
                    </div>
                    <button type="button" class="bt-filter-reset" :disabled="!hasResultsFilters"
                      @click="resetResultsFilters">
                      清空筛选
                    </button>
                  </div>

                  <div v-if="filteredRuns.length === 0" :class="[emptyStateClass, 'p-6 text-center text-sm']">
                    {{ emptyResultsMessage }}
                  </div>
                  <div v-else class="bt-history-list">
                    <div v-for="run in pagedRuns" :key="run.id" class="bt-history-run cursor-pointer transition" :class="focusedRun && focusedRun.id === run.id
                      ? 'bt-history-run--selected'
                      : 'bt-history-run--idle bt-border bt-bg-surface'" role="button" tabindex="0"
                      @click="selectFocusedRun(run.id)" @keydown.enter.prevent="selectFocusedRun(run.id)"
                      @keydown.space.prevent="selectFocusedRun(run.id)">
                      <div class="flex items-start gap-2">
                        <div class="min-w-0 flex-1">
                          <div class="bt-history-run__title">
                            <span class="truncate">
                              {{ resolveStrategyName(run.request.definitionId) }} ·
                              <InstrumentIdentity :instrument-id="run.request.symbol" compact />
                            </span>
                            <span class="bt-history-run__status" :class="`is-${run.status}`">
                              {{ statusChip(run.status).label }}
                            </span>
                          </div>
                          <div class="bt-history-run__meta">
                            <span>{{ run.request.interval }}</span>
                            <span>{{ formatBacktestRunDate(run.request.startDate) }} → {{
                              formatBacktestRunDate(run.request.endDate)
                              }}</span>
                            <span v-if="run.request.definitionVersion">{{
                              formatStrategyVersion(run.request.definitionVersion)
                              }}</span>
                          </div>
                          <div class="bt-history-run__id" :title="run.id">
                            {{ run.id }} · {{ resolveRunSessionMode(run) }} · {{
                              formatBacktestRehabType(run.request.rehabType) }}
                          </div>
                          <div v-if="run.status === 'running' || run.status === 'queued'"
                            class="mt-2 flex items-center gap-3">
                            <v-progress-linear :color="run.status === 'running' ? 'teal' : 'warning'" indeterminate
                              rounded :height="6" class="flex-1" />
                            <span class="text-xs whitespace-nowrap shrink-0" :class="run.status === 'running'
                              ? 'bt-text-running'
                              : 'bt-text-queued'">
                              {{ run.status === "running" ? "回测运行中…" : "排队等待中…" }}
                            </span>
                          </div>
                        </div>
                        <v-btn v-if="isTerminalBacktestStatus(run.status)" icon="fa-solid fa-trash"
                          class="bt-history-run__delete" size="x-small" variant="text" color="error" title="删除回测结果"
                          @click.stop="requestDeleteRun(run.id)" />
                      </div>
                    </div>
                  </div>

                  <div v-if="resultsPageCount > 1" class="flex justify-center p-2">
                    <v-pagination v-model="resultsPage" class="bt-sidebar-pagination" :length="resultsPageCount"
                      :total-visible="3" density="comfortable" />
                  </div>
                </div>
              </section>
</template>

<style scoped>
.bt-backtest-results-filters {
  display: grid;
  gap: 6px;
  padding: 8px;
  border-bottom: 1px solid var(--tv-border);
}

.bt-filter-reset {
  justify-self: end;
  min-height: 24px;
  border: 0;
  background: transparent;
  color: var(--tv-accent);
  padding: 0 4px;
  font-size: 0.7rem;
  font-weight: 700;
}

.bt-filter-reset:disabled {
  color: var(--tv-text-dim);
  opacity: 0.5;
}

.bt-history-list {
  display: grid;
  gap: 0;
}

.bt-history-run {
  min-width: 0;
  border-width: 0 0 1px;
  border-style: solid;
  border-radius: 0;
  padding: 8px;
}

.bt-history-run__title {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  color: var(--tv-text);
  font-size: 0.78rem;
  font-weight: 750;
}

.bt-history-run__status {
  display: inline-flex;
  min-height: 18px;
  flex: 0 0 auto;
  align-items: center;
  border-radius: 999px;
  background: var(--tv-bg-elevated);
  padding: 0 6px;
  color: var(--tv-text-muted);
  font-size: 0.64rem;
  font-weight: 750;
}

.bt-history-run__status.is-running {
  color: #2dd4bf;
}

.bt-history-run__status.is-failed,
.bt-history-run__status.is-cancelled {
  color: #f87171;
}

.bt-history-run__meta,
.bt-history-run__id {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.67rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-history-run__meta {
  display: flex;
  gap: 7px;
  margin-top: 3px;
}

.bt-history-run__id {
  margin-top: 2px;
  color: var(--tv-text-dim);
}

.bt-history-run__delete {
  flex: 0 0 auto;
  opacity: 0;
  transition: opacity 120ms ease;
}

.bt-history-run:is(:hover, :focus-within) .bt-history-run__delete {
  opacity: 1;
}

.bt-sidebar-pagination {
  max-width: 100%;
  min-width: 0;
}

.bt-sidebar-pagination :deep(.v-pagination__list) {
  flex-wrap: wrap;
  justify-content: center;
  max-width: 100%;
  min-width: 0;
}

.bt-sidebar-pagination :deep(.v-btn) {
  height: 30px;
  min-width: 30px;
  width: 30px;
}

.bt-history-run--selected {
  border-color: color-mix(in srgb, var(--tv-accent) 54%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 12%, var(--tv-bg-surface));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 18%, transparent);
}

.bt-history-run--idle:hover {
  border-color: color-mix(in srgb, var(--tv-accent) 42%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 6%, var(--tv-bg-surface));
}

.bt-text-running {
  color: var(--tv-status-success-fg);
}

.bt-text-queued {
  color: var(--tv-status-warning-fg);
}
</style>
