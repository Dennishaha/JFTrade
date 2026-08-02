<script setup lang="ts">
import type { BacktestOrderBookEntry } from "@/composables/backtest/useBacktestRuns";

import {
  formatBacktestFee,
  formatBacktestOrderPrice,
  formatBacktestOrderSide,
  formatBacktestOrderStatus,
  formatBacktestQuantity,
  formatBacktestTimestamp,
} from "./backtestRunPresentation";

const RENDER_WINDOW = 200;
const props = defineProps<{ entries: BacktestOrderBookEntry[] }>();

function visibleEntries(): BacktestOrderBookEntry[] {
  return props.entries.slice(0, RENDER_WINDOW);
}

function hiddenEntryCount(): number {
  return Math.max(0, props.entries.length - RENDER_WINDOW);
}
</script>

<template>
  <div class="h-full min-h-0 overflow-auto p-2">
    <div v-if="entries.length" class="bt-order-card">
      <div class="bt-order-card__header">
        <span>订单</span>
        <span>{{ entries.length }} 笔</span>
      </div>
      <div class="bt-order-table-scroll max-h-[520px] overflow-auto">
        <table class="bt-order-table min-w-full text-sm">
          <thead class="sticky top-0 text-left text-xs uppercase tracking-(--jf-tracking-1)">
            <tr>
              <th>下单</th><th>成交</th><th>方向</th><th>数量</th>
              <th>委托价</th><th>成交价</th><th>费用</th><th>状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(entry, index) in visibleEntries()"
              :key="`${entry.orderId || index}-${entry.filledAt ?? entry.submittedAt ?? ''}`">
              <td>
                <div>{{ formatBacktestTimestamp(entry.submittedAt) }}</div>
                <div class="bt-order-table__meta">
                  #{{ entry.orderId }}<span v-if="entry.clientOrderId"> · {{ entry.clientOrderId }}</span>
                </div>
              </td>
              <td>{{ formatBacktestTimestamp(entry.filledAt) }}</td>
              <td>{{ formatBacktestOrderSide(entry.side) }}</td>
              <td>
                <div>{{ formatBacktestQuantity(entry.quantity, entry.quantityText) }}</div>
                <div v-if="entry.filledQuantity !== undefined" class="bt-order-table__meta">
                  成交 {{ formatBacktestQuantity(entry.filledQuantity, entry.filledQuantityText) }}
                </div>
              </td>
              <td>{{ formatBacktestOrderPrice(entry.orderPrice, entry.orderType, entry.orderPriceText) }}</td>
              <td>{{ formatBacktestOrderPrice(entry.filledPrice, undefined, entry.filledPriceText) }}</td>
              <td>
                <div>{{ formatBacktestFee(entry.totalFee, entry.feeCurrency) }}</div>
                <div v-if="entry.totalFee" class="bt-order-table__meta">
                  券商 {{ formatBacktestFee(entry.brokerFee, entry.feeCurrency) }} ｜
                  市场 {{ formatBacktestFee(entry.marketFee, entry.feeCurrency) }}
                </div>
              </td>
              <td>
                {{ formatBacktestOrderStatus(entry.status) }}
                <span v-if="entry.warmup" class="bt-warmup-label">预热</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="hiddenEntryCount() > 0" class="bt-order-card__more">
        另有 {{ hiddenEntryCount() }} 笔订单。
      </div>
    </div>
    <div v-else class="bt-order-empty">暂无订单记录。</div>
  </div>
</template>

<style scoped>
.bt-order-card,
.bt-order-empty {
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-lg);
}

.bt-order-card { overflow: hidden; }

.bt-order-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--jf-space-2) var(--jf-space-3);
  border-bottom: 1px solid var(--tv-border);
  color: var(--tv-text);
  font-size: 0.875rem;
  font-weight: 650;
}

.bt-order-card__header span:last-child,
.bt-order-table__meta,
.bt-order-card__more { color: var(--tv-text-muted); font-size: 0.75rem; }

.bt-order-card__more { padding: var(--jf-space-2) var(--jf-space-4); border-top: 1px solid var(--tv-border); }
.bt-order-empty { padding: var(--jf-space-6); color: var(--tv-text-muted); text-align: center; }
.bt-order-table-scroll { max-width: 100%; min-width: 0; overscroll-behavior: contain; }
.bt-order-table { min-width: 48rem; border-collapse: collapse; }
.bt-order-table thead { background: var(--tv-bg-surface-2); color: var(--tv-text-muted); }
.bt-order-table tbody { background: var(--tv-bg-surface); color: var(--tv-text); }
.bt-order-table th,
.bt-order-table td { padding: 6px var(--jf-space-3); vertical-align: top; border-bottom: 1px solid var(--tv-border); }
.bt-order-table th { font-weight: 500; }
.bt-warmup-label { margin-left: var(--jf-space-1); color: var(--tv-status-warning-fg); font-size: 0.72rem; }

@media (max-width: 768px) {
  .bt-order-table { min-width: 42rem; }
}
</style>
