<script setup lang="ts">
import BacktestChart from "@/components/BacktestChart.vue";
import InstrumentIdentity from "@/components/domain/market-data/InstrumentIdentity.vue";
import type { BacktestRun } from "@/composables/useBacktestRuns";

import {
  formatBacktestRunDate,
  type BacktestReportTab,
} from "./backtestRunPresentation";
import BacktestOrderTable from "./BacktestOrderTable.vue";
import BacktestReportProperties from "./BacktestReportProperties.vue";
import BacktestReportSummary from "./BacktestReportSummary.vue";

defineProps<{
  run: BacktestRun;
  resultReady: boolean;
  hasChartData: boolean;
  strategyName: string;
  strategyVersionLabel: string;
  statusLabel: string;
  quoteCurrency: string;
  sessionMode: string;
  rehabLabel: string;
  versionNotice: string;
  detailLoading: boolean;
  detailError: string;
}>();

const activeTab = defineModel<BacktestReportTab>("activeTab", { required: true });
</script>

<template>
  <section class="bt-report-workspace">
    <div class="bt-report-topbar">
      <span class="bt-report-topbar__title">
        {{ strategyName }} · <InstrumentIdentity :instrument-id="run.request.symbol" compact />
      </span>
      <span v-if="run.request.definitionVersion" class="bt-report-topbar__chip">{{ strategyVersionLabel }}</span>
      <span class="bt-report-topbar__chip">{{ run.request.interval }}</span>
      <span class="bt-report-topbar__chip bt-report-topbar__chip--status" :class="`is-${run.status}`">
        {{ statusLabel }}
      </span>
    </div>
    <div class="bt-report-context-bar">
      <span class="bt-report-context-bar__id" :title="run.id">{{ run.id }}</span>
      <span>{{ formatBacktestRunDate(run.request.startDate) }} → {{ formatBacktestRunDate(run.request.endDate) }}</span>
      <span>{{ sessionMode }}</span><span>{{ rehabLabel }}</span>
      <span>{{ run.request.initialBalance.toLocaleString() }} {{ quoteCurrency }}</span>
    </div>

    <div v-if="run.status === 'running' || run.status === 'queued' || versionNotice || detailLoading || detailError"
      class="bt-report-notices">
      <div v-if="run.status === 'running' || run.status === 'queued'" class="bt-report-notice flex items-center gap-3">
        <v-progress-linear :color="run.status === 'running' ? 'teal' : 'warning'" indeterminate rounded :height="4" class="flex-1" />
        <span class="shrink-0 whitespace-nowrap text-xs" :class="run.status === 'running' ? 'bt-text-running' : 'bt-text-queued'">
          {{ run.status === "running" ? "回测运行中…" : "排队等待中…" }}
        </span>
      </div>
      <div v-if="versionNotice" class="bt-report-notice bt-report-notice--warning">{{ versionNotice }}</div>
      <div v-if="detailLoading" class="bt-report-notice">正在加载完整回测详情…</div>
      <div v-if="detailError" class="bt-report-notice bt-report-notice--error">{{ detailError }}</div>
    </div>

    <BacktestReportSummary :result="run.result" :ready="resultReady" :currency="quoteCurrency" />

    <v-tabs v-model="activeTab" bg-color="transparent" density="compact" class="bt-report-tabs shrink-0">
      <v-tab value="chart"><v-icon size="13" class="mr-1">fa-solid fa-chart-line</v-icon>图表</v-tab>
      <v-tab value="orders"><v-icon size="13" class="mr-1">fa-solid fa-list-check</v-icon>订单</v-tab>
      <v-tab value="properties"><v-icon size="13" class="mr-1">fa-solid fa-sliders</v-icon>属性</v-tab>
    </v-tabs>

    <v-window v-model="activeTab" class="bt-report-window min-h-0 flex-1 overflow-hidden">
      <v-window-item value="chart" class="bt-report-window-item bt-report-window-item--chart">
        <div class="bt-report-chart-tab flex h-full min-h-0 flex-col">
          <div v-if="resultReady && run.result" class="bt-report-chart-stage min-h-0 flex-1">
            <BacktestChart v-if="hasChartData" :candles="run.result.candles ?? []" :trades="run.result.trades ?? []"
              :pnl-curve="run.result.pnlCurve ?? []" :drawdown-curve="run.result.drawdownCurve ?? []"
              :initial-balance="run.request.initialBalance" :chart-type="run.result.chartType ?? run.request.chartType"
              :heikin-ashi-seed="run.result.heikinAshiSeed" :currency-unit="quoteCurrency" fit-container
              empty-text="暂无权益曲线数据" />
            <div v-else class="bt-report-empty">暂无权益曲线数据。</div>
          </div>
          <div v-else class="bt-report-empty">当前回测尚未生成完整报告。</div>
        </div>
      </v-window-item>
      <v-window-item value="orders" class="bt-report-window-item">
        <BacktestOrderTable :entries="run.result?.orderBook ?? []" />
      </v-window-item>
      <v-window-item value="properties" class="bt-report-window-item">
        <BacktestReportProperties :run="run" />
      </v-window-item>
    </v-window>
  </section>
</template>

<style scoped>
.bt-report-workspace { display: flex; min-width: 0; min-height: 0; flex: 1 1 auto; flex-direction: column; overflow: hidden; background: var(--tv-bg-app); container-type: inline-size; }
.bt-report-topbar { display: flex; min-width: 0; min-height: 34px; flex: 0 0 34px; align-items: center; gap: 6px; overflow: hidden; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface); padding: 0 var(--jf-space-2); white-space: nowrap; }
.bt-report-topbar__title { display: flex; min-width: 0; flex: 1 1 auto; align-items: center; gap: var(--jf-space-1); overflow: hidden; color: var(--tv-text); font-size: 0.8rem; font-weight: 650; text-overflow: ellipsis; }
.bt-report-topbar__chip { display: inline-flex; min-height: 20px; flex: 0 0 auto; align-items: center; border: 1px solid var(--tv-border); border-radius: var(--jf-radius-pill); padding: 1px 6px; color: var(--tv-text-muted); font-size: 0.68rem; font-weight: 650; line-height: 1; white-space: nowrap; }
.bt-report-topbar__chip--status.is-running { border-color: var(--tv-status-success-border); color: var(--tv-status-success-fg); }
.bt-report-topbar__chip--status:is(.is-failed, .is-cancelled) { border-color: var(--tv-status-error-border); color: var(--tv-status-error-fg); }
.bt-report-context-bar { display: flex; min-width: 0; min-height: 30px; flex: 0 0 30px; align-items: center; gap: 10px; overflow: hidden; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface); padding: 0 var(--jf-space-2); color: var(--tv-text-muted); font-size: 0.68rem; white-space: nowrap; }
.bt-report-context-bar__id { max-width: 15rem; overflow: hidden; color: var(--tv-text); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; text-overflow: ellipsis; }
.bt-report-notices { display: grid; flex: 0 0 auto; border-bottom: 1px solid var(--tv-border); }
.bt-report-notice { min-width: 0; min-height: 28px; padding: 5px var(--jf-space-2); background: var(--tv-bg-surface); color: var(--tv-text-muted); font-size: 0.72rem; line-height: 1.35; }
.bt-report-notice + .bt-report-notice { border-top: 1px solid var(--tv-border); }
.bt-report-notice--warning { background: var(--tv-status-warning-bg); color: var(--tv-status-warning-fg); }
.bt-report-notice--error { background: var(--tv-status-error-bg); color: var(--tv-status-error-fg); }
.bt-text-running { color: var(--tv-status-success-fg); }
.bt-text-queued { color: var(--tv-status-warning-fg); }
.bt-report-window,
.bt-report-window-item,
.bt-report-chart-tab,
.bt-report-chart-stage { min-width: 0; min-height: 0; }
.bt-report-chart-tab { height: 100%; padding: var(--jf-space-1); }
.bt-report-empty { display: flex; height: 100%; min-height: 220px; align-items: center; justify-content: center; border: 1px solid var(--tv-border); border-radius: var(--jf-radius-lg); color: var(--tv-text-muted); padding: var(--jf-space-6); text-align: center; }
.bt-report-window :deep(.v-window__container),
.bt-report-window :deep(.v-window-item),
.bt-report-window :deep(.v-window-item--active) { min-width: 0; height: 100%; min-height: 0; }
.bt-report-tabs { min-height: 32px; height: 32px; max-width: 100%; min-width: 0; border-bottom: 1px solid var(--tv-border); background: var(--tv-bg-surface); }
.bt-report-tabs :deep(.v-slide-group__content) { height: 32px; }
.bt-report-tabs :deep(.v-tab) { min-height: 32px; height: 32px; padding-inline: 10px; font-size: 0.73rem; }
.bt-report-tabs :deep(.v-slide-group__container) { min-width: 0; overflow-x: auto; }

@media (max-width: 768px) {
  .bt-report-topbar,
  .bt-report-context-bar { gap: 7px; overflow-x: auto; }
  .bt-report-window { overflow: hidden; }
  .bt-report-tabs :deep(.v-tab) { min-width: 0; padding-inline: 10px; }
}
</style>
