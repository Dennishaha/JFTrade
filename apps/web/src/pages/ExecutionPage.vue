<script setup lang="ts">
import { computed, watch } from "vue";

import PageHeader from "../components/PageHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  brokerOrderFees,
  executionEventsError,
  executionOrderEvents,
  executionOrders,
  isLoadingExecutionEvents,
  isLoadingOrderFees,
  loadExecutionOrderDetails,
  orderFeesError,
  selectedExecutionOrder,
  selectedExecutionOrderId,
} = useConsoleData();

const preferredExecutionOrderId = computed(
  () =>
    executionOrders.value.orders.find(
      (order) => order.internalOrderId === selectedExecutionOrderId.value,
    )?.internalOrderId ?? executionOrders.value.orders[0]?.internalOrderId ?? "",
);

watch(
  [preferredExecutionOrderId, isLoadingExecutionEvents, isLoadingOrderFees],
  ([nextOrderId, loadingExecutionEvents, loadingOrderFees]) => {
    if (
      nextOrderId === "" ||
      loadingExecutionEvents ||
      loadingOrderFees ||
      executionOrderEvents.value.internalOrderId === nextOrderId
    ) {
      return;
    }

    void loadExecutionOrderDetails(nextOrderId);
  },
  { immediate: true },
);

const executionHeaderStats = computed(() => [
  {
    label: "Orders",
    value: executionOrders.value.orders.length,
  },
  {
    label: "Selected",
    value: selectedExecutionOrderId.value || "None",
    hint:
      selectedExecutionOrder.value?.symbol ??
      "Select an order to inspect the event stream.",
  },
  {
    label: "Events",
    value: executionOrderEvents.value.events.length,
  },
  {
    label: "Fees",
    value: brokerOrderFees.value.fees.length,
    hint:
      selectedExecutionOrder.value?.tradingEnvironment === "REAL"
        ? "REAL fee projection available."
        : "SIMULATE orders do not query broker fees.",
  },
]);
</script>

<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Execution blotter"
      title="Execution / Orders"
      description="把 execution 主订单、事件时间线和真实交易费用联动到一个订单工作区，先选中订单，再查看事件和 broker 费用。"
      :stats="executionHeaderStats"
    />

    <v-card flat class="card-shell border-0">
      <div class="px-4 pt-4 flex items-center justify-between gap-3">
        <div class="text-xl font-semibold text-slate-900">Execution Orders</div>
        <v-chip variant="outlined" size="small">{{ executionOrders.orders.length }}</v-chip>
      </div>

      <v-card-text>
      <div v-if="executionOrders.orders.length" class="grid gap-5 xl:grid-cols-[1.1fr_0.9fr]">
        <div class="grid gap-3 md:grid-cols-2">
          <div
            v-for="order in executionOrders.orders.slice(0, 6)"
            :key="order.internalOrderId"
            class="rounded-3xl border px-4 py-4 transition"
            :class="selectedExecutionOrderId === order.internalOrderId ? 'border-teal-400 bg-teal-50/60 shadow-teal-glow' : 'border-slate-200 bg-white'"
          >
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="text-base font-semibold text-slate-900">{{ order.symbol ?? 'Unknown Symbol' }}</div>
                <div class="mt-1 text-sm text-slate-500">{{ order.internalOrderId }}</div>
              </div>
              <v-chip variant="outlined" size="small">{{ order.status }}</v-chip>
            </div>
            <div class="mt-4 grid gap-3 sm:grid-cols-2">
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Order</div>
                <div class="mt-2 truncate text-sm font-semibold text-slate-900">{{ order.brokerOrderId ?? 'Pending' }}</div>
              </div>
              <div class="rounded-2xl bg-slate-50 px-3 py-3">
                <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Filled</div>
                <div class="mt-2 text-lg font-semibold text-slate-900">{{ order.filledQuantity ?? 0 }}</div>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between gap-3 text-xs text-slate-500">
              <span>{{ order.updatedAt }}</span>
              <button
                type="button"
                class="rounded-full border border-slate-300 px-3 py-1 font-medium text-slate-700 transition hover:border-teal-500 hover:text-teal-700"
                @click="loadExecutionOrderDetails(order.internalOrderId)"
              >
                {{ selectedExecutionOrderId === order.internalOrderId ? '当前事件' : '查看事件' }}
              </button>
            </div>
          </div>
        </div>

        <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
          <div class="flex items-center justify-between gap-3">
            <div>
              <div class="text-base font-semibold text-slate-900">Execution Events</div>
              <div class="mt-1 text-sm text-slate-500">{{ selectedExecutionOrderId || '请选择 execution order' }}</div>
            </div>
            <v-chip variant="outlined" size="small">{{ executionOrderEvents.events.length }}</v-chip>
          </div>

          <div v-if="isLoadingExecutionEvents" class="mt-4 text-sm text-slate-500">正在加载 execution order events...</div>

          <v-alert
            v-else-if="executionEventsError"
            class="mt-4"
            type="warning"
            :closable="false"
            title="Execution Events Warning"
          >
            {{ executionEventsError }}
          </v-alert>

          <div v-else-if="executionOrderEvents.events.length" class="mt-4 grid gap-3">
            <div
              v-for="event in executionOrderEvents.events.slice(0, 8)"
              :key="event.id"
              class="rounded-2xl bg-slate-50 px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="text-sm font-semibold text-slate-900">{{ event.eventType }}</div>
                <div class="text-xs text-slate-500">{{ event.createdAt }}</div>
              </div>
              <div class="mt-2 text-sm text-slate-600">{{ event.previousStatus ?? 'N/A' }} -> {{ event.nextStatus }}</div>
            </div>
          </div>

          <v-empty-state v-else text="当前 execution order 暂无事件。" class="mt-4" />

          <div class="mt-5 border-t border-slate-200 pt-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="text-base font-semibold text-slate-900">Broker Order Fees</div>
                <div class="mt-1 text-sm text-slate-500">{{ selectedExecutionOrder?.brokerOrderId ?? 'Pending broker order id' }}</div>
              </div>
              <v-chip variant="outlined" size="small">{{ brokerOrderFees.fees.length }}</v-chip>
            </div>

            <v-empty-state
                   v-if="selectedExecutionOrder?.tradingEnvironment !== 'REAL'"
                   text="当前仅在真实交易环境下查询券商订单费用。"
                   class="mt-4"
                 />

            <div v-else-if="isLoadingOrderFees" class="mt-4 text-sm text-slate-500">正在加载 broker order fees...</div>

            <v-alert
              v-else-if="orderFeesError"
              class="mt-4"
              type="warning"
              :closable="false"
              title="Broker Order Fees Warning"
            >
              {{ orderFeesError }}
            </v-alert>

            <div v-else-if="brokerOrderFees.fees.length" class="mt-4 grid gap-3">
              <div
                v-for="fee in brokerOrderFees.fees"
                :key="fee.brokerOrderId"
                class="rounded-2xl bg-slate-50 px-4 py-3"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="text-sm font-semibold text-slate-900">{{ fee.brokerOrderId }}</div>
                  <div class="text-sm font-medium text-slate-700">{{ fee.totalFee ?? 'N/A' }} {{ fee.currency ?? '' }}</div>
                </div>
                <div v-if="fee.details.length" class="mt-3 flex flex-wrap gap-2">
                  <span
                    v-for="detail in fee.details"
                    :key="`${fee.brokerOrderId}-${detail.title}`"
                    class="rounded-full border border-slate-200 bg-white px-2 py-1 text-xs font-medium text-slate-700"
                  >
                    {{ detail.title }}: {{ detail.amount ?? 'N/A' }}
                  </span>
                </div>
              </div>
            </div>

            <v-empty-state v-else text="当前订单暂无可展示的券商费用数据。" class="mt-4" />
          </div>
        </div>
      </div>

      <v-empty-state v-else text="当前还没有 execution 订单状态。" />
      </v-card-text>
    </v-card>
  </section>
</template>