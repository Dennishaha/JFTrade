<script setup lang="ts">
import { computed } from "vue";

import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";
import {
  formatDateTime,
  formatExecutionEventTypeLabel,
  formatExecutionOrderSourceLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
} from "../../../composables/consoleDataFormatting";
import { pricePrecisionForMarket } from "../../../composables/marketProfiles";
import { useConsoleData } from "../../../composables/useConsoleData";
import {
  formatMarketPrice,
  formatMoney,
  formatNumber,
} from "../../../utils/numberFormat";
import {
  type AccountExecutionOrder,
  formatExecutionStatusTransition,
  formatOrderKind,
  orderStatusClass,
} from "./executionOrderFormat";

const props = defineProps<{
  orders: AccountExecutionOrder[];
  totalCount: number;
  isLoading: boolean;
  error: string;
  hasMore: boolean;
  selectedOrderId?: string | null;
}>();

const emit = defineEmits<{
  select: [internalOrderId: string];
  loadMore: [];
}>();

const {
  brokerFills,
  brokerFunds,
  brokerOrderFees,
  executionEventsError,
  executionOrderEvents,
  isLoadingBrokerFills,
  isLoadingExecutionEvents,
  isLoadingOrderFees,
  orderFeesError,
  resolveBrokerReadFeatureQueryRequirements,
  selectedExecutionOrder,
  supportsBrokerReadFeature,
} = useConsoleData();

const recentBrokerFills = computed(() => brokerFills.value.fills.slice(0, 12));

const brokerFillsRequirements = computed(() =>
  resolveBrokerReadFeatureQueryRequirements("fills", {
    market: selectedExecutionOrder.value?.market ?? null,
    tradingEnvironment: selectedExecutionOrder.value?.tradingEnvironment ?? null,
  }),
);

const brokerFillsDescription = computed(() =>
  brokerFillsRequirements.value.supportsHistory
    ? "展示当前账户最近 30 天的券商成交记录。"
    : "当前券商未声明历史成交能力，展示当前刷新窗口内可见的成交记录。",
);

const supportsSelectedExecutionOrderFees = computed(() => {
  const order = selectedExecutionOrder.value;
  if (order == null) {
    return false;
  }
  return supportsBrokerReadFeature("orderFees", {
    market: order.market,
    tradingEnvironment: order.tradingEnvironment,
  });
});

const hasSelectedOrderSummary = computed(() => {
  const order = selectedExecutionOrder.value;
  if (order == null) return false;
  return (
    (order.legs?.length ?? 0) > 0 ||
    order.requestedAmount != null ||
    order.payout != null
  );
});

function formatQuantity(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 4 });
}

function formatFillPrice(
  value: number | null | undefined,
  market: string | null | undefined,
): string {
  return formatMarketPrice(value, {
    market: market ?? null,
    precision: pricePrecisionForMarket(market),
  });
}

function formatFeeMoney(value: number | null | undefined): string {
  return formatMoney(value, brokerFunds.value.summary?.currency, {
    maximumFractionDigits: 4,
  });
}
</script>

<template>
  <div class="order-history">
    <section class="order-history__list">
      <header class="order-history__list-head">
        <span class="tv-panel-title">历史订单</span>
        <span class="order-history__count">{{ props.totalCount }} 笔</span>
      </header>

      <div class="order-history__list-body">
        <div v-if="isLoading && orders.length === 0" class="order-history__hint">
          正在加载历史订单...
        </div>
        <div
          v-else-if="error"
          class="order-history__error tv-status--warning tv-status-surface"
        >
          {{ error }}
        </div>
        <template v-else-if="orders.length">
          <button
            v-for="order in orders"
            :key="order.internalOrderId"
            type="button"
            class="order-history__item"
            :class="{ 'is-active': selectedOrderId === order.internalOrderId }"
            @click="emit('select', order.internalOrderId)"
          >
            <div class="order-history__item-head">
              <InstrumentIdentity
                v-if="order.symbol"
                :market="order.market"
                :instrument-id="order.symbol"
                compact
              />
              <span v-else>未知标的</span>
              <span
                class="order-history__status tv-status-surface"
                :class="orderStatusClass(order.status)"
              >
                {{ formatExecutionOrderStatusLabel(order.status) }}
              </span>
            </div>
            <div class="order-history__item-meta">
              <span>{{ formatOrderSideLabel(order.side) }}</span>
              <span>{{ formatOrderKind(order) }}</span>
              <span>{{ formatExecutionOrderSourceLabel(order.source, order.sourceDetail) }}</span>
              <span>数量 {{ formatQuantity(order.requestedQuantity) }}</span>
              <span>成交 {{ formatQuantity(order.filledQuantity) }}</span>
            </div>
            <div class="order-history__item-time">{{ formatDateTime(order.updatedAt) }}</div>
          </button>

          <div v-if="hasMore" class="order-history__more">
            <button type="button" class="tv-btn tv-btn-ghost" @click="emit('loadMore')">
              加载更多（已显示 {{ orders.length }} / {{ totalCount }}）
            </button>
          </div>
        </template>
        <div v-else-if="isLoading" class="order-history__hint">正在加载历史订单...</div>
        <div v-else class="order-history__hint">当前账户暂无历史订单。</div>
      </div>
    </section>

    <section class="order-history__detail">
      <header class="order-history__detail-head">
        <span class="tv-panel-title">订单事件与费用</span>
        <span class="order-history__count" :title="selectedExecutionOrder?.internalOrderId ?? ''">
          {{ selectedExecutionOrder?.internalOrderId ?? "请选择一笔订单" }}
        </span>
      </header>

      <div class="order-history__detail-body">
        <div
          v-if="selectedExecutionOrder?.rawBrokerStatus"
          class="order-history__raw-status"
        >
          券商原始状态：{{ selectedExecutionOrder.rawBrokerStatus }}
        </div>

        <template v-if="hasSelectedOrderSummary && selectedExecutionOrder">
          <div class="order-history__chips">
            <span class="order-history__chip">{{ formatOrderKind(selectedExecutionOrder) }}</span>
            <span v-if="selectedExecutionOrder.requestedAmount != null" class="order-history__chip">
              投入 {{ formatQuantity(selectedExecutionOrder.requestedAmount) }}
            </span>
            <span
              v-if="selectedExecutionOrder.payout != null"
              class="order-history__chip tv-status--success tv-status-surface"
            >
              Payout {{ formatQuantity(selectedExecutionOrder.payout) }}
            </span>
          </div>
          <div
            v-for="leg in selectedExecutionOrder.legs ?? []"
            :key="leg.id"
            class="order-history__leg"
          >
            <span class="order-history__leg-id">{{ leg.instrumentId }}</span>
            <span>{{ formatOrderSideLabel(leg.side) }}</span>
            <span>× {{ leg.ratio }}</span>
            <span>{{ leg.predictionSide || formatQuantity(leg.requestedQuantity) }}</span>
            <span
              class="order-history__status tv-status-surface"
              :class="orderStatusClass(leg.status)"
            >
              {{ formatExecutionOrderStatusLabel(leg.status) }}
            </span>
          </div>
        </template>

        <div v-if="isLoadingExecutionEvents" class="order-history__hint">
          正在加载订单事件...
        </div>
        <div
          v-else-if="executionEventsError"
          class="order-history__error tv-status--warning tv-status-surface"
        >
          {{ executionEventsError }}
        </div>
        <template v-else-if="executionOrderEvents.events.length">
          <div
            v-for="event in executionOrderEvents.events"
            :key="event.id"
            class="order-history__event"
          >
            <div class="order-history__event-head">
              <b>{{ formatExecutionEventTypeLabel(event.eventType) }}</b>
              <span>{{ formatDateTime(event.createdAt) }}</span>
            </div>
            <div class="order-history__event-body">
              {{ formatExecutionStatusTransition(event.previousStatus, event.nextStatus) }}
            </div>
          </div>
        </template>
        <div v-else class="order-history__hint">当前订单暂无事件。</div>

        <div class="order-history__sub-head">
          <span class="tv-panel-title">券商费用</span>
          <span class="order-history__count">
            {{ selectedExecutionOrder?.brokerOrderIdEx ?? selectedExecutionOrder?.brokerOrderId ?? "暂无券商订单号" }}
            · {{ brokerOrderFees.fees.length }} 条
          </span>
        </div>
        <div
          v-if="selectedExecutionOrder != null && !supportsSelectedExecutionOrderFees"
          class="order-history__hint"
        >
          当前券商未为该交易环境声明费用查询能力。
        </div>
        <div v-else-if="isLoadingOrderFees" class="order-history__hint">
          正在加载券商费用...
        </div>
        <div
          v-else-if="orderFeesError"
          class="order-history__error tv-status--warning tv-status-surface"
        >
          {{ orderFeesError }}
        </div>
        <template v-else-if="brokerOrderFees.fees.length">
          <div
            v-for="fee in brokerOrderFees.fees"
            :key="fee.brokerOrderIdEx"
            class="order-history__event"
          >
            <div class="order-history__event-head">
              <b>{{ fee.brokerOrderIdEx }}</b>
              <span class="tv-num">{{ formatFeeMoney(fee.feeAmount) }}</span>
            </div>
            <div v-if="fee.feeItems?.length" class="order-history__chips">
              <span
                v-for="detail in fee.feeItems"
                :key="`${fee.brokerOrderIdEx}-${detail.title}`"
                class="order-history__chip"
              >
                {{ detail.title }}：{{ formatFeeMoney(detail.value) }}
              </span>
            </div>
          </div>
        </template>
        <div v-else class="order-history__hint">当前订单暂无券商费用。</div>

        <div class="order-history__sub-head">
          <span class="tv-panel-title">最近成交</span>
          <span class="order-history__count">{{ recentBrokerFills.length }} 条</span>
        </div>
        <div class="order-history__hint order-history__hint--desc">
          {{ brokerFillsDescription }}
        </div>
        <div v-if="isLoadingBrokerFills" class="order-history__hint">
          正在加载券商成交...
        </div>
        <div
          v-else-if="brokerFills.lastError"
          class="order-history__error tv-status--warning tv-status-surface"
        >
          {{ brokerFills.lastError }}
        </div>
        <div v-else-if="recentBrokerFills.length" class="order-history__fills">
          <table class="tv-table">
            <thead>
              <tr>
                <th>成交号</th>
                <th>标的</th>
                <th>方向</th>
                <th class="tv-num">数量</th>
                <th class="tv-num">价格</th>
                <th>状态</th>
                <th>成交时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="fill in recentBrokerFills" :key="fill.brokerFillId">
                <td class="order-history__mono">{{ fill.brokerFillIdEx ?? fill.brokerFillId }}</td>
                <td>
                  <InstrumentIdentity
                    :market="fill.market"
                    :instrument-id="fill.symbol"
                    :name="fill.symbolName"
                  />
                </td>
                <td>{{ formatOrderSideLabel(fill.side) }}</td>
                <td class="tv-num">{{ formatQuantity(fill.filledQuantity) }}</td>
                <td class="tv-num">{{ formatFillPrice(fill.fillPrice, fill.market) }}</td>
                <td>{{ fill.status ?? "—" }}</td>
                <td>{{ formatDateTime(fill.filledAt) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="order-history__hint">当前账户暂无券商成交。</div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.order-history {
  display: grid;
  min-height: 0;
  flex: 1;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 0.95fr);
  overflow: hidden;
}

.order-history__list {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  border-right: 1px solid var(--tv-border);
}

.order-history__list-head,
.order-history__detail-head,
.order-history__sub-head {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.order-history__sub-head {
  margin-top: 14px;
  border-top: 1px solid var(--tv-border);
}

.tv-panel-title {
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.order-history__count {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.order-history__list-body {
  display: grid;
  flex: 1;
  align-content: start;
  gap: 8px;
  overflow: auto;
  padding: 10px;
  scrollbar-width: thin;
}

.order-history__item {
  display: block;
  width: 100%;
  padding: 9px 11px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  text-align: left;
}

.order-history__item:hover {
  border-color: var(--tv-accent);
}

.order-history__item.is-active {
  border-color: var(--tv-accent);
  background: color-mix(in srgb, var(--tv-accent) 8%, var(--tv-bg-surface-2));
}

.order-history__item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.order-history__item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 6px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.order-history__item-time {
  margin-top: 4px;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.order-history__status {
  display: inline-block;
  flex: 0 0 auto;
  padding: 2px 8px;
  border: 1px solid;
  border-radius: 999px;
  font-size: 10px;
  white-space: nowrap;
}

.order-history__more {
  display: flex;
  justify-content: center;
  padding-top: 2px;
}

.order-history__detail {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.order-history__detail-body {
  flex: 1;
  overflow: auto;
  padding: 10px 12px 14px;
  scrollbar-width: thin;
}

.order-history__raw-status {
  margin-bottom: 8px;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.order-history__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.order-history__chip {
  padding: 2px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.order-history__leg {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) 64px 48px 80px auto;
  gap: 8px;
  align-items: center;
  margin-bottom: 6px;
  padding: 7px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
  font-size: 11px;
}

.order-history__leg-id {
  overflow: hidden;
  color: var(--tv-text);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.order-history__event {
  margin-bottom: 8px;
  padding: 8px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.order-history__event-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
}

.order-history__event-head b {
  color: var(--tv-text);
  font-weight: 600;
}

.order-history__event-head span {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.order-history__event-body {
  margin-top: 4px;
  color: var(--tv-text-muted);
  font-size: 11px;
}

.order-history__hint {
  padding: 14px 2px;
  color: var(--tv-text-dim);
  font-size: 11px;
}

.order-history__hint--desc {
  padding: 6px 2px 0;
  font-size: 10px;
}

.order-history__error {
  margin-bottom: 8px;
  padding: 8px 10px;
  border: 1px solid;
  border-radius: 6px;
  font-size: 11px;
}

.order-history__fills {
  margin-top: 8px;
  overflow-x: auto;
}

.order-history__mono {
  font-family: ui-monospace, monospace;
  font-size: 10px;
}

@media (max-width: 1180px) {
  .order-history {
    grid-template-columns: minmax(0, 1fr);
    overflow: auto;
  }

  .order-history__list {
    border-right: 0;
    border-bottom: 1px solid var(--tv-border);
  }
}
</style>
