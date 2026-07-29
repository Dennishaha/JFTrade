<script setup lang="ts">
import StrategySourceDiff from "../StrategySourceDiff.vue";
import { useBacktestPageContext } from "../../composables/useBacktestPage";

const {
  changeComparisonDefinition,
  changeComparisonRun,
  changeComparisonVersion,
  comparisonConditionsMatch,
  comparisonConfigRows,
  comparisonDefinitionId,
  comparisonDefinitionOptions,
  comparisonMetricDelta,
  comparisonMetrics,
  comparisonRunsReady,
  comparisonSnapshotErrors,
  comparisonSnapshotLoading,
  comparisonSourcesReady,
  comparisonVersions,
  comparisonVersionsError,
  emptyStateClass,
  formatComparisonMetric,
  isLoadingComparisonVersions,
  leftComparisonRun,
  leftComparisonRunId,
  leftComparisonRunOptions,
  leftComparisonRuns,
  leftComparisonSnapshot,
  leftComparisonVersion,
  leftComparisonVersionSelectOptions,
  nativeSelectValue,
  resolveRunQuoteCurrency,
  rightComparisonRun,
  rightComparisonRunId,
  rightComparisonRunOptions,
  rightComparisonRuns,
  rightComparisonSnapshot,
  rightComparisonVersion,
  rightComparisonVersionSelectOptions,
} = useBacktestPageContext();
</script>

<template>
<section class="bt-version-comparison">
                <div class="bt-report-topbar">
                  <span class="bt-report-topbar__title">策略版本对比</span>
                </div>
                <div class="bt-version-comparison__body">
                  <div class="bt-version-compare-definition">
                    <div>
                      <label class="text-xs font-semibold bt-text-strong">策略定义</label>
                      <span>基线在左，候选在右；只使用各版本已完成的回测。</span>
                    </div>
                    <select :value="comparisonDefinitionId" class="bt-native-select"
                      data-testid="backtest-comparison-definition" aria-label="对比策略定义"
                      @change="changeComparisonDefinition(nativeSelectValue($event))">
                      <option value="" disabled>选择策略</option>
                      <option v-for="option in comparisonDefinitionOptions" :key="option.value" :value="option.value">
                        {{ option.title }}
                      </option>
                    </select>
                  </div>

                  <div v-if="isLoadingComparisonVersions" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    正在加载版本历史…
                  </div>
                  <div v-else-if="comparisonVersionsError"
                    class="bt-version-compare-notice bt-version-compare-notice--warning">
                    版本历史暂不可用：{{ comparisonVersionsError }}
                  </div>
                  <div v-else-if="comparisonDefinitionId === ''" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    请选择拥有版本历史的策略。
                  </div>
                  <div v-else-if="comparisonVersions.length < 2" :class="[emptyStateClass, 'p-5 text-center text-sm']">
                    至少需要两个已保存策略版本才能比较。
                  </div>
                  <template v-else>
                    <div class="bt-version-compare-selectors">
                      <section class="bt-version-compare-selector" data-testid="backtest-comparison-left">
                        <div class="bt-version-compare-selector__eyebrow">基线（较早版本）</div>
                        <select :value="leftComparisonVersion" class="bt-native-select"
                          data-testid="backtest-comparison-left-version" aria-label="选择基线版本"
                          @change="changeComparisonVersion('left', nativeSelectValue($event))">
                          <option value="" disabled>选择基线版本</option>
                          <option v-for="option in leftComparisonVersionSelectOptions" :key="option.value"
                            :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                        <div v-if="leftComparisonRuns.length === 0" class="bt-version-compare-selector__empty">
                          该版本暂无已完成回测。
                        </div>
                        <select v-else :value="leftComparisonRunId" class="bt-native-select"
                          data-testid="backtest-comparison-left-run" aria-label="关联回测"
                          @change="changeComparisonRun('left', nativeSelectValue($event))">
                          <option value="" disabled>关联回测</option>
                          <option v-for="option in leftComparisonRunOptions" :key="option.value" :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                      </section>
                      <section class="bt-version-compare-selector" data-testid="backtest-comparison-right">
                        <div class="bt-version-compare-selector__eyebrow">候选（较新版本）</div>
                        <select :value="rightComparisonVersion" class="bt-native-select"
                          data-testid="backtest-comparison-right-version" aria-label="选择候选版本"
                          @change="changeComparisonVersion('right', nativeSelectValue($event))">
                          <option value="" disabled>选择候选版本</option>
                          <option v-for="option in rightComparisonVersionSelectOptions" :key="option.value"
                            :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                        <div v-if="rightComparisonRuns.length === 0" class="bt-version-compare-selector__empty">
                          该版本暂无已完成回测。
                        </div>
                        <select v-else :value="rightComparisonRunId" class="bt-native-select"
                          data-testid="backtest-comparison-right-run" aria-label="关联回测"
                          @change="changeComparisonRun('right', nativeSelectValue($event))">
                          <option value="" disabled>关联回测</option>
                          <option v-for="option in rightComparisonRunOptions" :key="option.value" :value="option.value">
                            {{ option.title }}
                          </option>
                        </select>
                      </section>
                    </div>

                    <div v-if="comparisonRunsReady && leftComparisonRun && rightComparisonRun"
                      class="bt-version-compare-results">
                      <div class="bt-version-compare-notice"
                        :class="comparisonConditionsMatch ? 'bt-version-compare-notice--ok' : 'bt-version-compare-notice--warning'">
                        <template v-if="comparisonConditionsMatch">
                          两次回测的配置一致，可将指标差异作为策略版本变化的参考。
                        </template>
                        <template v-else>
                          两次回测存在配置差异，结果不可直接归因于策略代码。请结合下方配置表评估。
                        </template>
                      </div>

                      <section class="bt-version-compare-section">
                        <div class="bt-version-compare-section__title">绩效指标</div>
                        <div class="bt-version-compare-metrics">
                          <div class="bt-version-compare-metrics__head">指标</div>
                          <div class="bt-version-compare-metrics__head">基线 v{{ leftComparisonVersion }}</div>
                          <div class="bt-version-compare-metrics__head">候选 v{{ rightComparisonVersion }}</div>
                          <div class="bt-version-compare-metrics__head">候选 − 基线</div>
                          <template v-for="metric in comparisonMetrics" :key="metric.label">
                            <div class="bt-version-compare-metrics__label">{{ metric.label }}</div>
                            <div>{{ formatComparisonMetric(metric.left, metric.kind,
                              resolveRunQuoteCurrency(leftComparisonRun)) }}</div>
                            <div>{{ formatComparisonMetric(metric.right, metric.kind,
                              resolveRunQuoteCurrency(rightComparisonRun)) }}</div>
                            <div>{{ comparisonMetricDelta(metric) }}</div>
                          </template>
                        </div>
                      </section>

                      <section class="bt-version-compare-section">
                        <div class="bt-version-compare-section__title">回测配置</div>
                        <div class="bt-version-compare-config">
                          <div class="bt-version-compare-config__head">字段</div>
                          <div class="bt-version-compare-config__head">基线</div>
                          <div class="bt-version-compare-config__head">候选</div>
                          <template v-for="row in comparisonConfigRows" :key="row.label">
                            <div class="bt-version-compare-config__label" :class="{ 'is-different': !row.same }">{{
                              row.label }}</div>
                            <div :class="{ 'is-different': !row.same }">{{ row.left }}</div>
                            <div :class="{ 'is-different': !row.same }">{{ row.right }}</div>
                          </template>
                        </div>
                      </section>
                    </div>
                    <div v-else class="bt-version-compare-notice bt-version-compare-notice--warning">
                      请选择两个版本各自的已完成回测后查看指标与配置对比。
                    </div>

                    <section class="bt-version-compare-section">
                      <div class="bt-version-compare-section__title">Pine 源码差异</div>
                      <StrategySourceDiff
                        v-if="comparisonSourcesReady && leftComparisonSnapshot && rightComparisonSnapshot"
                        :left-label="`基线 v${leftComparisonVersion}`" :right-label="`候选 v${rightComparisonVersion}`"
                        :left-source="leftComparisonSnapshot.script || ''"
                        :right-source="rightComparisonSnapshot.script || ''" />
                      <div v-else class="bt-version-compare-notice bt-version-compare-notice--warning">
                        <template v-if="comparisonSnapshotLoading.left || comparisonSnapshotLoading.right">
                          正在加载历史源码快照…
                        </template>
                        <template v-else-if="comparisonSnapshotErrors.left || comparisonSnapshotErrors.right">
                          策略版本快照不可用：{{ comparisonSnapshotErrors.left || comparisonSnapshotErrors.right
                          }}。升级前回测可能只保留指标和配置，无法伪造源码差异。
                        </template>
                        <template v-else>
                          选择两个不同版本后可查看只读源码差异。
                        </template>
                      </div>
                    </section>
                  </template>
                </div>
              </section>
</template>

<style scoped>
.bt-version-comparison {
  display: flex;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  overflow: auto;
  background: var(--tv-bg-app);
}

.bt-version-comparison__body {
  display: grid;
  gap: 8px;
  padding: 8px;
}

.bt-version-compare-definition {
  display: grid;
  grid-template-columns: minmax(14rem, 1fr) minmax(16rem, 24rem);
  align-items: center;
  gap: 8px;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding-bottom: 8px;
}

.bt-version-compare-definition>div {
  display: grid;
  gap: 2px;
}

.bt-version-compare-definition>div>span {
  color: var(--tv-text-muted);
  font-size: 0.68rem;
}

.bt-version-compare-selectors {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.bt-version-compare-selector {
  display: grid;
  align-content: start;
  gap: 6px;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 62%, transparent);
  padding: 8px;
}

.bt-version-compare-selector__eyebrow,
.bt-version-compare-section__title {
  color: var(--tv-text-muted);
  font-size: 0.75rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.bt-version-compare-selector__empty {
  border: 1px dashed var(--tv-border);
  border-radius: 0.4rem;
  color: var(--tv-text-muted);
  padding: 0.6rem;
  font-size: 0.78rem;
}

.bt-version-compare-results,
.bt-version-compare-section {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.bt-version-compare-notice {
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 70%, transparent);
  color: var(--tv-text-muted);
  padding: 6px 8px;
  font-size: 0.73rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.bt-version-compare-notice--warning {
  border-color: color-mix(in srgb, #f59e0b 46%, var(--tv-border));
  background: color-mix(in srgb, #f59e0b 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #fbbf24 72%, var(--tv-text));
}

.bt-version-compare-notice--ok {
  border-color: color-mix(in srgb, #22c55e 48%, var(--tv-border));
  background: color-mix(in srgb, #22c55e 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, #86efac 72%, var(--tv-text));
}

.bt-version-compare-metrics {
  display: grid;
  grid-template-columns: minmax(7rem, 1fr) repeat(3, minmax(8rem, 1fr));
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  font-size: 0.76rem;
}

.bt-version-compare-metrics>div {
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding: 4px 6px;
  overflow-wrap: anywhere;
}

.bt-version-compare-metrics>div:nth-last-child(-n + 4) {
  border-bottom: 0;
}

.bt-version-compare-metrics__head,
.bt-version-compare-config__head {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 800;
}

.bt-version-compare-metrics__label,
.bt-version-compare-config__label {
  color: var(--tv-text);
  font-weight: 700;
}

.bt-version-compare-config {
  display: grid;
  grid-template-columns: minmax(6rem, 0.7fr) repeat(2, minmax(10rem, 1fr));
  overflow: auto;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  font-size: 0.74rem;
}

.bt-version-compare-config>div {
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  padding: 4px 6px;
  overflow-wrap: anywhere;
}

.bt-version-compare-config>div:nth-last-child(-n + 3) {
  border-bottom: 0;
}

.bt-version-compare-config .is-different {
  background: color-mix(in srgb, #f59e0b 8%, var(--tv-bg-surface));
  color: var(--tv-text);
}

.bt-native-select {
  width: 100%;
  min-width: 0;
  min-height: 32px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 5px 8px;
  font-size: 0.83rem;
  line-height: 1.25;
  outline: none;
}

.bt-native-select:focus {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

@media (max-width: 768px) {
  .bt-version-compare-selectors {
    grid-template-columns: minmax(0, 1fr);
  }

  .bt-version-compare-metrics {
    grid-template-columns: minmax(6.5rem, 1fr) repeat(3, minmax(7rem, 1fr));
  }

  .bt-version-compare-definition {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
