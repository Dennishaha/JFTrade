<script setup lang="ts">
import { computed } from "vue";

import type {
  PortfolioReconciliationResponse,
  PortfolioReconciliationStatus,
} from "@/contracts";

import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";
import {
  resolvePortfolioReconciliationStatusLabel,
  resolvePortfolioReconciliationTagType,
} from "../../../composables/consoleDataFormatting";
import { formatUserMarketLabel } from "../../../composables/instrumentPresentation";
import { pricePrecisionForMarket } from "../../../composables/marketProfiles";
import {
  formatMarketPrice,
  formatMoney,
  formatNumber,
} from "../../../utils/numberFormat";

export interface AccountPositionRow {
  symbol: string;
  name: string | null;
  market: string;
  quantity: number | null;
  averagePrice: number | null | undefined;
  lastPrice: number | null | undefined;
  marketValue: number | null;
  unrealizedPnl: number | null | undefined;
  pnlRatio: number | null | undefined;
  currency: string | null | undefined;
  productClass: string | null;
  strategyType: string | null;
  positionType: string | null;
  payoutIfWin: number | null;
  source: string;
  updatedAt: string | null;
}

const props = withDefaults(
  defineProps<{
    positions: AccountPositionRow[];
    reconciliation?: PortfolioReconciliationResponse["positions"];
    emptyText?: string;
  }>(),
  {
    reconciliation: () => [],
    emptyText: "当前账户暂无持仓。",
  },
);

const showReconciliation = computed(() => props.reconciliation.length > 0);

const reconciliationByPosition = computed(() => {
  const map = new Map<
    string,
    PortfolioReconciliationResponse["positions"][number]
  >();
  for (const entry of props.reconciliation) {
    map.set(`${entry.market}|${entry.symbol}`.toUpperCase(), entry);
  }
  return map;
});

function reconciliationFor(
  position: AccountPositionRow,
): PortfolioReconciliationResponse["positions"][number] | null {
  return (
    reconciliationByPosition.value.get(
      `${position.market}|${position.symbol}`.toUpperCase(),
    ) ?? null
  );
}

function reconciliationLabel(
  position: AccountPositionRow,
): string {
  const entry = reconciliationFor(position);
  if (entry == null) return "未对账";
  return resolvePortfolioReconciliationStatusLabel(
    entry.status as PortfolioReconciliationStatus,
  );
}

function reconciliationClass(position: AccountPositionRow): string {
  const entry = reconciliationFor(position);
  if (entry == null) return "tv-status--info";
  const tag = resolvePortfolioReconciliationTagType(
    entry.status as PortfolioReconciliationStatus,
  );
  return tag === "danger" ? "tv-status--error" : `tv-status--${tag}`;
}

function formatQuantity(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 4 });
}

function formatPositionPrice(
  value: number | null | undefined,
  market: string | null | undefined,
): string {
  return formatMarketPrice(value, {
    market: market ?? null,
    precision: pricePrecisionForMarket(market),
  });
}

function formatPositionMoney(
  value: number | null | undefined,
  currency: string | null | undefined,
): string {
  return formatMoney(value, currency, { maximumFractionDigits: 4 });
}

function formatPnlRatio(value: number | null | undefined): string {
  if (value == null) return "--";
  const percent = Math.abs(value) <= 1 ? value * 100 : value;
  const sign = percent > 0 ? "+" : "";
  return `${sign}${percent.toFixed(2)}%`;
}

function pnlClass(value: number | null | undefined): string {
  if (value == null || value === 0) return "";
  return value > 0 ? "tv-up" : "tv-down";
}

function formatPositionProduct(value: string | null | undefined): string {
  switch (value) {
    case "option":
      return "期权";
    case "future":
      return "期货";
    case "event_contract":
      return "预测合约";
    case "fund":
      return "基金/信托";
    default:
      return "证券";
  }
}
</script>

<template>
  <div class="positions-table">
    <table v-if="positions.length" class="tv-table">
      <thead>
        <tr>
          <th>标的</th>
          <th>市场</th>
          <th class="tv-num">数量</th>
          <th class="tv-num">现价</th>
          <th class="tv-num">成本价</th>
          <th class="tv-num">市值</th>
          <th class="tv-num">未实现盈亏</th>
          <th class="tv-num">盈亏比例</th>
          <th>产品·组合</th>
          <th>来源</th>
          <th v-if="showReconciliation">对账</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="position in positions"
          :key="`${position.source}-${position.market}-${position.symbol}`"
        >
          <td>
            <InstrumentIdentity
              :market="position.market"
              :instrument-id="position.symbol"
              :name="position.name"
            />
          </td>
          <td>{{ formatUserMarketLabel(position.market) }}</td>
          <td class="tv-num">{{ formatQuantity(position.quantity) }}</td>
          <td class="tv-num">{{ formatPositionPrice(position.lastPrice, position.market) }}</td>
          <td class="tv-num">{{ formatPositionPrice(position.averagePrice, position.market) }}</td>
          <td class="tv-num">{{ formatPositionMoney(position.marketValue, position.currency) }}</td>
          <td class="tv-num" :class="pnlClass(position.unrealizedPnl)">
            {{ formatPositionMoney(position.unrealizedPnl, position.currency) }}
          </td>
          <td class="tv-num" :class="pnlClass(position.unrealizedPnl)">
            {{ formatPnlRatio(position.pnlRatio) }}
          </td>
          <td>
            {{ formatPositionProduct(position.productClass) }}
            <span v-if="position.strategyType" class="positions-table__dim">
              · {{ position.strategyType }}
            </span>
            <span v-if="position.positionType" class="positions-table__dim">
              · {{ position.positionType }}
            </span>
          </td>
          <td>{{ position.source }}</td>
          <td v-if="showReconciliation">
            <span class="positions-table__recon" :class="reconciliationClass(position)">
              <i class="tv-state-dot"></i>{{ reconciliationLabel(position) }}
            </span>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="positions-table__empty">{{ emptyText }}</div>
  </div>
</template>

<style scoped>
.positions-table {
  min-height: 0;
  flex: 1;
  overflow: auto;
  scrollbar-width: thin;
}

.positions-table__dim {
  color: var(--tv-text-dim);
}

.positions-table__recon {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-muted));
  font-size: 11px;
  white-space: nowrap;
}

.positions-table__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 12px;
  color: var(--tv-text-dim);
  font-size: 12px;
}
</style>
