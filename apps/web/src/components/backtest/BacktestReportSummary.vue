<script setup lang="ts">
import type { BacktestRunResult } from "@/composables/backtest/useBacktestRuns";
import { formatNumber, formatPercent } from "@/utils/numberFormat";

import {
  backtestFillCount,
  drawdownColor,
  formatBacktestFee,
  formatPercentMetric,
  pnlColor,
  pnlPrefix,
  usesClosedTradeStats,
} from "./backtestRunPresentation";

defineProps<{
  result: BacktestRunResult | undefined;
  ready: boolean;
  currency: string;
}>();
</script>

<template>
  <div class="bt-report-summary">
    <div v-if="ready && result" class="bt-report-stats-grid">
      <div class="bt-report-stat" data-testid="backtest-kpi-final-balance">
        <div class="bt-report-stat__label">最终资金</div>
        <div class="bt-report-stat__value">
          {{ formatNumber(result.finalBalance, { minimumFractionDigits: 2, maximumFractionDigits: 3 }) }}
        </div>
        <div class="bt-report-stat__meta">{{ currency }}</div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-pnl">
        <div class="bt-report-stat__label">收益</div>
        <div class="bt-report-stat__value" :class="pnlColor(result.pnl)">
          {{ pnlPrefix(result.pnl) }}{{ formatNumber(result.pnl, { minimumFractionDigits: 2, maximumFractionDigits: 3 }) }}
        </div>
        <div class="bt-report-stat__meta">{{ currency }}</div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-trades">
        <div class="bt-report-stat__label">
          {{ usesClosedTradeStats(result) ? "已平仓交易" : "历史成交数" }}
        </div>
        <div class="bt-report-stat__value">{{ result.totalTrades }}</div>
        <div v-if="usesClosedTradeStats(result)" class="bt-report-stat__meta">
          成交 {{ backtestFillCount(result) }} 笔
        </div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-win-rate">
        <div class="bt-report-stat__label">
          {{ usesClosedTradeStats(result) ? "已平仓胜率" : "历史胜率" }}
        </div>
        <div class="bt-report-stat__value">
          {{ usesClosedTradeStats(result) ? formatPercent(result.winRate, { input: "ratio", maximumFractionDigits: 1 }) : "--" }}
        </div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-max-drawdown">
        <div class="bt-report-stat__label">最大回撤</div>
        <div class="bt-report-stat__value" :class="drawdownColor(result.maxDrawdown)">
          {{ formatPercentMetric(result.maxDrawdown) }}
        </div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-current-drawdown">
        <div class="bt-report-stat__label">当前回撤</div>
        <div class="bt-report-stat__value" :class="drawdownColor(result.currentDrawdown)">
          {{ formatPercentMetric(result.currentDrawdown) }}
        </div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-broker-fees">
        <div class="bt-report-stat__label">券商费用</div>
        <div class="bt-report-stat__value">{{ formatBacktestFee(result.totalBrokerFees, currency) }}</div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-market-fees">
        <div class="bt-report-stat__label">市场费用</div>
        <div class="bt-report-stat__value">{{ formatBacktestFee(result.totalMarketFees, currency) }}</div>
      </div>
      <div class="bt-report-stat" data-testid="backtest-kpi-total-fees">
        <div class="bt-report-stat__label">总费用</div>
        <div class="bt-report-stat__value">{{ formatBacktestFee(result.totalFees, currency) }}</div>
      </div>
    </div>
    <div v-else class="bt-report-summary__empty">当前回测尚未生成完整报告。</div>
    <div v-if="result && backtestFillCount(result) === 0 && !result.error" class="bt-report-zero-trades">
      未产生任何交易。可能原因：策略未调用 placeOrder()，或订阅的K线周期未同步。
    </div>
  </div>
</template>

<style scoped>
.bt-report-summary {
  flex: 0 0 auto;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.bt-report-summary__empty {
  min-height: 52px;
  padding: var(--jf-space-4);
  color: var(--tv-text-muted);
  font-size: 0.76rem;
  text-align: center;
}

.bt-report-zero-trades {
  min-height: 28px;
  border-top: 1px solid color-mix(in srgb, var(--tv-status-warning-fg) 30%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-status-warning-fg) 8%, var(--tv-bg-surface));
  padding: 5px var(--jf-space-2);
  color: color-mix(in srgb, var(--tv-status-warning-fg) 72%, var(--tv-text));
  font-size: 0.7rem;
}

.bt-report-stats-grid {
  display: grid;
  grid-template-columns: repeat(9, minmax(0, 1fr));
  min-width: 0;
}

.bt-report-stat {
  display: grid;
  min-width: 0;
  min-height: 50px;
  align-content: center;
  gap: 2px;
  border-right: 1px solid var(--tv-border);
  padding: var(--jf-space-1) var(--jf-space-2);
}

.bt-report-stat:last-child { border-right: 0; }

.bt-report-stat__label,
.bt-report-stat__value,
.bt-report-stat__meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-report-stat__label {
  color: var(--tv-text-muted);
  font-size: 0.61rem;
  font-weight: 750;
  letter-spacing: var(--jf-tracking-3);
  text-transform: uppercase;
}

.bt-report-stat__value {
  color: var(--tv-text);
  font-size: 0.88rem;
  font-weight: 500;
  line-height: 1.15;
}

.bt-report-stat__meta {
  color: var(--tv-text-dim);
  font-size: 0.62rem;
}

.bt-metric-negative { color: var(--tv-price-down); }

@container (max-width: 900px) {
  .bt-report-stats-grid { grid-template-columns: repeat(5, minmax(0, 1fr)); }
  .bt-report-stat { border-bottom: 1px solid var(--tv-border); }
  .bt-report-stat:nth-child(5n) { border-right: 0; }
  .bt-report-stat:nth-child(n + 6) { border-bottom: 0; }
}

@container (max-width: 560px) {
  .bt-report-stats-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .bt-report-stat:nth-child(5n) { border-right: 1px solid var(--tv-border); }
  .bt-report-stat:nth-child(3n) { border-right: 0; }
  .bt-report-stat:nth-child(n + 6) { border-bottom: 1px solid var(--tv-border); }
  .bt-report-stat:nth-child(n + 7) { border-bottom: 0; }
}

@container (max-width: 360px) {
  .bt-report-stats-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .bt-report-stat:nth-child(3n) { border-right: 1px solid var(--tv-border); }
  .bt-report-stat:nth-child(2n) { border-right: 0; }
  .bt-report-stat:nth-child(n + 7) { border-bottom: 1px solid var(--tv-border); }
  .bt-report-stat:nth-child(n + 9) { border-bottom: 0; }
}
</style>
