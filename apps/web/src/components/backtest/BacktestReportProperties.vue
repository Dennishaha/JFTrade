<script setup lang="ts">
import { computed } from "vue";

import type { BacktestRun } from "@/composables/useBacktestRuns";

import {
  resolveBacktestPriceBasisNote,
  resolveQueriedCandleBounds,
  runtimeErrorRepeatCount,
  runtimeErrorSummary,
  warningSummary,
} from "./backtestRunPresentation";

const RUNTIME_ERROR_WINDOW = 120;
const WARNING_WINDOW = 120;
const LOG_WINDOW = 120;
const props = defineProps<{ run: BacktestRun }>();

const candleBounds = computed(() => resolveQueriedCandleBounds(props.run.result?.candles));
const visibleRuntimeErrors = computed(() => props.run.result?.runtimeErrors?.slice(0, RUNTIME_ERROR_WINDOW) ?? []);
const visibleWarnings = computed(() => props.run.result?.warnings?.slice(0, WARNING_WINDOW) ?? []);
const visibleLogs = computed(() => props.run.result?.logs?.slice(0, LOG_WINDOW) ?? []);
const hiddenRuntimeErrors = computed(() => Math.max(0, (props.run.result?.runtimeErrors?.length ?? 0) - RUNTIME_ERROR_WINDOW));
const hiddenWarnings = computed(() => Math.max(0, (props.run.result?.warnings?.length ?? 0) - WARNING_WINDOW));
const hiddenLogs = computed(() => Math.max(0, (props.run.result?.logs?.length ?? 0) - LOG_WINDOW));
</script>

<template>
  <div class="grid h-full min-h-0 gap-2 overflow-auto p-2">
    <div v-if="run.result" class="bt-report-properties__basis">
      <div>{{ resolveBacktestPriceBasisNote(run) }}</div>
      <div class="mt-1">
        费用口径：券商 {{ run.result.tradingCosts?.brokerFees?.mode ?? "market_preset" }} ｜
        市场 {{ run.result.tradingCosts?.marketFees?.mode ?? "market_preset" }}
      </div>
      <div v-if="candleBounds" class="mt-1">
        查询到的周期边界：左边界 {{ candleBounds.left }} ｜ 右边界 {{ candleBounds.right }} ｜
        共 {{ candleBounds.count }} 根
      </div>
    </div>

    <details v-if="run.result?.runtimeErrors?.length" class="bt-prop-block bt-prop-block--error">
      <summary class="bt-prop-block__summary">
        <v-icon size="13">fa-solid fa-circle-exclamation</v-icon>
        {{ runtimeErrorSummary(run.result) }}
      </summary>
      <div class="mt-1.5 max-h-48 space-y-1 overflow-y-auto">
        <div v-for="(message, index) in visibleRuntimeErrors" :key="index" class="bt-prop-block__item">
          <span v-if="runtimeErrorRepeatCount(run.result, message) > 1" class="font-semibold">
            x{{ runtimeErrorRepeatCount(run.result, message) }}
          </span>
          {{ message }}
        </div>
        <div v-if="hiddenRuntimeErrors > 0" class="bt-prop-block__more">另有 {{ hiddenRuntimeErrors }} 条错误。</div>
      </div>
    </details>

    <details v-if="run.result?.warnings?.length" class="bt-prop-block bt-prop-block--warning">
      <summary class="bt-prop-block__summary">
        <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
        {{ warningSummary(run.result) }}
      </summary>
      <div class="mt-1.5 max-h-48 space-y-1 overflow-y-auto">
        <div v-for="(warning, index) in visibleWarnings" :key="index" class="bt-prop-block__item">{{ warning }}</div>
        <div v-if="hiddenWarnings > 0" class="bt-prop-block__more">另有 {{ hiddenWarnings }} 条警告。</div>
      </div>
    </details>

    <div v-if="visibleLogs.length" class="bt-prop-block bt-prop-block--warning space-y-1">
      <div v-for="(log, index) in visibleLogs" :key="index" class="flex gap-2">
        <v-icon size="12" class="mt-0.5">fa-solid fa-circle-info</v-icon><span>{{ log }}</span>
      </div>
      <div v-if="hiddenLogs > 0">另有 {{ hiddenLogs }} 条日志。</div>
    </div>

    <div v-if="run.result?.error" class="bt-prop-block bt-prop-block--error whitespace-pre-wrap">
      {{ run.result.error }}
    </div>
    <div v-if="!run.result" class="bt-report-properties__empty">暂无属性。</div>
  </div>
</template>

<style scoped>
.bt-report-properties__basis,
.bt-report-properties__empty {
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-lg);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  font-size: 0.75rem;
}
.bt-report-properties__basis { padding: 6px 10px; }
.bt-report-properties__empty { padding: var(--jf-space-6); color: var(--tv-text-muted); text-align: center; }
.bt-prop-block { min-width: 0; border: 1px solid var(--tv-border); border-radius: var(--jf-radius-md); padding: 6px 10px; font-size: 0.72rem; line-height: 1.45; }
.bt-prop-block--error { border-color: var(--tv-status-error-border); background: var(--tv-status-error-bg); color: var(--tv-status-error-fg); }
.bt-prop-block--warning { border-color: var(--tv-status-warning-border); background: var(--tv-status-warning-bg); color: var(--tv-status-warning-fg); }
.bt-prop-block__summary { display: flex; align-items: center; gap: var(--jf-space-2); font-weight: 700; cursor: pointer; user-select: none; }
.bt-prop-block__item { border: 1px solid color-mix(in srgb, currentColor 22%, transparent); border-radius: var(--jf-radius-xs); background: var(--tv-bg-surface); padding: var(--jf-space-1) var(--jf-space-2); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.7rem; line-height: 1.5; }
.bt-prop-block__more { font-size: 0.7rem; }
</style>
