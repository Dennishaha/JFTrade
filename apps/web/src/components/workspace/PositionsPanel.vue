<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ExecutionOrdersResponse } from "@/contracts";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  formatConnectivityLabel,
  formatDateTime,
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatMarketLabel,
  formatOrderSideLabel,
  formatTradingEnvironment,
  isFinalExecutionOrderStatus,
} from "../../composables/consoleDataFormatting";
import { useConsoleData } from "../../composables/useConsoleData";
import { useNotifications } from "../../composables/useNotifications";

type Tab = "positions" | "active" | "historical" | "fills";

type ExecutionOrder = ExecutionOrdersResponse["orders"][number];

interface ExecutionOrderCommandResult {
  accepted: boolean;
  operation: string;
  internalOrderId?: string | null;
  brokerOrderId?: string | null;
  brokerOrderIdEx?: string | null;
  orderStatus?: string | null;
  brokerErrorCode?: string | null;
  message: string;
  checkedAt: string;
}

const {
  brokerOrders,
  activeExecutionOrders,
  historicalExecutionOrders,
  executionOrderEvents,
  isLoadingBrokerOrders,
  isLoadingHistoricalOrders,
  historicalOrdersError,
  portfolioPositions,
  selectedBrokerAccount,
  loadHistoricalExecutionOrders,
  systemStatus,
} = useConsoleData();
const notifications = useNotifications();

const tab = ref<Tab>("positions");
const cancellingOrderIds = ref<Set<string>>(new Set());
const hasLoadedHistoricalOrders = ref(false);

const positions = computed(() => portfolioPositions.value.positions);
const activeOrderScope = computed(() => {
  const selected = selectedBrokerAccount.value;
  return {
    accountId: selected?.accountId ?? "",
    tradingEnvironment:
      selected?.tradingEnvironment ??
      systemStatus.value.defaultTradingEnvironment,
    market: selected?.market ?? "",
  };
});
const activeExecs = computed(() =>
  activeExecutionOrders.value.orders.filter((order) => orderMatchesActiveScope(order)),
);
const historicalExecs = computed(() =>
  historicalExecutionOrders.value.orders.filter((order) => orderMatchesActiveScope(order)),
);
const pendingExecs = computed(() =>
  activeExecs.value.filter((order) => !isFinalExecutionOrderStatus(order.status)),
);
const completedExecs = computed(() =>
  historicalExecs.value.filter((order) => isFinalExecutionOrderStatus(order.status)),
);
const activeExecutionOrderIds = computed(
  () => new Set(activeExecs.value.map((order) => order.internalOrderId)),
);
const events = computed(() =>
  executionOrderEvents.value.events.filter((event) =>
    activeExecutionOrderIds.value.has(event.internalOrderId),
  ),
);

const isPositionsLoaded = computed(() => !isLoadingBrokerOrders.value);
const isActiveOrdersLoaded = computed(() => !isLoadingBrokerOrders.value);
const isHistoricalOrdersLoaded = computed(() => hasLoadedHistoricalOrders.value && !isLoadingHistoricalOrders.value);
const isEventsLoaded = computed(() => !isLoadingBrokerOrders.value);

function formatTabCount(count: number, loaded: boolean): string {
  return loaded ? `（${count}）` : "";
}

function ensureHistoricalOrdersLoaded(): void {
  if (hasLoadedHistoricalOrders.value) return;
  hasLoadedHistoricalOrders.value = true;
  const selected = selectedBrokerAccount.value;
  if (selected == null) return;
  const params = new URLSearchParams();
  params.set("brokerId", selected.brokerId);
  params.set("tradingEnvironment", selected.tradingEnvironment);
  if (selected.accountId) params.set("accountId", selected.accountId);
  if (selected.market) params.set("market", selected.market);
  void loadHistoricalExecutionOrders({
    brokerId: selected.brokerId,
    brokerQuery: params.toString(),
  });
}

watch(tab, (newTab) => {
  if (newTab === "historical") {
    ensureHistoricalOrdersLoaded();
  }
});

function sideClass(side: string | null | undefined): string {
  if (!side) return "";
  return side.toUpperCase().includes("SELL") ? "tv-down" : "tv-up";
}

function orderMatchesActiveScope(order: {
  accountId: string;
  tradingEnvironment: string;
  market: string;
}): boolean {
  const scope = activeOrderScope.value;
  if (
    order.tradingEnvironment.trim().toUpperCase() !==
    scope.tradingEnvironment.trim().toUpperCase()
  ) {
    return false;
  }
  if (scope.accountId !== "" && order.accountId !== scope.accountId) {
    return false;
  }
  if (
    scope.market !== "" &&
    order.market.trim().toUpperCase() !== scope.market.trim().toUpperCase()
  ) {
    return false;
  }
  return true;
}

function isCancellingOrder(internalOrderId: string): boolean {
  return cancellingOrderIds.value.has(internalOrderId);
}

function canCancelOrder(order: ExecutionOrder): boolean {
  if (isFinalExecutionOrderStatus(order.status)) {
    return false;
  }
  if (isCancellingOrder(order.internalOrderId)) {
    return false;
  }
  const normalized = order.status.trim().toUpperCase();
  if (
    normalized === "CANCELING" ||
    normalized === "PENDING_CANCEL" ||
    normalized === "CANCEL_REQUESTED"
  ) {
    return false;
  }
  return true;
}

async function cancelOrder(order: ExecutionOrder): Promise<void> {
  if (!canCancelOrder(order)) {
    return;
  }

  const nextCancelling = new Set(cancellingOrderIds.value);
  nextCancelling.add(order.internalOrderId);
  cancellingOrderIds.value = nextCancelling;

  try {
    const result = await fetchEnvelopeWithInit<ExecutionOrderCommandResult>(
      `/api/v1/execution/orders/${encodeURIComponent(order.internalOrderId)}/cancel`,
      { method: "POST" },
    );
    notifications.push({
      level: "success",
      title: `已提交撤单 ${order.symbol ?? order.internalOrderId}`,
      message: result.message,
      source: "positions-panel",
    });
  } catch (error) {
    const message =
      error instanceof Error && error.message.trim() !== ""
        ? error.message
        : "撤单请求失败。";
    notifications.push({
      level: "error",
      title: `撤单失败 ${order.symbol ?? order.internalOrderId}`,
      message,
      source: "positions-panel",
    });
  } finally {
    const nextCancellingDone = new Set(cancellingOrderIds.value);
    nextCancellingDone.delete(order.internalOrderId);
    cancellingOrderIds.value = nextCancellingDone;
  }
}
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <div class="tv-seg">
        <button :class="{ 'is-active': tab === 'positions' }" @click="tab = 'positions'">持仓{{ formatTabCount(positions.length, isPositionsLoaded) }}</button>
        <button :class="{ 'is-active': tab === 'active' }" @click="tab = 'active'">近期订单{{ formatTabCount(pendingExecs.length, isActiveOrdersLoaded) }}</button>
        <button :class="{ 'is-active': tab === 'historical' }" @click="tab = 'historical'">历史订单{{ formatTabCount(completedExecs.length, isHistoricalOrdersLoaded) }}</button>
        <button :class="{ 'is-active': tab === 'fills' }" @click="tab = 'fills'">事件{{ formatTabCount(events.length, isEventsLoaded) }}</button>
      </div>
      <div style="flex: 1"></div>
      <span style="color: var(--tv-text-dim); font-size: 11px">{{ formatConnectivityLabel(brokerOrders.connectivity) }}</span>
    </div>
    <div class="tv-panel-body is-flush">
      <table v-if="tab === 'positions'" class="tv-table">
        <thead>
          <tr>
            <th>标的</th><th>市场</th><th>账户</th><th>环境</th>
            <th class="tv-num">数量</th><th class="tv-num">均价</th><th class="tv-num">市值</th><th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in positions" :key="`${p.brokerId}-${p.accountId}-${p.market}-${p.symbol}`">
            <td style="font-weight: 600">{{ p.symbol }}</td>
            <td>{{ formatMarketLabel(p.market) }}</td>
            <td style="color: var(--tv-text-muted)">{{ p.accountId }}</td>
            <td>{{ formatTradingEnvironment(p.tradingEnvironment) }}</td>
            <td class="tv-num" :class="p.quantity >= 0 ? 'tv-up' : 'tv-down'">{{ p.quantity }}</td>
            <td class="tv-num">{{ p.averagePrice }}</td>
            <td class="tv-num">{{ p.marketValue }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(p.updatedAt) }}</td>
          </tr>
          <tr v-if="!isPositionsLoaded">
            <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">正在加载持仓...</td>
          </tr>
          <tr v-else-if="positions.length === 0">
            <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无持仓</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="tab === 'active'" class="tv-table">
        <thead>
          <tr>
            <th>内部编号</th><th>标的</th><th>方向</th><th>状态</th>
            <th class="tv-num">委托</th><th class="tv-num">已成交</th><th class="tv-num">均价</th><th>更新时间</th><th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in pendingExecs" :key="o.internalOrderId">
            <td style="font-family: monospace; font-size: 11px">{{ o.internalOrderId }}</td>
            <td>{{ o.market }}:{{ o.symbol }}</td>
            <td :class="sideClass(o.side)" style="font-weight: 600">{{ formatOrderSideLabel(o.side) }}</td>
            <td>{{ formatExecutionOrderStatusLabel(o.status) }}</td>
            <td class="tv-num">{{ o.requestedQuantity ?? "—" }}</td>
            <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
            <td class="tv-num">{{ o.filledAveragePrice ?? "—" }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
            <td>
              <button
                v-if="canCancelOrder(o)"
                class="tv-btn-cancel"
                :disabled="isCancellingOrder(o.internalOrderId)"
                @click="cancelOrder(o)"
              >
                {{ isCancellingOrder(o.internalOrderId) ? '撤单中...' : '撤单' }}
              </button>
            </td>
          </tr>
          <tr v-if="!isActiveOrdersLoaded">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">正在加载近期订单...</td>
          </tr>
          <tr v-else-if="pendingExecs.length === 0">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无近期订单</td>
          </tr>
        </tbody>
      </table>

      <template v-else-if="tab === 'historical'">
        <div v-if="isLoadingHistoricalOrders && !hasLoadedHistoricalOrders" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">
          正在加载历史订单...
        </div>
        <v-alert
          v-else-if="historicalOrdersError"
          type="warning"
          :closable="false"
          density="compact"
          style="margin: 8px"
        >
          {{ historicalOrdersError }}
        </v-alert>
        <table v-else class="tv-table">
          <thead>
            <tr>
              <th>内部编号</th><th>标的</th><th>方向</th><th>状态</th>
              <th class="tv-num">委托</th><th class="tv-num">已成交</th><th class="tv-num">均价</th><th>更新时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="o in completedExecs" :key="o.internalOrderId">
              <td style="font-family: monospace; font-size: 11px">{{ o.internalOrderId }}</td>
              <td>{{ o.market }}:{{ o.symbol }}</td>
              <td :class="sideClass(o.side)" style="font-weight: 600">{{ formatOrderSideLabel(o.side) }}</td>
              <td>{{ formatExecutionOrderStatusLabel(o.status) }}</td>
              <td class="tv-num">{{ o.requestedQuantity ?? "—" }}</td>
              <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
              <td class="tv-num">{{ o.filledAveragePrice ?? "—" }}</td>
              <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
            </tr>
            <tr v-if="isLoadingHistoricalOrders">
              <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">正在加载历史订单...</td>
            </tr>
            <tr v-else-if="completedExecs.length === 0">
              <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无历史订单</td>
            </tr>
          </tbody>
        </table>
      </template>

      <table v-else class="tv-table">
        <thead>
          <tr><th>事件</th><th>前状态</th><th>后状态</th><th>订单</th><th>时间</th></tr>
        </thead>
        <tbody>
          <tr v-for="ev in events" :key="ev.id">
            <td style="font-weight: 600">{{ formatExecutionEventTypeLabel(ev.eventType) }}</td>
            <td style="color: var(--tv-text-muted)">{{ formatExecutionOrderStatusLabel(ev.previousStatus) }}</td>
            <td>{{ formatExecutionOrderStatusLabel(ev.nextStatus) }}</td>
            <td style="font-family: monospace; font-size: 11px">{{ ev.internalOrderId }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(ev.createdAt) }}</td>
          </tr>
          <tr v-if="!isEventsLoaded">
            <td colspan="5" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">正在加载订单事件...</td>
          </tr>
          <tr v-else-if="events.length === 0">
            <td colspan="5" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无事件</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.tv-btn-cancel {
  padding: 2px 10px;
  font-size: 11px;
  color: var(--tv-text-dim);
  background: transparent;
  border: 1px solid var(--tv-text-dim);
  border-radius: 3px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s, background 0.15s;
}
.tv-btn-cancel:hover:not(:disabled) {
  color: #e53e3e;
  border-color: #e53e3e;
}
.tv-btn-cancel:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
