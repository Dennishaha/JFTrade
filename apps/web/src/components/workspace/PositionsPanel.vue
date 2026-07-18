<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { ExecutionOrdersResponse } from "@/contracts";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  formatDateTime,
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatOrderSideLabel,
  formatTradingEnvironment,
  isFinalExecutionOrderStatus,
} from "../../composables/consoleDataFormatting";
import {
  formatInstrumentIdentityText,
  formatUserMarketLabel,
} from "../../composables/instrumentPresentation";
import { useConsoleData } from "../../composables/useConsoleData";
import { useNotifications } from "../../composables/useNotifications";
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import MarketStatusBadge from "../domain/market-data/MarketStatusBadge.vue";
import OptionComboConfirmDialog from "../product/OptionComboConfirmDialog.vue";

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

const props = defineProps<{
  view?: Tab;
  hideHeader?: boolean;
  orderKindFilter?: string;
  focusOrderId?: string;
}>();

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
const expandedOrderIds = ref<Set<string>>(new Set());
const pendingCancelOrder = ref<ExecutionOrder | null>(null);
const activeTab = computed<Tab>(() => props.view ?? tab.value);

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
  activeExecutionOrders.value.orders.filter(
    (order) => orderMatchesActiveScope(order) && orderMatchesKind(order),
  ),
);
const historicalExecs = computed(() =>
  historicalExecutionOrders.value.orders.filter(
    (order) => orderMatchesActiveScope(order) && orderMatchesKind(order),
  ),
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
const orderConnectivityIssue = computed(() => {
  const connectivity = brokerOrders.value.connectivity?.trim().toLowerCase() ?? "";
  if (connectivity === "connected") return null;
  if (connectivity === "disconnected") {
    return {
      state: "error" as const,
      label: "订单连接中断",
      title: "券商订单连接已中断，持仓与订单可能无法及时更新",
    };
  }
  if (connectivity === "degraded") {
    return {
      state: "stale" as const,
      label: "订单连接降级",
      title: "券商订单连接已降级，持仓与订单可能存在延迟",
    };
  }
  return null;
});

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

watch(activeTab, (newTab) => {
  if (newTab === "historical") {
    ensureHistoricalOrdersLoaded();
  }
}, { immediate: true });
watch(
  () => props.focusOrderId,
  (internalOrderId) => {
    if (!internalOrderId) return;
    expandedOrderIds.value = new Set([
      ...expandedOrderIds.value,
      internalOrderId,
    ]);
  },
  { immediate: true },
);

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

function orderMatchesKind(order: { orderKind?: string }): boolean {
  return (
    !props.orderKindFilter ||
    order.orderKind === props.orderKindFilter
  );
}

function isExpanded(internalOrderId: string): boolean {
  return expandedOrderIds.value.has(internalOrderId);
}

function toggleExpanded(internalOrderId: string): void {
  const next = new Set(expandedOrderIds.value);
  if (next.has(internalOrderId)) next.delete(internalOrderId);
  else next.add(internalOrderId);
  expandedOrderIds.value = next;
}

function requestCancelOrder(order: ExecutionOrder): void {
  if (order.orderKind === "option_combo") {
    pendingCancelOrder.value = order;
    return;
  }
  void cancelOrder(order);
}

function confirmCancelOrder(): void {
  const order = pendingCancelOrder.value;
  pendingCancelOrder.value = null;
  if (order != null) void cancelOrder(order);
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
    const cancelPath =
      order.orderKind === "option_combo"
        ? `/api/v1/execution/combos/${encodeURIComponent(order.internalOrderId)}/cancel`
        : `/api/v1/execution/orders/${encodeURIComponent(order.internalOrderId)}/cancel`;
    const result = await fetchEnvelopeWithInit<ExecutionOrderCommandResult>(
      cancelPath,
      { method: "POST" },
    );
    notifications.push({
      level: "success",
      title: `已提交撤单 ${
        order.symbol
          ? formatInstrumentIdentityText({
              market: order.market,
              instrumentId: order.symbol,
            })
          : order.internalOrderId
      }`,
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
      title: `撤单失败 ${
        order.symbol
          ? formatInstrumentIdentityText({
              market: order.market,
              instrumentId: order.symbol,
            })
          : order.internalOrderId
      }`,
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
    <div v-if="!hideHeader" class="tv-panel-head">
      <div class="tv-seg">
        <button :class="{ 'is-active': activeTab === 'positions' }" @click="tab = 'positions'">持仓{{ formatTabCount(positions.length, isPositionsLoaded) }}</button>
        <button :class="{ 'is-active': activeTab === 'active' }" @click="tab = 'active'">近期订单{{ formatTabCount(pendingExecs.length, isActiveOrdersLoaded) }}</button>
        <button :class="{ 'is-active': activeTab === 'historical' }" @click="tab = 'historical'">历史订单{{ formatTabCount(completedExecs.length, isHistoricalOrdersLoaded) }}</button>
        <button :class="{ 'is-active': activeTab === 'fills' }" @click="tab = 'fills'">事件{{ formatTabCount(events.length, isEventsLoaded) }}</button>
      </div>
      <div style="flex: 1"></div>
      <MarketStatusBadge
        v-if="orderConnectivityIssue"
        data-testid="order-connectivity-issue"
        :state="orderConnectivityIssue.state"
        :label="orderConnectivityIssue.label"
        :title="orderConnectivityIssue.title"
      />
    </div>
    <div class="tv-panel-body is-flush">
      <table v-if="activeTab === 'positions'" class="tv-table">
        <thead>
          <tr>
            <th>标的</th><th>市场</th><th>账户</th><th>环境</th>
            <th class="tv-num">数量</th><th class="tv-num">均价</th><th class="tv-num">市值</th><th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in positions" :key="`${p.brokerId}-${p.accountId}-${p.market}-${p.symbol}`">
            <td style="font-weight: 600">
              <InstrumentIdentity :market="p.market" :instrument-id="p.symbol" compact />
            </td>
            <td>{{ formatUserMarketLabel(p.market) }}</td>
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

      <table v-else-if="activeTab === 'active'" class="tv-table">
        <thead>
          <tr>
            <th>内部编号</th><th>标的</th><th>方向</th><th>状态</th>
            <th class="tv-num">委托</th><th class="tv-num">已成交</th><th class="tv-num">均价</th><th>更新时间</th><th>操作</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="o in pendingExecs" :key="o.internalOrderId">
          <tr :class="{ 'is-focused-order': o.internalOrderId === focusOrderId }">
            <td style="font-family: monospace; font-size: 11px">
              <button
                v-if="o.legs?.length"
                class="tv-order-expand"
                :aria-label="`${isExpanded(o.internalOrderId) ? '收起' : '展开'}组合腿`"
                @click="toggleExpanded(o.internalOrderId)"
              >{{ isExpanded(o.internalOrderId) ? "▾" : "▸" }}</button>
              {{ o.internalOrderId }}
            </td>
            <td><InstrumentIdentity :market="o.market" :instrument-id="o.symbol" compact /></td>
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
                @click="requestCancelOrder(o)"
              >
                {{ isCancellingOrder(o.internalOrderId) ? '撤单中...' : '撤单' }}
              </button>
            </td>
          </tr>
          <tr v-if="isExpanded(o.internalOrderId)" class="tv-order-legs">
            <td colspan="9">
              <div v-for="leg in o.legs ?? []" :key="leg.id">
                <span>#{{ leg.index + 1 }}</span>
                <strong :class="sideClass(leg.side)">{{ formatOrderSideLabel(leg.side) }} {{ leg.ratio }}</strong>
                <code>{{ leg.instrumentId }}</code>
                <span>{{ formatExecutionOrderStatusLabel(leg.status) }}</span>
                <span>成交 {{ leg.filledQuantity ?? 0 }} / {{ leg.requestedQuantity ?? "—" }}</span>
                <span>均价 {{ leg.averagePrice ?? "—" }}</span>
                <span>费用 {{ leg.fees ?? "—" }}</span>
              </div>
            </td>
          </tr>
          </template>
          <tr v-if="!isActiveOrdersLoaded">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">正在加载近期订单...</td>
          </tr>
          <tr v-else-if="pendingExecs.length === 0">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无近期订单</td>
          </tr>
        </tbody>
      </table>

      <template v-else-if="activeTab === 'historical'">
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
            <template v-for="o in completedExecs" :key="o.internalOrderId">
            <tr :class="{ 'is-focused-order': o.internalOrderId === focusOrderId }">
              <td style="font-family: monospace; font-size: 11px">
                <button
                  v-if="o.legs?.length"
                  class="tv-order-expand"
                  :aria-label="`${isExpanded(o.internalOrderId) ? '收起' : '展开'}组合腿`"
                  @click="toggleExpanded(o.internalOrderId)"
                >{{ isExpanded(o.internalOrderId) ? "▾" : "▸" }}</button>
                {{ o.internalOrderId }}
              </td>
              <td><InstrumentIdentity :market="o.market" :instrument-id="o.symbol" compact /></td>
              <td :class="sideClass(o.side)" style="font-weight: 600">{{ formatOrderSideLabel(o.side) }}</td>
              <td>{{ formatExecutionOrderStatusLabel(o.status) }}</td>
              <td class="tv-num">{{ o.requestedQuantity ?? "—" }}</td>
              <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
              <td class="tv-num">{{ o.filledAveragePrice ?? "—" }}</td>
              <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
            </tr>
            <tr v-if="isExpanded(o.internalOrderId)" class="tv-order-legs">
              <td colspan="8">
                <div v-for="leg in o.legs ?? []" :key="leg.id">
                  <span>#{{ leg.index + 1 }}</span>
                  <strong :class="sideClass(leg.side)">{{ formatOrderSideLabel(leg.side) }} {{ leg.ratio }}</strong>
                  <code>{{ leg.instrumentId }}</code>
                  <span>{{ formatExecutionOrderStatusLabel(leg.status) }}</span>
                  <span>成交 {{ leg.filledQuantity ?? 0 }} / {{ leg.requestedQuantity ?? "—" }}</span>
                  <span>均价 {{ leg.averagePrice ?? "—" }}</span>
                  <span>费用 {{ leg.fees ?? "—" }}</span>
                </div>
              </td>
            </tr>
            </template>
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
    <OptionComboConfirmDialog
      :open="pendingCancelOrder != null"
      mode="cancel"
      :account-label="pendingCancelOrder?.accountId ?? ''"
      :environment="pendingCancelOrder?.tradingEnvironment ?? ''"
      strategy-label="组合期权订单"
      :legs="[]"
      :price="pendingCancelOrder?.requestedPrice ?? 0"
      :quantity="pendingCancelOrder?.requestedQuantity ?? 0"
      :real-confirmation-required="false"
      required-confirmation-text=""
      @close="pendingCancelOrder = null"
      @confirm="confirmCancelOrder"
    />
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

.tv-order-expand {
  width: 20px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
}

.is-focused-order td {
  background: color-mix(in srgb, var(--tv-accent) 9%, transparent);
}

.tv-order-legs td {
  padding: 5px 12px 7px 28px;
  background: color-mix(in srgb, var(--tv-bg-surface-2) 82%, transparent);
}

.tv-order-legs div {
  display: grid;
  grid-template-columns: 30px 70px minmax(160px, 1fr) 100px 120px 90px 80px;
  min-height: 27px;
  align-items: center;
  gap: 8px;
  color: var(--tv-text-muted);
  font-size: 10px;
}

.tv-order-legs code {
  overflow: hidden;
  color: var(--tv-text);
  text-overflow: ellipsis;
}
</style>
