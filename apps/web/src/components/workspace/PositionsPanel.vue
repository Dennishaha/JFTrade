<script setup lang="ts">
import { computed, ref } from "vue";

import { formatDateTime } from "../../composables/consoleDataFormatting";
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
  <section class="tv-panel tv-grid-area-positions">
    <div class="tv-panel-head">
      <div class="tv-seg">
        <button :class="{ 'is-active': tab === 'positions' }" @click="tab = 'positions'">Positions ({{ positions.length }})</button>
        <button :class="{ 'is-active': tab === 'orders' }" @click="tab = 'orders'">Broker Orders ({{ orders.length }})</button>
        <button :class="{ 'is-active': tab === 'executions' }" @click="tab = 'executions'">Execution ({{ execs.length }})</button>
        <button :class="{ 'is-active': tab === 'fills' }" @click="tab = 'fills'">Events ({{ events.length }})</button>
      </div>
      <div style="flex: 1"></div>
      <span style="color: var(--tv-text-dim); font-size: 11px">{{ brokerOrders.connectivity }}</span>
    </div>
    <div class="tv-panel-body is-flush">
      <table v-if="tab === 'positions'" class="tv-table">
        <thead>
          <tr>
            <th>Symbol</th><th>Market</th><th>Account</th><th>Env</th>
            <th class="tv-num">Qty</th><th class="tv-num">Avg</th><th class="tv-num">MV</th><th>Updated</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in positions" :key="`${p.brokerId}-${p.accountId}-${p.market}-${p.symbol}`">
            <td style="font-weight: 600">{{ p.symbol }}</td>
            <td>{{ p.market }}</td>
            <td style="color: var(--tv-text-muted)">{{ p.accountId }}</td>
            <td>{{ p.tradingEnvironment }}</td>
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
            <th>Order ID</th><th>Symbol</th><th>Side</th><th>Type</th><th>Status</th>
            <th class="tv-num">Qty</th><th class="tv-num">Filled</th><th class="tv-num">Price</th><th>Updated</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in orders" :key="o.brokerOrderId">
            <td style="font-family: monospace; font-size: 11px">{{ o.brokerOrderId }}</td>
            <td>{{ o.market }}:{{ o.symbol }}</td>
            <td :class="sideClass(o.side)" style="font-weight: 600">{{ o.side }}</td>
            <td>{{ o.orderType }}</td>
            <td>{{ o.status }}</td>
            <td class="tv-num">{{ o.quantity }}</td>
            <td class="tv-num">{{ o.filledQuantity ?? 0 }}</td>
            <td class="tv-num">{{ o.price ?? "—" }}</td>
            <td style="color: var(--tv-text-dim); font-size: 11px">{{ formatDateTime(o.updatedAt) }}</td>
          </tr>
          <tr v-if="orders.length === 0">
            <td colspan="9" style="text-align: center; padding: 24px; color: var(--tv-text-dim)">暂无 Broker 订单</td>
          </tr>
        </tbody>
      </table>

      <table v-else-if="tab === 'executions'" class="tv-table">
        <thead>
          <tr>
            <th>Internal ID</th><th>Symbol</th><th>Side</th><th>Status</th>
            <th class="tv-num">Req</th><th class="tv-num">Filled</th><th class="tv-num">Avg</th><th>Updated</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in execs" :key="o.internalOrderId">
            <td style="font-family: monospace; font-size: 11px">{{ o.internalOrderId }}</td>
            <td>{{ o.market }}:{{ o.symbol }}</td>
            <td :class="sideClass(o.side)" style="font-weight: 600">{{ o.side }}</td>
            <td>{{ o.status }}</td>
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
          <tr><th>Event</th><th>Prev</th><th>Next</th><th>Order</th><th>At</th></tr>
        </thead>
        <tbody>
          <tr v-for="ev in events" :key="ev.id">
            <td style="font-weight: 600">{{ ev.eventType }}</td>
            <td style="color: var(--tv-text-muted)">{{ ev.previousStatus ?? "—" }}</td>
            <td>{{ ev.nextStatus }}</td>
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
