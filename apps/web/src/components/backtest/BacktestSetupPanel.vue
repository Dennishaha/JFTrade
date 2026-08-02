<script setup lang="ts">
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import InstrumentSearchBox from "../domain/market-data/InstrumentSearchBox.vue";
import { useBacktestPageContext } from "@/composables/backtest/useBacktestPage";

const {
  BACKTEST_BROKER_FEE_MODE_OPTIONS,
  BACKTEST_MARKET_FEE_MODE_OPTIONS,
  KLINE_CHART_TYPES,
  KLINE_PERIODS,
  brokerFeeMode,
  brokerFeeRulesText,
  cancelSync,
  chartType,
  costModeSummary,
  definitions,
  displayInstrumentId,
  endDate,
  expandedBacktestPanels,
  extendedHoursHint,
  extendedHoursSupported,
  handleResolvedBacktestInstrument,
  initialBalance,
  instrumentSearchQuery,
  instrumentSelectionResolved,
  interval,
  marketFeeMode,
  marketFeeRulesText,
  quoteCurrency,
  rehabType,
  running,
  selectedDefinitionId,
  showNewBacktestForm,
  startBacktest,
  startDate,
  syncKlines,
  syncProgress,
  syncing,
  toggleBacktestPanel,
  useExtendedHours,
  warmupPreviewNote,
  warmupPreviewValue,
} = useBacktestPageContext();
</script>

<template>
<section class="bt-sidebar-panel bt-sidebar-panel--setup"
                :class="{ 'is-expanded': expandedBacktestPanels.includes('setup') }">
                <button type="button" class="bt-sidebar-panel__title" data-testid="backtest-side-panel-setup-title"
                  :aria-expanded="expandedBacktestPanels.includes('setup')" @click="toggleBacktestPanel('setup')">
                  <v-icon size="11">fa-solid fa-chevron-right</v-icon>
                  <span>回测配置</span>
                  <em>{{ selectedDefinitionId ? "已选择策略" : "等待策略" }}</em>
                </button>
                <div v-if="showNewBacktestForm" class="bt-sidebar-panel__body bt-sidebar-panel__body--setup">
                  <div class="bt-new-backtest-form">
                    <div class="bt-new-backtest-fields">
                      <section class="grid gap-1.5">
                        <div class="flex items-center justify-between gap-2">
                          <div class="text-sm font-semibold bt-text-strong">策略与标的</div>
                          <div class="truncate text-xs bt-text-muted">
                            <InstrumentIdentity v-if="displayInstrumentId" :instrument-id="displayInstrumentId"
                              compact />
                            <template v-else>等待标的</template>
                          </div>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-definition">策略定义</label>
                          <select id="bt-field-definition" v-model="selectedDefinitionId" class="bt-native-select">
                            <option value="" disabled>选择策略</option>
                            <option v-for="definition in definitions" :key="definition.id" :value="definition.id">
                              {{ definition.name }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label">代码或名称</label>
                          <InstrumentSearchBox v-model="instrumentSearchQuery" action-label="查询"
                            input-test-id="backtest-instrument-code" placeholder="输入代码或名称"
                            root-test-id="backtest-instrument-search" submit-test-id="backtest-instrument-submit"
                            variant="backtest" @select="handleResolvedBacktestInstrument" />
                        </div>
                        <div v-if="!instrumentSelectionResolved" class="bt-inline-warning"
                          data-testid="backtest-instrument-unresolved">
                          当前输入尚未解析。请查询并选择标的后再同步或运行；未解析内容不会覆盖已保存标的。
                        </div>
                      </section>

                      <section class="grid gap-1.5 border-t bt-border pt-2">
                        <div class="text-sm font-semibold bt-text-strong">数据范围</div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-interval">K线周期</label>
                          <select id="bt-field-interval" v-model="interval" class="bt-native-select">
                            <option v-for="period in KLINE_PERIODS" :key="period.value" :value="period.value">
                              {{ period.label }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-chart-type">图表类型</label>
                          <select id="bt-field-chart-type" v-model="chartType" class="bt-native-select"
                            :disabled="interval === 'tick'">
                            <option v-for="type in KLINE_CHART_TYPES" :key="type.value" :value="type.value">
                              {{ type.label }}
                            </option>
                          </select>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-start-date">起始日期</label>
                          <input id="bt-field-start-date" v-model="startDate" type="date" class="bt-native-input" />
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-end-date">结束日期</label>
                          <input id="bt-field-end-date" v-model="endDate" type="date" class="bt-native-input" />
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-rehab">复权方式</label>
                          <select id="bt-field-rehab" v-model="rehabType" class="bt-native-select">
                            <option value="forward">前复权</option>
                            <option value="backward">后复权</option>
                            <option value="none">不复权</option>
                          </select>
                        </div>
                        <div v-if="extendedHoursSupported"
                          class="bt-extended-hours rounded-md border bt-border px-2 py-1.5">
                          <label class="bt-form-check">
                            <input v-model="useExtendedHours" type="checkbox" class="bt-form-check__input" />
                            <span class="min-w-0 flex-1">
                              <span class="bt-form-check__title">扩展交易时段</span>
                              <span class="bt-form-check__hint">{{ extendedHoursHint }}</span>
                            </span>
                          </label>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label">预热K线</label>
                          <div class="bt-warmup-preview" :title="warmupPreviewNote">
                            <span class="bt-warmup-preview__value">{{ warmupPreviewValue }}</span>
                            <span class="bt-warmup-preview__note">{{ warmupPreviewNote }}</span>
                          </div>
                        </div>
                      </section>

                      <section class="grid gap-1.5 border-t bt-border pt-2">
                        <div class="flex items-center justify-between gap-2">
                          <div class="text-sm font-semibold bt-text-strong">资金与成本</div>
                          <div class="text-xs bt-text-muted">{{ costModeSummary }}</div>
                        </div>
                        <div class="bt-form-row">
                          <label class="bt-form-row__label" for="bt-field-initial-balance">初始资金</label>
                          <span class="bt-input-suffix">
                            <input id="bt-field-initial-balance" v-model.number="initialBalance" type="number"
                              :min="1000" class="bt-native-input" />
                            <span class="bt-input-suffix__text">{{ quoteCurrency }}</span>
                          </span>
                        </div>
                        <div class="grid grid-cols-2 gap-2">
                          <div class="bt-form-row bt-form-row--compact">
                            <label class="bt-form-row__label" for="bt-field-broker-fee">券商费用</label>
                            <select id="bt-field-broker-fee" v-model="brokerFeeMode" class="bt-native-select">
                              <option v-for="option in BACKTEST_BROKER_FEE_MODE_OPTIONS" :key="option.value"
                                :value="option.value">
                                {{ option.title }}
                              </option>
                            </select>
                          </div>
                          <div class="bt-form-row bt-form-row--compact">
                            <label class="bt-form-row__label" for="bt-field-market-fee">市场费用</label>
                            <select id="bt-field-market-fee" v-model="marketFeeMode" class="bt-native-select">
                              <option v-for="option in BACKTEST_MARKET_FEE_MODE_OPTIONS" :key="option.value"
                                :value="option.value">
                                {{ option.title }}
                              </option>
                            </select>
                          </div>
                        </div>
                        <div v-if="brokerFeeMode === 'custom'" class="grid gap-1">
                          <label class="bt-form-row__label" for="bt-field-broker-rules">券商费用规则 JSON</label>
                          <textarea id="bt-field-broker-rules" v-model="brokerFeeRulesText" class="bt-native-textarea"
                            rows="3" />
                        </div>
                        <div v-if="marketFeeMode === 'custom'" class="grid gap-1">
                          <label class="bt-form-row__label" for="bt-field-market-rules">市场费用规则 JSON</label>
                          <textarea id="bt-field-market-rules" v-model="marketFeeRulesText" class="bt-native-textarea"
                            rows="3" />
                        </div>
                      </section>
                    </div>

                    <section class="bt-new-backtest-run grid gap-1.5">
                      <div class="bt-run-actions">

                        <!-- Sync section -->
                        <div v-if="syncing && !syncProgress" class="bt-sync-block bt-sync-block--pending">
                          <span>正在启动同步…</span>
                        </div>
                        <div v-else-if="syncing && syncProgress" class="bt-sync-block">
                          <div class="bt-sync-block__head">
                            <span class="bt-sync-block__title">
                              同步中 · {{ syncProgress.currentInterval || "准备" }}
                            </span>
                            <button class="bt-sync-block__cancel" type="button" @click="cancelSync">
                              取消
                            </button>
                          </div>
                          <div class="bt-sync-block__bar">
                            <div class="bt-sync-block__bar-fill" :style="{
                              width:
                                syncProgress.totalIntervals > 0
                                  ? (syncProgress.completedIntervals /
                                    syncProgress.totalIntervals) *
                                  100 +
                                  '%'
                                  : '10%',
                            }" />
                          </div>
                          <div class="bt-sync-block__meta">
                            <span>{{ syncProgress.completedBatches }} 批</span>
                            <span v-if="syncProgress.retries > 0" class="bt-text-queued">重试 {{ syncProgress.retries
                              }}</span>
                          </div>
                        </div>
                        <div v-else-if="syncProgress?.status === 'cancelled'"
                          class="bt-sync-block bt-sync-block--cancelled">
                          同步已取消 · {{ syncProgress.completedBatches }} 批已完成
                        </div>
                        <!-- Sync button -->
                        <button v-else class="bt-run-btn" :disabled="running || !instrumentSelectionResolved"
                          type="button" @click="syncKlines">
                          <v-icon size="13">fa-solid fa-cloud-arrow-down</v-icon>
                          同步K线
                        </button>

                        <!-- Run button -->
                        <button class="bt-run-btn bt-run-btn--primary"
                          :disabled="running || !selectedDefinitionId || !instrumentSelectionResolved" type="button"
                          @click="startBacktest">
                          <v-progress-circular v-if="running" indeterminate :size="16" :width="2" color="white" />
                          <v-icon v-else size="13">fa-solid fa-play</v-icon>
                          {{ running ? "启动中..." : "开始回测" }}
                        </button>
                      </div>

                    </section>
                  </div>
                </div>
              </section>
</template>

<style scoped>
.bt-new-backtest-form {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
}

.bt-new-backtest-fields {
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.bt-new-backtest-fields>section,
.bt-new-backtest-run {
  gap: 6px !important;
  padding: 8px;
}

.bt-new-backtest-fields>section+section,
.bt-new-backtest-run {
  border-top: 1px solid var(--tv-border);
}

.bt-new-backtest-fields>section> :is(.text-sm, .flex:first-child) {
  min-height: 24px;
}

.bt-new-backtest-run {
  flex: 0 0 auto;
  background: var(--tv-bg-surface);
  box-shadow: 0 -8px 18px color-mix(in srgb, var(--tv-bg-app) 42%, transparent);
}

.bt-run-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.bt-run-actions>div {
  grid-column: 1 / -1;
}

.bt-run-actions>div+button:last-child {
  grid-column: 1 / -1;
}

.bt-run-btn {
  display: inline-flex;
  min-width: 0;
  min-height: 30px;
  height: 30px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid color-mix(in srgb, var(--jf-accent-teal) 45%, var(--tv-border));
  border-radius: 5px;
  background: color-mix(in srgb, var(--jf-accent-teal) 12%, var(--tv-bg-surface));
  color: var(--jf-accent-teal-text);
  padding: 0 10px;
  font-size: 0.75rem;
  font-weight: 750;
  cursor: pointer;
  transition: background 120ms ease, border-color 120ms ease;
}

.bt-run-btn:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--jf-accent-teal) 62%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-teal) 18%, var(--tv-bg-surface));
}

.bt-run-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.bt-run-btn--primary {
  border-color: color-mix(in srgb, var(--jf-accent-teal) 62%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-teal) 68%, var(--tv-bg-surface));
  color: #f0fdfa;
}

.bt-run-btn--primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--jf-accent-teal) 78%, var(--tv-bg-surface));
}

.bt-run-btn--primary:disabled {
  border-color: var(--tv-border);
  background: color-mix(in srgb, var(--tv-bg-elevated) 60%, var(--tv-border) 40%);
  color: var(--tv-text-dim);
  opacity: 1;
}

.bt-sync-block {
  display: grid;
  min-width: 0;
  gap: 6px;
  border: 1px solid color-mix(in srgb, var(--jf-accent-teal) 40%, var(--tv-border));
  border-radius: 6px;
  background: color-mix(in srgb, var(--jf-accent-teal) 8%, var(--tv-bg-surface));
  padding: 6px 8px;
  color: var(--jf-accent-teal-text);
  font-size: 0.72rem;
}

.bt-sync-block--pending {
  place-items: center;
  text-align: center;
}

.bt-sync-block__head,
.bt-sync-block__meta {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.bt-sync-block__title {
  min-width: 0;
  overflow: hidden;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sync-block__cancel {
  flex: 0 0 auto;
  border: 1px solid color-mix(in srgb, var(--jf-accent-red) 45%, var(--tv-border));
  border-radius: 999px;
  background: transparent;
  color: color-mix(in srgb, var(--jf-accent-red-text) 78%, var(--tv-text));
  padding: 1px 8px;
  font-size: 0.68rem;
  cursor: pointer;
}

.bt-sync-block__cancel:hover {
  background: color-mix(in srgb, var(--jf-accent-red) 10%, var(--tv-bg-surface));
}

.bt-sync-block__bar {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: color-mix(in srgb, var(--jf-accent-teal) 22%, var(--tv-bg-surface));
}

.bt-sync-block__bar-fill {
  height: 100%;
  border-radius: 999px;
  background: var(--jf-accent-teal);
  transition: width 500ms ease;
}

.bt-sync-block--cancelled {
  border-color: color-mix(in srgb, var(--jf-accent-amber) 44%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-amber) 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-amber-text) 72%, var(--tv-text));
}

input {
  min-width: 0;
  max-width: 100%;
}

.grid {
  min-width: 0;
}

.grid>*,
.flex>* {
  min-width: 0;
}

.bt-form-row {
  display: grid;
  min-width: 0;
  grid-template-columns: 76px minmax(0, 1fr);
  align-items: center;
  gap: 6px;
}

.bt-form-row--compact {
  grid-template-columns: auto minmax(0, 1fr);
}

.bt-form-row__label {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-overflow: ellipsis;
  text-transform: uppercase;
  white-space: nowrap;
}

.bt-input-suffix {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.bt-input-suffix__text {
  flex: 0 0 auto;
  color: var(--tv-text-dim);
  font-size: 0.7rem;
}

.bt-inline-warning {
  border: 1px solid color-mix(in srgb, var(--jf-accent-amber) 44%, var(--tv-border));
  border-radius: 4px;
  background: color-mix(in srgb, var(--jf-accent-amber) 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-amber-text) 72%, var(--tv-text));
  padding: 4px 8px;
  font-size: 0.72rem;
  line-height: 1.35;
}

.bt-form-check {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
  cursor: pointer;
}

.bt-form-check__input {
  width: auto;
  min-height: 0;
  margin-top: 2px;
  accent-color: var(--jf-accent-teal);
  cursor: pointer;
}

.bt-form-check__title {
  display: block;
  color: var(--tv-text);
  font-size: 0.72rem;
  font-weight: 700;
}

.bt-form-check__hint {
  display: block;
  color: var(--tv-text-dim);
  font-size: 0.68rem;
  line-height: 1.35;
}

.bt-warmup-preview {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface-2);
  padding: 4px 8px;
}

.bt-warmup-preview__value {
  flex: 0 0 auto;
  color: var(--tv-text);
  font-size: 0.78rem;
}

.bt-warmup-preview__note {
  min-width: 0;
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 0.68rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-text-running {
  color: var(--tv-status-success-fg);
}

.bt-text-queued {
  color: var(--tv-status-warning-fg);
}
</style>
