<script setup lang="ts">
import { computed, nextTick, ref } from "vue";

import { useConsoleData } from "../composables/useConsoleData";
import { useWorkspaceLayout } from "../composables/useWorkspaceLayout";

interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
}

const messages = ref<Message[]>([
  {
    id: "welcome",
    role: "assistant",
    content:
      "嗨，我是 JFTrade 助手。可以问我当前自选行情、风控状态、订单情况等问题。例如：「当前持仓盈亏」、「风控是否启用」、「最近的审计动作」。",
  },
]);

const draft = ref("");
const busy = ref(false);
const scrollHost = ref<HTMLElement | null>(null);

const {
  selectedBrokerAccount,
  systemStatus,
  realTradeKillSwitchState,
  realTradeRiskState,
  brokerOrders,
  portfolioPositions,
  storageOverview,
  marketDataSubscriptions,
} = useConsoleData();
const { prefs } = useWorkspaceLayout();

const suggestions = computed(() => [
  "当前交易环境",
  "风控状态",
  "持仓概览",
  "最近订单",
  "活跃行情订阅",
]);

function localReply(question: string): string {
  const q = question.toLowerCase();
  if (q.includes("环境") || q.includes("env") || q.includes("trading")) {
    return selectedBrokerAccount.value == null
      ? `当前默认交易环境：${systemStatus.value.defaultTradingEnvironment}，券商层：${systemStatus.value.broker.displayName}。`
      : `当前查询作用域：${selectedBrokerAccount.value.brokerId} / ${selectedBrokerAccount.value.accountId} / ${selectedBrokerAccount.value.tradingEnvironment} / ${selectedBrokerAccount.value.market}。`;
  }
  if (q.includes("风控") || q.includes("kill") || q.includes("risk")) {
    return `Kill switch：${realTradeKillSwitchState.value.killSwitchActive ? "ACTIVE ⚠" : "clear"}；风控限额：${realTradeRiskState.value.riskEnabled ? "已启用" : "未启用"}。`;
  }
  if (q.includes("持仓") || q.includes("position") || q.includes("portfolio")) {
    const n = portfolioPositions.value.positions.length;
    return `投影层共 ${n} 条持仓记录。打开 Portfolio 页查看明细。`;
  }
  if (q.includes("订单") || q.includes("order")) {
    const n = brokerOrders.value.orders.length;
    return `Broker 订单视图共 ${n} 条记录。可在 Execution / Broker 页查看详情。`;
  }
  if (q.includes("订阅") || q.includes("sub") || q.includes("market")) {
    return `当前行情活跃订阅 ${marketDataSubscriptions.value.totalActiveSubscriptions} 条，配额 ${marketDataSubscriptions.value.quota.totalUsed}/${marketDataSubscriptions.value.quota.totalLimit ?? "∞"}。`;
  }
  if (q.includes("审计") || q.includes("audit") || q.includes("log")) {
    const n = storageOverview.value.recentAuditLogs.length;
    return `最近 ${n} 条审计日志可在右栏 / System 页查看。`;
  }
  if (q.includes("标的") || q.includes("symbol")) {
    return `当前工作区标的：${prefs.value.market}:${prefs.value.symbol}，周期 ${prefs.value.period}。`;
  }
  return "（本地助手）目前我可以回答关于交易环境、风控、持仓、订单、订阅、审计的问题。后续会接入大模型 / 策略生成能力。";
}

async function tryRemote(question: string): Promise<string | null> {
  // Try to call an optional backend AI endpoint, fall back gracefully.
  try {
    const res = await fetch("/api/v1/assistant/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: question }),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { reply?: string };
    return data.reply ?? null;
  } catch {
    return null;
  }
}

async function send(): Promise<void> {
  const text = draft.value.trim();
  if (!text || busy.value) return;
  draft.value = "";
  messages.value.push({ id: `u-${Date.now()}`, role: "user", content: text });
  busy.value = true;

  const remote = await tryRemote(text);
  const reply = remote ?? localReply(text);
  messages.value.push({
    id: `a-${Date.now()}`,
    role: "assistant",
    content: reply,
  });
  busy.value = false;
  await nextTick();
  if (scrollHost.value) {
    scrollHost.value.scrollTop = scrollHost.value.scrollHeight;
  }
}

function pick(s: string): void {
  draft.value = s;
  void send();
}
</script>

<template>
  <div style="display: flex; flex-direction: column; height: 100%; min-height: 0">
    <div ref="scrollHost" style="flex: 1; overflow-y: auto; padding: 10px">
      <div v-for="m in messages" :key="m.id" class="tv-ai-msg" :class="{ 'is-user': m.role === 'user' }">
        <div style="font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--tv-text-muted); margin-bottom: 4px">
          {{ m.role === "user" ? "You" : "Assistant" }}
        </div>
        <div>{{ m.content }}</div>
      </div>
    </div>
    <div style="border-top: 1px solid var(--tv-border); padding: 8px 10px">
      <div style="display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 6px">
        <button
          v-for="s in suggestions"
          :key="s"
          class="tv-btn tv-btn-ghost"
          style="height: 22px; font-size: 10px; padding: 0 8px"
          @click="pick(s)"
        >
          {{ s }}
        </button>
      </div>
      <form style="display: flex; gap: 6px" @submit.prevent="send">
        <input
          v-model="draft"
          class="tv-input"
          placeholder="问点什么…"
          :disabled="busy"
        />
        <button type="submit" class="tv-btn" :disabled="busy || !draft.trim()">
          {{ busy ? "…" : "发送" }}
        </button>
      </form>
    </div>
  </div>
</template>
