<script setup lang="ts">
import { computed, ref } from "vue";

import {
  formatOrderSideLabel,
  formatOrderTypeLabel,
  formatTimeInForceLabel,
} from "../../composables/consoleDataFormatting";
import { useConsoleData } from "../../composables/useConsoleData";
import { useNotifications } from "../../composables/useNotifications";
import { useWorkspaceLayout } from "../../composables/useWorkspaceLayout";

const { selectedBrokerAccount, systemStatus } = useConsoleData();
const { prefs } = useWorkspaceLayout();
const notifications = useNotifications();

type Side = "BUY" | "SELL";
type OrderType = "LIMIT" | "MARKET" | "STOP" | "STOP_LIMIT";
type TIF = "DAY" | "GTC" | "IOC" | "FOK";

const side = ref<Side>("BUY");
const orderType = ref<OrderType>("LIMIT");
const tif = ref<TIF>("DAY");
const quantity = ref<number>(100);
const price = ref<number>(0);
const stopPrice = ref<number>(0);
const submitting = ref(false);

const isRealMode = computed(
  () =>
    (selectedBrokerAccount.value?.tradingEnvironment ??
      systemStatus.value.defaultTradingEnvironment) === "REAL",
);
const isStop = computed(
  () => orderType.value === "STOP" || orderType.value === "STOP_LIMIT",
);
const isLimit = computed(
  () => orderType.value === "LIMIT" || orderType.value === "STOP_LIMIT",
);

function estimate(): string {
  const px = isLimit.value ? price.value : 0;
  if (!px || !quantity.value) return "—";
  return (px * quantity.value).toFixed(2);
}

async function submit(): Promise<void> {
  if (submitting.value) return;
  if (!quantity.value || quantity.value <= 0) {
    notifications.push({
      level: "warn",
      title: "数量无效",
      source: "order-entry",
    });
    return;
  }
  if (isLimit.value && !price.value) {
    notifications.push({
      level: "warn",
      title: "限价单需要价格",
      source: "order-entry",
    });
    return;
  }

  submitting.value = true;
  try {
    const payload = {
      market: prefs.value.market,
      symbol: prefs.value.symbol,
      side: side.value,
      orderType: orderType.value,
      timeInForce: tif.value,
      quantity: quantity.value,
      price: isLimit.value ? price.value : undefined,
      stopPrice: isStop.value ? stopPrice.value : undefined,
      env:
        selectedBrokerAccount.value?.tradingEnvironment ??
        systemStatus.value.defaultTradingEnvironment,
    };

    let placedRemotely = false;
    let successMessage = `已提交订单（${formatOrderTypeLabel(orderType.value)}，${formatTimeInForceLabel(tif.value)}）`;
    let failureMessage = "下单请求未送达后端。";
    try {
      const res = await fetch("/api/v1/execution/orders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      placedRemotely = res.ok;
      if (res.ok) {
        const body = (await res.json().catch(() => null)) as {
          data?: {
            internalOrderId?: string | null;
            brokerOrderId?: string | null;
          };
        } | null;
        const brokerOrderId = body?.data?.brokerOrderId?.trim();
        const internalOrderId = body?.data?.internalOrderId?.trim();
        if (brokerOrderId) {
          successMessage = `已提交订单，券商单号 ${brokerOrderId}`;
        } else if (internalOrderId) {
          successMessage = `已提交订单，内部单号 ${internalOrderId}`;
        }
      } else {
        const body = (await res.json().catch(() => null)) as {
          error?: { message?: string };
        } | null;
        failureMessage =
          body?.error?.message?.trim() || `下单失败，HTTP ${res.status}`;
      }
    } catch {
      placedRemotely = false;
    }

    notifications.push({
      level: placedRemotely ? "success" : "warn",
      title: `${formatOrderSideLabel(side.value)} ${quantity.value} ${prefs.value.market}:${prefs.value.symbol}`,
      message: placedRemotely
        ? successMessage
        : failureMessage,
      source: "order-entry",
    });
  } finally {
    submitting.value = false;
  }
}

function setSide(s: Side): void {
  side.value = s;
}
</script>

<template>
  <section class="tv-panel tv-grid-area-order">
    <div class="tv-panel-head">
      <span class="tv-panel-title">下单</span>
      <span style="color: var(--tv-text); font-weight: 600">{{ prefs.market }}:{{ prefs.symbol }}</span>
      <div style="flex: 1"></div>
      <span
        v-if="isRealMode"
        style="font-size: 10px; padding: 2px 6px; border-radius: 4px; background: var(--tv-down); color: #fff; font-weight: 600"
      >
        实盘
      </span>
    </div>
    <div class="tv-panel-body">
      <div class="tv-seg" style="width: 100%; margin-bottom: 10px">
        <button style="flex: 1" :class="{ 'is-active': side === 'BUY' }" @click="setSide('BUY')">买入</button>
        <button style="flex: 1" :class="{ 'is-active': side === 'SELL' }" @click="setSide('SELL')">卖出</button>
      </div>

      <div class="tv-form-row">
        <label>类型</label>
        <select v-model="orderType" class="tv-select">
          <option value="LIMIT">限价</option>
          <option value="MARKET">市价</option>
          <option value="STOP">止损</option>
          <option value="STOP_LIMIT">止损限价</option>
        </select>
      </div>

      <div class="tv-form-row">
        <label>数量</label>
        <input v-model.number="quantity" type="number" min="1" class="tv-input" />
      </div>

      <div v-if="isLimit" class="tv-form-row">
        <label>价格</label>
        <input v-model.number="price" type="number" step="0.01" class="tv-input" />
      </div>

      <div v-if="isStop" class="tv-form-row">
        <label>止损价</label>
        <input v-model.number="stopPrice" type="number" step="0.01" class="tv-input" />
      </div>

      <div class="tv-form-row">
        <label>有效期</label>
        <select v-model="tif" class="tv-select">
          <option value="DAY">当日有效</option>
          <option value="GTC">撤单前有效</option>
          <option value="IOC">立即成交剩余取消</option>
          <option value="FOK">全部成交否则取消</option>
        </select>
      </div>

      <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--tv-text-muted); margin: 4px 0 10px">
        <span>名义金额</span>
        <span class="tv-num" style="color: var(--tv-text)">{{ estimate() }}</span>
      </div>

      <button
        type="button"
        class="tv-btn"
        :class="side === 'BUY' ? 'tv-btn-buy' : 'tv-btn-sell'"
        style="width: 100%; height: 38px; font-weight: 600; letter-spacing: 0.04em"
        :disabled="submitting"
        @click="submit"
      >
        {{ submitting ? "提交中..." : `${formatOrderSideLabel(side)} ${prefs.symbol}` }}
      </button>
    </div>
  </section>
</template>
