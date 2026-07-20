<script setup lang="ts">
import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";
import {
  formatDateTime,
  formatExecutionOrderSourceLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
} from "../../../composables/consoleDataFormatting";
import { formatNumber } from "../../../utils/numberFormat";
import {
  type AccountExecutionOrder,
  formatOrderKind,
  orderStatusClass,
} from "./executionOrderFormat";

export type { AccountExecutionOrder } from "./executionOrderFormat";

defineProps<{
  orders: AccountExecutionOrder[];
  selectedOrderId?: string | null;
  canCancel: (order: AccountExecutionOrder) => boolean;
  isCancelling: (internalOrderId: string) => boolean;
  emptyText?: string;
}>();

const emit = defineEmits<{
  cancel: [order: AccountExecutionOrder];
  viewEvents: [order: AccountExecutionOrder];
}>();

function statusClass(status: string): string {
  return orderStatusClass(status);
}

function formatQuantity(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 4 });
}
</script>

<template>
  <div class="active-orders">
    <table v-if="orders.length" class="tv-table">
      <thead>
        <tr>
          <th>标的</th>
          <th>方向</th>
          <th>类型</th>
          <th>来源</th>
          <th class="tv-num">数量</th>
          <th class="tv-num">已成交</th>
          <th>状态</th>
          <th>更新时间</th>
          <th class="tv-num">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="order in orders"
          :key="order.internalOrderId"
          :class="{ 'is-selected': selectedOrderId === order.internalOrderId }"
        >
          <td>
            <InstrumentIdentity
              v-if="order.symbol"
              :market="order.market"
              :instrument-id="order.symbol"
              compact
            />
            <span v-else>未知标的</span>
            <div class="active-orders__order-id">{{ order.internalOrderId }}</div>
          </td>
          <td>{{ formatOrderSideLabel(order.side) }}</td>
          <td>{{ formatOrderKind(order) }}</td>
          <td>
            <span class="active-orders__source">
              {{ formatExecutionOrderSourceLabel(order.source, order.sourceDetail) }}
            </span>
          </td>
          <td class="tv-num">{{ formatQuantity(order.requestedQuantity) }}</td>
          <td class="tv-num">{{ formatQuantity(order.filledQuantity) }}</td>
          <td>
            <span class="active-orders__status tv-status-surface" :class="statusClass(order.status)">
              {{ formatExecutionOrderStatusLabel(order.status) }}
            </span>
          </td>
          <td>{{ formatDateTime(order.updatedAt) }}</td>
          <td class="tv-num">
            <div class="active-orders__actions">
              <button
                type="button"
                class="tv-btn tv-btn-ghost active-orders__action"
                @click="emit('viewEvents', order)"
              >
                查看事件
              </button>
              <button
                type="button"
                class="tv-btn tv-btn-ghost active-orders__action active-orders__action--danger"
                :disabled="!canCancel(order)"
                @click="emit('cancel', order)"
              >
                {{ isCancelling(order.internalOrderId) ? "撤单中..." : "撤单" }}
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="active-orders__empty">
      {{ emptyText ?? "当前账户没有在途订单。" }}
    </div>
  </div>
</template>

<style scoped>
.active-orders {
  min-height: 0;
  flex: 1;
  overflow: auto;
  scrollbar-width: thin;
}

.active-orders tr.is-selected td {
  background: color-mix(in srgb, var(--tv-accent) 8%, transparent);
}

.active-orders__order-id {
  margin-top: 2px;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.active-orders__source {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.active-orders__status {
  display: inline-block;
  padding: 2px 8px;
  border: 1px solid;
  border-radius: 999px;
  font-size: 10px;
  white-space: nowrap;
}

.active-orders__actions {
  display: inline-flex;
  gap: 6px;
}

.active-orders__action {
  height: 24px;
  padding: 0 8px;
  font-size: 11px;
}

.active-orders__action--danger:not(:disabled):hover {
  border-color: var(--tv-status-error-fg);
  color: var(--tv-status-error-fg);
}

.active-orders__action:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.active-orders__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 12px;
  color: var(--tv-text-dim);
  font-size: 12px;
}
</style>
