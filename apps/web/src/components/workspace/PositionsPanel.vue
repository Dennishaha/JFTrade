<script setup lang="ts">
import { computed, ref } from "vue";

import {
  formatConnectivityLabel,
  formatDateTime,
  formatExecutionEventTypeLabel,
  formatExecutionOrderStatusLabel,
  formatMarketLabel,
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTradingEnvironment,
} from "../../composables/consoleDataFormatting";
import { useConsoleData } from "../../composables/useConsoleData";

type Tab = "positions" | "orders" | "fills" | "executions";

const {
  brokerOrders,
  executionOrders,
  executionOrderEvents,
  portfolioPositions,
} = useConsoleData();

const tab = ref<Tab>("positions");

const positions = computed(() => portfolioPositions.value.positions);
const orders = computed(() => brokerOrders.value.orders);
const execs = computed(() => executionOrders.value.orders);
const events = computed(() => executionOrderEvents.value.events);

function sideClass(side: string | null | undefined): string {
  if (!side) return "";
  return side.toUpperCase().includes("SELL") ? "tv-down" : "tv-up";
}
</script>

<template>
  <section class="tv-panel">
    <div class="tv-panel-head">
      <div class="tv-seg">
        <button :class="{ 'is-active': tab === 'positions' }" @click="tab = 'positions'">持仓（{{ positions.length }}）</button>
        <button :class="{ 'is-active': tab === 'orders' }" @click="tab = 'orders'">券商订单（{{ orders.length }}）</button>
        <button :class="{ 'is-active': tab === 'executions' }" @click="tab = 'executions'">执行订单（{{ execs.length }}）</button>
        <button :class="{ 'is-active': tab === 'fills' }" @click="tab = 'fills'">事件（{{ events.length }}）</button>
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
          <tr v-if="positions.length === 0">
            <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无持仓</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="tab === 'orders'" class="tv-table">
        <thead>
          <tr>
            <th>订单号</th><th>标的</th><th>方向</th><th>类型</th><th>状态</th>
            <th class="tv-num">数量</th><th class="tv-num">已成交</th><th class="tv-num">价格</th><th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in orders" :key="o.brokerOrderId">
            <td style="font-family: monospace; font-size: 11px">{{ o.brokerOrderId }}</td>
            <td>{{ o.market }}:{{ o.symbol }}</td>
            <td :class="sideClass(o.side)" style="font-weight: 600">{{ formatOrderSideLabel(o.side) }}</td>
            <td>{{ formatOrderTypeLabel(o.orderType) }}</td>
            <td>{{ formatExecutionOrderStatusLabel(o.status) }}</td>
            <td class="tv-num">{{ o.quantity }}</td>
            <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
            <td class="tv-num">{{ o.price ?? "—" }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
          </tr>
          <tr v-if="orders.length === 0">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无券商订单</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="tab === 'executions'" class="tv-table">
        <thead>
          <tr>
            <th>内部编号</th><th>标的</th><th>方向</th><th>状态</th>
            <th class="tv-num">委托</th><th class="tv-num">已成交</th><th class="tv-num">均价</th><th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in execs" :key="o.internalOrderId">
            <td style="font-family: monospace; font-size: 11px">{{ o.internalOrderId }}</td>
            <td>{{ o.market }}:{{ o.symbol }}</td>
            <td :class="sideClass(o.side)" style="font-weight: 600">{{ formatOrderSideLabel(o.side) }}</td>
            <td>{{ formatExecutionOrderStatusLabel(o.status) }}</td>
            <td class="tv-num">{{ o.requestedQuantity ?? "—" }}</td>
            <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
            <td class="tv-num">{{ o.filledAveragePrice ?? "—" }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
          </tr>
          <tr v-if="execs.length === 0">
            <td colspan="8" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无执行订单</td>
          </tr>
        </tbody>
      </table>

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
          <tr v-if="events.length === 0">
            <td colspan="5" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无事件</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
